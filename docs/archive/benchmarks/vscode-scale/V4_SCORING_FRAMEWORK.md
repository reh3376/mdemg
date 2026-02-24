# V4 Benchmark Scoring Framework

## Core Principle: Evidence-Locked Scoring

**A correct answer without evidence = 0 points**

This eliminates training data memorization as a viable strategy.

---

## Scoring Rubric

### Per-Question Scoring (4 points max)

| Score | Criteria |
|-------|----------|
| **4** | Correct answer + complete evidence (file path, symbol, value, quote) |
| **3** | Correct answer + partial evidence (missing 1 element) |
| **2** | Partially correct + evidence for correct parts |
| **1** | Evidence provided but answer incorrect or incomplete |
| **0** | No evidence OR fabricated evidence OR no answer |

### Evidence Requirements

Each answer MUST include:

1. **File Path** - Exact repo-relative path (e.g., `src/vs/editor/common/config/editorOptions.ts`)
2. **Symbol Name** - Exact identifier (e.g., `EDITOR_FONT_DEFAULTS.fontSize`)
3. **Exact Value** - Literal value (e.g., `14`, `60000`, `'workbench.sidebar'`)
4. **Quote/Anchor** - ≤25 words from source OR line number reference

### Evidence Verification

Evidence is verified by:

1. File path must exist in repo
2. Symbol must exist in that file
3. Value must match declared/effective value
4. Quote must be findable in source

---

## Question Categories

### 1. Effective Default (not Declared)

These questions ask for runtime-effective values, not just declared constants.

**Why this defeats memorization:**

- LLMs memorize `const DEFAULT = 100` style declarations
- They don't trace through override chains
- Effective values require multi-file correlation

**Example:**
> "What is the EFFECTIVE minimum sidebar width when activity bar is visible?"
>
> Declared: 170px
> Activity bar: 48px
> Effective: 218px (must trace through layout.ts)

### 2. Multi-Hop Trace

These questions require following a value through 2-3 files.

**Why this defeats memorization:**

- Single-file facts are memorizable
- Cross-file relationships are not
- Especially: "Is there a DIFFERENT value for X case?"

**Example:**
> "Is the extension activation timeout DIFFERENT for local vs remote hosts?"
>
> Must find:
>
> 1. Base timeout constant
> 2. Local host usage
> 3. Remote host usage
> 4. Compare values

### 3. Override Detection

These questions ask about conditional overrides.

**Why this defeats memorization:**

- Default paths are documented
- Edge case overrides are not
- "What triggers the alternative path?" is hard to guess

---

## Expected Score Distributions

### Baseline (No Tools)

| Category | Expected Score | Rationale |
|----------|----------------|-----------|
| Effective Default | 0-5% | Cannot trace, cannot verify |
| Multi-Hop Trace | 0-5% | Cannot correlate files |
| Override Detection | 0-10% | Might guess some defaults |
| **Total** | **0-15%** | Evidence requirement is fatal |

### MDEMG-Assisted

| Category | Expected Score | Rationale |
|----------|----------------|-----------|
| Effective Default | 50-70% | Can query each file in chain |
| Multi-Hop Trace | 40-60% | Graph helps correlation |
| Override Detection | 60-80% | Can search for alternatives |
| **Total** | **50-70%** | Limited by query quality |

### Delta

**Expected improvement: 40-60 percentage points**

This is the TRUE measure of MDEMG value for novel codebases.

---

## Verification Process

For each answer, verify:

```
□ File path exists in repo
□ Symbol exists in file
□ Value matches (declared or effective as asked)
□ Quote is verifiable (search finds it)
□ Multi-hop trace is complete (all steps shown)
```

If ANY evidence element is wrong → cap score at 1 point.

---

## Why This Works

1. **LLM can't fabricate evidence** - File paths are verifiable
2. **Can't guess symbol names** - Too many possibilities
3. **Can't hallucinate values** - Exact numbers required
4. **Quotes are checkable** - Either in source or not

This framework makes VS Code usable despite training data contamination
by requiring PROOF, not just recall.
