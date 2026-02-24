# UATS Hash Verification

SHA256 hash verification ensures UATS spec file integrity. Each spec stores a hash of its own content in `config.sha256`. The runner verifies this hash on load, rejecting tampered specs before test execution.

## How It Works

### Hash Computation

1. Deep-copy the spec JSON
2. Remove `config.sha256` (the hash field itself is excluded)
3. Normalize: `json.dumps(spec, sort_keys=True, separators=(',', ':'))` — sorted keys, compact separators
4. Compute `SHA256(normalized_json.encode('utf-8'))`
5. Return hex digest

Sorted keys and consistent separators ensure deterministic hashing regardless of field ordering in the source file.

### Storage

The hash is stored inside the spec file at `config.sha256`:

```json
{
  "config": {
    "timeout_ms": 15000,
    "sha256": "be9d7c8eb58eebec060c2984ca76a853535813d5ac31909172b26c498da1b667"
  }
}
```

## Commands

### `add-hashes` — Generate or Update Hashes

```bash
python3 runners/uats_runner.py add-hashes --spec-dir specs/
```

For each spec file:

1. Compute hash (excluding existing `config.sha256`)
2. If unchanged: print `Hash unchanged: {hash[:12]}...`
3. If new/changed: write updated hash to file, print `Added`/`Updated`

Exit code 1 if any files fail to process.

### `verify-hashes` — Standalone Verification

```bash
python3 runners/uats_runner.py verify-hashes --spec-dir specs/
```

For each spec file:

- `✓` — Hash matches
- `-` — No hash present (not an error)
- `✗` — Hash mismatch (prints expected vs actual)

Exit code 1 only on **mismatches** (missing hashes are not failures).

### Auto-Verification on Test Run

During `validate` and `validate-all`, the runner automatically verifies hashes before executing tests:

- Hash valid or missing: proceed with test execution
- Hash mismatch: return ERROR result immediately, skip test execution
- `--skip-hash` flag: bypass verification entirely

## Workflow

### After Modifying a Spec

```bash
# 1. Edit the spec file
vim specs/ingest.uats.json

# 2. Regenerate hash
python3 runners/uats_runner.py add-hashes --spec-dir specs/

# 3. Commit both the spec change and updated hash
git add specs/ingest.uats.json
git commit -m "feat: add new ingest test variant"
```

### CI Integration

```bash
# Verify no specs were modified without updating hashes
python3 runners/uats_runner.py verify-hashes --spec-dir specs/
# Exit code 1 = spec was changed but hash wasn't regenerated
```

## Comparison with UPTS

UATS hash verification mirrors the UPTS pattern:

| Aspect | UATS (API specs) | UPTS (parser fixtures) |
|--------|------------------|------------------------|
| Hash location | `config.sha256` | `fixture.sha256` |
| Scope | Entire spec content | Fixture file content |
| Purpose | Detect unintended spec changes | Detect unintended fixture changes |
| Commands | `add-hashes`, `verify-hashes` | `add-hashes`, `verify-hashes` |

## Related Files

| File | Description |
|------|-------------|
| `runners/uats_runner.py` | Hash computation, add/verify commands, auto-verification on load |
| `specs/*.uats.json` | Spec files with `config.sha256` field |
