# U7 — Review / HITL (Human-In-The-Loop) (7 slides)

**Intent**: `/ui/ Review` tab + keyboard-driven grading + the HITL curation arc. HITL is where operators FEED the retrain corpus by grading guidance outcomes, LLM audits, and contradicted drafts the system cannot auto-judge.
**Target slides**: 7
**Mermaid**: 0 (table + CLI examples carry the content)

---

## U7.1 — Section Landing

**Title**: `U7 · HITL — You Grade What The System Can't`

**Body**:
- Intent: "The classifier can't tell if a `partial_compliance` verdict was actually `followed` with reasonable judgment. The retrain corpus needs human verdicts. HITL is the UI + CLI that makes operator grading fast enough to be sustainable."
- Preview: dataset registry → /ui/ Review → keyboard grading → auto-grader + schedule → sink invariant → trajectory
- Points back to U1.5 "How does it continuously improve?" — this is loop #3 (operator-fed training signal)

**Provenance**:
- CLAUDE.md "Human-in-the-loop review (HITL-REVIEW-001)" + "HITL-CURATION-002/003"

---

## U7.2 — The Dataset Registry

**Title**: `What You Can Grade`

**Body**:
- Shipped datasets (17 total): guidance corpus + 16 LLM call-site datasets (ape.reflect, jiminy.evaluate_llm, consulting.classify, retrieval.rerank_cross, etc.)
- Each dataset = `ReviewableDataset` interface implementation: `FetchCandidates` / `FetchItem` / `Rubric` / `Sink` + optional `Description` + `AutogradePromptHinter` + `NonReinforcingApplier`
- Rubrics are VERSIONED (`rubric_version` field) — grades are joined to source rows by `(dataset_id, item_id)`
- Guidance corpus is the ONLY dataset with a REINFORCING sink (grade → trust-score mutation + substrate reinforcement); all 16 LLM call-site datasets ship with `NoopSink` (gold-only, no live mutation)

**Provenance**:
- CLAUDE.md "HITL-REVIEW-001" dataset catalog + "HITL-CURATION-002" invariant

**note**:
- Operator cue: "Reviewing guidance CHANGES behavior. Reviewing LLM outputs produces training data without touching behavior. Different stakes."

---

## U7.3 — /ui/ Review Tab (Keyboard-Driven)

**Title**: `The Review Tab — Grade Fast`

**Body**:
- Where: `/ui/` → **Review**
- Flow:
  1. Pick a dataset from the registry dropdown
  2. See the next pending candidate (candidate_count displayed; backlog-capped at 200 per fetch)
  3. Read the row — the rubric dimensions appear as 0-4 radio inputs below
  4. Grade with keyboard: `0`-`4` set every dimension; `Space` or `Enter` submits; `n` next; `u` reverse (undo last grade); `?` help; `Esc` close-help/cancel-auto-advance
  5. Auto-advance 400ms after submit with visible status-line (reversible via `u`)
- Pre-fill: `POST /v1/review/autograde-preview` → LLM proposal fills radios; operator confirms or corrects (JIMINY-HITL-VELOCITY-001 → HITL-AUTOGRADE-PREVIEW-001)
- Session counter banner at top (bright-green)

**Provenance**:
- CLAUDE.md "JIMINY-HITL-VELOCITY-001" (keyboard bindings) + "HITL-AUTOGRADE-PREVIEW-001"
- `internal/api/ui/tabs/review.js`

**note**:
- Operator cue: "If grading feels slow, you're using the mouse. 0-4 + Space is the whole flow."

---

## U7.4 — Auto-Grader + Scheduled Autograde

**Title**: `When The System Grades Itself`

**Body**:
- CLI: `mdemg review autograde --dataset <ds> --space-id <id> [--min-confidence 0.80] [--limit 50]`
- The auto-grader is an LLM-driven grader with per-dataset `AutogradePromptHinter` — teaches the model dataset-specific semantics
- Scheduled loop (AUTOGRADE-SCHEDULE-001): opt-in via `REVIEW_AUTOGRADE_SCHEDULE_ENABLED=true` + `REVIEW_AUTOGRADE_SCHEDULE_DATASETS=<comma,separated>`; runs every `_INTERVAL_HOURS` (default 6)
- Auto-grade invariant (HITL-CURATION-002): every auto-grade row has `grader_id` starting with `review.AutoGraderPrefix` (`"auto:"`), POSTs `reinforce:false` → NEVER mutates substrate
- Auto-dismiss (HITL-AUTO-DISMISS-001): dim=0 dismisses the row (`MarkDismissed`); dim=4 approvals stay operator-only via `NonReinforcingApplier.handled=false`

**Provenance**:
- CLAUDE.md "HITL-CURATION-002" invariant + "AUTOGRADE-SCHEDULE-001" + "HITL-AUTO-DISMISS-001"

**note**:
- Operator cue: "The auto-grader drains the EASY cases. Your operator attention stays on the hard middle (dim=2 unclear) and dim=4 approvals that mutate substrate."

---

## U7.5 — Where /ui/ Review Lives

**Title**: `Review Tab Context`

**Body**:
- Review tab complements other HITL surfaces:
  - **Contradicted drafts** (`contradicted_drafts` dataset): shows LLM-authored "did the agent CONTRADICT this rule?" judgments; operator grades + optional auto-dismiss
  - **Guidance corpus**: shows classifier verdicts; operator correction here MUTATES substrate (reinforce=true)
  - **LLM audits**: 16 LLM call-site datasets; operator-graded output quality feeds future retrain corpus
- Related panels:
  - Grafana `mdemg-hitl` dashboard: grade cadence, auto vs operator share, pending drafts queue, reinforcement applied 7d (HITL-ANALYTICS-TILE-001)
- Related CLI: `mdemg review autograde` + `mdemg review reverse <grade_id>` (reversal path)

**Provenance**:
- CLAUDE.md "JIMINY-CONTRADICTED-BRIDGE-001" + "HITL-ANALYTICS-TILE-001"
- `deploy/docker/grafana/dashboards/mdemg-hitl.json`

**note**:
- Operator cue: "Grafana's `Reinforcement Applied 7d` panel is the HONEST answer to 'did substrate actually change this week?' — ONLY operator-approved guidance grades count."

---

## U7.6 — Current State + Trajectory

**Title**: `HITL Today — Honest`

**Body**:
- Current state (`mdemg-dev`, 2026-10):
  - 17 datasets registered
  - Contradicted drafts queue: ~5-10 pending typically (auto-dismiss drains dim=0; operator approves dim=4)
  - Guidance corpus: feeds JIMINY-CEILING-BREAK-2 arc Phase 4b retrain target
  - Scheduled autograde: `contradicted_drafts,guidance` opt-in via `.env`
- The AUTOGRADE-SCHEDULE-001 loop closes the drain mechanism: pre-sprint queue grew indefinitely; post-sprint it self-drains for auto-dismissible classes
- Trajectory arc: `hitl-curation-stalled` alert fires if operator-grade count drops to 0 over 7d (excludes auto:* rows) — forces visibility

**Provenance**:
- CLAUDE.md "HITL-CURATION-002/003" + "AUTOGRADE-SCHEDULE-001"

**note**:
- Operator cue: "If `hitl-curation-stalled` fires on your session, 10 minutes of keyboard-grading in /ui/ Review clears it."

---

## U7.7 — Gotchas + Where To Go Deeper

**Title**: `HITL — Gotchas + More`

**Body**:
- Gotchas:
  - `reinforce=true` + non-guidance dataset = ERROR (NonReinforcingApplier invariant protects every LLM-audit dataset from mutating substrate)
  - Reversing a grade (`POST /v1/review/reverse`) restores prior trust-score EXACTLY (REVIEW-GRADE-VALIDATE-ITEM-ID-001 pin test)
  - Auto-grader can refuse to grade (`AutoGraderPrefix` prefix + llm_dataset.FetchCandidates filters `caller_canceled:*` and empty responses per REVIEW-CANDIDATES-EXCLUDE-ERROR-ROWS-001)
  - `item_id` on grade POST MUST exist in the dataset (handler 404s on bogus; REVIEW-GRADE-VALIDATE-ITEM-ID-001 fix)
- Feature docs:
  - `docs/features/hitl-review.md` — platform mechanics
  - `docs/features/hitl-auto-curation.md` — the auto-grader shape
- CLAUDE.md pins: HITL-REVIEW-001, HITL-CURATION-002/003, HITL-AUTO-DISMISS-001, AUTOGRADE-SCHEDULE-001, HITL-AUTOGRADE-PREVIEW-001, HITL-ANALYTICS-TILE-001, REVIEW-GRADE-VALIDATE-ITEM-ID-001, REVIEW-CANDIDATES-DEDUP-409-001
- Where this feeds into: U8 Learning/Training (gold grades feed retrain), U5 RSIC (grade cadence feeds self-assessment), U4 Jiminy (correction grades mutate the corpus Jiminy reads)

**note**:
- Operator cue: "HITL is where the system's long-term improvement budget LANDS. 10 min of grading today is a measurable improvement signal next retrain."
- Transition: "U8 next — Learning / Training Data, where those grades get WOVEN into the next model generation."
