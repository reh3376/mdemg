# J17 Design Recommendations

**Date**: 2026-03-21
**Author**: Claude (Opus 4.6)
**Status**: Draft for review — awaiting user input before planning

---

## Executive Summary

After surveying 30+ protocols, 50+ academic papers, and dozens of open source projects, my recommendation is **Option C: Hybrid — Designed Core + Negotiated Extensions**, building primarily on concepts from **Agora Protocol** (tiered encoding), **TLS session tickets** (state persistence), and **SOAR cognitive architecture** (three-layer enforcement).

No existing protocol can be adopted as-is. But we don't need to build from scratch either — the building blocks exist and are proven.

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
- **Agora's three-tier encoding** applied to the core protocol
- **TLS-style session tickets** for context reset survival
- **Graceful degradation** — if negotiation fails, core protocol still works in full natural language (NL)

---

## Specific Design Choices and Reasoning

### 1. Session Ticket Architecture (from TLS + JWT)

**Choice**: Compact (~500 byte), signed, opaque state blob issued by Jiminy at pre-compaction and re-injected at session start.

**Why this over alternatives**:
- **Better than full state replay**: Replay costs too many tokens. A compact ticket carries just enough to resume.
- **Better than relying on CMS recall alone**: CMS recall is semantic (similarity-based). Session state is exact — you need the precise escalation count, not "something similar to an escalation."
- **Better than storing state in markdown memory**: Memory files are suggestions; the agent can ignore them. A session ticket processed by Jiminy hooks is mechanical — the agent can't opt out.

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
The `C:!` / `C:?` / `X:!` prefix is all Claude needs to understand severity. The content is a compressed constraint name (not a full English sentence). Source node ID enables traceability. This format is:
- **Human-readable** (satisfies R7 auditability)
- **Machine-parseable** (hooks can validate format)
- **Compact** (~15 tokens vs ~50-100 tokens for natural language equivalent)
- **Deterministic** (same constraint always produces same code — cacheable)

**Tier 2 — Telegraphic guidance** (~50-100 tokens, 15% of traffic):
```
Auth middleware rewrite: compliance-driven not tech-debt. Favor compliance over ergonomics. (Node: n1234)
```
Drops articles, pronouns, filler. Still natural language but compressed. Used for context-specific guidance where a code wouldn't capture the nuance.

**Tier 3 — Full natural language narrative** (current synthesis output, 5% of traffic):
Only for truly novel situations where the LLM synthesizer needs to reason about multiple conflicting constraints. This is what J8/J15 synthesis already produces.

**Why not go more compact?** The research on binary/latent encoding (doc 02) shows that since Claude processes text tokens (not binary), wire-level compression provides no benefit. The compression must be semantic. And going *too* compact (single-character codes with no mnemonic value) would sacrifice auditability and graceful degradation.

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

### 6. Negotiable Extensions (Not Core)

**Choice**: Agent can request (not demand) encoding optimizations.

**Examples of negotiable extensions**:
- "I understand your constraint codes. Send Tier 1 only, skip Tier 2/3 unless novel." → Reduces injection size
- "Batch constraints per-file instead of per-prompt when I'm doing multi-file edits." → Reduces frequency
- "Use abbreviated node IDs (first 8 chars) — I can look up full IDs if needed." → Saves ~5 tokens per constraint

**What is NOT negotiable**:
- Constraint types and severity codes
- Escalation state machine transitions
- Session ticket format and lifecycle
- Sequence numbering

**Why allow negotiation at all?** The "Language Modeling is Compression" paper (ICLR 2024) establishes that Claude is an excellent decompressor. If Claude signals it understands the constraint vocabulary, we can send even more compressed codes and rely on Claude to reconstruct the full meaning. This is provably more efficient. But the choice to compress further must be Claude's — Jiminy shouldn't assume.

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

---

## Implementation Effort Estimate

| Component | Complexity | New Files | Modified Files |
|-----------|-----------|-----------|----------------|
| Session ticket (checkpoint/resume endpoints) | Medium | 2-3 | 3-4 |
| Constraint code encoder/decoder | Low | 1-2 | 2-3 |
| Tier selection logic (which tier for which guidance) | Low | 0 | 2-3 |
| Sequence counter + event log | Medium | 1-2 | 3-4 |
| Hook updates (pre-compact checkpoint, session-start resume) | Low | 0 | 2-3 |
| Trust score persistence | Low | 0 | 2-3 |
| Extension negotiation handshake | Medium | 1-2 | 2-3 |
| Tests | Medium | 2-3 | 2-3 |
| UATS specs | Low | 3-5 | 0 |

**Total estimate**: ~10-15 new/modified Go files, ~5 UATS specs, ~3 hook updates.

---

## Open Questions for Discussion

1. **Should the session ticket be encrypted or just signed?** ~~Encryption prevents the agent from reading its own state (which might be useful if we want to hide trust scores). Signing alone is simpler and still prevents tampering.~~ **RESOLVED**: Signing only, no encryption. Nothing in the AI-to-AI communication is easily exploitable, and transparency is a design goal — both agents should see communication clearly without concern about secondary intent. Signing provides tamper protection, which is all we need.

2. **How should we handle the first session (no prior ticket)?** ~~Options: (a) full natural language guidance on first session, (b) bootstrap ticket from CMS observations, (c) start with empty ticket and build up.~~ **RESOLVED**: Option (c) with a compact spec header. Full natural language is unnecessary — any well-trained LLM natively understands structured specification formats (RFC 2119 keywords, key:value notation, markdown hierarchy) because these are heavily represented in training data. The bootstrap sends a ~50 token protocol spec block alongside an empty state ticket:
   ```
   J17:INIT|v1
   CODES: C=constraint X=correction F=frontier D=decision
   SEV: !=must ?=should ~=info
   ESC: 0=clear 1=warned 2=escalated 3=blocked
   FMT: TYPE:SEV|content|esc:N|src:NODE_ID
   TICKET: signed, opaque, echo back on resume
   SEQ: monotonic, report last_seq on resume
   ```
   This is Agora's Tier 2 (telegraphic) applied to the bootstrap itself. Per "Language Modeling is Compression" (ICLR 2024): send the minimum viable signal, let the LLM reconstruct full meaning. No negotiation round needed — Jiminy starts sending Tier 1 codes immediately.

3. **Should Tier 1 codes be human-designed or LLM-generated?** ~~We could have Jiminy's LLM generate optimal codes from the constraint content, or we could design them by hand. LLM-generated would be more consistent; hand-designed would be more predictable.~~ **RESOLVED**: LLM-generated once at constraint creation time, then frozen. When a new constraint enters the CMS graph, Jiminy's LLM generates a compact kebab-case mnemonic code (e.g., `no-force-push-main` from "Never force push to the main branch"). The code is stored as a property on the constraint node in Neo4j and never regenerated. This gives: (1) determinism without rigidity — codes are static DB lookups, not runtime LLM output, surviving model upgrades and provider switches; (2) automatic scaling — no human bottleneck at constraint 301; (3) mnemonic quality — self-documenting codes that an LLM can roughly interpret even without the bootstrap spec (`C:!|test-before-commit` is self-evident); (4) no drift — frozen at creation, immune to model changes; (5) collision prevention — generation prompt includes existing codes as negative examples. This is the URL-slug/npm-package-name pattern: human-readable, machine-parseable, generated once, then immutable.

4. **What's the right ticket TTL?** ~~Too short = frequent re-handshakes. Too long = stale state. Candidates: 1 hour, 4 hours, 24 hours, session-based (until server restart).~~ **RESOLVED**: Event-driven renewal with a 4-hour safety-net TTL. Time-based expiration is the wrong primary axis — state accuracy matters more than elapsed time. A 3-hour-old ticket reflecting current reality is fine; a 5-minute-old ticket generated before an escalation event is dangerous. Ticket renewal is cheap (serialize + sign, no LLM call), so renew aggressively on events:

   | Trigger | Why |
   |---------|-----|
   | Pre-compaction | Primary mechanism — bridge across context amnesia (~every 20-30 min) |
   | Context reset / new session | Clean slate — full state re-injection needed |
   | Agent change (new model/instance) | New agent hasn't earned prior trust score |
   | Escalation state transition | Most dangerous if stale — violation history must persist immediately |
   | Constraint set mutation | Ticket's active constraint list is stale |

   The 4-hour TTL is a backstop only — in normal operation, event-driven renewal refreshes every 20-30 minutes. The TTL catches the failure case where all event triggers break simultaneously. Defense in depth, like the circuit breaker pattern.

5. **Should we implement LLMLingua-2 compression as a quick win before J17?** ~~The research shows 2-5x natural language compression with zero protocol changes. Could ship in days, not weeks.~~ **RESOLVED**: No. Focus effort on J17 — the full protocol is the high-value target. LLMLingua-2 becomes redundant once Tier 1 coded constraints (5-10x reduction) are in place.

---

*All open questions resolved. Ready for J17 development planning.*
