# Global Roadmap — End-User Path (U1-U7)

**Sprint**: ONBOARDING-TUTORIAL-ENDUSER-001
**Pattern**: adapted from [`../mdemg-onboarding-deck-001/global_roadmap.md`](../mdemg-onboarding-deck-001/global_roadmap.md) (F1-F10 → U1-U7)
**Status**: DRAFT — awaiting operator sign-off (E1 gate)
**Last updated**: 2026-10-26
**Envelope**: 28-40 slides (per `presentation_spec.md` §2); current proposal **32 slides**

> This roadmap sets the slide-count breakdown + section intent + natural stopping points for the end-user path. Per-section detail (slide-by-slide titles + content pointers) lives in `roadmaps/U*.md` and gets authored at E2.

---

## 1. Section Breakdown

| § | Section | Slides | Running total | Target walk-time | Natural Q&A anchor |
|---|---|---:|---:|---:|---|
| — | **U-section TOC entry** (added to main deck TOC after Thank You pointer) | 0 | 0 | — | — |
| U1 | **Welcome + What You Have** | 4 | 4 | ~3 min | after U1 recap |
| U2 | **The /ui/ Dashboard** | 6 | 10 | ~5 min | after U2.4 Review tab |
| U3 | **Grafana — What To Actually Watch** | 5 | 15 | ~4 min | after U3.3 red-today panel |
| U4 | **CLI — The ~15 Commands You'll Use** | 7 | 22 | ~6 min | after U4.5 ops commands |
| U5 | **Memory — Inspect + Correct** | 5 | 27 | ~4 min | after U5.4 correct example |
| U6 | **Alerts + Enforcement Escape Hatches** | 5 | 32 | ~4 min | after U6.4 override flow |
| U7 | **Where To Go Next** | 2 | 34 | ~1 min | — (end of path) |
| **Totals** | | **34** | **34** | **~27 min** | — |

**Fits envelope**: 34 slides is inside the 28-40 target band and well below the 45 hard cap.
**Walk-time**: ~27 min end-to-end is faster than the dev deck's 115 min — matches "~20-30 min" target per `presentation_spec.md` §2.

---

## 2. Per-Section Intents (One Sentence Each)

### U1 — Welcome + What You Have (4 slides)
**Intent**: Set the mental model: "you have 5 containers + 1 server; here's the 4-command path from `brew install` to `/ui/` open."

**Slide shape**:
- U1.1 Landing (replaces `#end-user-path` stub) — "You're operating MDEMG — here's what the next ~25 min cover"
- U1.2 What's Running — `docker compose ps` output; 5 containers named; one-sentence role each
- U1.3 The 4-Command Happy Path — `init / up -d / healthz / open /ui/`
- U1.4 Verify You're Alive — `mdemg healthz` + what `status=ok` vs `status=degraded` means

**Natural Q&A anchor**: after U1 — operator can pause before diving into UI/CLI.

---

### U2 — The /ui/ Dashboard (6 slides)
**Intent**: Walk the 4 tabs operators actually visit; name what each shows + what you can do there.

**Slide shape**:
- U2.1 Section Landing — "The /ui/ dashboard is your central operator surface. 4 tabs matter day-to-day."
- U2.2 Memory tab — what you see (nodes, spaces, recent activity); when you'd open it
- U2.3 Jiminy Rules tab — view/add/edit/tombstone rules; strict vs suggest toggle
- U2.4 Review tab — HITL queues (contradicted drafts, guidance, LLM audits); keyboard shortcuts
- U2.5 Config tab — what's safe to toggle live; what requires restart
- U2.6 Recap — "when you'd open each tab" cheat sheet

**Natural Q&A anchor**: after U2.4 Review tab — densest subsection.

---

### U3 — Grafana — What To Actually Watch (5 slides)
**Intent**: Opinionated panel-triage: 9 dashboards exist, but 3-5 panels matter day-to-day. Name them. Name their red/yellow/green thresholds.

**Slide shape**:
- U3.1 Section Landing — "Grafana has 9 dashboards + ~70 panels. You watch 5."
- U3.2 The Daily 5 — list: Overview, Jiminy follow-rate, Neo4j health, FT benchmark freshness, LLM error rate. Why each.
- U3.3 Red Panels — current honest red/yellow states operators should know about (Jiminy Follow Rate, FT Benchmark, Neo4j Backup) + what they MEAN (trajectory framing, not "by design")
- U3.4 How To Read A Panel — p95 vs mean, time range selector, link-to-dashboard pattern
- U3.5 When To Alert vs. When To Ignore — the alert channel is `~/.mdemg/alerts/current.json`; HIGH/MEDIUM/LOW thresholds

**Natural Q&A anchor**: after U3.3 red-today panel — operators often ask "but what IS this red thing?"

---

### U4 — CLI — The ~15 Commands You'll Use (7 slides)
**Intent**: 90+ shipped commands, operators use ~15. Group them, name them, show invocation.

**Slide shape**:
- U4.1 Section Landing — "4 command groups, ~15 total commands. Everything else is advanced."
- U4.2 Lifecycle — `start / stop / restart / status / healthz / version` (6)
- U4.3 Memory — `data observe / data recall / data correct` (3)
- U4.4 Introspection — `synergy status / watchdog status / jiminy mode` (3)
- U4.5 Ops — `backup / upgrade / data export / data clean` (4)
- U4.6 The One-Shot `mdemg tutorial` — the command that opens THIS deck
- U4.7 Recap — printable cheat sheet (all 15 in one visual)

**Natural Q&A anchor**: after U4.5 ops — the backup command catches most operators off-guard.

---

### U5 — Memory — Inspect + Correct (5 slides)
**Intent**: Operators tell OTHER operators about this — "you can inspect what MDEMG remembers, and you can correct it." Show the operator-facing memory surface.

**Slide shape**:
- U5.1 Section Landing — "MDEMG remembers across sessions. Here's how to see it + change it."
- U5.2 Observe — `mdemg data observe --content "..."` + what obs_types mean
- U5.3 Recall — `mdemg data recall --query "..."` + reading the result shape
- U5.4 Correct — `mdemg data correct --incorrect "X" --correct "Y" --context "..."` + what L0→L1 promotion means later
- U5.5 Space Freshness — `mdemg synergy status` + when memory feels stale

**Natural Q&A anchor**: after U5.4 correct example — the "wait, so I can just TYPE a correction?" moment.

---

### U6 — Alerts + Enforcement Escape Hatches (5 slides)
**Intent**: Operators handle alerts; operators sometimes need to override enforcement. Show both.

**Slide shape**:
- U6.1 Section Landing — "When MDEMG blocks something or shouts about something, here's your playbook."
- U6.2 Reading The Terminal Banner — `~/.mdemg/alerts/current.json` + the top-of-session banner + severity meaning
- U6.3 Clearing Alerts — `mdemg alerts clear` (acknowledgement, not resolution)
- U6.4 Enforcement — `mdemg jiminy mode <strict|suggest>` toggle; what blocks in strict mode
- U6.5 The Override Escape Hatch — `mdemg jiminy override apply --constraint X --reason Y --duration 1h` + the audit trail

**Natural Q&A anchor**: after U6.4 override flow — "wait, I can turn it OFF?" moment.

---

### U7 — Where To Go Next (2 slides)
**Intent**: Honest pointers; name what's open; close the walkthrough.

**Slide shape**:
- U7.1 Documentation Surfaces — docs/user/, docs/features/, FAQ, how to file issues, how to contribute observations
- U7.2 Honest Open Items — HIGH alerts you're currently seeing (Jiminy Follow Rate, FT Benchmark staleness, Neo4j Backup) — framed as arcs-in-progress, not acceptance; return-links to fork + TOC + README

**Natural Q&A anchor**: — (end of path; operator exits here)

---

## 3. Mermaid + Screenshot Tally (Expected)

- **Mermaid diagrams**: 0-2 across the whole path. Per `presentation_spec.md` §4.2 default is NO Mermaid.
  - Candidate 1 (judgment call at E3): U1.2 — a tiny `graph LR` of the 5 containers → MDEMG server → operator
  - Candidate 2 (judgment call at E3): U6.5 — a tiny alert-flow sketch (block → operator → override → audit trail)
- **Screenshots**: 0 shipped at E4. Operator decides at E5 if any /ui/ or Grafana tab description reads better with a visual.

---

## 4. Natural Stopping Points For Operator Review

If E5 operator-review needs to split across sessions (90 min is a lot of continuous focus), these are the natural pause points:

| Pause after | Running total | Walk-time so far |
|---|---:|---:|
| U2 recap (U2.6) | 10 slides | ~8 min |
| U3 end (U3.5) | 15 slides | ~12 min |
| U4 recap (U4.7) | 22 slides | ~18 min |
| U5 end (U5.5) | 27 slides | ~22 min |
| U6 end (U6.5) | 32 slides | ~26 min |
| U7 end (U7.2) | 34 slides | ~27 min |

Operator can tag the pause point in `review_notes.md` and resume cleanly.

---

## 5. Open Questions For Operator (E1 Gate)

Pre-authoring questions the operator should decide BEFORE E2 kicks off:

1. **U-section ordering** — current order is U1→U2→U3→U4→U5→U6→U7. Is this right, or would you prefer e.g. CLI (U4) earlier (before /ui/)?
2. **U7 open-items honesty** — comfortable showing current HIGH alerts (Jiminy Follow Rate, FT Benchmark, Neo4j Backup) in U7.2 as "arcs-in-progress"? Alternative: list them + name the arc, no screenshot of the current count.
3. **Mermaid candidates** — approve both U1.2 and U6.5 Mermaid sketches up front, or defer both to judgment-call-at-E3?
4. **Screenshots** — any U-section where you already know you want a screenshot (e.g. the /ui/ tabs)? Or stay with "screenshot-opt-in at E5"?
5. **Per-section walk-time budgets** — comfortable with the ~27 min total? If you want ~20 min, we trim U4 (CLI deepest section) OR split it.
6. **TOC placement** — U-section pointer gets added to the deck TOC. Where in the TOC? Current proposal: appended after F10 as a top-level entry, OR as a sub-row under F10. Operator preference?

If any answer wants roadmap changes, revise this doc BEFORE E2 roadmaps start — cheap to fix here, expensive to fix after per-section roadmaps are written.

---

**Operator sign-off (E1 gate — second half)**:

- [ ] 34-slide proposal fits mental model of "end-user walkthrough"
- [ ] U1-U7 section ordering is right (or revise per Q1)
- [ ] Section intents (one sentence each) match what operator wants shown
- [ ] Walk-time ~27 min feels right (not too long, not too thin)
- [ ] Open-questions §5 answered — each one captured here before E2 begins
- [ ] No scope creep lurking (no proposed U8, no massive U-section ballooning)

If any checkbox fails, iterate here before E2 begins.
