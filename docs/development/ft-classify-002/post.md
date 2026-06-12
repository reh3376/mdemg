# FT-CLASSIFY-002 — Sprint Post

**Status: COMPLETE — gate verdict NO-PROMOTE, operator-accepted (option a)**
· 2026-06-12 · branch `reh3376_dev01` · PR #446

Verdict: candidate archived (`adapters/ft_classify_002/`,
`.local-models/ft-classify-002-candidate.Q5_K_M.gguf`,
`candidate_ftclassify002_fixedreward.json`). Production serving
unchanged. Gate table, stage timings, and all six 6a-grade findings:
`run_record.md` (the authoritative sprint record). CHANGELOG carries the
summary. The FT line's next moves: GUARDRAIL-PRODUCER-001 (producer for
guardrail.evaluate), then the FT-RECURSIVE-001 build sprints — for which
this slice was the proving run: the production-row → retrain →
regression-gate path is demonstrated end-to-end, including the gate
correctly rejecting a candidate.

Deviation note vs plan §8: "classify materially up" passed (+1.9pp)
against the corrected baseline; the plan's premise (0.668 baseline) was
itself measurement artifact — disclosed in the PR.
