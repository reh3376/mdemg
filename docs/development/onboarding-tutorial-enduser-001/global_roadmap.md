# Global Roadmap · End-User Path (U1-U17)

**Sprint**: ONBOARDING-TUTORIAL-ENDUSER-001
**Pattern**: adapted from [`../mdemg-onboarding-deck-001/global_roadmap.md`](../mdemg-onboarding-deck-001/global_roadmap.md) (F1-F10 → U1-U17)
**Status**: v4 (operator-directed no-caps 2026-10-26), extensive + highly informative; operators-only (dev-path serves developers/contributors); slides/figures/tables/diagrams driven by information requirement, not an envelope
**Envelope**: **NO CAPS** on slides, figures, tables, diagrams. Current working estimate ~105 slides but authoring may expand any U-section as needed to convey its content faithfully.

> This roadmap sets the slide-count breakdown + section intent + natural stopping points for the end-user path. Per-section detail (slide-by-slide titles + content pointers) lives in `roadmaps/U*.md` and gets authored at E2.

**v2 change log (2026-10-26)**: operator directed topic-per-section expansion covering every major MDEMG topic (RSIC, UxTS, Jiminy, Logs/Training Data, APIs+gRPC, etc.) instead of 7 operator-task sections. Envelope raised 28-40 → 45-60. Walk-time raised ~27 min → ~45 min. Covers all 13 /ui/ tabs + Grafana + CLI + APIs/gRPC + mental-model areas (RSIC, UxTS) not surfaced by any single tab.

**v3 change log (2026-10-26)**: operator directed "extensive and highly informative onboarding mechanism" with "I am not overly concerned about the total number of slides or time required to review", AND clarified the slideshow forks at landing into 2 paths (end-user vs software-dev-contributor); this end-user path targets operators only (the dev path is already comprehensive for developers/contributors, shipped as the F1-F10 107-slide deck). Per-topic depth expanded from 3-4 slides to 5-9 slides via a 7-element depth template (landing / mental-model / where-to-see / how-to-touch / current-state / gotchas / deeper). Envelope uncapped; expected range ~80-120 slides.

**v4 change log (2026-10-26)**: operator directive, "do not set unnecessary constraints on figures, tables, diagrams, or slides. This will take as much as necessary to provide the data required." Removed all upper and lower caps on slide count, Mermaid diagrams, tables, and other visual elements. The 7-element depth template remains as a GUIDE (not a straitjacket). Section slide counts below are working estimates subject to expansion/contraction during E3 authoring. Also: U1 restructured as pure Opening (framing, 4 strategic questions + integrator per operator 2026-10-26); stack/running content moved into expanded U2. Added 1 approved Mermaid (U1.6 integrator).

---

## 1. Section Breakdown (v3 depth)

| § | Section | Slides | Running total | Walk-time | Natural Q&A anchor |
|---|---|---:|---:|---:|---|
| U1 | **Opening, The 4 Questions + Integrator** | 6 | 6 | ~5 min | after U1.6 |
| U2 | **The Stack + Status + Health** | 7 | 13 | ~6 min |, |
| U3 | **Memory (CMS)** | 8 | 21 | ~7 min | after U3 |
| U4 | **Jiminy** (guidance + rules + mode) | 11 | 32 | ~9 min | after U4 |
| U5 | **RSIC** (self-improvement) | 8 | 40 | ~7 min | after U5 |
| U6 | **UxTS Framework Family** | 9 | 49 | ~7 min | after U6 |
| U7 | **Review / HITL** | 7 | 56 | ~5 min |, |
| U8 | **Learning / Training Data** | 7 | 63 | ~6 min | after U8 |
| U9 | **Backup + Ops** | 6 | 69 | ~5 min |, |
| U10 | **Logs** | 5 | 74 | ~4 min |, |
| U11 | **Config + Features** | 7 | 81 | ~5 min |, |
| U12 | **Plugins** | 5 | 86 | ~4 min |, |
| U13 | **Grafana** | 7 | 93 | ~6 min | after U13 |
| U14 | **CLI, The Full Command Surface** | 9 | 102 | ~7 min |, |
| U15 | **APIs + gRPC** | 9 | 111 | ~7 min | after U15 |
| U16 | **Alerts + Enforcement Escape Hatches** | 7 | 118 | ~6 min | after U16 |
| U17 | **Where To Go Next** | 4 | 122 | ~3 min |, (end) |
| **Totals** | | **122** | **122** | **~99 min** |, |

**Depth template per section** (per `presentation_spec.md` §2, GUIDE, not straitjacket): landing · mental-model · where-to-see · how-to-touch · current-state (honest trajectory) · gotchas · where-to-go-deeper. Sections expand slides/figures/tables/diagrams as the content requires (operator directive 2026-10-26, no caps).

---

## 2. Per-Section Intents (one sentence each)

### U1 · Welcome + What's Running (4 slides)
Set the stack mental model: 5 containers + mdemg server + the 4-command happy path.
- Mermaid #1: stack sketch (operator → mdemg → {neo4j, timescaledb, neural-sidecar, grafana}).

### U2 · Status + Health (3 slides)
`/ui/ Status` tab + `mdemg healthz` + watchdog state, the "is it alive right now?" surface.

### U3 · Memory (CMS) (5 slides)
The central idea. `/ui/ Memory` tab + CMS mental model (L0 obs → L1 constraint/correction → L2+ emergent concept) + `mdemg data observe|recall|correct`.
- Mermaid #2: L0 → L1 → L2+ consolidation flow.

### U4 · Jiminy (5 slides)
`/ui/ Jiminy` + `/ui/ Rules` tabs + mode toggle (strict/suggest) + the guidance → outcome → trust-signal loop + current follow-rate honest trajectory.
- Mermaid #3: Jiminy outcome loop.

### U5 · RSIC (4 slides)
`/ui/ RSIC` tab + the self-improvement loop + admission/cooldown + Hebbian learning + what operators see when it's running.
- Mermaid #4: RSIC cycle (reflect → trigger → admit → outcome).

### U6 · UxTS Framework Family (4 slides)
The forcing-function pattern + the current family (UATS/UVTS/ULTS/UOTS/UDTS/UBTS/UOBS/UNTS/USTS/UAITS/UAMS/UETS/UITS/UTDS/UBENCH) + how operators interact (`make test-X-lint`/`-run`) + the drift-gate principle.
- Mermaid #5: UxTS diamond (spec → fixture → runner → CI gate).

### U7 · Review / HITL (3 slides)
`/ui/ Review` tab + keyboard-driven grading (0-4 radio + Space-to-advance) + the HITL curation arc (auto-dismiss + schedule).

### U8 · Learning / Training Data (3 slides)
`/ui/ Learning` + `/ui/ Training Data` tabs + the recursive-retrain loop state (6a/6b/7/9) + benchmark freshness.

### U9 · Backup + Ops (3 slides)
`/ui/ Backup` tab + scheduled jobs + restore drills + manual `mdemg backup` CLI.

### U10 · Logs (2 slides)
`/ui/ Logs` tab + the log-scope filter + when to look at logs vs alerts.

### U11 · Config + Features (3 slides)
`/ui/ Config` + `/ui/ Features` tabs + what's safe to toggle live + what requires restart + sensitive-value redaction (`AUTH_API_KEYS`, `TSDB_PASSWORD`, etc.).

### U12 · Plugins (2 slides)
`/ui/ Plugins` tab + the plugin surface + how an operator inspects an installed plugin.

### U13 · Grafana (3 slides)
The 9 dashboards + opinionated "watch these 5 panels" + how to read p95/mean + alert-link pattern.

### U14 · CLI, The ~15 Commands You'll Use (4 slides)
4 command groups × ~4 commands each (lifecycle / memory / introspection / ops) + the one-shot `mdemg tutorial`.

### U15 · APIs + gRPC (4 slides)
HTTP REST (`/v1/*`, 199 routes) + the 4 gRPC surfaces (mdemg-module plugin RPC, unts, devspace, space-transfer) + when operators touch which + inspection via `curl`/`grpcurl`.
- Mermaid #6: surface map (operators via CLI → REST API; plugins/transfer/devspace via gRPC).

### U16 · Alerts + Enforcement Escape Hatches (4 slides)
Terminal banner + severity map + `mdemg jiminy mode <strict|suggest>` + `mdemg jiminy override` audit trail.
- Mermaid #7: block → operator → override → audit-trail flow.

### U17 · Where To Go Next (2 slides)
Documentation surfaces + honest open items (current HIGH alerts framed as arcs-in-progress) + exit return-links.

---

## 3. Mermaid Tally (7 diagrams across 58 slides, ~12%)

Matches the dev deck's ~14% density; each diagram carries a mental model operators can't easily reconstruct from CLI output or UI screenshots.

| Diagram | Slide | What it shows |
|---|---|---|
| #1 | U1.2 | Stack: 5 containers + mdemg server |
| #2 | U3 | Memory hierarchy: L0 → L1 → L2+ consolidation |
| #3 | U4 | Jiminy outcome loop (surface → classify → outcome → trust) |
| #4 | U5 | RSIC cycle (reflect → trigger → admit → outcome) |
| #5 | U6 | UxTS diamond (spec → fixture → runner → CI gate) |
| #6 | U15 | API surface map (CLI → REST; plugins/transfer → gRPC) |
| #7 | U16 | Alert override flow (block → operator → override → audit) |

---

## 4. Natural Stopping Points For Operator Review

E5 review session may split across sittings (85 min is a lot of continuous focus); these are natural pause points:

| Pause after | Running total | Walk-time so far |
|---|---:|---:|
| U3 Memory | 18 | ~15 min |
| U5 RSIC | 34 | ~29 min |
| U8 Learning/Training | 54 | ~44 min |
| U11 Config+Features | 69 | ~55 min |
| U14 CLI | 86 | ~68 min |
| U17 end | 102 | ~85 min |

---

## 5. Deferred Open Questions

Questions from v1 §5 of this doc that the operator resolved (direction set, no further action needed):

1. U-section ordering, kept natural (Welcome → Status → core topics → surfaces → CLI/API → Alerts → Next). Operator approved.
2. U17 honesty, current HIGH alerts shown as arcs-in-progress (trajectory language). Operator approved.
3. Mermaid policy, 7 diagrams approved up front (slot list in §3 above). Operator approved.
4. Screenshots, opt-in at E5 (operator decides live). Operator approved.
5. Walk-time budget, ~45 min accepted for full-depth coverage. Operator approved.
6. TOC placement, appended after F10 as top-level entry (deferred to E4 stitch time).

New open question tracked for E5:

**Q-A**: U15 APIs + gRPC: is the HTTP REST surface meant as a reference (operators glance at the route count) or as a usage tutorial (operators learn `curl POST /v1/memory/observe`)? Current roadmap assumes reference-with-one-example-per-group. Operator may redirect at E3 draft review.

---

**Status**: v2 approved by operator 2026-10-26 (B + U15 APIs + gRPC). Proceeding to E2 per-section roadmap authoring.
