# Retrieval — Per-Path Score Suppression

**Sprint**: RETRIEVAL-META-DOC-SUPPRESSION-001 (2026-08-24) · task #143
**Status**: shipped — verified 7/10 top-3 on MDEMG-usage probe suite (from 6/10 baseline)
**Feature surface**: env config only (`RETRIEVAL_SUPPRESS_PATHS`, `RETRIEVAL_SUPPRESS_FACTOR`)

## Why

MDEMG-DOCS-INGEST-001 (task #142) surfaced the "meta-doc dominance" problem: a small set of repo-root project-overview files (`CHANGELOG.md`, `CLAUDE.md`, `README.md`, etc.) systematically dominates retrieval results on MDEMG-usage queries via BM25 + short-content + repeated "MDEMG" term. These files score 10-100× above semantically-relevant feature docs but rarely contain the answer to any specific question.

**Not `activation_confidence`** (verified: all 3 initially-flagged files had default `act_conf=0.5`). **Not archivable** (each is a heavy graph hub — CHANGELOG.md has 531 outgoing edges anchoring L1 Hidden clusters). **Not fixable by jiminy re-rank** (LLM re-rank is nondeterministic; A/B showed it made things WORSE 6→4 top-3).

This feature is a narrow, targeted, reversible intervention that downweights the fused score of nodes at operator-specified paths.

## Choices

### Exact-path match (not regex)

Most auditable + surprise-free. Operator names specific paths; no accidental over-broadening. Feature doc + PR review make the list scrutinizable.

### Post-rerank hook (not pre-fusion)

The naive place to intervene is pre-fusion on `cand.RRFScore`. Doesn't work: the default column-voting path (`ScoreAndRankRRF`) overwrites `RRFScore` from column votes; the LLM reranker then rewrites `Score` wholesale. **Only a post-rerank hook survives all upstream re-scoring.** Sprint execution required 3 iterations to find this.

Hook position: `internal/retrieval/service.go` around line 1090, right after `ApplyReverseRefQuota` + before `ApplyDiversityFilter` + before topK truncation. Suppressed nodes' scores get multiplied → they slide down → they fall out of topK naturally.

### Default OFF (opt-in)

Empty `RETRIEVAL_SUPPRESS_PATHS` = no-op. Operators opt in only after live-verifying the pattern for their substrate.

### Cache-key namespace inclusion

`scorerVersion()` includes both `RetrievalSuppressPaths` (sorted + hashed) and `RetrievalSuppressFactor`. Any operator change to the config bumps the cache namespace, ensuring stale scores don't survive.

## How it works

```
   POST /v1/memory/retrieve
             │
             ▼
   ┌─────────────────────────────────────┐
   │ vector recall + BM25 in parallel    │
   └────────────────┬────────────────────┘
                    ▼
   ┌─────────────────────────────────────┐
   │ RRF fusion → []Candidate            │
   └────────────────┬────────────────────┘
                    ▼
   ┌─────────────────────────────────────┐
   │ SuppressCandidatesByPath (pre-fusion │
   │   safety net; column-voting later    │
   │   overwrites RRFScore so this is a   │
   │   defense-in-depth only)             │
   └────────────────┬────────────────────┘
                    ▼
   ┌─────────────────────────────────────┐
   │ seed extraction + spreading         │
   │   activation                        │
   └────────────────┬────────────────────┘
                    ▼
   ┌─────────────────────────────────────┐
   │ Scoring: ScoreAndRankRRF (default)  │
   │   OR ScoreAndRank (linear fallback) │
   │   OR ScoreAndRankWithBreakdown      │
   │   (jiminy)                          │
   └────────────────┬────────────────────┘
                    ▼
   ┌─────────────────────────────────────┐
   │ Sparse-gate filter                  │
   │ Reasoning module (optional)         │
   │ LLM reranker (default-on)           │
   │ Concrete-quota / reverse-ref quotas │
   └────────────────┬────────────────────┘
                    ▼
   ┌─────────────────────────────────────┐
   │ SuppressResultsByPath (POST-RERANK  │
   │   — the LOAD-BEARING hook. Score    │
   │   multiplied by factor for matched  │
   │   paths; stable re-sort desc.)       │
   └────────────────┬────────────────────┘
                    ▼
   ┌─────────────────────────────────────┐
   │ Diversity filter                    │
   │ TopK truncation                     │
   │ Normalized confidence               │
   └────────────────┬────────────────────┘
                    ▼
              response.Results
```

## How to use

### Enable

Add to `.env`:

```bash
# Downweight repo-root project-overview meta-docs
RETRIEVAL_SUPPRESS_PATHS=/.goreleaser.yaml,/CHANGELOG.md,/CLAUDE.md,/ELI5.md,/README.md,/VISION.md,/docs/FAQ.md,/docs/scrap.md
RETRIEVAL_SUPPRESS_FACTOR=0.3
```

For immediate effect without .env re-source:

```bash
launchctl setenv RETRIEVAL_SUPPRESS_PATHS "/.goreleaser.yaml,/CHANGELOG.md,/CLAUDE.md,/ELI5.md,/README.md,/VISION.md,/docs/FAQ.md,/docs/scrap.md"
launchctl setenv RETRIEVAL_SUPPRESS_FACTOR "0.3"
launchctl kickstart -k gui/$UID/com.mdemg.server
```

### Disable / revert

```bash
launchctl unsetenv RETRIEVAL_SUPPRESS_PATHS
launchctl unsetenv RETRIEVAL_SUPPRESS_FACTOR
# Remove from .env
# Then restart
launchctl kickstart -k gui/$UID/com.mdemg.server
```

### Choose the right factor

| Factor | Effect | Use case |
|---|---|---|
| **0.3** (default) | Downweight — matched nodes slide down; discoverable if they DO match a very specific query | Recommended default. Meta-docs stop dominating without being erased. |
| 0.1 | Aggressive downweight | If 0.3 doesn't suppress enough for your substrate. |
| 0.0 | Score drops to zero but node stays in the pool | Effectively archive-via-config; more conservative than actual archive because relationships stay intact. |
| 1.0 | No-op | Testing / verifying config plumbing works. |

### Choose the right path list

Start with the 8-file baseline that shipped this sprint (see `.env`). Extend by inspection:

```bash
# See what dominates top-5 for MDEMG-usage queries
python3 <<'PY'
import json, urllib.request
for q in ["how do I run mdemg X", "what does mdemg Y do", "how does Z work in mdemg"]:
    req = urllib.request.Request('http://127.0.0.1:9999/v1/memory/retrieve',
        data=json.dumps({'space_id': 'mdemg-dev', 'query_text': q, 'top_k': 5,
                         'candidate_k': 200, 'include_content': True}).encode(),
        headers={'Content-Type': 'application/json'})
    resp = json.loads(urllib.request.urlopen(req).read())
    print(f"\n{q}:")
    for r in resp.get('results', [])[:5]:
        print(f"  score={r.get('score',0):.3f}  path={r.get('path','?')}")
PY
```

Look for repo-root paths that recur across unrelated queries with high scores. Those are your suppress candidates.

### Verify the intervention worked

After enabling, use the sprint's probe harness at `/tmp/mdemg_probe.py` — 10 hand-authored MDEMG-usage queries across CLI/API/feature/config axes. Target: ≥7/10 top-3 (the ✅ threshold).

## Config reference

| Env var | Default | Purpose |
|---|---|---|
| `RETRIEVAL_SUPPRESS_PATHS` | (empty) | Comma-separated list of exact-match node paths to downweight. Empty = no-op. |
| `RETRIEVAL_SUPPRESS_FACTOR` | `0.3` | Score multiplier applied to matched paths. 0 = drop to zero; 1 = no-op. |

## References

- Sprint plan + verdict: `docs/development/retrieval-meta-doc-suppression-001/`
- Predecessor: `docs/features/mdemg-docs-ingest.md` (task #142 — surfaced the meta-doc dominance pattern)
- Deep-dive workflow `wf_b389463a-61b` — A2 investigation predicted this exact class
- CLAUDE.md pins: HEBB-ETA-001 (why activation_confidence is default 0.5 not elevated), RRF-SCALE-001 (fused-score contract)
