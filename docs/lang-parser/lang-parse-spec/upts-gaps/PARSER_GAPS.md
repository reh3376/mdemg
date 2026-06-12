# Parser Gaps - UPTS Compliance Tracker

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Last Updated:** 2026-01-29

This document tracks known gaps between parser output and UPTS canonical expectations.
These are handled via TYPE_COMPAT in `upts_runner.py` but should be fixed in parsers.

---

## Python Parser Gaps

| Gap | Expected | Actual | Location | Fix |
|-----|----------|--------|----------|-----|
| Protocol detection | `interface` | `class` | `python_parser.go` | Check for `Protocol` base class |
| Enum detection | `enum` | `class` | `python_parser.go` | Check for `Enum` base class |
| Method extraction | `method` with `parent` | `function` without `parent` | `python_parser.go` | Track class context during parse |
| Type alias detection | `type` | `variable` | `python_parser.go` | Detect `TypeAlias` annotation or simple assignment patterns |

### Example: Protocol Not Detected

```python
# Fixture
class UserRepository(Protocol):
    def find_by_id(self, user_id: str) -> Optional["User"]: ...

# Parser outputs
{"name": "UserRepository", "type": "class", ...}  # Should be "interface"
{"name": "find_by_id", "type": "function", ...}   # Should be "method" with parent
```

### Example: Type Alias Not Detected

```python
# Fixture
UserId = str

# Parser outputs
{"name": "UserId", "type": "variable", ...}  # Should be "type"
```

---

## TypeScript Parser Gaps

| Gap | Expected | Actual | Location | Fix |
|-----|----------|--------|----------|-----|
| Arrow function detection | `function` | `constant` | `typescript_parser.go` | Check if const value is arrow function |
| Class method extraction | `method` with `parent` | Not extracted | `typescript_parser.go` | Walk class body for method definitions |
| Abstract class detection | `class` | Not extracted | `typescript_parser.go` | Handle `abstract class` syntax |

### Example: Arrow Function Not Detected

```typescript
// Fixture
export const validateId = (id: string): boolean => { ... }

// Parser outputs
{"name": "validateId", "type": "constant", ...}  # Should be "function"
```

### Example: Class Methods Not Extracted

```typescript
// Fixture
export class UserService {
    async findById(id: UserId): Promise<UserDto | null> { ... }
}

// Parser outputs
{"name": "UserService", "type": "class", ...}
// Missing: {"name": "findById", "type": "method", "parent": "UserService", ...}
```

---

## Go Parser

**Status: Complete** - No known gaps. All UPTS patterns passing.

---

## TYPE_COMPAT Workarounds

These mappings in `upts_runner.py` compensate for parser gaps:

```python
TYPE_COMPAT = {
    "interface": {"interface", "trait", "protocol", "class"},  # PARSER-GAP: Python Protocol
    "method": {"method", "function"},                          # PARSER-GAP: Python methods
    "enum": {"enum", "class"},                                 # PARSER-GAP: Python enums
    "type": {"type", "variable"},                              # PARSER-GAP: Python type aliases
    "function": {"function", "constant"},                      # PARSER-GAP: TS arrow functions
}
```

---

## Resolution Priority

1. **High**: Class method extraction (affects code navigation)
2. **High**: Protocol/Interface detection (affects type hierarchy)
3. **Medium**: Enum detection (affects constant grouping)
4. **Medium**: Type alias detection (affects type navigation)
5. **Low**: Arrow function detection (cosmetic - symbol is found, just wrong type)

---

## Verification

After fixing a parser gap:

1. Remove the TYPE_COMPAT entry
2. Run `make test-parsers`
3. If PASS, update this document
4. If FAIL, the fix is incomplete
