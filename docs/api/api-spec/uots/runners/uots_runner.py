#!/usr/bin/env python3
"""UOTS runner.

Validates UOTS specs for:
- prometheus_metrics
- grafana_dashboard
- alert_rules

`log_format` and `trace_propagation` are intentionally explicit failures until
implemented to avoid false-pass behavior.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
import time
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple
from urllib.parse import urljoin

import requests

import sys as _sys
_sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', '..', '..', '..', 'tests'))
from uxts_report import build_result as _canonical_result, build_report as _canonical_report, print_summary as _print_summary, save_report as _save_report

ROOT = Path(__file__).resolve().parents[5]
UOTS_VERSION = "1.0.0"


# ============================================================
# PARITY CHECK (Section 8 — Portable Agent Spec v2.2.0)
# ============================================================

_UOTS_KNOWN_TOP_FIELDS = {"uots_version", "observability", "target", "expected", "config", "metadata"}
_UOTS_KNOWN_OBSERVABILITY_FIELDS = {"name", "type", "tags"}
_UOTS_KNOWN_TARGET_FIELDS = {"endpoint", "method", "file"}
_UOTS_KNOWN_CONFIG_FIELDS = {"timeout_ms", "base_url"}

def _validate_supported_features(spec_dict):
    """Detect spec fields this runner does not implement (parity check)."""
    errors = []
    unknown_top = set(spec_dict.keys()) - _UOTS_KNOWN_TOP_FIELDS
    if unknown_top:
        errors.append(f"PARITY FAILURE: Unimplemented top-level fields: {unknown_top}")
    if "variants" in spec_dict:
        errors.append("PARITY FAILURE: 'variants' field is not implemented by this runner")
    if "observability" in spec_dict and isinstance(spec_dict["observability"], dict):
        unknown_obs = set(spec_dict["observability"].keys()) - _UOTS_KNOWN_OBSERVABILITY_FIELDS
        if unknown_obs:
            errors.append(f"PARITY FAILURE: Unimplemented observability fields: {unknown_obs}")
    if "target" in spec_dict and isinstance(spec_dict["target"], dict):
        unknown_tgt = set(spec_dict["target"].keys()) - _UOTS_KNOWN_TARGET_FIELDS
        if unknown_tgt:
            errors.append(f"PARITY FAILURE: Unimplemented target fields: {unknown_tgt}")
    if "config" in spec_dict and isinstance(spec_dict["config"], dict):
        unknown_cfg = set(spec_dict["config"].keys()) - _UOTS_KNOWN_CONFIG_FIELDS
        if unknown_cfg:
            errors.append(f"PARITY FAILURE: Unimplemented config fields: {unknown_cfg}")
    return errors


@dataclass
class CheckResult:
    name: str
    passed: bool
    message: str
    details: Optional[Dict[str, Any]] = None


@dataclass
class UOTSResult:
    spec_path: str
    spec_name: str
    test_type: str
    passed: bool
    total_checks: int
    passed_checks: int
    failed_checks: int
    started_at: str
    ended_at: str
    checks: List[CheckResult] = field(default_factory=list)

    def to_dict(self) -> Dict[str, Any]:
        return {
            "spec_path": self.spec_path,
            "spec_name": self.spec_name,
            "test_type": self.test_type,
            "passed": self.passed,
            "total_checks": self.total_checks,
            "passed_checks": self.passed_checks,
            "failed_checks": self.failed_checks,
            "started_at": self.started_at,
            "ended_at": self.ended_at,
            "checks": [
                {
                    "name": c.name,
                    "passed": c.passed,
                    "message": c.message,
                    "details": c.details,
                }
                for c in self.checks
            ],
        }


def _now_iso() -> str:
    return datetime.now().isoformat()


def _load_json(path: Path) -> Dict[str, Any]:
    with path.open("r", encoding="utf-8") as f:
        return json.load(f)


def _resolve_env_vars(value: str) -> str:
    pattern = re.compile(r"\$\{([A-Za-z_][A-Za-z0-9_]*)\}")

    def repl(match: re.Match[str]) -> str:
        name = match.group(1)
        return os.environ.get(name, match.group(0))

    return pattern.sub(repl, value)


def _resolve_target_file(spec_path: Path, file_value: str) -> Path:
    raw = _resolve_env_vars(file_value)
    candidate = Path(raw)
    if candidate.is_absolute():
        return candidate

    root_relative = ROOT / candidate
    if root_relative.exists():
        return root_relative

    return spec_path.parent / candidate


def _parse_prometheus_text(content: str) -> tuple[Dict[str, str], Dict[str, str], Dict[str, List[Tuple[Dict[str, str], float]]]]:
    types: Dict[str, str] = {}
    helps: Dict[str, str] = {}
    samples: Dict[str, List[Tuple[Dict[str, str], float]]] = {}

    type_re = re.compile(r"^#\s*TYPE\s+([A-Za-z_:][A-Za-z0-9_:]*)\s+(\w+)\s*$")
    help_re = re.compile(r"^#\s*HELP\s+([A-Za-z_:][A-Za-z0-9_:]*)\s+(.+)$")
    sample_re = re.compile(r"^([A-Za-z_:][A-Za-z0-9_:]*)(\{[^}]*\})?\s+([+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?)")

    for raw in content.splitlines():
        line = raw.strip()
        if not line:
            continue

        m_type = type_re.match(line)
        if m_type:
            types[m_type.group(1)] = m_type.group(2)
            continue

        m_help = help_re.match(line)
        if m_help:
            helps[m_help.group(1)] = m_help.group(2)
            continue

        if line.startswith("#"):
            continue

        m_sample = sample_re.match(line)
        if not m_sample:
            continue

        metric_name = m_sample.group(1)
        labels_blob = m_sample.group(2)
        value = float(m_sample.group(3))

        labels: Dict[str, str] = {}
        if labels_blob:
            inner = labels_blob[1:-1].strip()
            if inner:
                # Commas in quoted values are rare for these specs; keep parser simple.
                for kv in inner.split(","):
                    if "=" not in kv:
                        continue
                    key, raw_val = kv.split("=", 1)
                    labels[key.strip()] = raw_val.strip().strip('"')

        samples.setdefault(metric_name, []).append((labels, value))

    return types, helps, samples


def _build_result(spec_path: Path, spec: Dict[str, Any], test_type: str, checks: List[CheckResult], started_at: str) -> UOTSResult:
    passed_checks = sum(1 for c in checks if c.passed)
    failed_checks = sum(1 for c in checks if not c.passed)
    total_checks = len(checks)
    return UOTSResult(
        spec_path=str(spec_path),
        spec_name=spec.get("observability", {}).get("name", spec_path.name),
        test_type=test_type,
        passed=failed_checks == 0,
        total_checks=total_checks,
        passed_checks=passed_checks,
        failed_checks=failed_checks,
        started_at=started_at,
        ended_at=_now_iso(),
        checks=checks,
    )


def _run_prometheus_metrics(spec_path: Path, spec: Dict[str, Any], cli_base_url: str) -> UOTSResult:
    started = _now_iso()
    checks: List[CheckResult] = []

    target = spec.get("target", {})
    config = spec.get("config", {})
    endpoint = target.get("endpoint", "/v1/prometheus")
    method = target.get("method", "GET")
    timeout_ms = int(config.get("timeout_ms", 5000))

    spec_base_url = _resolve_env_vars(str(config.get("base_url", ""))).strip()
    base_url = cli_base_url or spec_base_url or os.environ.get("MDEMG_BASE_URL", "http://localhost:9999")

    try:
        response = requests.request(
            method=method,
            url=urljoin(base_url, endpoint),
            timeout=max(timeout_ms / 1000.0, 1.0),
        )
    except requests.RequestException as exc:
        checks.append(CheckResult("fetch_metrics", False, f"request failed: {exc}"))
        return _build_result(spec_path, spec, "prometheus_metrics", checks, started)

    if response.status_code != 200:
        checks.append(
            CheckResult(
                "fetch_metrics",
                False,
                f"expected HTTP 200 from {endpoint}, got {response.status_code}",
            )
        )
        return _build_result(spec_path, spec, "prometheus_metrics", checks, started)

    checks.append(CheckResult("fetch_metrics", True, f"HTTP 200 from {endpoint}"))

    types, helps, samples = _parse_prometheus_text(response.text)

    for metric in spec.get("expected", {}).get("metrics", []):
        name = metric.get("name")
        if not name:
            checks.append(CheckResult("metric_<missing_name>", False, "metric assertion missing name"))
            continue

        metric_samples = samples.get(name, [])
        if not metric_samples:
            checks.append(CheckResult(f"metric_{name}", False, f"metric {name} not found"))
            continue

        passed = True
        messages: List[str] = []

        expected_type = metric.get("type")
        if expected_type:
            actual_type = types.get(name)
            if actual_type != expected_type:
                passed = False
                messages.append(f"type mismatch: expected {expected_type}, got {actual_type}")

        help_contains = metric.get("help_contains")
        if help_contains:
            help_text = helps.get(name, "")
            if help_contains.lower() not in help_text.lower():
                passed = False
                messages.append(f"HELP missing substring '{help_contains}'")

        expected_labels = metric.get("labels", [])
        if expected_labels:
            label_union = set()
            for labels, _ in metric_samples:
                label_union.update(labels.keys())
            missing_labels = [lbl for lbl in expected_labels if lbl not in label_union]
            if missing_labels:
                passed = False
                messages.append(f"missing labels: {', '.join(sorted(missing_labels))}")

        value_range = metric.get("value_range", {})
        if value_range:
            values = [v for _, v in metric_samples]
            min_v = value_range.get("min")
            max_v = value_range.get("max")
            if min_v is not None and any(v < min_v for v in values):
                passed = False
                messages.append(f"value below min {min_v}")
            if max_v is not None and any(v > max_v for v in values):
                passed = False
                messages.append(f"value above max {max_v}")

        if not messages:
            messages.append("metric assertion passed")

        checks.append(
            CheckResult(
                name=f"metric_{name}",
                passed=passed,
                message="; ".join(messages),
                details={"samples": len(metric_samples)},
            )
        )

    return _build_result(spec_path, spec, "prometheus_metrics", checks, started)


def _extract_panel_titles(panels: List[Any]) -> List[str]:
    titles: List[str] = []
    for panel in panels:
        if not isinstance(panel, dict):
            continue
        title = panel.get("title")
        if isinstance(title, str) and title:
            titles.append(title)
    return titles


def _extract_panel_queries(panels: List[Any]) -> List[str]:
    queries: List[str] = []
    for panel in panels:
        if not isinstance(panel, dict):
            continue
        targets = panel.get("targets", [])
        if not isinstance(targets, list):
            continue
        for target in targets:
            if not isinstance(target, dict):
                continue
            for key in ("expr", "query", "expression"):
                value = target.get(key)
                if isinstance(value, str) and value.strip():
                    queries.append(value)
    return queries


def _run_grafana_dashboard(spec_path: Path, spec: Dict[str, Any]) -> UOTSResult:
    started = _now_iso()
    checks: List[CheckResult] = []

    file_value = spec.get("target", {}).get("file")
    if not isinstance(file_value, str) or not file_value:
        checks.append(CheckResult("target_file", False, "target.file is required for grafana_dashboard"))
        return _build_result(spec_path, spec, "grafana_dashboard", checks, started)

    dashboard_path = _resolve_target_file(spec_path, file_value)
    if not dashboard_path.exists():
        checks.append(CheckResult("target_file", False, f"dashboard file not found: {dashboard_path}"))
        return _build_result(spec_path, spec, "grafana_dashboard", checks, started)

    try:
        dashboard_doc = _load_json(dashboard_path)
    except Exception as exc:
        checks.append(CheckResult("parse_dashboard", False, f"invalid dashboard JSON: {exc}"))
        return _build_result(spec_path, spec, "grafana_dashboard", checks, started)

    dashboard = dashboard_doc.get("dashboard", dashboard_doc)
    if not isinstance(dashboard, dict):
        checks.append(CheckResult("dashboard_shape", False, "dashboard JSON must be object"))
        return _build_result(spec_path, spec, "grafana_dashboard", checks, started)

    checks.append(CheckResult("target_file", True, f"loaded {dashboard_path}"))

    expected = spec.get("expected", {}).get("dashboard", {})

    uid = expected.get("uid")
    if uid is not None:
        actual_uid = dashboard.get("uid")
        checks.append(CheckResult("dashboard_uid", actual_uid == uid, f"expected {uid}, got {actual_uid}"))

    title = expected.get("title")
    if title is not None:
        actual_title = dashboard.get("title")
        checks.append(CheckResult("dashboard_title", actual_title == title, f"expected {title}, got {actual_title}"))

    panels = dashboard.get("panels", [])
    if not isinstance(panels, list):
        panels = []

    panel_count_min = expected.get("panel_count_min")
    if panel_count_min is not None:
        checks.append(
            CheckResult(
                "panel_count",
                len(panels) >= int(panel_count_min),
                f"panel count {len(panels)} (min {panel_count_min})",
            )
        )

    panel_titles = _extract_panel_titles(panels)
    required_titles = expected.get("required_panel_titles", [])
    if required_titles:
        missing_titles = [t for t in required_titles if t not in panel_titles]
        checks.append(
            CheckResult(
                "panel_titles",
                len(missing_titles) == 0,
                "all required panel titles present" if not missing_titles else f"missing: {', '.join(missing_titles)}",
            )
        )

    required_queries = expected.get("required_queries", [])
    if required_queries:
        available_queries = _extract_panel_queries(panels)
        missing_queries = [
            query for query in required_queries
            if not any(query in candidate for candidate in available_queries)
        ]
        checks.append(
            CheckResult(
                "dashboard_queries",
                len(missing_queries) == 0,
                "all required queries present" if not missing_queries else f"missing: {', '.join(missing_queries)}",
                details={"queries_found": len(available_queries)},
            )
        )

    required_tags = expected.get("tags", [])
    if required_tags:
        tags = dashboard.get("tags", [])
        if not isinstance(tags, list):
            tags = []
        missing_tags = [t for t in required_tags if t not in tags]
        checks.append(
            CheckResult(
                "dashboard_tags",
                len(missing_tags) == 0,
                "all required tags present" if not missing_tags else f"missing: {', '.join(missing_tags)}",
            )
        )

    return _build_result(spec_path, spec, "grafana_dashboard", checks, started)


def _parse_alerts_yaml(content: str) -> List[Dict[str, Any]]:
    alerts: List[Dict[str, Any]] = []
    current: Optional[Dict[str, Any]] = None

    lines = content.splitlines()
    i = 0
    while i < len(lines):
        raw = lines[i]
        stripped = raw.strip()

        if stripped.startswith("- alert:"):
            if current is not None:
                alerts.append(current)
            current = {
                "name": stripped.split(":", 1)[1].strip(),
                "expr": "",
                "for": "",
                "labels": {},
            }
            i += 1
            continue

        if current is None:
            i += 1
            continue

        if stripped.startswith("expr:"):
            expr_value = stripped.split(":", 1)[1].strip()
            if expr_value in {"|", ">", "|-", ">-"}:
                base_indent = len(raw) - len(raw.lstrip(" "))
                block: List[str] = []
                j = i + 1
                while j < len(lines):
                    nxt = lines[j]
                    nxt_strip = nxt.strip()
                    nxt_indent = len(nxt) - len(nxt.lstrip(" "))
                    if nxt_strip and nxt_indent <= base_indent:
                        break
                    if nxt_strip:
                        block.append(nxt_strip)
                    j += 1
                current["expr"] = " ".join(block).strip()
                i = j
                continue
            current["expr"] = expr_value
            i += 1
            continue

        if stripped.startswith("for:"):
            current["for"] = stripped.split(":", 1)[1].strip()
            i += 1
            continue

        if stripped == "labels:":
            base_indent = len(raw) - len(raw.lstrip(" "))
            j = i + 1
            while j < len(lines):
                nxt = lines[j]
                nxt_strip = nxt.strip()
                nxt_indent = len(nxt) - len(nxt.lstrip(" "))
                if nxt_strip and nxt_indent <= base_indent:
                    break
                if ":" in nxt_strip:
                    key, value = nxt_strip.split(":", 1)
                    current["labels"][key.strip()] = value.strip().strip('"')
                j += 1
            i = j
            continue

        i += 1

    if current is not None:
        alerts.append(current)

    return alerts


def _run_alert_rules(spec_path: Path, spec: Dict[str, Any]) -> UOTSResult:
    started = _now_iso()
    checks: List[CheckResult] = []

    file_value = spec.get("target", {}).get("file")
    if not isinstance(file_value, str) or not file_value:
        checks.append(CheckResult("target_file", False, "target.file is required for alert_rules"))
        return _build_result(spec_path, spec, "alert_rules", checks, started)

    rules_path = _resolve_target_file(spec_path, file_value)
    if not rules_path.exists():
        checks.append(CheckResult("target_file", False, f"alert rules file not found: {rules_path}"))
        return _build_result(spec_path, spec, "alert_rules", checks, started)

    try:
        content = rules_path.read_text(encoding="utf-8")
    except Exception as exc:
        checks.append(CheckResult("read_rules", False, f"failed to read alert rules: {exc}"))
        return _build_result(spec_path, spec, "alert_rules", checks, started)

    parsed_alerts = _parse_alerts_yaml(content)
    by_name = {entry.get("name"): entry for entry in parsed_alerts if entry.get("name")}
    checks.append(CheckResult("target_file", True, f"loaded {rules_path} ({len(parsed_alerts)} alerts parsed)"))

    for assertion in spec.get("expected", {}).get("alerts", []):
        name = assertion.get("name")
        if not name:
            checks.append(CheckResult("alert_<missing_name>", False, "alert assertion missing name"))
            continue

        parsed = by_name.get(name)
        if not parsed:
            checks.append(CheckResult(f"alert_{name}", False, f"alert '{name}' not found"))
            continue

        passed = True
        messages: List[str] = []

        severity = assertion.get("severity")
        if severity is not None:
            actual_severity = parsed.get("labels", {}).get("severity")
            if actual_severity != severity:
                passed = False
                messages.append(f"severity mismatch: expected {severity}, got {actual_severity}")

        expr_contains = assertion.get("expr_contains")
        if expr_contains and expr_contains not in parsed.get("expr", ""):
            passed = False
            messages.append(f"expr missing substring '{expr_contains}'")

        for_duration = assertion.get("for_duration")
        if for_duration is not None and parsed.get("for") != for_duration:
            passed = False
            messages.append(f"for mismatch: expected {for_duration}, got {parsed.get('for')}")

        if not messages:
            messages.append("alert assertion passed")

        checks.append(
            CheckResult(
                name=f"alert_{name}",
                passed=passed,
                message="; ".join(messages),
            )
        )

    return _build_result(spec_path, spec, "alert_rules", checks, started)


def run_spec(spec_path: Path, base_url: str) -> UOTSResult:
    started = _now_iso()

    try:
        spec = _load_json(spec_path)
    except Exception as exc:
        check = CheckResult("load_spec", False, f"failed to load spec: {exc}")
        return UOTSResult(
            spec_path=str(spec_path),
            spec_name=spec_path.name,
            test_type="unknown",
            passed=False,
            total_checks=1,
            passed_checks=0,
            failed_checks=1,
            started_at=started,
            ended_at=_now_iso(),
            checks=[check],
        )

    test_type = spec.get("observability", {}).get("type")
    if test_type == "prometheus_metrics":
        return _run_prometheus_metrics(spec_path, spec, base_url)
    if test_type == "grafana_dashboard":
        return _run_grafana_dashboard(spec_path, spec)
    if test_type == "alert_rules":
        return _run_alert_rules(spec_path, spec)

    return _build_result(
        spec_path,
        spec,
        str(test_type or "unknown"),
        [
            CheckResult(
                "unsupported_type",
                False,
                f"UOTS type '{test_type}' is unimplemented. Implement runner support before marking this spec passing.",
            )
        ],
        started,
    )


def _status_icon(passed: bool) -> str:
    return "PASS" if passed else "FAIL"


def print_result(result: UOTSResult) -> None:
    print("\n" + "=" * 72)
    print(f"Spec:   {result.spec_name}")
    print(f"Type:   {result.test_type}")
    print(f"Status: {_status_icon(result.passed)}")
    print(f"Checks: {result.passed_checks}/{result.total_checks} passed")
    print("-" * 72)
    for check in result.checks:
        prefix = "[OK]" if check.passed else "[XX]"
        print(f"{prefix} {check.name}: {check.message}")


def save_report(report_path: Path, results: List[UOTSResult]) -> None:
    data = {
        "timestamp": _now_iso(),
        "summary": {
            "total_specs": len(results),
            "passed": sum(1 for r in results if r.passed),
            "failed": sum(1 for r in results if not r.passed),
        },
        "results": [r.to_dict() for r in results],
    }
    report_path.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")


def collect_specs(spec: Optional[Path], spec_dir: Optional[Path], pattern: str) -> List[Path]:
    if spec is not None:
        return [spec]
    if spec_dir is None:
        return []
    return sorted(spec_dir.glob(pattern))


def main() -> int:
    parser = argparse.ArgumentParser(description="UOTS runner")
    parser.add_argument("--spec", type=Path, help="single UOTS spec path")
    parser.add_argument("--spec-dir", type=Path, help="directory containing UOTS specs")
    parser.add_argument("--pattern", type=str, default="*.uots.json", help="glob pattern used with --spec-dir")
    parser.add_argument("--base-url", default=os.environ.get("MDEMG_BASE_URL", "http://localhost:9999"))
    parser.add_argument("--report", type=Path, help="optional JSON report path")
    args = parser.parse_args()

    if bool(args.spec) == bool(args.spec_dir):
        parser.error("provide exactly one of --spec or --spec-dir")

    specs = collect_specs(args.spec, args.spec_dir, args.pattern)
    if not specs:
        print("No specs matched.")
        return 1

    started = time.time()
    results = [run_spec(spec_path, args.base_url) for spec_path in specs]
    canonical_results = []

    for result in results:
        print_result(result)

        # Parity check
        try:
            spec_dict = _load_json(Path(result.spec_path))
            parity_errors = _validate_supported_features(spec_dict)
        except Exception:
            parity_errors = []

        # Build canonical result
        failures = []
        for c in result.checks:
            if not c.passed:
                failures.append(f"{c.name}: {c.message}")
        failures.extend(parity_errors)

        canonical_status = "pass" if result.passed and not parity_errors else "fail"
        # Compute duration from started_at/ended_at
        try:
            dt_start = datetime.fromisoformat(result.started_at)
            dt_end = datetime.fromisoformat(result.ended_at)
            duration_ms = (dt_end - dt_start).total_seconds() * 1000
        except Exception:
            duration_ms = 0.0

        canonical_results.append(_canonical_result(
            spec_path=result.spec_path,
            status=canonical_status,
            duration_ms=round(duration_ms, 2),
            hash_verified=None,  # UOTS has no hash implementation
            assertions_evaluated=result.total_checks,
            assertions_passed=result.passed_checks,
            failures=failures if failures else [],
        ))

    total_duration = (time.time() - started) * 1000
    report = _canonical_report("uots", UOTS_VERSION, canonical_results, duration_ms=round(total_duration, 2))
    _print_summary(report)

    if args.report:
        _save_report(report, str(args.report))

    failed = sum(1 for r in results if not r.passed)
    return 0 if failed == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
