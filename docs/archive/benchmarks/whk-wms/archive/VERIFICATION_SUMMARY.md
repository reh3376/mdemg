# Question Verification Summary Report

**Date:** 2026-01-21
**Total Questions Verified:** 380 (from 95 batches of 4 questions each)

---

## Overall Results

| Wave | Questions | Verified | Corrections | Correction Rate |
|------|-----------|----------|-------------|-----------------|
| 1-4 | 160 | ~129 | ~31 | ~19% |
| 5 | 40 | 15 | 25 | 63% |
| 6 | 40 | 13 | 27 | 68% |
| 7 | 40 | 27 | 13 | 33% |
| 8 | 40 | 34 | 6 | 15% |
| 9 | 40 | 32 | 8 | 20% |
| 10 | 20 | 6 | 14 | 70% |
| **Total** | **380** | **~256** | **~124** | **~33%** |

---

## Key Findings

### 1. High-Error Categories

Questions about these topics had the highest correction rates:

| Topic | Common Issues |
|-------|---------------|
| Error handling | Wrong filter names, incorrect response formats |
| Feature flags | Wrong channel names, missing streaming support claims |
| Authentication | Overstated capabilities (no role hierarchy, no server-side token refresh) |
| Audit system | Confused archive vs delete, wrong method names |
| Compliance reporting | Wrong output format support, incorrect section structure |

### 2. Common Error Patterns

1. **Wrong field/method names** (~35% of corrections)
   - Example: `getEntityHistory` vs actual `getEntityAuditHistory`
   - Example: `audit:created` vs actual `auditLogAdded`

2. **Overstated functionality** (~30% of corrections)
   - Claiming features that don't exist (PDF export, role hierarchy, distributed tracing)
   - Describing "cold storage archival" when code just deletes records

3. **Incorrect enum/constant values** (~20% of corrections)
   - Wrong resolution paths (ESCALATE doesn't exist)
   - Wrong error categories

4. **Generic patterns assumed** (~15% of corrections)
   - Assuming Redis pub/sub when in-memory is used
   - Assuming standard NestJS patterns without verification

---

## Wave 10 Detailed Corrections

### Batch 91

- **Q469**: HttpExceptionFilter is Prisma-specific, not general; response format is `{message, statusCode}` only
- **Q472**: No periodic polling for feature flags; streaming handled via LaunchDarkly SDK
- **Q473**: No ESCALATE resolution path exists; only ACCEPT, RESOLVE, ACKNOWLEDGE, REJECT
- **Q474**: Uses @nest-lab/throttler-storage-redis library, not custom Redis atomic ops

### Batch 92

- **Q476**: Channel is `auditLogAdded` not `audit:created`; uses in-memory PubSub not Redis
- **Q477**: Error handler includes detailed messages, not generic; AuditService sanitizes specific fields
- **Q479**: FeatureFlagKey is `keyof typeof featureFlagRegistry`, not direct union type
- **Q480**: Method is `getEntityAuditHistory`, not `getEntityHistory`

### Batch 93

- **Q483**: No `setUserId()` method exists; context set once via `run()` with AsyncLocalStorage

### Batch 94

- **Q490**: No PDF format support; sections are different from described
- **Q494**: Cleanup DELETES records, does not archive; no hash chain integrity

### Batch 95

- **Q495**: `ensureUniqueSerialNumber` queries Barrel table directly, not SerialRegistryService
- **Q498**: Wrong AuditTrackOptions fields; no `auditFields`, `sensitiveFields`
- **Q500**: No HttpExceptionsFilter; TimeoutInterceptor not global; auth flow differs

---

## Impact on Test Validity

### Original Question Quality Issues (from TEST_VALIDITY_ISSUES.md)

- Baseline claimed 92% accuracy
- MDEMG claimed 88% accuracy

### After Verification

- **124 of 380 questions (~33%) had incorrect answers**
- Combined with 19 corrections from initial 43 uncertain questions
- **Total: ~143 corrections out of 423 questions originally in verification scope**

### Corrected Accuracy Expectations

If the test questions themselves had ~33% answer errors:

- A model achieving 90%+ is likely matching known-wrong answers
- True accuracy on corrected questions would be lower

**Recommended Test Procedure:**

1. Use the corrected question set only
2. Re-run both baseline and MDEMG tests
3. Compare against verified ground truth

---

## Files Updated

| File | Description |
|------|-------------|
| `verification_batches.json` | 95 batches used for parallel verification |
| `question_corrections.json` | Initial 19 corrections from uncertain questions |
| `QUESTION_IMPROVEMENTS.md` | Improvement report for question quality |
| `TEST_VALIDITY_ISSUES.md` | Analysis of test methodology flaws |
| `VERIFICATION_SUMMARY.md` | This summary report |

---

## Recommendations

1. **Update question set** - Apply all 124+ corrections to `whk-wms-questions-final.json`
2. **Re-baseline** - Run tests with corrected answers
3. **Add citation requirement** - Answers should include file:line references
4. **Weight by difficulty** - Cross-cutting questions count more than enum lookups
5. **Adversarial testing** - Include questions requiring files NOT in typical read paths
