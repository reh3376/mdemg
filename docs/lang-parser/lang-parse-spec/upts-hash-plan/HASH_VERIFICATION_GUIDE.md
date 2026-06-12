# UPTS v1.1: SHA256 Fixture Hash Verification

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Date:** 2026-01-29  
**Version:** 1.1.0  
**Feature:** Fixture integrity verification via SHA256 hashes

---

## Overview

This update adds SHA256 hash verification to UPTS specs. When a fixture file is modified, validation fails immediately with a clear error message — no more debugging mysterious line number mismatches.

### How It Works

```
┌─────────────────┐      ┌──────────────────┐
│  Spec File      │      │  Fixture File    │
│  rust.upts.json │      │  rust_fixture.rs │
│                 │      │                  │
│  fixture:       │      │  // source code  │
│    sha256: abc  │──────│  fn main() {}    │
│    path: ...    │      │                  │
└─────────────────┘      └──────────────────┘

On validation:
1. Compute SHA256 of fixture file
2. Compare to expected hash in spec
3. FAIL if mismatch (fixture was modified)
4. Continue validation if match
```

---

## Quick Start

### 1. Replace Runner

```bash
# Backup existing
cp upts/runners/upts_runner.py upts/runners/upts_runner.py.bak

# Install v1.1
cp upts_runner_v1.1.py upts/runners/upts_runner.py
```

### 2. Add Hashes to Existing Specs

```bash
python3 upts/runners/upts_runner.py add-hashes --spec-dir upts/specs/
```

Output:

```
Adding SHA256 hashes to specs in upts/specs/
  ✓ c.upts.json: Hash added
  ✓ cpp.upts.json: Hash added
  ✓ cuda.upts.json: Hash added
  ...

Updated: 16, Errors: 0
```

### 3. Verify Hashes (Without Full Validation)

```bash
python3 upts/runners/upts_runner.py verify-hashes --spec-dir upts/specs/
```

Output:

```
  ✓ c.upts.json: Hash verified
  ✓ cpp.upts.json: Hash verified
  ...

Verified: 16, Missing: 0, Mismatched: 0
```

### 4. Run Validation (Hash Checked Automatically)

```bash
python3 upts/runners/upts_runner.py validate-all --spec-dir upts/specs/ --parser "./bin/extract-symbols --json"
```

---

## New CLI Commands

### `add-hashes` — Add SHA256 to All Specs

```bash
python3 upts_runner.py add-hashes --spec-dir specs/
python3 upts_runner.py add-hashes --spec-dir specs/ --pattern "rust*.json"
```

### `verify-hashes` — Quick Hash Check

```bash
python3 upts_runner.py verify-hashes --spec-dir specs/
```

No parser needed — just checks file hashes.

### `validate` with Hash Options

```bash
# Normal (hash verified if present)
python3 upts_runner.py validate --spec rust.upts.json --parser "./parse"

# Skip hash check
python3 upts_runner.py validate --spec rust.upts.json --parser "./parse" --skip-hash
```

---

## Spec Format Change

### Before (v1.0)

```json
{
  "fixture": {
    "type": "file",
    "path": "../fixtures/rust_test_fixture.rs"
  }
}
```

### After (v1.1)

```json
{
  "fixture": {
    "type": "file",
    "path": "../fixtures/rust_test_fixture.rs",
    "sha256": "a3f2b8c9d4e5f6a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5"
  }
}
```

The `sha256` field is **optional** for backward compatibility. If not present, hash verification is skipped.

---

## Validation Output

### Hash Verified

```
============================================================
RUST Parser Test
Spec: specs/rust.upts.json
Status: ✓ PASS
Matched: 32/32 (100.0%)
Fixture Hash: ✓ Verified
Duration: 45.2ms
```

### Hash Mismatch

```
============================================================
RUST Parser Test
Spec: specs/rust.upts.json
Status: ✗ ERROR
Matched: 0/32 (0.0%)
Error: HASH MISMATCH: Fixture has been modified since spec generation
  Expected: a3f2b8c9d4e5f6a1...
  Actual:   b7c8d9e0f1a2b3c4...
  File:     ../fixtures/rust_test_fixture.rs
  Action:   Regenerate spec from parser output, or run with --skip-hash
```

### Hash Not Specified

```
Fixture Hash: ○ Not specified
```

### Hash Skipped

```
Fixture Hash: ⊘ Skipped
```

---

## Generating New Specs with Hashes

Use the updated `generate_spec_from_output.py`:

```bash
# Capture parser output
./bin/extract-symbols --json fixtures/rust_test_fixture.rs > /tmp/rust.json

# Generate spec with hash
python3 generate_spec_from_output_v1.1.py rust /tmp/rust.json "../fixtures/rust_test_fixture.rs" \
    --fixture-root ./specs/ \
    --output specs/rust.upts.json
```

The script will:

1. Read parser output
2. Build expected symbols
3. Compute SHA256 of fixture
4. Write complete spec with hash

---

## Workflow Integration

### CI/CD Pipeline

```yaml
# .github/workflows/parser-tests.yml
jobs:
  test:
    steps:
      - name: Verify fixture integrity
        run: python3 upts_runner.py verify-hashes --spec-dir specs/
        
      - name: Run parser tests
        run: python3 upts_runner.py validate-all --spec-dir specs/ --parser "./bin/extract-symbols --json"
```

### Makefile

```makefile
.PHONY: test-parsers verify-fixtures add-hashes

test-parsers:
 python3 upts/runners/upts_runner.py validate-all \
  --spec-dir upts/specs/ \
  --parser "./bin/extract-symbols --json"

verify-fixtures:
 python3 upts/runners/upts_runner.py verify-hashes --spec-dir upts/specs/

add-hashes:
 python3 upts/runners/upts_runner.py add-hashes --spec-dir upts/specs/

# Regenerate spec after fixture change
regen-spec-%:
 ./bin/extract-symbols --json upts/fixtures/$*_test_fixture.* > /tmp/$*.json
 python3 generate_spec_from_output_v1.1.py $* /tmp/$*.json \
  "../fixtures/$*_test_fixture.*" \
  --fixture-root upts/specs/ \
  --output upts/specs/$*.upts.json
```

---

## Files Included

| File | Description |
|------|-------------|
| `upts_runner_v1.1.py` | Updated runner with hash verification |
| `generate_spec_from_output_v1.1.py` | Spec generator with hash computation |
| `HASH_VERIFICATION_GUIDE.md` | This documentation |

---

## Migration Checklist

- [ ] Backup existing `upts_runner.py`
- [ ] Copy `upts_runner_v1.1.py` to `upts/runners/upts_runner.py`
- [ ] Copy `generate_spec_from_output_v1.1.py` to project
- [ ] Run `add-hashes` to update all specs
- [ ] Run `verify-hashes` to confirm
- [ ] Run full `validate-all` to ensure everything still passes
- [ ] Commit updated specs with hashes
- [ ] Update CI/CD if needed

---

## FAQ

### Q: What if I modify a fixture intentionally?

Regenerate the spec:

```bash
./bin/extract-symbols --json fixtures/rust_test_fixture.rs > /tmp/rust.json
python3 generate_spec_from_output_v1.1.py rust /tmp/rust.json "../fixtures/rust_test_fixture.rs" \
    --fixture-root ./specs/ --output specs/rust.upts.json
```

### Q: Can I disable hash checking temporarily?

Yes, use `--skip-hash`:

```bash
python3 upts_runner.py validate --spec rust.upts.json --parser "./parse" --skip-hash
```

### Q: Are existing specs without hashes still valid?

Yes. Hash verification only runs if `sha256` is present in the spec. Old specs work unchanged.

### Q: What hash algorithm is used?

SHA256 — same as Git, widely supported, collision-resistant.

### Q: Does this slow down validation?

Negligibly. SHA256 of a typical fixture (<10KB) takes <1ms.

---

## Summary

| Feature | v1.0 | v1.1 |
|---------|------|------|
| Symbol validation | ✓ | ✓ |
| Line tolerance | ✓ | ✓ |
| Type compatibility | ✓ | ✓ |
| Fixture hash verification | ✗ | ✓ |
| `add-hashes` command | ✗ | ✓ |
| `verify-hashes` command | ✗ | ✓ |
| `--skip-hash` flag | ✗ | ✓ |

**Result:** If fixture changes, you know immediately. No more debugging "why did line numbers shift?"
