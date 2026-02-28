# MDEMG Makefile
# Build, test, and utility targets

# Dynamic port discovery: read .mdemg.port if available, fall back to 9999
BASE_URL ?= http://localhost:$(shell cat .mdemg.port 2>/dev/null || echo 9999)

# Export MDEMG_BASE_URL so UATS spec files can resolve ${MDEMG_BASE_URL}
# via the runner's env-var fallback when --base-url is not passed directly
export MDEMG_BASE_URL ?= $(BASE_URL)

.PHONY: all build build-cli build-parser test test-parsers verify-upts-schema verify-uxts-canonical clean test-ubts-smoke test-udts test-unts-report test-sidecar test-sidecar-unit test-sidecar-integration release-snapshot release-local

# Build-time version info
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -ldflags "-X mdemg/internal/cli.Version=$(VERSION) -X mdemg/internal/cli.Commit=$(COMMIT) -X mdemg/internal/cli.BuildDate=$(BUILD_DATE)"

# Default target
all: build

# Build all binaries (unified CLI + parser tools)
build: build-cli build-parser
	@echo "Build complete"

# Build the unified MDEMG CLI binary
build-cli:
	@echo "Building mdemg unified CLI..."
	@mkdir -p bin
	go build $(LDFLAGS) -o bin/mdemg ./cmd/mdemg
	@echo "Built bin/mdemg ($(VERSION))"

# Build the parser tools (legacy standalone binaries)
build-parser:
	@echo "Building extract-symbols..."
	@mkdir -p bin
	go build -o bin/extract-symbols ./cmd/extract-symbols
	@echo "Building ingest-codebase..."
	go build -o bin/ingest-codebase ./cmd/ingest-codebase

# Run all tests
test: test-parsers verify-uxts-canonical verify-uxts-drift
	@echo "All tests complete"

# Validate UPTS schema/spec enum parity
verify-upts-schema:
	@echo "Checking UPTS schema/spec parity..."
	python3 scripts/verify_upts_schema_parity.py

# Validate canonical UxTS specs/drafts split (UDTS + UVTS)
verify-uxts-canonical:
	@echo "Checking canonical UxTS specs..."
	python3 scripts/verify_uxts_canonical_specs.py

# Check for framework drift (spec counts, runner existence, fixtures, hashes)
verify-uxts-drift:
	@echo "Checking UxTS framework drift..."
	python3 scripts/verify_uxts_drift.py

# Run UPTS parser validation tests
# Validates language parsers against Universal Parser Test Specifications
test-parsers: build-parser verify-upts-schema
	@echo "Running UPTS parser validation..."
	python3 docs/lang-parser/lang-parse-spec/upts/runners/upts_runner.py validate-all \
		--spec-dir docs/lang-parser/lang-parse-spec/upts/specs/ \
		--parser "./bin/extract-symbols --json" \
		--report /tmp/parser-report.json
	@echo "Report saved to /tmp/parser-report.json"

# Validate single language parser
# Usage: make test-parser-go, test-parser-python, test-parser-typescript
test-parser-%: build-parser
	@echo "Validating $* parser..."
	python3 docs/lang-parser/lang-parse-spec/upts/runners/upts_runner.py validate \
		--spec docs/lang-parser/lang-parse-spec/upts/specs/$*.upts.json \
		--parser "./bin/extract-symbols --json"

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f /tmp/parser-report.json

# Install development dependencies
dev-setup:
	@echo "Setting up development environment..."
	go mod download
	@echo "Done"

# Run the MDEMG server via unified CLI
run: build-cli
	./bin/mdemg serve

# Help target
help:
	@echo "MDEMG Makefile targets:"
	@echo "  build          - Build all binaries (unified CLI + parsers)"
	@echo "  build-cli      - Build unified mdemg CLI binary"
	@echo "  build-parser   - Build standalone parser tools"
	@echo "  verify-upts-schema - Check UPTS schema/spec parity"
	@echo "  verify-uxts-canonical - Check canonical UDTS/UVTS specs vs drafts"
	@echo "  test           - Run all tests"
	@echo "  test-parsers   - Run UPTS parser validation (all languages)"
	@echo "  test-parser-X  - Run UPTS validation for language X (go, python, typescript)"
	@echo "  test-api       - Run all UATS API validation specs"
	@echo "  test-udts      - Run UDTS gRPC contract tests with Section 8A report"
	@echo "  test-ubts-smoke- Run UBTS smoke benchmark (quick, 10 requests)"
	@echo "  test-ubts-load - Run UBTS load benchmark (1000 requests)"
	@echo "  test-uots      - Run all UOTS observability contract specs"
	@echo "  test-unts-report- Generate UNTS Section 8A report from registry"
	@echo "  release-snapshot- Build release snapshot locally (no publish)"
	@echo "  release-local  - Build release locally (no publish, requires tag)"
	@echo "  clean          - Remove build artifacts"
	@echo "  dev-setup      - Install dependencies"
	@echo "  run            - Build and run MDEMG server"
# ============================================================
# Release Targets (requires goreleaser: brew install goreleaser)
# ============================================================

# Build a release snapshot locally (no publish, no tag required)
release-snapshot:
	goreleaser release --snapshot --clean

# Build a release locally (no publish, requires a tag)
release-local:
	goreleaser release --skip=publish --clean

# ============================================================
# UATS API Testing Targets
# ============================================================

.PHONY: test-api test-api-% test-smoke test-all test-uots uats-setup

# Run all UATS API validation tests (excludes optional modules that require explicit enablement)
test-api:
	@echo "Running UATS API validation..."
	@mkdir -p /tmp/uats-test-codebase
	@echo 'package main' > /tmp/uats-test-codebase/main.go
	python3 docs/api/api-spec/uats/runners/uats_runner.py validate-all \
		--spec-dir docs/api/api-spec/uats/specs/ \
		--base-url $(BASE_URL) \
		--exclude-tag unts \
		--report /tmp/api-report.json
	@echo "Report saved to /tmp/api-report.json"

# Validate single API endpoint
# Usage: make test-api-health, test-api-retrieve, test-api-ingest
test-api-%:
	@echo "Validating $* API..."
	@mkdir -p /tmp/uats-test-codebase
	@echo 'package main' > /tmp/uats-test-codebase/main.go
	python3 docs/api/api-spec/uats/runners/uats_runner.py validate \
		--spec docs/api/api-spec/uats/specs/$*.uats.json \
		--base-url $(BASE_URL)

# Smoke tests (health + readiness only)
test-smoke:
	@echo "Running smoke tests..."
	python3 docs/api/api-spec/uats/runners/uats_runner.py validate \
		--spec docs/api/api-spec/uats/specs/health.uats.json \
		--base-url $(BASE_URL)
	python3 docs/api/api-spec/uats/runners/uats_runner.py validate \
		--spec docs/api/api-spec/uats/specs/readiness.uats.json \
		--base-url $(BASE_URL)

# Run all tests (parsers + API)
test-all: test-parsers test-api
	@echo "All tests complete (UPTS + UATS)"

# Run all UOTS observability contract tests
test-uots:
	@echo "Running UOTS observability validation..."
	python3 docs/api/api-spec/uots/runners/uots_runner.py \
		--spec-dir docs/api/api-spec/uots/specs/ \
		--base-url $(BASE_URL) \
		--report /tmp/uots-report.json
	@echo "Report saved to /tmp/uots-report.json"

# Install UATS dependencies
uats-setup:
	pip install requests jsonpath-ng

# Test with custom base URL
# Usage: make test-api-remote BASE_URL=https://staging.example.com
test-api-remote:
	python3 docs/api/api-spec/uats/runners/uats_runner.py validate-all \
		--spec-dir docs/api/api-spec/uats/specs/ \
		--base-url $(BASE_URL) \
		--report /tmp/api-report.json

# ============================================================
# UBTS Benchmark Testing Targets
# ============================================================
.PHONY: test-ubts-smoke test-ubts-load

# Run UBTS smoke benchmark (quick check — 10 requests, single-threaded)
test-ubts-smoke:
	@echo "Running UBTS smoke benchmark..."
	python3 docs/tests/ubts/runners/ubts_runner.py \
		--spec "docs/tests/ubts/specs/retrieve_latency.ubts.json" \
		--profile docs/tests/ubts/profiles/smoke.profile.json \
		--base-url $(BASE_URL) \
		--report /tmp/ubts-report.json
	@echo "UBTS smoke benchmark complete"

# Run UBTS load benchmark (moderate load — 1000 requests, 10 concurrent)
test-ubts-load:
	@echo "Running UBTS load benchmark..."
	python3 docs/tests/ubts/runners/ubts_runner.py \
		--spec "docs/tests/ubts/specs/*.ubts.json" \
		--profile docs/tests/ubts/profiles/load.profile.json \
		--base-url $(BASE_URL) \
		--report /tmp/ubts-report.json
	@echo "UBTS load benchmark complete"

# ============================================================
# RSIC Testing Targets
# ============================================================
.PHONY: test-rsic test-rsic-unit test-rsic-integration test-rsic-uats

test-rsic: test-rsic-unit test-rsic-integration test-rsic-uats
	@echo "All RSIC tests complete"

test-rsic-unit:
	@echo "Running RSIC unit tests..."
	go test -v ./internal/ape/...

test-rsic-integration:
	@echo "Running RSIC integration tests..."
	go test -v -tags=integration ./tests/integration/... -run "TestRSIC_"

test-rsic-uats:
	@echo "Running RSIC UATS contract tests..."
	python3 docs/api/api-spec/uats/runners/uats_runner.py validate-all \
		--spec-dir docs/api/api-spec/uats/specs/ \
		--base-url $(BASE_URL) \
		--include-tag rsic

# ============================================================
# UNTS Testing Targets
# ============================================================
.PHONY: test-unts test-unts-uats test-unts-report test-udts

test-unts:
	@echo "Running UNTS unit tests..."
	go test -v ./internal/unts/...

test-unts-uats:
	@echo "Running UNTS UATS contract tests..."
	python3 docs/api/api-spec/uats/runners/uats_runner.py validate-all \
		--spec-dir docs/api/api-spec/uats/specs/ \
		--base-url $(BASE_URL) \
		--include-tag unts

# Generate UNTS Section 8A report from registry
test-unts-report:
	@echo "Generating UNTS Section 8A report..."
	python3 scripts/unts_report_adapter.py \
		--registry docs/specs/unts-registry.json \
		--report /tmp/unts-report.json
	@echo "Report saved to /tmp/unts-report.json"

# ============================================================
# Sidecar Testing Targets
# ============================================================

# Run all sidecar tests (unit + integration)
test-sidecar: test-sidecar-unit test-sidecar-integration

# Run sidecar unit tests (pure-logic helpers in internal/sidecar + internal/cli)
test-sidecar-unit:
	@echo "Running sidecar unit tests..."
	go test -v ./internal/sidecar/... ./internal/cli/... \
		-run "TestExtractPort|TestDoctorStatus|TestDoctorNext|TestRunConfig|TestGenerate|TestBuildHealth|TestIsEmpty"

# Run sidecar integration tests (binary-exec, requires built CLI)
test-sidecar-integration: build-cli
	@echo "Running sidecar integration tests..."
	MDEMG_BINARY=$(PWD)/bin/mdemg go test -v -tags=integration \
		./tests/integration/... -run "TestSidecar_" -timeout 120s

# ============================================================
# UDTS Contract Testing Targets
# ============================================================

# Run UDTS gRPC contract tests with Section 8A report
test-udts:
	@echo "Running UDTS gRPC contract tests..."
	python3 docs/api/api-spec/udts/runners/udts_runner.py \
		--report /tmp/udts-report.json
	@echo "Report saved to /tmp/udts-report.json"
