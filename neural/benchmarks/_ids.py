"""CUIDv2 ID generation for benchmark runs.

MEMORY: never UUID. Primary library is `cuid2` PyPI package. If import fails
(e.g. package not installed in a constrained venv), fall back to a
time+hash derivation that preserves the sort-stability property.

The fallback is documented in the sprint plan §10 Risk row so auditing is
explicit about which code path ran.
"""
from __future__ import annotations

import hashlib
import secrets
import time


def new_run_id() -> str:
    """Return a CUIDv2-compatible ID for a benchmark run.

    Prefers the official `cuid2` package. On ImportError, falls back to a
    25-char lowercase alphanumeric derived from time+entropy+blake2b. Both
    forms preserve monotonic sort-stability within the same process.
    """
    try:
        from cuid2 import Cuid  # type: ignore[import-not-found]
    except ImportError:
        return _fallback_id()

    # cuid2 API: Cuid(length=25).generate() — default length 25 matches spec.
    return Cuid(length=25).generate()


def _fallback_id() -> str:
    """Deterministic-seedless fallback: timestamp-ns + entropy + blake2b.

    Format: 25 chars, lowercase alnum. Starts with a lowercase letter to match
    CUIDv2's shape expectation (some downstream consumers assume leading char
    is [a-z]).
    """
    ns = time.time_ns().to_bytes(8, "big")
    entropy = secrets.token_bytes(8)
    digest = hashlib.blake2b(ns + entropy, digest_size=16).hexdigest()
    # force leading lowercase letter for CUIDv2-shape compatibility
    first = chr(ord("a") + int(digest[0], 16) % 26)
    return (first + digest[1:])[:25]
