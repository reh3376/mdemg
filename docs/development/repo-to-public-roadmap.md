# MDEMG Repo-to-Public Roadmap

## 1. Overview

This document outlines the strategic plan to transform the MDEMG repository from a private development environment into a secure, professional, and collaborative public open-source project.

## 2. Rationale for Changes

To successfully open-source MDEMG, we must address three critical pillars:

* **Security & Hardening**: Ensure no secrets, personal file paths, or internal database errors are exposed to the public.
* **Control & Governance**: Establish branch protection rules and contribution workflows to prevent the core "Main" branch from being compromised by unverified changes.
* **Modular Extensibility**: Formalize the "Binary Sidecar" architecture so that the community can contribute SME Modules (Linear, Obsidian, PLC, etc.) without needing to modify the core Go engine.

---

## 3. Transformation Roadmap

### Phase 1: Governance & Collaboration (The Community Layer)

* **PR & Issue Templates**: Implement structured templates to ensure high-quality bug reports and feature proposals.
* **CONTRIBUTING.md**: A comprehensive guide for external developers, with a heavy focus on building gRPC-based modules.
* **CODE_OF_CONDUCT.md & SECURITY.md**: Standard community and safety protocols.

### Phase 2: Security & Environment Hardening

* **Secret Scrubbing**: Audit the entire codebase and git history for hardcoded keys or personal config.
* **Path Normalization**: Standardize scripts (`start-mdemg.sh`, `docker-compose.yml`) to use relative paths and configurable environment variables.
* **Error Sanitization**: Refactor API handlers to return user-friendly, sanitized errors while logging detailed traces internally.

### Phase 3: Repository Restructuring

* **Standard Go Layout**: Move core logic to root, keeping a clean separation between `internal/` (private engine) and `pkg/` (importable client logic).
* **Documentation Consolidation**: Centralize technical specs, benchmarks, and research papers into a single, structured `/docs` hierarchy.
* **Cleanup**: Relocate internal development artifacts to a private internal folder.

### Phase 4: Continuous Integration (CI) Guards

* **GitHub Actions**: Automate linting and testing on every PR.
* **Integration CI**: Spawn a temporary Neo4j instance in CI to verify that PRs don't break the retrieval or learning logic.

### Phase 5: Public Onboarding

* **README.md Overhaul**: Rewrite the root README to lead with the **Modular Intelligence** vision, clear architecture diagrams, and a 3-step Docker quick-start.
* **Release Automation**: Implement Semantic Versioning (SemVer) and automated GitHub Releases for binary distribution.
* **License**: Add a standard MIT License.

---

## 4. Technical Readiness Checklist (Hardened Criteria)

This section defines the "Pass/Fail" criteria for the final public release.

### 4.1 Security & Compliance

* [x] **Secret Audit**: `gitleaks` runs in CI on every push and PR via `gitleaks/gitleaks-action@v2.3.9` in `.github/workflows/ci.yml` (`security` job). `trufflehog` is not used but is not required — `gitleaks` covers the same scope. Trivy (CRITICAL/HIGH, exit-code 1) also runs in the same job.
* [ ] **License Audit**: All `go.mod` dependencies have not yet been formally audited for MIT compatibility. The dependency list is small and well-known (neo4j driver, cobra, grpc, protobuf, etc.) but no automated tool (e.g., `go-licenses`) has been run or added to CI. **Needs completion.**
* [x] **Static Analysis**: `gosec` is enabled in `.golangci.yml` and runs as part of `golangci-lint` in the CI `lint` job. Suppressions are documented with rationale (G104, G115, G204, etc.). CI blocks PRs if lint fails.
* [x] **Error Sanitization**: `sanitizeError()` and `writeInternalError()` in `internal/api/server.go` (lines 1745–1761) log full errors internally and return only `"internal error during <operation>"` to clients. The pattern is used consistently across all handlers.
* [x] **Endpoint scope enforcement** — `RequireScope` middleware wired to 14 destructive endpoints (GAP-16)

### 4.2 Portability & Developer Experience

* [ ] **Path Independence**: Hardcoded `/Users/reh3376/...` paths appear as comments/examples in `internal/cli/synergy.go` (line 436) and `internal/api/handlers_synergy.go` (line 99) — these are in code comments only, not functional paths. However, `CLAUDE.md` at the repo root contains literal `/Users/reh3376/mdemg` paths in setup instructions. `docker-compose.yml` and `scripts/` are clean. The comment-only instances are low risk but should be replaced with placeholder examples (e.g., `/path/to/project`) before public release. **Needs completion.**
* [ ] **Dependency Isolation**: `start-mdemg.sh` does not exist. The project starts via `./bin/mdemg serve` or `./bin/mdemg start --auto-migrate`. There is no single entry-point script for fresh-machine onboarding. `scripts/install.sh` exists but is for package installation, not local development bootstrapping. **Needs completion** — create `start-mdemg.sh` or equivalent `Makefile` target.
* [ ] **Sidecar SDK**: `CONTRIBUTING_SIDEBARS.md` does not exist at the repo root or in `docs/`. The sidecar architecture is implemented (`internal/sidecar/`, `scripts/sidecar-acceptance.sh`, proto definitions), but no community-facing "Hello World" guide has been written. **Needs completion.**

### 4.3 Reliability & Performance

* [x] **Regression Suite**: `tests/integration/scoring_golden_test.go` implements a golden test graph with cosine similarity targets, activation scores, and baseline score assertions. Runs under the `integration` build tag in CI via `go test -v -tags=integration ./tests/integration/...`.
* [x] **Neo4j managed transactions** — 32 `session.Run()` migrated to `ExecuteRead`/`ExecuteWrite` with automatic retry
* [x] **CI Pipeline**: PRs to `main` are blocked by three required CI jobs: `build` (compile check), `test` (unit + integration + UATS contract tests with live Neo4j service container), and `lint` (`golangci-lint` via `golangci-lint-action`). All three must pass before merge.
* [x] **Documentation Integrity**: UATS contract tests in `docs/api/api-spec/uats/specs/` cover all active `/v1` endpoints with example request/response assertions. The UATS runner validates these against a live server in CI. 100+ spec files exist covering the full API surface.

---

## 5. Maintenance & Evolution

Post-launch, the repository will be maintained via a **"Main-is-Durable"** policy, where all feature work (including module development) occurs in feature branches and requires a passing integration test suite before merging.
