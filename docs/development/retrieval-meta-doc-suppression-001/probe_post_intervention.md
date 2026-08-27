
=== MDEMG-DOCS-INGEST-001 Epic 5 — 10 live-verify probes ===


[1] 🟡 cli: 'how do I run mdemg upgrade to update the binary and docker instances...'
    expect hint: 'mdemg upgrade' → rank #4
    #1 score=0.100: YAML file: .goreleaser.yaml Type: yaml-config  --- Content --- # yaml-language-server: $schema=https
    #2 score=0.300: # Changelog  All notable changes to MDEMG will be documented in this file.  The format is based on [
    #3 score=0.700: # MDEMG Project Instructions  ---  ## Repositories  | Role | Repo URL | |------|----------| | **MAIN
    #4 score=0.047: # MDEMG Install Guide  MDEMG runs as a Docker Compose stack. This guide covers installation on all s
    #5 score=0.045: # Upgrade Guide  This guide covers upgrading between MDEMG versions. Start with the section matching

[2] 🟡 cli: 'what does mdemg data export do and what artifacts does it produce...'
    expect hint: 'data export' → rank #4
    #1 score=0.100: YAML file: .goreleaser.yaml Type: yaml-config  --- Content --- # yaml-language-server: $schema=https
    #2 score=0.400: # Changelog  All notable changes to MDEMG will be documented in this file.  The format is based on [
    #3 score=0.800: # MDEMG Project Instructions  ---  ## Repositories  | Role | Repo URL | |------|----------| | **MAIN
    #4 score=0.011: ## Training Data Export (FT-DATA Sprint)  The export pipeline extracts TSDB training data as `.tar.g  <── hint found
    #5 score=0.011: ## What it does  Closes the loop from `mdemg beta-share` (opt-in tester submission, shipped v0.11.0-

[3] ✅ cli: 'how do I ingest MDEMG's own documentation into the substrate...'
    expect hint: 'mdemg-docs-ingest' → rank #2
    #1 score=0.137: # Claude Code Docs — Substrate Ingest  **Sprint**: CLAUDE-DOCS-INGEST-001 (2026-08-17) **Status**: s
    #2 score=0.070: # MDEMG Docs — Substrate Ingest  **Sprint**: MDEMG-DOCS-INGEST-001 (2026-08-24) · task #142 **Status  <── hint found
    #3 score=0.052: Ingest MDEMG's own documentation into the mdemg-dev substrate.  Sprint MDEMG-DOCS-INGEST-001 (task #  <── hint found
    #4 score=0.037: Pattern of 3 code elements in: jiminy-substrate-native-001, claude-docs-ingest-001, features
    #5 score=0.032: ## How to use  ### Full corpus ingest ``` mdemg claude-docs-ingest ``` Reads `training_data/claude-d

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
    #4 score=0.012: ## Configuration  | Env var | Default | Purpose | |---|---|---| | `JIMINY_RULES_UI_WRITE_ENABLED` | 
    #5 score=0.012: Pattern of 3 code elements in: embed-callsite-002, jiminy-rules-ui-001, features  <── hint found

[6] ❌ api: 'what is the payload shape for POST /v1/memory/ingest with content_hash...'
    expect hint: 'content_hash' → rank #10
    #1 score=0.012: Pattern of 9 code elements in: api-reference, ingestion-guide, ingest-codebase-api
    #2 score=0.012: ## What it does  `mdemg claude-docs-ingest` reads the curated Claude Code docs Q&A JSONL corpus (`tr
    #3 score=0.012: Pattern of 9 code elements in: api-reference, ingestion-guide, ingest-codebase-api
    #4 score=0.012: Meta-concept over 4 clusters. Sub-concepts: Pattern of 9 code elements in: api-reference, ingestion-
    #5 score=0.011: ## Ingestion Pipeline API  The ingestion trigger API (successor to the removed `/v1/memory/ingest-co

[7] ✅ feature: 'how does the FT recursive retraining loop decide to promote a new adap...'
    expect hint: 'promote' → rank #1
    #1 score=0.102: ## FT Recursive-Retraining Loop (FT-RECURSIVE-*)  ### `mdemg ft-loop`  **Synopsis:** `mdemg ft-loop 
    #2 score=0.077: # FT Recursive-Retraining Loop — Observability (Phase 6a)  > Status: **Phase 6a shipped** (FT-RECURS
    #3 score=0.056: ## Phase 9 (FT-RECURSIVE-004, 2026-07-23) — drift monitoring: the arc is COMPLETE  - **`ft_loop_neve
    #4 score=0.056: ## Phase 7 (FT-RECURSIVE-003, 2026-07-23) — promotion executor, canary, auto-rollback, autonomy poli
    #5 score=0.055: ## The actuator (Phase 6b — FT-RECURSIVE-002, shipped default-off)  Phase 6b makes the no-op `trigge

[8] ✅ feature: 'how does Jiminy classify guidance outcomes as followed ignored or cont...'
    expect hint: 'outcome' → rank #2
    #1 score=0.029: Go package jiminy in file internal/jiminy/types.go  --- Code --- // Package jiminy provides the Jimi
    #2 score=0.014: --- created: 2026-03-30 updated: 2026-04-04 version: v0.5.4 author: reh3376 status: active phase: "A
    #3 score=0.013: ## Jiminy Inner-Voice Guidance  Jiminy is MDEMG's proactive guidance service -- an "inner voice" for
    #4 score=0.012: # JIMINY-RELEVANCE-001 — Step 1 Diagnostic: the ignored-guidance population  **Date:** 2026-06-23 · 
    #5 score=0.011: ## How It Works  ### Guide → Track → Feedback Flow  ``` ┌──────────────────┐       ┌────────────────

[9] ✅ config: 'what environment variables control RSIC alert thresholds and cooldowns...'
    expect hint: 'RSIC' → rank #2
    #1 score=0.069: What are the environment variables for Claude Code settings in Claude Code?  Environment variables l
    #2 score=0.045: # RSIC Guidance-Health Floors  **Shipped in:** DASHBOARD-TRUTH-003 (2026-08-01)  ## Why  Two RSIC se  <── hint found
    #3 score=0.040: ## Alert Sources  | Source | Service | Severities | |--------|---------|------------| | RSIC: Jiminy  <── hint found
    #4 score=0.039: Go package alert in file internal/alert/cooldown.go  --- Code --- package alert  import ( 	"sync" 	"
    #5 score=0.039: # RSIC Orchestration — Admission, Archival Attribution, Rollback  ## Why  RSIC's self-improvement cy  <── hint found

[10] ✅ config: 'what does MDEMG_MODEL_RAM_TIERS default to for the v2 base model...'
    expect hint: 'RAM' → rank #1
    #1 score=0.515: Go package config in file internal/config/model_defaults.go  --- Code --- package config  import "st
    #2 score=0.045: In Claude Code, what does 'Organization default model' mean in the context of Model configuration?  
    #3 score=0.045: In Claude Code, what does 'Default model behavior' mean in the context of Model configuration?  The 
    #4 score=0.041: In Claude Code, what does '`default` model setting' mean in the context of Model configuration?  The
    #5 score=0.026: ## Model versions  | Model | Base | Published | Adapter? | Default? | Notes | |---|---|---|---|---|-

============================================================

=== VERDICT SUMMARY (10 probes) ===
  in top-3     : 6/10  (60%)
  in top-5     : 8/10  (80%)
  not found    : 0/10

  VERDICT: ⚠️ MIXED — file narrow RETRIEVAL-META-DOC-SUPPRESSION-001 sprint scoped by ACTUAL failure pattern

  JSON written to /tmp/mdemg_probe_results.json
