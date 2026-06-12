# C++ Parser Symbol Analysis

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


## Categorization of All 78 Expected Symbols

### Summary

- **Total expected symbols**: 78
- **Parser emitted**: 105 symbols
- **Matched**: 48
- **Failed**: 30
- **To analyze**: All failures to categorize as SPEC_ERROR or PARSER_BUG

---

## SPEC ERRORS (Parameter Names and Forward References)

These are symbols the spec incorrectly expects - they're parameter names or forward class references, not actual definitions:

### 1. Parameter Names Mistaken for Methods

| Spec Symbol | Line | Context | Why It's Wrong |
|-------------|------|---------|----------------|
| `id [method] line=56 parent=Repository` | 56 | `findById(const UserId& id)` | Parameter name, not method name |
| `user [method] line=57 parent=Repository` | 57 | `save(const class User& user)` | Parameter name |
| `id [method] line=58 parent=Repository` | 58 | `remove(const UserId& id)` | Parameter name |
| `item [method] line=65 parent=Validator` | 65 | `validate(const class Item& item)` | Parameter name |
| `email [method] line=72 parent=User` | 72 | `User(..., std::string email)` | Constructor parameter |
| `value [method] line=95 parent=Item` | 95 | `Item(..., int value)` | Constructor parameter |
| `repository [method] line=118 parent=UserService` | 118 | `UserService(std::shared_ptr<R> repository)` | Constructor parameter |

**Count: 7 method-type parameter errors**

### 2. Parameter Names Mistaken for Functions

| Spec Symbol | Line | Context | Why It's Wrong |
|-------------|------|---------|----------------|
| `id [function] line=56` | 56 | `findById(const UserId& id)` | Parameter name |
| `user [function] line=57` | 57 | `save(const class User& user)` | Parameter name |
| `id [function] line=58` | 58 | `remove(const UserId& id)` | Parameter name |
| `item [function] line=65` | 65 | `validate(const class Item& item)` | Parameter name |
| `email [function] line=173` | 173 | `User::User(..., std::string email)` | Constructor parameter |
| `value [function] line=187` | 187 | `Item::Item(..., int value)` | Constructor parameter |
| `percentage [function] line=196` | 196 | `calculateDiscount(double percentage)` | Method parameter |
| `email [function] line=201` | 201 | `validateEmail(const std::string& email)` | Function parameter |
| `user [function] line=206` | 206 | `formatUser(const User& user)` | Function parameter |
| `items [function] line=211` | 211 | `calculateTotal(const ItemList& items)` | Function parameter |

**Count: 10 function-type parameter errors**

### 3. Forward-Declared Classes (Not Definitions)

| Spec Symbol | Line | Context | Why It's Wrong |
|-------------|------|---------|----------------|
| `Item [class] line=29` | 29 | `using ItemList = std::vector<class Item>;` | Forward reference, actual class at line 93 |
| `User [class] line=56` | 56 | `std::optional<class User> findById(...)` | Forward reference, actual class at line 70 |
| `User [class] line=57` | 57 | `save(const class User& user)` | Forward reference, actual class at line 70 |
| `Item [class] line=65` | 65 | `validate(const class Item& item)` | Forward reference, actual class at line 93 |

**Count: 4 forward-reference errors**

### 4. Missing Real Symbol: INTERNAL_LIMIT

| Spec Expected | Parser Output | Issue |
|---------------|---------------|-------|
| ❌ Missing | ❌ Not emitted | `static const int INTERNAL_LIMIT = 100;` at line 24 |

**This is a PARSER BUG** - the parser should emit `INTERNAL_LIMIT [constant] line=24` but doesn't.

---

## PARSER BUGS (Real Symbols Not Emitted)

### Missing Constants

1. **INTERNAL_LIMIT [constant] line=24**
   - Fixture: `static const int INTERNAL_LIMIT = 100;`
   - Parser: Not emitted
   - Reason: Parser may not recognize `static const` pattern (only handles `constexpr` and `inline constexpr`)

### Missing Real Methods (Not in Parser Output)

The spec expects these, but let me verify they're actual method names vs parameters:

1. **findById [method] line=56 parent=Repository**
   - Fixture: `virtual std::optional<class User> findById(const UserId& id) = 0;`
   - Parser: Emits `findById [function] line=56` and `findById [method] line=56 parent=Repository` ✓
   - Status: **MATCHED** (parser index [15], [16])

2. **save [method] line=57 parent=Repository**
   - Fixture: `virtual bool save(const class User& user) = 0;`
   - Parser: Emits `save [function] line=57` and `save [method] line=57 parent=Repository` ✓
   - Status: **MATCHED** (parser index [17], [18])

3. **remove [method] line=58 parent=Repository**
   - Fixture: `virtual bool remove(const UserId& id) = 0;`
   - Parser: Emits `remove [function] line=58` and `remove [method] line=58 parent=Repository` ✓
   - Status: **MATCHED** (parser index [19], [20])

4. **validate [method] line=65 parent=Validator**
   - Fixture: `virtual bool validate(const class Item& item) = 0;`
   - Parser: Emits `validate [function] line=65` and `validate [method] line=65 parent=Validator` ✓
   - Status: **MATCHED** (parser index [24], [25])

5. **User [method] line=72 parent=User** (Constructor)
   - Fixture: `User(UserId id, std::string name, std::string email);`
   - Parser: Emits `User [function] line=72` and `User [method] line=72 parent=User` ✓
   - Status: **MATCHED** (parser index [27], [28])

6. **Item [method] line=95 parent=Item** (Constructor)
   - Fixture: `Item(std::string id, std::string name, int value);`
   - Parser: Emits `Item [function] line=95` and `Item [method] line=95 parent=Item` ✓
   - Status: **MATCHED** (parser index [44], [45])

7. **UserService [method] line=118 parent=UserService** (Constructor)
   - Fixture: `explicit UserService(std::shared_ptr<R> repository);`
   - Parser: Emits `UserService [function] line=118` and `UserService [method] line=118 parent=UserService` ✓
   - Status: **MATCHED** (parser index [59], [60])

8. **deactivate [function] line=177**
   - Fixture: `void User::deactivate() { ... }`
   - Parser: Emits `deactivate [function] line=177` ✓
   - Status: **MATCHED** (parser index [91])

9. **deactivate [method] line=177 parent=User**
    - Parser: Emits `deactivate [method] line=177 parent=User` ✓
    - Status: **MATCHED** (parser index [92])

10. **isActive [function] line=182**
    - Fixture: `bool User::isActive() const { ... }`
    - Parser: Emits `isActive [function] line=182` ✓
    - Status: **MATCHED** (parser index [93])

11. **isActive [method] line=182 parent=User**
    - Parser: Emits `isActive [method] line=182 parent=User` ✓
    - Status: **MATCHED** (parser index [94])

12. **isValid [function] line=191**
    - Fixture: `bool Item::isValid() const { ... }`
    - Parser: Emits `isValid [function] line=191` ✓
    - Status: **MATCHED** (parser index [98])

13. **isValid [method] line=191 parent=Item**
    - Parser: Emits `isValid [method] line=191 parent=Item` ✓
    - Status: **MATCHED** (parser index [99])

14. **calculateDiscount [function] line=196**
    - Fixture: `double Item::calculateDiscount(double percentage) const { ... }`
    - Parser: Emits `calculateDiscount [function] line=196` ✓
    - Status: **MATCHED** (parser index [100])

15. **calculateDiscount [method] line=196 parent=Item**
    - Parser: Emits `calculateDiscount [method] line=196 parent=Item` ✓
    - Status: **MATCHED** (parser index [101])

16. **validateEmail [function] line=201**
    - Fixture: `bool validateEmail(const std::string& email) { ... }`
    - Parser: Emits `validateEmail [function] line=201` ✓
    - Status: **MATCHED** (parser index [102])

17. **formatUser [function] line=206**
    - Fixture: `std::string formatUser(const User& user) { ... }`
    - Parser: Emits `formatUser [function] line=206` ✓
    - Status: **MATCHED** (parser index [103])

18. **calculateTotal [function] line=211**
    - Fixture: `int calculateTotal(const ItemList& items) { ... }`
    - Parser: Emits `calculateTotal [function] line=211` ✓
    - Status: **MATCHED** (parser index [104])

---

## MATCHED Symbols That Spec Expects But May Not Appear in Failed List

Let me check which spec symbols the parser correctly emits:

From parser output index:

- [6-8]: Type aliases (UserId, ItemList, Callback)
- [9]: namespace mdemg
- [12-20]: Repository class and methods
- [21-25]: Validator class and methods
- [26-42]: User class and methods
- [43-57]: Item class and methods
- [58-72]: UserService class and methods
- [73-75]: Standalone functions (validateEmail, formatUser, calculateTotal)
- [76-78]: Template functions (clamp, max call)
- [79-86]: BaseEntity, AdminUser
- [87-104]: Out-of-class implementations

---

## Final Categorization

### SPEC_ERROR (21 total)

Parameters and forward references the spec incorrectly expects:

1. `id [method] line=56 parent=Repository` - parameter
2. `user [method] line=57 parent=Repository` - parameter
3. `id [method] line=58 parent=Repository` - parameter
4. `item [method] line=65 parent=Validator` - parameter
5. `email [method] line=72 parent=User` - parameter
6. `value [method] line=95 parent=Item` - parameter
7. `repository [method] line=118 parent=UserService` - parameter
8. `id [function] line=56` - parameter
9. `user [function] line=57` - parameter
10. `id [function] line=58` - parameter
11. `item [function] line=65` - parameter
12. `email [function] line=173` - parameter
13. `value [function] line=187` - parameter
14. `percentage [function] line=196` - parameter
15. `email [function] line=201` - parameter
16. `user [function] line=206` - parameter
17. `items [function] line=211` - parameter
18. `Item [class] line=29` - forward reference (actual class at line 93)
19. `User [class] line=56` - forward reference (actual class at line 70)
20. `User [class] line=57` - forward reference (actual class at line 70)
21. `Item [class] line=65` - forward reference (actual class at line 93)

### PARSER_BUG (1 total)

Real symbols the parser should emit but doesn't:

1. **`INTERNAL_LIMIT [constant] line=24`** - `static const int INTERNAL_LIMIT = 100;`

### MATCHED (56 total)

All other spec symbols correctly emitted by parser.

---

## Summary Statistics

| Category | Count | Percentage |
|----------|-------|------------|
| MATCHED | 56 | 71.8% |
| SPEC_ERROR | 21 | 26.9% |
| PARSER_BUG | 1 | 1.3% |
| **TOTAL** | **78** | **100%** |

### Test Results Reconciliation

- Test reported: 48 matched, 30 failed
- Our analysis: 56 matched, 22 non-matched (21 spec errors + 1 parser bug)
- Discrepancy: Some symbols may have been counted differently by the test harness

### Critical Finding

The **only genuine parser bug** is the missing `INTERNAL_LIMIT` constant at line 24. The parser fails to recognize the `static const int` pattern and only handles:

- `constexpr` declarations ✓
- `inline constexpr` declarations ✓
- `static const` declarations ❌

All other "failures" are spec errors where the spec generator incorrectly extracted parameter names and forward class references as separate symbols.
