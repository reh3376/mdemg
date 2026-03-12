## Go

**Variants**

- `go.mod`, `go.sum`

- `*_test.go`

- `go:generate` directives

**Additional considerations**

- Extract: package/import graph, exported symbols, interfaces, struct tags, build tags

- Treat `internal/` and `cmd/` structure as semantic signals

- Capture defaults from `flag.*` and `cobra/viper` config patterns

- Evidence: file:line for struct fields + constants is high-value

---

## Rust

**Variants**

- `Cargo.toml`, `Cargo.lock`

- feature flags (`cfg`, `#[cfg(feature="...")]`)

- workspace layouts

**Additional considerations**

- Extract: modules (`mod`), `use` graph, traits/impls, macros, `pub` API

- Capture default behaviors: `Default::default()`, `clap` args, `serde(default)`

- Pay attention to `unsafe` blocks and FFI boundaries

- Handle `build.rs` as build-time behavior

---

## Java

**Variants**

- Gradle (`build.gradle`, `build.gradle.kts`)

- Maven (`pom.xml`)

- Spring configs (`application.yml`, `application.properties`)

- Kotlin interop in mixed repos

**Additional considerations**

- Extract: packages, classes/methods, annotations, dependency injection wiring

- Identify entrypoints: `main`, Spring bootstraps

- Config defaults often live in annotations + `application.*`

- Keep an eye on generated sources (`target/`, `build/`) — usually exclude

---

## JavaScript / TypeScript

**Variants**

- `.js`, `.mjs`, `.cjs`

- `.ts`, `.tsx`, `.d.ts`

- `package.json`, `pnpm-lock.yaml`, `yarn.lock`

- `tsconfig.json`, `eslint/prettier` configs

**Additional considerations**

- Extract: import/export graph, public API surfaces, config defaults

- Resolve path aliases (`tsconfig paths`)

- Don’t ingest `node_modules/`, `dist/`, `build/`

- Framework hooks: Next.js/React/Vite conventions matter

---

## Python

**Variants**

- `pyproject.toml`, `requirements.txt`, `Pipfile`, `setup.py`

- notebooks (`.ipynb`) if you choose

- type stubs (`.pyi`)

**Additional considerations**

- Extract: modules/import graph, classes/functions, dataclasses, type hints

- Capture configuration sources: env vars, `pydantic`, `argparse`, `click/typer`

- Be careful with “dynamic” patterns (monkeypatching, reflection)

- Exclude venvs, site-packages, `__pycache__`

---

## C

**Variants**

- headers: `.h`

- build: Make, CMake, Autotools

- platform-specific files (`_linux.c`, `_win32.c`)

**Additional considerations**

- Extract: functions, structs, macros, compile flags that change behavior

- Header inclusion graph is core architecture

- Treat macros as first-class “symbols” (they drive defaults)

- Exclude generated files unless they’re required evidence

---

## C++

**Variants**

- headers: `.hpp`, `.hh`, `.hxx`, `.inl`

- templates-heavy codebases

- build: CMake, Bazel

**Additional considerations**

- Extract: classes, templates, namespaces, overload sets

- Pay attention to `constexpr`, `inline`, and compile-time constants

- Dependency graph is often hidden in headers

- Large repos: consider symbol-level ingestion instead of snippet-level everywhere

---

## CUDA

**Variants**

- `.cu`, `.cuh`

- `nvcc` flags in build files (CMake/Bazel)

- mixed with C++ templates and macros

**Additional considerations**

- Extract: kernel definitions (`__global__`), launches (`<<<>>>`), device funcs

- Capture constants controlling behavior (block sizes, shared memory)

- Include compute capability guards (`#if __CUDA_ARCH__`)

- Treat build configuration as part of the “truth”

---

## JSON (and variants)

**Variants**

- JSONC (comments) – common in `tsconfig`, VS Code configs

- JSON5

- `package.json`, `tsconfig.json`, `settings.json`, `launch.json`

**Additional considerations**

- Normalize + flatten key paths (`a.b[0].c`) for retrieval

- Preserve source locations if possible (line mapping for evidence)

- Identify schema-bearing files (`OpenAPI`, `tsconfig`, etc.)

- Watch for huge lockfiles; ingest selectively or summarize

---

## Markdown

**Variants**

- GitHub-flavored MD

- MDX (`.mdx`)

- docs tooling: MkDocs, Docusaurus

**Additional considerations**

- Extract: headings as structure, code fences as embedded language blocks

- Capture links between docs (`[[wikilinks]]`, relative paths)

- Prefer “section nodes” over whole-file embeddings for precision

---

## XML (and variants)

**Variants**

- Maven POM (`pom.xml`)

- Android XML

- `.csproj` (technically XML)

- SVG (often huge)

**Additional considerations**

- Flatten element paths + key attributes

- Extract build/dependency semantics (Maven, MSBuild)

- Consider excluding large SVG unless needed

- Preserve namespaces; they matter

---

## SQL

**Variants**

- PostgreSQL, MySQL, SQLite, MSSQL, BigQuery dialect differences

- migration frameworks: Flyway/Liquibase

**Additional considerations**

- Extract: tables, columns, constraints, indexes, views, triggers

- Identify migration order + versioning

- Capture default values and constraints (your benchmark gold)

- SQL embedded in code: consider separate “SQL snippet” extraction pass later

---

## Cypher (recommended add)

**Variants**

- `.cypher` migrations

- Cypher embedded in app code strings

**Additional considerations**

- Extract: node labels, rel types, constraints/index creation, match patterns

- Capture parameter names + expected shapes

- Super aligned with MDEMG/Neo4j; makes your own system more introspectable

---

## YAML (add ASAP)

**Variants**

- `.yml`, `.yaml`

- GitHub Actions workflows

- K8s manifests, Helm values

- docker-compose YAML

**Additional considerations**

- Flatten key paths + extract “named blocks” (jobs/steps/kinds/services)

- Handle anchors/aliases (they create hidden coupling)

- YAML is config truth; defaults live here

---

## TOML

**Variants**

- `Cargo.toml`, `pyproject.toml`

- tool configs (`ruff.toml`, `taplo.toml`)

**Additional considerations**

- Extract sections + key/value defaults

- Treat tables/arrays-of-tables as “units”

- Great for cross-file default provenance

---

## Shell (Bash/Sh/Zsh)

**Variants**

- `.sh`, `.bash`, `.zsh`

- CI scripts, release scripts

**Additional considerations**

- Extract: env var exports, command invocations, key conditionals

- Shell often defines the real build/run behavior

- Evidence: show the exact line that sets env/config

---

## Dockerfile

**Variants**

- multistage builds

- `docker-compose.yml` (YAML but paired)

**Additional considerations**

- Extract: base images, build args, env vars, entrypoint/cmd, exposed ports

- These define runtime behavior and are great for evidence-weighted questions

---

## INI / dotenv / properties (small but high leverage)

**Variants**

- `.env`, `.env.*`

- `.ini`, `.cfg`

- Java `.properties`

**Additional considerations**

- Flatten key/value pairs, preserve file:line

- Default values and environment overrides are frequently the “why”
