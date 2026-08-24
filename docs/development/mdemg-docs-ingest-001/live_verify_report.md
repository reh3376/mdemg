
=== MDEMG-DOCS-INGEST-001 Epic 5 — 10 live-verify probes ===


[1] 🟡 cli: 'how do I run mdemg upgrade to update the binary and docker instances...'
    expect hint: 'mdemg upgrade' → rank #4
    #1 score=0.100: YAML file: .goreleaser.yaml Type: yaml-config  --- Content --- # yaml-language-server: $schema=https
    #2 score=0.300: # Changelog  All notable changes to MDEMG will be documented in this file.  The format is based on [
    #3 score=0.700: # MDEMG Project Instructions  ---  ## Repositories  | Role | Repo URL | |------|----------| | **MAIN
    #4 score=0.030: ## Updating  ### Homebrew (recommended)  ```bash brew upgrade mdemg ```  This updates the binary and
    #5 score=0.014: Download and install the latest mdemg release from GitHub, then update all running MDEMG Docker Comp

[2] ❌ cli: 'what does mdemg data export do and what artifacts does it produce...'
    expect hint: 'data export' → rank #7
    #1 score=0.100: YAML file: .goreleaser.yaml Type: yaml-config  --- Content --- # yaml-language-server: $schema=https
    #2 score=0.400: # Changelog  All notable changes to MDEMG will be documented in this file.  The format is based on [
    #3 score=0.800: # MDEMG Project Instructions  ---  ## Repositories  | Role | Repo URL | |------|----------| | **MAIN
    #4 score=0.029: ## Overview  The `mdemg teardown` command completely removes all traces of an MDEMG instance from a 
    #5 score=0.012: Pattern of 19 code elements in: beta-import, beta-share

[3] ✅ cli: 'how do I ingest MDEMG's own documentation into the substrate...'
    expect hint: 'mdemg-docs-ingest' → rank #2
    #1 score=0.137: # Claude Code Docs — Substrate Ingest  **Sprint**: CLAUDE-DOCS-INGEST-001 (2026-08-17) **Status**: s
    #2 score=0.052: Ingest MDEMG's own documentation into the mdemg-dev substrate.  Sprint MDEMG-DOCS-INGEST-001 (task #  <── hint found
    #3 score=0.035: Pattern of 3 code elements in: jiminy-substrate-native-001, claude-docs-ingest-001, features
    #4 score=0.032: ## How to use  ### Full corpus ingest ``` mdemg claude-docs-ingest ``` Reads `training_data/claude-d
    #5 score=0.008: Pattern of 15 code elements in: user, tests, homebrew-mdemg, mdemg-windows, development

[4] ❌ api: 'what does POST /v1/jiminy/classify return and what fields does the res...'
    expect hint: 'verdict' → rank #6
    #1 score=0.046: Go struct jiminy.ClassifyResponse ClassifyResponse is the output from the /strict classification end
    #2 score=0.022: Go function jiminy.Classify Classify determines the outcome of a guidance item given an action summa
    #3 score=0.012: Pattern of 12 code elements in: conversation, jiminy, consulting, retrieval, api
    #4 score=0.012: Go struct jiminy.ClassifyRequest ClassifyRequest is the input to the /strict classification endpoint
    #5 score=0.011: Go struct jiminy.ClassificationResult ClassificationResult holds the result of semantic outcome clas

[5] ✅ api: 'what fields does GET /v1/jiminy/rules accept and how do I filter by ty...'
    expect hint: 'jiminy-rules' → rank #1
    #1 score=0.015: # Jiminy Rules UI  **Status:** In development (JIMINY-RULES-UI-001, 2026-08-13) — Epic 1 shipping no  <── hint found
    #2 score=0.014: ## How it works  ### Data model (unchanged from shipped substrate)  Rules are `MemoryNode` records i
    #3 score=0.014: ## How to use  ### Operator flow (during arc window — READ-only)  1. Open `/ui/rules` — see the live
    #4 score=0.012: ## Why  Jiminy enforces a live corpus of ~33 constraint + 3 correction nodes (post-JIMINY-CORPUS-003
    #5 score=0.012: # Sprint Plan — JIMINY-RULES-UI-001  ## 1. Header & Metadata  - **Sprint ID:** JIMINY-RULES-UI-001 -  <── hint found

[6] 🟡 api: 'what is the payload shape for POST /v1/memory/ingest with content_hash...'
    expect hint: 'content_hash' → rank #5
    #1 score=0.033: ## What it does  `mdemg claude-docs-ingest` reads the curated Claude Code docs Q&A JSONL corpus (`tr
    #2 score=0.032: ## Ingestion Pipeline API  The ingestion trigger API (successor to the removed `/v1/memory/ingest-co
    #3 score=0.031: Pattern of 4 code elements in: development, docs, specs
    #4 score=0.031: ## Codebase Ingestion API  **Removed in DORMANT-CENSUS-001.** The legacy `/v1/memory/ingest-codebase
    #5 score=0.031: # Claude Code Docs — Substrate Ingest  **Sprint**: CLAUDE-DOCS-INGEST-001 (2026-08-17) **Status**: s

[7] ✅ feature: 'how does the FT recursive retraining loop decide to promote a new adap...'
    expect hint: 'promote' → rank #1
    #1 score=0.015: # FT Recursive-Retraining Loop — Observability (Phase 6a)  > Status: **Phase 6a shipped** (FT-RECURS
    #2 score=0.015: ## FT Recursive-Retraining Loop (FT-RECURSIVE-*)  ### `mdemg ft-loop`  **Synopsis:** `mdemg ft-loop 
    #3 score=0.014: # FT Recursive-Retraining Loop — Observability (Phase 6a)  > Status: **Phase 6a shipped** (FT-RECURS
    #4 score=0.013: ## Phase 9 (FT-RECURSIVE-004, 2026-07-23) — drift monitoring: the arc is COMPLETE  - **`ft_loop_neve
    #5 score=0.013: # Adapter Promotion Gate — Trustworthy Eval  **Sprint**: EVAL-INTEGRITY-001 (2026-06-13) · lead spri

[8] ✅ feature: 'how does Jiminy classify guidance outcomes as followed ignored or cont...'
    expect hint: 'outcome' → rank #1
    #1 score=0.090: --- created: 2026-03-30 updated: 2026-04-04 version: v0.5.4 author: reh3376 status: active phase: "A
    #2 score=0.079: # JIMINY-ACTIONABILITY-001 — Baseline Composition (Epic 1)  The before-state, from the live `constra  <── hint found
    #3 score=0.074: # Sprint JIMINY-CONTRADICTED-BRIDGE-001 — contradicted-outcome → correction draft bridge  ## 1. Head  <── hint found
    #4 score=0.066: ## Dataset: contradicted_drafts (Sprint JIMINY-CONTRADICTED-BRIDGE-001, 2026-07-20)  **Purpose.** Wh
    #5 score=0.063: ## Jiminy Inner-Voice Guidance  Jiminy is MDEMG's proactive guidance service -- an "inner voice" for

[9] ✅ config: 'what environment variables control RSIC alert thresholds and cooldowns...'
    expect hint: 'RSIC' → rank #2
    #1 score=0.031: Go package alert in file internal/alert/cooldown.go  --- Code --- package alert  import ( 	"sync" 	"
    #2 score=0.014: ## What is RSIC?  The **Recursive Self-Improvement Cycle** is MDEMG's automated memory maintenance s  <── hint found
    #3 score=0.012: # Alert Dispatcher (SR-001)  Service resilience alert system that delivers MDEMG health events to th
    #4 score=0.012: ## Alert Sources  | Source | Service | Severities | |--------|---------|------------| | RSIC: Jiminy  <── hint found
    #5 score=0.012: Pattern of 10 code elements in: alert-dispatcher

[10] ✅ config: 'what does MDEMG_MODEL_RAM_TIERS default to for the v2 base model...'
    expect hint: 'RAM' → rank #1
    #1 score=0.415: Go package config in file internal/config/model_defaults.go  --- Code --- package config  import "st
    #2 score=0.007: # Local Model Distribution  **Sprint**: MODEL-DIST-001 (2026-05-11), MODEL-DIST-002 adapter path (20
    #3 score=0.006: In Claude Code, what does '`default` model setting' mean in the context of Model configuration?  The
    #4 score=0.006: ## How to use  ### Quick start  ```bash # Prerequisites: ollama installed brew install ollama       
    #5 score=0.006: ## Model versions  | Model | Base | Published | Adapter? | Default? | Notes | |---|---|---|---|---|-

============================================================

=== VERDICT SUMMARY (10 probes) ===
  in top-3     : 6/10  (60%)
  in top-5     : 8/10  (80%)
  not found    : 0/10

  VERDICT: ⚠️ MIXED — file narrow RETRIEVAL-META-DOC-SUPPRESSION-001 sprint scoped by ACTUAL failure pattern

  JSON written to /tmp/mdemg_probe_results.json
