#!/usr/bin/env python3
"""distill_driver.py — Sprint FT-LORA-DATA Epic 2

Teacher-routing orchestrator for the 5 absent/scarce ULTS tasks (4 with
0 rows in the 21-day window, plus hidden.summarize which has 1 raw row —
would need 200× duplication, far exceeding the 5× ceiling, so synthesized
as a 5th teacher task per plan Refinement 9 / user decision 2026-04-22).
Thin wrapper over :mod:`training.teacher_distill` that adds:

  * Mixed-endpoint routing (OpenAI for reasoning-heavy tasks; local MLX
    for cheap classification / summarization).
  * Per-call token accounting + a **hard** cumulative budget cap with
    partial-output preservation when the cap fires.
  * Provenance meta sidecar per row (cuid2 trace_id, synth-source tag,
    ``synthesis_version = v1-{commit_sha_short}``, weak_signal flag,
    ``dataset_ver``).
  * ULTS output-schema validation; per-task valid-rate reported (warn
    only at < 95% — does NOT abort per plan §5 Epic 2 gate).
  * ``--dry-run`` token + cost estimate without any network call.
  * Raw-SHA pin assertion (defaults to Sprint-plan-pinned value from
    :mod:`training.recurate`).

Output format (identical to recurate.py so a downstream balanced_sampler
can concatenate real + synth rows without a format split):

    {
      "messages": [ {"role": "system", ...}, {"role": "user", ...},
                    {"role": "assistant", ...} ],
      "meta": {
        "source": "synth-gpt-5.4-mini" | "synth-qwen-local",
        "dataset_ver": "v2026-04-22",
        "task_name": "consulting.synthesis",
        "trace_id": "<cuid2>",
        "instance_id": "synth-gpt-5.4-mini",
        "space_id": "synth",
        "quality": null,
        "quality_source": "teacher_distillation",
        "source_path": "distill_driver:<task>",
        "synthesis_version": "v1-<commit_sha_short>",
        "weak_signal": true | false
      }
    }

Usage::

    # One task at a time (safe — small spend):
    python -m training.distill_driver \\
        --task consulting.synthesis \\
        --out training_data/curated/sft_synth_v1/consulting_synthesis.jsonl \\
        --dataset-ver v2026-04-22 \\
        --budget-cap-usd 0.50

    # All four tasks:
    python -m training.distill_driver --all \\
        --out-dir training_data/curated/sft_synth_v1 \\
        --dataset-ver v2026-04-22 \\
        --budget-cap-usd 0.50

    # Estimate cost without calling APIs:
    python -m training.distill_driver --all --dry-run \\
        --out-dir /tmp/dry \\
        --dataset-ver v2026-04-22
"""

from __future__ import annotations

import argparse
import dataclasses
import json
import os
import re
import subprocess
import sys
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

# sys.path setup — match the convention in recurate.py / paradigm_router.py.
_NEURAL_ROOT = Path(__file__).resolve().parents[1]
if str(_NEURAL_ROOT) not in sys.path:
    sys.path.insert(0, str(_NEURAL_ROOT))

from training.format_converter import convert_record, validate_mlx_format  # noqa: E402
from training.recurate import SPRINT_PLAN_RAW_SHA_PIN, assert_raw_sha  # noqa: E402
from training.teacher_distill import (  # noqa: E402
    extract_system_prompt,
    generate_inputs,
    validate_output,
)

# ── Defaults ────────────────────────────────────────────────────────────────

# Teacher routing: which tasks go to which endpoint. Values are declared as
# named fields rather than loose dicts so a typo fails at config-load time
# rather than mid-budget-burn.
@dataclass
class TeacherTask:
    teacher_id: str           # short id used in provenance tags
    endpoint: str             # "openai" | "mlx_local"
    count: int                # target row count per task (plan: 200)
    weak_signal: bool         # metalearn is known weak; flagged per-row
    model: str                # wire-level model name sent to the API


DEFAULT_TEACHER_CONFIG: dict[str, TeacherTask] = {
    "consulting.synthesis": TeacherTask(
        teacher_id="gpt-5.4-mini",
        endpoint="openai",
        count=200,
        weak_signal=False,
        model="gpt-5.4-mini",
    ),
    "metalearn.generalize": TeacherTask(
        teacher_id="gpt-5.4-mini",
        endpoint="openai",
        count=200,
        weak_signal=True,  # plan §10 row — propagated to downstream eval
        model="gpt-5.4-mini",
    ),
    "retrieval.rerank_nli": TeacherTask(
        teacher_id="qwen3.6-local",
        endpoint="mlx_local",
        count=200,
        weak_signal=False,
        model="mlx-community/Qwen3.6-35B-A3B-mxfp4",
    ),
    "summarize.generate": TeacherTask(
        teacher_id="qwen3.6-local",
        endpoint="mlx_local",
        count=200,
        weak_signal=False,
        model="mlx-community/Qwen3.6-35B-A3B-mxfp4",
    ),
    # Added 2026-04-22: hidden.summarize has 1 row in raw (would force
    # 200× duplication, far exceeding 5× ceiling). Synthesized as a 5th
    # teacher task. Reasoning-think family → OpenAI teacher.
    "hidden.summarize": TeacherTask(
        teacher_id="gpt-5.4-mini",
        endpoint="openai",
        count=200,
        weak_signal=False,
        model="gpt-5.4-mini",
    ),
}

# Endpoint URLs (OpenAI-compatible chat/completions).
# Canonical MLX port pinned to :8101 (Sprint FT-LORA-DATA plan §1 Header,
# 2026-04-22). Docker's com.docker.backend reserves :8100 via IPv6 LISTEN so
# MLX cannot bind there. Change requires plan update + CMS re-pin.
ENDPOINT_DEFAULTS = {
    "openai":    {"base_url": "https://api.openai.com/v1",  "api_key_env": "OPENAI_API_KEY"},
    "mlx_local": {"base_url": "http://127.0.0.1:8101/v1",   "api_key_env": None},
}

# Output filename mapping (plan §10 spec).
OUTPUT_FILENAME = {
    "consulting.synthesis":  "consulting_synthesis.jsonl",
    "metalearn.generalize":  "metalearn_generalize.jsonl",
    "retrieval.rerank_nli":  "rerank_nli.jsonl",
    "summarize.generate":    "summarize_generate.jsonl",
    "hidden.summarize":      "hidden_summarize.jsonl",
}


# ── Pricing (CLI-configurable, no hardcoded value in the call path) ────────

# gpt-5.4-mini observed public pricing at 2026-04-22 — defaults only;
# override via CLI when provider pricing changes.
DEFAULT_INPUT_PRICE_PER_M = 0.25   # $ per 1e6 input tokens
DEFAULT_OUTPUT_PRICE_PER_M = 2.00  # $ per 1e6 output tokens

# Rough token estimate = ceil(chars / 4). Used for --dry-run and for
# running cost estimation before the API returns real usage numbers.
def estimate_tokens(text: str) -> int:
    return max(1, (len(text) + 3) // 4)


# ── Commit SHA (synthesis_version stamp) ────────────────────────────────────

def _commit_sha_short(repo_root: Path | None = None) -> str:
    """Return the short commit SHA of the current HEAD. ``unknown`` on error.

    Sprint plan §3 Synthesis versioning policy: every synth row stamps
    ``synthesis_version = v1-{commit_sha_short}``. If git is unavailable
    (e.g. CI worktree rebuild) we still produce output, just with an
    ``unknown`` tag — tests cover this path.
    """
    try:
        out = subprocess.check_output(
            ["git", "rev-parse", "--short=8", "HEAD"],
            cwd=str(repo_root) if repo_root else None,
            stderr=subprocess.DEVNULL,
            text=True,
        )
        return out.strip() or "unknown"
    except (OSError, subprocess.CalledProcessError):
        return "unknown"


# ── Budget tracker ──────────────────────────────────────────────────────────

@dataclass
class BudgetTracker:
    cap_usd: float
    input_price_per_m: float = DEFAULT_INPUT_PRICE_PER_M
    output_price_per_m: float = DEFAULT_OUTPUT_PRICE_PER_M
    # endpoint -> {"input_tokens": n, "output_tokens": n, "cost_usd": x}
    per_endpoint: dict[str, dict[str, float]] = field(default_factory=dict)

    def _price(self, endpoint: str, in_toks: int, out_toks: int) -> float:
        if endpoint != "openai":
            return 0.0
        return (
            in_toks * self.input_price_per_m / 1_000_000.0
            + out_toks * self.output_price_per_m / 1_000_000.0
        )

    def charge(self, endpoint: str, in_toks: int, out_toks: int) -> float:
        cost = self._price(endpoint, in_toks, out_toks)
        row = self.per_endpoint.setdefault(
            endpoint, {"input_tokens": 0, "output_tokens": 0, "cost_usd": 0.0},
        )
        row["input_tokens"] += in_toks
        row["output_tokens"] += out_toks
        row["cost_usd"] += cost
        return cost

    @property
    def total_cost_usd(self) -> float:
        return sum(r["cost_usd"] for r in self.per_endpoint.values())

    def would_exceed(self, projected_additional_cost: float) -> bool:
        return (self.total_cost_usd + projected_additional_cost) > self.cap_usd

    def snapshot(self) -> dict[str, Any]:
        return {
            "cap_usd": self.cap_usd,
            "total_cost_usd": round(self.total_cost_usd, 6),
            "per_endpoint": {
                k: {
                    "input_tokens": v["input_tokens"],
                    "output_tokens": v["output_tokens"],
                    "cost_usd": round(v["cost_usd"], 6),
                }
                for k, v in self.per_endpoint.items()
            },
            "input_price_per_m": self.input_price_per_m,
            "output_price_per_m": self.output_price_per_m,
        }


class BudgetExceeded(Exception):
    """Raised mid-run when the next API call would breach the cap."""


class PreflightFailed(Exception):
    """Raised when Epic 6.0 pre-flight checks detect a blocking env issue."""


# ── Epic 6.0 — Pre-flight + single-instance enforcement + per-row logging ───

def preflight_endpoint(
    base_url: str, timeout: float = 5.0, api_key: str | None = None,
) -> None:
    """Verify an OpenAI-compatible endpoint is reachable via GET /models.

    Pattern reused from ``scripts/test_vllm_mlx.py:243-251``. Fails loudly
    and fast so the operator never watches 200 silent rows fail.

    ``api_key`` is forwarded as ``Authorization: Bearer`` when set — real
    OpenAI rejects unauthenticated GET /models with 401, so this is
    mandatory for ``endpoint=openai``.
    """
    url = f"{base_url.rstrip('/')}/models"
    headers = {}
    if api_key:
        headers["Authorization"] = f"Bearer {api_key}"
    try:
        req = Request(url, method="GET", headers=headers)
        with urlopen(req, timeout=timeout) as resp:
            resp.read()
    except (HTTPError, URLError, TimeoutError, OSError) as e:
        raise PreflightFailed(
            f"Endpoint unreachable at {url}: {e}. "
            f"Verify server is running and URL is correct before distillation."
        ) from e


def count_mlx_server_instances() -> list[tuple[int, str]]:
    """Return ``(pid, command)`` tuples for every running ``mlx_lm.server``.

    Used by the single-instance guard — Sprint plan §3 Constraints pins the
    canonical MLX on ``:8101``; a stray instance on ``:8200`` was observed
    2026-04-22 confusing diagnostics. Returns empty list on any ps failure
    (guard opens rather than aborts — rely on preflight_endpoint for reach).
    """
    try:
        out = subprocess.check_output(
            ["ps", "-Ao", "pid,command"], text=True, stderr=subprocess.DEVNULL,
        )
    except (OSError, subprocess.CalledProcessError):
        return []
    hits: list[tuple[int, str]] = []
    for line in out.splitlines():
        stripped = line.strip()
        # Match both ``python -m mlx_lm.server …`` and direct ``mlx_lm.server``.
        if "mlx_lm.server" not in stripped:
            continue
        parts = stripped.split(None, 1)
        if len(parts) < 2 or not parts[0].isdigit():
            continue
        hits.append((int(parts[0]), parts[1]))
    return hits


def enforce_single_mlx_instance() -> None:
    """Abort if more than one ``mlx_lm.server`` is running.

    Sprint plan Epic 6.0 item 3: the canonical instance lives on :8101;
    any stragglers must be killed before a distillation run to keep
    diagnostics unambiguous.
    """
    hits = count_mlx_server_instances()
    if len(hits) > 1:
        listing = "\n".join(f"  PID {pid}: {cmd}" for pid, cmd in hits)
        raise PreflightFailed(
            f"Multiple mlx_lm.server instances running ({len(hits)}); "
            f"Sprint plan pins exactly one on :8101. "
            f"Kill strays before proceeding:\n{listing}"
        )


def log_row(
    task_name: str,
    i: int,
    count: int,
    status: str,
    cum_cost: float,
    tokens_in: int,
    tokens_out: int,
    elapsed_ms: int,
    extra: str | None = None,
) -> None:
    """Emit one structured per-row stderr line and flush.

    Format matches Sprint plan Epic 6.0 item 1 so grep/awk extraction is
    trivial. Status is one of ``ok``, ``api_error``, ``schema_invalid``,
    ``budget_abort``, ``preflight_ok``.
    """
    tail = f"  {extra}" if extra else ""
    sys.stderr.write(
        f"[{task_name} {i}/{count}] status={status} cost=${cum_cost:.4f} "
        f"tokens_in={tokens_in} tokens_out={tokens_out} "
        f"elapsed_ms={elapsed_ms}{tail}\n"
    )
    sys.stderr.flush()


def _redact(s: str, max_len: int = 500) -> str:
    """Redact Bearer/Authorization/api-key substrings and truncate."""
    s = re.sub(r"(?i)(bearer\s+|authorization:\s*|api[_-]?key[\"'=:\s]+)[^\s\"']+",
               r"\1***REDACTED***", s)
    if len(s) > max_len:
        return s[:max_len] + f"…(+{len(s) - max_len} chars)"
    return s


def write_debug_entry(debug_path: Path | None, entry: dict[str, Any]) -> None:
    """Append a JSON line to the debug log if configured."""
    if debug_path is None:
        return
    try:
        debug_path.parent.mkdir(parents=True, exist_ok=True)
        with open(debug_path, "a") as fh:
            fh.write(json.dumps(entry, default=str) + "\n")
    except OSError as e:
        sys.stderr.write(f"WARN: failed to write debug log {debug_path}: {e}\n")


# ── HTTP call with usage capture ────────────────────────────────────────────

def _chat_completion(
    base_url: str,
    model: str,
    system_prompt: str,
    user_prompt: str,
    max_tokens: int,
    think_mode: bool,
    api_key: str | None,
    timeout: int = 180,
    max_retries: int = 3,
    debug_log: Path | None = None,
    task_name: str = "",
    attempt_idx: int = 0,
) -> tuple[str | None, str | None, dict[str, int], str | None, str | None]:
    """Call OpenAI-compatible /chat/completions.

    Returns ``(response, think_content, usage_dict, error_str, body_preview)``.
    ``usage_dict`` has ``prompt_tokens`` + ``completion_tokens``; the API
    result is authoritative, so we use it verbatim when present.

    Epic 6.0:
      * ``timeout`` is a float-seconds wall clock (no longer 180 hardcoded).
      * Retries up to ``max_retries`` on 5xx / 429 / timeout with
        exponential backoff (1s, 2s, 4s).
      * On any failure, returns a redacted ``body_preview`` (first ~500
        chars of response) and writes a JSONL entry to ``debug_log`` when
        configured so the operator can post-mortem without re-running.
    """
    # OpenAI's newer models (gpt-5.x, gpt-4.1+) require
    # ``max_completion_tokens``; MLX-local / legacy endpoints still accept
    # the original ``max_tokens``. Pick the right key based on base_url.
    is_openai = "api.openai.com" in base_url
    token_key = "max_completion_tokens" if is_openai else "max_tokens"
    body: dict[str, Any] = {
        "model": model,
        "messages": [
            {"role": "system", "content": system_prompt},
            {"role": "user", "content": user_prompt},
        ],
        token_key: max_tokens,
        "temperature": 0.7,  # matches Sprint C Gate 2 canonical T/C/J groups
    }
    # Qwen/MLX template hook. OpenAI rejects this field — so only forward
    # it to MLX-local. OpenAI reasoning models (o1/o3) would use
    # ``reasoning_effort`` instead; gpt-5.x chat-completions does its own
    # thinking internally without a user-facing switch.
    #
    # Qwen3.6 defaults thinking=ON at the template level; unless we
    # explicitly set enable_thinking=False for think_mode tasks, the
    # model returns ``message.reasoning`` (and no ``content``), which
    # causes distill_driver to drop the row as api_error. So ALWAYS
    # send the explicit boolean to MLX — never leave it unset.
    if not is_openai:
        body["chat_template_kwargs"] = {"enable_thinking": bool(think_mode)}

    headers = {"Content-Type": "application/json"}
    if api_key:
        headers["Authorization"] = f"Bearer {api_key}"

    req_body = json.dumps(body).encode()
    req = Request(
        f"{base_url.rstrip('/')}/chat/completions",
        data=req_body,
        headers=headers,
        method="POST",
    )

    request_body_summary = {
        "model": model,
        "token_key": token_key,
        "max_tokens_requested": max_tokens,
        "think_mode": think_mode,
        "system_prompt_len": len(system_prompt),
        "user_prompt_len": len(user_prompt),
    }

    last_err: str | None = None
    last_preview: str | None = None
    last_status: int | None = None
    attempts_used = 0
    max_attempts = max(1, max_retries)
    for attempt in range(max_attempts):
        attempts_used = attempt + 1
        try:
            with urlopen(req, timeout=timeout) as resp:
                raw = resp.read()
                payload = json.loads(raw)
            break  # success
        except HTTPError as e:
            try:
                err_body = e.read().decode()
            except Exception:
                err_body = ""
            last_preview = _redact(err_body)
            last_status = e.code
            last_err = f"HTTP {e.code}: {e.reason} — {last_preview}"
            # Retry on 429 / 5xx only.
            if e.code == 429 or 500 <= e.code < 600:
                if attempt + 1 < max_attempts:
                    time.sleep(2 ** attempt)
                    continue
            # Non-retriable — break out.
            write_debug_entry(debug_log, {
                "task": task_name, "row": attempt_idx,
                "status_code": e.code, "attempts": attempts_used,
                "body_preview": last_preview,
                "request_body_summary": request_body_summary,
            })
            return None, None, {}, last_err, last_preview
        except (URLError, TimeoutError) as e:
            last_err = f"network/timeout: {e}"
            last_preview = last_err
            last_status = None
            if attempt + 1 < max_attempts:
                time.sleep(2 ** attempt)
                continue
            write_debug_entry(debug_log, {
                "task": task_name, "row": attempt_idx,
                "status_code": None, "attempts": attempts_used,
                "body_preview": last_preview,
                "request_body_summary": request_body_summary,
            })
            return None, None, {}, last_err, last_preview
        except json.JSONDecodeError as e:
            last_err = f"bad json from teacher: {e}"
            last_preview = _redact(raw.decode(errors="replace") if "raw" in dir() else "")
            write_debug_entry(debug_log, {
                "task": task_name, "row": attempt_idx,
                "status_code": 200, "attempts": attempts_used,
                "body_preview": last_preview,
                "request_body_summary": request_body_summary,
            })
            return None, None, {}, last_err, last_preview
    else:  # pragma: no cover — loop exits via break/return
        return None, None, {}, last_err or "unknown error", last_preview

    try:
        content = payload["choices"][0]["message"]["content"]
    except (KeyError, IndexError, TypeError) as e:
        preview = _redact(json.dumps(payload)[:500])
        write_debug_entry(debug_log, {
            "task": task_name, "row": attempt_idx,
            "status_code": 200, "attempts": attempts_used,
            "body_preview": preview,
            "request_body_summary": request_body_summary,
            "error": f"malformed response: {e}",
        })
        return None, None, {}, f"malformed response: {e}", preview

    usage = payload.get("usage") or {}
    usage_dict = {
        "prompt_tokens": int(usage.get("prompt_tokens") or 0),
        "completion_tokens": int(usage.get("completion_tokens") or 0),
    }

    # Extract <think>...</think> if present.
    think_content = None
    response = content
    think_match = re.match(r"<think>\n?(.*?)\n?</think>\n\n?(.*)", content, re.DOTALL)
    if think_match:
        think_content = think_match.group(1).strip()
        response = think_match.group(2).strip()

    return response, think_content, usage_dict, None, None


# ── ULTS spec loading ───────────────────────────────────────────────────────

def _strip_fence(text: str) -> str:
    """Strip ```json ... ``` or ``` ... ``` fences if present. Best-effort."""
    t = (text or "").strip()
    if t.startswith("```"):
        # drop first line (```json or ```) and trailing ```
        lines = t.split("\n")
        if len(lines) >= 2:
            lines = lines[1:]
            if lines and lines[-1].strip().startswith("```"):
                lines = lines[:-1]
            t = "\n".join(lines).strip()
    return t


def _validate_output_local(response: str, schema: dict[str, Any]) -> bool:
    """Schema-aware validator that also handles arrays and fenced JSON.

    Upstream ``teacher_distill.validate_output`` only handles string and
    dict; our ULTS specs include ``type: array`` (retrieval.rerank_nli)
    and teachers frequently wrap JSON in markdown fences. This local
    override accepts both.
    """
    stype = schema.get("type", "string")
    if stype == "string":
        return bool(response and response.strip())
    try:
        parsed = json.loads(_strip_fence(response))
    except (json.JSONDecodeError, TypeError):
        return False
    if stype == "array":
        return isinstance(parsed, list)
    if not isinstance(parsed, dict):
        return False
    required = schema.get("required", []) or []
    return all(k in parsed for k in required)


def _schema_instruction(schema: dict[str, Any]) -> str:
    """Build a per-schema instruction appended to the system prompt.

    Rationale: Go-source-extracted system prompts describe the task in
    natural language — they do not instruct the teacher to emit a
    machine-parseable structure, because in production the Go runtime
    constrains format out-of-band (structured decoding, grammars, etc.).
    For teacher distillation we need the teacher's reply to pass
    ``validate_output`` against the ULTS ``output_schema``, so we
    explicitly tell the teacher what JSON shape to return.
    """
    stype = schema.get("type")
    if stype == "object":
        props = schema.get("properties", {}) or {}
        required = schema.get("required", []) or []
        if not props:
            return (
                "\n\nRespond ONLY with a valid JSON object (no prose, "
                "no markdown fence). Return the object directly."
            )
        field_lines = []
        for k, v in props.items():
            tt = v.get("type", "string") if isinstance(v, dict) else "string"
            tag = " (required)" if k in required else ""
            field_lines.append(f"  - {k}: {tt}{tag}")
        return (
            "\n\nRespond ONLY with a JSON object matching this schema "
            "(no prose before or after, no markdown code fences):\n"
            + "\n".join(field_lines)
        )
    if stype == "array":
        items = schema.get("items") or {}
        hint = ""
        if isinstance(items, dict) and items.get("type"):
            hint = f" of {items['type']} items"
        return (
            f"\n\nRespond ONLY with a valid JSON array{hint} "
            "(no prose before or after, no markdown code fences)."
        )
    return ""


def _filename_for_task(task_name: str) -> str:
    """consulting.synthesis -> consulting_synthesis.ults.json"""
    return task_name.replace(".", "_") + ".ults.json"


def load_spec(ults_dir: Path, task_name: str) -> dict[str, Any]:
    spec_path = ults_dir / _filename_for_task(task_name)
    if not spec_path.exists():
        raise FileNotFoundError(f"ULTS spec not found: {spec_path}")
    with open(spec_path) as f:
        return json.load(f)


# ── Per-row synth → chat format adapter ─────────────────────────────────────

def _make_raw_record(
    task_name: str,
    system_prompt: str,
    user_prompt: str,
    response: str,
    think_content: str | None,
    think_mode: bool,
    trace_id: str,
) -> dict[str, Any]:
    """Shape a teacher reply into the raw schema that format_converter expects."""
    return {
        "task_name": task_name,
        "system_prompt": system_prompt,
        "user_prompt": user_prompt,
        "response": response,
        "think_content": think_content,
        "think_mode": think_mode,
        "trace_id": trace_id,
        # RAFT context is never included for synth rows — no retrieval graph.
        "retrieval_node_ids": None,
        "retrieval_scores": None,
    }


def _build_meta(
    *,
    task_name: str,
    trace_id: str,
    cfg: TeacherTask,
    dataset_ver: str,
    synthesis_version: str,
) -> dict[str, Any]:
    return {
        "source": f"synth-{cfg.teacher_id}",
        "dataset_ver": dataset_ver,
        "task_name": task_name,
        "trace_id": trace_id,
        "instance_id": f"synth-{cfg.teacher_id}",
        "space_id": "synth",
        "quality": None,
        "quality_source": "teacher_distillation",
        "source_path": f"distill_driver:{task_name}",
        "synthesis_version": synthesis_version,
        "weak_signal": cfg.weak_signal,
    }


# ── Per-task driver ─────────────────────────────────────────────────────────

@dataclass
class TaskResult:
    task_name: str
    attempts: int = 0
    emitted: int = 0
    invalid_schema: int = 0
    api_errors: int = 0

    @property
    def valid_rate(self) -> float:
        if self.attempts == 0:
            return 0.0
        return self.emitted / self.attempts


def run_one_task(
    task_name: str,
    cfg: TeacherTask,
    ults_dir: Path,
    project_root: Path,
    out_path: Path,
    dataset_ver: str,
    synthesis_version: str,
    budget: BudgetTracker,
    dry_run: bool,
    sleep_ms: int,
    seed: int,
    cuid_gen,
    endpoint_urls: dict[str, dict[str, str]],
    *,
    count_override: int | None = None,
    strict: bool = False,
    http_timeout_s: float = 60.0,
    max_retries: int = 3,
    debug_log: Path | None = None,
    preflight: bool = True,
) -> TaskResult:
    """Drive one task end-to-end. Writes JSONL to ``out_path`` row-by-row so
    a mid-run abort preserves partial output.

    Epic 6.0 additions:
      * ``count_override`` — override ``cfg.count`` (smoke-test friendly).
      * ``strict`` — abort the task on first non-ok row instead of silently
        continuing.
      * ``http_timeout_s`` / ``max_retries`` — forwarded to ``_chat_completion``.
      * ``debug_log`` — JSONL path that captures response payloads on
        failure.
      * ``preflight`` — when True (default), verify endpoint reachability
        via GET /models before the first request.
      * Per-row structured stderr logging with ``fout.flush()`` after every
        successful write so on-disk row count reflects real progress.
    """
    spec = load_spec(ults_dir, task_name)
    prompt_cfg = spec.get("prompt", {})
    think_mode = bool(prompt_cfg.get("think_mode"))
    schema = spec.get("output_schema", {})
    perf = spec.get("performance", {})
    max_tokens = int(perf.get("max_tokens", 3000))

    # System prompt extraction from Go source (Sprint C / teacher_distill convention).
    source_ref = prompt_cfg.get("system_prompt_source")
    system_prompt = None
    if source_ref:
        system_prompt = extract_system_prompt(source_ref, str(project_root))
    if not system_prompt:
        desc = spec.get("task", {}).get("description", task_name)
        system_prompt = f"You are {desc}."

    # Append schema-driven JSON instruction so the teacher emits output
    # that will pass validate_output (Epic 6 hotfix 2026-04-22 — without
    # this, consulting.synthesis emitted 0/34 rows because the Go-source
    # prompt doesn't constrain format).
    system_prompt = system_prompt + _schema_instruction(schema)

    target_count = count_override if count_override is not None else cfg.count
    inputs = generate_inputs(task_name, target_count, seed=seed)
    if not inputs:
        raise RuntimeError(
            f"No input templates registered for task '{task_name}' in teacher_distill.py",
        )

    endpoint_cfg = endpoint_urls[cfg.endpoint]
    base_url = endpoint_cfg["base_url"]
    api_key = (
        os.environ.get(endpoint_cfg["api_key_env"]) if endpoint_cfg.get("api_key_env") else None
    )
    if cfg.endpoint == "openai" and not api_key and not dry_run:
        raise RuntimeError(
            f"OPENAI_API_KEY not set; task {task_name} requires OpenAI "
            "(use --dry-run to estimate without calling).",
        )

    # Epic 6.0 item 2 — endpoint pre-flight before the first request.
    if preflight and not dry_run:
        preflight_endpoint(base_url, api_key=api_key)
        log_row(task_name, 0, target_count, "preflight_ok",
                budget.total_cost_usd, 0, 0, 0,
                extra=f"endpoint={base_url}")

    result = TaskResult(task_name=task_name)
    out_path.parent.mkdir(parents=True, exist_ok=True)

    with open(out_path, "w") as fout:
        for user_prompt in inputs:
            est_in = estimate_tokens(system_prompt) + estimate_tokens(user_prompt)
            est_out = max_tokens  # worst-case for dry-run estimation
            result.attempts += 1
            row_idx = result.attempts
            t_start = time.monotonic()

            if dry_run:
                # No network; account estimated tokens and continue. The
                # cap check happens AFTER charging, so the first
                # over-cap row still lands in the tally (matches plan
                # §5 Epic 2 Step 4: "After every row: if cumulative
                # estimated cost crosses --budget-cap-usd, abort").
                budget.charge(cfg.endpoint, est_in, est_out)
                if budget.total_cost_usd > budget.cap_usd:
                    raise BudgetExceeded(
                        f"Cumulative cost ${budget.total_cost_usd:.4f} crossed cap "
                        f"${budget.cap_usd:.2f} after attempt {result.attempts} "
                        f"in {task_name} (dry-run estimate); "
                        f"{result.emitted} rows preserved.",
                    )
                log_row(task_name, row_idx, target_count, "ok",
                        budget.total_cost_usd, est_in, est_out,
                        int((time.monotonic() - t_start) * 1000),
                        extra="dry_run")
                continue

            response, think_content, usage, error, body_preview = _chat_completion(
                base_url=base_url,
                model=cfg.model,
                system_prompt=system_prompt,
                user_prompt=user_prompt,
                max_tokens=max_tokens,
                think_mode=think_mode,
                api_key=api_key,
                timeout=http_timeout_s,
                max_retries=max_retries,
                debug_log=debug_log,
                task_name=task_name,
                attempt_idx=row_idx,
            )
            elapsed_ms = int((time.monotonic() - t_start) * 1000)
            # Charge real usage if the API gave it, else estimated.
            in_toks = usage.get("prompt_tokens") or est_in
            out_toks = usage.get("completion_tokens") or 0
            budget.charge(cfg.endpoint, in_toks, out_toks)

            # Post-charge cap check (after-every-row semantics).
            if budget.total_cost_usd > budget.cap_usd:
                log_row(task_name, row_idx, target_count, "budget_abort",
                        budget.total_cost_usd, in_toks, out_toks, elapsed_ms)
                raise BudgetExceeded(
                    f"Cumulative cost ${budget.total_cost_usd:.4f} crossed cap "
                    f"${budget.cap_usd:.2f} on real call #{result.attempts} "
                    f"in {task_name}; {result.emitted} rows preserved.",
                )

            if error:
                result.api_errors += 1
                log_row(task_name, row_idx, target_count, "api_error",
                        budget.total_cost_usd, in_toks, out_toks, elapsed_ms,
                        extra=f"err={_redact(error, 120)}")
                if strict:
                    raise RuntimeError(
                        f"--strict: task {task_name} aborted on api_error "
                        f"at row {row_idx}: {error}"
                    )
                continue

            # Strip markdown code fences from JSON-shaped responses so the
            # training corpus contains clean payloads (without this, the
            # student learns to emit ```json fences).
            if schema.get("type") in ("object", "array"):
                response = _strip_fence(response)

            if not _validate_output_local(response, schema):
                result.invalid_schema += 1
                write_debug_entry(debug_log, {
                    "task": task_name, "row": row_idx,
                    "status_code": 200, "error": "validate_output=False",
                    "body_preview": _redact(response or "", 500),
                })
                log_row(task_name, row_idx, target_count, "schema_invalid",
                        budget.total_cost_usd, in_toks, out_toks, elapsed_ms,
                        extra="validate_output=False")
                if strict:
                    raise RuntimeError(
                        f"--strict: task {task_name} aborted on schema_invalid "
                        f"at row {row_idx}"
                    )
                continue

            trace_id = cuid_gen()
            raw = _make_raw_record(
                task_name=task_name,
                system_prompt=system_prompt,
                user_prompt=user_prompt,
                response=response,
                think_content=think_content,
                think_mode=think_mode,
                trace_id=trace_id,
            )
            # Chat-format conversion; synth rows never include RAFT, so
            # raft_ratio=0 keeps the user message clean.
            converted = convert_record(raw, raft_ratio=0.0)
            schema_errors = validate_mlx_format(converted)
            if schema_errors:
                # Shouldn't happen given validate_output already passed, but
                # guard so one malformed response doesn't corrupt the file.
                result.invalid_schema += 1
                write_debug_entry(debug_log, {
                    "task": task_name, "row": row_idx,
                    "status_code": 200, "error": f"mlx_format: {schema_errors}",
                    "body_preview": _redact(response or "", 500),
                })
                log_row(task_name, row_idx, target_count, "schema_invalid",
                        budget.total_cost_usd, in_toks, out_toks, elapsed_ms,
                        extra=f"mlx_format={schema_errors}")
                if strict:
                    raise RuntimeError(
                        f"--strict: task {task_name} aborted on mlx_format "
                        f"at row {row_idx}: {schema_errors}"
                    )
                continue

            converted["meta"] = _build_meta(
                task_name=task_name,
                trace_id=trace_id,
                cfg=cfg,
                dataset_ver=dataset_ver,
                synthesis_version=synthesis_version,
            )
            fout.write(json.dumps(converted) + "\n")
            fout.flush()  # Epic 6.0 item 1 — live progress on disk
            result.emitted += 1
            log_row(task_name, row_idx, target_count, "ok",
                    budget.total_cost_usd, in_toks, out_toks, elapsed_ms)

            if sleep_ms > 0:
                time.sleep(sleep_ms / 1000.0)

    return result


# ── CLI ─────────────────────────────────────────────────────────────────────

def _print_report(
    results: list[TaskResult],
    budget: BudgetTracker,
    dry_run: bool,
    warn_threshold: float,
) -> None:
    print()
    print("═══ distill_driver.py — report ═══")
    print(f"Dry run:          {dry_run}")
    print(f"Budget cap:       ${budget.cap_usd:.2f}")
    print(f"Spend so far:     ${budget.total_cost_usd:.4f}")
    for endpoint, v in budget.per_endpoint.items():
        print(
            f"  {endpoint:10s}  in_toks={v['input_tokens']:>8}  "
            f"out_toks={v['output_tokens']:>8}  cost=${v['cost_usd']:.4f}"
        )
    print()
    print(f"{'task':30s} {'attempts':>8} {'emitted':>8} "
          f"{'schema_err':>10} {'api_err':>8} {'valid_rate':>10}")
    for r in results:
        warn = "  WARN" if r.valid_rate < warn_threshold else ""
        print(
            f"{r.task_name:30s} {r.attempts:>8} {r.emitted:>8} "
            f"{r.invalid_schema:>10} {r.api_errors:>8} {r.valid_rate:>10.3f}{warn}"
        )


def _resolve_tasks(args) -> list[str]:
    if args.all:
        return list(DEFAULT_TEACHER_CONFIG.keys())
    if args.task:
        unknown = [t for t in args.task if t not in DEFAULT_TEACHER_CONFIG]
        if unknown:
            raise SystemExit(
                f"ERROR: unknown task(s): {unknown}. "
                f"Known: {sorted(DEFAULT_TEACHER_CONFIG.keys())}",
            )
        return args.task
    raise SystemExit("ERROR: must pass --task NAME [--task NAME ...] or --all")


def _out_path_for(args, task_name: str) -> Path:
    if args.all or not args.out:
        return Path(args.out_dir) / OUTPUT_FILENAME[task_name]
    return Path(args.out)


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(
        description="Sprint FT-LORA-DATA Epic 2 — teacher-routing distillation orchestrator.",
    )
    sel = p.add_mutually_exclusive_group()
    sel.add_argument("--task", action="append",
                     help="Task name to distill; repeatable. Mutually exclusive with --all.")
    sel.add_argument("--all", action="store_true",
                     help="Distill all 5 absent/scarce tasks using DEFAULT_TEACHER_CONFIG.")

    p.add_argument("--out", type=Path,
                   help="Output JSONL (single-task mode).")
    p.add_argument("--out-dir", type=Path,
                   default=Path("training_data/curated/sft_synth_v1"),
                   help="Output directory (multi-task / --all mode).")
    p.add_argument("--dataset-ver", required=True,
                   help="Dataset version stamp for each row's meta.dataset_ver "
                        "(e.g. v2026-04-22).")
    p.add_argument("--budget-cap-usd", type=float, default=100.00,
                   help="Hard abort cap for cumulative OpenAI spend (default $100; "
                        "per user directive 2026-04-22 — only flag if spend > $100).")
    p.add_argument("--input-price-per-m", type=float, default=DEFAULT_INPUT_PRICE_PER_M,
                   help="OpenAI input token price per 1e6 tokens (default: gpt-5.4-mini).")
    p.add_argument("--output-price-per-m", type=float, default=DEFAULT_OUTPUT_PRICE_PER_M,
                   help="OpenAI output token price per 1e6 tokens (default: gpt-5.4-mini).")
    p.add_argument("--dry-run", action="store_true",
                   help="Estimate tokens + cost without any API calls.")
    p.add_argument("--sleep-ms", type=int, default=200,
                   help="Inter-call sleep (ms) to be polite to the endpoint.")
    p.add_argument("--seed", type=int, default=42,
                   help="Seed for input-template generation (default 42).")
    p.add_argument("--ults-dir", type=Path,
                   default=Path("docs/tests/ults/specs"),
                   help="Directory of *.ults.json specs.")
    p.add_argument("--project-root", type=Path, default=Path("."),
                   help="Repo root for system-prompt extraction.")
    p.add_argument("--raw-input", type=Path,
                   default=Path("training_data/raw/extracted/llm_interactions.jsonl"),
                   help="Raw corpus path — SHA-pinned to sprint plan.")
    p.add_argument("--expected-raw-sha256", default=SPRINT_PLAN_RAW_SHA_PIN,
                   help="SHA256 pin for --raw-input. Default: sprint-plan-pinned value.")
    p.add_argument("--skip-sha-check", action="store_true",
                   help="Skip raw-input SHA verification. Use ONLY for unit tests.")
    p.add_argument("--warn-rate", type=float, default=0.95,
                   help="Per-task valid-rate below this prints WARN (does NOT abort).")
    p.add_argument("--openai-base-url", default=ENDPOINT_DEFAULTS["openai"]["base_url"],
                   help="Override OpenAI base URL.")
    p.add_argument("--mlx-local-url", default=ENDPOINT_DEFAULTS["mlx_local"]["base_url"],
                   help="Override local MLX base URL.")
    p.add_argument("--report", type=Path,
                   help="Optional JSON report output path.")
    # Epic 6.0 CLI flags (added 2026-04-22).
    p.add_argument("--count", type=int, default=None,
                   help="Override per-task row count (smoke-test friendly). "
                        "Default: use DEFAULT_TEACHER_CONFIG.count.")
    p.add_argument("--strict", action="store_true",
                   help="Abort a task on first api_error / schema_invalid row "
                        "(off by default; on recommended for smoke tests).")
    p.add_argument("--http-timeout-s", type=float, default=60.0,
                   help="HTTP timeout in seconds for teacher calls (default 60s).")
    p.add_argument("--max-retries", type=int, default=3,
                   help="Max attempts per row on 5xx/429/timeout with exponential "
                        "backoff (default 3).")
    p.add_argument("--debug-log", type=Path, default=None,
                   help="JSONL path for per-failure payload + request-body capture. "
                        "Default: {out_dir}/_debug.jsonl (only when unset and multi-task).")
    p.add_argument("--skip-preflight", action="store_true",
                   help="Skip endpoint GET /models pre-flight. Use ONLY for unit tests.")
    p.add_argument("--skip-single-instance-check", action="store_true",
                   help="Skip the MLX single-instance guard. Use ONLY for tests.")
    args = p.parse_args(argv)

    if not args.skip_sha_check:
        if not args.raw_input.exists():
            sys.stderr.write(f"ERROR: --raw-input not found: {args.raw_input}\n")
            return 2
        assert_raw_sha(args.raw_input, args.expected_raw_sha256)

    tasks = _resolve_tasks(args)

    # Epic 6.0 item 3 — single-instance enforcement when any task uses MLX.
    if (
        not args.skip_single_instance_check
        and not args.dry_run
        and any(DEFAULT_TEACHER_CONFIG[t].endpoint == "mlx_local" for t in tasks)
    ):
        try:
            enforce_single_mlx_instance()
        except PreflightFailed as e:
            sys.stderr.write(f"PREFLIGHT: {e}\n")
            return 4
    if len(tasks) > 1 and args.out:
        raise SystemExit(
            "ERROR: --out is only valid for a single task; use --out-dir for multi-task.",
        )

    # cuid2 generator (lazily imported so unit tests that mock it can stub).
    from cuid2 import Cuid  # noqa: PLC0415 — intentional import-on-use
    cuid_gen = Cuid(length=24).generate

    endpoint_urls = {
        "openai":    {"base_url": args.openai_base_url,
                      "api_key_env": ENDPOINT_DEFAULTS["openai"]["api_key_env"]},
        "mlx_local": {"base_url": args.mlx_local_url,
                      "api_key_env": None},
    }

    budget = BudgetTracker(
        cap_usd=args.budget_cap_usd,
        input_price_per_m=args.input_price_per_m,
        output_price_per_m=args.output_price_per_m,
    )
    synthesis_version = f"v1-{_commit_sha_short(args.project_root)}"

    print(f"Synthesis version: {synthesis_version}")
    print(f"Tasks:             {tasks}")
    print(f"Dry-run:           {args.dry_run}")
    print(f"Budget cap:        ${args.budget_cap_usd:.2f} (hard abort)")
    print("─" * 72)

    # Resolve debug log path.
    debug_log_path = args.debug_log
    if debug_log_path is None and not args.dry_run:
        # Default to {out_dir}/_debug.jsonl for multi-task runs; for single
        # --out mode place it next to the output file.
        if args.all or not args.out:
            debug_log_path = Path(args.out_dir) / "_debug.jsonl"
        else:
            debug_log_path = Path(args.out).with_suffix(".debug.jsonl")

    results: list[TaskResult] = []
    abort_reason: str | None = None
    for t in tasks:
        cfg = DEFAULT_TEACHER_CONFIG[t]
        out_path = _out_path_for(args, t)
        effective_count = args.count if args.count is not None else cfg.count
        print(f"• {t} → {out_path} (endpoint={cfg.endpoint}, count={effective_count}, "
              f"strict={args.strict}, timeout={args.http_timeout_s}s)")
        try:
            r = run_one_task(
                task_name=t, cfg=cfg,
                ults_dir=args.ults_dir,
                project_root=args.project_root,
                out_path=out_path,
                dataset_ver=args.dataset_ver,
                synthesis_version=synthesis_version,
                budget=budget,
                dry_run=args.dry_run,
                sleep_ms=args.sleep_ms,
                seed=args.seed,
                cuid_gen=cuid_gen,
                endpoint_urls=endpoint_urls,
                count_override=args.count,
                strict=args.strict,
                http_timeout_s=args.http_timeout_s,
                max_retries=args.max_retries,
                debug_log=debug_log_path,
                preflight=not args.skip_preflight,
            )
            results.append(r)
        except BudgetExceeded as e:
            sys.stderr.write(f"BUDGET CAP HIT: {e}\n")
            abort_reason = str(e)
            break
        except PreflightFailed as e:
            sys.stderr.write(f"PREFLIGHT: {e}\n")
            abort_reason = str(e)
            break
        except RuntimeError as e:
            # --strict aborts surface here; other RuntimeErrors are genuine bugs.
            if args.strict and "--strict:" in str(e):
                sys.stderr.write(f"STRICT ABORT: {e}\n")
                abort_reason = str(e)
                break
            raise

    _print_report(results, budget, args.dry_run, args.warn_rate)

    if args.report:
        args.report.parent.mkdir(parents=True, exist_ok=True)
        with open(args.report, "w") as f:
            json.dump({
                "synthesis_version": synthesis_version,
                "dry_run": args.dry_run,
                "budget": budget.snapshot(),
                "tasks": [dataclasses.asdict(r) for r in results],
                "abort_reason": abort_reason,
            }, f, indent=2)
        print(f"Report written to {args.report}")

    return 3 if abort_reason else 0


if __name__ == "__main__":
    raise SystemExit(main())
