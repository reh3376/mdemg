#!/usr/bin/env python3
"""APE-REFLECT-EVAL-REFRESH-001 — regenerate valid_clean.jsonl's ape.reflect rows.

The original 20 rows carried pre-APE-PROMPT-BUDGET-001 prompts (~4,000 est.
tokens): under llama-server's 8,192-token per-slot KV bound the model's output
truncates mid-array (finish_reason=length) and json_valid honestly scores 0 —
an eval artifact, not a model regression. This script replaces ONLY the
ape.reflect rows with post-2026-06-14 production rows (budget-bounded prompts,
avg ~1,370 est. tokens), preserving the exact row schema and leaving every
other task's rows byte-identical.

Reproducible: samples deterministically (chronological spread over the clean
candidate set). Backup written before splice.
"""
import json
import subprocess
import sys
import hashlib
from datetime import datetime, timezone

EVAL = "training_data/eval/valid_clean.jsonl"
BACKUP = "training_data/eval/valid_clean.jsonl.pre_ape_refresh_bak"
TARGET = 20
CUTOFF = "2026-06-14"  # first full day after APE-PROMPT-BUDGET-001 (2026-06-13)

SQL = f"""
WITH clean AS (
  SELECT trace_id, time, system_prompt, user_prompt, response, system_prompt_hash
  FROM llm_interactions
  WHERE task_name='ape.reflect' AND time > '{CUTOFF}'
    AND error = '' AND response IS NOT NULL AND response <> ''
    AND user_prompt IS NOT NULL AND user_prompt <> ''
    AND system_prompt IS NOT NULL AND system_prompt <> ''
), numbered AS (
  SELECT *, ROW_NUMBER() OVER (ORDER BY time) AS rn, COUNT(*) OVER () AS n FROM clean
)
SELECT COALESCE(json_agg(row_to_json(numbered.*) ORDER BY rn), '[]'::json)
FROM numbered WHERE (rn - 1) % GREATEST(n / 120, 1) = 0
"""


def fetch_candidates():
    out = subprocess.run(
        ["docker", "exec", "mdemg-timescaledb-1", "psql", "-U", "mdemg",
         "-d", "mdemg_metrics", "-t", "-A", "-c", SQL],
        capture_output=True, text=True, check=True)
    return json.loads(out.stdout.strip())


def response_is_valid_array(resp: str) -> bool:
    try:
        return isinstance(json.loads(resp), list)
    except (json.JSONDecodeError, TypeError):
        return False


def main() -> int:
    rows = [json.loads(l) for l in open(EVAL)]
    old_ape_idx = [i for i, r in enumerate(rows)
                   if (r.get("meta") or {}).get("task_name") == "ape.reflect"]
    if len(old_ape_idx) != TARGET:
        print(f"expected {TARGET} ape.reflect rows, found {len(old_ape_idx)}", file=sys.stderr)
        return 1

    cands = [c for c in fetch_candidates() if response_is_valid_array(c["response"])]
    if len(cands) < TARGET:
        print(f"only {len(cands)} valid-array candidates — need {TARGET}", file=sys.stderr)
        return 1
    # Even chronological spread over the filtered candidates.
    step = len(cands) / TARGET
    picked = [cands[int(i * step)] for i in range(TARGET)]

    new_rows = []
    for c in picked:
        new_rows.append({
            "messages": [
                {"role": "system", "content": c["system_prompt"]},
                {"role": "user", "content": c["user_prompt"]},
                {"role": "assistant", "content": c["response"]},
            ],
            "meta": {
                "task_name": "ape.reflect",
                "sampling_group": "T",
                "source": "tsdb",
                "tsdb_trace_id": c["trace_id"],
                "tsdb_time": c["time"],
                "system_prompt_hash": c.get("system_prompt_hash"),
                "quality": None,
                "extracted_by": "refresh_ape_reflect_eval_rows.py (APE-REFLECT-EVAL-REFRESH-001)",
            },
        })

    open(BACKUP, "w").write("".join(json.dumps(r, ensure_ascii=False) + "\n" for r in rows))
    for i, nr in zip(old_ape_idx, new_rows):
        rows[i] = nr
    body = "".join(json.dumps(r, ensure_ascii=False) + "\n" for r in rows)
    open(EVAL, "w").write(body)

    sha = hashlib.sha256(body.encode()).hexdigest()
    sizes = [len(next(m["content"] for m in r["messages"] if m["role"] == "user")) for r in new_rows]
    print(f"replaced {TARGET} ape.reflect rows (of {len(rows)} total)")
    print(f"new prompt chars: min={min(sizes)} avg={sum(sizes)//len(sizes)} max={max(sizes)} (~{max(sizes)//4} est. tokens max)")
    print(f"new sha256: {sha}")
    print(f"built_at: {datetime.now(timezone.utc).isoformat()}")
    print(f"backup: {BACKUP}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
