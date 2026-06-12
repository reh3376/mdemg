# C++ Parser Analysis - Executive Summary

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


## Test Results

**Overall**: 48/78 matched (61.5%), 30/78 failed (38.5%)

- **Parser emitted**: 105 symbols (27 extra symbols due to dual function/method detection)
- **Spec expected**: 78 symbols
- **Matched**: 48 symbols
- **Failed**: 30 symbols

---

## Root Cause Analysis

### 96.7% of Failures Are Spec Errors

Of the 30 failures, **29 are spec generation errors** (96.7%), not parser bugs:

| Error Type | Count | Examples |
|------------|-------|----------|
| Parameters mistaken for methods | 7 | `id [method] line=56` from `findById(const UserId& id)` |
| Parameters mistaken for functions | 17 | `email [function] line=173` from constructor parameter |
| Forward class references | 4 | `Item [class] line=29` from `std::vector<class Item>` |
| Missing constructors | 3 | Spec has parameter name instead of constructor name |

### 1 Genuine Parser Bug

**INTERNAL_LIMIT [constant] line=24** is not detected by the parser.

```cpp
static const int INTERNAL_LIMIT = 100;  // ❌ NOT DETECTED
```

**Issue**: Parser only recognizes `constexpr` and `inline constexpr`, not `static const`.

---

## What the Parser Does Well

The parser correctly detects:

✅ **Constants**: `constexpr` and `inline constexpr` patterns
✅ **Type aliases**: `using` declarations
✅ **Classes**: All class declarations including template classes
✅ **Methods**: Inline methods, virtual methods, pure virtual, override
✅ **Functions**: Both declarations and out-of-class implementations
✅ **Constructors & Destructors**: Default, deleted, user-defined
✅ **Enums**: `enum class` declarations
✅ **Namespaces**: Including reopened namespaces
✅ **Inheritance**: Public, protected, private; multiple inheritance
✅ **Templates**: Template classes and functions

---

## Detailed Breakdown of 30 Failures

### Parameters Mistaken for Symbols (24 total)

The spec generator incorrectly extracted parameter names as separate symbols:

#### As Methods (7)

1. `id` from `findById(const UserId& id)` - line 56
2. `user` from `save(const class User& user)` - line 57
3. `id` from `remove(const UserId& id)` - line 58
4. `item` from `validate(const class Item& item)` - line 65
5. `email` from `User(UserId id, std::string name, std::string email)` - line 72
6. `value` from `Item(std::string id, std::string name, int value)` - line 95
7. `repository` from `UserService(std::shared_ptr<R> repository)` - line 118

#### As Functions (17)

8-11. Same as #1-4 above, but as function type
12. `email` from `User::User(..., std::string email)` - line 173
13. `value` from `Item::Item(..., int value)` - line 187
14. `percentage` from `calculateDiscount(double percentage)` - line 196
15-17. **Critical**: Wrong function names at lines 201, 206, 211:
    - Spec expects `email`, parser correctly emits `validateEmail`
    - Spec expects `user`, parser correctly emits `formatUser`
    - Spec expects `items`, parser correctly emits `calculateTotal`

### Forward Class References (4 total)

The spec incorrectly treats forward-declared classes as definitions:

1. `Item [class] line=29` - from `using ItemList = std::vector<class Item>;`
   - **Actual definition**: line 93
2. `User [class] line=56` - from return type `std::optional<class User>`
   - **Actual definition**: line 70
3. `User [class] line=57` - from parameter type `const class User&`
   - **Actual definition**: line 70
4. `Item [class] line=65` - from parameter type `const class Item&`
   - **Actual definition**: line 93

### Missing Constructor Declarations (3 total)

The spec replaced constructor names with parameter names:

1. **Missing** `User [method] line=72` - spec has `email` instead
2. **Missing** `Item [method] line=95` - spec has `value` instead
3. **Missing** `UserService [method] line=118` - spec has `repository` instead

---

## Recommendations

### Priority 1: Fix Parser Bug (MUST FIX)

**Add support for `static const` constant pattern**

```cpp
// Currently NOT detected
static const int INTERNAL_LIMIT = 100;
static const char* API_KEY = "key";

// Currently detected ✓
constexpr int MAX_RETRIES = 3;
inline constexpr size_t BUFFER_SIZE = 1024;
```

**Implementation**: Update constant detection in C++ parser to handle `static const` declarations.

### Priority 2: Regenerate Spec (IMMEDIATE)

**Fix spec generator to exclude:**

1. Parameter names (check for parameter context before extracting)
2. Forward class references (only include actual `class ClassName {` definitions)
3. Validate constructor/destructor names (should match class name, not parameter)

**Validation checklist for new spec:**

- [ ] No parameter names in symbols list
- [ ] No forward `class` references
- [ ] All constructors have correct names (class name, not param name)
- [ ] Function names are function names, not parameter names
- [ ] All `static const` constants included (after parser fix)

### Priority 3: Add Regression Tests

1. **Constant patterns test**: Ensure `static const`, `constexpr`, and `inline constexpr` all work
2. **Parameter filtering test**: Verify spec generator doesn't extract parameters
3. **Forward reference test**: Ensure only actual class definitions are included
4. **Constructor naming test**: Validate constructors have class name, not param name

---

## Expected Outcome After Fixes

| Metric | Before | After |
|--------|--------|-------|
| Matched | 48/78 (61.5%) | 78/78 (100%) |
| Failed | 30/78 (38.5%) | 0/78 (0%) |
| Spec errors | 29 | 0 |
| Parser bugs | 1 | 0 |

---

## Files Reference

- **Spec**: `/Users/reh3376/mdemg/docs/lang-parser/lang-parse-spec/upts/specs/cpp.upts.json`
- **Fixture**: `/Users/reh3376/mdemg/docs/lang-parser/lang-parse-spec/upts/fixtures/cpp_test_fixture.cpp`
- **Analysis docs**:
  - `/Users/reh3376/mdemg/docs/lang-parser/cpp_final_summary.md`
  - `/Users/reh3376/mdemg/docs/lang-parser/cpp_symbol_table_clean.md`
  - `/Users/reh3376/mdemg/docs/lang-parser/cpp_failure_categorization.md`

---

## Next Steps

1. **Fix parser**: Add `static const` support (est. 30 min)
2. **Fix spec generator**: Filter parameters and forward refs (est. 1 hour)
3. **Regenerate spec**: Run corrected generator on C++ fixture (est. 5 min)
4. **Re-run tests**: Expect 100% pass rate (est. 2 min)
5. **Commit changes**: Update parser, spec, and tests (est. 10 min)

**Total estimated time**: ~2 hours
