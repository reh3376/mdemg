#!/usr/bin/env python3
"""
Universal Emergence Test Specification (UETS) Test Runner

Validates LLM emergence concept-naming quality against UETS specifications.
Model-agnostic — works with any OpenAI-compatible or Ollama endpoint.

Usage:
    # Validate single spec
    python uets_runner.py validate --spec specs/qwen2.5-72b-mlx.uets.json

    # Validate all specs
    python uets_runner.py validate-all --spec-dir specs/

    # Generate JSON report
    python uets_runner.py validate-all --spec-dir specs/ --report report.json

    # Add hashes to fixture references
    python uets_runner.py add-hashes --spec-dir specs/

    # Verify fixture hashes
    python uets_runner.py verify-hashes --spec-dir specs/

    # Extract clusters from Neo4j (requires NEO4J_PASS env)
    python uets_runner.py extract-clusters --output ../fixtures/clusters.json

Author: reh3376
Version: 1.0.0
"""

from __future__ import annotations

import json
import hashlib
import sys
import time
import os
from pathlib import Path
from dataclasses import dataclass, field
from typing import Optional, Any
from datetime import datetime
import argparse
import http.client
import urllib.parse


# ============================================================
# CONSTANTS
# ============================================================

UETS_VERSION = "1.0.0"

VALID_LABELS = {"pattern", "principle", "bridge", "concern", "workflow"}

EMERGENCE_SYSTEM_PROMPT = """You are an AI concept discovery engine analyzing a knowledge graph.

You have found a cluster of nodes frequently co-activated together but not
matching any known pattern category (not auth, config, temporal, UI, or comparisons).

Your task: Name the emergent concept they collectively represent.

## Rules
- Name MUST be concise (3-6 words), descriptive, domain-specific
- Description MUST explain WHY these nodes cluster (1-2 sentences)
- proposed_label MUST be one of: "pattern", "principle", "bridge", "concern", "workflow"
- Output ONLY valid JSON — no markdown, no preamble

{"name": "<concept>", "description": "<why>", "proposed_label": "<label>"}"""


# ============================================================
# HASH UTILITIES
# ============================================================

def compute_sha256(filepath: Path) -> str:
    """Compute SHA256 hash of a file."""
    sha256_hash = hashlib.sha256()
    with open(filepath, "rb") as f:
        for chunk in iter(lambda: f.read(8192), b""):
            sha256_hash.update(chunk)
    return sha256_hash.hexdigest()


def verify_fixture_hash(fixture_path: Path, expected_hash: Optional[str]) -> tuple[bool, str]:
    """Verify fixture integrity. Returns (is_valid, message)."""
    if expected_hash is None:
        return True, "no hash specified"
    actual = compute_sha256(fixture_path)
    if actual == expected_hash:
        return True, "hash verified"
    return False, f"hash mismatch: expected {expected_hash[:12]}..., got {actual[:12]}..."


# ============================================================
# DATA TYPES
# ============================================================

@dataclass
class ClusterMember:
    node_id: str
    name: str
    summary: str = ""
    path: str = ""
    layer: int = 0


@dataclass
class Cluster:
    cluster_id: int
    members: list[ClusterMember]
    total_weight: float = 0.0


@dataclass
class NamingResult:
    cluster_id: int
    raw_response: str
    latency_ms: float
    json_valid: bool = False
    name: str = ""
    description: str = ""
    proposed_label: str = ""
    label_valid: bool = False
    name_word_score: float = 0.0
    error: str = ""


@dataclass
class SpecResult:
    spec_path: str
    model_name: str
    status: str  # "pass" or "fail"
    total_clusters: int = 0
    json_valid_count: int = 0
    label_valid_count: int = 0
    name_quality_count: int = 0
    avg_latency_ms: float = 0.0
    json_valid_rate: float = 0.0
    label_valid_rate: float = 0.0
    name_quality_rate: float = 0.0
    hash_verified: bool = False
    duration_ms: float = 0.0
    failures: list[str] = field(default_factory=list)
    details: list[dict[str, Any]] = field(default_factory=list)


# ============================================================
# SPEC LOADER
# ============================================================

class UETSLoader:
    """Parse and validate UETS spec files."""

    @staticmethod
    def load(spec_path: Path) -> dict:
        with open(spec_path) as f:
            spec = json.load(f)

        # Validate required fields
        for req in ("uets_version", "model", "fixture", "expected"):
            if req not in spec:
                raise ValueError(f"Missing required field '{req}' in {spec_path}")

        if spec["uets_version"] != "1.0.0":
            raise ValueError(f"Unsupported UETS version: {spec['uets_version']}")

        model = spec["model"]
        for req in ("name", "endpoint", "model_id", "type"):
            if req not in model:
                raise ValueError(f"Missing model.{req} in {spec_path}")

        if model["type"] not in ("openai", "ollama"):
            raise ValueError(f"Invalid model.type '{model['type']}' in {spec_path}")

        return spec

    @staticmethod
    def load_clusters(spec: dict, spec_dir: Path) -> list[Cluster]:
        """Load cluster fixture data from spec."""
        fixture = spec["fixture"]
        if fixture["type"] == "inline":
            raw = fixture["clusters"]
        elif fixture["type"] == "file":
            fixture_path = (spec_dir / fixture["path"]).resolve()
            if not fixture_path.exists():
                raise FileNotFoundError(f"Fixture not found: {fixture_path}")
            with open(fixture_path) as f:
                raw = json.load(f)
        else:
            raise ValueError(f"Unknown fixture type: {fixture['type']}")

        clusters = []
        for c in raw:
            members = [
                ClusterMember(
                    node_id=m["node_id"],
                    name=m["name"],
                    summary=m.get("summary", ""),
                    path=m.get("path", ""),
                    layer=m.get("layer", 0),
                )
                for m in c["members"]
            ]
            clusters.append(Cluster(
                cluster_id=c["cluster_id"],
                members=members,
                total_weight=c.get("total_weight", 0.0),
            ))
        return clusters


# ============================================================
# MODEL INTERFACE
# ============================================================

class ModelInterface:
    """Send emergence naming requests to OpenAI-compatible or Ollama endpoints."""

    def __init__(self, model_cfg: dict, run_cfg: dict):
        self.endpoint = model_cfg["endpoint"]
        self.model_id = model_cfg["model_id"]
        self.api_type = model_cfg["type"]
        self.temperature = run_cfg.get("temperature", 0.3)
        self.max_tokens = run_cfg.get("max_tokens", 500)
        self.timeout_ms = run_cfg.get("timeout_ms", 30000)

    def _build_user_prompt(self, cluster: Cluster) -> str:
        lines = ["## Cluster Members"]
        for m in cluster.members:
            lines.append(
                f'[Node: name="{m.name}", path="{m.path}", summary="{m.summary}"]'
            )
        return "\n".join(lines)

    def call(self, cluster: Cluster) -> NamingResult:
        """Send a naming request and return the result."""
        user_prompt = self._build_user_prompt(cluster)

        start = time.monotonic()
        try:
            if self.api_type == "openai":
                raw = self._call_openai(user_prompt)
            else:
                raw = self._call_ollama(user_prompt)
            latency = (time.monotonic() - start) * 1000
        except Exception as e:
            latency = (time.monotonic() - start) * 1000
            return NamingResult(
                cluster_id=cluster.cluster_id,
                raw_response="",
                latency_ms=latency,
                error=str(e),
            )

        return self._parse_response(cluster.cluster_id, raw, latency)

    def _call_openai(self, user_prompt: str) -> str:
        """Call OpenAI-compatible chat/completions endpoint."""
        body = json.dumps({
            "model": self.model_id,
            "messages": [
                {"role": "system", "content": EMERGENCE_SYSTEM_PROMPT},
                {"role": "user", "content": user_prompt},
            ],
            "temperature": self.temperature,
            "max_tokens": self.max_tokens,
        }).encode()

        parsed = urllib.parse.urlparse(self.endpoint)
        # endpoint already includes /v1/chat/completions or similar
        host = parsed.hostname
        port = parsed.port or (443 if parsed.scheme == "https" else 80)
        path = parsed.path

        timeout_s = self.timeout_ms / 1000
        if parsed.scheme == "https":
            conn = http.client.HTTPSConnection(host, port, timeout=timeout_s)
        else:
            conn = http.client.HTTPConnection(host, port, timeout=timeout_s)

        try:
            conn.request("POST", path, body=body, headers={
                "Content-Type": "application/json",
            })
            resp = conn.getresponse()
            data = resp.read().decode()
            if resp.status != 200:
                raise RuntimeError(f"HTTP {resp.status}: {data[:200]}")
            result = json.loads(data)
            return result["choices"][0]["message"]["content"].strip()
        finally:
            conn.close()

    def _call_ollama(self, user_prompt: str) -> str:
        """Call Ollama /api/generate endpoint with JSON schema format."""
        schema = {
            "type": "object",
            "properties": {
                "name": {"type": "string"},
                "description": {"type": "string"},
                "proposed_label": {
                    "type": "string",
                    "enum": ["pattern", "principle", "bridge", "concern", "workflow"],
                },
            },
            "required": ["name", "description", "proposed_label"],
        }

        prompt = EMERGENCE_SYSTEM_PROMPT + "\n\n" + user_prompt
        body = json.dumps({
            "model": self.model_id,
            "prompt": prompt,
            "stream": False,
            "format": schema,
            "options": {"temperature": self.temperature},
        }).encode()

        parsed = urllib.parse.urlparse(self.endpoint)
        host = parsed.hostname
        port = parsed.port or 80
        path = parsed.path

        timeout_s = self.timeout_ms / 1000
        conn = http.client.HTTPConnection(host, port, timeout=timeout_s)

        try:
            conn.request("POST", path, body=body, headers={
                "Content-Type": "application/json",
            })
            resp = conn.getresponse()
            data = resp.read().decode()
            if resp.status != 200:
                raise RuntimeError(f"HTTP {resp.status}: {data[:200]}")
            result = json.loads(data)
            return result["response"].strip()
        finally:
            conn.close()

    @staticmethod
    def _strip_code_fence(s: str) -> str:
        """Remove markdown code fences."""
        s = s.strip()
        if s.startswith("```json"):
            s = s[7:]
        elif s.startswith("```"):
            s = s[3:]
        if s.endswith("```"):
            s = s[:-3]
        return s.strip()

    def _parse_response(self, cluster_id: int, raw: str, latency_ms: float) -> NamingResult:
        """Parse and validate the LLM response."""
        cleaned = self._strip_code_fence(raw)

        result = NamingResult(
            cluster_id=cluster_id,
            raw_response=raw,
            latency_ms=latency_ms,
        )

        try:
            parsed = json.loads(cleaned)
        except json.JSONDecodeError as e:
            result.error = f"invalid JSON: {e}"
            return result

        # Check required fields
        if not all(k in parsed for k in ("name", "description", "proposed_label")):
            result.error = f"missing required fields (got keys: {list(parsed.keys())})"
            return result

        result.json_valid = True
        result.name = str(parsed["name"]).strip()
        result.description = str(parsed["description"]).strip()
        result.proposed_label = str(parsed["proposed_label"]).strip().lower()

        # Validate label
        result.label_valid = result.proposed_label in VALID_LABELS

        # Score name quality (word count)
        word_count = len(result.name.split())
        if 3 <= word_count <= 6:
            result.name_word_score = 1.0
        elif word_count in (2, 7):
            result.name_word_score = 0.5
        else:
            result.name_word_score = 0.0

        return result


# ============================================================
# VALIDATOR
# ============================================================

class Validator:
    """Validate model output against UETS spec thresholds."""

    @staticmethod
    def validate(spec: dict, results: list[NamingResult]) -> SpecResult:
        """Compare results against expected thresholds."""
        thresholds = spec["expected"]["thresholds"]
        total = len(results)
        if total == 0:
            return SpecResult(
                spec_path="",
                model_name=spec["model"]["name"],
                status="fail",
                failures=["no clusters to evaluate"],
            )

        json_valid = sum(1 for r in results if r.json_valid)
        label_valid = sum(1 for r in results if r.label_valid)
        name_quality = sum(1 for r in results if r.name_word_score >= 1.0)
        avg_latency = sum(r.latency_ms for r in results) / total

        json_rate = json_valid / total
        label_rate = label_valid / total
        name_rate = name_quality / total

        failures = []

        # E1: JSON conformance
        req_json = thresholds.get("json_valid_rate", 0)
        if json_rate < req_json:
            failures.append(
                f"E1_JSON_CONFORMANCE: {json_rate:.1%} < {req_json:.1%} threshold"
            )

        # E2: Label constraint
        req_label = thresholds.get("label_valid_rate", 0)
        if label_rate < req_label:
            failures.append(
                f"E2_LABEL_CONSTRAINT: {label_rate:.1%} < {req_label:.1%} threshold"
            )

        # E3: Name quality
        req_name = thresholds.get("name_quality_rate")
        if req_name is not None and name_rate < req_name:
            failures.append(
                f"E3_NAME_QUALITY: {name_rate:.1%} < {req_name:.1%} threshold"
            )

        # E5: Latency
        req_latency = thresholds.get("max_avg_latency_ms")
        if req_latency is not None and avg_latency > req_latency:
            failures.append(
                f"E5_LATENCY: {avg_latency:.0f}ms > {req_latency:.0f}ms threshold"
            )

        details = []
        for r in results:
            d = {
                "cluster_id": r.cluster_id,
                "json_valid": r.json_valid,
                "label_valid": r.label_valid,
                "name": r.name,
                "proposed_label": r.proposed_label,
                "name_word_score": r.name_word_score,
                "latency_ms": round(r.latency_ms, 1),
            }
            if r.error:
                d["error"] = r.error
            details.append(d)

        return SpecResult(
            spec_path="",
            model_name=spec["model"]["name"],
            status="pass" if not failures else "fail",
            total_clusters=total,
            json_valid_count=json_valid,
            label_valid_count=label_valid,
            name_quality_count=name_quality,
            avg_latency_ms=round(avg_latency, 1),
            json_valid_rate=round(json_rate, 4),
            label_valid_rate=round(label_rate, 4),
            name_quality_rate=round(name_rate, 4),
            failures=failures,
            details=details,
        )


# ============================================================
# REPORTER
# ============================================================

class Reporter:
    """Format and display results."""

    RESET = "\033[0m"
    GREEN = "\033[32m"
    RED = "\033[31m"
    YELLOW = "\033[33m"
    BOLD = "\033[1m"

    @classmethod
    def print_result(cls, result: SpecResult) -> None:
        """Print a single spec result."""
        icon = f"{cls.GREEN}PASS{cls.RESET}" if result.status == "pass" else f"{cls.RED}FAIL{cls.RESET}"
        print(f"\n  [{icon}] {result.model_name}")
        print(f"         Clusters: {result.total_clusters}")
        print(f"         JSON valid:    {result.json_valid_count}/{result.total_clusters} ({result.json_valid_rate:.1%})")
        print(f"         Label valid:   {result.label_valid_count}/{result.total_clusters} ({result.label_valid_rate:.1%})")
        print(f"         Name quality:  {result.name_quality_count}/{result.total_clusters} ({result.name_quality_rate:.1%})")
        print(f"         Avg latency:   {result.avg_latency_ms:.0f}ms")

        if result.failures:
            for fail in result.failures:
                print(f"         {cls.RED}  {fail}{cls.RESET}")

    @classmethod
    def print_summary_table(cls, results: list[SpecResult]) -> None:
        """Print summary comparison table."""
        print(f"\n{cls.BOLD}{'Model':<25} | {'JSON%':>6} | {'Label%':>6} | {'Name%':>6} | {'Avg Latency':>12} | {'Status':>6}{cls.RESET}")
        print("-" * 80)
        for r in results:
            status = f"{cls.GREEN}PASS{cls.RESET}" if r.status == "pass" else f"{cls.RED}FAIL{cls.RESET}"
            print(
                f"{r.model_name:<25} | {r.json_valid_rate:>5.1%} | {r.label_valid_rate:>5.1%} | "
                f"{r.name_quality_rate:>5.1%} | {r.avg_latency_ms:>9.0f}ms | {status}"
            )

    @classmethod
    def print_overall(cls, results: list[SpecResult]) -> None:
        """Print overall pass/fail summary."""
        passed = sum(1 for r in results if r.status == "pass")
        total = len(results)
        rate = (passed / total * 100) if total > 0 else 0
        color = cls.GREEN if passed == total else cls.YELLOW if passed > 0 else cls.RED
        print(f"\n{cls.BOLD}Overall: {color}{passed}/{total} specs passed ({rate:.0f}%){cls.RESET}")

    @staticmethod
    def generate_report(results: list[SpecResult], output_path: Path) -> None:
        """Write JSON report file."""
        passed = sum(1 for r in results if r.status == "pass")
        report = {
            "timestamp": datetime.utcnow().isoformat() + "Z",
            "uets_version": UETS_VERSION,
            "summary": {
                "total_specs": len(results),
                "passed": passed,
                "failed": len(results) - passed,
                "pass_rate": round(passed / len(results) * 100, 2) if results else 0,
            },
            "results": [],
        }
        for r in results:
            report["results"].append({
                "spec_path": r.spec_path,
                "model_name": r.model_name,
                "status": r.status,
                "total_clusters": r.total_clusters,
                "json_valid_rate": r.json_valid_rate,
                "label_valid_rate": r.label_valid_rate,
                "name_quality_rate": r.name_quality_rate,
                "avg_latency_ms": r.avg_latency_ms,
                "hash_verified": r.hash_verified,
                "duration_ms": r.duration_ms,
                "failures": r.failures,
                "details": r.details,
            })
        with open(output_path, "w") as f:
            json.dump(report, f, indent=2)
        print(f"\nReport written to {output_path}")


# ============================================================
# CLUSTER EXTRACTION (from Neo4j)
# ============================================================

def extract_clusters_from_neo4j(
    uri: str, user: str, password: str,
    space_id: str = "mdemg-dev",
    min_weight: float = 0.1,
) -> list[dict]:
    """Query Neo4j for CO_ACTIVATED_WITH clusters and run union-find."""
    try:
        from neo4j import GraphDatabase
    except ImportError:
        print("Error: neo4j package required. Install: pip install neo4j", file=sys.stderr)
        sys.exit(1)

    query = """
    MATCH (a:MemoryNode {space_id: $spaceId})-[r:CO_ACTIVATED_WITH]->(b:MemoryNode {space_id: $spaceId})
    WHERE a.layer >= 0 AND a.layer <= 4
      AND b.layer >= 0 AND b.layer <= 4
      AND r.weight >= $minWeight
      AND NOT a.role_type IN ['concern','config','comparison','temporal','ui','constraint','dynamic_emergent']
      AND NOT b.role_type IN ['concern','config','comparison','temporal','ui','constraint','dynamic_emergent']
    RETURN a.node_id AS sourceId, b.node_id AS targetId,
           a.name AS sourceName, b.name AS targetName,
           a.summary AS sourceSummary, b.summary AS targetSummary,
           a.path AS sourcePath, b.path AS targetPath,
           a.layer AS sourceLayer, b.layer AS targetLayer,
           r.weight AS weight
    ORDER BY weight DESC
    LIMIT 500
    """

    driver = GraphDatabase.driver(uri, auth=(user, password))
    try:
        with driver.session() as session:
            result = session.run(query, spaceId=space_id, minWeight=min_weight)
            pairs = [dict(record) for record in result]
    finally:
        driver.close()

    if not pairs:
        print("No CO_ACTIVATED_WITH edges found.", file=sys.stderr)
        return []

    print(f"Found {len(pairs)} edge pairs", file=sys.stderr)

    # Union-find
    parent: dict[str, str] = {}

    def find(x: str) -> str:
        if x not in parent:
            parent[x] = x
        while parent[x] != x:
            parent[x] = parent[parent[x]]
            x = parent[x]
        return x

    def union(a: str, b: str) -> None:
        fa, fb = find(a), find(b)
        if fa != fb:
            parent[fa] = fb

    node_info: dict[str, dict] = {}
    for p in pairs:
        union(p["sourceId"], p["targetId"])
        node_info[p["sourceId"]] = {
            "node_id": p["sourceId"],
            "name": p["sourceName"],
            "summary": p["sourceSummary"] or "",
            "path": p["sourcePath"] or "",
            "layer": p["sourceLayer"],
        }
        node_info[p["targetId"]] = {
            "node_id": p["targetId"],
            "name": p["targetName"],
            "summary": p["targetSummary"] or "",
            "path": p["targetPath"] or "",
            "layer": p["targetLayer"],
        }

    clusters_map: dict[str, set[str]] = {}
    cluster_weights: dict[str, float] = {}
    for p in pairs:
        root = find(p["sourceId"])
        clusters_map.setdefault(root, set()).update([p["sourceId"], p["targetId"]])
        cluster_weights[root] = cluster_weights.get(root, 0.0) + p["weight"]

    clusters = []
    cid = 0
    for root in sorted(clusters_map, key=lambda r: cluster_weights[r], reverse=True):
        member_ids = clusters_map[root]
        if len(member_ids) < 3:
            continue
        cid += 1
        clusters.append({
            "cluster_id": cid,
            "members": [node_info[nid] for nid in sorted(member_ids)],
            "total_weight": round(cluster_weights[root], 4),
        })

    print(f"Formed {len(clusters)} clusters (>= 3 members)", file=sys.stderr)
    return clusters


# ============================================================
# COMMANDS
# ============================================================

def cmd_validate(args: argparse.Namespace) -> int:
    """Validate a single spec."""
    spec_path = Path(args.spec)
    spec = UETSLoader.load(spec_path)
    spec_dir = spec_path.parent

    # Load clusters
    clusters = UETSLoader.load_clusters(spec, spec_dir)
    if not clusters:
        print(f"No clusters found in fixture for {spec_path}", file=sys.stderr)
        return 1

    # Hash verification
    fixture = spec["fixture"]
    hash_ok = True
    if fixture["type"] == "file" and not args.skip_hash:
        fixture_path = (spec_dir / fixture["path"]).resolve()
        sha = fixture.get("sha256")
        if sha:
            ok, msg = verify_fixture_hash(fixture_path, sha)
            hash_ok = ok
            if not ok:
                print(f"  WARNING: {msg}", file=sys.stderr)

    # Run model
    model_cfg = spec["model"]
    run_cfg = spec.get("config", {})
    interface = ModelInterface(model_cfg, run_cfg)

    print(f"Testing {model_cfg['name']} against {len(clusters)} clusters...")
    results = []
    start = time.monotonic()
    for cluster in clusters:
        r = interface.call(cluster)
        results.append(r)
        status = "OK" if r.json_valid and r.label_valid else "FAIL"
        print(f"  Cluster {cluster.cluster_id}: {status} ({r.latency_ms:.0f}ms) {r.name or r.error}")
    duration = (time.monotonic() - start) * 1000

    # Validate
    spec_result = Validator.validate(spec, results)
    spec_result.spec_path = str(spec_path)
    spec_result.hash_verified = hash_ok
    spec_result.duration_ms = round(duration, 1)

    Reporter.print_result(spec_result)

    if args.report:
        Reporter.generate_report([spec_result], Path(args.report))

    return 0 if spec_result.status == "pass" else 1


def cmd_validate_all(args: argparse.Namespace) -> int:
    """Validate all specs in a directory."""
    spec_dir = Path(args.spec_dir)
    spec_files = sorted(spec_dir.glob("*.uets.json"))

    if not spec_files:
        print(f"No UETS specs found in {spec_dir}", file=sys.stderr)
        return 1

    print(f"Found {len(spec_files)} UETS specs\n")
    all_results = []

    for spec_path in spec_files:
        try:
            spec = UETSLoader.load(spec_path)
        except Exception as e:
            print(f"  SKIP {spec_path.name}: {e}", file=sys.stderr)
            continue

        clusters = UETSLoader.load_clusters(spec, spec_dir)
        if not clusters:
            print(f"  SKIP {spec_path.name}: no clusters", file=sys.stderr)
            continue

        # Hash check
        fixture = spec["fixture"]
        hash_ok = True
        if fixture["type"] == "file" and not args.skip_hash:
            fp = (spec_dir / fixture["path"]).resolve()
            sha = fixture.get("sha256")
            if sha:
                ok, msg = verify_fixture_hash(fp, sha)
                hash_ok = ok

        model_cfg = spec["model"]
        run_cfg = spec.get("config", {})
        interface = ModelInterface(model_cfg, run_cfg)

        print(f"Testing {model_cfg['name']}...")
        results = []
        start = time.monotonic()
        for cluster in clusters:
            r = interface.call(cluster)
            results.append(r)
        duration = (time.monotonic() - start) * 1000

        spec_result = Validator.validate(spec, results)
        spec_result.spec_path = str(spec_path)
        spec_result.hash_verified = hash_ok
        spec_result.duration_ms = round(duration, 1)
        all_results.append(spec_result)

        Reporter.print_result(spec_result)

    Reporter.print_summary_table(all_results)
    Reporter.print_overall(all_results)

    if args.report:
        Reporter.generate_report(all_results, Path(args.report))

    failed = sum(1 for r in all_results if r.status == "fail")
    return 1 if failed > 0 else 0


def cmd_add_hashes(args: argparse.Namespace) -> int:
    """Add SHA256 hashes to fixture references in specs."""
    spec_dir = Path(args.spec_dir)
    for spec_path in sorted(spec_dir.glob("*.uets.json")):
        with open(spec_path) as f:
            spec = json.load(f)
        fixture = spec.get("fixture", {})
        if fixture.get("type") != "file":
            continue
        fp = (spec_dir / fixture["path"]).resolve()
        if not fp.exists():
            print(f"  SKIP {spec_path.name}: fixture not found at {fp}")
            continue
        sha = compute_sha256(fp)
        if fixture.get("sha256") == sha:
            print(f"  OK   {spec_path.name}: hash unchanged")
            continue
        fixture["sha256"] = sha
        with open(spec_path, "w") as f:
            json.dump(spec, f, indent=2)
            f.write("\n")
        print(f"  SET  {spec_path.name}: {sha[:16]}...")
    return 0


def cmd_verify_hashes(args: argparse.Namespace) -> int:
    """Verify fixture hashes in specs."""
    spec_dir = Path(args.spec_dir)
    failures = 0
    for spec_path in sorted(spec_dir.glob("*.uets.json")):
        with open(spec_path) as f:
            spec = json.load(f)
        fixture = spec.get("fixture", {})
        if fixture.get("type") != "file":
            continue
        sha = fixture.get("sha256")
        if not sha:
            print(f"  SKIP {spec_path.name}: no hash")
            continue
        fp = (spec_dir / fixture["path"]).resolve()
        ok, msg = verify_fixture_hash(fp, sha)
        if ok:
            print(f"  OK   {spec_path.name}: {msg}")
        else:
            print(f"  FAIL {spec_path.name}: {msg}")
            failures += 1
    return 1 if failures > 0 else 0


def cmd_extract_clusters(args: argparse.Namespace) -> int:
    """Extract clusters from Neo4j for use as fixtures."""
    uri = os.getenv("NEO4J_URI", "bolt://localhost:7687")
    user = os.getenv("NEO4J_USER", "neo4j")
    password = os.getenv("NEO4J_PASS", "")
    if not password:
        print("Error: NEO4J_PASS environment variable required", file=sys.stderr)
        return 1

    clusters = extract_clusters_from_neo4j(
        uri, user, password,
        space_id=args.space_id,
        min_weight=args.min_weight,
    )

    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    with open(output, "w") as f:
        json.dump(clusters, f, indent=2)
    print(f"Wrote {len(clusters)} clusters to {output}", file=sys.stderr)
    return 0


# ============================================================
# CLI
# ============================================================

def main() -> int:
    parser = argparse.ArgumentParser(
        description="Universal Emergence Test Specification (UETS) Runner"
    )
    sub = parser.add_subparsers(dest="command")

    # validate
    p_val = sub.add_parser("validate", help="Validate a single UETS spec")
    p_val.add_argument("--spec", required=True, help="Path to .uets.json spec file")
    p_val.add_argument("--skip-hash", action="store_true", help="Skip fixture hash verification")
    p_val.add_argument("--report", help="Output JSON report path")

    # validate-all
    p_all = sub.add_parser("validate-all", help="Validate all UETS specs in directory")
    p_all.add_argument("--spec-dir", required=True, help="Directory containing .uets.json specs")
    p_all.add_argument("--skip-hash", action="store_true", help="Skip fixture hash verification")
    p_all.add_argument("--report", help="Output JSON report path")

    # add-hashes
    p_hash = sub.add_parser("add-hashes", help="Add SHA256 hashes to fixture references")
    p_hash.add_argument("--spec-dir", required=True, help="Directory containing .uets.json specs")

    # verify-hashes
    p_vhash = sub.add_parser("verify-hashes", help="Verify fixture hashes")
    p_vhash.add_argument("--spec-dir", required=True, help="Directory containing .uets.json specs")

    # extract-clusters
    p_ext = sub.add_parser("extract-clusters", help="Extract clusters from Neo4j")
    p_ext.add_argument("--output", default="fixtures/clusters.json", help="Output path")
    p_ext.add_argument("--space-id", default="mdemg-dev", help="Space ID")
    p_ext.add_argument("--min-weight", type=float, default=0.1, help="Minimum edge weight")

    args = parser.parse_args()
    if not args.command:
        parser.print_help()
        return 1

    commands = {
        "validate": cmd_validate,
        "validate-all": cmd_validate_all,
        "add-hashes": cmd_add_hashes,
        "verify-hashes": cmd_verify_hashes,
        "extract-clusters": cmd_extract_clusters,
    }
    return commands[args.command](args)


if __name__ == "__main__":
    sys.exit(main())
