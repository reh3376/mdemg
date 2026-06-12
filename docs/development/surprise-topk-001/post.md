# SURPRISE-TOPK-001 — Sprint Post

**Status: COMPLETE** · 2026-06-12 · branch `reh3376_dev01`

## Verdict
The surprise→edge-weight chain fires for the first time in system
history. Live smoke: session factor distribution `{1.0: 4, 1.5: 3}`
(was 1.0×221,504 all-time). HIGH (2.0×) verified reachable by
thresholds (corrections score ~0.455 ≥ 0.4); first live 2.0 row expected
from ongoing correction traffic — MEDIUM confirmed live satisfies the
plan's gate ("factors > 1.0 appear").

## The three compounding causes (all addressed or recorded)
1. Unordered LIMIT 50 novelty sample → exact ORDER-BY top-K
   (config-driven; vector-index route live-rejected: label-wide index
   crowded by emergent_concept centroids — evidence in plan/commits).
2. **78% empty-array embeddings** on conversation observations
   (4,564/5,810) poisoning the average via NULL cosines → guarded by
   `size(embedding)=dims`; backfill recorded as follow-up
   (`mdemg embeddings backfill`).
3. Thresholds 0.7/0.4 unreachable on the real scale → config-driven
   (0.4/0.15 live-calibrated: corrections ~0.455, noise 0.02–0.10) at
   BOTH ApplyCoactivation and CoactivateSession (audit confirmed the
   session path computes the CASE).

Also: the silent embedding-novelty error path now logs loudly.

## Follow-ups recorded
- Embedding backfill for the 4,564 empty-embedding observations.
- SURPRISE-TERMS-001 candidate: non-correction discrimination is weak
  (novel 0.10 vs mundane 0.08) — the term-novelty component, not this
  sprint's scope.
- Revisit exact-scan → index when the role population nears ~50k.

## Tier evidence
T1 unit green (conversation/learning/config); scanner 673/673 consumed;
lint 0; live before/after distributions captured in plan + this post.
