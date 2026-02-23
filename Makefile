# MDEMG Makefile
# Build, test, and utility targets

# Dynamic port discovery: read .mdemg.port if available, fall back to 9999
BASE_URL ?= http://localhost:$(shell cat .mdemg.port 2>/dev/null || echo 9999)

# Export MDEMG_BASE_URL so UATS spec files can resolve ${MDEMG_BASE_URL}
# via the runner's env-var fallback when --base-url is not passed directly
export MDEMG_BASE_URL ?= $(BASE_URL)

.PHONY: all build build-parser test test-parsers clean

# Default target
all: build

# Build all binaries
build: build-parser
	@echo "Build complete"

# Build the parser tools
build-parser:
	@echo "Building extract-symbols..."
	@mkdir -p bin
	go build -o bin/extract-symbols ./cmd/extract-symbols
	@echo "Building ingest-codebase..."
	go build -o bin/ingest-codebase ./cmd/ingest-codebase

# Run all tests
test: test-parsers
	@echo "All tests complete"

# Run UPTS parser validation tests
# Validates language parsers against Universal Parser Test Specifications
test-parsers: build-parser
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

# Run the MDEMG server
run:
	go run ./cmd/mdemg

# Help target
help:
	@echo "MDEMG Makefile targets:"
	@echo "  build          - Build all binaries"
	@echo "  build-parser   - Build ingest-codebase parser"
	@echo "  test           - Run all tests"
	@echo "  test-parsers   - Run UPTS parser validation (all languages)"
	@echo "  test-parser-X  - Run UPTS validation for language X (go, python, typescript)"
	@echo "  clean          - Remove build artifacts"
	@echo "  dev-setup      - Install dependencies"
	@echo "  run            - Run MDEMG server"
# ============================================================
# UATS API Testing Targets
# ============================================================

.PHONY: test-api test-api-% test-smoke test-all uats-setup

# Run all UATS API validation tests
test-api:
	@echo "Running UATS API validation (101 specs)..."
	@mkdir -p /tmp/uats-test-codebase
	@echo 'package main' > /tmp/uats-test-codebase/main.go
	python3 docs/api/api-spec/uats/runners/uats_runner.py validate-all \
		--spec-dir docs/api/api-spec/uats/specs/ \
		--base-url $(BASE_URL) \
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
.PHONY: test-unts test-unts-uats

test-unts:
	@echo "Running UNTS unit tests..."
	go test -v ./internal/unts/...

test-unts-uats:
	@echo "Running UNTS UATS contract tests..."
	python3 docs/api/api-spec/uats/runners/uats_runner.py validate-all \
		--spec-dir docs/api/api-spec/uats/specs/ \
		--base-url $(BASE_URL) \
		--include-tag unts
