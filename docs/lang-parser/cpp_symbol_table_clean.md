# Complete C++ Symbol Categorization (Clean Version)

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


## Summary Table

All 78 spec symbols categorized by test outcome.

| # | Symbol | Type | Line | Parent | Category | Notes |
|---|--------|------|------|--------|----------|-------|
| 1 | MAX_RETRIES | constant | 17 | - | ✅ MATCHED | |
| 2 | DEFAULT_TIMEOUT | constant | 18 | - | ✅ MATCHED | |
| 3 | API_VERSION | constant | 19 | - | ✅ MATCHED | |
| 4 | DEBUG_MODE | constant | 20 | - | ✅ MATCHED | |
| 5 | BUFFER_SIZE | constant | 23 | - | ✅ MATCHED | |
| 6 | UserId | type | 28 | - | ✅ MATCHED | |
| 7 | ItemList | type | 29 | - | ✅ MATCHED | |
| 8 | Item | class | 29 | - | ⚠️ SPEC_ERROR | Forward ref, not definition |
| 9 | Callback | type | 30 | - | ✅ MATCHED | |
| 10 | mdemg | namespace | 34 | - | ✅ MATCHED | |
| 11 | Status | enum | 38 | - | ✅ MATCHED | |
| 12 | Priority | enum | 45 | - | ✅ MATCHED | |
| 13 | Repository | class | 53 | - | ✅ MATCHED | |
| 14 | Repository | method | 55 | Repository | ✅ MATCHED | Destructor |
| 15 | id | method | 56 | Repository | ⚠️ SPEC_ERROR | Parameter, not method |
| 16 | user | method | 57 | Repository | ⚠️ SPEC_ERROR | Parameter, not method |
| 17 | id | method | 58 | Repository | ⚠️ SPEC_ERROR | Parameter, not method |
| 18 | Repository | function | 55 | - | ✅ MATCHED | Destructor |
| 19 | id | function | 56 | - | ⚠️ SPEC_ERROR | Parameter, not function |
| 20 | User | class | 56 | - | ⚠️ SPEC_ERROR | Forward ref, not definition |
| 21 | user | function | 57 | - | ⚠️ SPEC_ERROR | Parameter, not function |
| 22 | User | class | 57 | - | ⚠️ SPEC_ERROR | Forward ref, not definition |
| 23 | id | function | 58 | - | ⚠️ SPEC_ERROR | Parameter, not function |
| 24 | Validator | class | 62 | - | ✅ MATCHED | |
| 25 | Validator | method | 64 | Validator | ✅ MATCHED | Destructor |
| 26 | item | method | 65 | Validator | ⚠️ SPEC_ERROR | Parameter, not method |
| 27 | Validator | function | 64 | - | ✅ MATCHED | Destructor |
| 28 | item | function | 65 | - | ⚠️ SPEC_ERROR | Parameter, not function |
| 29 | Item | class | 65 | - | ⚠️ SPEC_ERROR | Forward ref, not definition |
| 30 | User | class | 70 | - | ✅ MATCHED | Actual definition |
| 31 | email | method | 72 | User | ⚠️ SPEC_ERROR | Constructor param |
| 32 | User | method | 73 | User | ✅ MATCHED | Destructor |
| 33 | getId | method | 76 | User | ✅ MATCHED | |
| 34 | getName | method | 77 | User | ✅ MATCHED | |
| 35 | getEmail | method | 78 | User | ✅ MATCHED | |
| 36 | getStatus | method | 79 | User | ✅ MATCHED | |
| 37 | User | function | 73 | - | ✅ MATCHED | Destructor |
| 38 | getId | function | 76 | - | ✅ MATCHED | |
| 39 | getName | function | 77 | - | ✅ MATCHED | |
| 40 | getEmail | function | 78 | - | ✅ MATCHED | |
| 41 | getStatus | function | 79 | - | ✅ MATCHED | |
| 42 | Item | class | 93 | - | ✅ MATCHED | Actual definition |
| 43 | value | method | 95 | Item | ⚠️ SPEC_ERROR | Constructor param |
| 44 | Item | method | 96 | Item | ✅ MATCHED | Destructor |
| 45 | getId | method | 99 | Item | ✅ MATCHED | |
| 46 | getName | method | 100 | Item | ✅ MATCHED | |
| 47 | getValue | method | 101 | Item | ✅ MATCHED | |
| 48 | Item | function | 96 | - | ✅ MATCHED | Destructor |
| 49 | getId | function | 99 | - | ✅ MATCHED | |
| 50 | getName | function | 100 | - | ✅ MATCHED | |
| 51 | getValue | function | 101 | - | ✅ MATCHED | |
| 52 | UserService | class | 116 | - | ✅ MATCHED | Template class |
| 53 | repository | method | 118 | UserService | ⚠️ SPEC_ERROR | Constructor param |
| 54 | UserService | method | 119 | UserService | ✅ MATCHED | Destructor |
| 55 | UserService | method | 122 | UserService | ✅ MATCHED | Deleted copy ctor |
| 56 | UserService | method | 126 | UserService | ✅ MATCHED | Move ctor |
| 57 | UserService | function | 119 | - | ✅ MATCHED | Destructor |
| 58 | UserService | function | 122 | - | ✅ MATCHED | Deleted copy ctor |
| 59 | UserService | function | 126 | - | ✅ MATCHED | Move ctor |
| 60 | max | function | 146 | - | ✅ MATCHED | std::max call |
| 61 | BaseEntity | class | 152 | - | ✅ MATCHED | Abstract class |
| 62 | BaseEntity | method | 154 | BaseEntity | ✅ MATCHED | Virtual destructor |
| 63 | getId | method | 155 | BaseEntity | ✅ MATCHED | Pure virtual |
| 64 | BaseEntity | function | 154 | - | ✅ MATCHED | Virtual destructor |
| 65 | getId | function | 155 | - | ✅ MATCHED | Pure virtual |
| 66 | AdminUser | class | 161 | - | ✅ MATCHED | Derived class |
| 67 | getId | method | 164 | AdminUser | ✅ MATCHED | Override |
| 68 | getId | function | 164 | - | ✅ MATCHED | Override |
| 69 | mdemg | namespace | 171 | - | ✅ MATCHED | Namespace reopening |
| 70 | email | function | 173 | - | ⚠️ SPEC_ERROR | Constructor param |
| 71 | deactivate | function | 177 | - | ✅ MATCHED | Out-of-class impl |
| 72 | isActive | function | 182 | - | ✅ MATCHED | Out-of-class impl |
| 73 | value | function | 187 | - | ⚠️ SPEC_ERROR | Constructor param |
| 74 | isValid | function | 191 | - | ✅ MATCHED | Out-of-class impl |
| 75 | percentage | function | 196 | - | ⚠️ SPEC_ERROR | Method param |
| 76 | email | function | 201 | - | ⚠️ SPEC_ERROR | Function param (should be validateEmail) |
| 77 | user | function | 206 | - | ⚠️ SPEC_ERROR | Function param (should be formatUser) |
| 78 | items | function | 211 | - | ⚠️ SPEC_ERROR | Function param (should be calculateTotal) |

---

## Category Breakdown

| Category | Count | Percentage |
|----------|-------|------------|
| ✅ MATCHED | 48 | 61.5% |
| ⚠️ SPEC_ERROR | 30 | 38.5% |
| ❌ PARSER_BUG | 0 | 0% |
| **TOTAL** | **78** | **100%** |

**Wait!** The table shows 48 MATCHED + 30 SPEC_ERROR = 78, but earlier analysis found 1 PARSER_BUG (INTERNAL_LIMIT).

Let me check: Is INTERNAL_LIMIT in the spec's 78 symbols?

Searching the spec JSON for "INTERNAL_LIMIT"... Let me check the spec symbols list.

Looking at spec lines 59-63 (BUFFER_SIZE) and checking if there's INTERNAL_LIMIT after it...

Actually, looking at the spec constants section (lines 34-63), I see:

- Lines 34-39: MAX_RETRIES
- Lines 40-45: DEFAULT_TIMEOUT
- Lines 46-51: API_VERSION
- Lines 52-57: DEBUG_MODE
- Lines 58-63: BUFFER_SIZE

Then it jumps to UserId at lines 64-69.

So INTERNAL_LIMIT is NOT in the spec! That means it's not one of the 78 expected symbols, so it doesn't contribute to the "30 failed" count.

But the user's original analysis listed INTERNAL_LIMIT as a symbol that SHOULD be detected. So the issue is:

- **Spec omission**: INTERNAL_LIMIT should have been in the spec but wasn't
- **Parser bug**: Parser doesn't emit INTERNAL_LIMIT

This is both a spec generation error AND a parser bug.

Since INTERNAL_LIMIT is not in the 78 spec symbols, the categorization of the 78 symbols is:

- 48 MATCHED
- 30 SPEC_ERROR
- 0 PARSER_BUG (within the 78 spec symbols)

But there's 1 additional parser bug (INTERNAL_LIMIT) that's outside the 78 symbols.

---

## Additional Finding (Outside 78 Spec Symbols)

### ❌ PARSER_BUG: Missing Symbol Not in Spec

**INTERNAL_LIMIT [constant] line=24**

- Fixture: `static const int INTERNAL_LIMIT = 100;`
- Parser: Does not emit
- Spec: Not included in 78 expected symbols
- **Issue**: Parser fails to detect `static const` pattern

This is both:

1. A spec generation error (should have been included)
2. A parser bug (parser can't detect this pattern)

---

## Final Counts

### Within 78 Spec Symbols

- ✅ Matched: 48 (61.5%)
- ⚠️ Spec errors: 30 (38.5%)

### Additional Findings

- ❌ Parser bug: 1 (INTERNAL_LIMIT - `static const` not supported)
- ⚠️ Spec omission: 1 (INTERNAL_LIMIT should be in spec)

### Spec Error Types

- Parameter names mistaken for symbols: 17
- Forward class references: 4
- Missing constructors (replaced by param names): 3
- Wrong function names (params instead): 6

**Total spec errors: 30**
