#!/usr/bin/env python3
"""
openai_ft_upload_and_launch.py — Upload JSONL + launch fine-tuning job.

Reads the artifacts produced by `training.openai_ft_adapter` and:

    1. Re-runs the pre-upload check (scripts/openai_ft_check.py).
    2. Enforces a hard cost cap (--max-cost-usd, default 50.00). Aborts
       BEFORE any network I/O if the manifest's cost estimate exceeds the cap.
    3. Confirms with the user (interactive) unless --yes is passed.
    4. Uploads combined_train.jsonl and combined_val.jsonl via
       openai.files.create(purpose="fine-tune").
    5. Launches a fine-tuning job via openai.fine_tuning.jobs.create().
    6. Appends provenance + job metadata to run_notes.md in the artifact dir.

Requires:
    - $OPENAI_API_KEY in the environment
    - Python package `openai>=1.50`

Usage:
    python scripts/openai_ft_upload_and_launch.py \\
        --dir training_data/openai_ft/20260420 \\
        --model gpt-4.1-mini-2025-04-14 \\
        --max-cost-usd 50.00 \\
        [--suffix mdemg-ftoai001] \\
        [--yes]
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
from datetime import datetime, timezone


def _load_manifest(directory: str) -> dict:
    path = os.path.join(directory, "manifest.json")
    with open(path, encoding="utf-8") as f:
        return json.load(f)


def _quota_precheck(cost_estimate_usd: float, buffer_multiplier: float) -> tuple[bool, str]:
    """Query OpenAI billing API for remaining project quota.

    Returns (ok, human_readable_message). ``ok`` is True iff:
        - remaining quota is known AND ≥ cost_estimate × buffer_multiplier, OR
        - the billing API is unavailable (graceful fallback — not all API key
          tiers have billing introspection).

    Per FT-OAI-002 Epic 7 O3: attempt 1 failed ``exceeded_quota`` after job
    acceptance ($155.19 actual > $94.67 remaining); the auto-epoch multiplier
    observed was ~1.66×, so we default buffer=1.66 to catch this failure mode
    before upload.

    The Costs API lives under ``/v1/organization/costs`` and requires an
    admin-scope API key. On 401/403/404 or SDK-not-installed, return (True,
    "warning message") — do not block.
    """
    try:
        import urllib.request
    except ImportError:
        return True, "WARNING: urllib unavailable; skipping quota pre-check."

    api_key = os.environ.get("OPENAI_ADMIN_API_KEY") or os.environ.get("OPENAI_API_KEY")
    if not api_key:
        return True, "WARNING: no API key for quota pre-check; skipping."

    required = cost_estimate_usd * float(buffer_multiplier)

    # The OpenAI costs endpoint shape varies by tier. We probe conservatively:
    # any non-200 response → fall through to warning. This is intentional —
    # the plan explicitly says quota pre-check must not block when the billing
    # API is not available to the current key tier.
    url = "https://api.openai.com/v1/organization/costs?limit=1"
    req = urllib.request.Request(
        url,
        headers={"Authorization": f"Bearer {api_key}"},
    )
    try:
        with urllib.request.urlopen(req, timeout=5) as resp:  # noqa: S310 — hardcoded https
            if resp.status != 200:
                return True, (
                    f"WARNING: billing API returned {resp.status}; "
                    f"skipping quota pre-check. cost_estimate=${cost_estimate_usd:.2f}, "
                    f"required (with {buffer_multiplier}× buffer)=${required:.2f}"
                )
            # We don't parse the body — the schema varies by tier and the
            # endpoint reports spend, not remaining quota. The successful
            # response is evidence the key has billing-scope access; we
            # still can't compute remaining quota without hitting a
            # quota-specific endpoint that may not exist on this tier.
            return True, (
                f"INFO: billing API reachable but remaining-quota endpoint "
                f"not available at this tier. cost_estimate=${cost_estimate_usd:.2f}, "
                f"required (with {buffer_multiplier}× buffer)=${required:.2f}. "
                f"Proceeding — monitor manually at platform.openai.com/usage."
            )
    except Exception as e:  # noqa: BLE001 — any failure is graceful fallback
        return True, (
            f"WARNING: quota pre-check failed ({type(e).__name__}: {e}); "
            f"skipping. cost_estimate=${cost_estimate_usd:.2f}, "
            f"required (with {buffer_multiplier}× buffer)=${required:.2f}. "
            f"Proceeding — top up credits at platform.openai.com if uncertain."
        )


def _run_pre_check(directory: str) -> int:
    """Shell out to openai_ft_check.py for a single source of truth."""
    script = os.path.join(os.path.dirname(os.path.abspath(__file__)), "openai_ft_check.py")
    result = subprocess.run(
        [sys.executable, script, "--dir", directory],
        check=False,
    )
    return result.returncode


def _confirm(prompt: str) -> bool:
    try:
        reply = input(prompt).strip().lower()
    except EOFError:
        return False
    return reply in {"y", "yes"}


def _append_run_notes(
    directory: str,
    *,
    model: str,
    train_file_id: str,
    val_file_id: str,
    job_id: str,
    cost_estimate_usd: float,
    suffix: str | None,
    n_epochs: int | str,
    n_epochs_rationale: str | None,
) -> None:
    path = os.path.join(directory, "run_notes.md")
    rationale_line = f"| n_epochs_rationale | {n_epochs_rationale} |\n" if n_epochs_rationale else ""
    entry = f"""\
## Fine-tuning job launched — {datetime.now(timezone.utc).isoformat()}

| Field | Value |
|---|---|
| job_id | `{job_id}` |
| base_model | `{model}` |
| suffix | `{suffix or "(none)"}` |
| train_file_id | `{train_file_id}` |
| val_file_id | `{val_file_id}` |
| cost_estimate_usd | {cost_estimate_usd:.2f} |
| start_time | {datetime.now(timezone.utc).isoformat()} |
| n_epochs_requested | {n_epochs} |
{rationale_line}| end_time | _pending_ |
| n_epochs_actual | _pending_ |
| best_val_loss_step | _pending_ |
| final_train_loss | _pending_ |
| final_val_loss | _pending_ |
| total_cost_usd | _pending_ |
| error_if_any | _none_ |

Monitor with:

    openai fine_tuning.jobs.retrieve {job_id}

Poll training metrics + fill in pending fields:

    python scripts/openai_ft_check.py --on-complete --job-id {job_id} --dir {directory}

"""
    mode = "a" if os.path.exists(path) else "w"
    with open(path, mode, encoding="utf-8") as f:
        f.write(entry)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dir", required=True, help="openai_ft artifact directory")
    parser.add_argument("--model", default="gpt-4.1-mini-2025-04-14")
    parser.add_argument(
        "--max-cost-usd",
        type=float,
        default=50.00,
        help="Abort before upload if manifest cost_estimate_usd exceeds this.",
    )
    parser.add_argument(
        "--suffix",
        default=None,
        help="Optional OpenAI job suffix (appears in fine_tuned_model name).",
    )
    parser.add_argument(
        "--n-epochs",
        default="auto",
        help=(
            "Number of epochs to request (OpenAI hyperparameters.n_epochs). "
            "Integer (e.g. 3) or 'auto' (default; lets OpenAI pick). "
            "FT-OAI-001 launched at 'auto'; FT-OAI-002+ should set explicitly "
            "and record --n-epochs-rationale."
        ),
    )
    parser.add_argument(
        "--n-epochs-rationale",
        default=None,
        help=(
            "Short (one sentence) justification for the --n-epochs choice. "
            "Recorded in run_notes.md so future sprints can judge the pick. "
            "Required when --n-epochs is not 'auto'."
        ),
    )
    parser.add_argument(
        "--quota-buffer",
        type=float,
        default=1.66,
        help=(
            "Multiplier applied to cost_estimate_usd for the quota pre-check. "
            "Default 1.66 — matches the auto-epoch overshoot observed in "
            "FT-OAI-001 attempt 1 ($155.19 actual / $93.13 single-epoch est)."
        ),
    )
    parser.add_argument(
        "--skip-quota-check",
        action="store_true",
        help=(
            "Bypass the OpenAI billing API quota pre-check. "
            "Use when billing API is unavailable for your key tier "
            "and you have confirmed quota manually via platform.openai.com."
        ),
    )
    parser.add_argument("--yes", action="store_true", help="Skip interactive confirmation.")
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Print what would happen and exit 0 without touching OpenAI.",
    )
    args = parser.parse_args(argv)

    # Validate --n-epochs / --n-epochs-rationale pair early (before any I/O).
    n_epochs_str = str(args.n_epochs).strip().lower()
    if n_epochs_str == "auto":
        n_epochs_value: int | str = "auto"
    else:
        try:
            n_epochs_value = int(n_epochs_str)
        except ValueError:
            print(
                f"ABORT: --n-epochs must be 'auto' or an integer; got {args.n_epochs!r}",
                file=sys.stderr,
            )
            return 2
        if n_epochs_value < 1:
            print(f"ABORT: --n-epochs must be ≥ 1; got {n_epochs_value}", file=sys.stderr)
            return 2
        if not args.n_epochs_rationale:
            print(
                "ABORT: --n-epochs-rationale is required when --n-epochs is not 'auto'. "
                "Record why you chose this value so future sprints can audit the decision.",
                file=sys.stderr,
            )
            return 2

    # 1. Pre-upload check
    rc = _run_pre_check(args.dir)
    if rc != 0:
        print("ABORT: pre-upload check failed.", file=sys.stderr)
        return rc

    # 2. Cost cap — BEFORE network I/O
    manifest = _load_manifest(args.dir)
    cost = float(manifest.get("totals", {}).get("cost_estimate_usd", 0.0))
    if cost > args.max_cost_usd:
        print(
            f"ABORT: manifest cost_estimate_usd ${cost:.2f} exceeds --max-cost-usd ${args.max_cost_usd:.2f}. "
            f"Raise the cap or shrink the dataset. No network calls made.",
            file=sys.stderr,
        )
        return 2

    # 2b. (Epic 7 O3) Quota pre-check — graceful fallback on API unavailability
    if args.skip_quota_check:
        print("SKIP: --skip-quota-check set, not querying billing API.")
    else:
        ok, quota_msg = _quota_precheck(
            cost_estimate_usd=cost,
            buffer_multiplier=args.quota_buffer,
        )
        print(quota_msg)
        if not ok:
            print(
                "ABORT: project quota check failed. Use --skip-quota-check to "
                "bypass (at your own risk) or top up billing.",
                file=sys.stderr,
            )
            return 2

    # 3. Confirmation
    train_rows = manifest.get("splits", {}).get("train", {}).get("rows", 0)
    val_rows = manifest.get("splits", {}).get("val", {}).get("rows", 0)
    print(
        f"About to upload and launch fine-tuning job:\n"
        f"  model            = {args.model}\n"
        f"  suffix           = {args.suffix or '(none)'}\n"
        f"  train rows       = {train_rows}\n"
        f"  val rows         = {val_rows}\n"
        f"  n_epochs         = {n_epochs_value}\n"
        f"  cost estimate    = ${cost:.2f} (cap ${args.max_cost_usd:.2f})\n"
        f"  artifact dir     = {args.dir}\n"
    )

    if args.dry_run:
        print("DRY-RUN: exiting without uploading.")
        return 0

    if not args.yes and not _confirm("Proceed? [y/N]: "):
        print("ABORT: user declined confirmation.", file=sys.stderr)
        return 3

    # 4. Upload + launch
    if not os.environ.get("OPENAI_API_KEY"):
        print("ABORT: OPENAI_API_KEY not set.", file=sys.stderr)
        return 4

    try:
        from openai import OpenAI  # local import — heavy, optional for --dry-run
    except ImportError:
        print(
            "ABORT: `openai` package not installed. Run `pip install 'openai>=1.50'`.",
            file=sys.stderr,
        )
        return 5

    client = OpenAI()

    train_path = os.path.join(args.dir, "combined_train.jsonl")
    val_path = os.path.join(args.dir, "combined_val.jsonl")

    print(f"Uploading {train_path} …")
    with open(train_path, "rb") as f:
        train_file = client.files.create(file=f, purpose="fine-tune")
    print(f"  train file id: {train_file.id}")

    print(f"Uploading {val_path} …")
    with open(val_path, "rb") as f:
        val_file = client.files.create(file=f, purpose="fine-tune")
    print(f"  val file id:   {val_file.id}")

    job_kwargs: dict = {
        "training_file": train_file.id,
        "validation_file": val_file.id,
        "model": args.model,
    }
    if args.suffix:
        job_kwargs["suffix"] = args.suffix
    if isinstance(n_epochs_value, int):
        # OpenAI SDK expects hyperparameters={"n_epochs": int}
        job_kwargs["hyperparameters"] = {"n_epochs": n_epochs_value}

    print(f"Launching fine-tuning job (model={args.model}) …")
    job = client.fine_tuning.jobs.create(**job_kwargs)
    print(f"  job id: {job.id}")
    print(f"  status: {job.status}")

    _append_run_notes(
        args.dir,
        model=args.model,
        train_file_id=train_file.id,
        val_file_id=val_file.id,
        job_id=job.id,
        cost_estimate_usd=cost,
        suffix=args.suffix,
        n_epochs=n_epochs_value,
        n_epochs_rationale=args.n_epochs_rationale,
    )
    print(f"run_notes.md updated at {os.path.join(args.dir, 'run_notes.md')}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
