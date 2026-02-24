# UPTS v1.1 Hash Verification: Implementation Assumptions

**Date:** 2026-01-29  
**Status:** Awaiting confirmation before implementation

---

## File Location Assumptions

| # | Assumption | Example Value |
|---|------------|---------------|
| 1 | Runner lives at `upts/runners/upts_runner.py` | — |
| 2 | Specs are in `upts/specs/*.upts.json` | `upts/specs/rust.upts.json` |
| 3 | Fixtures are in `upts/fixtures/` | `upts/fixtures/rust_test_fixture.rs` |
| 4 | Spec-to-fixture relative path is `../fixtures/` | `"path": "../fixtures/rust_test_fixture.rs"` |

**Confirm:** Are these paths correct for your project structure?

---

## Behavior Assumptions

| # | Assumption | Implementation |
|---|------------|----------------|
| 5 | Hash verification is **optional** | Specs without `sha256` field skip verification (backward compatible with v1.0 specs) |
| 6 | Hash mismatch = **hard ERROR** | Stops validation immediately, does NOT continue to symbol checking |
| 7 | `--skip-hash` flag available | User can bypass hash check when needed (e.g., during development) |
| 8 | `add-hashes` modifies specs **in place** | Overwrites existing spec files with updated JSON |
| 9 | Hash computed at **spec generation time** | Stored in spec, not computed dynamically on every validation |

**Confirm:**

- Should mismatch be ERROR (stop) or WARNING (continue)?
- Should `add-hashes` modify in place, or create new files?

---

## Technical Assumptions

| # | Assumption | Choice Made |
|---|------------|-------------|
| 10 | Hash algorithm | SHA256 |
| 11 | File read mode | Binary (`rb`) for consistent hashing across platforms |
| 12 | Chunk size for hashing | 8192 bytes |
| 13 | Path resolution logic | Fixture path in spec is resolved relative to spec file's parent directory |
| 14 | Hash format in spec | Full 64-character hex string (not truncated) |

**Confirm:** Is SHA256 acceptable, or prefer different algorithm?

---

## CLI Assumptions

| # | Assumption | Implementation |
|---|------------|----------------|
| 15 | Existing commands unchanged | `validate`, `validate-all`, `convert` keep same syntax |
| 16 | New command: `add-hashes` | Bulk add/update hashes in all specs in a directory |
| 17 | New command: `verify-hashes` | Quick integrity check without running parser |
| 18 | New flag: `--skip-hash` | Added to `validate` and `validate-all` commands |

**Confirm:** Are new commands/flags acceptable?

---

## Workflow Assumptions

| # | Assumption | Implementation |
|---|------------|----------------|
| 19 | User wants bulk hash updates | `add-hashes --spec-dir` processes all `*.upts.json` files |
| 20 | User wants quick integrity check | `verify-hashes` runs without needing parser binary |
| 21 | CI/CD should fail on hash mismatch | Exit code 1 on any mismatch |
| 22 | Hash shown in validation output | Displays "✓ Verified", "⊘ Skipped", or "○ Not specified" |

**Confirm:** Correct CI/CD behavior?

---

## Schema Assumption

| # | Assumption | Status |
|---|------------|--------|
| 23 | `sha256` field already exists in schema under `fixture` | ✅ Confirmed (checked your schema) |

---

## Questions Requiring Your Input

### Q1: Hard ERROR vs WARNING on mismatch?

**Option A (current implementation):**

```
Status: ✗ ERROR
Error: HASH MISMATCH: Fixture has been modified
       → Validation stops, no symbol checking
```

**Option B (alternative):**

```
Status: ⚠ WARNING  
Warning: HASH MISMATCH: Fixture has been modified
         → Continues with symbol checking anyway
```

**Your preference:** A or B?

---

### Q2: In-place modification for `add-hashes`?

**Option A (current implementation):**

```bash
python3 upts_runner.py add-hashes --spec-dir specs/
# Modifies specs/rust.upts.json directly
```

**Option B (alternative):**

```bash
python3 upts_runner.py add-hashes --spec-dir specs/ --output-dir specs-updated/
# Writes to new directory, preserves originals
```

**Your preference:** A or B?

---

### Q3: Default hash behavior?

**Option A (current implementation):**

- If spec has `sha256`: verify it
- If spec lacks `sha256`: skip verification silently

**Option B (stricter):**

- If spec has `sha256`: verify it
- If spec lacks `sha256`: show warning "No hash specified"

**Option C (strictest):**

- Require `--skip-hash` flag to run without hashes
- Fail if spec lacks `sha256` and flag not provided

**Your preference:** A, B, or C?

---

### Q4: Anything else?

Are there any other assumptions I should document or behaviors you want changed?

---

## Summary Table

| Category | Count | Status |
|----------|-------|--------|
| File locations | 4 | ❓ Awaiting confirmation |
| Behaviors | 5 | ❓ Awaiting confirmation |
| Technical | 5 | ❓ Awaiting confirmation |
| CLI | 4 | ❓ Awaiting confirmation |
| Workflow | 4 | ❓ Awaiting confirmation |
| Schema | 1 | ✅ Confirmed |
| **Total** | **23** | — |

---

Please review and confirm or correct each assumption. I'll adjust the implementation based on your feedback before you deploy.
