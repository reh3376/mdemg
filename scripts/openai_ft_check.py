#!/usr/bin/env python3
"""
openai_ft_check.py — Pre-upload validator + post-job harvester for OpenAI FT.

Two modes:

**Pre-upload validator (default)** — reads a directory produced by
`training.openai_ft_adapter` (combined_train.jsonl, combined_val.jsonl,
manifest.json) and verifies:

    1. Required files exist.
    2. Manifest schema_version is recognized.
    3. SHA256 digests in manifest match re-computed digests of the JSONL files.
    4. Every record parses as JSON and has a valid OpenAI chat-messages shape.
    5. Row counts match manifest claims.
    6. Cost estimate is present and non-negative.
    7. Minimum sample thresholds (OpenAI: ≥10 train, recommended ≥50 train + ≥20 val).

**Post-job harvester (``--on-complete --job-id``)** — fetches a completed
fine-tuning job and writes two artifacts into ``--dir``:

    - ``training_metrics.json``  — parsed from OpenAI's ``result_file`` CSV.
      Captures per-step ``train_loss``, ``valid_loss``, the step index of
      minimum valid_loss (``best_val_loss_step``), and final-step values.
    - ``job_lifecycle.json``     — job state transitions + timing
      (created_at, finished_at, status, n_epochs_actual, error_if_any).

Exits 0 if all checks pass. Exits 1 with a list of errors otherwise.

Usage:
    python scripts/openai_ft_check.py --dir training_data/openai_ft/20260420

    # After job completion:
    python scripts/openai_ft_check.py \\
        --on-complete --job-id ftjob-abc123 \\
        --dir training_data/openai_ft/20260420
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import sys
from datetime import datetime, timezone


def _sha256_of_file(path: str) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def _iter_jsonl(path: str):
    with open(path, encoding="utf-8") as f:
        for lineno, line in enumerate(f, 1):
            line = line.strip()
            if not line:
                continue
            try:
                yield lineno, json.loads(line)
            except json.JSONDecodeError as e:
                yield lineno, {"__parse_error__": str(e)}


def _check_record(rec: dict) -> str:
    if "__parse_error__" in rec:
        return f"parse error: {rec['__parse_error__']}"
    msgs = rec.get("messages")
    if not isinstance(msgs, list) or not msgs:
        return "messages missing or empty"
    roles = {m.get("role") for m in msgs if isinstance(m, dict)}
    if not roles.issubset({"system", "user", "assistant"}):
        return f"invalid roles: {roles - {'system', 'user', 'assistant'}}"
    if "assistant" not in roles:
        return "no assistant message"
    for m in msgs:
        c = m.get("content") if isinstance(m, dict) else None
        if not isinstance(c, str) or not c.strip():
            return f"non-string or empty content (role={m.get('role') if isinstance(m, dict) else None})"
    return ""


def check(directory: str) -> list[str]:
    errors: list[str] = []
    required = ["combined_train.jsonl", "combined_val.jsonl", "manifest.json"]
    for f in required:
        if not os.path.exists(os.path.join(directory, f)):
            errors.append(f"missing file: {f}")
    if errors:
        return errors

    with open(os.path.join(directory, "manifest.json"), encoding="utf-8") as f:
        manifest = json.load(f)

    if manifest.get("schema_version") != 1:
        errors.append(
            f"unsupported manifest schema_version: {manifest.get('schema_version')}"
        )

    # SHA256 integrity + row counts + record validation per split.
    for split_name, split in manifest.get("splits", {}).items():
        path = split.get("path")
        if not path or not os.path.exists(path):
            # Fall back to relative path inside directory.
            path = os.path.join(directory, f"combined_{split_name}.jsonl")
        if not os.path.exists(path):
            errors.append(f"{split_name}: file not found at {path}")
            continue

        actual_sha = _sha256_of_file(path)
        claimed_sha = split.get("sha256")
        if claimed_sha and claimed_sha != actual_sha:
            errors.append(
                f"{split_name}: sha256 mismatch (manifest={claimed_sha[:12]}…, actual={actual_sha[:12]}…)"
            )

        rows = 0
        for lineno, rec in _iter_jsonl(path):
            rows += 1
            reason = _check_record(rec)
            if reason:
                errors.append(f"{split_name}:{lineno}: {reason}")
                if len([e for e in errors if e.startswith(f"{split_name}:")]) > 20:
                    errors.append(f"{split_name}: (truncated — >20 bad records)")
                    break
        claimed_rows = split.get("rows")
        if claimed_rows is not None and rows != claimed_rows:
            errors.append(
                f"{split_name}: row count mismatch (manifest={claimed_rows}, file={rows})"
            )

    # Sample-size minima.
    train_rows = manifest.get("splits", {}).get("train", {}).get("rows", 0)
    val_rows = manifest.get("splits", {}).get("val", {}).get("rows", 0)
    if train_rows < 10:
        errors.append(f"train rows {train_rows} below OpenAI hard minimum of 10")
    if train_rows < 50:
        errors.append(
            f"train rows {train_rows} below recommended floor of 50 (non-fatal warning)"
        )
    if val_rows < 20:
        errors.append(
            f"val rows {val_rows} below recommended floor of 20 (non-fatal warning)"
        )

    cost = manifest.get("totals", {}).get("cost_estimate_usd")
    if cost is None or cost < 0:
        errors.append(f"invalid cost_estimate_usd: {cost}")

    return errors


def parse_training_metrics_csv(csv_text: str) -> dict:
    """Parse OpenAI's fine-tuning result_file CSV → structured metrics dict.

    The CSV's columns as of the 2024-12 FT API:
        step, train_loss, train_accuracy, valid_loss, valid_mean_token_accuracy

    Not every row has all fields — eval rows may lack train_loss and vice
    versa. We coerce blanks → None and compute aggregates only from non-None
    values.
    """
    import csv
    import io

    reader = csv.DictReader(io.StringIO(csv_text))
    rows: list[dict] = []
    for raw in reader:
        row: dict = {}
        for k, v in raw.items():
            if v is None or v == "":
                row[k] = None
            elif k == "step":
                try:
                    row[k] = int(v)
                except ValueError:
                    row[k] = None
            else:
                try:
                    row[k] = float(v)
                except ValueError:
                    row[k] = v
        rows.append(row)

    def _nonnull(key: str) -> list[tuple[int, float]]:
        out = []
        for r in rows:
            step = r.get("step")
            val = r.get(key)
            if isinstance(step, int) and isinstance(val, (int, float)):
                out.append((step, float(val)))
        return out

    train_series = _nonnull("train_loss")
    valid_series = _nonnull("valid_loss")

    best_val_step: int | None = None
    best_val_loss: float | None = None
    if valid_series:
        best_val_step, best_val_loss = min(valid_series, key=lambda t: t[1])

    final_train_loss = train_series[-1][1] if train_series else None
    final_valid_loss = valid_series[-1][1] if valid_series else None
    final_step = rows[-1].get("step") if rows else None

    return {
        "n_rows": len(rows),
        "final_step": final_step,
        "final_train_loss": final_train_loss,
        "final_valid_loss": final_valid_loss,
        "best_val_loss_step": best_val_step,
        "best_val_loss": best_val_loss,
        "train_loss_series": [{"step": s, "loss": v} for s, v in train_series],
        "valid_loss_series": [{"step": s, "loss": v} for s, v in valid_series],
    }


def _emit_queue_stuck_alert(directory: str, job_id: str, queued_minutes: float) -> None:
    """Write an alert.log entry + stdout note when a job is queue-stuck.

    By design (Epic 7 O2): we do NOT auto-cancel. Automating the cancel could
    cost money if the job is about to start. Human confirmation required.
    """
    import time as _time

    alert_path = os.path.join(directory, "alert.log")
    line = (
        f"[{datetime.now(timezone.utc).isoformat()}] QUEUE-STUCK "
        f"job_id={job_id} queued_for={queued_minutes:.1f}min — no auto-cancel\n"
    )
    # Local import to avoid top-level time coupling.
    _ = _time
    with open(alert_path, "a", encoding="utf-8") as f:
        f.write(line)
    print(f"ALERT: {line.strip()}", file=sys.stderr)
    print(f"       wrote {alert_path}", file=sys.stderr)


def on_complete(
    directory: str,
    job_id: str,
    queue_timeout_minutes: float | None = None,
) -> int:
    """Harvest training_metrics.json + job_lifecycle.json for a completed job.

    If ``queue_timeout_minutes`` is set and the job has been queued longer than
    that with no transition to ``running``, emit an alert (non-blocking).
    """
    if not os.environ.get("OPENAI_API_KEY"):
        print("ABORT: OPENAI_API_KEY not set.", file=sys.stderr)
        return 4
    try:
        from openai import OpenAI
    except ImportError:
        print(
            "ABORT: `openai` package not installed. "
            "Run `pip install 'openai>=1.50'`.",
            file=sys.stderr,
        )
        return 5

    os.makedirs(directory, exist_ok=True)
    client = OpenAI()

    job = client.fine_tuning.jobs.retrieve(job_id)
    status = getattr(job, "status", None)
    print(f"job {job_id}: status={status}")

    created_at = getattr(job, "created_at", None)
    finished_at = getattr(job, "finished_at", None)

    # Derive queue + train seconds (best-effort; absent timestamps → None).
    # OpenAI's job object exposes created_at (epoch s) and finished_at;
    # intermediate queued_at/running_at aren't currently surfaced in the SDK.
    # We approximate queue_seconds from events[].created_at if available.
    queue_seconds: int | None = None
    train_seconds: int | None = None
    running_at: int | None = None
    try:
        events_page = client.fine_tuning.jobs.list_events(job_id, limit=50)
        for ev in reversed(list(getattr(events_page, "data", []) or [])):
            # Events go from oldest-last to newest-first typically; scan for
            # a "running" / "training" signal.
            msg = (getattr(ev, "message", "") or "").lower()
            if running_at is None and ("running" in msg or "started" in msg or "training" in msg):
                running_at = getattr(ev, "created_at", None)
                break
    except Exception as e:  # noqa: BLE001 — non-fatal diagnostic
        print(f"  (could not fetch events: {e})", file=sys.stderr)

    if isinstance(created_at, int) and isinstance(running_at, int):
        queue_seconds = max(0, running_at - created_at)
    if isinstance(running_at, int) and isinstance(finished_at, int):
        train_seconds = max(0, finished_at - running_at)

    lifecycle = {
        "job_id": job_id,
        "status": status,
        "created_at": created_at,
        "running_at": running_at,
        "finished_at": finished_at,
        "queue_seconds": queue_seconds,
        "train_seconds": train_seconds,
        "model": getattr(job, "model", None),
        "fine_tuned_model": getattr(job, "fine_tuned_model", None),
        "trained_tokens": getattr(job, "trained_tokens", None),
        "training_file": getattr(job, "training_file", None),
        "validation_file": getattr(job, "validation_file", None),
        "result_files": list(getattr(job, "result_files", []) or []),
        "error": getattr(getattr(job, "error", None), "message", None)
        if getattr(job, "error", None)
        else None,
    }
    hyper = getattr(job, "hyperparameters", None)
    if hyper is not None:
        lifecycle["n_epochs_actual"] = getattr(hyper, "n_epochs", None)

    lifecycle_path = os.path.join(directory, "job_lifecycle.json")
    with open(lifecycle_path, "w", encoding="utf-8") as f:
        json.dump(lifecycle, f, indent=2, default=str)
    print(f"wrote {lifecycle_path}")

    # O2 — queue-stuck alert (non-blocking, human decides whether to cancel).
    if queue_timeout_minutes is not None and queue_timeout_minutes > 0:
        import time as _time

        if status in {"queued", "validating_files"} and isinstance(created_at, int):
            queued_minutes = (_time.time() - created_at) / 60.0
            if queued_minutes > queue_timeout_minutes:
                _emit_queue_stuck_alert(directory, job_id, queued_minutes)

    if status != "succeeded":
        print(
            f"WARNING: job status is {status!r}, not 'succeeded'. "
            f"Skipping training_metrics.json harvest.",
            file=sys.stderr,
        )
        # Still exit 0 — lifecycle.json is the useful artifact when the
        # job hasn't finished yet. Caller can re-run after completion.
        return 0

    result_file_ids = lifecycle["result_files"]
    if not result_file_ids:
        print("WARNING: job has no result_files; skipping metrics harvest.", file=sys.stderr)
        return 0

    # First result file is the training metrics CSV.
    rfid = result_file_ids[0]
    print(f"fetching result_file {rfid} …")
    content = client.files.content(rfid)
    csv_text = content.text if hasattr(content, "text") else content.read().decode("utf-8")

    metrics = parse_training_metrics_csv(csv_text)
    metrics["source_result_file_id"] = rfid
    metrics_path = os.path.join(directory, "training_metrics.json")
    with open(metrics_path, "w", encoding="utf-8") as f:
        json.dump(metrics, f, indent=2)
    print(f"wrote {metrics_path}")
    print(
        f"  best_val_loss_step = {metrics['best_val_loss_step']} "
        f"(loss={metrics['best_val_loss']})"
    )
    print(
        f"  final_train_loss   = {metrics['final_train_loss']}, "
        f"final_valid_loss = {metrics['final_valid_loss']}"
    )
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--dir",
        required=True,
        help="Directory produced by training.openai_ft_adapter",
    )
    parser.add_argument(
        "--on-complete",
        action="store_true",
        help=(
            "Post-job mode: fetch a completed fine-tuning job via its "
            "--job-id and write training_metrics.json + job_lifecycle.json "
            "into --dir. Requires OPENAI_API_KEY."
        ),
    )
    parser.add_argument(
        "--job-id",
        default=None,
        help="OpenAI fine-tuning job id (required with --on-complete).",
    )
    parser.add_argument(
        "--queue-timeout-minutes",
        type=float,
        default=None,
        help=(
            "With --on-complete: if the job is still queued N minutes after "
            "creation, emit an alert to alert.log (does NOT auto-cancel). "
            "Sensible default for a campaign: 180 (3 hours). "
            "FT-OAI-002 Epic 7 O2."
        ),
    )
    args = parser.parse_args(argv)

    if args.on_complete:
        if not args.job_id:
            print("ABORT: --job-id is required with --on-complete.", file=sys.stderr)
            return 2
        return on_complete(
            args.dir, args.job_id,
            queue_timeout_minutes=args.queue_timeout_minutes,
        )

    errors = check(args.dir)
    # Separate fatal errors from "below recommended" warnings.
    fatal = [e for e in errors if "non-fatal" not in e]
    warn = [e for e in errors if "non-fatal" in e]

    if warn:
        print("WARNINGS:", file=sys.stderr)
        for w in warn:
            print(f"  - {w}", file=sys.stderr)

    if fatal:
        print(f"FAIL: {len(fatal)} errors", file=sys.stderr)
        for e in fatal:
            print(f"  - {e}", file=sys.stderr)
        return 1

    print(f"OK: {args.dir} passes all fatal pre-upload checks.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
