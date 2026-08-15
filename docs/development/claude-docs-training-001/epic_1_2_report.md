# CLAUDE-DOCS-TRAINING-001 — Epic 1 + 2 Report

**Date**: 2026-08-14
**Status**: Epic 1 + 2 SHIPPED. Epic 3-6 pending; Epic 3 approach decision requested.

## Epic 1 — Discovery + robots.txt verdicts

### Domain reframe (sprint plan needed update)

Sprint plan targeted `docs.claude.com` + `docs.anthropic.com`. Actual state:
- `docs.claude.com` → 301 redirects to `platform.claude.com`
- `docs.anthropic.com` → 301 redirects to `platform.claude.com/docs`
- **`code.claude.com`** — separate domain (repo homepage on `anthropics/claude-code`) hosts the Claude Code CLI docs

**Recovered scope** (post-discovery):
1. **`code.claude.com/docs/en/*`** — Claude Code CLI docs (settings, hooks, slash commands, MCP, sub-agents, Agent SDK, workflows). 247 English URLs via sitemap.
2. **`platform.claude.com/docs/en/build-with-claude/*`** — Anthropic API usage prose (messages, streaming, tool_use). ~50 URLs.
3. ~~`platform.claude.com/docs/en/api/*`~~ — **BLOCKED** by robots.txt `Disallow: /api/`. Skipped.

### robots.txt verdicts (fetched 2026-08-14)

**`code.claude.com/robots.txt`** — explicit AI-training permission:
```
User-agent: *
Content-Signal: ai-train=yes, search=yes, ai-input=yes    ← Anthropic INVITES AI training
Disallow: /cdn-cgi/
Disallow: /_next/  (Allow: /_next/image)
Sitemap: https://code.claude.com/docs/sitemap.xml
```

**`platform.claude.com/robots.txt`**:
```
User-Agent: *
Disallow: /api/                     ← Messages API reference explicitly disallowed
Sitemap: https://platform.claude.com/sitemap.xml
```

### Two-orders-of-magnitude simplification found

Both domains publish `.md` files at every doc URL + `.md` suffix. **No HTML parsing needed** — the docs site serves the underlying markdown source directly. Sprint plan assumed HTML→text extraction via the docs-scraper plugin; the .md shortcut eliminates that entire step.

Both domains publish `llms.txt` — a curated LLM-training index maintained by Anthropic themselves. `code.claude.com/docs/llms.txt` lists ~200 URLs grouped by section; used as the source-of-truth for URL enumeration.

### Data-decided scope

**Included** (~150 URLs, 60% of the 247 total on code.claude.com):
- Core Documentation (overview, quickstart, how-claude-code-works, changelog)
- Features & Extension (memory, permission-modes, sessions, workflows, best-practices)
- Configuration (settings, permissions, sandbox-environments)
- **Reference** (cli-reference, commands, env-vars, tools-reference, hooks, keybindings) — operator's stated highest-priority target
- Multi-Agent (agents, sub-agents, agent-teams, workflows, worktrees)
- MCP & Extensions (mcp, mcp-quickstart, skills, plugins)
- Output & Collaboration (artifacts, hooks-guide, channels, scheduled-tasks)
- Agent SDK (all 26 URLs — overview, sessions, tools, subagents, hooks, TypeScript + Python refs)
- Platforms (vs-code, jetbrains, chrome, github-actions, slack)
- Troubleshooting + Admin/Setup + Security

**Excluded** (~50 URLs, low ROI for CLI-user knowledge):
- `/whats-new/*` — 20 weekly changelogs (churn-heavy, redundant with current docs)
- `/claude-apps-gateway-*` + `/self-hosted-environments-*` — enterprise deployment
- `/amazon-bedrock`, `/google-vertex-ai`, `/microsoft-foundry` — cloud-provider hosting
- `/llm-gateway-*` — LLM gateway config
- `/communications-kit`, `/champion-kit` — rollout/adoption

**`platform.claude.com/docs/en/build-with-claude/*`** — DEFERRED to a follow-up sprint. Current-turn corpus is Claude Code CLI knowledge only, per operator's explicit request ("slash commands and settings"). Anthropic API prose is a separate concern.

## Epic 2 — Scrape + cache raw corpus

### Deliverables

- `configs/scrape/claude_docs.yaml` — 130 URLs (final count after de-dup of the llms.txt list against my classification), rate_limit 1s, user_agent identifies as MDEMG training scraper
- `scripts/scrape_claude_docs.py` — Python scraper: fetches each URL + `.md`, respects rate limit, SHA-manifest, idempotent (cache-hit skips), fail-open on 4xx/5xx with success-rate gate
- `training_data/claude-docs/raw/*.md` — 130 files (gitignored; regenerable from the manifest)
- `training_data/claude-docs/scrape_manifest.json` — per-URL: fetch_ts, http_status, size_bytes, content_sha256, parsed title (source of truth for downstream curation)

### Live results

```
scrape complete in 188.32s:
  fetched_ok:     125
  cached_skipped: 5      (from the earlier 5-URL smoke)
  fetched_error:  0
  success_rate:   100.0%
```

Corpus stats:
- **130 files**, 6.4 MB total markdown
- **~680K words** total
- File sizes range 1KB (small reference) → 543KB (changelog) → 284KB (agent-sdk--typescript)

Epic 1 success-rate gate (≥95%) exceeded at 100%.

## Epic 3 — Curation design decision requested

The sprint plan called for "curate to structured Q&A pairs (~200-500 rows)" via deterministic per-concept-type extraction rules (one per settings key, one per slash command, etc). With 680K words across 130 pages, two viable curation approaches:

**Option A — Deterministic H2/H3 section extraction** (recommended for a first pass):
- Walk each markdown file; treat each `## Header` or `### Header` as one canonical concept.
- Emit Q&A pair as: `Q: What is <header>? / How do I <header>?` `A: <section body, code-blocks preserved verbatim>`.
- Pros: 100% reproducible, zero LLM cost, deterministic → curator output SHA-stable across reruns.
- Cons: Q phrasing is templated (less diverse); no filtering of low-value sections.
- Estimated output: ~800-1500 raw pairs → filter/quality-gate to ~400-600.

**Option B — LLM-based Q&A extraction** (mdemg-llm-v1 local, no OpenAI spend):
- Pass each markdown section to local LLM: "Extract N canonical Q&A pairs about this concept."
- Pros: Q phrasing is natural + diverse; can filter low-signal sections; extracts implicit Q&As from prose that lacks a header.
- Cons: recursive (local model generating its own training data — risk of amplifying model's current biases); ~15-30 min wall time; harder to audit than deterministic extraction.

**Option C — Hybrid**: Deterministic H2/H3 for the reference-shaped pages (cli-reference, commands, env-vars, keybindings, tools-reference — highly structured); LLM-based for the prose-heavy guides (best-practices, how-claude-code-works, common-workflows).

**Recommendation**: **Option A** for the first shipped corpus, **Option C** as a follow-up if quality is insufficient. Reason: option A is auditable, reproducible, and produces a solid baseline; if we later measure the resulting adapter and find quality gaps, hybrid can layer LLM-generated pairs on top.

**Waiting for operator sign-off on curation approach before proceeding to Epic 3.**

## Files touched this cycle

- `configs/scrape/claude_docs.yaml` (new)
- `scripts/scrape_claude_docs.py` (new)
- `training_data/claude-docs/raw/.gitignore` (new)
- `training_data/claude-docs/scrape_manifest.json` (new — 130 records)
- `docs/development/claude-docs-training-001/sprint_plan.md` (exists — prior sprint)
- `docs/development/claude-docs-training-001/epic_1_2_report.md` (this file)

## Next steps (post-approval)

- **Epic 3**: curate raw markdown → `training_data/claude-docs/curated/qa.jsonl` per chosen approach (A/B/C).
- **Epic 4**: register new ULTS task `claude.code_knowledge` + hand-author ~50-row golden holdout + UBENCH contract update.
- **Epic 5**: LoRA subset dry-run then full run atop Phase-5 adapter.
- **Epic 6**: HITL grade + A/B benchmark + operator promotion decision via `mdemg ft-loop promote`.

## Documents Accessed

- `plugins/docs-scraper/{manifest.json,fetcher.go,ingestion.go,extractor.go}` — assessed shipped plugin; concluded gRPC + Neo4j observation shape is wrong for our need (want raw markdown to disk); replaced with standalone Python scraper.
- `https://docs.claude.com/robots.txt` (301 → `platform.claude.com`)
- `https://docs.anthropic.com/robots.txt` (301 → `platform.claude.com/docs`)
- `https://platform.claude.com/robots.txt` — Disallow: /api/
- `https://platform.claude.com/sitemap.xml` — 1300+ URLs across 12 languages
- `https://platform.claude.com/docs/en/intro` — confirmed platform.claude.com is the API/agent docs surface
- `https://platform.claude.com/docs/en/build-with-claude/working-with-messages.md` — confirmed `.md` suffix serves raw markdown
- `https://platform.claude.com/docs/llms.txt` — 566 English pages (deferred to follow-up sprint)
- `gh api repos/anthropics/claude-code` — discovered `code.claude.com/docs/en/overview` via repo homepage field
- `https://code.claude.com/robots.txt` — `Content-Signal: ai-train=yes`
- `https://code.claude.com/docs/en/overview` — sidebar tree + CLI docs surface confirmation
- `https://code.claude.com/docs/llms.txt` — 200+ URLs, source-of-truth for URL enumeration
- `https://code.claude.com/docs/sitemap.xml` — 247 English URLs across `/docs/en/*` and `/docs/en/agent-sdk/*`
- Existing sprint context: `docs/development/claude-docs-training-001/sprint_plan.md`.
