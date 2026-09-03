#!/usr/bin/env python3
"""MDEMG-USAGE-CORPUS-CURATE-001 Epic 1 — export ingested MDEMG doc nodes.

Reads MemoryNode rows from the `mdemg-dev` Neo4j space and emits one JSONL row
per ingested doc section under `mdemg-docs/{features,user,api,claude,cli-help}/`
(the 5 surfaces ingested by MDEMG-DOCS-INGEST-001).

⚠️ Sprint MDEMG-USAGE-CORPUS-CURATE-002 (task #148) tightened the query WHERE
clause + `classify_surface` to REQUIRE the `mdemg-docs/` prefix. Prior loose
`CONTAINS 'docs/features'` predicate leaked 504 vendored Python-symbol rows +
13 claude-docs rows into the raw export. The prefix is anchored to what
`mdemg mdemg-docs-ingest` actually writes; nothing else survives.

Output row shape (see docs/development/mdemg-usage-corpus-curate-001/sprint_plan.md §5 Epic 1):

    {
      "node_id": "<CUIDv2>",
      "path": "mdemg-docs/features/...",
      "surface": "features|user_api|cli-help|CLAUDE.md",
      "section_header": "How it works",  // optional; may be None
      "content": "...",
      "content_sha256": "..."
    }

Read-only. Paginated 500-row batches. No substrate mutation.

Usage:
    python3 scripts/mdemg_usage_export_docs.py --space-id mdemg-dev \
        --out training_data/mdemg_usage/raw/nodes.jsonl
    python3 scripts/mdemg_usage_export_docs.py --dry-run  # print counts only

Sprint: docs/development/mdemg-usage-corpus-curate-001/
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import sys
from dataclasses import dataclass
from pathlib import Path


# Sprint MDEMG-USAGE-CORPUS-CURATE-002 (task #148) — prefix-anchored surface
# classification. The shipped `mdemg mdemg-docs-ingest` CLI writes paths of the
# shape `mdemg-docs/{surface}/{file_slug}/{NNN}__{header_slug}` where
# {surface} ∈ {features, user, api, claude, cli-help}. Requiring this exact
# prefix at BOTH the query WHERE clause (root cause) and the classify_surface
# function (defense-in-depth) prevents pollution from:
#   - vendored Python symbols like `/docs/api/api-spec/uats/.venv/**/*.py#Symbol`
#     (504 rows in the 2026-08-24 snapshot)
#   - claude-docs corpus rows (13 rows in the same snapshot; ingested by
#     claude-docs-ingest with the `claude-docs/` prefix — a different corpus)
#   - stray `CLAUDE.md` symbol nodes from the code-symbol ingester
# The old broad `CONTAINS 'docs/features'` predicate matched any of those.
MDEMG_DOCS_PATH_PREFIX = "mdemg-docs/"


@dataclass(frozen=True)
class SurfaceRule:
    name: str
    # Prefix relative to MDEMG_DOCS_PATH_PREFIX (i.e. the sub-surface segment).
    subprefix: str


SURFACES: tuple[SurfaceRule, ...] = (
    SurfaceRule("features", "features/"),
    SurfaceRule("user_api", "user/"),
    SurfaceRule("user_api", "api/"),
    SurfaceRule("cli-help", "cli-help/"),
    SurfaceRule("CLAUDE.md", "claude/"),
)


def classify_surface(path: str) -> str | None:
    """Classify a path into one of the 5 mdemg-docs surfaces.

    Requires the `mdemg-docs/` prefix (task #148). Returns None for any path
    that doesn't carry the prefix OR whose sub-surface doesn't match any
    known rule.
    """
    if not path or not path.startswith(MDEMG_DOCS_PATH_PREFIX):
        return None
    tail = path[len(MDEMG_DOCS_PATH_PREFIX):]
    for rule in SURFACES:
        if tail.startswith(rule.subprefix):
            return rule.name
    return None


def sha256_str(s: str) -> str:
    return hashlib.sha256(s.encode("utf-8")).hexdigest()


def build_query() -> str:
    # Sprint MDEMG-USAGE-CORPUS-CURATE-002 (task #148) — root-cause fix.
    # The pre-#148 predicate was 5× `n.path CONTAINS '<sub>'` which matched
    # ANY node whose path had those substrings ANYWHERE. On the 2026-08-24
    # snapshot this pulled 504 non-mdemg-docs rows into the raw export
    # (vendored Python symbol nodes like `/docs/api/api-spec/uats/.venv/lib/
    # python3.12/site-packages/urllib3/exceptions.py#EmptyPoolError`) plus
    # 13 claude-docs rows from a DIFFERENT ingest pipeline.
    #
    # Fix: `STARTS WITH 'mdemg-docs/'` anchors the predicate to the exact
    # prefix `mdemg mdemg-docs-ingest` writes; nothing else can match.
    # Defense-in-depth mirror: `classify_surface()` also gates on the
    # prefix + subprefix — both layers together prevent regression.
    return (
        "MATCH (n:MemoryNode {space_id: $space_id}) "
        "WHERE n.path IS NOT NULL "
        f"AND n.path STARTS WITH '{MDEMG_DOCS_PATH_PREFIX}' "
        "AND coalesce(n.is_archived, false) = false "
        "AND n.content IS NOT NULL AND n.content <> '' "
        "RETURN n.node_id AS node_id, n.path AS path, n.name AS section_header, n.content AS content "
        "ORDER BY n.node_id "
        "SKIP $skip LIMIT $limit"
    )


def fetch_page(driver, space_id: str, skip: int, limit: int) -> list[dict]:
    with driver.session() as sess:
        result = sess.run(
            build_query(), space_id=space_id, skip=skip, limit=limit
        )
        return [dict(r) for r in result]


def export(space_id: str, out_path: Path, batch: int, dry_run: bool) -> tuple[int, dict[str, int]]:
    try:
        from neo4j import GraphDatabase  # type: ignore
    except ImportError:
        print("ERROR: neo4j driver not installed. Install: pip install neo4j", file=sys.stderr)
        sys.exit(2)

    uri = os.environ.get("NEO4J_URI", "bolt://127.0.0.1:7687")
    user = os.environ.get("NEO4J_USER", "neo4j")
    password = os.environ.get("NEO4J_PASSWORD", "testpassword")

    driver = GraphDatabase.driver(uri, auth=(user, password))
    try:
        total = 0
        per_surface: dict[str, int] = {}
        skipped_no_surface = 0

        out_fh = None if dry_run else out_path.open("w", encoding="utf-8")
        try:
            skip = 0
            while True:
                rows = fetch_page(driver, space_id, skip, batch)
                if not rows:
                    break
                for r in rows:
                    path = r.get("path") or ""
                    surface = classify_surface(path)
                    if surface is None:
                        skipped_no_surface += 1
                        continue
                    content = r.get("content") or ""
                    row = {
                        "node_id": r.get("node_id"),
                        "path": path,
                        "surface": surface,
                        "section_header": r.get("section_header"),
                        "content": content,
                        "content_sha256": sha256_str(content),
                    }
                    if out_fh is not None:
                        out_fh.write(json.dumps(row, ensure_ascii=False) + "\n")
                    total += 1
                    per_surface[surface] = per_surface.get(surface, 0) + 1
                skip += batch
                print(f"  page skip={skip}: total={total}", file=sys.stderr)
        finally:
            if out_fh is not None:
                out_fh.close()

        print(f"\nExport complete: total={total} skipped_no_surface={skipped_no_surface}")
        for surface, n in sorted(per_surface.items(), key=lambda kv: (-kv[1], kv[0])):
            print(f"  {surface}: {n}")
        if not dry_run:
            print(f"Wrote: {out_path}")
        return total, per_surface
    finally:
        driver.close()


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.strip().splitlines()[0])
    ap.add_argument("--space-id", default=os.environ.get("MDEMG_SPACE_ID", "mdemg-dev"))
    ap.add_argument(
        "--out",
        type=Path,
        default=Path("training_data/mdemg_usage/raw/nodes.jsonl"),
    )
    ap.add_argument("--batch", type=int, default=500)
    ap.add_argument("--dry-run", action="store_true")
    args = ap.parse_args()

    if not args.dry_run:
        args.out.parent.mkdir(parents=True, exist_ok=True)

    total, per_surface = export(args.space_id, args.out, args.batch, args.dry_run)

    if total < 1500:
        print(
            f"\nWARN: total={total} < gate threshold 1500. "
            "Epic 2 will still run; check per-surface distribution.",
            file=sys.stderr,
        )
    return 0


if __name__ == "__main__":
    sys.exit(main())
