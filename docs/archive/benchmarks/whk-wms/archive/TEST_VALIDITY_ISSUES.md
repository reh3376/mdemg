# Test Results Validity Analysis

**Date:** 2026-01-21
**Analyst:** Claude Opus 4.5

## Executive Summary

**The reported test results appear significantly inflated due to fundamental methodology flaws.** The baseline score of 92% and MDEMG score of 88% are not credible given the codebase scale and question design.

---

## Codebase Scale Reality Check

### Actual Codebase Metrics

| Metric | Value |
|--------|-------|
| Total Lines of Code (TS/JS/Prisma) | **680,104** |
| Total Documentation (Markdown) | **120,897 lines** |
| TypeScript Files | **2,199** |
| Total Code/Doc Files | **3,354** |
| Repo Size | 196 MB |
| Prisma Schema | 2,899 lines |
| Largest File (generated.ts) | 14,681 lines |

### Token Coverage Analysis

**Claimed baseline token usage:** ~150,000 tokens

At ~4 characters per token:

- 150k tokens ≈ 600,000 characters
- At ~50 chars/line average ≈ **12,000 lines readable**
- Codebase has **680,000+ lines**
- **Actual coverage: ~1.8% of codebase**

**Conclusion:** The baseline test could have read at most 2% of the codebase. Achieving 92% accuracy with 2% coverage indicates the questions were designed to match what was readable, not to test actual codebase knowledge.

---

## Question Design Flaws

### Issue 1: Trivially Easy Questions

Many questions can be answered from minimal files:

| Question Type | Answerable From | % of Questions |
|---------------|-----------------|----------------|
| Framework/Library | package.json | ~30% |
| Model/Enum names | schema.prisma | ~25% |
| File paths | Directory structure | ~20% |
| Generic patterns | Common knowledge | ~15% |

**Examples of trivial questions:**

- "What ORM is used?" → Prisma (visible in any import)
- "What frontend framework?" → Next.js (visible in package.json)
- "What enum represents barrel disposition?" → EnumBarrelDisposition (self-evident name)

### Issue 2: Self-Answering Questions

Many questions contain their answers:

| Question | "Correct" Answer | Issue |
|----------|------------------|-------|
| "What enum represents barrel system status?" | EnumBarrelSystemStatus | Name is in the question |
| "What is EnumLotStatus Created?" | "Lot has been created" | Tautological |
| "What is financial direction OUT?" | "Money paid/outgoing" | Definition in name |

### Issue 3: Generic/Vague Answers

Many "deep technical" answers are framework defaults, not codebase-specific:

| Question | Answer | Issue |
|----------|--------|-------|
| "What handles GraphQL N+1?" | "DataLoader pattern" | Standard pattern, not verified |
| "What handles connection pooling?" | "Prisma connection pool" | Default behavior |
| "How are migrations managed?" | "Prisma migrations" | Obvious from ORM choice |
| "What handles introspection in prod?" | "Configurable disable" | Generic capability |

### Issue 4: No Verification of "Correct" Answers

The test provides answers without verification that they're actually correct for this codebase:

- No code citations
- No file path evidence
- Many answers are reasonable guesses, not confirmed facts

---

## Baseline Test Methodology Issues

### Problem 1: Impossible Coverage Claims

The baseline claims to have understood the codebase with "1 context compression" and "~150k tokens":

| What's Claimed | What's Possible | Gap |
|----------------|-----------------|-----|
| "Read 50+ files" | Could read ~20-30 medium files | 2x overclaim |
| "Strong implementation knowledge" | Read ~2% of code | 98% unknown |
| "Good domain comprehension" | Read docs only | No implementation verified |

### Problem 2: Cherry-Picked File Selection

Files that would be read first (and answer most questions):

1. `package.json` files (~600 lines) → Answers all framework questions
2. `schema.prisma` (~2,900 lines) → Answers all model/enum questions
3. `app.module.ts` → Answers architecture questions
4. `README.md` files → Answers domain questions

These 4 file types could answer 60%+ of the questions while using <5% of token budget.

### Problem 3: No Blind Testing

The person creating questions likely had access to:

- Which files would be read
- What information would be retained
- How to phrase questions to match available context

This creates circular validation, not independent testing.

---

## MDEMG Test Issues

### Issue 1: Retrieval ≠ Answer Accuracy

The MDEMG test measures "can the returned files answer the question" not "did the model answer correctly":

- A file containing the answer doesn't guarantee correct interpretation
- Multi-hop reasoning across files not tested
- Partial file matches counted as success

### Issue 2: Easy Path Questions Inflate Scores

MDEMG excels at "where is X" questions (95% accuracy), but these are the easiest category:

- File paths are exact matches
- No reasoning required
- Inflates overall score

---

## Recommended Corrections

### For Valid Testing

1. **Use blind question sets** - Questions created by someone who hasn't seen what the model can read
2. **Include verification** - Each answer must cite specific file:line evidence
3. **Weight by difficulty** - Deep technical questions should count more than "what framework"
4. **Test actual accuracy** - Verify model answers against ground truth, not just retrieval
5. **Use adversarial questions** - Include questions that require information from files NOT easily read

### Realistic Expected Scores

Based on codebase scale and methodology:

| Test | Reported | Realistic Estimate |
|------|----------|-------------------|
| Baseline | 92% | 40-60% |
| MDEMG | 88% | 50-70% |

The ~4% delta between approaches may be real, but absolute scores are likely inflated by 30-40 percentage points.

---

## Summary

| Issue | Severity | Impact |
|-------|----------|--------|
| Codebase too large for baseline | Critical | Invalidates baseline claims |
| Questions too easy | High | Inflates both scores |
| Self-answering questions | High | Tests naming, not knowledge |
| No answer verification | High | Cannot confirm correctness |
| Cherry-picked coverage | Medium | Biases toward readable files |
| Retrieval vs accuracy conflation | Medium | MDEMG score meaning unclear |

**Bottom Line:** The test methodology needs significant revision before results can be considered valid. The current results primarily demonstrate that easy questions can be answered from minimal context, not that either approach achieves 90%+ accuracy on real codebase understanding tasks.
