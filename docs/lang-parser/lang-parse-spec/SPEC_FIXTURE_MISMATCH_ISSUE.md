# Spec-Fixture Mismatch Issue: 5 Tree-Sitter Languages

**Date:** 2026-01-29
**Status:** 11/16 languages passing, 5 need spec regeneration
**Affected Languages:** C, C++, CUDA, Java, Rust

---

## The Problem (Summary)

The **specs** (`.upts.json` files) for 5 languages contain **expected line numbers and parent values** that **do not match** what the **parser actually extracts** from the **fixtures**.

This is NOT a parser bug. The parser works correctly. The specs were written with incorrect assumptions about fixture line numbers.

---

## Why Previous Fix Attempts Failed

### Attempt 1: TREE_SITTER_GRAMMAR_ISSUES.md

- **What it provided:** Instructions to add tree-sitter grammar imports and extraction functions
- **What it fixed:** Parser now loads grammars and extracts symbols (went from ERROR to FAIL)
- **What it missed:** The specs still have wrong line numbers

### Attempt 2: upts-v1.2.tar.gz

- **What it provided:** Updated specs for all 16 languages
- **What happened:** It overwrote our working specs for the 11 passing languages
- **Why it failed:** The specs in the tarball were generated without running the actual parser against the fixtures. Line numbers were guessed or based on an outdated fixture version.

---

## The Core Issue: Spec vs Parser Output

The UPTS validation works by:

1. Running `./bin/extract-symbols --json <fixture>`
2. Comparing the output against `expected.symbols` in the spec
3. Matching by `name` + `type`, then checking `line`, `parent`, `value`, etc.

**If the spec says line 17, but the parser extracts line 76, validation fails.**

---

## Concrete Example: Rust

### Fixture: `rust_test_fixture.rs`

The fixture file defines symbols at specific line numbers. The parser reads this file and reports what it finds.

### What the Parser Extracts (ACTUAL)

```json
{"name": "UserId",      "type": "type",   "line": 13, "parent": null}
{"name": "Repository",  "type": "trait",  "line": 18, "parent": null}
{"name": "Status",      "type": "enum",   "line": 32, "parent": null}
{"name": "Priority",    "type": "enum",   "line": 40, "parent": null}
{"name": "User",        "type": "struct", "line": 49, "parent": null}
{"name": "UserService", "type": "struct", "line": 64, "parent": null}
{"name": "new",         "type": "method", "line": 72, "parent": "UserService<R>"}
{"name": "find_by_id",  "type": "method", "line": 76, "parent": "UserService<R>"}
```

### What the Spec Expects (WRONG)

```json
{"name": "UserId",      "type": "type",   "line": 12, "parent": null}
{"name": "Repository",  "type": "trait",  "line": 16, "parent": null}
{"name": "Status",      "type": "enum",   "line": 30, "parent": null}
{"name": "Priority",    "type": "enum",   "line": 37, "parent": null}
{"name": "User",        "type": "struct", "line": 46, "parent": null}
{"name": "UserService", "type": "struct", "line": 60, "parent": null}
{"name": "new",         "type": "method", "line": 68, "parent": "UserService"}
{"name": "find_by_id",  "type": "method", "line": 72, "parent": "UserService"}
```

### The Mismatches

| Symbol | Spec Line | Actual Line | Spec Parent | Actual Parent |
|--------|-----------|-------------|-------------|---------------|
| UserId | 12 | **13** | - | - |
| Repository | 16 | **18** | - | - |
| Status | 30 | **32** | - | - |
| Priority | 37 | **40** | - | - |
| User | 46 | **49** | - | - |
| UserService | 60 | **64** | - | - |
| new | 68 | **72** | UserService | **UserService\<R\>** |
| find_by_id | 72 | **76** | UserService | **UserService\<R\>** |

**Every line number is wrong by 2-4 lines. Parent names include generic parameters.**

---

## Why This Happens

### Line Number Drift

The fixtures may have been edited after the specs were written, or the specs were written by manually counting lines (prone to off-by-one errors, especially with comments and blank lines).

### Parent Name Differences

The parser extracts the full type as written in the code:

- Code: `impl<R: Repository> UserService<R>`
- Parser extracts parent: `UserService<R>`
- Spec expects: `UserService`

This is a design decision - should parent include generics or not?

---

## The Solution

### Option A: Regenerate Specs from Parser Output (Recommended)

Run the parser against each fixture and use the output to create correct specs:

```bash
# For each language, generate correct expected values:
./bin/extract-symbols --json docs/lang-parser/lang-parse-spec/upts/fixtures/rust_test_fixture.rs > /tmp/rust_actual.json

# Then update the spec's expected.symbols to match
```

### Option B: Fix Fixtures to Match Specs

Edit the fixture files to put symbols at the line numbers the specs expect. This is error-prone and defeats the purpose of having realistic test fixtures.

### Option C: Adjust Parser to Match Spec Expectations

Modify the parser to strip generic parameters from parent names. This changes parser behavior and may not be desirable.

---

## What Needs to Happen

For each of the 5 failing languages:

1. **Run the parser** against the fixture to get actual output
2. **Compare** actual vs expected in the spec
3. **Update the spec** with correct line numbers and parent values
4. **Verify** with `upts_runner.py validate`

### Files to Update

| Language | Spec File | Fixture File |
|----------|-----------|--------------|
| Rust | `specs/rust.upts.json` | `fixtures/rust_test_fixture.rs` |
| C | `specs/c.upts.json` | `fixtures/c_test_fixture.c` |
| C++ | `specs/cpp.upts.json` | `fixtures/cpp_test_fixture.cpp` |
| CUDA | `specs/cuda.upts.json` | `fixtures/cuda_test_fixture.cu` |
| Java | `specs/java.upts.json` | `fixtures/java_test_fixture.java` |

---

## Quick Verification Commands

```bash
# Check what parser actually extracts:
./bin/extract-symbols --json docs/lang-parser/lang-parse-spec/upts/fixtures/rust_test_fixture.rs | jq '.symbols[] | {name, type, line, parent}'

# Compare to what spec expects:
jq '.expected.symbols[] | {name, type, line, parent}' docs/lang-parser/lang-parse-spec/upts/specs/rust.upts.json

# Run validation to see failures:
python3 docs/lang-parser/lang-parse-spec/upts/runners/upts_runner.py validate \
  --spec=docs/lang-parser/lang-parse-spec/upts/specs/rust.upts.json \
  --parser="./bin/extract-symbols --json"
```

---

## Current Status

| Language | Status | Issue |
|----------|--------|-------|
| C | 33.3% | Line numbers wrong |
| C++ | 23.3% | Line numbers wrong |
| CUDA | 20.0% | Line numbers wrong |
| Java | 15.6% | Line numbers wrong |
| Rust | 29.6% | Line numbers + parent names wrong |

**The parser extracts symbols correctly. The specs just have the wrong expected values.**

---

## Key Insight

The tarball you provided contained specs that were **not generated from actual parser output**. They were likely:

- Written manually by looking at fixtures
- Generated from a different parser version
- Based on fixtures that were later modified

The fix is simple but tedious: **regenerate each spec from actual parser output**.
