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

* [ ] **Secret Audit**: Git history scanned with `trufflehog` or `gitleaks`.
* [ ] **License Audit**: All `go.mod` dependencies verified for MIT compatibility.
* [ ] **Static Analysis**: `gosec` (Go Security) passes with zero "High" severity issues.
* [ ] **Error Sanitization**: API responses verified to contain no raw Neo4j stack traces.

### 4.2 Portability & Developer Experience

* [ ] **Path Independence**: Zero instances of hardcoded home directory paths (e.g., `/Users/...`).
* [ ] **Dependency Isolation**: `start-mdemg.sh` verified to run on a fresh machine with only Docker and Go installed.
* [ ] **Sidecar SDK**: `CONTRIBUTING_SIDEBARS.md` provides clear proto definitions and a "Hello World" module example.

### 4.3 Reliability & Performance

* [ ] **Regression Suite**: Retrieval golden tests pass with `v10` baseline scores.
* [ ] **CI Pipeline**: PRs are blocked until `go test ./...` and `golangci-lint` pass.
* [ ] **Documentation Integrity**: Every `/v1` endpoint is documented with example request/response payloads.

---

## 5. Maintenance & Evolution

Post-launch, the repository will be maintained via a **"Main-is-Durable"** policy, where all feature work (including module development) occurs in feature branches and requires a passing integration test suite before merging.
