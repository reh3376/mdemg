# Parser Implementation Issues

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Generated:** 2026-01-29
**Last Updated:** 2026-02-05
**Status:** All Issues Resolved - 20/20 UPTS-validated parsers passing

---

## Summary

| Language | Status | Match Rate | Category |
|----------|--------|------------|----------|
| Go | **PASS** | 100% | Regex |
| Rust | **PASS** | 100% | Regex |
| Python | **PASS** | 100% | Regex |
| TypeScript | **PASS** | 100% | Regex |
| Java | **PASS** | 100% | Regex |
| C# | **PASS** | 100% | Regex |
| Kotlin | **PASS** | 100% | Regex |
| C++ | **PASS** | 100% | Regex |
| C | **PASS** | 100% | Regex |
| CUDA | **PASS** | 100% | Regex |
| SQL | **PASS** | 100% | Regex |
| Cypher | **PASS** | 100% | Regex |
| Terraform | **PASS** | 100% | Regex |
| YAML | **PASS** | 100% | Regex |
| TOML | **PASS** | 100% | Regex |
| JSON | **PASS** | 100% | Regex |
| INI | **PASS** | 100% | Regex |
| Makefile | **PASS** | 100% | Regex |
| Dockerfile | **PASS** | 100% | Regex |
| Shell | **PASS** | 100% | Regex |

**Current Pass Rate:** 100% (20/20 UPTS-validated languages)
**Additional parsers without UPTS specs:** Markdown, XML (total: 22 parsers)

---

## Resolved Issues (2026-02-05)

### Parser Fixes Applied

All parsers now use regex-based extraction in `cmd/ingest-codebase/languages/`. Major fixes applied:

| Parser | Issue | Resolution |
|--------|-------|------------|
| Cypher | Type mismatch ("class" vs "label") | Changed parser to emit correct types: `label`, `relationship_type`, `constraint`, `index` |
| Cypher | Relationship regex missed properties `{...}` | Updated regex to allow content between type and `]` |
| Dockerfile | ARG/ENV had "ARG:" prefix in name | Removed prefix from symbol names |
| Dockerfile | Missing VOLUME extraction | Added VOLUME parsing with `VOLUME:` prefix |
| JSON | Sibling sections incorrectly nested | Fixed brace depth tracking to correctly pop sections |
| SQL | Only extracted columns, no tables/indexes/etc | Complete rewrite to extract tables, columns, indexes, views, functions, triggers, enums, sequences with line numbers |
| SQL | Missing line numbers on symbols | Added line-by-line scanning with proper lineNum tracking |
| C | Typedef struct pointer parsing | Added patterns for `typedef struct X* Y;` and `typedef struct X X;` |
| C | Enum value extraction | Fixed pattern to not require trailing punctuation |
| C++ | Inline method parent tracking | Moved brace depth update to END of loop |
| C++ | `std::` return types filtered | Removed incorrect `::` check |
| CUDA | Negative lookahead panic | Replaced `(?!...)` with code-based filtering |
| CUDA | Multi-line kernel declarations | Changed pattern to not require closing paren on same line |
| Python | String value escaping | Fixed `"\"1.0.0\""` to `"1.0.0"` in spec |

### Spec Fixes Applied

Some specs had genuine bugs (semantic impossibilities, incorrect auto-generation):

| Spec | Issue | Resolution |
|------|-------|------------|
| c.upts.json | Function names were parameter names | Fixed: `user` → `user_create`, etc. |
| cpp.upts.json | Duplicate method+function entries | Removed duplicates, corrected names |
| cuda.upts.json | Wrong shared variable exported flag | Changed from `true` to `false` |
| python.upts.json | String value had escaped quotes | Changed `"\"1.0.0\""` to `"1.0.0"` |

---

## Verification

```bash
# Run all UPTS tests (Go-native harness)
go test ./cmd/ingest-codebase/languages/ -run TestUPTS -v

# Expected output:
# --- PASS: TestUPTS (0.02s)
#     --- PASS: TestUPTS/c (0.00s)
#     --- PASS: TestUPTS/cpp (0.00s)
#     ... (20 languages)
# PASS
```

---

## All Issues Resolved

All previously documented issues have been fixed. The parser suite now achieves:

- **20/20 UPTS-validated languages passing (100%)**
- **22 total parsers (including Markdown, XML without UPTS specs)**
