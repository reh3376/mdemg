# UPTS Changelog

## [1.5.0] - 2026-02-07

### Added

- **Scraper Markdown Parser**: `scraper_markdown_parser.go` — delegates symbol extraction to `MarkdownParser.ExtractSymbols()` for UPTS validation of the web scraper's symbol extraction path
- **Scraper Markdown UPTS Spec**: `scraper_markdown.upts.json` — 24 expected symbols (headings, code blocks, links) from a web-scraped documentation fixture
- **Lua Parser**: `lua_parser.go` — functions, local variables, module tables, metatables

### Changed

- **MarkdownParser.ExtractSymbols()**: Exported (was `extractSymbols`) so `internal/scraper` can reuse the UPTS-validated extraction

### Status After v1.5

| Category | Passing | Total |
|----------|---------|-------|
| All parsers | 27 | 27 |

---

## [1.2.1] - 2026-02-05

### Fixed

- **Cypher parser**: Changed symbol types from generic ("class", "constant") to Cypher-specific ("label", "relationship_type", "constraint", "index")
- **Cypher parser**: Fixed relationship regex to handle properties `{...}` and path patterns `*1..3`
- **Dockerfile parser**: Removed `ARG:` and `ENV:` prefixes from symbol names (now just variable names)
- **Dockerfile parser**: Added VOLUME instruction extraction with `VOLUME:` prefix
- **Dockerfile parser**: Added doc_comment to stage symbols showing FROM image
- **JSON parser**: Fixed brace depth tracking - sibling sections no longer incorrectly nested
- **SQL parser**: Complete rewrite - now extracts tables, columns, indexes, views, functions, triggers, enums, sequences with proper line numbers
- **SQL parser**: Added PRIMARY KEY to doc_comment for column symbols

### Spec Bug Fixes

- **c.upts.json**: Fixed function names (were showing parameter names from auto-generation bug)
- **cpp.upts.json**: Removed duplicate method+function entries, corrected symbol names
- **cuda.upts.json**: Fixed shared variable exported flag (true → false)
- **python.upts.json**: Fixed string value escaping (`"\"1.0.0\""` → `"1.0.0"`)

### Status After v1.2.1

| Category | Passing | Total |
|----------|---------|-------|
| All parsers | 20 | 20 |

---

## [1.2.0] - 2026-02-05

### Added

- **4 New Language Parsers + UPTS Specs**: C# (.cs), Kotlin (.kt, .kts), Terraform/HCL (.tf, .tfvars), Makefile (.mk)
- **Evidence Validation**: `validate_evidence` config flag enables structural consistency checks (LineEnd, CodeElement ranges, symbol containment, LineEnd matching)
- **Go-native test harness**: `validateEvidence()` function with 4 checks, wired into `TestUPTS`

### Fixed

- **Makefile parser**: `:=` variable assignments were incorrectly rejected by the target/variable disambiguation logic

### Status After v1.2

| Category | Passing | Total |
|----------|---------|-------|
| All parsers | 20 | 20 |

---

## [1.1.0] - 2026-01-29

### Fixed

#### Fixture Path Resolution

- **Issue:** Spec files referenced fixtures with paths relative to `upts/` root
- **Fix:** Changed all fixture paths to be relative to spec file location
- **Before:** `"path": "fixtures/go_test_fixture.go"`
- **After:** `"path": "../fixtures/go_test_fixture.go"`
- **Affected:** All 16 spec files in `specs/`

#### Runner: Parser Command Parsing

- **Issue:** Parser commands with spaces/quotes failed
- **Fix:** Use `shlex.split()` for proper shell-style parsing
- **Before:** `subprocess.run([self.parser_cmd, str(file_path)], ...)`
- **After:** `subprocess.run(shlex.split(self.parser_cmd) + [str(file_path)], ...)`

#### Runner: Config-Aware Validation

- **Issue:** Validation flags in config were ignored
- **Fix:** Read and apply `validate_signature`, `validate_value`, `validate_parent` from spec config
- **Impact:** Parent validation now skippable via `"validate_parent": false`

### Status After v1.1

| Category | Passing | Total |
|----------|---------|-------|
| Tree-sitter | 7 | 8 |
| Config | 4 | 8 |
| **Total** | **11** | **16** |

---

## [1.0.0] - 2026-01-29

### Added

- Initial UPTS schema and runner
- Specs for 16 languages: Go, Python, TypeScript, Rust, C, C++, CUDA, Java, YAML, TOML, JSON, INI, Shell, Dockerfile, SQL, Cypher
- Test fixtures for all 16 languages
- Fallback parsers for config languages
- Documentation: README, PARSER_ROADMAP, PARSER_GAPS, PARSER_FIX_GUIDE, INCONSISTENCIES_REPORT

### Passing Languages (v1.0)

- Go (100%)
- Python (100%)
- TypeScript (100%)
