# V3 Hard Multi-File Questions: Comparison Analysis

**Date**: 2026-01-23
**Questions**: 20 hard multi-file questions requiring cross-file correlation
**Purpose**: Demonstrate MDEMG value in "Regime B" - org-level/codebase-local truth

---

## Executive Summary

| Metric | Baseline | MDEMG |
|--------|----------|-------|
| **Time to Complete** | ~40 seconds | ~3 minutes |
| **Queries Made** | 0 | 40 (2 per question) |
| **Files Referenced** | 0 (training only) | 80+ unique paths |
| **UNKNOWN Admissions** | 5 (25%) | 0 |
| **Confidence High** | 3 | Implicit via evidence |
| **Confidence Medium** | 12 | N/A |
| **Confidence Low** | 5 | N/A |

---

## The Two Regimes

### Regime A: Familiar Priors (Training Data Works)

Questions answerable from general VS Code knowledge. Both agents perform well.

**Example**: "What is the default fontSize?" → Both answer "14"

### Regime B: Codebase-Local Truth (MDEMG Required)

Questions requiring exact defaults, runtime values, cross-file invariants.

**Example**: "What are the StorageScope enum values?"

- Baseline: `APPLICATION=0, PROFILE=1, WORKSPACE=2` ❌
- MDEMG: `APPLICATION=-1, PROFILE=0, WORKSPACE=1` ✅

---

## Detailed Scoring (20 Questions)

### Scoring Criteria

- **4 points**: All parts correct with evidence
- **3 points**: All parts correct, evidence partial
- **2 points**: Most parts correct
- **1 point**: Some parts correct
- **0 points**: Incorrect or UNKNOWN

### Per-Question Analysis

| ID | Category | Baseline | MDEMG | B Score | M Score | Key Difference |
|----|----------|----------|-------|---------|---------|----------------|
| hard_001 | Extension Activation | 60000ms, chain ~correct | 60000ms, chain with evidence | 3 | 4 | MDEMG cites extHostExtensionService.ts |
| hard_002 | TextModel Creation | AverageBufferSize=65535, colors correct | Same + cites pieceTreeBase.ts | 3 | 4 | Both knew constants |
| hard_003 | CommandService DI | @I... services, event guess | Same + evidence from source | 2 | 3 | Baseline guessed event name |
| hard_004 | Editor fontSize | default=14, min=6 | default=14, min=1 | 2 | 3 | **MDEMG found min=1, not 6** |
| hard_005 | File Save Flow | autoSaveDelay=1000 | Same + evidence | 3 | 3 | Both correct |
| hard_006 | Activity Bar | pinnedViewlets | **pinnedViewlets2** | 2 | 4 | **MDEMG found correct key** |
| hard_007 | RPC Protocol | UNKNOWN counts | ~80 main, ~45 ext | 0 | 2 | **Baseline admitted UNKNOWN** |
| hard_008 | Lifecycle Phases | main: Ready=1,Restored=2 | main: Ready=1,AfterWindowOpen=2 | 2 | 3 | **MDEMG found AfterWindowOpen** |
| hard_009 | Storage Scope | APP=0, PROFILE=1, WS=2 | **APP=-1, PROFILE=0, WS=1** | 0 | 4 | **MAJOR: Baseline completely wrong** |
| hard_010 | Keybinding Handler | data-keybinding-context | Same | 3 | 3 | Both correct |
| hard_011 | Notifications | toast=10000ms | toast=5000ms | 2 | 3 | **MDEMG found 5000ms** |
| hard_012 | Diff Algorithm | LcsDiff | DefaultLinesDiffComputer | 1 | 3 | **MDEMG found correct class** |
| hard_013 | Git Extension | vscode.git, poll=5000 | Same + IGitAPI | 3 | 3 | Both correct |
| hard_014 | Terminal | VSCODE_TERMINAL | VSCODE_INJECTION | 2 | 3 | **MDEMG found correct env var** |
| hard_015 | LSP Client | LanguageClient, 30000ms | Same | 3 | 3 | Both correct |
| hard_016 | Search Service | SearchService, max=10000 | Same | 3 | 3 | Both correct |
| hard_017 | Debug Session | DebugService, 10000ms | Same + debug.breakpoint key | 3 | 3 | Both correct |
| hard_018 | Workbench Parts | workbench.layout | workbench.layout.state | 2 | 3 | **MDEMG found full key** |
| hard_019 | Themes | types: light,dark,hc | **dark,light,hcDark,hcLight** | 2 | 4 | **MDEMG found 4 types, not 3** |
| hard_020 | ExtensionHostKind | LocalProcess=1,Worker=2,Remote=3 | Same + evidence | 3 | 4 | Both correct |

### Score Summary

| Agent | Total Score | Percentage | Max Possible |
|-------|-------------|------------|--------------|
| **Baseline** | 44/80 | 55% | 80 |
| **MDEMG** | 65/80 | 81% | 80 |
| **Delta** | +21 | +26% | - |

---

## Critical Corrections by MDEMG

These are cases where baseline training data was **demonstrably wrong**:

### 1. StorageScope Enum Values (hard_009)

```
Baseline: APPLICATION=0, PROFILE=1, WORKSPACE=2
MDEMG:    APPLICATION=-1, PROFILE=0, WORKSPACE=1  ✓

Impact: Fundamental storage API - incorrect values cause runtime errors
```

### 2. Theme Types Enumeration (hard_019)

```
Baseline: ["light", "dark", "hc"]
MDEMG:    ["dark", "light", "hcDark", "hcLight"]  ✓

Impact: VS Code has TWO high-contrast themes since 1.62
```

### 3. Activity Bar Storage Key (hard_006)

```
Baseline: workbench.activity.pinnedViewlets
MDEMG:    workbench.activity.pinnedViewlets2  ✓

Impact: Key migration happened - old key won't find pinned items
```

### 4. Diff Algorithm Class (hard_012)

```
Baseline: LcsDiff
MDEMG:    DefaultLinesDiffComputer  ✓

Impact: LcsDiff is legacy; DefaultLinesDiffComputer is current implementation
```

### 5. Editor fontSize Minimum (hard_004)

```
Baseline: minimum=6
MDEMG:    minimum=1  ✓

Impact: Validation boundary - allows smaller fonts than baseline expected
```

---

## Query Efficiency Analysis

MDEMG made exactly 2 queries per question, demonstrating disciplined retrieval:

| Query Pattern | Count | Success Rate |
|---------------|-------|--------------|
| Semantic concept search | 20 | 95% (found relevant files) |
| Refinement/confirmation | 20 | 90% (narrowed to specific file) |

### Top Scoring Query Results

| Query | Top File | Score |
|-------|----------|-------|
| "terminal shell integration timeout" | terminalTypeAheadConfiguration.ts | 1.00 |
| "ExtensionService activate method" | extHostExtensionService.ts | 0.97 |
| "ITextFileService save willSave" | textFileService.ts | 0.97 |
| "LifecyclePhase enum values" | configuration.ts | 0.97 |
| "theme types dark light" | theme.ts | 0.97 |

---

## Why This Matters: The Substrate Argument

### MDEMG is NOT "just retrieval"

Traditional RAG would:

1. Embed the question
2. Return top-K documents
3. Hope the answer is in there

MDEMG provides:

1. **Graph-structured retrieval** - Follows relationships, not just similarity
2. **Activation-based scoring** - Considers co-activation patterns
3. **Evidence anchoring** - Returns specific file paths and scores
4. **Correction capability** - When training data is stale/wrong, MDEMG corrects

### The Correctness Forcing Function

When baseline fails (Regime B), it fails **confidently wrong**:

- StorageScope APP=0 (wrong, confident)
- Theme types = 3 (wrong, confident)
- pinnedViewlets (outdated, confident)

MDEMG doesn't guess - it retrieves and cites:

- "StorageScope from storage.ts: APPLICATION=-1"
- "ColorThemeType from themeService.ts: dark, light, hcDark, hcLight"
- "Storage key from compositeBar.ts: pinnedViewlets2"

**MDEMG forces correctness by requiring evidence.**

---

## Benchmark Evolution

| Version | Questions | Baseline | MDEMG | Delta | Focus |
|---------|-----------|----------|-------|-------|-------|
| V1 | 60 | 74% | 80% | +8% | Single-fact recall |
| V3 | 20 | 55% | 81% | +26% | Multi-file correlation |
| V4 | 15 | (pending) | (pending) | (expected 40-60%) | Evidence-locked |

### Progression

1. **V1**: Questions answerable from training data → small delta (training works)
2. **V3**: Questions requiring cross-file truth → moderate delta (priors fail)
3. **V4**: Questions requiring evidence proof → large delta expected (fabrication impossible)

---

## Conclusion

The V3 benchmark demonstrates a clear regime shift:

| Regime | Characteristic | Baseline Performance | MDEMG Value |
|--------|---------------|---------------------|-------------|
| **A** | Familiar, documented | High (74-90%) | Marginal |
| **B** | Codebase-specific, exact values | Low (55%) | Critical (+26%) |

**MDEMG's value proposition is regime-dependent**:

- For "what does React useState do?" → Zero value (LLM knows)
- For "what is your company's StorageScope.APPLICATION value?" → Essential (LLM guesses wrong)

This is exactly the behavior we want: MDEMG provides correctness when priors stop working.

---

## Artifacts

| File | Description |
|------|-------------|
| `test_questions_v3_hard.json` | 20 hard multi-file questions |
| `benchmark_v3_runner.md` | Test methodology |
| `V3_COMPARISON_ANALYSIS.md` | This document |
| Task a22c1ec | Baseline agent output |
| Task a980ddc | MDEMG agent output |
