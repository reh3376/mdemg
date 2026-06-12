# Complete C++ Symbol Categorization Table

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


All 78 spec symbols categorized as MATCHED, SPEC_ERROR, or PARSER_BUG.

| # | Symbol Name | Type | Line | Parent | Category | Reason |
|---|-------------|------|------|--------|----------|--------|
| 1 | MAX_RETRIES | constant | 17 | - | ✅ MATCHED | Correct constant detection |
| 2 | DEFAULT_TIMEOUT | constant | 18 | - | ✅ MATCHED | Correct constant detection |
| 3 | API_VERSION | constant | 19 | - | ✅ MATCHED | Correct constant detection |
| 4 | DEBUG_MODE | constant | 20 | - | ✅ MATCHED | Correct constant detection |
| 5 | BUFFER_SIZE | constant | 23 | - | ✅ MATCHED | Correct constant detection |
| 6 | INTERNAL_LIMIT | constant | 24 | - | ❌ PARSER_BUG | Missing: `static const` not detected |
| 7 | UserId | type | 28 | - | ✅ MATCHED | Type alias detected |
| 8 | ItemList | type | 29 | - | ✅ MATCHED | Type alias detected |
| 9 | Item | class | 29 | - | ⚠️ SPEC_ERROR | Forward ref in type alias, not definition |
| 10 | Callback | type | 30 | - | ✅ MATCHED | Type alias detected |
| 11 | mdemg | namespace | 34 | - | ✅ MATCHED | Namespace detected |
| 12 | Status | enum | 38 | - | ✅ MATCHED | Enum detected |
| 13 | Priority | enum | 45 | - | ✅ MATCHED | Enum detected |
| 14 | Repository | class | 53 | - | ✅ MATCHED | Class detected |
| 15 | Repository | method | 55 | Repository | ✅ MATCHED | Destructor detected |
| 16 | id | method | 56 | Repository | ⚠️ SPEC_ERROR | Parameter of findById(), not method |
| 17 | user | method | 57 | Repository | ⚠️ SPEC_ERROR | Parameter of save(), not method |
| 18 | id | method | 58 | Repository | ⚠️ SPEC_ERROR | Parameter of remove(), not method |
| 19 | Repository | function | 55 | - | ✅ MATCHED | Destructor detected |
| 20 | id | function | 56 | - | ⚠️ SPEC_ERROR | Parameter of findById(), not function |
| 21 | User | class | 56 | - | ⚠️ SPEC_ERROR | Forward ref in return type, not definition |
| 22 | user | function | 57 | - | ⚠️ SPEC_ERROR | Parameter of save(), not function |
| 23 | User | class | 57 | - | ⚠️ SPEC_ERROR | Forward ref in parameter, not definition |
| 24 | id | function | 58 | - | ⚠️ SPEC_ERROR | Parameter of remove(), not function |
| 25 | Validator | class | 62 | - | ✅ MATCHED | Class detected |
| 26 | Validator | method | 64 | Validator | ✅ MATCHED | Destructor detected |
| 27 | item | method | 65 | Validator | ⚠️ SPEC_ERROR | Parameter of validate(), not method |
| 28 | Validator | function | 64 | - | ✅ MATCHED | Destructor detected |
| 29 | item | function | 65 | - | ⚠️ SPEC_ERROR | Parameter of validate(), not function |
| 30 | Item | class | 65 | - | ⚠️ SPEC_ERROR | Forward ref in parameter, not definition |
| 31 | User | class | 70 | - | ✅ MATCHED | Class detected |
| 32 | email | method | 72 | User | ⚠️ SPEC_ERROR | Constructor param, should be "User" |
| 33 | User | method | 73 | User | ✅ MATCHED | Destructor detected |
| 34 | getId | method | 76 | User | ✅ MATCHED | Inline method detected |
| 35 | getName | method | 77 | User | ✅ MATCHED | Inline method detected |
| 36 | getEmail | method | 78 | User | ✅ MATCHED | Inline method detected |
| 37 | getStatus | method | 79 | User | ✅ MATCHED | Inline method detected |
| 38 | User | function | 73 | - | ✅ MATCHED | Destructor detected |
| 39 | getId | function | 76 | - | ✅ MATCHED | Inline method as function |
| 40 | getName | function | 77 | - | ✅ MATCHED | Inline method as function |
| 41 | getEmail | function | 78 | - | ✅ MATCHED | Inline method as function |
| 42 | getStatus | function | 79 | - | ✅ MATCHED | Inline method as function |
| 43 | Item | class | 93 | - | ✅ MATCHED | Class detected |
| 44 | value | method | 95 | Item | ⚠️ SPEC_ERROR | Constructor param, should be "Item" |
| 45 | Item | method | 96 | Item | ✅ MATCHED | Destructor detected |
| 46 | getId | method | 99 | Item | ✅ MATCHED | Inline method detected |
| 47 | getName | method | 100 | Item | ✅ MATCHED | Inline method detected |
| 48 | getValue | method | 101 | Item | ✅ MATCHED | Inline method detected |
| 49 | Item | function | 96 | - | ✅ MATCHED | Destructor detected |
| 50 | getId | function | 99 | - | ✅ MATCHED | Inline method as function |
| 51 | getName | function | 100 | - | ✅ MATCHED | Inline method as function |
| 52 | getValue | function | 101 | - | ✅ MATCHED | Inline method as function |
| 53 | UserService | class | 116 | - | ✅ MATCHED | Template class detected |
| 54 | repository | method | 118 | UserService | ⚠️ SPEC_ERROR | Constructor param, should be "UserService" |
| 55 | UserService | method | 119 | UserService | ✅ MATCHED | Destructor detected |
| 56 | UserService | method | 122 | UserService | ✅ MATCHED | Deleted copy constructor |
| 57 | UserService | method | 126 | UserService | ✅ MATCHED | Move constructor |
| 58 | UserService | function | 119 | - | ✅ MATCHED | Destructor detected |
| 59 | UserService | function | 122 | - | ✅ MATCHED | Deleted copy constructor |
| 60 | UserService | function | 126 | - | ✅ MATCHED | Move constructor |
| 61 | max | function | 146 | - | ✅ MATCHED | Template function call detected |
| 62 | BaseEntity | class | 152 | - | ✅ MATCHED | Abstract class detected |
| 63 | BaseEntity | method | 154 | BaseEntity | ✅ MATCHED | Virtual destructor detected |
| 64 | getId | method | 155 | BaseEntity | ✅ MATCHED | Pure virtual method detected |
| 65 | BaseEntity | function | 154 | - | ✅ MATCHED | Virtual destructor detected |
| 66 | getId | function | 155 | - | ✅ MATCHED | Pure virtual method as function |
| 67 | AdminUser | class | 161 | - | ✅ MATCHED | Derived class detected |
| 68 | getId | method | 164 | AdminUser | ✅ MATCHED | Override method detected |
| 69 | getId | function | 164 | - | ✅ MATCHED | Override method as function |
| 70 | mdemg | namespace | 171 | - | ✅ MATCHED | Namespace reopening detected |
| 71 | email | function | 173 | - | ⚠️ SPEC_ERROR | Constructor param, parser emits "User" |
| 72 | deactivate | function | 177 | - | ✅ MATCHED | Out-of-class method impl detected |
| 73 | isActive | function | 182 | - | ✅ MATCHED | Out-of-class method impl detected |
| 74 | value | function | 187 | - | ⚠️ SPEC_ERROR | Constructor param, parser emits "Item" |
| 75 | isValid | function | 191 | - | ✅ MATCHED | Out-of-class method impl detected |
| 76 | percentage | function | 196 | - | ⚠️ SPEC_ERROR | Method param, parser emits "calculateDiscount" |
| 77 | email | function | 201 | - | ⚠️ SPEC_ERROR | Function param, parser emits "validateEmail" |
| 78 | user | function | 206 | - | ⚠️ SPEC_ERROR | Function param, parser emits "formatUser" |

**Note**: Row #79 (items at line 211) was accidentally added. The spec has exactly 78 symbols. Let me check if I miscounted or if "items" is actually in the spec...

Checking spec JSON lines 521-525:

```json
{"name": "items", "type": "function", "line": 211, "exported": true}
```

Yes, "items" IS in the spec as the 78th symbol. Let me verify the count more carefully:

Actually, I need to recount from the spec JSON. Looking at the symbols array:

- Lines 34-39: MAX_RETRIES (symbol 1)
- Lines 40-45: DEFAULT_TIMEOUT (symbol 2)
...continuing through...
- Lines 521-525: items (symbol 78)

The spec has 78 symbols. But looking at my table rows 71-78, that's only 8 rows for the last section. Let me check what's missing.

Spec symbols for out-of-class implementations (from spec lines 467-525):

1. mdemg [namespace] line=171 - ✓ row 70
2. email [function] line=173 - ✓ row 71
3. deactivate [function] line=177 - ✓ row 72
4. isActive [function] line=182 - ✓ row 73
5. value [function] line=187 - ✓ row 74
6. isValid [function] line=191 - ✓ row 75
7. percentage [function] line=196 - ✓ row 76
8. email [function] line=201 - ✓ row 77
9. user [function] line=206 - ✓ row 78
10. items [function] line=211 - ❌ MISSING

So I do need to add row 79 for "items". But that makes 79 total, not 78. Let me check where I double-counted.

Oh! I see the issue. Looking at row 71, I have:
| 71 | email | function | 173 | - | ⚠️ SPEC_ERROR | Constructor param, parser emits "User" |

But row 77 is:
| 77 | email | function | 201 | - | ⚠️ SPEC_ERROR | Function param, parser emits "validateEmail" |

There are TWO "email" symbols in the spec! One at line 173 and one at line 201. So the spec does have 78 unique entries, but some have duplicate names.

Wait, I'm confusing myself. Let me just count rows in my table:

Rows 1-78 should map to spec symbols 1-78. Let me verify row 78 is the last row, and if "items" needs to be added.

Looking at my table, row 78 is "user [function] line=206". That should be spec symbol 77 or 78.

Let me check the spec more carefully:

From spec JSON (showing only name and line):
...

- Symbol around index 76: email, line=201
- Symbol around index 77: user, line=206
- Symbol around index 78: items, line=211

So "items [function] line=211" should be the 78th symbol and should be row 78 in my table.

But my table has "user [function] line=206" at row 78, which means "items" is missing!

Let me add it:

| 79 | items | function | 211 | - | ⚠️ SPEC_ERROR | Function param, parser emits "calculateTotal" |

Now I have 79 rows for 78 spec symbols, which means I double-counted somewhere. Let me find the duplicate...

Actually, let me just regenerate the last section more carefully to make sure I have exactly 78 rows for 78 spec symbols.
