"""ULTS spec preflight — asserts every spec has fields the benchmark runner requires.

Abort on any missing field with a per-spec diff. Writes a preflight_report.json
to the location named in configs/benchmark_phase10.yaml:output.preflight_report.

Usage:
    python -m neural.benchmarks.preflight --config configs/benchmark_phase10.yaml
"""
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any

try:
    import yaml
except ImportError:  # pragma: no cover
    print("PyYAML required: pip install pyyaml", file=sys.stderr)
    sys.exit(2)


def _load_config(path: Path) -> dict[str, Any]:
    with path.open() as fh:
        return yaml.safe_load(fh)


def _dotted_get(obj: dict[str, Any], dotted: str) -> Any:
    """Return value at dotted path or raise KeyError."""
    cur: Any = obj
    for part in dotted.split("."):
        if not isinstance(cur, dict) or part not in cur:
            raise KeyError(dotted)
        cur = cur[part]
    return cur


def check_spec(spec_path: Path, required_fields: list[str],
               allowed_groups: list[str],
               performance_floors: dict[str, dict[str, int]] | None = None,
               ) -> list[str]:
    """Return list of issue strings for a single spec (empty = spec OK)."""
    try:
        with spec_path.open() as fh:
            spec = json.load(fh)
    except (OSError, json.JSONDecodeError) as exc:
        return [f"unreadable: {exc}"]

    issues: list[str] = []

    for field in required_fields:
        try:
            value = _dotted_get(spec, field)
        except KeyError:
            issues.append(f"missing field: {field}")
            continue
        if value in (None, "", [], {}):
            issues.append(f"empty field: {field}")

    group = spec.get("sampling_group")
    if group is not None and group not in allowed_groups:
        issues.append(f"sampling_group '{group}' not in {allowed_groups}")

    reward_fns = spec.get("reward_functions")
    if isinstance(reward_fns, list) and not reward_fns:
        issues.append("reward_functions is empty list")

    # Performance-budget floors: think-mode tasks need more headroom for
    # chain-of-thought; tight budgets truncate and silently zero-credit.
    if performance_floors:
        perf = spec.get("performance") or {}
        prompt_cfg = spec.get("prompt") or {}
        is_think = bool(prompt_cfg.get("think_mode"))
        floor_key = "think_mode" if is_think else "default"
        floor = performance_floors.get(floor_key) or {}
        mt_floor = floor.get("max_tokens")
        lb_floor = floor.get("latency_budget_ms")
        mt = perf.get("max_tokens")
        lb = perf.get("latency_budget_ms")
        if mt_floor is not None and (mt is None or mt < mt_floor):
            issues.append(
                f"performance.max_tokens={mt} below {floor_key} floor {mt_floor} "
                f"(think_mode={is_think})"
            )
        if lb_floor is not None and (lb is None or lb < lb_floor):
            issues.append(
                f"performance.latency_budget_ms={lb} below {floor_key} floor "
                f"{lb_floor} (think_mode={is_think})"
            )

    return issues


def _probe_mlx_group(
    mlx_base_url: str, mlx_model_name: str, kwargs: dict[str, Any],
    timeout_s: float = 30.0,
) -> tuple[bool, str]:
    """Hit a live MLX /chat/completions with the resolved kwargs.

    Returns (ok, detail). ``ok=False`` means the server rejected or closed the
    connection — surface the detail so the operator can fix the recipe.
    """
    import urllib.error
    import urllib.request

    body = {
        "model": mlx_model_name,
        "messages": [{"role": "user", "content": "probe: reply with 'ok'"}],
        "max_tokens": 16,
        **kwargs,
    }
    data = json.dumps(body).encode("utf-8")
    req = urllib.request.Request(
        f"{mlx_base_url.rstrip('/')}/chat/completions",
        data=data,
        headers={"Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout_s) as resp:
            resp.read()
        return True, "ok"
    except urllib.error.HTTPError as e:
        return False, f"http {e.code}: {e.read()[:200].decode('utf-8', 'replace')}"
    except Exception as e:  # RemoteDisconnected, TimeoutError, etc.
        return False, f"{type(e).__name__}: {e}"


def run_preflight(
    config_path: Path,
    *,
    probe_mlx: bool = False,
    mlx_base_url: str | None = None,
    mlx_model_name: str | None = None,
) -> dict[str, Any]:
    config = _load_config(config_path)
    specs_dir = Path(config["ults"]["specs_dir"])
    required_fields = list(config["ults"]["required_spec_fields"])
    allowed_groups = list(config["ults"]["allowed_sampling_groups"])
    performance_floors = (config["ults"].get("performance_floors") or None)

    if not specs_dir.is_dir():
        return {
            "status": "fail",
            "reason": f"specs_dir not found: {specs_dir}",
            "specs_checked": 0,
            "issues": {},
        }

    spec_files = sorted(specs_dir.glob("*.ults.json"))
    issues_by_spec: dict[str, list[str]] = {}
    group_counts: dict[str, int] = {g: 0 for g in allowed_groups}

    for spec_path in spec_files:
        issues = check_spec(
            spec_path, required_fields, allowed_groups,
            performance_floors=performance_floors,
        )
        if issues:
            issues_by_spec[spec_path.name] = issues
        # count groups for reporting
        try:
            with spec_path.open() as fh:
                spec = json.load(fh)
            g = spec.get("sampling_group")
            if g in group_counts:
                group_counts[g] += 1
        except (OSError, json.JSONDecodeError):
            pass

    # Optional live MLX sampling probe — catches recipe/server incompatibilities
    # (e.g. top_k=-1 crashes MLX) at preflight instead of mid-benchmark.
    mlx_probes: dict[str, dict[str, Any]] = {}
    if probe_mlx and mlx_base_url and mlx_model_name:
        from neural.benchmarks.sampling_policy import resolve_sampling

        for group in allowed_groups:
            fake_spec = {"sampling_group": group}
            try:
                kwargs = resolve_sampling(fake_spec, config)
            except Exception as e:
                mlx_probes[group] = {"ok": False, "detail": f"resolve: {e}"}
                continue
            ok, detail = _probe_mlx_group(mlx_base_url, mlx_model_name, kwargs)
            mlx_probes[group] = {
                "ok": ok, "detail": detail, "sampling_kwargs": kwargs,
            }

    probe_failures = {g: p for g, p in mlx_probes.items() if not p["ok"]}
    status = "pass" if not issues_by_spec and not probe_failures else "fail"
    return {
        "status": status,
        "specs_checked": len(spec_files),
        "sampling_group_counts": group_counts,
        "issues": issues_by_spec,
        "mlx_probes": mlx_probes,
    }


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--config", type=Path,
                    default=Path("configs/benchmark_phase10.yaml"))
    ap.add_argument("--probe-mlx", action="store_true",
                    help="Hit a live MLX server to validate each sampling-group recipe")
    ap.add_argument("--mlx-base-url", default="http://127.0.0.1:8101/v1")
    ap.add_argument("--mlx-model-name", default=None,
                    help="MLX model id to probe (required with --probe-mlx)")
    args = ap.parse_args()

    if not args.config.exists():
        print(f"config not found: {args.config}", file=sys.stderr)
        return 2

    if args.probe_mlx and not args.mlx_model_name:
        print("--probe-mlx requires --mlx-model-name", file=sys.stderr)
        return 2

    report = run_preflight(
        args.config,
        probe_mlx=args.probe_mlx,
        mlx_base_url=args.mlx_base_url,
        mlx_model_name=args.mlx_model_name,
    )
    config = _load_config(args.config)
    out_path = Path(config["output"]["preflight_report"])
    out_path.parent.mkdir(parents=True, exist_ok=True)
    with out_path.open("w") as fh:
        json.dump(report, fh, indent=2, sort_keys=True)
    print(json.dumps(report, indent=2, sort_keys=True))
    return 0 if report["status"] == "pass" else 1


if __name__ == "__main__":
    sys.exit(main())
