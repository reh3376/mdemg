# Web Scraper Ingestion Module (Phase 51)

## Architecture

The web scraper is split between a **plugin** (handles fetching + extraction) and the **core server** (handles API, jobs, persistence, review, ingestion).

```
┌─────────────────────────────────┐     ┌─────────────────────────────────┐
│  MDEMG Core Server              │     │  docs-scraper Plugin            │
│                                 │     │                                 │
│  API: /v1/scraper/*             │     │  gRPC: ModuleLifecycle +        │
│  Job orchestration              │◄───►│         IngestionModule         │
│  Neo4j persistence              │gRPC │  HTTP fetching + robots.txt     │
│  Dedup (vector similarity)      │Unix │  HTML → Markdown extraction     │
│  Review workflow                │sock │  Quality scoring + tagging      │
│  Ingestion (→ conversation)     │     │                                 │
└─────────────────────────────────┘     └─────────────────────────────────┘
```

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SCRAPER_ENABLED` | `false` | Enable web scraper module |
| `SCRAPER_DEFAULT_SPACE_ID` | `web-scraper` | Default target space for scraped content |
| `SCRAPER_MAX_CONCURRENT_JOBS` | `3` | Max concurrent scrape jobs |
| `SCRAPER_DEFAULT_DELAY_MS` | `1000` | Delay between requests (ms) |
| `SCRAPER_DEFAULT_TIMEOUT_MS` | `30000` | HTTP timeout per request (ms) |
| `SCRAPER_CACHE_TTL_SECONDS` | `3600` | robots.txt cache TTL (seconds) |
| `SCRAPER_RESPECT_ROBOTS_TXT` | `true` | Respect robots.txt |
| `SCRAPER_MAX_CONTENT_LENGTH_KB` | `500` | Max content length (KB) |

### Plugin Config (manifest.json)

Located at `plugins/docs-scraper/manifest.json`. Config values passed to plugin via handshake.

## API Endpoints

### Create Job

```bash
curl -X POST http://localhost:9999/v1/scraper/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "urls": ["https://docs.example.com/getting-started"],
    "target_space_id": "my-docs",
    "options": {
      "extraction_profile": "documentation",
      "delay_ms": 2000
    }
  }'
```

Returns `202 Accepted` with job ID.

### List Jobs

```bash
curl http://localhost:9999/v1/scraper/jobs
```

### Get Job Status

```bash
curl http://localhost:9999/v1/scraper/jobs/{job_id}
```

Returns job details including scraped content items.

### Cancel Job

```bash
curl -X DELETE http://localhost:9999/v1/scraper/jobs/{job_id}
```

### Review Content

```bash
curl -X POST http://localhost:9999/v1/scraper/jobs/{job_id}/review \
  -H "Content-Type: application/json" \
  -d '{
    "decisions": [
      {"content_id": "abc-123", "action": "approve"},
      {"content_id": "def-456", "action": "reject"},
      {"content_id": "ghi-789", "action": "edit", "edit_content": "Updated text", "edit_tags": ["new-tag"]}
    ]
  }'
```

### List Spaces

```bash
curl http://localhost:9999/v1/scraper/spaces
```

## Extraction Profiles

### `documentation` (default)

Removes nav, footer, header, sidebar, breadcrumbs, cookie banners. Focuses on `<main>`, `<article>`, or `.content` areas. Best for technical documentation sites.

### `generic`

Minimal filtering. Removes only `<script>` and `<style>` tags. Extracts full `<body>` content.

## Job Lifecycle

1. **Create**: POST /v1/scraper/jobs → status: `pending`
2. **Running**: Plugin fetches each URL → status: `running`
3. **Awaiting Review**: All URLs processed → status: `awaiting_review`
4. **Review**: User approves/rejects/edits each item
5. **Completed**: All items reviewed → status: `completed`

Approved items are ingested as pinned conversation observations via `conversation.Observe()`.

### Section Chunking (Phase 51b)

When a page is approved, the UPTS-backed scraper parser (`internal/scraper/parser.go`) runs on the content:

| Page Size | Behavior | Sections Created |
|-----------|----------|-----------------|
| < 2000 words | Pass-through (no chunking) | 1 |
| 2000-5000 words | Split at `##` headings | 2-5 |
| 5000+ words | Split at `##` headings, merge tiny sections | 5-20 |

Each section gets its own `conversation.Observe()` call, producing focused MemoryNodes with tighter embeddings. Symbol extraction delegates to the UPTS-validated `MarkdownParser.ExtractSymbols()`, and symbols are stored in `structured_data`.

Section metadata (`section_index`, `section_title`, `total_sections`) is attached to each node. The `INGESTED_AS` edge points to the first section's node.

## Building the Plugin

```bash
go build -o plugins/docs-scraper/docs-scraper ./plugins/docs-scraper/
```

Run tests:

```bash
go test ./plugins/docs-scraper/... -v
```

## Neo4j Node Types

- **ScrapeJob**: Tracks job metadata and status
- **ScrapedContent**: Individual scraped pages, linked to job via `[:BELONGS_TO]`
- **MemoryNode**: Ingested content, linked from ScrapedContent via `[:INGESTED_AS]`

## Dedup

Before showing content for review, the system embeds the scraped text and queries the vector index for similar existing MemoryNodes (threshold: 0.85). Similar node IDs are attached to the ScrapedContent for user awareness.

## Key Files

### Core

- `internal/scraper/types.go` — Domain types and constants
- `internal/scraper/service.go` — Service skeleton
- `internal/scraper/store.go` — Neo4j persistence
- `internal/scraper/orchestrator.go` — Job pipeline
- `internal/scraper/dedup.go` — Vector similarity dedup
- `internal/scraper/reviewer.go` — Review workflow + ingestion (with section chunking)
- `internal/scraper/parser.go` — Section chunking + symbol extraction via UPTS markdown parser
- `internal/scraper/parser_test.go` — Parser unit tests (15 tests)
- `internal/api/handlers_scraper.go` — REST handlers
- `internal/api/scraper_adapters.go` — Conversation service adapter (passes StructuredData)

### Plugin

- `plugins/docs-scraper/main.go` — Entry point
- `plugins/docs-scraper/lifecycle.go` — gRPC lifecycle
- `plugins/docs-scraper/ingestion.go` — Matches/Parse/Sync
- `plugins/docs-scraper/fetcher.go` — HTTP client
- `plugins/docs-scraper/extractor.go` — HTML → Markdown
- `plugins/docs-scraper/quality.go` — Quality scoring
- `plugins/docs-scraper/tagger.go` — Auto-tagging
