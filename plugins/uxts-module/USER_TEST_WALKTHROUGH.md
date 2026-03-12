# UxTS Module — User Test Walkthrough

Interactive testing guide. Work through each section sequentially.
Each step has a command to run and expected output to verify.

---

## Step 1: Pre-flight Checks

### 1a. Verify branch
```bash
git branch --show-current
```
**Expected:** `mdemg-dev01`

### 1b. Build the plugin
```bash
go build -o plugins/uxts-module/uxts-module ./plugins/uxts-module/
```
**Expected:** No output (clean build)

### 1c. Lint check
```bash
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./plugins/uxts-module/...
```
**Expected:** `0 issues.`

### 1d. Verify manifest
```bash
cat plugins/uxts-module/manifest.json | python3 -m json.tool > /dev/null && echo "Valid JSON"
```
**Expected:** `Valid JSON`

---

## Step 2: Unit Tests (35 tests)

Pure logic, no dependencies, fastest tier.

```bash
go test -v -count=1 ./plugins/uxts-module/... 2>&1 | grep -E "^(=== RUN|--- |PASS|FAIL|ok)"
```

**Verify:**
- [ ] All lines show `--- PASS`
- [ ] Final line: `ok  mdemg/plugins/uxts-module`
- [ ] No `FAIL` lines

---

## Step 3: Integration Tests (6 tests)

Full gRPC lifecycle via in-process Unix socket. No external services needed.

```bash
go test -v -count=1 -tags=integration -run "^TestIntegration" ./plugins/uxts-module/...
```

**Verify each test:**

### 3a. FullLifecycle
- [ ] `handshake from MDEMG 1.0.0` logged
- [ ] `loaded 1 specs across 1 frameworks` logged
- [ ] `processing 2 candidates` logged (matched + unmatched)
- [ ] `processing 0 candidates` logged (empty test)
- [ ] `shutdown requested` logged
- [ ] Test passes

### 3b. MultiFrameworkIndex
- [ ] `loaded 5 specs across 2 frameworks` logged (3 UATS + 2 UDTS)
- [ ] Test passes

### 3c. DriftDetection
- [ ] `loaded 2 specs across 1 frameworks` logged
- [ ] `drift_detected = 1` verified by test
- [ ] Test passes

### 3d. RealSpecFiles
- [ ] Log shows actual spec counts from your `docs/` directory
- [ ] Total specs > 200
- [ ] Frameworks >= 9
- [ ] Candidates annotated with `uxts_coverage_count`, `uxts_frameworks`
- [ ] Non-matching candidate shows `coverage=0`

### 3e. ConcurrentProcessing
- [ ] 10 concurrent `processing 1 candidates` lines logged
- [ ] `requests_handled = 10` verified by test
- [ ] No race conditions or panics

### 3f. ScoreAdjustment
- [ ] Covered candidate score > 0.50 (boost applied)
- [ ] Untested candidate score = 0.55 (unchanged)

---

## Step 4: E2E Tests (6 tests)

Validates binary, manifest, real spec files, drift detection, and performance.

```bash
go test -v -count=1 -tags=e2e -run "^TestE2E" ./plugins/uxts-module/...
```

**Verify each test:**

### 4a. ManifestValid
- [ ] Plugin ID = `uxts-module`
- [ ] Type = `REASONING`
- [ ] All required config keys present

### 4b. BinaryBuilds
- [ ] `Binary size: NNNNN bytes` logged (should be ~16-17 MB)
- [ ] Compiles successfully

### 4c. BinaryShowsHelp
- [ ] Exit code non-zero without `--socket` flag
- [ ] Error message mentions `--socket flag is required`

### 4d. RealSpecCount
- [ ] Verify framework counts logged:
  - UATS >= 100
  - UDTS >= 10
  - UPTS >= 20
  - UETS >= 5
  - Total >= 200

### 4e. DriftDetectionMatchesPython
- [ ] `Drift summary: NNN clean, NNN drifted, NNN without hash` logged
- [ ] Clean count > 0 (drift detection working)
- [ ] Not ALL specs drifted (sanity check)

### 4f. SpecIndexLookupPerformance
- [ ] `Lookups completed: 4000 path + 400 text` logged
- [ ] Completes in < 1 second

---

## Step 5: UDTS Contract Tests (5 tests)

Tests the plugin's own gRPC contract via spec files. Requires starting the binary.

### 5a. Start the plugin (Terminal 1)
```bash
mkdir -p /tmp/mdemg-plugins
./plugins/uxts-module/uxts-module --socket /tmp/mdemg-plugins/mdemg-uxts-module.sock
```
**Expected:** `uxts-module: listening on /tmp/mdemg-plugins/mdemg-uxts-module.sock`

### 5b. Run contract tests (Terminal 2 or background)
```bash
UDTS_MODULE_SOCKET=/tmp/mdemg-plugins/mdemg-uxts-module.sock \
  go test -v -count=1 ./tests/udts/... -run "^TestUxTS"
```

**Verify:**
- [ ] `TestUxTSModuleHandshake` — PASS
- [ ] `TestUxTSModuleHealthCheck` — PASS
- [ ] `TestUxTSModuleShutdown` — PASS
- [ ] `TestUxTSModuleProcessEmpty` — PASS
- [ ] `TestUxTSModuleProcessWithCandidates` — PASS

### 5c. Stop the plugin
Press Ctrl+C in Terminal 1 or the process will be killed by the Shutdown test.

---

## Step 6: Live Plugin Operation (optional, requires MDEMG server)

### 6a. Start MDEMG with plugins
```bash
./bin/mdemg serve --auto-migrate
```

### 6b. Verify plugin discovered
```bash
curl -s http://localhost:9999/v1/plugins | python3 -m json.tool
```
**Verify:** `uxts-module` appears in plugin list with status `running`

### 6c. Run 4-level validation
```bash
curl -s -X POST http://localhost:9999/v1/plugins/uxts-module/validate | python3 -m json.tool
```
**Verify:**
- [ ] `manifest.valid = true`
- [ ] `proto.implemented` includes `Handshake`, `HealthCheck`, `Shutdown`, `Process`
- [ ] `health.healthy = true`
- [ ] `lifecycle.success = true`

### 6d. Query retrieval and check annotations
```bash
curl -s -X POST http://localhost:9999/v1/memory/retrieve \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","query":"memory retrieval API","top_k":5}' | \
  python3 -c "
import json, sys
data = json.load(sys.stdin)
for r in data.get('results', []):
    meta = r.get('metadata', {})
    uxts = {k: v for k, v in meta.items() if k.startswith('uxts_')}
    if uxts:
        print(f\"  {r.get('name', 'unnamed')}: {uxts}\")
    else:
        print(f\"  {r.get('name', 'unnamed')}: (no uxts metadata)\")
"
```
**Verify:** At least some results have `uxts_coverage_count > 0`

---

## Step 7: Drift Verification

### 7a. Run Python drift checker
```bash
python3 scripts/verify_uxts_drift.py 2>&1
```
**Verify:** No UDTS-related failures (UATS count mismatch is pre-existing)

### 7b. Verify UDTS spec count
```bash
ls docs/api/api-spec/udts/specs/*.udts.json | wc -l
```
**Expected:** `12` (7 original + 5 uxts-module)

### 7c. Verify proto hash
```bash
shasum -a 256 api/proto/mdemg-module.proto | awk '{print $1}'
```
**Expected:** Matches `proto_sha256` in UDTS specs:
```bash
python3 -c "
import json, glob
for f in sorted(glob.glob('docs/api/api-spec/udts/specs/uxts_module_*.udts.json')):
    spec = json.load(open(f))
    print(f\"{spec['method']:20s} proto_sha256={spec['config']['proto_sha256'][:16]}...\")
"
```

---

## Summary Checklist

| Step | Tests | Status |
|------|-------|--------|
| 1. Pre-flight | Build + lint | [ ] |
| 2. Unit | 35 tests | [ ] |
| 3. Integration | 6 tests | [ ] |
| 4. E2E | 6 tests | [ ] |
| 5. UDTS Contract | 5 tests | [ ] |
| 6. Live Plugin | 4 checks | [ ] |
| 7. Drift Verification | 3 checks | [ ] |

**Total: 52 automated tests + 7 manual verification checks**
