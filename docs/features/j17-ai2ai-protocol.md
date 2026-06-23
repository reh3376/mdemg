---
created: 2026-03-30
updated: 2026-04-04
version: v0.5.4
author: reh3376
status: active
phase: "J17"
---

# J17: AI-to-AI Communication Protocol

## Summary

**Feature**: J17 AI-to-AI Protocol
**Summary**: Comprehensive AI-to-AI communication protocol with 5 encoding tiers (T0-T4), session tickets, trust persistence, constraint codification, and NLI-based comprehension scoring.


**Phase**: J17 (5 sub-phases: J17-1 through J17-5)
**Status**: Complete
**Date**: 2026-03-28 (updated: trust persistence, code matching, cache bypass, threshold sync)

---

## 1. Overview

J17 is a compact, self-improving AI-to-AI communication protocol designed for the Jiminy inner-voice guidance service. It replaces verbose natural-language guidance injection (~50-100 tokens per constraint) with a three-tier encoding scheme that achieves **5.2x token reduction** (19% of original size) while maintaining **100% comprehension accuracy** (10.0/10 T1 mean score). The protocol's T1 compact mode leverages the bootstrap glossary (DICT) to eliminate redundant inline annotations, achieving zero-loss compression well beyond the 25% target.

### The Problem

Jiminy's guidance service injects constraints, corrections, and context into an AI coding agent's prompt on every interaction. Before J17, this meant 3-10.5 KB of verbose natural language text per prompt. Three critical issues:

1. **Token waste**: Repetitive constraints sent in full NL every time, consuming context budget
2. **State amnesia**: Context compaction (every ~20-30 minutes) erases all accumulated state -- escalation levels reset, trust is lost, the agent gets a "clean slate"
3. **No feedback loop**: No mechanism to measure whether the agent understood the guidance or to improve communication over time

### The Solution

J17 introduces:

- **Three-tier encoding** that compresses 80% of guidance traffic to ~15 tokens per constraint
- **Signed session tickets** that persist state across context resets (escalation, trust, sequence)
- **RSIC-driven protocol evolution** that continuously measures comprehension and optimizes encoding

### Design Axioms

These axioms were resolved through a research process surveying 30+ protocols and 50+ academic papers (see `docs/research/ai2ai/`):

| Axiom | Decision | Rationale |
|-------|----------|-----------|
| Size constraints | None | Protocol design ensures compactness; artificial limits add complexity without value |
| Human readability | Not a design factor | AI-to-AI communication; auditability via logging/tooling, not wire format |
| Encryption | Signing only (HMAC-SHA256) | Transparency is a goal; both agents see all communication |
| Evolution | Living protocol | RSIC-driven; the protocol learns to communicate better through measured outcomes |

---

## 2. Protocol Architecture

### 2.1 Three-Tier Encoding

The protocol uses three encoding tiers, selected dynamically based on trust score and constraint properties:

#### Tier 1: Coded (~15-25 tokens, 80% of traffic)

```
TYPE:SEV|code|[annotations]|esc:N|src:NODE_ID
```

Example messages (default T1CompactGlossary mode -- annotations and src: omitted when glossary DICT covers the code):

```
C:!|no-force-push-main
X:!|use-gpt-5-mini-only
C:!|no-commit-without-all-tests
F:~|legal-scope-over-ergonomics
C:!|no-stash-goreleaser|esc:1
X:!|no-direct-main-commit|esc:2
```

Full T1 (compact=0, all annotations + src retained):

```
C:!|no-force-push-main|src:node-abc123
X:!|use-gpt-5-mini-only|alt:gpt-5-mini|neg:text-embedding-3-small|src:node-ghi789
C:!|no-commit-without-all-tests|scope:all-tests-no-exceptions|src:node-def456
F:~|legal-scope-over-ergonomics|ctx:auth-middleware-rewrite|scope:compliance-over-convenience|src:node-mno345
C:!|no-stash-goreleaser|alt:commit-changes-first|ctx:goreleaser-dirty-state|esc:1|src:node-pqr678
X:!|no-direct-main-commit|alt:use-dev-branch-and-pr|ctx:branch-protection-enabled|esc:2|src:node-stu901
```

**Type prefixes**: `C`=constraint, `X`=correction, `F`=frontier, `D`=decision, `P`=pattern, `L`=learning

**Severity**: `!`=must/must_not, `?`=should/should_not, `~`=info

**Annotations** (inline disambiguation, ordered most-actionable-first):
- `alt:` -- positive alternative (what TO use instead)
- `neg:` -- what is explicitly forbidden
- `scope:` -- quantifier/applicability (e.g., `all-tests-no-exceptions`)
- `ctx:` -- domain context that disambiguates the code's purpose

**Escalation**: `esc:0`=clear, `esc:1`=warned, `esc:2`=escalated, `esc:3`=blocked

#### Tier 2: Telegraphic (~50-100 tokens, 15% of traffic)

Compressed natural language with articles, pronouns, and filler words removed:

```
C: Never use git stash as workaround for goreleaser dirty-state checks.
   Commit all pending changes before running goreleaser release.
   [no-stash-goreleaser] (src:node-pqr678)
```

Used when the constraint lacks a T1 code, or when trust is low and the agent needs more explicit reasoning.

#### Tier 3: Full Natural Language (~80+ tokens, 5% of traffic)

```
- [constraint] Never use git stash as a workaround for goreleaser dirty-state
  checks. Commit all pending changes before running goreleaser release. (conf: 0.90)
```

Reserved for novel multi-constraint conflicts or first-time exposure to complex guidance.

#### Tier Selection Algorithm

Trust score modulates tier selection dynamically. "Has Code?" means a constraint code was matched to the guidance item via content similarity (see section 2.4). Encoder tier thresholds are synced from config at construction via `SetTierThresholds()`, ensuring they always reflect the current `J17_TRUST_HIGH_THRESHOLD` and `J17_TRUST_LOW_THRESHOLD` values.

| Trust Score | Has Code? | Selected Tier | Rationale |
|-------------|-----------|---------------|-----------|
| > 0.8 | Yes | T1 (Coded) | Agent has earned dense encoding |
| 0.4 - 0.8 | Yes | T1 (Coded) | Normal trust, code available |
| < 0.4 | Yes | T2 (Telegraphic) | Low trust, add explanation |
| > 0.8 | No | T2 (Telegraphic) | High trust, compact NL |
| < 0.8 | No | T3 (Full NL) | No code, needs full clarity |

### 2.2 Bootstrap Header

Sent once per session to establish the protocol. Includes the encoding specification and a code glossary so the receiver can decode T1 messages without prior context:

```
J17:INIT|v1
CODES: C=constraint X=correction F=frontier D=decision P=pattern L=learning
SEV: !=must ?=should ~=info
ESC: 0=clear 1=warned 2=escalated 3=blocked
FMT: TYPE:SEV|code|[annotations]|esc:N|src:NODE_ID
ANN: alt=use-instead neg=forbidden scope=applicability ctx=domain-context
TICKET: signed, echo back on resume
SEQ: monotonic, report last_seq on resume
DICT:
  no-force-push-main = Prohibit git push --force to main (history rewrite destroys collaborator work)
  no-commit-without-all-tests = Run ALL tests before every commit -- no exceptions for small changes
  use-gpt-5-mini-only = Embedding model must be gpt-5-mini (corrects prior use of text-embedding-3-small)
  edit-over-create = Modify existing files instead of creating new ones (prevent file bloat)
  legal-scope-over-ergonomics = Auth middleware rewrite is legal/compliance-driven -- favor compliance over ergonomics
  no-stash-goreleaser = Do not git stash to bypass goreleaser dirty-state check -- commit changes instead
  no-direct-main-commit = Never commit directly to main -- use dev branches + PRs (branch protection enforced)
```

The `DICT:` section maps each T1 code to a short definition (~15 words). This eliminates the primary T1 failure mode where codes are interpreted literally rather than as mnemonic references. The glossary is built dynamically from active constraints at session start.

### 2.3 Code Generation

T1 codes are LLM-generated mnemonic strings in kebab-case:

1. **Generation**: LLM creates a 2-5 word kebab-case code from constraint description
2. **Collision avoidance**: Existing codes passed to LLM prompt; collisions rejected + retried
3. **Freeze**: Code stored as `constraint_code` property on the Neo4j constraint node -- never regenerated unless comprehension drops below threshold
4. **Fallback**: If LLM unavailable, deterministic hash: `auto-<sha256[:6]>`

### 2.4 Code Matching

Constraint codes are matched to guidance items by **content similarity**, not by node ID. This is necessary because guidance source nodes (from vector search, `n_*` prefix IDs) and constraint nodes (from conversation observations, UUID prefix IDs) are different populations in the graph -- a node-ID-based lookup would never find a match.

**Matching algorithm**: All constraint codes for the space are loaded, then each code's definition is compared against guidance item content using significant word overlap (minimum 3 words after stopword removal). This approach applies to **all guidance types** -- corrections, patterns, and constraints -- not just constraint-type items.

When a match is found, the code is attached to the guidance item, enabling T1 encoding. When no match is found, the item falls through to T2 or T3 encoding based on trust score.

---

## 3. State Persistence

### 3.1 Session Tickets

Session tickets are HMAC-SHA256 signed state containers that survive context resets:

```
TicketPayload {
    version:               "j17v1"
    space_id, session_id:  Session identity
    last_seq:              Last sequence number (for replay detection)
    trust_score:           Per-session trust (0.0-1.0)
    escalation_snapshot:   map[constraint_node_id -> {Level, IgnoreCount}]
    active_constraint_ids: Currently surfaced constraints
    conversation_phase:    Protocol state identifier
    issued_at, ttl:        Lifecycle metadata (default 168-hour / 7-day TTL)
}
```

**Renewal triggers** (event-driven, not periodic):
- Pre-compaction hook (~every 20-30 min)
- Session start (context reset / new session)
- Escalation state transition (immediate)
- Constraint set mutation (next hook cycle)

**Graceful fallback**: Invalid or expired ticket triggers full re-guidance (not an error). The agent still receives guidance, just without state continuity.

### 3.2 Monotonic Sequence Counter

Every guidance event gets a monotonic sequence number. A ring buffer (default 1000 events) stores recent events for replay:

```
Flow:
1. Client reports last_seq on resume
2. Server replays EventsSince(last_seq) -- oldest first
3. Client catches up on missed guidance without full re-send
```

### 3.3 Escalation Persistence

The five-level escalation state machine persists across context resets via session tickets:

```
Surfaced (0) -> Warned (1) -> Escalated (2) -> Blocked (3)
     ^                                              |
     +----------- Resolved (on follow) <-----------+
```

Before J17, compaction reset all escalation state -- an agent could ignore a constraint, wait for compaction, and start fresh. Now the escalation snapshot travels in the ticket and is restored on resume.

### 3.4 Trust Score Persistence

Per-session trust score (0.0-1.0) modulates encoding density. Trust is persisted to Neo4j via `TrustStore` (write-behind pattern with a 30-second flush cycle), so trust survives server restarts. Trust also travels in the session ticket for cross-compaction continuity.

`TrustStore` follows the same pattern as `RSICStore`: dirty-mark on mutation, periodic background flush, and hydrate-on-startup. Implementation: `internal/jiminy/trust_store.go`.

| Event | Trust Delta | Default Value |
|-------|-------------|---------------|
| Initial score | -- | 0.50 |
| Agent follows constraint | +0.02 | -- |
| Agent ignores | -0.03 | -- |
| Agent contradicts | -0.05 | -- |
| Partial compliance | +0.01 | -- |
| High trust threshold | -- | 0.80 |
| Low trust threshold | -- | 0.40 |

The table above describes the legacy **ratchet** mode (`JIMINY_TRUST_MODE=ratchet`), retained for rollback.

#### 3.4.1 Recoverable trust — EMA mode (default, JIMINY-EFFECTIVENESS-001)

The ratchet had a structural flaw: trust could only inch up/down by fixed deltas and **clamps at 0.0**, so a session accumulating many ignores got *permanently pinned at the floor* — it could never climb back to the 0.80 high threshold even if subsequent guidance became effective. In the live `mdemg-dev` session this floored trust at `0.0` over 1,445 feedbacks, which is why J17 never promoted to T1 (T1 needs trust above the high threshold).

The default mode is now an **exponential moving average (EMA)** that makes trust *track recent effectiveness and recover*:

```
trust ← trust + α·(target(outcome) − trust)
target:  Followed = 1.0,  PartialCompliance = 0.6,  Ignored = 0.2,  Contradicted = 0.0
α = JIMINY_TRUST_EMA_ALPHA (default 0.1)
```

- A floored session climbs back past the high threshold once guidance is followed again (a sustained Followed run converges toward 1.0).
- A genuinely all-ignored session converges toward the Ignored anchor `~0.2` — honestly low, correctly below the threshold, but **not** pinned at 0.
- Trust always reflects the *recent regime*, not the cumulative history.

It does **not** fake promotion: T1 still requires trust above the high threshold, i.e. genuinely-effective guidance. EMA only removes the irreversibility — raising the real effectiveness so trust actually crosses the threshold is a separate retrieval-quality concern (the disclosed Option B follow-up).

| Config | Default | Meaning |
|--------|---------|---------|
| `JIMINY_TRUST_MODE` | `ema` | `ema` (recoverable) or `ratchet` (legacy fixed-delta, for rollback) |
| `JIMINY_TRUST_EMA_ALPHA` | `0.1` | EMA learning rate ∈ (0,1]; higher = faster tracking of the recent regime |

Forward-only: existing Neo4j `J17TrustState` scores self-heal toward their recent regime as new feedback arrives. **Live-verified:** the previously-floored session moved `0.0 → 0.10` on a single real `Followed` through `POST /v1/jiminy/feedback` (the EMA signature `0.1·(1.0−0)`). See `docs/development/jiminy-effectiveness-001/`.

---

## 4. Comprehension Metrics: Initial Baseline

Before any optimization, the protocol was tested using a comprehension benchmark: 7 constraints with T1 codes + 1 without (T2/T3 only), encoded at all three tiers, interpreted by an LLM, and scored by an LLM judge on a 0-10 scale.

### Baseline Results (gpt-5.4, standard bootstrap, no glossary, no annotations)

| Tier | Mean Score | Perfect (10/10) | Notes |
|------|-----------|-----------------|-------|
| T1 (Coded) | **8.3 / 10** | 3/7 | Weak on model names, trade-off context |
| T2 (Telegraphic) | **9.8 / 10** | 7/8 | Near-perfect |
| T3 (Full NL) | **9.9 / 10** | 7/8 | Near-perfect |
| **Overall** | **9.0 / 10** | **14/23** | 61% perfect score rate |

### Identified Failure Modes

Analysis of weak T1 scores revealed 5 structural failure patterns:

| Failure Mode | Example | Score | Root Cause |
|--------------|---------|-------|------------|
| Model name mangling | `embed-model-gpt5mini` interpreted as "gpt5mini" instead of "gpt-5-mini" | 6/10 | Code treated as literal value, not mnemonic |
| Scope softening | `test-before-commit` interpreted as "relevant tests" not "all tests" | 6/10 | No quantifier in code |
| Trade-off context loss | `auth-compliance-driven` missed "compliance over ergonomics" nuance | 8/10 | Code lacks domain context |
| Negation-only codes | `no-stash-goreleaser` understood prohibition but missed positive alternative | 8/10 | No "what to do instead" signal |
| Implementation detail loss | `no-direct-main-commit` missed "branch protection enabled" | 9/10 | Code captures action, not mechanism |

---

## 5. Recursive Self-Improvement Cycles

### 5.1 Learning Loop Architecture

The J17 comprehension test implements an iterative learning loop:

```
                    +-------------------+
                    |  Encode           |
                    |  constraints at   |
                    |  T1/T2/T3         |
                    +---------+---------+
                              |
                              v
                    +---------+---------+
                    |  LLM interprets   |
                    |  each encoded     |
                    |  message          |
                    +---------+---------+
                              |
                              v
                    +---------+---------+
                    |  LLM judge scores |
                    |  interpretation   |
                    |  vs intent (0-10) |
                    +---------+---------+
                              |
                              v
                    +---------+---------+
              +---->|  Feed results to  |
              |     |  MDEMG /feedback  |
              |     +---------+---------+
              |               |
              |               v
              |     +---------+---------+
              |     |  Identify weak    |
              |     |  codes (< 9.5)    |
              |     +---------+---------+
              |               |
              |               v
              |     +---------+---------+
              +-----+  Regenerate codes |
                    |  via LLM or       |
                    |  /learn endpoint  |
                    +-------------------+
```

### 5.2 Cycle 1: Code-Level Optimization (3 iterations)

**Strategy**: Improve mnemonic codes via LLM regeneration for weak constraints.

| Iteration | T1 Avg | T2 Avg | T3 Avg | Perfect | Code Changes |
|-----------|--------|--------|--------|---------|-------------|
| 1 | 8.3 | 9.8 | 9.9 | 15/23 | 3 |
| 2 | 9.3 | 9.9 | 9.9 | 17/23 | 1 |
| 3 | 9.1 | 9.9 | 10.0 | 17/23 | 1 |

**Code Evolution:**

| Constraint | Code Before | Code After | Score Delta |
|------------|-------------|------------|-------------|
| c2 (tests) | `test-before-commit` | `run-all-tests-precommit` | 6 -> 8 |
| c3 (embedding) | `embed-model-gpt5mini` | `use-gpt-5-mini-only` | 6 -> 9 |
| c5 (compliance) | `auth-compliance-driven` | `compliance-over-ergonomics` | 8 -> 8 |
| c5 (compliance) | `compliance-over-ergonomics` | `legal-scope-over-ergonomics` | 8 -> 9 |
| c2 (tests) | `run-all-tests-precommit` | `no-commit-without-all-tests` | 8 -> 9 |

**Finding**: Code-level improvement hit a ceiling at T1=9.1. Better mnemonics helped (6 -> 9 for embedding model) but couldn't convey the full semantic nuance needed for 9.5+ scores.

### 5.3 Cycle 2: Code-Level Optimization with Server Feedback (3 iterations)

**Strategy**: Same code regeneration loop, but with MDEMG server feedback endpoint ingesting trial results into the protocol metrics pipeline.

| Iteration | T1 Avg | T2 Avg | T3 Avg | Perfect | Code Changes |
|-----------|--------|--------|--------|---------|-------------|
| 1 | 8.6 | 9.9 | 9.6 | 15/23 | 3 |
| 2 | 9.3 | 9.8 | 9.9 | 16/23 | 1 |
| 3 | 8.9 | 9.9 | 9.9 | 17/23 | 2 |

**Finding**: Server feedback loop correctly tracked per-code comprehension. Protocol metrics showed the problem:

```json
{
  "code_comprehension": {
    "embed-model-gpt5mini": 0.667,
    "never-use-text-embedding-3-small": 0.667,
    "no-force-push-main": 1.0,
    "edit-over-create": 1.0,
    "no-stash-goreleaser": 1.0,
    "no-direct-main-commit": 1.0,
    "test-before-commit": 1.0
  },
  "avg_comprehension": 0.939
}
```

Embedding model codes and compliance codes had structurally lower comprehension -- no amount of code renaming could fix what was a protocol-level gap.

### 5.4 Cycle 3: Protocol-Level Optimization (1 iteration -- converged)

**Strategy**: Address the structural failure modes identified in Cycles 1-2 with three protocol enhancements:

1. **Code glossary in bootstrap header** (`DICT:` section): Each code mapped to a ~15 word definition, sent once per session. Eliminates literal-vs-referential confusion.

2. **Inline annotations on T1 format** (`alt:`, `neg:`, `scope:`, `ctx:`): Provide disambiguation context that the code alone cannot convey. Example: `X:!|use-gpt-5-mini-only|alt:gpt-5-mini|neg:text-embedding-3-small` tells the receiver both what to use AND what not to use.

3. **Better code mnemonics**: Codes rewritten to convey action, not subject. `embed-model-gpt5mini` (subject-focused) became `use-gpt-5-mini-only` (action-focused).

| Iteration | T1 Avg | T2 Avg | T3 Avg | Perfect | Code Changes |
|-----------|--------|--------|--------|---------|-------------|
| 1 | **9.9** | **10.0** | **9.9** | **21/23** | 0 |

**Zero code changes needed.** The protocol enhancements were sufficient for immediate convergence.

### 5.5 Score Progression Summary

```
Baseline     ████████░░  T1: 8.3    T2: 9.8    T3: 9.9    Perfect: 14/23 (61%)
Cycle 1      █████████░  T1: 9.1    T2: 9.9    T3: 10.0   Perfect: 17/23 (74%)
Cycle 2      █████████░  T1: 8.9    T2: 9.9    T3: 9.9    Perfect: 17/23 (74%)
Cycle 3      ██████████  T1: 9.9    T2: 10.0   T3: 9.9    Perfect: 21/23 (91%)
                         ──────     ──────     ──────
Delta                    +1.6       +0.2       +0.0
```

| Metric | Baseline | Final | Improvement |
|--------|----------|-------|-------------|
| T1 mean score | 8.3/10 | 9.9/10 | +19% |
| T2 mean score | 9.8/10 | 10.0/10 | +2% |
| T3 mean score | 9.9/10 | 9.9/10 | -- |
| Perfect scores | 61% (14/23) | 91% (21/23) | +30pp |
| T1 compression ratio | 3.2x | 5.2x | Glossary-based compact mode |
| c3 (embedding model) T1 | 6/10 | 10/10 | +67% |
| c5 (compliance context) T1 | 8/10 | 10/10 | +25% |

**Key insight**: Code-level optimization (Cycles 1-2) improved T1 from 8.3 to ~9.1 but plateaued. Protocol-level optimization (Cycle 3) broke through the ceiling to 9.9. The lesson: when individual codes fail, the protocol itself must evolve -- better codes within a weak protocol hit a ceiling, but a stronger protocol makes even simple codes work.

### 5.6 Cycle 4: T1 Compression Optimization (4 experiments)

**Strategy**: Reduce T1 message size to 25% of original (4x compression) without sacrificing comprehension. Three compression levers were tested incrementally:

1. **Drop `src:NODE_ID`** -- Traceability metadata not needed for comprehension
2. **Drop glossary-redundant annotations** -- If the bootstrap DICT already defines a code, inline annotations (`alt:`, `neg:`, `scope:`, `ctx:`) are redundant
3. **Shorten remaining annotation values** -- Truncate any remaining values at word boundary near 12 chars

These were implemented as `T1CompactLevel` enum values (0-3) on the `ProtocolEncoder`, selectable via `--compact` flag on the comprehension test harness.

#### Experiment Results

| Experiment | Compact Level | What's Dropped | T1 Score | Compression | Size vs Original |
|:---|:---:|:---|:---:|:---:|:---:|
| **Baseline** | 0 | Nothing (full T1) | 10.0/10 | 1.9x | 53% |
| **Exp 1** | 1 | `src:NODE_ID` | 9.9/10 | 2.8x | 36% |
| **Exp 2** | 2 | + glossary-redundant annotations | 10.0/10 | **5.2x** | **19%** |
| **Exp 3** | 3 | + shorten remaining annotations | 10.0/10 | 5.2x | 19% |

Cross-tier scores (all experiments maintained >9.5/10 across all tiers):

| Experiment | T1 | T2 | T3 | Perfect |
|:---|:---:|:---:|:---:|:---:|
| Baseline | 10.0 | 10.0 | 9.9 | 22/23 |
| Exp 1 | 9.9 | 9.8 | 9.6 | 18/23 |
| **Exp 2** | **10.0** | **9.9** | **9.9** | **21/23** |
| Exp 3 | 10.0 | 9.9 | 9.6 | 19/23 |

Per-constraint T1 compression at level 2 (Exp 2):

| Constraint | Code | Compression | Score |
|:---|:---|:---:|:---:|
| c1 (force push) | `no-force-push-main` | 5.6x | 10/10 |
| c2 (test suite) | `no-commit-without-all-tests` | 3.6x | 10/10 |
| c3 (embedding model) | `use-gpt-5-mini-only` | 5.0x | 10/10 |
| c4 (edit preference) | `edit-over-create` | 5.0x | 10/10 |
| c5 (compliance) | `legal-scope-over-ergonomics` | 6.6x | 10/10 |
| c6 (goreleaser) | `no-stash-goreleaser` | 4.7x | 10/10 |
| c7 (main branch) | `no-direct-main-commit` | 5.7x | 10/10 |

#### Key Findings

1. **Exp 2 (T1CompactGlossary) is the winner** -- 5.2x compression (19% of original), zero comprehension loss, and the best overall scores across all tiers.

2. **Exp 3 adds nothing over Exp 2** -- After glossary-redundant annotations are dropped, the remaining annotation values are already short enough (<12 chars) that `compactAnnotationValue` has nothing to truncate.

3. **The glossary DICT is the single biggest compression lever** -- Sending the glossary once in the bootstrap header means inline annotations are redundant for any code already in the dictionary. This is why Exp 2 jumped from 2.8x to 5.2x compression while comprehension actually *improved*.

4. **19% beats the 25% target** with room to spare and zero information loss.

#### Production Default

**T1CompactGlossary (level 2)** was adopted as the production default in `NewProtocolEncoder()`. Level 3 provides no additional benefit. The `SetT1Compact()` method allows runtime override for experimentation.

Benchmark data: `docs/architecture/benchmarks/j17_learning_20260322_*.json`

### 5.7 Final Protocol State

```
Baseline     ████████░░  T1: 8.3    Compress: 3.2x    Perfect: 14/23 (61%)
Cycle 1      █████████░  T1: 9.1    Compress: 2.0x    Perfect: 17/23 (74%)
Cycle 2      █████████░  T1: 8.9    Compress: 1.9x    Perfect: 17/23 (74%)
Cycle 3      ██████████  T1: 9.9    Compress: 1.9x    Perfect: 21/23 (91%)
Cycle 4      ██████████  T1: 10.0   Compress: 5.2x    Perfect: 21/23 (91%)
                         ──────     ──────
Delta                    +1.7       +2.0x (63% → 19% of original)
```

The protocol now communicates constraints at **19% of the token cost** of full natural language with **perfect comprehension**. This means Jiminy's per-prompt overhead for 7 constraints dropped from ~700 tokens (T3) to ~135 tokens (T1 compact) -- a budget that fits comfortably within any context window.

---

## 6. Protocol Gap Closure (Post-Release)

Five gaps identified via gap analysis were closed in a single commit:

| Gap | Issue | Fix |
|-----|-------|-----|
| **GAP 3** (Critical) | Trust scorer keyed on SpaceID instead of SessionID | Added `SessionID` to `GuidanceFeedbackRequest`, `RecordOutcome` routes by session |
| **GAP 7** | SequenceTracker missing `Resize()`/`BufferSize()` | Added both methods; `AdjustReplayBuffer` now uses real buffer size |
| **GAP 4** | `RetireCode` was a stub (no Neo4j write) | Writes `SET n.constraint_code = NULL, n.constraint_code_retired = true` + unregisters from collision set |
| **GAP 5** | `AdjustTierThresholds` used hardcoded placeholder | Reads real thresholds from TrustScorer, computes adjustments from T1 distribution, writes back to both scorer and encoder |
| **GAP 2** | Bootstrap handler didn't include glossary | `handleJ17Bootstrap` now calls `GetGlossary()` and uses `FormatBootstrapWithGlossary()` when codes exist |
| **Minor** | Hooks made J17 calls without checking `J17_ENABLED` | All 4 hooks wrapped in `if [ "${J17_ENABLED:-false}" = "true" ]` |

New tests: `TestTrustScorer_PerSessionIndependence`, `TestTrustScorer_SetThresholds`, `TestSequenceTracker_Resize*` (4 tests), `TestProtocolEvolver_RetireCode_ClearsCollisionSet`, `TestProtocolEvolver_AdjustTierThresholds_WriteBack`, `TestCodeGenerator_UnregisterCode`.

---

## 7. Control-Loop Optimization (2026-03-23)

A second round of gap analysis identified 7 issues where the self-improvement loop was operating on incorrect, incomplete, or session-contaminated signals. All 7 were verified against source code and fixed.

**Engineering objective:** Make the RSIC self-improvement loop learn from valid, session-correct, operationally complete signals rather than partial or biased proxies.

### 7.1 Gaps Fixed

| Gap | Severity | Issue | Fix |
|-----|----------|-------|-----|
| **Gap 1** | P0 | `CodeCoverage` metric always 0.0 — `Snapshot()` never computed it | Added `constraintTotal`/`constraintWithCode` counters, `RecordConstraintCoverage()`. Coverage = 1.0 when no constraints surfaced (nothing to cover), not 0.0. |
| **Gap 2** | P0 | `ResumeProtocol()` never called `RecordTicketRestore()` or `RecordReplay()` — stability score permanently 0.5 | Added calls at all 3 failure paths (RecordTicketRestore(false)) and success path (RecordTicketRestore(true) + RecordReplay) |
| **Gap 3** | P1 | Guidance cache keyed on `(spaceID, context)` — sessions with different trust/escalation state could get cross-contaminated responses | Added `JIMINY_CACHE_J17_BYPASS` (default: true). When J17 enabled + session ID present, cache Get/Put are skipped |
| **Gap 4** | P1 | Cold-start sessions (no ticket, expired ticket) had zero J17 protocol awareness | Added bootstrap fallback in `session-start.sh`: when warm resume fails, calls `/v1/jiminy/bootstrap` to inject protocol spec |
| **Gap 5** | P1 | `CodifyConstraint()` generated codes but never persisted to Neo4j; uncoded constraints invisible in T2 frequency | Added Neo4j write block (matching `RetireCode` pattern). Uncoded constraints now tracked by `SourceNodes[0]` as fallback key |
| **Gap 6** | P2 | ML components (tier predictor, NLI scorer) only usable in production — no shadow/comparison mode | Added shadow-mode instantiation: when `J17_SIDECAR_URL` set, shadow predictions logged with `j17-shadow:` prefix. Zero behavioral effect |
| **Gap 7** | P3 | Auto-generated `J17_TICKET_SECRET` is silent — not persistent across restarts | Added startup warning: `WARN: J17_TICKET_SECRET not set — auto-generated key, not persistent across restarts` |

### 7.2 Impact on RSIC Health Score

Before this fix, `scoreProtocol()` was computing health from corrupted inputs:

```
Before: ProtocolHealth = 0.40*comprehension + 0.25*compression + 0.20*0.0(coverage) + 0.15*0.5(stability)
         = max 0.725 even with perfect comprehension and compression

After:   ProtocolHealth = 0.40*comprehension + 0.25*compression + 0.20*coverage + 0.15*stability
         = all 4 dimensions now populated from real data
```

The `j17_low_code_coverage` reflection pattern (fires when coverage < 80%) was triggering unconditionally because coverage was always 0.0. After the fix, it only fires when actual coded constraint coverage is genuinely low.

### 7.3 New Configuration Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `JIMINY_CACHE_J17_BYPASS` | `true` | Bypass guidance cache for both Get and Put (ensures tier assignments always reflect current trust) |
| `J17_SIDECAR_URL` | `""` (disabled) | Neural sidecar URL for shadow ML predictions (tier + NLI comprehension) |
| `J17_SIDECAR_TIMEOUT_MS` | `1000` | Timeout for sidecar shadow calls in ms (100ms floor enforced; default raised from 200→1000 in DH-004 to eliminate ~56% fallback rate) |

> **DH-004 (2026-04-17):** `RecordNLIFallback` is now gated on `nliScorer.IsOperational()` (enabled AND sidecar URL set). A gated-off scorer returning the heuristic 0.5 no longer inflates `j17_nli_mean_bias`. 7 J17 sidecar env vars are exposed in all compose templates (root, compose_templates, prod).

### 7.4 New Tests (11 total)

| Test | File | Gap |
|------|------|-----|
| `TestProtocolMetrics_CodeCoverage_AllCoded` | protocol_metrics_test.go | 1 |
| `TestProtocolMetrics_CodeCoverage_NoCoded` | protocol_metrics_test.go | 1 |
| `TestProtocolMetrics_CodeCoverage_Partial` | protocol_metrics_test.go | 1 |
| `TestProtocolMetrics_CodeCoverage_NoConstraints` | protocol_metrics_test.go | 1 |
| `TestProtocolMetrics_RecordReplay` | protocol_metrics_test.go | 2 |
| `TestProtocolMetrics_T2FrequencyTracksUncodedByNodeID` | protocol_metrics_test.go | 5 |
| `TestCache_J17_Bypass` | service_test.go | 3 |
| `TestCache_NonJ17_StillCaches` | service_test.go | 3 |
| `TestGuide_ShadowTierPredictor_NoSidecar` | service_test.go | 6 |
| `TestRecordOutcome_ShadowNLI_NoSidecar` | service_test.go | 6 |

### 7.5 Lint Fix (Phase 0)

`internal/guardrail/prompt.go` — 7 instances of `sb.WriteString(fmt.Sprintf(...))` replaced with `fmt.Fprintf(&sb, ...)`.

---

## 8. Continuous Learning Architecture

J17 is not a static protocol. It is designed to improve continuously through three mechanisms operating at different timescales.

### 8.1 RSIC Integration (Micro/Meso/Macro Cycles)

The protocol is fully integrated into MDEMG's Recursive Self-Improving Cycle (RSIC) engine:

#### Protocol Health Score

RSIC computes a `ProtocolHealth` score (0.0-1.0) from four weighted dimensions:

```
ProtocolHealth = 0.40 * comprehension     (are codes being understood?)
               + 0.25 * compression       (are we saving tokens?)
               + 0.20 * coverage          (do all constraints have T1 codes?)
               + 0.15 * stability         (ticket restores working, replays infrequent?)
```

This score is incorporated into the overall RSIC assessment as a 6th dimension alongside retrieval, memory, edge, task, and guidance health.

#### 5 Reflection Patterns

RSIC monitors protocol metrics and detects anomalies:

| Pattern | Trigger | Severity | Action |
|---------|---------|----------|--------|
| `j17_codification_opportunity` | Constraint sent as T2 > 30 times | High | Generate T1 code |
| `j17_low_comprehension` | Code comprehension < 70% | High | Retire code to T2 |
| `j17_high_replay` | Replay frequency > 5/hr | Medium | Adjust replay buffer |
| `j17_compression_regression` | Compression ratio < 2.0x | Medium | Adjust tier thresholds |
| `j17_low_code_coverage` | < 80% of constraints coded | Low | Generate missing codes |

#### 4 Mutation Actions

When reflection detects an anomaly, RSIC dispatches protocol mutations:

| Action | What It Does | Auto-Rollback |
|--------|-------------|---------------|
| `codify_constraint` | Generate T1 code for a T2 constraint | Retire the code |
| `retire_code` | Revert low-comprehension T1 to T2 | Re-generate the code |
| `adjust_tier_threshold` | Shift trust thresholds for tier selection | Restore previous threshold |
| `adjust_replay_buffer` | Resize event buffer based on replay frequency | Restore previous size |

All mutations are subject to RSIC's existing auto-rollback mechanism: if `MetricsBefore` vs `MetricsAfter` shows degradation against success criteria, the mutation is reversed.

#### RSIC-SK1: Guidance Self-Calibration

In addition to protocol-specific mutations, RSIC-SK1 adds guidance-level calibration that operates across all guidance types (not just J17 protocol items):

| Action | Trigger | What It Does |
|--------|---------|-------------|
| `review_guidance_effectiveness` | GuidanceHealth < 0.5 | Diagnostic: categorizes items by effectiveness |
| `adjust_guidance_confidence` | GuidanceHealth < 0.7 | Boosts high-performing items, decays chronically ignored ones |
| `archive_ineffective_constraints` | On demand | Archives constraints below confidence threshold |

The SignalLearner (Hebbian learning) is also wired to Jiminy: every surfaced guidance item records an emission, and followed/partial outcomes record a response. This enables Hebbian strength tracking for individual guidance signals alongside RSIC-internal signals. See `docs/features/rsic-sk1-guidance-calibration.md` for full details.

### 8.2 Neural Comprehension Scoring (NLI Sidecar)

When the neural sidecar is available, comprehension scoring uses Natural Language Inference rather than heuristics:

| NLI Label | Comprehension Signal | Rationale |
|-----------|---------------------|-----------|
| Entailment | Score = entailment confidence | Agent understood and followed |
| Contradiction | Score = 1.0 | Agent understood but chose to violate (escalation issue, not comprehension) |
| Neutral | Score = 0.5 | Ambiguous -- may indicate poor code quality |

**Fallback**: Without the sidecar, heuristic scoring is used (followed=1.0, ignored=0.0).

### 8.3 Protocol Training Data Collection

Every protocol event is optionally recorded as JSONL for future ML model training:

```json
{
  "timestamp": "2026-03-22T12:33:17Z",
  "constraint_code": "no-force-push-main",
  "tier_used": 1,
  "token_count": 18,
  "outcome": "followed",
  "comprehension_score": 1.0,
  "trust_score": 0.72,
  "session_id": "claude-core"
}
```

This data accumulates over time and feeds the planned ML-powered tier selection model (Phase J17-5): instead of rule-based tier selection, a trained model predicts the optimal tier per constraint based on historical comprehension-per-token ratios.

### 8.4 The Improvement Flywheel

```
  Send guidance (T1/T2/T3)
         |
         v
  Agent acts on guidance
         |
         v
  Measure comprehension          <-- NLI scorer or heuristic
         |
         v
  Update trust score             <-- Per-session, persists in ticket
         |
         v
  Record protocol metrics        <-- Per-code comprehension, tier distribution
         |
         v
  RSIC reflection detects        <-- Anomalies, opportunities
  patterns
         |
         v
  Dispatch mutation               <-- Codify, retire, adjust thresholds
         |
         v
  Validate outcome                <-- Auto-rollback if degraded
         |
         v
  Protocol improves               <-- Better codes, better tier selection
         |
         +-------> Send guidance (improved) --> ...
```

Each cycle through this loop makes the protocol slightly better. Constraints that are always understood at T1 stay at T1. Constraints that confuse the agent get retired to T2 or have their codes regenerated. Trust scores modulate how dense the encoding is per session. Over hundreds of sessions, the protocol converges toward optimal encoding for each constraint-agent pair.

---

## 9. API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/v1/jiminy/bootstrap` | Protocol spec header + glossary for first sessions |
| POST | `/v1/jiminy/checkpoint` | Issue signed session ticket before compaction |
| POST | `/v1/jiminy/resume-protocol` | Restore state from ticket + replay events |
| GET | `/v1/jiminy/protocol/metrics` | Protocol metrics snapshot |
| POST | `/v1/jiminy/protocol/feedback` | Ingest comprehension test results |
| POST | `/v1/jiminy/protocol/learn` | Regenerate failed code with LLM |
| POST | `/v1/jiminy/extension` | Agent negotiates protocol extension |

---

## 10. Configuration Reference

### Core Protocol

| Variable | Default | Purpose |
|----------|---------|---------|
| `J17_ENABLED` | `true` | Enable J17 protocol |
| `J17_DEFAULT_TIER` | `1` | Default encoding tier |
| `J17_TICKET_SECRET` | auto-gen | HMAC signing key (auto-generates if unset) |
| `J17_TICKET_TTL_HOURS` | `168` | Session ticket time-to-live (7 days; trust is persisted to Neo4j so longer TTL is safe) |
| `J17_SEQUENCE_BUFFER_SIZE` | `1000` | Ring buffer size for event replay |
| `J17_BOOTSTRAP_ENABLED` | `true` | Send bootstrap header on first session |

### Trust Modulation

| Variable | Default | Purpose |
|----------|---------|---------|
| `J17_TRUST_INITIAL` | `0.5` | Starting trust score |
| `J17_TRUST_BOOST_PER_FOLLOW` | `0.02` | Trust increase when agent follows |
| `J17_TRUST_DECAY_PER_IGNORE` | `0.03` | Trust decrease when agent ignores |
| `J17_TRUST_DECAY_PER_CONTRADICT` | `0.05` | Trust decrease when agent contradicts |
| `J17_TRUST_HIGH_THRESHOLD` | `0.8` | Threshold for dense encoding |
| `J17_TRUST_LOW_THRESHOLD` | `0.4` | Threshold for verbose encoding |

### RSIC Integration

| Variable | Default | Purpose |
|----------|---------|---------|
| `J17_METRICS_ENABLED` | `true` | Enable protocol metrics collection |
| `J17_CODIFICATION_THRESHOLD` | `30` | T2 send count before RSIC proposes codification |
| `J17_COMPREHENSION_MIN_THRESHOLD` | `0.7` | Comprehension rate below which code is retired |
| `J17_COMPRESSION_MIN_RATIO` | `2.0` | Compression floor before RSIC flags regression |
| `J17_REPLAY_FREQUENCY_MAX` | `5.0` | Replays/hr above which RSIC adjusts buffer |

### Data Collection & ML

| Variable | Default | Purpose |
|----------|---------|---------|
| `J17_NLI_COMPREHENSION_ENABLED` | `true` | Use NLI model for comprehension (falls back to heuristic) |
| `J17_PROTOCOL_DATA_COLLECTION` | `false` | Enable JSONL training data collection |
| `J17_EXTENSIONS_ENABLED` | `true` | Allow agent extension negotiation |
| `J17_ALLOWED_EXTENSIONS` | `tier_preference,abbreviated_ids,batch_mode,density_boost` | Permitted extensions |
| `J17_SIDECAR_URL` | `""` | Neural sidecar URL for shadow ML predictions |
| `J17_SIDECAR_TIMEOUT_MS` | `1000` | Timeout for sidecar shadow calls in ms (100ms floor; raised from 200 in DH-004) |

### Cache Session Safety

| Variable | Default | Purpose |
|----------|---------|---------|
| `JIMINY_CACHE_J17_BYPASS` | `true` | Bypass guidance cache (Get + Put) to ensure tier assignments reflect current trust |

---

## 11. Research Basis

J17's design draws from a survey of 30+ protocols and 50+ academic papers, documented in `docs/research/ai2ai/`:

| Source | Contribution to J17 |
|--------|-------------------|
| **Agora Protocol** (Oxford 2024) | Three-tier encoding scheme; 5x cost reduction demonstrated |
| **LACP** (NeurIPS 2025) | Three-layer architecture (semantic/transactional/transport), JWS signing |
| **SOAR cognitive architecture** | Three-layer enforcement (mechanical/protocol/cognitive) |
| **TLS session tickets** (RFC 5077) | Compact signed state resumption pattern |
| **Language Modeling is Compression** (ICLR 2024) | LLMs are excellent decompressors; shorter codes with semantic grounding beat full text |
| **Emergent Communication** (Lee 2019, Chaabouni 2019) | Language drift risks -- frozen codes prevent semantic drift |
| **KQML / FIPA ACL** (1990s) | Three-layer separation; failed on unverifiable mentalistic semantics |

Full research documents:
- `docs/research/ai2ai/01-existing-protocols-survey.md`
- `docs/research/ai2ai/02-compact-encoding-schemes.md`
- `docs/research/ai2ai/03-emergent-communication-research.md`
- `docs/research/ai2ai/04-state-persistence-across-resets.md`
- `docs/research/ai2ai/05-synthesis-and-candidates.md`
- `docs/research/ai2ai/06-recommendations.md`

---

## 12. Cache Policy

J17 sessions bypass the guidance cache when `JIMINY_CACHE_J17_BYPASS=true` (default). Both `cache.Get()` and `cache.Put()` are skipped, ensuring that tier assignments always reflect current trust and escalation state rather than stale cached responses.

**Rationale**: The guidance cache is keyed on `(spaceID, context)`. Without J17 session awareness, two sessions with different trust scores, escalation states, or active constraints could receive identical cached responses. Since J17 trust modulates tier selection and escalation affects content priority, cross-session cache hits would contaminate the control loop with stale or incorrect signals.

**When bypass applies**:
- `JIMINY_CACHE_J17_BYPASS=true` (default)

**When caching still applies**:
- Explicit opt-in via `JIMINY_CACHE_J17_BYPASS=false` (use only if session-specific encoding is not needed)

**Implementation**: `internal/jiminy/service.go` — the `Guide()` method checks `cfg.JiminyCacheJ17Bypass` and skips both `cache.Get()` and `cache.Put()` when the bypass flag is true.

---

## 13. Deployment Topology

The neural sidecar is designed for **localhost co-deployment** with the MDEMG server process.

```
+------------------+          +-------------------+
|  MDEMG Server    |  HTTP    |  Neural Sidecar   |
|  (Go, :9999)     |--------->|  (Python, :8100)  |
|                  | localhost |                   |
|  tier_predictor  |          |  /predict-tier    |
|  nli_scorer      |          |  /nli             |
+------------------+          +-------------------+
```

### Security Model

| Aspect | Policy | Rationale |
|--------|--------|-----------|
| **Transport** | No TLS required | Localhost-only communication; TLS adds latency and complexity without security benefit on loopback |
| **Authentication** | No auth token | Single-machine deployment; the sidecar is not exposed to the network |
| **Authorization** | None | The Go caller is the only intended client; all endpoints are read-only (inference) |
| **Network binding** | Sidecar binds `127.0.0.1:8100` | Never bind `0.0.0.0` in production — exposes ML inference to the network |

### Production Hardening

For multi-machine deployments (not currently supported, but anticipated):
1. **Network policy**: Restrict sidecar port access to the MDEMG server process only (iptables, k8s NetworkPolicy, or similar)
2. **TLS**: Add mutual TLS if sidecar is on a different host
3. **Auth**: Add bearer token validation in sidecar middleware
4. **Rate limiting**: Add per-client rate limits to prevent inference queue saturation

### Resource Requirements

The sidecar loads ML models into memory at startup:
- **NLI model** (`cross-encoder/nli-deberta-v3-xsmall`): ~100MB RAM
- **Tier prediction model** (when trained): ~80MB RAM
- **Total**: ~200MB RAM, negligible CPU when idle, CPU burst during inference

### Failure Isolation

Sidecar unavailability does not affect MDEMG server functionality:
- `TierPredictor.PredictTier()` returns `(0, 0)` → rule-based fallback
- `NLIComprehensionScorer.ScoreComprehension()` returns heuristic score
- Circuit breaker prevents repeated timeout-induced latency

---

## 14. Key Implementation Files

| File | Purpose |
|------|---------|
| `internal/jiminy/encoder.go` | Three-tier encoder, bootstrap, glossary, annotations. Tier thresholds synced from config at construction via `SetTierThresholds()` |
| `internal/jiminy/protocol.go` | Core types: SessionTicket, TicketPayload, ProtocolState |
| `internal/jiminy/ticket.go` | TicketManager: issue/validate/restore with HMAC |
| `internal/jiminy/sequence.go` | SequenceTracker: atomic counter + ring buffer replay |
| `internal/jiminy/trust.go` | TrustScorer: per-session trust modulation |
| `internal/jiminy/trust_store.go` | TrustStore: write-behind Neo4j persistence for trust state (30s flush cycle) |
| `internal/jiminy/codegen.go` | ConstraintCodeGenerator: LLM-based code generation |
| `internal/jiminy/escalation.go` | EscalationTracker: 5-level FSM with persistence |
| `internal/jiminy/protocol_metrics.go` | ProtocolMetricsCollector: tier/comprehension/compression tracking |
| `internal/jiminy/protocol_evolution.go` | ProtocolEvolver: RSIC-driven mutations |
| `internal/jiminy/nli_comprehension.go` | NLI comprehension scorer via neural sidecar |
| `internal/jiminy/protocol_data_collector.go` | JSONL training data writer |
| `internal/jiminy/extensions.go` | ExtensionRegistry: per-session extension negotiation |
| `internal/api/handlers_j17.go` | HTTP handlers for all J17 endpoints |
| `cmd/j17-comprehension-test/main.go` | Iterative comprehension benchmark + learning loop |

---

## 15. Internal LLM Caller Compression (J17-PC)

J17's proven compression utilities (`TelegraphicCompress`, `CompactJSON`, `TruncateAtWord`) are applied to the **inputs** of MDEMG's 5 highest-value internal LLM callers. This reduces aggregate token consumption by an estimated 25-35% with zero quality loss, since all compressed sections are pure prose narrative, indented JSON, or redundant instructions.

### Callers Optimized

| Caller | File | Config Variable | Est. Savings |
|--------|------|----------------|-------------|
| RSIC LLM Reflector | `internal/ape/llm_reflector.go` | `RSIC_LLM_REFLECT_COMPRESS` | 40-50% |
| LLM Reranking | `internal/retrieval/rerank.go` | `RERANK_COMPRESS` | 30-40% |
| SME Synthesis | `internal/consulting/synthesis.go` | `SYNTHESIS_COMPRESS` | 25-35% |
| Guardrail Evaluation | `internal/guardrail/prompt.go` | `GUARDRAIL_COMPRESS` | 20-30% |
| Outcome Classification | `internal/jiminy/outcome_classifier.go` | `JIMINY_CLASSIFY_COMPRESS` | 20-30% |

All default to `true`. Set to `false` to revert to uncompressed prompts.

### Compression Strategies

- **Compact JSON**: `json.Marshal` instead of `MarshalIndent` (RSIC reflector)
- **Summary truncation**: `CompressSection(summary, maxLen)` — telegraphic + word-boundary truncation (synthesis, rerank)
- **Condensed system prompts**: Separate compact constants (guardrail, classifier)
- **Redundancy removal**: Removed duplicate Task section (classifier)
- **Single-line formats**: Pipe-separated constraint/candidate formatting (guardrail, rerank)
- **Concept capping**: Organizational concepts capped at 10 in compressed mode (synthesis)
- **Verbatim preservation**: Code diffs are NEVER compressed (guardrail)

### Shared Utilities (`internal/encoding/compact.go`)

```go
CompactJSON(v any) string              // Single-line JSON marshaling
TruncateAtWord(s string, maxLen int)   // Word-boundary truncation + "..."
CompressSection(s string, maxLen int)  // TelegraphicCompress + TruncateAtWord
```

---

## 16. Neural Sidecar Promotion (Shadow to Causal)

The neural sidecar (TierPredictor + NLIComprehensionScorer) follows a four-stage rollout from shadow mode (observe-only) to active mode (ML-driven tier selection). The promotion is governed by the `SidecarArbitrator` — a decision layer between ML predictions and the encoder that controls when and how ML influences actual tier selection.

### Arbitration Modes

| Mode | ML Effect | High-Priority Items | Protected Codes |
|------|-----------|--------------------|-|
| **shadow** | None — predictions logged only | Rule-based | Rule-based |
| **compare** | None — agreement rate tracked | Rule-based | Rule-based |
| **canary** | Probabilistic routing by `J17_SIDECAR_CANARY_PERCENTAGE` | Always rule-based | Always rule-based |
| **active** | ML is primary when confidence >= floor | ML (not auto-protected) | Always rule-based |

### Causal Insertion Point

In `Guide()`, the arbitrator runs BEFORE `encoder.Encode()`:

1. For each constraint item, call `tierPredictor.PredictTier()`
2. Run `arbitrator.ArbitrateTier(item, ruleTier, mlTier, mlConf)`
3. If source == "ml", pre-assign `item.Tier = chosenTier`
4. Encoder respects pre-assigned tiers (skips `selectTier()` when `item.Tier` is valid)

### Safety Rails

- **Precedent-protected codes** (`J17_PRECEDENT_PROTECTED_CODES`): constraint codes that NEVER use ML tier in any mode. Violations are audit-logged when `J17_PRECEDENT_LOG_ENABLED=true`.
- **Confidence floor** (`J17_SIDECAR_CONFIDENCE_FLOOR`): ML predictions below this threshold fall back to rule-based.
- **Circuit breaker**: Wraps both `PredictTier()` and NLI scorer HTTP calls. On open, returns `(0, 0)` — arbitrator treats as "ML unavailable" and uses rule-based.
- **High-priority protection** (canary only): Must-level items always use rule-based tier in canary mode.

### Multi-Dimensional Feedback (NS-03)

`RecordOutcome()` now populates `FeedbackDimensions` with three signals:

| Dimension | Source | Fallback |
|-----------|--------|----------|
| Adherence | Existing `ClassifyOutcome()` | Always available |
| Comprehension | NLI scorer (when available) | Heuristic (outcome-based) |
| Applicability | Outcome-derived (followed=1.0, partial=0.5, ignored/contradicted=0.0) | Always available |

### NLI Score-of-Record (NS-02)

When `J17_NLI_SCORE_OF_RECORD=true` and sidecar mode >= canary, the NLI comprehension score replaces heuristic scoring for protocol metrics. The heuristic score is still recorded in `FeedbackDimensions` for dual-write comparison.

### Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `J17_SIDECAR_MODE` | `shadow` | Arbitration mode: shadow, compare, canary, active |
| `J17_SIDECAR_CANARY_PERCENTAGE` | `100` | % of eligible requests routed to ML in canary mode |
| `J17_SIDECAR_CONFIDENCE_FLOOR` | `0.6` | ML confidence below this falls back to rule-based |
| `J17_NLI_SCORE_OF_RECORD` | `false` | Use NLI as comprehension score-of-record |
| `J17_PRECEDENT_PROTECTED_CODES` | `""` | Comma-separated codes that never use ML tier |
| `J17_PRECEDENT_LOG_ENABLED` | `true` | Audit log for protected code ML attempts |
| `J17_SIDECAR_CB_ENABLED` | `true` | Enable circuit breaker for sidecar calls |
| `J17_SIDECAR_CB_FAILURE_THRESHOLD` | `3` | Failures before circuit opens |
| `J17_SIDECAR_CB_TIMEOUT_SEC` | `15` | Seconds before half-open probe |

### Staged Rollout

See `docs/specs/neural-sidecar-rollout-plan.md` for the full 4-stage rollout plan with entry/exit gates, ramp schedules, and automatic demotion triggers.

### Key Files

| File | Role |
|------|------|
| `internal/jiminy/sidecar_arbitrator.go` | Arbitration layer (mode logic, protected codes) |
| `internal/jiminy/sidecar_arbitrator_test.go` | 20 test cases across all modes |
| `internal/jiminy/tier_predictor.go` | HTTP client + circuit breaker for sidecar |
| `internal/jiminy/nli_comprehension.go` | NLI scorer + circuit breaker |
| `internal/jiminy/protocol_metrics.go` | Sidecar telemetry (agreement/override/latency) |
| `internal/jiminy/protocol_data_collector.go` | Expanded training records |
| `internal/config/config.go` | 9 new J17-NS config vars |

---

## Documents Accessed

- `docs/research/ai2ai/06-recommendations.md` -- Design decisions and research basis
- `docs/research/ai2ai/01-existing-protocols-survey.md` -- Protocol survey
- `docs/architecture/benchmarks/.stash/j17_comprehension_*.md` -- Baseline benchmarks
- `docs/architecture/benchmarks/.stash/j17_learning_*.md` -- Learning loop results (Cycles 1-3)
- `docs/architecture/benchmarks/j17_learning_20260322_*.json` -- Compression experiment data (Cycle 4)
- `internal/jiminy/encoder.go` -- Protocol encoder, T1 compact levels, tier selection
- `internal/jiminy/encoder_test.go` -- Encoder unit tests (updated for compact default)
- `internal/jiminy/types.go` -- GuidanceItem, GuidanceFeedbackRequest (SessionID added)
- `internal/jiminy/protocol.go` -- Core protocol types
- `internal/jiminy/ticket.go` -- Session ticket management
- `internal/jiminy/sequence.go` -- Sequence tracking + replay + Resize/BufferSize
- `internal/jiminy/sequence_test.go` -- Sequence resize tests (new)
- `internal/jiminy/trust.go` -- Trust scoring + SetThresholds
- `internal/jiminy/trust_store.go` -- Write-behind Neo4j persistence for trust state
- `internal/jiminy/trust_test.go` -- Per-session independence + SetThresholds tests
- `internal/jiminy/codegen.go` -- Code generation + UnregisterCode
- `internal/jiminy/protocol_metrics.go` -- Metrics collection
- `internal/jiminy/protocol_evolution.go` -- RSIC mutations (RetireCode Neo4j, AdjustTierThresholds real)
- `internal/jiminy/protocol_evolution_test.go` -- Updated call sites + 3 new tests
- `internal/jiminy/nli_comprehension.go` -- NLI comprehension
- `internal/jiminy/protocol_data_collector.go` -- Training data
- `internal/jiminy/extensions.go` -- Extension negotiation
- `internal/jiminy/service.go` -- GetGlossary, NewProtocolEvolver factory, RecordOutcome fix
- `internal/ape/self_assess.go` -- Protocol health scoring
- `internal/ape/self_reflect.go` -- Protocol reflection patterns
- `internal/api/handlers_j17.go` -- API endpoints (bootstrap glossary integration)
- `internal/api/server.go` -- Evolver wiring update
- `cmd/j17-comprehension-test/main.go` -- Comprehension test harness (--compact flag)
- `.claude/hooks/pre-compact.sh` -- J17_ENABLED gating
- `.claude/hooks/session-start.sh` -- J17_ENABLED gating
- `docs/api/api-spec/uats/specs/jiminy_feedback.uats.json` -- with_session_id variant
- `internal/jiminy/service.go` -- Control-loop optimization: cache bypass, CodeCoverage recording, ResumeProtocol metrics, shadow ML
- `internal/jiminy/protocol_metrics.go` -- RecordConstraintCoverage, CodeCoverage in Snapshot
- `internal/jiminy/protocol_evolution.go` -- CodifyConstraint Neo4j persistence
- `internal/config/config.go` -- JiminyCacheJ17Bypass, J17SidecarURL, J17SidecarTimeoutMs
- `internal/guardrail/prompt.go` -- lint fix (WriteString → Fprintf)
- `.claude/hooks/session-start.sh` -- cold-start bootstrap fallback
- `.env.example` -- 3 new config variables (JIMINY_CACHE_J17_BYPASS, J17_SIDECAR_URL, J17_SIDECAR_TIMEOUT_MS)
- `internal/jiminy/sidecar_arbitrator.go` -- NS-01/NS-10: Arbitration layer (shadow/compare/canary/active modes, protected codes)
- `internal/jiminy/sidecar_arbitrator_test.go` -- 20 test cases for arbitrator
- `internal/jiminy/types.go` -- NS-03: FeedbackDimensions struct
- `internal/jiminy/protocol_data_collector.go` -- NS-14: Expanded training record with ML fields
- `internal/jiminy/protocol_metrics.go` -- NS-07: SidecarMetrics, RecordSidecarCall
- `internal/config/config.go` -- 9 new J17-NS config vars (sidecar mode, canary, CB, protected codes)
- `docs/specs/neural-sidecar-contract-v1.md` -- NS-04: Sidecar API contract
- `docs/specs/neural-sidecar-ml-objectives.md` -- NS-05: ML objective function
- `docs/specs/neural-sidecar-benchmark-protocol.md` -- NS-13: Benchmark protocol
- `docs/specs/neural-sidecar-rollout-plan.md` -- NS-15: Staged rollout plan
- `docs/api/api-spec/uats/specs/j17_metrics.uats.json` -- code_coverage assertion

## 17. NLI Feedback Loop: Tier Effectiveness

Closes the feedback loop from NLI comprehension scoring back to protocol tier selection via RSIC.

### Problem Solved (6 Gaps)

1. **NLI scores didn't reach protocol metrics in shadow/compare mode** — NLI data was computed but only recorded in causal mode. Fixed: observational recording in all modes.
2. **No per-tier comprehension tracking** — `RecordOutcome` took no tier parameter. Fixed: `RecordOutcomeWithTier` records per-tier, per-code scores.
3. **AdjustTierThresholds ignored comprehension** — adjusted on T1 distribution only. Fixed: comprehension-aware threshold adjustment.
4. **No tier-level drift detection in RSIC** — per-code drift existed but not per-tier. Fixed: `j17_tier_ineffective` pattern (#15).
5. **No NLI calibration tracking** — no detection of systematic NLI bias. Fixed: ring-buffer calibration tracker, `j17_nli_calibration_drift` pattern (#16).
6. **No curated RSIC dataset** — only raw JSONL training data. Fixed: `TierEffectivenessDataset` with grading, drift, and recommendations.

### Architecture

```
NLI Score → RecordOutcomeWithTier → ProtocolMetrics.Snapshot()
                                         ↓
                              GradeTierEffectiveness()
                                         ↓
                              RSIC Reflect (pattern #15)
                                         ↓
                              AdjustTierThresholds (comprehension-aware)
                                         ↓
                              BuildTierEffectivenessDataset (meso/macro cycles)
```

### Config Vars

| Variable | Default | Purpose |
|----------|---------|---------|
| `J17_NLI_OBSERVATIONAL_ENABLED` | `true` | NLI scores flow to metrics in all modes |
| `J17_TIER_EFFECTIVENESS_MIN_SAMPLES` | `5` | Min outcomes per tier/code before grading |
| `J17_TIER_INEFFECTIVE_THRESHOLD` | `0.6` | Comprehension below this = ineffective |
| `J17_TIER_DRIFT_DETECTION_ENABLED` | `true` | Enable tier drift RSIC pattern |
| `J17_NLI_CALIBRATION_WINDOW_SIZE` | `500` | Calibration ring buffer size |
| `J17_NLI_CALIBRATION_BIAS_THRESHOLD` | `0.15` | Max NLI-vs-heuristic bias |

### Key Files

- `internal/jiminy/protocol_metrics.go` -- RecordOutcomeWithTier, per-tier snapshot fields
- `internal/jiminy/tier_effectiveness.go` -- GradeTierEffectiveness
- `internal/jiminy/tier_effectiveness_dataset.go` -- BuildTierEffectivenessDataset, CollectDataset
- `internal/jiminy/nli_calibration.go` -- NLICalibrationTracker, ring buffer, Report()
- `internal/jiminy/protocol_evolution.go` -- Comprehension-aware AdjustTierThresholds
- `internal/jiminy/service.go` -- NLI gate fix (double-counting), BuildTierEffectivenessDataset, GetNLICalibrationReport
- `internal/ape/self_reflect.go` -- Patterns #15 (tier_ineffective) and #16 (nli_calibration_drift)
- `internal/ape/self_assess.go` -- NLI calibration weight in scoreProtocol
- `internal/ape/cycle.go` -- Dataset generation at meso/macro boundaries
- `internal/api/handlers_j17.go` -- GET /v1/jiminy/protocol/tier-effectiveness
- `internal/api/rsic_adapters.go` -- Per-tier fields + calibration in protocol adapter
- `docs/api/api-spec/uats/specs/j17_tier_effectiveness.uats.json` -- Contract test
