
=== MDEMG-DOCS-INGEST-001 Epic 5 — 10 live-verify probes ===


[1] ❌ cli: 'how do I run mdemg upgrade to update the binary and docker instances...'
    expect hint: 'mdemg upgrade' → rank #7
    #1 score=1.000: # CMS — Conversation Memory System  ## Goal  The Conversation Memory System (CMS) provides **persist
    #2 score=0.270: # MDEMG - Explain Like I'm 5 (Well, Maybe 12)  ## What's the Problem?  Have you ever used an AI chat
    #3 score=0.210: # MDEMG Project Instructions  ---  ## Repositories  | Role | Repo URL | |------|----------| | **MAIN
    #4 score=0.200: <!-- markdownlint-disable MD036 MD032 MD027 MD037 --> <!-- LinkedIn article format — bold pseudo-hea
    #5 score=0.180: # MDEMG - Multi-Dimensional Emergent Memory Graph  [![License: MIT](https://img.shields.io/badge/Lic

[2] ✅ cli: 'what does mdemg data export do and what artifacts does it produce...'
    expect hint: 'data export' → rank #1
    #1 score=0.406: ## Training Data Export (FT-DATA Sprint)  The export pipeline extracts TSDB training data as `.tar.g  <── hint found
    #2 score=0.300: # MDEMG - Explain Like I'm 5 (Well, Maybe 12)  ## What's the Problem?  Have you ever used an AI chat
    #3 score=0.240: # MDEMG Project Instructions  ---  ## Repositories  | Role | Repo URL | |------|----------| | **MAIN
    #4 score=0.210: # MDEMG - Multi-Dimensional Emergent Memory Graph  [![License: MIT](https://img.shields.io/badge/Lic
    #5 score=0.200: # Contributing to MDEMG  Thank you for your interest in contributing to MDEMG (Multi-Dimensional Eme

[3] ✅ cli: 'how do I ingest MDEMG's own documentation into the substrate...'
    expect hint: 'mdemg-docs-ingest' → rank #1
    #1 score=0.014: Ingest MDEMG's own documentation into the mdemg-dev substrate.  Sprint MDEMG-DOCS-INGEST-001 (task #  <── hint found
    #2 score=0.012: Pattern of 3 code elements in: jiminy-substrate-native-001, claude-docs-ingest-001, features
    #3 score=0.012: # Claude Code Docs — Substrate Ingest  **Sprint**: CLAUDE-DOCS-INGEST-001 (2026-08-17) **Status**: s
    #4 score=0.008: Pattern of 12 code elements in: unified-cli, api-reference, cms-rsic-guide, ingestion-guide, install
    #5 score=0.008: Pattern of 12 code elements in: unified-cli, api-reference, cms-rsic-guide, ingestion-guide, install

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
    #3 score=0.013: ## How to use  ### Operator flow (during arc window — READ-only)  1. Open `/ui/rules` — see the live
    #4 score=0.012: ## Configuration  | Env var | Default | Purpose | |---|---|---| | `JIMINY_RULES_UI_WRITE_ENABLED` | 
    #5 score=0.012: Pattern of 3 code elements in: embed-callsite-002, jiminy-rules-ui-001, features  <── hint found

[6] ❌ api: 'what is the payload shape for POST /v1/memory/ingest with content_hash...'
    expect hint: 'content_hash' → rank #10
    #1 score=0.032: Pattern of 9 code elements in: api-reference, ingestion-guide, ingest-codebase-api
    #2 score=0.032: ## What it does  `mdemg claude-docs-ingest` reads the curated Claude Code docs Q&A JSONL corpus (`tr
    #3 score=0.032: Pattern of 9 code elements in: api-reference, ingestion-guide, ingest-codebase-api
    #4 score=0.032: Meta-concept over 4 clusters. Sub-concepts: Pattern of 9 code elements in: api-reference, ingestion-
    #5 score=0.031: ## Ingestion Pipeline API  The ingestion trigger API (successor to the removed `/v1/memory/ingest-co

[7] ✅ feature: 'how does the FT recursive retraining loop decide to promote a new adap...'
    expect hint: 'promote' → rank #1
    #1 score=0.014: # FT Recursive-Retraining Loop — Observability (Phase 6a)  > Status: **Phase 6a shipped** (FT-RECURS
    #2 score=0.014: ## FT Recursive-Retraining Loop (FT-RECURSIVE-*)  ### `mdemg ft-loop`  **Synopsis:** `mdemg ft-loop 
    #3 score=0.014: Pattern of 7 code elements in: ft-recursive-loop, cli-reference, ft-loop
    #4 score=0.014: Pattern of 7 code elements in: ft-recursive-loop, cli-reference, ft-loop
    #5 score=0.013: # FT Recursive-Retraining Loop — Observability (Phase 6a)  > Status: **Phase 6a shipped** (FT-RECURS

[8] ✅ feature: 'how does Jiminy classify guidance outcomes as followed ignored or cont...'
    expect hint: 'outcome' → rank #2
    #1 score=0.029: Go package jiminy in file internal/jiminy/types.go  --- Code --- // Package jiminy provides the Jimi
    #2 score=0.015: --- created: 2026-03-30 updated: 2026-04-04 version: v0.5.4 author: reh3376 status: active phase: "A
    #3 score=0.013: ## Jiminy Inner-Voice Guidance  Jiminy is MDEMG's proactive guidance service -- an "inner voice" for
    #4 score=0.012: # JIMINY-RELEVANCE-001 — Step 1 Diagnostic: the ignored-guidance population  **Date:** 2026-06-23 · 
    #5 score=0.011: ## How It Works  ### Guide → Track → Feedback Flow  ``` ┌──────────────────┐       ┌────────────────

[9] ✅ config: 'what environment variables control RSIC alert thresholds and cooldowns...'
    expect hint: 'RSIC' → rank #2
    #1 score=0.068: What are the environment variables for Claude Code settings in Claude Code?  Environment variables l
    #2 score=0.046: # RSIC Guidance-Health Floors  **Shipped in:** DASHBOARD-TRUTH-003 (2026-08-01)  ## Why  Two RSIC se  <── hint found
    #3 score=0.040: ## Alert Sources  | Source | Service | Severities | |--------|---------|------------| | RSIC: Jiminy  <── hint found
    #4 score=0.039: # RSIC Orchestration — Admission, Archival Attribution, Rollback  ## Why  RSIC's self-improvement cy  <── hint found
    #5 score=0.039: Go package alert in file internal/alert/cooldown.go  --- Code --- package alert  import ( 	"sync" 	"

[10] ✅ config: 'what does MDEMG_MODEL_RAM_TIERS default to for the v2 base model...'
    expect hint: 'RAM' → rank #1
    #1 score=0.515: Go package config in file internal/config/model_defaults.go  --- Code --- package config  import "st
    #2 score=0.045: In Claude Code, what does 'Organization default model' mean in the context of Model configuration?  
    #3 score=0.045: In Claude Code, what does 'Default model behavior' mean in the context of Model configuration?  The 
    #4 score=0.041: In Claude Code, what does '`default` model setting' mean in the context of Model configuration?  The
    #5 score=0.026: ## Model versions  | Model | Base | Published | Adapter? | Default? | Notes | |---|---|---|---|---|-

============================================================

=== VERDICT SUMMARY (10 probes) ===
  in top-3     : 7/10  (70%)
  in top-5     : 7/10  (70%)
  not found    : 0/10

  VERDICT: ✅ PATH CLEAR — file MDEMG-USAGE-CORPUS-CURATE-001 as next sprint

  JSON written to /tmp/mdemg_probe_results.json
