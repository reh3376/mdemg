# Conversation Memory System (CMS) — Engineering Spec (v2)

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


## 1. Purpose and Non-Goals

### 1.1 Purpose

The Conversation Memory System (CMS) provides **state persistence under context churn** (auto-compaction/resets) by converting conversational history into a persistent, self-organizing graph (“Internal Dialog”) rather than a flat chat log.

CMS enables:

- **Zero amnesia for committed state** (decisions/invariants/tasks/corrections), even after chat compaction.
- **Cross-session learning** via reinforcement and concept abstraction.
- **Modular intelligence substrate** that links to other MDEMG modules (code, Linear, Obsidian, etc.).
- **Multi-Tenant Collaboration**: Support for private "Internal Dialogs" that graduate into shared "Team Knowledge."

### 1.2 Non-Goals

- CMS is not a “chat transcript archive.” It optimizes for **commitments + evidence** rather than completeness.
- CMS does not guarantee truth of model-generated claims without provenance.

---

## 2. Core Behavioral Loop

CMS operates autonomously inside the agent loop.

### 2.1 Surprise-Driven Capture

Each turn yields one or more **Observations**. Observations receive a `surprise_score ∈ [0.0, 1.0]`. High-surprise observations (especially explicit user corrections) are prioritized for reinforcement and resumption.

### 2.2 Automatic Resumption (with Jiminy)

When the agent detects a context reset (e.g., compaction), it calls `POST /v1/conversation/resume` to rehydrate working state. The response includes a **Jiminy rationale** explaining *why* specific state was rehydrated (e.g., "Rehydrating due to high surprise score and recent reinforcement").

### 2.3 Hebbian Reinforcement

CMS reinforces associations using `CO_ACTIVATED_WITH` edges (“Hebbian wiring”). Reinforcement is windowed to avoid "hairballing."

---

## 3. Hierarchical Memory Model

CMS follows the standard 5-layer MDEMG hierarchy but specializes the role types for conversational data.

- **Layer 0 — Observations** (`role_type = conversation_observation`)
- **Layer 1 — Themes** (`role_type = conversation_theme`) formed by DBSCAN clustering of observation embeddings.
- **Layer 2+ — Emergent Concepts** (`role_type = emergent_concept`)

---

## 4. Data Model (Identity & Visibility)

### 4.1 Node Labels

CMS utilizes the **Identity Layer** to support collaborative environments.

#### 4.1.1 `:MemoryNode` (Observation / Theme / Concept)

**Identity & Visibility Properties**

- `user_id: string` (Owner of the memory)
- `visibility: enum` (`private | team | global`)
- `volatile: boolean` (True for unreinforced short-term thoughts)
- `stability_score: float` (0..1, managed by Context Cooler)

**Required Base Properties**

- `id: string` (UUID)
- `space_id: string` (MDEMG partition)
- `session_id: string` (conversation run/session)
- `role_type: enum`
- `layer: int`
- `content: string`
- `embedding: vector<float>`

#### 4.1.2 `:SymbolNode` (Cross-Module Linking)

Conversational observations can link directly to code symbols.

- **Edge**: `(Observation)-[:REFERS_TO]->(SymbolNode)`
- **Example**: A user correction "Actually, the timeout is 500ms" links to the `TIMEOUT` constant symbol.

---

## 5. Active Participant Engine (APE) & Context Cooler

The APE orchestrates the **Binary Sidecar Modules** and manages graph hygiene.

### 5.1 Context Cooler (Graduation Logic)

To prevent the "junk yard" effect, new thoughts follow a graduation path:

1. **Ingest**: Node created as `volatile: true` with `stability_score: 0.1`.
2. **Reinforcement Window**: Node remains `volatile` for 2 hours.
3. **Evaluation**:
   - If node is **reinforced** (co-activated or referenced), `stability_score` increases.
   - If node reaches `0.8` stability, it graduates to `volatile: false`.
   - If node remains stagnant after the window, it is **tombstoned**.

### 5.2 gRPC Sidecar Integration

While CMS exposes an HTTP API for agents, its internal reasoning can be extended via gRPC sidecars (e.g., a "Strategic Consistency" module written in Rust).

---

## 6. API (Hardened GA-Ready)

### 6.1 `POST /v1/conversation/observe`

**Request** (Includes Identity)

```json
{
  "space_id": "string",
  "session_id": "string",
  "user_id": "string",
  "visibility": "private|team|global",
  "observation_type": "decision|correction|task|invariant",
  "content": "string",
  "refers_to_symbol_id": "optional_uuid"
}
```

### 6.2 `POST /v1/conversation/resume`

**Response** (Includes Jiminy)

```json
{
  "snapshot": { "commitments": [...], "tasks": [...] },
  "jiminy": {
    "rationale": "Rehydrated task X because it is blocked by uncommitted decision Y.",
    "confidence": 0.95,
    "score_breakdown": { "surprise": 0.8, "recency": 0.9 }
  }
}
```

---

## 7. APE (Active Participant Engine) — Background Jobs

APE runs periodic hygiene and upgrades.

1) **Edge decay** for `CO_ACTIVATED_WITH`
2) **Context Cooler**: promote only via evidence/reinforcement gates
3) **DBSCAN clustering** to create Themes
4) **Concept promotion** based on cross-session reinforcement + commitments + evidence
5) **Gap interviews** (weekly): prioritize recurring `CapabilityGap` nodes
