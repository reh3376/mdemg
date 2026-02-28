"""
Shared runner-core helpers for UxTS framework runners.

This module centralizes small, reusable primitives that were previously
duplicated in individual framework runners.
"""

from __future__ import annotations

import hashlib
import json
from pathlib import Path
from typing import Any, Mapping, Sequence


def canonical_json_bytes(obj: Any) -> bytes:
    """Serialize JSON data into deterministic bytes (sorted keys, compact form)."""
    return json.dumps(obj, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode("utf-8")


def sha256_hex_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def sha256_hex_obj(obj: Any) -> str:
    return sha256_hex_bytes(canonical_json_bytes(obj))


def sha256_file(path: Path, chunk_size: int = 8192) -> str:
    hasher = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(chunk_size), b""):
            hasher.update(chunk)
    return hasher.hexdigest()


def _drop_path(obj: Any, field_path: Sequence[str]) -> None:
    if not field_path or not isinstance(obj, dict):
        return

    current = obj
    for key in field_path[:-1]:
        if not isinstance(current, dict):
            return
        current = current.get(key)
        if current is None:
            return
    if isinstance(current, dict):
        current.pop(field_path[-1], None)


def sha256_spec_without_field(spec: Mapping[str, Any], field_path: Sequence[str]) -> str:
    """
    Compute deterministic SHA256 for spec content while excluding a nested field path.

    Example field_path values:
    - ("config", "sha256")
    - ("integrity", "spec_hash")
    """
    spec_copy = json.loads(json.dumps(spec))
    _drop_path(spec_copy, field_path)
    return sha256_hex_obj(spec_copy)


def canonical_result_status(status: Any) -> str:
    """
    Normalize runner status values into report-schema supported statuses.

    Report schema allows: pass|fail|skip|error.
    Legacy internal 'warn' is normalized to 'fail' to avoid invalid reports.
    """
    value = str(getattr(status, "value", status)).strip().lower()
    if value == "warn":
        return "fail"
    if value in {"pass", "fail", "skip", "error"}:
        return value
    return "error"
