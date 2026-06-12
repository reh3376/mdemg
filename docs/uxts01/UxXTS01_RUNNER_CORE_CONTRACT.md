# UxXTS01 Runner-Core Contract

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


Status: Active  
Date: 2026-02-28  
Purpose: Reduce runner duplication and preserve consistent behavior across framework runners.

---

## 1. Why This Exists

Framework runners were independently re-implementing the same core primitives:

1. deterministic hash helpers,
2. canonical report-status normalization,
3. common report conversion behaviors.

This created linear maintenance cost as new frameworks were added.

---

## 2. Shared Module

Runner-core extraction lives at:

- `docs/tests/uxts_runner_core.py`

Current exported primitives:

1. `canonical_json_bytes(obj)`
2. `sha256_hex_bytes(data)`
3. `sha256_hex_obj(obj)`
4. `sha256_file(path)`
5. `sha256_spec_without_field(spec, field_path)`
6. `canonical_result_status(status)`

---

## 3. Adoption Rule

Active UxXTS runners should import runner-core primitives for shared behavior rather than re-implementing these functions locally.

Initial adopters:

1. `docs/api/api-spec/uats/runners/uats_runner.py`
2. `docs/lang-parser/lang-parse-spec/upts/runners/upts_runner.py`

---

## 4. Compatibility Guarantees

1. Runner-core functions are additive and backward-compatible.
2. Runner-specific domain logic remains in each framework runner.
3. Shared primitives must not change status semantics (`pass|fail|skip|error`) without an explicit decision-record update.

---

## 5. Planned Next Extractions

1. canonical failure/warning shaping helpers,
2. reusable environment-token resolution helpers,
3. shared parity classification helpers.
