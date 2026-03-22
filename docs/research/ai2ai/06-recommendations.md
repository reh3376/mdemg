# J17 Design Recommendations — Final Draft

**Date**: 2026-03-21
**Author**: Claude (Opus 4.6)
**Status**: Final draft — all open questions resolved, ready for development planning

---

## Executive Summary

After surveying 30+ protocols, 50+ academic papers, and dozens of open source projects, the recommendation is **Option C: Hybrid — Designed Core + Negotiated Extensions**, building primarily on concepts from **Agora Protocol** (tiered encoding), **TLS session tickets** (state persistence), **SOAR cognitive architecture** (three-layer enforcement), and **RSIC** (protocol evolution through self-improvement).

No existing protocol can be adopted as-is. But we don't need to build from scratch — the building blocks exist and are proven.

**Design axioms** (from user review):
1. **No artificial size constraints** — the protocol's purpose is compact communication; artificial limits add complexity without value
2. **AI-to-AI first** — human readability is not a design goal; auditability is achieved through logging/tooling, not wire format constraints
3. **Living protocol** — RSIC-driven evolution of negotiable extensions, not static specification
4. **Transparency** — both agents see all communication clearly; signing for tamper protection, no encryption

---

## Why Option C (Hybrid) Over the Others

### Why not Option A (Pure Agora Adaptation)?

Agora's three-tier encoding is excellent and we should use it. But Agora has no concept of:
- **Guardrail enforcement** — it's a general agent communication protocol, not a safety/guidance system
- **Context reset survival** — it assumes persistent connections
- **Escalation state machines** — it treats all messages as equal

Adapting Agora alone would require bolting on so many J17-specific features that we'd essentially be building Option C anyway, just with Agora's name on it.

### Why not Option B (Fully Negotiated)?

The emergent communication research (doc 03) proves LLMs *can* negotiate shared codes. But:

1. **Language drift is real** (Lee et al. 2019) — negotiated protocols diverge from interpretability without grounding constraints
2. **Anti-efficiency** (Chaabouni et al. 2019) — agents don't automatically optimize for compression; they need explicit inductive biases
3. **Guardrails can't be negotiable** — the whole point of Jiminy is enforcement. Letting the agent negotiate constraint encoding creates a vector for weakening constraints
4. **Re-negotiation cost** — after every context reset, the agent would need to re-negotiate or re-learn the protocol. That costs tokens and latency every ~20-30 minutes

### Why Option C works

Option C gives us:
- **Non-negotiable core** for guardrail enforcement (constraints, escalation, state persistence) — these are SOAR's "compiled production rules" that fire mechanically
- **Negotiable extensions** for efficiency optimization — the agent can request shorter codes, different verbosity, batching preferences
- **RSIC-driven evolution** — extensions aren't static; they're measured, evaluated, and improved through the self-improvement cycle
- **Agora's three-tier encoding** applied to the core protocol
- **TLS-style session tickets** for context reset survival
- **Graceful degradation** — if negotiation fails, core protocol still works in full natural language (NL)

---

## Specific Design Choices and Reasoning

### 1. Session Ticket Architecture (from TLS + JWT)

**Choice**: Compact, signed state blob issued by Jiminy on event triggers and re-injected at session start. No artificial size limit — the ticket carries exactly what's needed to resume, no more, no less.

**Why no size cap**: The ticket contents are inherently bounded (finite constraint set, escalation state enum, sequence counter, trust score float, conversation phase string). The protocol's tiered encoding already ensures compactness. An artificial byte limit adds edge-case complexity (truncation logic, overflow handling) without solving a real problem. The natural constraint is context window capacity, and the ticket is a small fraction of that.

**Why this over alternatives**:
- **Better than full state replay**: Replay costs too many tokens. A compact ticket carries just enough to resume.
- **Better than relying on CMS recall alone**: CMS recall is semantic (similarity-based). Session state is exact — you need the precise escalation count, not "something similar to an escalation."
- **Better than storing state in markdown memory**: Memory files are suggestions; the agent can ignore them. A session ticket processed by Jiminy hooks is mechanical — the agent can't opt out.

**Signing**: HMAC-SHA256 signature for tamper detection. No encryption — transparency is a design goal. Both agents see all communication clearly without concern about secondary intent.

**Renewal**: Event-driven with a 4-hour safety-net TTL. Ticket renewal is cheap (serialize + sign, no LLM call), so renew aggressively:

| Trigger | Why |
|---------|-----|
| Pre-compaction | Primary mechanism — bridge across context amnesia (~every 20-30 min) |
| Context reset / new session | Clean slate — full state re-injection needed |
| Agent change (new model/instance) | New agent hasn't earned prior trust score |
| Escalation state transition | Most dangerous if stale — violation history must persist immediately |
| Constraint set mutation | Ticket's active constraint list is stale |

The 4-hour TTL is a backstop only — catches the failure case where all event triggers break simultaneously.

**Implementation**: The `pre-compact.sh` hook already fires before compaction. It would call a new `POST /v1/jiminy/checkpoint` endpoint that returns a signed ticket. The `session-start.sh` hook would send this ticket back via `POST /v1/jiminy/resume-protocol` to restore state.

### 2. Three-Tier Encoding (from Agora)

**Choice**: Coded constraints (Tier 1) > Telegraphic guidance (Tier 2) > Full NL fallback (Tier 3).

**Why this specific tiering**:

**Tier 1 — Coded constraints** (~15 tokens each, 80% of traffic):
```
C:!|no-force-push-main|esc:0|src:abc123
X:!|never-stash-for-goreleaser|src:def456
C:?|test-before-commit|esc:1|src:ghi789
```
The `C:!` / `C:?` / `X:!` prefix encodes type and severity in 3 characters. The content is an LLM-generated mnemonic code (frozen at constraint creation). Source node ID enables traceability. This format is:
- **Machine-parseable** (hooks can validate format)
- **Compact** (~15 tokens vs ~50-100 tokens for natural language equivalent)
- **Deterministic** (same constraint always produces same code — cacheable)
- **Self-documenting to LLMs** (mnemonic codes like `test-before-commit` are interpretable without a lookup table)

**Code generation**: LLM-generated once at constraint creation time, then frozen as a property on the constraint node in Neo4j. Never regenerated. Generation prompt includes existing codes for collision prevention. This is the URL-slug pattern: machine-parseable, generated once, immutable.

**Tier 2 — Telegraphic guidance** (~50-100 tokens, 15% of traffic):
```
Auth middleware rewrite: compliance-driven not tech-debt. Favor compliance over ergonomics. (Node: n1234)
```
Drops articles, pronouns, filler. Still natural language but compressed. Used for context-specific guidance where a code wouldn't capture the nuance.

**Tier 3 — Full natural language narrative** (current synthesis output, 5% of traffic):
Only for truly novel situations where the LLM synthesizer needs to reason about multiple conflicting constraints. This is what J8/J15 synthesis already produces.

**Encoding density**: Since this is AI-to-AI communication, not human-facing, encoding density can be pushed further than traditional protocols. The research on binary/latent encoding (doc 02) shows compression must be semantic (LLMs process text tokens, not binary), but within that constraint, shorter codes and denser formats are preferred. RSIC tracks comprehension accuracy and can recommend further compression when the agent demonstrates reliable interpretation.

### 3. Monotonic Sequence Counter (from SSE Last-Event-ID)

**Choice**: Every guidance event gets a monotonic sequence number. Session ticket includes `last_seq`. After compaction, Jiminy replays events since `last_seq`.

**Why this matters**: Without it, there's a window between the last guidance injection and compaction where guidance could be lost. The agent might receive a critical constraint warning at sequence 47, then compaction hits before the next prompt. Without the sequence counter, that warning is gone. With it, Jiminy replays from 47 on resume.

### 4. SOAR-Inspired Three-Layer Enforcement

**Choice**: Three enforcement layers with different persistence characteristics.

| Layer | Mechanism | Survives Compaction? | Example |
|-------|-----------|---------------------|---------|
| L1: Mechanical | Hook scripts (pre-bash-check.py) | Always (hooks are files) | Block `rm -rf`, block force-push |
| L2: Protocol | Session ticket + coded constraints | Via ticket re-injection | Active constraint set, escalation state |
| L3: Cognitive | Full Jiminy LLM reasoning | Via CMS recall | Novel guidance, conflict resolution |

**Why three layers instead of one**: Defense in depth. If L3 fails (LLM timeout), L2 still enforces known constraints. If L2 fails (ticket corruption), L1 still blocks dangerous commands. This is the same principle as NeMo Guardrails: **external enforcement over prompt instructions**.

### 5. Trust Score and Escalation Persistence

**Choice**: Session ticket carries a `trust_score` (0.0-1.0) and `escalation_state` that persist across compaction.

**Why**: Currently, escalation state (J12) lives in-memory on the server. If the server restarts, or if the agent gets a fresh context, escalation history is lost. The agent effectively gets a "clean slate" every compaction — it can ignore 5 constraints, get compacted, and start fresh at 0 ignores.

With J17, the escalation count persists in the session ticket. The agent can't "reset" Jiminy by waiting for compaction. This closes a real enforcement gap.

### 6. RSIC-Driven Protocol Evolution (Negotiable Extensions)

**Choice**: Agent can request (not demand) encoding optimizations. RSIC measures effectiveness and drives protocol evolution over time. The protocol learns and improves — it is not static.

**Examples of negotiable extensions**:
- "I understand your constraint codes. Send Tier 1 only, skip Tier 2/3 unless novel." → Reduces injection size
- "Batch constraints per-file instead of per-prompt when I'm doing multi-file edits." → Reduces frequency
- "Use abbreviated node IDs (first 8 chars) — I can look up full IDs if needed." → Saves ~5 tokens per constraint

**What is NOT negotiable**:
- Constraint types and severity codes
- Escalation state machine transitions
- Session ticket lifecycle and signing
- Sequence numbering

**RSIC integration — the protocol as a learning system**:

RSIC already monitors MDEMG's retrieval quality, memory health, and learning edges. J17 extends this to protocol effectiveness:

| RSIC Function | J17 Application |
|---------------|-----------------|
| **Assess** | Track protocol metrics: tier distribution (% T1/T2/T3), compression ratios, agent comprehension accuracy, replay frequency after compaction, extension adoption rates |
| **Reflect** | Identify patterns: constraints frequently sent as T2 that could be codified to T1; extension requests repeated across sessions; codes that correlate with agent misinterpretation |
| **Plan** | Propose protocol mutations: new T1 codes for high-frequency guidance, tier threshold adjustments, batching optimizations, encoding density changes |
| **Dispatch** | Execute approved mutations: generate and freeze new constraint codes, update tier selection logic, adjust compression parameters |
| **Monitor** | Track outcomes of mutations: did the new code improve comprehension? Did the batching change reduce overhead without losing enforcement? |
| **Calibration** | Adjust confidence in protocol parameters: which extensions reliably improve efficiency vs. which introduce comprehension risk |
| **Watchdog** | Detect protocol degradation: comprehension accuracy dropping, tier distribution skewing unexpectedly, replay frequency increasing (suggests ticket renewal triggers are failing) |

**Evolution lifecycle**:
1. RSIC observes protocol performance across sessions
2. Identifies optimization opportunities (e.g., "constraint X has been sent as Tier 2 in 47 of the last 50 sessions — candidate for Tier 1 codification")
3. Proposes mutation (generate T1 code, update tier selection)
4. Mutation is applied and monitored
5. If comprehension accuracy holds or improves, mutation becomes permanent
6. If comprehension degrades, mutation is rolled back

This closes the loop: the protocol doesn't just communicate — it learns to communicate better over time, grounded in measured outcomes rather than assumptions.

**Why allow negotiation at all?** The "Language Modeling is Compression" paper (ICLR 2024) establishes that Claude is an excellent decompressor. If Claude signals it understands the constraint vocabulary, we can send even more compressed codes and rely on Claude to reconstruct the full meaning. This is provably more efficient. But the choice to compress further must be Claude's — Jiminy shouldn't assume.

---

## Bootstrap Protocol (First Session)

When no prior ticket exists, the session-start hook sends a compact protocol spec block (~50 tokens) alongside an empty state ticket:

```
J17:INIT|v1
CODES: C=constraint X=correction F=frontier D=decision
SEV: !=must ?=should ~=info
ESC: 0=clear 1=warned 2=escalated 3=blocked
FMT: TYPE:SEV|content|esc:N|src:NODE_ID
TICKET: signed, echo back on resume
SEQ: monotonic, report last_seq on resume
```

Full natural language is unnecessary — any well-trained LLM natively understands structured specification formats (RFC 2119 keywords, key:value notation) because these are heavily represented in training data. Per "Language Modeling is Compression" (ICLR 2024): send the minimum viable signal, let the LLM reconstruct full meaning. No negotiation round needed — Jiminy starts sending Tier 1 codes immediately.

---

## What We DON'T Need (And Why)

| Idea from Research | Why We Skip It |
|-------------------|----------------|
| Latent space communication (CIPHER, Interlat, LatentMAS) | Requires model internals access. We communicate via API text. |
| Full A2A protocol | Designed for agent marketplaces, not persistent guidance. Massive overhead. |
| Emergent language via self-play | Drift risk too high for safety-critical guardrails. |
| Binary serialization (Protobuf, MessagePack) | LLM processes text tokens, not binary. No benefit. |
| Full FIPA ACL | Failed in practice. Mental-state semantics are unverifiable. |
| KV cache sharing (DroidSpeak) | Requires same base model. Different architecture. |
| LLMLingua-2 compression | Redundant once Tier 1 coded constraints (5-10x reduction) are in place. |
| Artificial size constraints | Protocol's purpose is already compact communication; limits add edge-case complexity without value. |

---

## Implementation Effort Estimate

| Component | Complexity | New Files | Modified Files |
|-----------|-----------|-----------|----------------|
| Session ticket (checkpoint/resume endpoints, signing) | Medium | 2-3 | 3-4 |
| Constraint code generator (LLM-generated, frozen in Neo4j) | Medium | 1-2 | 2-3 |
| Tier selection logic (which tier for which guidance) | Low | 0 | 2-3 |
| Sequence counter + event log | Medium | 1-2 | 3-4 |
| Hook updates (pre-compact checkpoint, session-start resume) | Low | 0 | 2-3 |
| Trust score + escalation persistence | Low | 0 | 2-3 |
| Extension negotiation handshake | Medium | 1-2 | 2-3 |
| RSIC protocol metrics + evolution pipeline | Medium | 2-3 | 3-4 |
| Bootstrap protocol (init spec block) | Low | 0-1 | 1-2 |
| Tests | Medium | 3-4 | 3-4 |
| UATS specs | Low | 4-6 | 0 |

**Total estimate**: ~12-18 new/modified Go files, ~6 UATS specs, ~3 hook updates.

---

## Resolved Design Decisions

| # | Question | Resolution |
|---|----------|------------|
| Q1 | Encrypted or signed tickets? | **Signed only** (HMAC-SHA256). Transparency is a design goal — no encryption needed. |
| Q2 | First session bootstrap? | **Compact spec header** (~50 tokens) + empty state ticket. No full NL needed. |
| Q3 | Human-designed or LLM-generated codes? | **LLM-generated once, frozen in Neo4j.** Scales automatically, deterministic, no drift. |
| Q4 | Ticket TTL? | **Event-driven renewal** (compaction, reset, escalation, mutation) + **4-hour safety-net TTL**. |
| Q5 | LLMLingua-2 as quick win? | **No.** Focus on J17 full protocol — LLMLingua-2 becomes redundant. |
| Q6 | Size constraints on tickets/messages? | **None.** Protocol design ensures compactness; artificial limits add complexity without value. |
| Q7 | Human readability as design factor? | **No.** AI-to-AI communication — auditability via logging/tooling, not wire format. |
| Q8 | Static or evolving protocol? | **Evolving.** RSIC tracks effectiveness and drives protocol mutations through measured learning. |

---

*All design decisions resolved. This document is the basis for J17 development planning.*
