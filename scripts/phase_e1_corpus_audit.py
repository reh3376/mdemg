#!/usr/bin/env python3
"""
PHASE-E1-CORPUS-AUDIT-001 — audit whether Claude-Code fact-recall corpus
rows are covered by the substrate post-INGEST-TOPOLOGY-REPAIR-001 +
CLAUDE-DOCS-INGEST-001. For each row, extract the user question, query
/v1/memory/retrieve, and score whether the retrieved content contains
the assistant's answer facts.

Output: audit_rows.jsonl (per-row), audit_report.md (summary), and
rows_to_strip.jsonl / rows_to_keep.jsonl (for the E2 curation sprint).

Contract: read-only against substrate. Fail-safe classification: any
query error → AUDIT_ERROR, never PROVEN_COVERAGE.
"""

import argparse
import json
import os
import re
import sys
import time
import threading
from collections import Counter
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path
from typing import Any

import urllib.request
import urllib.error

# Deterministic word tokenization: lowercase, split on non-alphanum, drop
# stopwords + very short tokens. 3-grams over the resulting stream.
STOPWORDS = frozenset(
    "the a an of and or in to for from with is are was were be being been "
    "on at by as if it its this that these those you your yours we our "
    "have has had do does did will would should could may might can also "
    "not no than then so such but which when where what who why how".split()
)
_WORD_RE = re.compile(r"[a-z0-9]+")


def tokenize(text: str) -> list[str]:
    return [t for t in _WORD_RE.findall((text or "").lower()) if t not in STOPWORDS and len(t) > 2]


def ngrams(toks: list[str], n: int = 3) -> set[tuple[str, ...]]:
    if len(toks) < n:
        return set()
    return {tuple(toks[i : i + n]) for i in range(len(toks) - n + 1)}


def overlap_ratio(answer: str, content: str, n: int = 3) -> float:
    """Asymmetric overlap: how much of `answer`'s 3-gram set is present in `content`."""
    ans_grams = ngrams(tokenize(answer), n)
    if not ans_grams:
        return 0.0
    ctx_grams = ngrams(tokenize(content), n)
    if not ctx_grams:
        return 0.0
    return len(ans_grams & ctx_grams) / len(ans_grams)


def retrieve_content(base_url: str, space_id: str, query: str, top_k: int, timeout_s: int) -> tuple[list[dict[str, Any]], str | None]:
    """POST /v1/memory/retrieve, return (results, error). Retry once on 5xx."""
    payload = json.dumps(
        {
            "space_id": space_id,
            "query_text": query,
            "top_k": top_k,
            "candidate_k": max(top_k * 4, 20),
            "include_content": True,
        }
    ).encode("utf-8")
    url = f"{base_url}/v1/memory/retrieve"
    for attempt in range(2):
        req = urllib.request.Request(url, data=payload, headers={"Content-Type": "application/json"}, method="POST")
        try:
            with urllib.request.urlopen(req, timeout=timeout_s) as resp:
                body = resp.read().decode("utf-8")
                obj = json.loads(body)
                return (obj.get("results", []) or []), None
        except urllib.error.HTTPError as e:
            if 500 <= e.code < 600 and attempt == 0:
                time.sleep(1.0)
                continue
            return [], f"HTTPError {e.code}: {str(e)[:80]}"
        except (urllib.error.URLError, TimeoutError, json.JSONDecodeError) as e:
            if attempt == 0:
                time.sleep(1.0)
                continue
            return [], f"{type(e).__name__}: {str(e)[:80]}"
    return [], "retry exhausted"


def classify(overlap: float, err: str | None, threshold: float) -> str:
    if err is not None:
        return "AUDIT_ERROR"
    if overlap >= threshold:
        return "PROVEN_COVERAGE"
    return "SUBSTRATE_MISS"


def audit_row(row_idx: int, row: dict[str, Any], base_url: str, space_id: str, top_k: int, timeout_s: int, threshold: float) -> dict[str, Any]:
    msgs = row.get("messages", []) or []
    question = ""
    answer = ""
    for m in msgs:
        if m.get("role") == "user":
            question = m.get("content", "") or ""
        elif m.get("role") == "assistant":
            answer = m.get("content", "") or ""
    if not question or not answer:
        return {
            "row_idx": row_idx,
            "question_len": len(question),
            "answer_len": len(answer),
            "overlap": 0.0,
            "top1_name": "",
            "n_results": 0,
            "classification": "AUDIT_ERROR",
            "error": "missing question or answer",
        }
    results, err = retrieve_content(base_url, space_id, question, top_k, timeout_s)
    content = ""
    top1_name = ""
    if results:
        top1_name = (results[0].get("name") or "")[:120]
        # Concatenate content across top_k; overlap is asymmetric on answer side, so extra content only helps recall
        parts = [(r.get("content") or "") for r in results]
        content = "\n\n".join(p for p in parts if p)
    ov = overlap_ratio(answer, content)
    return {
        "row_idx": row_idx,
        "question_len": len(question),
        "answer_len": len(answer),
        "overlap": round(ov, 4),
        "top1_name": top1_name,
        "n_results": len(results),
        "classification": classify(ov, err, threshold),
        "error": err,
    }


def load_corpus(path: str) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    with open(path, encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                rows.append(json.loads(line))
            except json.JSONDecodeError:
                continue
    return rows


def write_report(out_dir: Path, results: list[dict[str, Any]], threshold: float, corpus_path: str) -> None:
    total = len(results)
    counts = Counter(r["classification"] for r in results)
    proven = counts.get("PROVEN_COVERAGE", 0)
    miss = counts.get("SUBSTRATE_MISS", 0)
    err = counts.get("AUDIT_ERROR", 0)
    pct = lambda n: f"{100.0 * n / max(total, 1):.1f}%"

    # Overlap histogram (buckets 0.0-0.1, 0.1-0.2, ...)
    buckets = Counter()
    for r in results:
        ov = r.get("overlap", 0.0) or 0.0
        b = int(ov * 10)
        if b > 9:
            b = 9
        buckets[b] += 1

    # Top-20 SUBSTRATE_MISS by lowest overlap
    misses = sorted(
        (r for r in results if r["classification"] == "SUBSTRATE_MISS"),
        key=lambda r: (r.get("overlap", 0.0), r["row_idx"]),
    )[:20]

    lines = [
        "# PHASE-E1-CORPUS-AUDIT-001 — audit report",
        "",
        f"**Source corpus**: `{corpus_path}`",
        f"**Overlap threshold** for PROVEN_COVERAGE: `{threshold}` (asymmetric 3-gram overlap of answer→retrieved content).",
        "",
        "## Summary",
        "",
        f"| Classification | Count | % |",
        f"|---|---|---|",
        f"| **PROVEN_COVERAGE** (safe to strip in E2) | {proven} | {pct(proven)} |",
        f"| **SUBSTRATE_MISS** (keep in FT corpus) | {miss} | {pct(miss)} |",
        f"| **AUDIT_ERROR** (manual review before strip) | {err} | {pct(err)} |",
        f"| **Total** | {total} | 100.0% |",
        "",
        "## Overlap distribution",
        "",
        "| Bucket | Count |",
        "|---|---|",
    ]
    for b in range(10):
        lo = b / 10.0
        hi = (b + 1) / 10.0
        lines.append(f"| [{lo:.1f}, {hi:.1f}) | {buckets.get(b, 0)} |")

    lines.extend(
        [
            "",
            "## Top-20 SUBSTRATE_MISS rows (lowest overlap first)",
            "",
            "| row_idx | overlap | n_results | top1_name |",
            "|---|---|---|---|",
        ]
    )
    for r in misses:
        name = (r.get("top1_name") or "")[:80]
        lines.append(f"| {r['row_idx']} | {r['overlap']:.3f} | {r['n_results']} | {name} |")

    # Recommendation
    proven_pct = proven / max(total, 1)
    lines.append("")
    lines.append("## Recommendation")
    lines.append("")
    if proven_pct >= 0.80:
        lines.append(f"**Proceed to E2**: {pct(proven)} of the corpus has substrate coverage. Strip PROVEN_COVERAGE rows in E2, benchmark retrain in E3 to confirm no fact-recall regression.")
    elif proven_pct >= 0.50:
        lines.append(f"**Proceed to E2 with narrower strip**: {pct(proven)} coverage is a substantial but not overwhelming majority. Strip PROVEN_COVERAGE only; SUBSTRATE_MISS rows stay in FT corpus. Consider re-ingesting the SUBSTRATE_MISS rows into the substrate first (E1a follow-up).")
    else:
        lines.append(f"**Do NOT proceed to E2 yet**: only {pct(proven)} coverage. Either the CLAUDE-DOCS-INGEST-001 ingest is incomplete, or the audit's overlap threshold is miscalibrated. Investigate before stripping.")

    (out_dir / "audit_report.md").write_text("\n".join(lines) + "\n", encoding="utf-8")


def main() -> int:
    p = argparse.ArgumentParser()
    p.add_argument("--corpus", default="training_data/sft/claude_code_knowledge_v2/train.jsonl")
    p.add_argument("--space-id", default="mdemg-dev")
    p.add_argument("--base-url", default=os.environ.get("MDEMG_BASE_URL", "http://localhost:9999"))
    p.add_argument("--concurrency", type=int, default=5)
    p.add_argument("--threshold", type=float, default=0.30)
    p.add_argument("--top-k", type=int, default=5)
    p.add_argument("--timeout-s", type=int, default=60)
    p.add_argument("--sample", type=int, default=0, help="0 = all rows; else audit only first N rows (deterministic).")
    p.add_argument("--out-dir", default="docs/development/phase-e1-corpus-audit-001")
    args = p.parse_args()

    out_dir = Path(args.out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)

    corpus_path = args.corpus
    rows = load_corpus(corpus_path)
    if args.sample > 0:
        rows = rows[: args.sample]
    total = len(rows)
    print(f"[audit] loaded {total} rows from {corpus_path}", flush=True)
    print(f"[audit] concurrency={args.concurrency} threshold={args.threshold} space_id={args.space_id}", flush=True)

    # Progress reporting
    progress_lock = threading.Lock()
    done = [0]

    def _one(idx_row: tuple[int, dict[str, Any]]) -> dict[str, Any]:
        idx, r = idx_row
        result = audit_row(idx, r, args.base_url, args.space_id, args.top_k, args.timeout_s, args.threshold)
        with progress_lock:
            done[0] += 1
            n = done[0]
            if n % 50 == 0 or n == total:
                print(f"[audit] progress {n}/{total}", flush=True)
        return result

    results: list[dict[str, Any]] = []
    with ThreadPoolExecutor(max_workers=args.concurrency) as ex:
        futures = [ex.submit(_one, (i, r)) for i, r in enumerate(rows)]
        for f in as_completed(futures):
            try:
                results.append(f.result())
            except Exception as e:  # noqa: BLE001
                results.append({"row_idx": -1, "overlap": 0.0, "classification": "AUDIT_ERROR", "error": f"unexpected: {type(e).__name__}"})
    # Deterministic sort by row_idx
    results.sort(key=lambda r: r.get("row_idx", -1))

    # audit_rows.jsonl — per-row raw output (for future scripting)
    with (out_dir / "audit_rows.jsonl").open("w", encoding="utf-8") as f:
        for r in results:
            f.write(json.dumps(r) + "\n")

    # rows_to_strip / rows_to_keep
    with (out_dir / "rows_to_strip.jsonl").open("w", encoding="utf-8") as f_strip, (out_dir / "rows_to_keep.jsonl").open("w", encoding="utf-8") as f_keep:
        for r in results:
            line = json.dumps({"row_idx": r["row_idx"], "classification": r["classification"], "overlap": r.get("overlap", 0.0), "reason": r.get("error") or ""}) + "\n"
            if r["classification"] == "PROVEN_COVERAGE":
                f_strip.write(line)
            else:
                f_keep.write(line)

    write_report(out_dir, results, args.threshold, corpus_path)
    print(f"[audit] wrote outputs to {out_dir}", flush=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())
