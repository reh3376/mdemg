# MDEMG Web Scraper — Developer Guide

## Purpose

The MDEMG Web Scraper is an ingestion module that converts external web content into persistent memory nodes within the MDEMG knowledge graph. It fetches web pages, extracts meaningful content from HTML, scores quality, suggests tags, checks for duplicates against existing memory, and presents everything for human review before committing to the graph.

The core problem it solves: getting structured, high-quality external knowledge (documentation sites, reference material, support articles) into the same memory graph that MDEMG uses for conversation memory, learning, and retrieval — without manual copy-paste or unreviewed bulk imports.

## Goals

1. **Automated ingestion** — Given one or more URLs, fetch, parse, and extract content without manual intervention
2. **Quality gate** — Every scraped page goes through human review (approve/reject/edit) before entering the graph
3. **Deduplication** — Vector similarity checks prevent importing content that already exists in the target space
4. **BFS link-following** — Optionally crawl outward from seed URLs, following same-domain links up to a configurable depth and page limit
5. **Async execution** — Jobs run in the background; the API returns immediately with a job ID for polling
6. **Plugin isolation** — All web fetching, HTML parsing, and content extraction happen in a separate out-of-process binary communicating via gRPC, keeping the core server clean

## Architecture

The scraper is split into two independent processes that communicate over a Unix socket via gRPC:

```
┌──────────────────────────────────────┐     ┌──────────────────────────────────┐
│  MDEMG Core Server                   │     │  docs-scraper Plugin (sidecar)   │
│  (internal/scraper + internal/api)   │     │  (plugins/docs-scraper/)         │
│                                      │     │                                  │
│  REST API                            │     │  gRPC Services:                  │
│    POST   /v1/scraper/jobs           │     │    ModuleLifecycle               │
│    GET    /v1/scraper/jobs           │     │      Handshake / Health / Shutdown│
│    GET    /v1/scraper/jobs/{id}      │     │    IngestionModule               │
│    DELETE /v1/scraper/jobs/{id}      │     │      Matches(url) → confidence   │
│    POST   /v1/scraper/jobs/{id}/     │     │      Parse(url) → observations   │
│           review                     │     │      Sync(urls) → stream obs     │
│    GET    /v1/scraper/spaces         │     │                                  │
│                                      │     │  Internal Pipeline:              │
│  Core Logic:                         │     │    HTTP Fetcher (auth, timeout,  │
│    Job queue + async orchestration   │◄───►│      robots.txt, rate limit)     │
│    BFS link-following (crawl)        │gRPC │    HTML → Markdown extraction    │
│    Neo4j persistence (ScrapeJob,     │Unix │    Documentation & Generic       │
│      ScrapedContent nodes)           │sock │      extraction profiles         │
│    Vector similarity dedup (0.85)    │     │    Quality scoring (0.0–1.0)     │
│    Human review workflow             │     │    Auto-tag suggestion           │
│    Ingestion → conversation.Observe  │     │    Link discovery                │
│    INGESTED_AS relationship          │     │                                  │
└──────────────────────────────────────┘     └──────────────────────────────────┘
```

### Why a Plugin (Sidecar)?

The scraper follows MDEMG's established plugin architecture (`api/proto/mdemg-module.proto`). The plugin:

- Runs as a **separate OS process**, started and managed by the plugin manager
- Communicates via **gRPC over a Unix socket** (`/tmp/mdemg-plugins/mdemg-docs-scraper.sock`)
- Implements two gRPC service contracts: `ModuleLifecycle` (handshake, health, shutdown) and `IngestionModule` (matches, parse, sync)
- Is **auto-discovered** by the plugin manager on server startup — no manual registration needed
- Crashes in the plugin do not affect the core server

This mirrors the existing `linear-module` and `test-ingestion-plugin` patterns.

## Enabling the Scraper

The scraper is **disabled by default**. To enable it:

### 1. Set the environment variable

```bash
export SCRAPER_ENABLED=true
```

Or add to your `.env` file:

```
SCRAPER_ENABLED=true
```

### 2. Build the plugin binary

```bash
go build -o plugins/docs-scraper/docs-scraper ./plugins/docs-scraper/
```

The binary must be at `plugins/docs-scraper/docs-scraper` (matching the `binary` field in `manifest.json`).

### 3. Start the server

```bash
./mdemg-server
```

On startup the server will:

1. Read `SCRAPER_ENABLED=true` from config
2. Create the `scraper.Service` with Neo4j driver, embedder, and plugin manager
3. The plugin manager discovers `plugins/docs-scraper/manifest.json`
4. The plugin manager launches the `docs-scraper` binary as a child process
5. gRPC handshake completes — plugin is ready
6. Server logs: `Web scraper enabled (space: web-scraper, max_jobs: 3)`

If `SCRAPER_ENABLED=false` (default), all `/v1/scraper/*` endpoints return `503 Service Unavailable` with `{"error": "scraper not enabled"}`.

### Plugin Registration (manifest.json)

The plugin self-describes via `plugins/docs-scraper/manifest.json`:

```json
{
  "id": "docs-scraper",
  "name": "Documentation Scraper",
  "version": "1.0.0",
  "type": "INGESTION",
  "binary": "docs-scraper",
  "capabilities": {
    "ingestion_sources": ["https://", "http://"],
    "content_types": ["text/html"]
  },
  "health_check_interval_ms": 10000,
  "startup_timeout_ms": 15000,
  "config": {
    "default_profile": "documentation",
    "rate_limit_ms": "1000",
    "respect_robots_txt": "true",
    "max_content_length_kb": "500",
    "user_agent": "MDEMG-Scraper/1.0"
  }
}
```

The `config` block is passed to the plugin during the gRPC handshake. The plugin applies these values as runtime defaults.

## Configuration Reference

### Server Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SCRAPER_ENABLED` | `false` | Master switch — enables scraper service and API endpoints |
| `SCRAPER_DEFAULT_SPACE_ID` | `web-scraper` | Default Neo4j space for ingested content |
| `SCRAPER_MAX_CONCURRENT_JOBS` | `3` | Maximum concurrent scrape jobs |
| `SCRAPER_DEFAULT_DELAY_MS` | `1000` | Default delay between HTTP requests (ms) |
| `SCRAPER_DEFAULT_TIMEOUT_MS` | `30000` | Default HTTP timeout per request (ms) |
| `SCRAPER_CACHE_TTL_SECONDS` | `3600` | robots.txt cache TTL (seconds) |
| `SCRAPER_RESPECT_ROBOTS_TXT` | `true` | Whether the plugin checks robots.txt before fetching |
| `SCRAPER_MAX_CONTENT_LENGTH_KB` | `500` | Max page content size — HTML beyond this is truncated |

### Plugin Config (via manifest.json)

| Key | Default | Description |
|-----|---------|-------------|
| `default_profile` | `documentation` | Extraction profile when not specified per-job |
| `rate_limit_ms` | `1000` | Minimum delay between requests within the plugin |
| `respect_robots_txt` | `true` | Check robots.txt before each fetch |
| `max_content_length_kb` | `500` | Truncate HTML bodies exceeding this size |
| `user_agent` | `MDEMG-Scraper/1.0` | HTTP User-Agent header |

## API Endpoints

All endpoints are under `/v1/scraper/`. All return JSON. All require `Content-Type: application/json` for POST requests.

---

### POST /v1/scraper/jobs — Create Job

Creates a new asynchronous scraping job. Returns immediately with `202 Accepted`.

**Request Body:**

```json
{
  "urls": ["https://example.com/docs/getting-started"],
  "target_space_id": "my-docs",
  "options": {
    "extraction_profile": "documentation",
    "follow_links": true,
    "max_depth": 2,
    "max_pages": 20,
    "delay_ms": 1500,
    "timeout_ms": 30000,
    "auth": {
      "type": "header",
      "credentials": {
        "Authorization": "Bearer <token>"
      }
    }
  }
}
```

**Fields:**

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `urls` | Yes | — | One or more seed URLs to scrape (must be `http://` or `https://`) |
| `target_space_id` | No | `web-scraper` | Neo4j space where approved content will be ingested |
| `options.extraction_profile` | No | `documentation` | `"documentation"` or `"generic"` (see Extraction Profiles below) |
| `options.follow_links` | No | `false` | Enable BFS link-following from seed URLs |
| `options.max_depth` | No | `2` (when follow_links=true) | How many link levels deep to crawl from seed URLs |
| `options.max_pages` | No | `50` (when follow_links=true) | Hard cap on total pages scraped per job |
| `options.delay_ms` | No | `1000` | Delay between HTTP requests (rate limiting) |
| `options.timeout_ms` | No | `30000` | HTTP timeout per individual page fetch |
| `options.auth.type` | No | `none` | Authentication method: `"none"`, `"cookie"`, `"header"`, `"basic"` |
| `options.auth.credentials` | No | — | Key-value pairs for auth (e.g., `{"Authorization": "Bearer ..."}`) |

**Response (202 Accepted):**

```json
{
  "job_id": "scrape-a94f0390",
  "status": "pending",
  "urls": ["https://example.com/docs/getting-started"],
  "target_space_id": "my-docs",
  "total_urls": 1,
  "processed_urls": 0,
  "created_at": "2026-02-07T19:42:22Z",
  "updated_at": "2026-02-07T19:42:22Z"
}
```

**Example — basic single-page scrape:**

```bash
curl -s -X POST http://localhost:9999/v1/scraper/jobs \
  -H "Content-Type: application/json" \
  -d '{"urls":["https://pkg.go.dev/net/http"],"target_space_id":"go-docs"}'
```

**Example — crawl with link-following:**

```bash
curl -s -X POST http://localhost:9999/v1/scraper/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "urls": ["https://support.claude.com/en/"],
    "target_space_id": "claude-docs",
    "options": {
      "follow_links": true,
      "max_depth": 2,
      "max_pages": 15,
      "delay_ms": 1500
    }
  }'
```

---

### GET /v1/scraper/jobs — List Jobs

Returns all scrape jobs.

```bash
curl -s http://localhost:9999/v1/scraper/jobs | jq .
```

**Response:**

```json
{
  "jobs": [
    {
      "job_id": "scrape-a94f0390",
      "status": "awaiting_review",
      "urls": ["https://pkg.go.dev/net/http"],
      "target_space_id": "go-docs",
      "total_urls": 1,
      "processed_urls": 5,
      "created_at": "2026-02-07T19:42:22Z",
      "updated_at": "2026-02-07T19:42:35Z"
    }
  ],
  "count": 1
}
```

---

### GET /v1/scraper/jobs/{job_id} — Get Job Status + Content

Returns full job details including all scraped content items.

```bash
curl -s http://localhost:9999/v1/scraper/jobs/scrape-a94f0390 | jq .
```

**Response includes `contents` array** with each scraped page:

```json
{
  "job_id": "scrape-a94f0390",
  "status": "awaiting_review",
  "processed_urls": 5,
  "contents": [
    {
      "content_id": "e2f1c3a4-...",
      "url": "https://pkg.go.dev/net/http",
      "title": "http package - net/http - Go Packages",
      "content_preview": "# http package...",
      "quality_score": 0.85,
      "word_count": 21447,
      "suggested_tags": ["source:web-scraper", "domain:pkg.go.dev", "docs"],
      "similar_existing": [],
      "status": "pending_review"
    }
  ]
}
```

---

### DELETE /v1/scraper/jobs/{job_id} — Cancel Job

Cancels a running job. Pages already scraped remain in the database for review.

```bash
curl -s -X DELETE http://localhost:9999/v1/scraper/jobs/scrape-a94f0390 | jq .
```

**Response:**

```json
{
  "job_id": "scrape-a94f0390",
  "status": "cancelled",
  "message": "job cancelled"
}
```

---

### POST /v1/scraper/jobs/{job_id}/review — Review Content

Submit review decisions for scraped content items. Three actions are available:

| Action | Behavior |
|--------|----------|
| `approve` | Ingest into the memory graph as a pinned `learning` observation via `conversation.Observe()` |
| `reject` | Mark as rejected — not ingested |
| `edit` | Update content and/or tags, then ingest the edited version |

```bash
curl -s -X POST http://localhost:9999/v1/scraper/jobs/scrape-a94f0390/review \
  -H "Content-Type: application/json" \
  -d '{
    "decisions": [
      {"content_id": "e2f1c3a4-...", "action": "approve"},
      {"content_id": "b7d2e8f1-...", "action": "reject"},
      {"content_id": "c3a4f5d6-...", "action": "edit", "edit_content": "Cleaned up text", "edit_tags": ["go", "stdlib"]}
    ]
  }'
```

**Decision fields:**

| Field | Required | Description |
|-------|----------|-------------|
| `content_id` | Yes | The `content_id` from the scraped content item |
| `action` | Yes | `"approve"`, `"reject"`, or `"edit"` |
| `edit_content` | No | Replacement content (only with `"edit"` action) |
| `edit_tags` | No | Replacement tags (only with `"edit"` action) |
| `space_id` | No | Override the target space for this specific item |

**Response:**

```json
{
  "job_id": "scrape-a94f0390",
  "reviewed": 3,
  "ingested": [
    {"content_id": "e2f1c3a4-...", "node_id": "mem-abc123", "url": "https://..."},
    {"content_id": "c3a4f5d6-...", "node_id": "mem-def456", "url": "https://..."}
  ],
  "rejected": 1,
  "status": "completed"
}
```

When all content items for a job have been reviewed, the job status transitions to `completed`.

---

### GET /v1/scraper/spaces — List Spaces

Returns distinct `space_id` values from existing MemoryNodes, useful for selecting a `target_space_id`.

```bash
curl -s http://localhost:9999/v1/scraper/spaces | jq .
```

**Response:**

```json
{
  "spaces": [
    {"space_id": "mdemg-dev", "node_count": 142},
    {"space_id": "web-scraper", "node_count": 35}
  ],
  "count": 2
}
```

## Job Lifecycle

```
  POST /v1/scraper/jobs
         │
         ▼
    ┌──────────┐
    │  pending  │  ← 202 returned to caller immediately
    └────┬─────┘
         │  (async goroutine starts)
         ▼
    ┌──────────┐
    │  running  │  ← Plugin fetches each URL via gRPC Parse()
    │          │     BFS enqueues discovered links (if follow_links=true)
    │          │     Dedup check against existing MemoryNodes
    │          │     Rate limiting between requests
    └────┬─────┘
         │  (all URLs processed or max_pages reached)
         ▼
  ┌─────────────────┐
  │ awaiting_review  │  ← Human reviews each scraped page
  └───────┬─────────┘
          │  POST .../review with approve/reject/edit
          ▼
    ┌───────────┐
    │ completed  │  ← Approved items now exist as MemoryNodes
    └───────────┘     with [:INGESTED_AS] relationship

  At any time:
    DELETE /v1/scraper/jobs/{id} → cancelled
    Plugin errors → failed (per-content or whole job)
```

### Status Values

**Job statuses:** `pending` → `running` → `awaiting_review` → `completed` (also: `cancelled`, `failed`)

**Content statuses:** `pending_review` → `approved`/`rejected`/`ingested` (also: `failed`)

## BFS Link-Following (Crawl Mode)

When `follow_links: true` is set in the job options, the orchestrator performs breadth-first search crawling:

1. Seed URLs are enqueued at **depth 0**
2. After each page is fetched and parsed, the plugin returns all discovered `<a href>` links in the observation metadata
3. The orchestrator filters links: **same-domain only**, not already visited, URL normalized (fragments stripped, trailing slashes removed)
4. New links are enqueued at **depth + 1** if `depth < max_depth`
5. Processing continues until the queue is empty or `max_pages` total pages have been scraped

**Defaults when follow_links=true:**

- `max_depth` = 2 (seed pages + 2 levels of links)
- `max_pages` = 50 (hard cap regardless of queue size)

**Important notes:**

- JavaScript-rendered sites (SPAs) will have few or no discoverable links since the scraper performs simple HTTP GETs without a headless browser
- The delay between requests (`delay_ms`) applies to every page in the BFS traversal
- All discovered pages go through the same quality scoring, tagging, and dedup pipeline as seed pages

## Extraction Profiles

### `documentation` (default)

Optimized for technical documentation sites. The plugin:

1. Removes `<script>`, `<style>`, `<nav>`, `<footer>`, `<header>` tags and their contents
2. Removes elements with class/id containing: `sidebar`, `nav`, `menu`, `breadcrumb`, `cookie`, `banner`
3. Extracts content from `<main>` → `<article>` → `<body>` (first found)
4. Converts HTML to Markdown (headings, code blocks, links, lists, bold/italic)
5. Cleans whitespace

### `generic`

Minimal filtering for general web pages:

1. Removes only `<script>` and `<style>` tags
2. Extracts full `<body>` content
3. Converts HTML to Markdown

## Quality Scoring

Each scraped page receives a quality score from 0.0 to 1.0, computed by the plugin using weighted heuristics:

| Factor | Weight | High Score | Low Score |
|--------|--------|------------|-----------|
| Content length | 25% | 500–5000 words | <50 or >50,000 words |
| Headings present | 20% | Multiple heading levels | No headings |
| Code blocks | 15% | Code examples present | No code |
| Link density | 15% (penalty) | Low link-to-text ratio | >50% links |
| Text coherence | 15% | Long sentences, paragraphs | Short fragments |
| Lists | 10% | Structured list content | No lists |

Quality scores help during review — low-quality pages (e.g., JS-rendered shells with 51 words) can be quickly rejected.

## Deduplication

Before presenting content for review, the orchestrator:

1. Embeds the scraped text using the configured embedding model
2. Queries the Neo4j vector index (`memNodeEmbedding`) for the top 5 most similar existing MemoryNodes in the target space
3. Attaches any nodes with similarity >= 0.85 to the `similar_existing` field on the ScrapedContent

This is informational — the user sees which existing nodes overlap and can decide whether to approve or reject.

## Neo4j Data Model

### Node Types

```
(:ScrapeJob {
  job_id, status, urls, target_space_id, options,
  total_urls, processed_urls, created_at, updated_at, error
})

(:ScrapedContent {
  content_id, job_id, url, title, content, content_preview,
  content_hash, quality_score, similar_existing, suggested_tags,
  summary, status, word_count, ingested_node_id
})

(:MemoryNode {
  node_id, space_id, content, role_type, obs_type,
  tags, pinned, metadata, ...
})
```

### Relationships

```
(ScrapedContent)-[:BELONGS_TO]->(ScrapeJob)
(ScrapedContent)-[:INGESTED_AS]->(MemoryNode)
```

The `[:INGESTED_AS]` edge is created only when content is approved during review. It provides traceability from scraped source to memory node.

## Ingestion Pipeline (What Happens on Approve)

When a review decision approves content, the reviewer runs the **UPTS-backed scraper parser** (`internal/scraper/parser.go`) to chunk and extract symbols before ingestion:

### Section Chunking

The parser checks word count against a threshold (default: 2000 words):

| Page Size | Behavior | Sections Created | Embedding Quality |
|-----------|----------|-----------------|-------------------|
| < 2000 words | Pass-through (no chunking) | 1 | Same as before |
| 2000-5000 words | Split at `##` headings | 2-5 | Focused per section |
| 5000+ words | Split at `##` headings, merge tiny sections | 5-20 | Highly focused |

Sections smaller than 100 words are merged with the next section to avoid fragmentation.

### Symbol Extraction

Each section's symbols are extracted via `MarkdownParser.ExtractSymbols()` — the same UPTS-validated code path used by the codebase ingestion pipeline (scraper-markdown parser #27). Extracted symbol types:

- **Headings** — with parent hierarchy tracking (h1 > h2 > h3)
- **Code blocks** — with language detection (go, python, bash, etc.)
- **Links** — standalone links with URL values

Symbols are stored in the `structured_data` field of the resulting MemoryNode.

### Per-Section Observe

For each section, the reviewer calls `conversation.Observe()` via the adapter with:

- `obs_type`: `"learning"`
- `pinned`: `true` (prevents decay/pruning)
- `tags`: base tags + `source:web-scraper` + `url:<source_url>` + `lang:<language>` + `domain:<host>` + `topic:<heading_term>`
- `metadata`: source_url, quality_score, scrape_job_id, content_hash, word_count, title, and (when chunked) section_index, section_title, total_sections
- `structured_data`: `{"symbols": [...]}` — extracted headings, code blocks, and links
- `session_id`: `"web-scraper"` (fixed)
- `content`: `# <page_title> > <section_title>\n\n<section_markdown>` (context prefix for embedding quality)

### Post-Ingestion

1. Creates `[:INGESTED_AS]` relationship from ScrapedContent to the **first section's** MemoryNode
2. Updates content status to `ingested`
3. When no `pending_review` items remain, marks the job `completed`

Ingested content becomes regular MemoryNodes — searchable via recall, participating in consolidation, learning edges, and all other MDEMG graph operations. Chunked pages produce multiple focused nodes with tighter embeddings, improving retrieval precision for large documentation pages.

## Internal Components

### Core Server (`internal/scraper/`)

| File | Purpose |
|------|---------|
| `types.go` | All domain types, status constants, extraction profiles |
| `service.go` | Service struct, config, dependency wiring |
| `store.go` | Neo4j CRUD for ScrapeJob and ScrapedContent nodes |
| `orchestrator.go` | Async job pipeline, BFS crawl logic, plugin bridge |
| `dedup.go` | Vector similarity checker (threshold 0.85) |
| `reviewer.go` | Review workflow, section chunking, per-section Observe calls |
| `parser.go` | Section chunking + symbol extraction via UPTS-validated MarkdownParser |
| `parser_test.go` | 15 unit tests for parser (chunking, symbols, tags, merging, helpers) |
| `summarizer.go` | Placeholder for LLM-based summaries (not yet active) |

### API Layer (`internal/api/`)

| File | Purpose |
|------|---------|
| `handlers_scraper.go` | 6 REST endpoint handlers |
| `scraper_adapters.go` | `scraperConvAdapter` — decouples scraper from concrete conversation.Service; passes StructuredData for symbols |

### Plugin (`plugins/docs-scraper/`)

| File | Purpose |
|------|---------|
| `manifest.json` | Plugin metadata, capabilities, and config |
| `main.go` | Entry point — Unix socket listener, gRPC server, signal handling |
| `lifecycle.go` | ModuleLifecycle gRPC service (handshake, health, shutdown) |
| `ingestion.go` | IngestionModule gRPC service (Matches, Parse, Sync) |
| `fetcher.go` | HTTP client with auth, timeouts, robots.txt caching |
| `extractor.go` | HTML → Markdown extraction with profile-based filtering |
| `quality.go` | Rule-based quality scoring (0.0–1.0) |
| `tagger.go` | Auto-tag suggestion (domain, URL patterns, code languages) |

## Building and Testing

```bash
# Build the server
go build -o mdemg-server ./cmd/server/

# Build the plugin (required for scraper to work)
go build -o plugins/docs-scraper/docs-scraper ./plugins/docs-scraper/

# Run plugin unit tests (11 tests — extraction, quality, tagging)
go test ./plugins/docs-scraper/... -v

# Run scraper parser unit tests (15 tests — chunking, symbols, tags, merging)
go test ./internal/scraper/... -v

# Run UPTS validation for scraper-markdown parser
go test ./cmd/ingest-codebase/languages/ -run TestUPTS/scraper-markdown -v

# Run full UATS contract test suite
make test-api BASE_URL=http://localhost:9999

# Static analysis
go vet ./...
```

## Limitations and Known Constraints

- **No headless browser** — The scraper performs plain HTTP GET requests. JavaScript-rendered SPAs will return minimal content and few discoverable links. Server-rendered and static HTML sites work best.
- **Regex-based HTML parsing** — The extractor uses regex patterns rather than a full DOM parser (no goquery dependency in the main module). This handles standard HTML well but may miss edge cases with malformed or deeply nested markup.
- **Single-domain crawling only** — BFS link-following is restricted to the seed URL's domain(s). Cross-domain links are ignored.
- **No incremental/scheduled scraping** — Each job is a one-time operation. There is no built-in scheduler for periodic re-scraping.
- **No JS execution for content** — Content behind client-side rendering, login walls with JS-based auth flows, or CAPTCHA protection will not be accessible.

## Roadmap: Scheduled Advanced Functionality

This implementation represents the **base scraper module**. The following enhancements are planned for future phases:

- **LLM-powered summarization** — Use the configured LLM to generate 2-3 sentence summaries for each scraped page (scaffolding exists in `summarizer.go`)
- **Scheduled/recurring scrape jobs** — Cron-style scheduling to re-scrape documentation sites periodically and detect changes
- **Topic-based URL discovery** — Given a topic (e.g., "Go concurrency patterns"), automatically find relevant URLs to scrape
- **Cross-domain crawling** — Opt-in mode to follow links beyond the seed domain with configurable allow/deny lists
- **Headless browser support** — Integration with a headless browser for JS-rendered SPA content
- **Incremental change detection** — Use content hashes to skip pages that haven't changed since last scrape
- **Batch review UI** — A dedicated interface for reviewing large scrape jobs with bulk approve/reject
- **LLM-based quality filtering** — Use an LLM to assess content relevance rather than purely heuristic scoring
- **Webhook notifications** — Notify external systems when scrape jobs complete or require review
- **PDF and non-HTML ingestion** — Extend the plugin to handle PDF, plain text, and other document formats
