
======================================================================
=== PROBE RUN — jiminy_enabled=False ===
======================================================================

[ 1] 🟡     cli rank= #4 :: how do I run mdemg upgrade to update the binary and docker i
      top3: #1 YAML file: .goreleaser.yaml
Type: yaml-config

--- | #2 # Changelog

All notable changes to MDEMG will be  | #3 # MDEMG Project Instructions

---

## Repositories
[ 2] 🟡     cli rank= #4 :: what does mdemg data export do and what artifacts does it pr
      top3: #1 YAML file: .goreleaser.yaml
Type: yaml-config

--- | #2 # Changelog

All notable changes to MDEMG will be  | #3 # MDEMG Project Instructions

---

## Repositories
[ 3] ✅     cli rank= #1 :: how do I ingest MDEMG's own documentation into the substrate
[ 4] ❌     api rank= #6 :: what does POST /v1/jiminy/classify return and what fields do
      top3: #1 Go struct jiminy.ClassifyResponse
ClassifyResponse | #2 Go function jiminy.Classify
Classify determines th | #3 Pattern of 12 code elements in: conversation, jimi
[ 5] ✅     api rank= #1 :: what fields does GET /v1/jiminy/rules accept and how do I fi
[ 6] ❌     api rank=#10 :: what is the payload shape for POST /v1/memory/ingest with co
      top3: #1 Pattern of 9 code elements in: api-reference, inge | #2 ## What it does

`mdemg claude-docs-ingest` reads  | #3 Pattern of 9 code elements in: api-reference, inge
[ 7] ✅ feature rank= #1 :: how does the FT recursive retraining loop decide to promote 
[ 8] ✅ feature rank= #2 :: how does Jiminy classify guidance outcomes as followed ignor
[ 9] ✅  config rank= #2 :: what environment variables control RSIC alert thresholds and
[10] ✅  config rank= #1 :: what does MDEMG_MODEL_RAM_TIERS default to for the v2 base m

======================================================================
=== PROBE RUN — jiminy_enabled=True ===
======================================================================

[ 1] 🟡     cli rank= #4 :: how do I run mdemg upgrade to update the binary and docker i
      top3: #1 YAML file: .goreleaser.yaml
Type: yaml-config

--- | #2 # Changelog

All notable changes to MDEMG will be  | #3 # MDEMG Project Instructions

---

## Repositories
[ 2] ❌     cli rank= NF :: what does mdemg data export do and what artifacts does it pr
[ 3] 🟡     cli rank= #5 :: how do I ingest MDEMG's own documentation into the substrate
      top3: #1 ## How to use

### Full corpus ingest
```
mdemg cl | #2 ## How it works

### Ingest schema (per row)
- **P | #3 # Claude Code Docs — Substrate Ingest

**Sprint**:
[ 4] ❌     api rank= NF :: what does POST /v1/jiminy/classify return and what fields do
[ 5] ✅     api rank= #2 :: what fields does GET /v1/jiminy/rules accept and how do I fi
[ 6] ❌     api rank= NF :: what is the payload shape for POST /v1/memory/ingest with co
[ 7] ✅ feature rank= #1 :: how does the FT recursive retraining loop decide to promote 
[ 8] ✅ feature rank= #1 :: how does Jiminy classify guidance outcomes as followed ignor
[ 9] ✅  config rank= #3 :: what environment variables control RSIC alert thresholds and
[10] 🟡  config rank= #5 :: what does MDEMG_MODEL_RAM_TIERS default to for the v2 base m
      top3: #1 In Claude Code, what does 'Enforce the allowlist f | #2 ## 2. The J17 handshake (what the agent does)

The | #3 In Claude Code, what does 'Organization default mo

======================================================================
=== SIDE-BY-SIDE: default vs jiminy_enabled=true ===
======================================================================

probe                                               default   jiminy    delta
--------------------------------------------------------------------------------
[ 1]     cli how do I run mdemg upgrade to update t          4        4       +0
[ 2]     cli what does mdemg data export do and wha          4       NF      →NF
[ 3]     cli how do I ingest MDEMG's own documentat          1        5       +4
[ 4]     api what does POST /v1/jiminy/classify ret          6       NF      →NF
[ 5]     api what fields does GET /v1/jiminy/rules           1        2       +1
[ 6]     api what is the payload shape for POST /v1         10       NF      →NF
[ 7] feature how does the FT recursive retraining l          1        1       +0
[ 8] feature how does Jiminy classify guidance outc          2        1       -1
[ 9]  config what environment variables control RSI          2        3       +1
[10]  config what does MDEMG_MODEL_RAM_TIERS defaul          1        5       +4

metric                       default     jiminy
top-3                     6/10 (60%)     4/10 (40%)
top-5                     8/10 (80%)     7/10 (70%)
not-found                          0          3

❌ NO IMPROVEMENT (6→4) — jiminy re-rank doesn't fix the meta-doc dominance; fall back to activation_confidence intervention
