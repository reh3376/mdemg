# C++ Parser Test - Complete Failure Categorization

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


## Test Results Summary

- Total expected symbols: 78 (from spec)
- Parser emitted: 105 symbols
- Matched: 48
- Failed: 30
- Status: Need to categorize all 30 failures

---

## Detailed Analysis of 30 Failed Symbols

### 1. `Item [class] line=29`

- **Fixture line 29**: `using ItemList = std::vector<class Item>;`
- **Issue**: Forward-declared `class Item` in type alias
- **Parser behavior**: Correctly emits `Item [class]` at line 93 (actual definition)
- **Category**: **SPEC_ERROR** - Spec picked up forward reference instead of definition

### 2. `id [method] line=56 parent=Repository`

- **Fixture line 56**: `virtual std::optional<class User> findById(const UserId& id) = 0;`
- **Issue**: Parameter name `id`, not method name (method name is `findById`)
- **Category**: **SPEC_ERROR** - Parameter mistaken for method

### 3. `user [method] line=57 parent=Repository`

- **Fixture line 57**: `virtual bool save(const class User& user) = 0;`
- **Issue**: Parameter name `user` (method name is `save`)
- **Category**: **SPEC_ERROR** - Parameter mistaken for method

### 4. `id [method] line=58 parent=Repository`

- **Fixture line 58**: `virtual bool remove(const UserId& id) = 0;`
- **Issue**: Parameter name `id` (method name is `remove`)
- **Category**: **SPEC_ERROR** - Parameter mistaken for method

### 5. `id [function] line=56`

- **Fixture line 56**: `virtual std::optional<class User> findById(const UserId& id) = 0;`
- **Issue**: Parameter name `id` (function name is `findById`)
- **Category**: **SPEC_ERROR** - Parameter mistaken for function

### 6. `User [class] line=56`

- **Fixture line 56**: `virtual std::optional<class User> findById(...)`
- **Issue**: Forward reference `class User` in return type
- **Parser behavior**: Correctly emits `User [class]` at line 70 (actual definition)
- **Category**: **SPEC_ERROR** - Forward reference instead of definition

### 7. `user [function] line=57`

- **Fixture line 57**: `virtual bool save(const class User& user) = 0;`
- **Issue**: Parameter name `user` (function name is `save`)
- **Category**: **SPEC_ERROR** - Parameter mistaken for function

### 8. `User [class] line=57`

- **Fixture line 57**: `virtual bool save(const class User& user) = 0;`
- **Issue**: Forward reference `class User` in parameter type
- **Category**: **SPEC_ERROR** - Forward reference instead of definition

### 9. `id [function] line=58`

- **Fixture line 58**: `virtual bool remove(const UserId& id) = 0;`
- **Issue**: Parameter name `id` (function name is `remove`)
- **Category**: **SPEC_ERROR** - Parameter mistaken for function

### 10. `item [method] line=65 parent=Validator`

- **Fixture line 65**: `virtual bool validate(const class Item& item) = 0;`
- **Issue**: Parameter name `item` (method name is `validate`)
- **Category**: **SPEC_ERROR** - Parameter mistaken for method

### 11. `item [function] line=65`

- **Fixture line 65**: `virtual bool validate(const class Item& item) = 0;`
- **Issue**: Parameter name `item` (function name is `validate`)
- **Category**: **SPEC_ERROR** - Parameter mistaken for function

### 12. `Item [class] line=65`

- **Fixture line 65**: `virtual bool validate(const class Item& item) = 0;`
- **Issue**: Forward reference `class Item` in parameter type
- **Category**: **SPEC_ERROR** - Forward reference instead of definition

### 13. `email [method] line=72 parent=User`

- **Fixture line 72**: `User(UserId id, std::string name, std::string email);`
- **Issue**: Constructor parameter name `email`
- **Category**: **SPEC_ERROR** - Parameter mistaken for method

### 14. `value [method] line=95 parent=Item`

- **Fixture line 95**: `Item(std::string id, std::string name, int value);`
- **Issue**: Constructor parameter name `value`
- **Category**: **SPEC_ERROR** - Parameter mistaken for method

### 15. `repository [method] line=118 parent=UserService`

- **Fixture line 118**: `explicit UserService(std::shared_ptr<R> repository);`
- **Issue**: Constructor parameter name `repository`
- **Category**: **SPEC_ERROR** - Parameter mistaken for method

### 16. `email [function] line=173`

- **Fixture line 173**: `User::User(UserId id, std::string name, std::string email)`
- **Issue**: Constructor parameter name `email`
- **Category**: **SPEC_ERROR** - Parameter mistaken for function

### 17. `value [function] line=187`

- **Fixture line 187**: `Item::Item(std::string id, std::string name, int value)`
- **Issue**: Constructor parameter name `value`
- **Category**: **SPEC_ERROR** - Parameter mistaken for function

### 18. `percentage [function] line=196`

- **Fixture line 196**: `double Item::calculateDiscount(double percentage) const {`
- **Issue**: Method parameter name `percentage`
- **Category**: **SPEC_ERROR** - Parameter mistaken for function

### 19. `email [function] line=201`

- **Fixture line 201**: `bool validateEmail(const std::string& email) {`
- **Issue**: Function parameter name `email`
- **Category**: **SPEC_ERROR** - Parameter mistaken for function

### 20. `user [function] line=206`

- **Fixture line 206**: `std::string formatUser(const User& user) {`
- **Issue**: Function parameter name `user`
- **Category**: **SPEC_ERROR** - Parameter mistaken for function

### 21. `items [function] line=211`

- **Fixture line 211**: `int calculateTotal(const ItemList& items) {`
- **Issue**: Function parameter name `items`
- **Category**: **SPEC_ERROR** - Parameter mistaken for function

### 22-30. Additional Failures to Verify

Let me check if there are more failures. The user mentioned 30 total, but only listed 21 explicitly. Let me analyze what the spec expects vs what parser emits.

Looking at the spec's 78 symbols, I need to find which ones aren't in the parser output:

From spec, checking against parser output [0-104]:

**Missing from parser output:**

- `INTERNAL_LIMIT [constant] line=24` - ❌ NOT in parser output
  - **Category**: **PARSER_BUG** - Real constant that should be detected

Let me verify other potential mismatches by looking at method declarations:

**Checking in-class vs out-of-class method implementations:**

- Parser emits BOTH in-class declarations AND out-of-class implementations
- Spec may expect only one or the other

**User class methods (lines 72-83):**

- Constructor at line 72: Parser emits [27]=User[method], [28]=User[function]
- Destructor at line 73: Parser emits [29]=User[function], [30]=User[method]
- deactivate at line 82: Parser emits [39]=deactivate[function], [40]=deactivate[method]
- isActive at line 83: Parser emits [41]=isActive[function], [42]=isActive[method]

**Item class methods (lines 95-105):**

- Constructor at line 95: Parser emits [44]=Item[function], [45]=Item[method]
- Destructor at line 96: Parser emits [46]=Item[function], [47]=Item[method]
- isValid at line 104: Parser emits [54]=isValid[function], [55]=isValid[method]
- calculateDiscount at line 105: Parser emits [56]=calculateDiscount[function], [57]=calculateDiscount[method]

**Standalone functions (lines 140-142, 146-147):**

- validateEmail at line 140: Parser emits [73]=validateEmail[function] ✓
- formatUser at line 141: Parser emits [74]=formatUser[function] ✓
- calculateTotal at line 142: Parser emits [75]=calculateTotal[function] ✓
- clamp at line 146: Parser emits [76]=clamp[function] ✓
- max at line 147: Parser emits [77]=max[function], [78]=max[method parent=std] ✓

**Out-of-class implementations (lines 173-217):**

- User::User at line 173: Parser emits [88]=User[function], [89]=User[method parent=User]
- User::deactivate at line 177: Parser emits [91]=deactivate[function], [92]=deactivate[method]
- User::isActive at line 182: Parser emits [93]=isActive[function], [94]=isActive[method]
- Item::Item at line 187: Parser emits [95]=Item[function], [96]=Item[method]
- Item::isValid at line 191: Parser emits [98]=isValid[function], [99]=isValid[method]
- Item::calculateDiscount at line 196: Parser emits [100]=calculateDiscount[function], [101]=calculateDiscount[method]
- validateEmail at line 201: Parser emits [102]=validateEmail[function]
- formatUser at line 206: Parser emits [103]=formatUser[function]
- calculateTotal at line 211: Parser emits [104]=calculateTotal[function]

Wait, I see duplicates! The parser emits BOTH the declaration AND the implementation.

Let me check: does the spec expect out-of-class implementations separately?

Looking at spec lines 472-525:

- Line 473: `email [function] line=173` - OUT-OF-CLASS IMPL parameter
- Line 479: `deactivate [function] line=177` - OUT-OF-CLASS IMPL ✓ matched
- Line 485: `isActive [function] line=182` - OUT-OF-CLASS IMPL ✓ matched
- Line 491: `value [function] line=187` - OUT-OF-CLASS IMPL parameter
- Line 497: `isValid [function] line=191` - OUT-OF-CLASS IMPL ✓ matched
- Line 503: `percentage [function] line=196` - OUT-OF-CLASS IMPL parameter
- Line 509: `email [function] line=201` - function parameter
- Line 515: `user [function] line=206` - function parameter
- Line 521: `items [function] line=211` - function parameter

So the spec DOES expect separate symbols for out-of-class implementations, which the parser correctly provides.

Now, let me check for method/function pairs. The spec has 78 symbols total. Let me count:

**Constants (5 + 1 missing):**

1. MAX_RETRIES ✓
2. DEFAULT_TIMEOUT ✓
3. API_VERSION ✓
4. DEBUG_MODE ✓
5. BUFFER_SIZE ✓
6. INTERNAL_LIMIT ❌ MISSING (line 24)

**Type aliases (3):**
7. UserId ✓
8. ItemList ✓
9. Callback ✓

**Forward class refs (4) - SPEC ERRORS:**
10. Item [class] line=29 - forward ref
11. User [class] line=56 - forward ref
12. User [class] line=57 - forward ref
13. Item [class] line=65 - forward ref

**Namespaces (2):**
14. mdemg line=34 ✓
15. mdemg line=171 ✓

**Enums (2):**
16. Status ✓
17. Priority ✓

**Repository class (4 + 6 params):**
18. Repository [class] ✓
19. Repository [method] line=55 ✓ (destructor)
20. Repository [function] line=55 ✓ (destructor)
21-26. Parameter names (id, user, id as methods/functions) - SPEC ERRORS

**Validator class (2 + 2 params):**
27. Validator [class] ✓
28. Validator [method] line=64 ✓ (destructor)
29. Validator [function] line=64 ✓
30-31. Parameter names (item as method/function) - SPEC ERRORS

**User class (9 + 1 param):**
32. User [class] line=70 ✓
33. User [method] line=73 ✓ (destructor)
34. User [function] line=73 ✓ (destructor)
35. getId [method] line=76 ✓
36. getName [method] line=77 ✓
37. getEmail [method] line=78 ✓
38. getStatus [method] line=79 ✓
39. getId [function] line=76 ✓
40. getName [function] line=77 ✓
41. getEmail [function] line=78 ✓
42. getStatus [function] line=79 ✓
43. email [method] line=72 - parameter - SPEC ERROR

**Item class (8 + 1 param):**
44. Item [class] line=93 ✓
45. Item [method] line=96 ✓ (destructor)
46. Item [function] line=96 ✓ (destructor)
47. getId [method] line=99 ✓
48. getName [method] line=100 ✓
49. getValue [method] line=101 ✓
50. getId [function] line=99 ✓
51. getName [function] line=100 ✓
52. getValue [function] line=101 ✓
53. value [method] line=95 - parameter - SPEC ERROR

**UserService class (4 + 1 param):**
54. UserService [class] ✓
55. UserService [method] line=119 ✓
56. UserService [method] line=122 ✓
57. UserService [method] line=126 ✓
58. UserService [function] line=119 ✓
59. UserService [function] line=122 ✓
60. UserService [function] line=126 ✓
61. repository [method] line=118 - parameter - SPEC ERROR

**Standalone functions (1):**
62. max [function] line=146 ✓ (actually std::max call, but parser detects it)

**BaseEntity class (4):**
63. BaseEntity [class] ✓
64. BaseEntity [method] line=154 ✓
65. getId [method] line=155 ✓
66. BaseEntity [function] line=154 ✓
67. getId [function] line=155 ✓

**AdminUser class (3):**
68. AdminUser [class] ✓
69. getId [method] line=164 ✓
70. getId [function] line=164 ✓

**Out-of-class implementations (8 params):**
71. email [function] line=173 - parameter - SPEC ERROR
72. deactivate [function] line=177 ✓ (but also declared at line 82 in-class)
73. isActive [function] line=182 ✓ (but also declared at line 83 in-class)
74. value [function] line=187 - parameter - SPEC ERROR
75. isValid [function] line=191 ✓ (but also declared at line 104 in-class)
76. percentage [function] line=196 - parameter - SPEC ERROR
77. email [function] line=201 - parameter - SPEC ERROR
78. user [function] line=206 - parameter - SPEC ERROR
79. items [function] line=211 - parameter - SPEC ERROR

Wait, that's 79 items, but spec says 78. Let me recount.

Actually, looking at the spec JSON more carefully at the "symbols" array (lines 33-526), there are exactly 78 entries.

Let me count the failures more precisely:

**SPEC_ERROR category (parameter names + forward refs): 21**

- 7 method-type parameters
- 10 function-type parameters
- 4 forward class references
= 21 total

**PARSER_BUG category: 1**

- INTERNAL_LIMIT [constant] line=24 - missing

**Potential discrepancy with test:**
Test says 30 failed, but I'm finding only 22 non-matches (21 spec errors + 1 parser bug).

Let me check if there are method implementations that are expected but not emitted.

Actually, wait. Let me re-read the user's question. They said "48 matched, 30 failed" which adds up to 78 total. That means EVERY spec symbol is accounted for.

So:

- 48 MATCHED (correct matches)
- 21 SPEC_ERROR (incorrect expectations)
- 1 PARSER_BUG (INTERNAL_LIMIT missing)
= 70 total... that's not 78.

Let me check if there are more missing symbols I haven't identified.

Actually, looking at the user's original numbered list, they listed items 1-21 and said "22-23. isValid, calculateDiscount - MATCHED".

So there might be 7-9 more failures they didn't list explicitly. Let me find them by checking the spec against parser output systematically.

Looking for spec symbols NOT in parser output:

1. INTERNAL_LIMIT [constant] line=24 - ✓ identified as PARSER_BUG

Let me check for method declarations that might be expected but missing:

**Repository methods - checking if parser emits them:**

- Line 56: findById - Parser has [15]=findById[function], [16]=findById[method parent=Repository] ✓
- Line 57: save - Parser has [17]=save[function], [18]=save[method parent=Repository] ✓
- Line 58: remove - Parser has [19]=remove[function], [20]=remove[method parent=Repository] ✓

**Validator methods:**

- Line 65: validate - Parser has [24]=validate[function], [25]=validate[method parent=Validator] ✓

**User methods:**

- Line 72: User constructor - Parser has [27]=User[function], [28]=User[method parent=User] ✓
- Line 82: deactivate (in-class decl) - Parser has [39]=deactivate[function], [40]=deactivate[method] ✓
- Line 83: isActive (in-class decl) - Parser has [41]=isActive[function], [42]=isActive[method] ✓

**Item methods:**

- Line 95: Item constructor - Parser has [44]=Item[function], [45]=Item[method parent=Item] ✓
- Line 104: isValid (in-class decl) - Parser has [54]=isValid[function], [55]=isValid[method] ✓
- Line 105: calculateDiscount (in-class decl) - Parser has [56]=calculateDiscount[function], [57]=calculateDiscount[method] ✓

**UserService methods:**

- Line 118: UserService constructor - Parser has [59]=UserService[function], [60]=UserService[method] ✓
- Line 130: findById - Parser has [67]=findById[function], [68]=findById[method parent=UserService] ✓
- Line 131: createUser - Parser has [69]=createUser[function], [70]=createUser[method] ✓
- Line 132: deleteUser - Parser has [71]=deleteUser[function], [72]=deleteUser[method] ✓

Hmm, all the real methods are being emitted. The only missing symbol is INTERNAL_LIMIT.

Let me check if there are duplicate method expectations. For example, does the spec expect BOTH:

- deactivate [function] line=82 (in-class declaration)
- deactivate [function] line=177 (out-of-class implementation)

Looking at spec lines 479-482:

```json
{
  "name": "deactivate",
  "type": "function",
  "line": 177,
  "exported": true
}
```

And earlier in the spec, I don't see deactivate at line 82. So the spec expects:

- In-class declarations for inline methods (getId, getName, etc.)
- Out-of-class implementations for non-inline methods (deactivate, isActive, etc.)

But the parser emits BOTH. So:

- Parser emits deactivate at line 82 (in-class decl) - not in spec
- Parser emits deactivate at line 177 (out-of-class impl) - in spec ✓

But wait, looking at the fixture:

```cpp
// Line 82
void deactivate();   // <-- declaration only, not inline
// Line 83
bool isActive() const;  // <-- declaration only, not inline
```

These are declarations in the class body, not implementations. The implementations are at lines 177 and 182.

So the spec expects only the implementations (lines 177, 182), not the declarations (lines 82, 83).

But the parser emits BOTH the declarations and implementations!

This means:

- Parser emits deactivate [method] line=82 - NOT in spec (extra, but not a failure)
- Spec expects deactivate [function] line=177 - Parser emits ✓

Since the spec has `allow_extra_symbols: true`, the extra emissions at lines 82-83 are OK.

So where are the other 8 failures? (30 total - 21 param/forward errors - 1 missing INTERNAL_LIMIT = 8 unaccounted)

Let me check if there are method declarations the spec expects but parser doesn't emit:

**Looking for UserService template class methods:**
Lines 130-132 in fixture:

```cpp
std::optional<User> findById(const UserId& id);
bool createUser(const User& user);
bool deleteUser(const UserId& id);
```

Parser output shows:

- [67]=findById [function] line=130
- [68]=findById [method] line=130 parent=UserService
- [69]=createUser [function] line=131
- [70]=createUser [method] line=131 parent=UserService
- [71]=deleteUser [function] line=132
- [72]=deleteUser [method] line=132 parent=UserService

All present! ✓

I think the discrepancy might be in how I'm counting. Let me verify the user's claim of 30 failures by checking the spec more carefully.

Actually, I realize the issue. The user said:

- "48 matched"
- "30 failed"

But they also said "the test produced 105 symbols" from the parser.

So:

- Parser emitted 105 symbols
- Spec expects 78 symbols
- 48 of the spec's 78 matched parser output
- 30 of the spec's 78 did NOT match parser output

That's 48 + 30 = 78 ✓

So I need to categorize those 30 failures:

- 21 are SPEC_ERRORs (parameters + forward refs)
- 1 is PARSER_BUG (INTERNAL_LIMIT)
- That leaves 8 unaccounted for

Let me re-examine the spec to find which 8 symbols might be expected but not matched.

Looking at the fixture for inline methods that might not be detected:

**User class inline methods (lines 76-79):**

```cpp
const UserId& getId() const { return id_; }
const std::string& getName() const { return name_; }
const std::string& getEmail() const { return email_; }
Status getStatus() const { return status_; }
```

Parser shows these at indices [31-38] as both function and method. ✓

**Item class inline methods (lines 99-101):**

```cpp
const std::string& getId() const { return id_; }
const std::string& getName() const { return name_; }
int getValue() const { return value_; }
```

Parser shows these at indices [48-53] as both function and method. ✓

**AdminUser class (line 164):**

```cpp
std::string getId() const override { return User::getId(); }
```

Parser shows this at indices [85-86] as both function and method. ✓

All inline methods are detected!

Let me check standalone function declarations vs implementations:

**Lines 140-142 (declarations in namespace):**

```cpp
bool validateEmail(const std::string& email);
std::string formatUser(const User& user);
int calculateTotal(const ItemList& items);
```

**Lines 201-217 (implementations):**

```cpp
bool validateEmail(const std::string& email) { ... }
std::string formatUser(const User& user) { ... }
int calculateTotal(const ItemList& items) { ... }
```

Does the spec expect both declarations and implementations?

Looking at spec, I see:

- validateEmail at line 140 (declaration) - NOT in spec symbols list
- formatUser at line 141 (declaration) - NOT in spec symbols list
- calculateTotal at line 142 (declaration) - NOT in spec symbols list
- validateEmail at line 201 (implementation) - spec expects this
- formatUser at line 206 (implementation) - spec expects this
- calculateTotal at line 211 (implementation) - spec expects this

But the parser emits at lines 140-142 (declarations) as:

- [73]=validateEmail [function] line=140
- [74]=formatUser [function] line=141
- [75]=calculateTotal [function] line=142

And again at lines 201-217 (implementations) as:

- [102]=validateEmail [function] line=201
- [103]=formatUser [function] line=206
- [104]=calculateTotal [function] line=211

So the parser emits BOTH declarations and implementations (6 total), but spec only expects implementations (3 total).

Since `allow_extra_symbols: true`, the declarations at 140-142 are OK (extra but not errors).

The implementations at 201, 206, 211 should match... but wait, the spec expects functions at these lines, but those lines have PARAMETERS in the spec's symbols list!

Looking at spec lines 509-524:

```json
{"name": "email", "type": "function", "line": 201},
{"name": "user", "type": "function", "line": 206},
{"name": "items", "type": "function", "line": 211}
```

These are parameter names, not function names! So the spec is WRONG here too.

But then where are the actual function names for validateEmail, formatUser, calculateTotal at lines 201, 206, 211?

Let me search the spec for these:

Searching for "validateEmail" in spec... NOT FOUND in expected symbols!
Searching for "formatUser" in spec... NOT FOUND in expected symbols!
Searching for "calculateTotal" in spec... NOT FOUND in expected symbols!

AH HA! The spec is missing these 3 standalone function implementations!

So we have:

- 21 SPEC_ERRORs (parameters + forward refs that shouldn't be there)
- 1 PARSER_BUG (INTERNAL_LIMIT missing)
- 3 MORE SPEC_ERRORs (spec has parameter names instead of function names for lines 201, 206, 211)
- 3 MORE PARSER_BUGS? (spec missing validateEmail, formatUser, calculateTotal at declaration lines 140-142)

But wait, if spec expects implementations at 201/206/211 but has wrong names (parameters instead), and parser emits correct functions at those lines, then:

- Parser emits validateEmail [function] line=201 ✓
- Spec expects email [function] line=201 ❌ wrong name
= MISMATCH counted as failure

Let me recalculate:

- Spec has 78 symbols
- 48 matched
- 30 failed

Of the 30 failures:

1. Item [class] line=29 - forward ref - SPEC_ERROR
2. id [method] line=56 parent=Repository - param - SPEC_ERROR
3. user [method] line=57 parent=Repository - param - SPEC_ERROR
4. id [method] line=58 parent=Repository - param - SPEC_ERROR
5. id [function] line=56 - param - SPEC_ERROR
6. User [class] line=56 - forward ref - SPEC_ERROR
7. user [function] line=57 - param - SPEC_ERROR
8. User [class] line=57 - forward ref - SPEC_ERROR
9. id [function] line=58 - param - SPEC_ERROR
10. item [method] line=65 parent=Validator - param - SPEC_ERROR
11. item [function] line=65 - param - SPEC_ERROR
12. Item [class] line=65 - forward ref - SPEC_ERROR
13. email [method] line=72 parent=User - param - SPEC_ERROR
14. value [method] line=95 parent=Item - param - SPEC_ERROR
15. repository [method] line=118 parent=UserService - param - SPEC_ERROR
16. email [function] line=173 - param - SPEC_ERROR
17. value [function] line=187 - param - SPEC_ERROR
18. percentage [function] line=196 - param - SPEC_ERROR
19. email [function] line=201 - param (spec expects this, parser emits validateEmail) - SPEC_ERROR
20. user [function] line=206 - param (spec expects this, parser emits formatUser) - SPEC_ERROR
21. items [function] line=211 - param (spec expects this, parser emits calculateTotal) - SPEC_ERROR
22. INTERNAL_LIMIT [constant] line=24 - missing - PARSER_BUG

That's only 22. Where are the other 8 failures?

OH! I need to check if there are spec symbols that exist but DON'T match because of line number mismatch (within tolerance) or parent mismatch.

Let me think about this differently. The test said "30 failed". These are spec symbols that failed to match. Not parser symbols that are extra.

So I need to go through all 78 spec symbols and identify which 30 didn't match.

Let me check constructor at line 72:

Spec expects:

```json
{"name": "email", "type": "method", "line": 72, "parent": "User"}
```

Parser emits:

- [27]=User [function] line=72
- [28]=User [method] line=72 parent=User

Spec expects `email` but parser emits `User`. MISMATCH.

Wait, but the user's analysis says "User [method] line=72" should MATCH. Let me re-read the fixture:

Line 72: `User(UserId id, std::string name, std::string email);`

This is a constructor. The spec expects "email" (parameter), but actually the spec SHOULD expect "User" (constructor name).

But looking at the spec symbols list, I see:

- Line 221-226: `{"name": "email", "type": "method", "line": 72, ...}`

So the spec does NOT have "User [method] line=72" in its expected list! It has "email [method] line=72" instead!

Therefore:

- Spec expects: email [method] line=72 parent=User - SPEC_ERROR
- Parser emits: User [method] line=72 parent=User - CORRECT
= Parser is right, spec is wrong

But there SHOULD be an entry in the spec for "User [method] line=72"! Let me search...

Searching spec for "User" with line 72... NOT FOUND as a method!

Wait, but earlier in the spec at lines 227-232:

```json
{"name": "User", "type": "method", "line": 73, "parent": "User"}
```

Line 73 is the DESTRUCTOR! `~User() = default;`

So the spec has:

- User [method] line=73 (destructor) ✓
- email [method] line=72 (ERROR - should be User constructor)

The spec is MISSING "User [method] line=72" (constructor)!

Let me check all constructors:

**Repository destructor:**

- Line 55: `virtual ~Repository() = default;`
- Spec expects: Repository [method] line=55 ✓
- Parser emits: [13]=Repository [function] line=55, [14]=Repository [method] parent=Repository ✓

**User constructor and destructor:**

- Line 72: `User(UserId id, std::string name, std::string email);`
- Spec expects: email [method] line=72 ❌ SPEC_ERROR
- Spec MISSING: User [method] line=72 ❌ SPEC_OMISSION
- Parser emits: [27]=User [function] line=72, [28]=User [method] parent=User ✓

- Line 73: `~User() = default;`
- Spec expects: User [method] line=73 ✓
- Parser emits: [29]=User [function] line=73, [30]=User [method] parent=User ✓

**Item constructors:**

- Line 95: `Item(std::string id, std::string name, int value);`
- Spec expects: value [method] line=95 ❌ SPEC_ERROR
- Spec MISSING: Item [method] line=95 ❌ SPEC_OMISSION
- Parser emits: [44]=Item [function] line=95, [45]=Item [method] parent=Item ✓

- Line 96: `~Item() = default;`
- Spec expects: Item [method] line=96 ✓
- Parser emits: [46]=Item [function] line=96, [47]=Item [method] parent=Item ✓

**UserService constructor:**

- Line 118: `explicit UserService(std::shared_ptr<R> repository);`
- Spec expects: repository [method] line=118 ❌ SPEC_ERROR
- Spec MISSING: UserService [method] line=118 ❌ SPEC_OMISSION
- Parser emits: [59]=UserService [function] line=118, [60]=UserService [method] parent=UserService ✓

So there are 3 SPEC_OMISSIONS where the spec is missing constructor declarations because it incorrectly has parameter names instead:

1. User [method] line=72 (spec has "email" instead)
2. Item [method] line=95 (spec has "value" instead)
3. UserService [method] line=118 (spec has "repository" instead)

And 3 corresponding SPEC_ERRORs:

1. email [method] line=72
2. value [method] line=95
3. repository [method] line=118

These are already counted in my 21 SPEC_ERRORs list.

But now I need to check: does the test count these OMISSIONS as additional failures?

If the spec is missing "User [method] line=72" entirely, then the test wouldn't check for it, so it wouldn't be a "failure". The spec only has 78 symbols, and the test checks those 78.

So the 30 failures must all be within those 78 spec symbols.

Let me verify by checking if there are more symbols I haven't identified:

Actually, I think I've been over-complicating this. Let me just count the SPEC_ERRORs from the user's numbered list:

From user's message:
1-21: Parameter names and forward references - 21 SPEC_ERRORs

The user then said:
"22. isValid [method] line=104 parent=Item - MATCHED"
"23. calculateDiscount [method] line=105 parent=Item - MATCHED"

So items 22-23 are NOT failures, they're confirmations that these MATCHED.

Then the user asks: "Focus especially on... Return a summary: total matched, total spec errors, total parser bugs"

So the user wants me to go through ALL 78 spec symbols and categorize them.

Let me check the spec systematically for what doesn't match:

Going through spec lines 34-526 (the "symbols" array):

I'll write a cleaner summary now.
