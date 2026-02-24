# MDEMG Parser Roadmap

**Created:** 2026-01-29  
**Status:** Planning  
**Completed:** Go ✅, Python ✅, TypeScript ✅

---

## Overview

15 additional parsers to build, organized by priority and complexity.

```
Phase 1 (Config Truth)     Phase 2 (Systems)      Phase 3 (Data/Docs)
├── YAML ⭐               ├── Rust               ├── SQL
├── TOML                   ├── Java               ├── Cypher
├── JSON/JSONC             ├── C                  ├── Markdown
├── INI/dotenv             ├── C++                └── XML
├── Dockerfile             └── CUDA
└── Shell
```

---

## Phase 1: Config Truth (High Leverage, Low Complexity)

These define runtime behavior and defaults. Quick wins with high evidence value.

### 1.1 YAML ⭐ (Priority: Critical)

**Why first:** GitHub Actions, K8s, docker-compose, Spring configs. Config truth lives here.

| Aspect | Details |
|--------|---------|
| Extensions | `.yml`, `.yaml` |
| Symbols | sections, keys, anchors, aliases |
| Patterns | P1 (values), custom (anchors) |
| Complexity | Medium (anchor/alias resolution) |
| Est. effort | 2-3 hours |

**Key extractions:**

- Top-level keys as symbols
- Nested paths flattened (`jobs.build.steps[0].run`)
- Anchors (`&name`) and aliases (`*name`) tracked
- Named blocks (K8s `kind`, GHA `jobs`, compose `services`)

---

### 1.2 TOML

**Why:** `Cargo.toml`, `pyproject.toml` - defines project metadata and dependencies.

| Aspect | Details |
|--------|---------|
| Extensions | `.toml` |
| Symbols | tables, keys, array-of-tables |
| Patterns | P1 (values) |
| Complexity | Low |
| Est. effort | 1-2 hours |

**Key extractions:**

- `[section]` as symbols
- `[[array.of.tables]]` as repeating symbols
- Key/value pairs with types

---

### 1.3 JSON / JSONC

**Why:** `package.json`, `tsconfig.json`, VS Code configs. Already structured.

| Aspect | Details |
|--------|---------|
| Extensions | `.json`, `.jsonc`, `.json5` |
| Symbols | top-level keys, nested paths |
| Patterns | P1 (values) |
| Complexity | Low (JSONC needs comment stripping) |
| Est. effort | 1-2 hours |

**Key extractions:**

- Flattened key paths
- Schema-aware parsing for known files (`tsconfig`, `package.json`)
- Preserve line numbers for evidence

---

### 1.4 INI / dotenv / properties

**Why:** Environment variables, Java properties. Direct config truth.

| Aspect | Details |
|--------|---------|
| Extensions | `.env`, `.ini`, `.cfg`, `.properties` |
| Symbols | keys, sections (INI) |
| Patterns | P1 (values) |
| Complexity | Very low |
| Est. effort | 1 hour |

**Key extractions:**

- Key/value pairs
- Section headers (INI)
- Comments as doc_comment

---

### 1.5 Dockerfile

**Why:** Defines build and runtime behavior. High evidence value.

| Aspect | Details |
|--------|---------|
| Extensions | `Dockerfile`, `Dockerfile.*`, `*.dockerfile` |
| Symbols | stages, instructions, args, env |
| Patterns | P1 (ARG/ENV), P2 (stages) |
| Complexity | Low-Medium |
| Est. effort | 2 hours |

**Key extractions:**

- `FROM` → base image symbols
- `ARG`/`ENV` → constants with defaults
- `EXPOSE`, `ENTRYPOINT`, `CMD` → runtime config
- Multi-stage build stages as symbols

---

### 1.6 Shell (Bash/Sh/Zsh)

**Why:** CI scripts, build scripts. Often the "real" behavior.

| Aspect | Details |
|--------|---------|
| Extensions | `.sh`, `.bash`, `.zsh` |
| Symbols | functions, exported vars, sourced files |
| Patterns | P1 (exports), P2 (functions) |
| Complexity | Medium (variable expansion) |
| Est. effort | 2-3 hours |

**Key extractions:**

- `function name()` or `name()` → functions
- `export VAR=value` → constants
- `source`/`.` includes
- Key command invocations

---

## Phase 2: Systems Languages (High Complexity, Core Codebases)

### 2.1 Rust

| Aspect | Details |
|--------|---------|
| Extensions | `.rs` |
| Symbols | mod, struct, enum, trait, impl, fn, macro |
| Patterns | All 7 |
| Complexity | High (macros, traits, impl blocks) |
| Est. effort | 4-6 hours |

**Key extractions:**

- `mod` declarations and `use` graph
- `trait` → interface, `impl Trait for Type`
- `pub` visibility for export detection
- `#[derive]`, `#[cfg]` attributes
- Macro definitions (`macro_rules!`)

---

### 2.2 Java

| Aspect | Details |
|--------|---------|
| Extensions | `.java` |
| Symbols | package, class, interface, enum, method, field |
| Patterns | All 7 |
| Complexity | Medium-High |
| Est. effort | 3-4 hours |

**Key extractions:**

- Package declarations
- Class/interface/enum with inheritance
- Annotations (`@Override`, `@Autowired`)
- Method signatures with generics
- Static fields as constants

---

### 2.3 C

| Aspect | Details |
|--------|---------|
| Extensions | `.c`, `.h` |
| Symbols | function, struct, typedef, macro, enum |
| Patterns | P1-P4, P5, P7 |
| Complexity | Medium (macros, preprocessor) |
| Est. effort | 3-4 hours |

**Key extractions:**

- Function declarations/definitions
- `struct`, `union`, `typedef`
- `#define` macros (constants and function-like)
- `enum` definitions
- Header include graph

---

### 2.4 C++

| Aspect | Details |
|--------|---------|
| Extensions | `.cpp`, `.hpp`, `.cc`, `.hh`, `.cxx`, `.hxx`, `.inl` |
| Symbols | class, struct, namespace, template, method |
| Patterns | All 7 |
| Complexity | Very High (templates, overloads) |
| Est. effort | 5-7 hours |

**Key extractions:**

- Everything from C plus:
- `class` with inheritance
- `namespace` hierarchy
- Templates (class and function)
- `constexpr`, `inline` constants
- Operator overloads

---

### 2.5 CUDA

| Aspect | Details |
|--------|---------|
| Extensions | `.cu`, `.cuh` |
| Symbols | kernel, device_function, constant, shared |
| Patterns | P1-P3, custom (kernels) |
| Complexity | High (builds on C++) |
| Est. effort | 3-4 hours (after C++) |

**Key extractions:**

- `__global__` kernels
- `__device__` functions
- `__constant__`, `__shared__` memory
- Kernel launch configurations
- `#if __CUDA_ARCH__` guards

---

## Phase 3: Data & Documentation

### 3.1 SQL

| Aspect | Details |
|--------|---------|
| Extensions | `.sql` |
| Symbols | table, column, index, view, function, trigger |
| Patterns | P1 (defaults), P2 (functions), P3 (tables) |
| Complexity | Medium (dialect variations) |
| Est. effort | 3-4 hours |

**Key extractions:**

- `CREATE TABLE` with columns and constraints
- `DEFAULT` values (critical for evidence)
- Indexes, views, triggers
- Migration version ordering

---

### 3.2 Cypher

| Aspect | Details |
|--------|---------|
| Extensions | `.cypher`, `.cql` |
| Symbols | label, relationship_type, constraint, index |
| Patterns | Custom |
| Complexity | Low-Medium |
| Est. effort | 2-3 hours |

**Key extractions:**

- Node labels from `CREATE`/`MATCH`
- Relationship types
- Constraints and indexes
- Property keys and patterns

**Note:** High strategic value - makes MDEMG introspectable.

---

### 3.3 Markdown

| Aspect | Details |
|--------|---------|
| Extensions | `.md`, `.mdx` |
| Symbols | heading, code_block, link |
| Patterns | Custom (sections) |
| Complexity | Low |
| Est. effort | 1-2 hours |

**Key extractions:**

- Headings as hierarchical symbols
- Code fences with language tags
- Links between documents
- Frontmatter (YAML) metadata

---

### 3.4 XML

| Aspect | Details |
|--------|---------|
| Extensions | `.xml`, `.pom`, `.csproj` |
| Symbols | element paths, attributes |
| Patterns | P1 (values) |
| Complexity | Medium |
| Est. effort | 2-3 hours |

**Key extractions:**

- Element paths flattened
- Key attributes as symbol metadata
- Namespace awareness
- Schema-specific parsing (Maven POM, MSBuild)

---

## Effort Summary

| Phase | Languages | Est. Hours | Cumulative |
|-------|-----------|------------|------------|
| 1 | YAML, TOML, JSON, INI, Dockerfile, Shell | 10-14 | 10-14 |
| 2 | Rust, Java, C, C++, CUDA | 18-25 | 28-39 |
| 3 | SQL, Cypher, Markdown, XML | 8-12 | 36-51 |

**Total estimate:** 36-51 hours of implementation

---

## Recommended Order

```
Week 1: Config parsers (high leverage, quick wins)
  1. JSON/JSONC (easiest, validates infra)
  2. YAML (critical for GHA, K8s)
  3. TOML (Cargo.toml, pyproject.toml)
  4. INI/dotenv (trivial)
  5. Dockerfile
  6. Shell

Week 2: Systems languages
  7. C (foundation for C++/CUDA)
  8. C++ (builds on C)
  9. CUDA (builds on C++)
  10. Rust
  11. Java

Week 3: Data & docs
  12. SQL
  13. Cypher (strategic for MDEMG)
  14. Markdown
  15. XML
```

---

## Next Steps

1. Generate UPTS specs for each language
2. Create fixture templates following canonical patterns
3. Implement Phase 1 parsers
4. CI integration with `make test-parsers`
