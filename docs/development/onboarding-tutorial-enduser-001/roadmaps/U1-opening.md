# U1 — Opening (6 slides)

**Intent**: Open with the 4 strategic questions + an integrating slide. Set the frame; each subsequent U-section is an in-depth answer to a piece of this.
**Walk-time**: ~5 min
**Target slides**: 6
**Mermaid**: 1 (U1.6 — integrated system flow)
**Natural Q&A anchor**: after U1.6

**Depth template coverage**: this section IS the question-answer landing; the "depth template" (mental model / where to see / etc.) is distributed across U2-U17.

**The 4 questions + the integrator** (operator-directed 2026-10-26 opening framing):
1. What is MDEMG?
2. What problem(s) does it attempt to solve?
3. How does it solve those problems?
4. How does it continuously get better at solving those problems?
5. **How does it all work together?** (the integration slide; U1.6)

---

## U1.1 — Section Landing (replaces `#end-user-path` stub)

**Title**: `U1 · Operator Onboarding — Start Here`
**Slide ID**: `end-user-path` (**ANCHOR PRESERVED** — SERVE-001 landing fork links here)

**Body**:
- 1-sentence intent: "The next ~85 minutes answer four questions. Each U-section drills into a piece of the answer. You'll own the operator view by the end."
- The 4 questions listed inline
- Mini-TOC: U1-U17 with 1-line topic each
- Return-link row to `#landing-fork` + `#table-of-contents`

**Provenance**:
- `presentation_spec.md` §1a Opening Framing
- CLAUDE.md anchor-preservation invariant (JIMINY-ARCHIVED-CODE-FILTER-001 class)

**note**:
- Operator cue: "If you're building MDEMG, hit ESC and pick the Software Dev path. Otherwise, let's answer these four questions."

---

## U1.2 — What Is MDEMG?

**Title**: `What Is MDEMG?`

**Body**:
- The mission quote (verbatim from CLAUDE.md `project_mdemg_purpose`):
  > **Help developers create software that connects in a manner that increases the likelihood of making better decisions.**
- One-paragraph expansion:
  - MDEMG is a **cognitive substrate** — persistent memory + decision-informing retrieval + behavioral guidance — not a tool
  - **Decisions are the atomic unit** — not features, not metrics
  - **Connections** (memory → retrieval → classification → synthesis) are what the system optimizes
- The analogy: internal dialogue is to a conscious biological neural network as MDEMG is to an artificial neural network (an AI agent)
- Where this expands: U3 Memory (the substrate itself), U4 Jiminy (how it guides decisions)

**Provenance**:
- CLAUDE.md "MDEMG Core Purpose" memory + "MDEMG IS Your Internal Dialogue" section
- `docs/features/mdemg-onboarding-deck.md`

**note**:
- Operator cue: "The dev-path's F1 opens with this same quote. Keep framing identical across paths."
- PM-fallback: "Think of MDEMG as a persistent, searchable 'notebook of lessons learned' that the AI agent reads before every action."

---

## U1.3 — What Problems Does It Solve?

**Title**: `What Problems Does It Solve?`

**Body**:
- Three core problem classes:
  1. **Context loss across sessions** — the AI agent forgets everything between runs; MDEMG remembers (persistent CMS) → *expanded in U3 Memory*
  2. **Behavioral drift** — the agent repeats mistakes; MDEMG surfaces durable guidance that reminds it before action (Jiminy + enforcement) → *expanded in U4 Jiminy, U16 Alerts/Enforcement*
  3. **Silent failure modes** — the system degrades without anyone noticing; MDEMG observes itself continuously via alerts + Grafana + RSIC → *expanded in U13 Grafana, U16 Alerts, U5 RSIC*
- Honest note: the arcs that MOVE each needle are ongoing; U5 (RSIC) and U17 (Where to Go) carry the trajectory

**Provenance**:
- CLAUDE.md "Project Context" section + "Service Alert System (SR-001 + SNA-001)"
- The 2026-08-01 `trust-signal-must-be-persisted-never-ignore-honest` directive

**note**:
- Operator cue: "If the agent you work with feels like it 'knows nothing' every morning, Problem 1 is your issue. If it 'keeps doing that thing you told it not to,' Problem 2."

---

## U1.4 — How Does It Solve Them?

**Title**: `How Does It Solve Them?`

**Body**:
- The four surfaces (each is a U-section):
  1. **Memory substrate** — Neo4j graph of observations + consolidation into layered abstractions → *U3 Memory*
  2. **Retrieval + guidance** — RRF 4-column retrieval + Jiminy surfaces durable rules on every prompt → *U3 Memory + U4 Jiminy*
  3. **Enforcement** — strict mode blocks violations; override escape hatches preserve operator agency → *U16 Alerts + Enforcement*
  4. **Observability** — Grafana + TSDB + alerts channel + /ui/ Status → *U2 Stack, U13 Grafana, U16 Alerts*
- Mental model: "sense → store → retrieve → classify → act → observe → learn" — the full loop
- Where this expands: literally U2 through U15 are the surfaces that make this real

**Provenance**:
- CLAUDE.md "Key Directories" + "Architecture Patterns"
- `docs/features/*.md` (41 shipped feature docs)

**note**:
- Operator cue: "If the four surfaces above sound abstract, they get concrete starting in U2. This slide is the map; the territory starts next."

---

## U1.5 — How Does It Continuously Get Better?

**Title**: `How Does It Continuously Improve?`

**Body**:
- Four self-improvement loops (each a U-section's deep topic):
  1. **RSIC** — recursive self-improvement cycle: reflect → trigger → admit → outcome → learn → *U5 RSIC*
  2. **UxTS forcing-functions** — contract-first framework family (UATS/UVTS/ULTS/…) prevents drift at merge time → *U6 UxTS*
  3. **HITL curation** — operator grading of guidance outcomes feeds the retrain corpus → *U7 Review/HITL*
  4. **Fine-tuning / recursive retrain** — the production LLM gets measurably better via Phase 5 SFT + the recursive-retrain loop → *U8 Learning/Training Data*
- Honest trajectory language rule (operator directive 2026-08-01):
  - WRONG: "follow rate 13% by design"
  - RIGHT: "follow rate ~13% currently; target ≥80% via the JIMINY-CEILING-BREAK-2 arc; today's trajectory is …"
- Where this expands: U5 is the master arc of continuous improvement; U6-U8 are the enabling machinery

**Provenance**:
- CLAUDE.md JIMINY-CEILING-BREAK-2 arc + RSIC pins + FT-RECURSIVE-{001,002,003,004} + HITL-CURATION-* sprints
- CLAUDE.md "Testing — Live System Testing Is Required"

**note**:
- Operator cue: "The system is honest about where it is. If a metric is below target, we name the arc moving it. You'll see this trajectory language everywhere from U2 onwards."

---

## U1.6 — How It All Works Together

**Title**: `How It All Works Together`

**Body**:
- 1-sentence context: "The four surfaces (memory, retrieval+guidance, enforcement, observability) and the four improvement loops (RSIC, UxTS, HITL, fine-tune) are not separate systems — they share one substrate and feed each other."
- **Mermaid #8** (integrated flow — the big picture):
  - Agent action → **Hook** (U-level: pre-write-check / pre-bash-check / post-tool-observe)
  - Hook → **Jiminy classify** (U4) uses **Memory recall** (U3) → surfaces durable rules + evaluates outcome
  - Agent action → **Observe** (U3) → L0 obs → consolidation → L1/L2+ nodes → fed back into recall
  - Outcomes → **RSIC** (U5) reflects → triggers improvement cycles
  - Outcomes also → **HITL Review** (U7) → graded → feeds **retrain corpus** (U8) → next model generation
  - All data → **TSDB** → **Grafana** (U13) + **Alerts** (U16)
  - **UxTS contracts** (U6) gate every merge → prevent drift
- One operator sentence: "You observe → MDEMG remembers → classifies → acts → measures → learns → fine-tunes → you see all of it on the dashboards."
- Honest trajectory: this loop is partially live-closed today; the HITL→retrain arc is the newest piece, actively iterating
- Where this expands: literally U2 through U16

**Provenance**:
- CLAUDE.md "Architecture Notes" (104 shipped arch rules)
- `docs/features/mdemg-onboarding-deck.md` governance doc

**note**:
- Operator cue: "This is the whole loop. Memorize this slide; everything after is a zoomed-in piece of it."
- PM-fallback: "Think of it like a flight-test rig — the plane flies, instruments record, engineers review, next build improves. MDEMG is that for AI agents and their decisions."
- Transition: "Four questions + how it fits together. Next up: U2 — the stack that implements all of this."
