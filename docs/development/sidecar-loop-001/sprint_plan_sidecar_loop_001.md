# Sprint Plan — SIDECAR-LOOP-001: Make the Reranker Collection Loop Coherent (Fix-but-Defer)

## 1. Header & Metadata
2026-06-13 · branch `reh3376_dev01` · Roadmap Q3 Phase 4 (committed) ·
effort ~1.5d (operator chose fix-but-defer, not the full 3d train+A/B) ·
risk low–medium (a data-collection correctness fix + a one-time archive
of mislabeled data; no scoring change, no model trained this sprint).

## 2. Problem Statement
2.5 months of daily rerank-training collection (6,814 events) sat dormant
— the textbook dormant-writer. Recon (live-verified) found the data IS
teacher-labeled (top-level `rerank_scores[]`, LLM reranker output), so it
is conceptually a valid distillation set — but **100% of records are
mislabeled**: the collector logs `req.Candidates[:topN]` (unsorted input)
against `rerankScores` (rerank-sorted output), so candidate↔score
correspondence is broken by length (84%) AND by position (the remaining
16%). Separately, the trainer (`neural/neural_sidecar/train.py`) reads a
FLAT schema while the collector writes NESTED, so 0 records ever parsed.
Rerank is default-off, so reviving the path is not urgent — the operator
decision is to make the loop COHERENT and let a CLEAN dataset accumulate
to a useful size, then train later; keep only valuable data (finite
storage).

## 3. Scope & Constraints
**In**: (1) **Collector alignment fix** — `rerank.go` logs
`result.Results` + `result.RerankScores` (built together, positionally
aligned, length-equal) instead of the unsorted input slice; every future
record is correctly labeled. (2) **Collector guard** — `Collect` drops
any record where `len(candidates) != len(rerank_scores)` (and skips
all-zero score arrays) so the dataset is clean by construction; loud
debug log on drop. (3) **Trainer schema reconciliation** —
`train.py::load_jsonl` reads the nested records and explodes each event
into (query, candidate text, `rerank_scores[i]`) pairs; min-samples guard
preserved. (4) **Existing-data disposition** — the pre-fix data is 100%
mislabeled; archive it out of the active training dir into
`.mdemg/neural/training-data-prefix-archive/` (a reversible move, not a
hard delete — serves the operator's "no value-less data / finite storage"
intent while staying recoverable; the operator may delete the archive at
will). Disclosed: the operator's "keep the 16%" was premised on the
earlier framing that the 16% was clean; it is not (positional bug), so
none of the pre-fix data is kept active. (5) **Flag honesty** —
`NEURAL_DATA_COLLECTION` documented as explicit opt-in (it stays ON here
so the clean dataset builds). (6) **Readiness signal** — a one-shot
`mdemg` count of clean training records (or a documented threshold) so
"large enough to train" is observable.
**Out**: training the cross-encoder; the shadow-A/B vs LLM rerank_cross;
a UVTS gate (all deferred until the clean dataset reaches size and rerank
is enabled — the deferred half of fix-but-defer); any retrieval-scoring
change.

## 4. Dependencies
Recon (this dir); `internal/retrieval/{rerank.go:208, rerank_collector.go}`;
`neural/neural_sidecar/train.py`; `internal/config/config.go`
(NEURAL_DATA_COLLECTION:987/3985, NEURAL_DATA_DIR:988).

## 5. Implementation Plan
Epic 0 plan+recon · **Epic 1** collector alignment fix + guard (Go) +
unit tests (aligned record written; misaligned dropped) · **Epic 2**
trainer schema reconciliation (Python) + a parse test over a fixture of
the real nested format · **Epic 3** existing-data archive (reversible
move; documented) + flag-honesty doc + readiness count · **Epic 4** live
Tier 3 (drive a rerank with collection on → confirm a NEW record is
written with aligned candidate↔score; confirm trainer parses the new
format; confirm archive moved the old files) · **Epic 5** docs (feature
doc, CHANGELOG, post), push.

## 6. Testing Plan
T1 (Go): `Collect` writes a record whose candidates align 1:1 with scores;
a misaligned (len-mismatch) input is dropped, not written; all-zero scores
dropped. T1 (Python): `load_jsonl` explodes a nested fixture into the
right pair count with correct labels; rejects the old flat assumption
cleanly. T2: `go test ./internal/retrieval/...`; `pytest neural/` rerank
trainer tests; config scanner. T3 (live): with `NEURAL_DATA_COLLECTION=
true` + rerank forced on for one call, observe a fresh JSONL record where
`len(candidates)==len(rerank_scores)` and candidate[i].name corresponds to
the scored item; run `train.py --data-dir <new> --dry-run`/parse and
confirm >0 samples loaded from the new record (vs 0 before); confirm the
pre-fix files moved to the archive dir.

## 7. Commit Strategy
Per-epic · lint each (Go) + ruff (Python) · push once · summary · CI watch.

## 8. Verification Checklist
- [ ] Collector logs aligned (reranked-order) candidates + scores
- [ ] Misaligned / all-zero records dropped by the guard (test)
- [ ] train.py parses the nested format → >0 samples (was 0)
- [ ] Pre-fix mislabeled data archived (reversible), active dir clean
- [ ] NEURAL_DATA_COLLECTION documented opt-in; still collecting
- [ ] Clean-record readiness count available
- [ ] Training/A/B explicitly deferred + documented
- [ ] Feature doc + CHANGELOG + post

## 9. Documentation Update — Epic 5 (never cut).

## 10. Risks & Mitigations
Archiving deletes useful data → it's a reversible MOVE, and the data is
100% mislabeled (verified), so nothing trainable is lost; the archive is
recoverable. Collector fix changes record shape → trainer updated in the
same sprint (Epic 2) so the contract is consistent; old shape is archived
not fed. Guard drops legitimate records → only drops len-mismatch/all-zero
(which are unlabelable); logged. Deferring training leaves the loop
"unfinished" → that's the operator's explicit choice (default-off path;
build the clean dataset first); the readiness signal makes the resume
trigger observable.

## 11. Documents Accessed
ROADMAP:64; recon_findings.md (this dir); rerank.go/rerank_collector.go;
neural/neural_sidecar/train.py; data-decides-not-operator + per-feature-doc
memory rules; operator decision (fix-but-defer, keep-only-valuable).

## 12. Rollback Procedures
Code: revert commits (collector call site, guard, trainer loader).
Data: the archive is a move — restore by moving files back. Flag:
NEURAL_DATA_COLLECTION already config-gated. No model trained, nothing to
promote/roll back.
