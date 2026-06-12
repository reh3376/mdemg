# Parser Gaps - UPTS Compliance Tracker

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Last Updated:** 2026-01-29  
**Status:** ✅ ALL GAPS RESOLVED

---

## Summary

All 3 parsers now pass UPTS validation at 100% with **no workarounds**.

| Language   | Matched      | Status |
|------------|--------------|--------|
| Go         | 17/17 (100%) | ✅ PASS |
| Python     | 35/35 (100%) | ✅ PASS |
| TypeScript | 13/13 (100%) | ✅ PASS |

---

## Fixes Implemented

### Python Parser (`internal/symbols/parser.go`)

| Gap | Fix | Status |
|-----|-----|:------:|
| Protocol → `interface` | Check superclasses for `Protocol` | ✅ |
| Enum → `enum` | Check superclasses for `Enum` | ✅ |
| Methods have `parent` | Track class context during AST walk | ✅ |
| Type aliases → `type` | `isPythonTypeAlias()` helper | ✅ |

### TypeScript Parser (`internal/symbols/parser.go`)

| Gap | Fix | Status |
|-----|-----|:------:|
| Arrow functions → `function` | Check `arrow_function` node type | ✅ |
| Class methods with `parent` | `extractTSClassMethods()` | ✅ |
| Abstract classes → `class` | Handle `abstract_class_declaration` | ✅ |

### Go Parser

**No gaps** - was already compliant.

---

## TYPE_COMPAT Final State

Only legitimate semantic equivalences remain:

```python
TYPE_COMPAT = {
    # Structural: Go struct ≈ class in other languages
    "class": {"class", "struct"},
    "struct": {"class", "struct"},
    
    # Abstract: Go interface ≈ Rust trait ≈ Python Protocol
    "interface": {"interface", "trait", "protocol"},
    "trait": {"interface", "trait", "protocol"},
    "protocol": {"interface", "trait", "protocol"},
}
```

No hacks. No workarounds. Clean.

---

## Adding New Languages

When adding a new language parser:

1. Create fixture: `fixtures/<lang>_test_fixture.<ext>`
2. Create spec: `specs/<lang>.upts.json`
3. Run: `python runners/upts_runner.py validate --spec specs/<lang>.upts.json --parser ./parser`
4. Fix any gaps until 100% pass
5. Document fixes here

---

## Verification

```bash
# Run all specs
python runners/upts_runner.py validate-all --spec-dir specs/ --parser ./parser

# Expected output:
# Go: 17/17 (100%) PASS
# Python: 35/35 (100%) PASS  
# TypeScript: 13/13 (100%) PASS
```
