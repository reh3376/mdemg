# J17 Research Synthesis: Candidate Protocols and Design Options

**Research Date**: 2026-03-21
**Purpose**: Evaluate existing protocols/projects against J17 requirements — identify adopt, adapt, or build-from-scratch path
**Researcher**: Claude (Opus 4.6)

---

## J17 Requirements Checklist

| # | Requirement | Weight |
|---|-------------|--------|
| R1 | Survives context resets / auto-compact | Critical |
| R2 | Constant background communication (hooks fire on every prompt + tool use) | Critical |
| R3 | Guardrail enforcement persistence (constraints can't be "forgotten") | Critical |
| R4 | Token-efficient (minimal context window cost) | High |
| R5 | Works over HTTP API (Jiminy is a Go server, agent is Claude via API) | High |
| R6 | Negotiable/evolvable encoding (not hardcoded English) | Medium |
| R7 | Auditable (humans can inspect communication if needed) | Medium |
| R8 | Graceful degradation (falls back to NL if protocol breaks) | Medium |
| R9 | Escalation state persistence across sessions | High |

---

## Candidate Evaluation Matrix

### Tier 1: Existing Protocols (Adopt or Adapt?)

| Protocol | R1 | R2 | R3 | R4 | R5 | R6 | R7 | R8 | R9 | Verdict |
|----------|----|----|----|----|----|----|----|----|----|----|
| **Agora** | Partial | No | No | Yes (5x) | Yes | Yes | Yes | Yes | No | **Best baseline to build on** |
| **LACP** | No | No | No | Partial | Yes | No | Yes | No | No | Good message taxonomy only |
| **A2A** | Partial | No | No | No (verbose) | Yes | No | Yes | Yes | Partial | Too heavyweight, wrong use case |
| **MCP** | No | No | No | Partial | Yes | No | Yes | No | No | Already used, but vertical not horizontal |
| **REP** | No | No | Partial | Yes | Yes | No | Yes | No | No | Sensitivity concept is valuable |

**Verdict**: No existing protocol meets J17 requirements out of the box. However, **Agora** is the strongest baseline — its three-tier encoding (structured/LLM-written/NL) directly maps to our use case, and its 5x cost reduction is proven. We would need to add: state persistence (R1/R9), hook integration (R2), and guardrail enforcement semantics (R3).

### Tier 2: Architectural Patterns (Incorporate)

| Pattern | Source | J17 Application |
|---------|--------|-----------------|
| **TLS Session Tickets** | RFC 5077/8446 | Opaque state blob survives compaction (R1) |
| **SSE Last-Event-ID** | Server-Sent Events spec | Monotonic sequence counters for gap detection (R1) |
| **JWT Self-Contained Tokens** | REST/OAuth | Every request carries full state (R1) |
| **SOAR Three-Layer** | Cognitive architecture | Automatic/Deliberative/Impasse layers (R3) |
| **ACT-R Activation Decay** | Cognitive architecture | Relevance-weighted constraint injection (R4) |
| **Event Sourcing** | Microservices pattern | Snapshot + delta replay (R1, R9) |
| **Blackboard Architecture** | HEARSAY-II | Shared state space Jiminy maintains (R1, R3) |

### Tier 3: Encoding Research (Inform Design)

| Research | Key Finding | J17 Implication |
|----------|-------------|-----------------|
| "Language Modeling is Compression" | LLMs are excellent decompressors | Send minimum viable signal, let Claude reconstruct |
| Anti-Efficient Encoding (Chaabouni) | Agents don't auto-compress by frequency | Must explicitly design frequency-based encoding |
| Emergent Machine Language | LLMs develop shared codes in 4 rounds | Protocol negotiation is feasible between LLMs |
| "Why Do AI Agents Speak English?" | NL is lossy projection from vector space | Structured encoding avoids NL overhead |
| LLMLingua-2 | 2-5x compression of NL text | Quick win for current guidance before full protocol |

---

## Three Design Options

### Option A: Agora-Inspired Tiered Protocol + Session Tickets

**Approach**: Adapt Agora's three-tier encoding with TLS-style session tickets and event sourcing for state persistence. Build on proven patterns.

**Tiers**:
1. **Coded constraints** (most frequent): Single-line structured codes for the ~30 most common constraint patterns
   - `C:MUST|no-force-push|src:n1234` (constraint, must type, source node)
   - `C:WARN|test-first|esc:2|src:n5678` (constraint, warning, escalation count 2)
   - `X:CORR|dont-mock-db|src:n9012` (correction, source node)
2. **Telegraphic guidance** (moderate frequency): Compressed NL for context-specific guidance
   - "Auth middleware rewrite: compliance-driven, not tech-debt. Favor compliance over ergonomics."
3. **Full NL narrative** (rare): Only for novel, complex situations requiring detailed reasoning

**State persistence**:
- Pre-compaction hook saves session ticket to CMS
- Ticket = compact signed blob (~500 bytes): active constraints, escalation state, last_seq, trust score, phase
- Session-start hook fetches ticket + replays missed guidance events since last_seq
- Event-sourced guidance log with snapshots

**Pros**: Proven 5x cost reduction. Grounded in real research. Incremental — can start with Tier 1 codes and add tiers.
**Cons**: Requires defining the code vocabulary. Not truly negotiated — designed by us.

### Option B: Negotiated Protocol via Handshake

**Approach**: At session start, Jiminy and the agent negotiate a compact encoding through a brief handshake exchange. The protocol spec is stored in CMS and re-sent after compaction.

**Handshake flow**:
1. Jiminy sends protocol proposal (compact encoding rules, ~20 lines)
2. Agent acknowledges understanding and may request clarifications
3. Protocol spec stored as pinned CMS observation
4. After compaction, protocol spec re-injected as part of session ticket
5. Both parties use negotiated encoding for subsequent communication

**Example negotiated protocol**:
```
JIMINY PROTOCOL v1 — Acknowledge by echoing "J17:ACK"
Codes: C=constraint, X=correction, F=frontier, D=decision
Severity: !=must, ?=should, ~=info
Escalation: 0=clear, 1=warned, 2=escalated, 3=blocked
Format: TYPE:SEVERITY|content|esc:N|src:NODE_ID
Example: C:!|no-force-push-main|esc:0|src:abc123
Respond with: J17:ACK|understood:N (N = count of codes understood)
```

**Pros**: Agent can optimize encoding for its own comprehension. Negotiation proves understanding. Extensible.
**Cons**: Handshake costs tokens. Must re-negotiate or re-inject after every compaction. Risk of drift.

### Option C: Hybrid — Designed Core + Negotiated Extensions

**Approach**: Fixed core protocol (designed by us, non-negotiable) for guardrail enforcement + negotiable extensions for optional guidance types. Best of both worlds.

**Fixed core** (never negotiated, mechanically enforced):
- Session ticket format and lifecycle
- Constraint code vocabulary (C/X/F/D types, severity codes)
- Escalation state machine
- Sequence numbering and replay semantics

**Negotiable extensions** (agent can request optimizations):
- Encoding density (agent can request shorter codes if it demonstrates understanding)
- Guidance verbosity level (agent can request more/less detail)
- Communication frequency (agent can request batched vs. per-prompt guidance)
- Domain-specific shorthand (project-specific abbreviations)

**State persistence** (same as Option A):
- TLS-style session tickets
- Event-sourced guidance log
- Monotonic sequence counters

**Pros**: Core guarantees are never at risk. Extensions allow efficiency optimization. Agent participation builds trust model. Future-proof — new encoding can be negotiated without protocol redesign.
**Cons**: More complex to implement. Need clear boundary between core and extensions.

---

## Recommendation

**Option C (Hybrid)** is the strongest path. Here's why:

1. **R1/R3 (persistence/enforcement)** require non-negotiable mechanical guarantees — you can't let the agent "negotiate away" guardrails
2. **R4/R6 (efficiency/evolvability)** benefit from negotiation — the agent knows best how to compress for its own comprehension
3. **R7 (auditability)** is maintained because the core protocol is human-readable and the extensions are documented in CMS
4. **R8 (graceful degradation)** is built in — if negotiation fails, the core protocol still works

The implementation path:
1. **Phase 1**: Design and implement the fixed core protocol (session tickets, constraint codes, escalation state machine, sequence numbering)
2. **Phase 2**: Add negotiable extensions (encoding density, verbosity control, batching)
3. **Phase 3**: Add protocol evolution (agent requests new shorthand codes that get stored in CMS for future sessions)

---

## Existing Projects to Build On

| Project | What to Use | What to Skip |
|---------|-------------|-------------|
| **Agora** (agoraprotocol.org) | Three-tier encoding concept, Protocol Document hash-based sharing | Their full network/marketplace infrastructure |
| **LACP** (github.com/LiXin97/LACP) | `PLAN/ACT/OBSERVE` message taxonomy, JWS envelope concept | Their telecom focus, full three-layer stack |
| **REP** (MIT) | Sensitivity-sharing concept for conditional guidance | Their supply chain domain focus |
| **NeMo Guardrails** (NVIDIA) | Colang DSL concept for programmatic rails | Their full runtime — we have hooks already |
| **COLLAPSE.md** | Drift detection thresholds, recovery protocol patterns | Their generic framework — we need Jiminy-specific |
| **ACE** (NeurIPS) | Playbook evolution, de-duplication, anti-collapse patterns | Their general agent focus |

---

## Token Budget Analysis

Current Jiminy guidance injection (post-J16): ~2000-5000 tokens per prompt (full NL).

With J17 Tier 1 coded constraints:
- 30 active constraints at ~15 tokens each = ~450 tokens
- Session ticket overhead = ~50 tokens
- Protocol header = ~30 tokens
- **Total: ~530 tokens** (vs 2000-5000 NL) = **4-10x reduction**

With J17 Tier 2 (telegraphic) for novel guidance:
- Add ~200-500 tokens for context-specific guidance
- **Total: ~730-1030 tokens** = **2-5x reduction**

With J17 Tier 3 (full NL fallback):
- Equivalent to current = ~2000-5000 tokens
- Only used for truly novel situations

**Expected steady-state**: 80% Tier 1 (coded), 15% Tier 2 (telegraphic), 5% Tier 3 (NL).
**Weighted average: ~600-800 tokens per guidance injection** = **3-7x reduction from current**.

---

*Documents Accessed: All four prior research documents (01-04), Agora Protocol paper, LACP paper, A2A spec, REP paper, NeMo Guardrails docs, COLLAPSE.md, ACE paper, TLS RFCs, SSE spec, JWT spec, SOAR/ACT-R literature.*
