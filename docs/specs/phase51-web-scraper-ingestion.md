# Feature Spec: Web Scraper Ingestion Module

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Phase**: Phase 51
**Status**: Approved
**Author**: reh3376 & Claude (gMEM-dev)
**Date**: 2026-02-06
**Priority**: Medium

---

## Overview

Add a web scraping ingestion module that allows users to discover and ingest web content into MDEMG. Users can define topics for discovery or provide specific URLs. The module runs asynchronously, presents scraped content for review, and supports authenticated scraping for internal documentation.

**Data flow:**

```
User Request (topic/URLs)
  → Async Job Created
    → Web Scraper Module (crawl4ai or similar)
      → Content Extraction + Quality Scoring
        → Deduplication Check
          → User Review UI (edit/approve/reject)
            → Standardized Ingestion
              → MDEMG Graph
```

## Requirements

### Functional Requirements

| ID | Requirement |
|----|-------------|
| FR-1 | Topic-based discovery: User provides topic, system discovers relevant URLs |
| FR-2 | URL-based scraping: User provides specific URL(s) for scraping |
| FR-3 | Asynchronous operation: Jobs run in background with status polling |
| FR-4 | Authenticated scraping: Support cookies, headers, API keys for internal docs |
| FR-5 | Content extraction profiles: `documentation`, `forum`, `blog`, `news`, `generic` |
| FR-6 | Quality scoring: Score content (0-1) for relevance and informativeness |
| FR-7 | Deduplication: Check for similar existing content before ingestion |
| FR-8 | Review workflow: Present content for user review/edit before ingestion |
| FR-9 | Space selection: Default to `web-scraper` space, allow selection of any existing space |
| FR-10 | LLM summary generation: Auto-generate summary for scraped content |
| FR-11 | Auto-tag suggestions: Suggest tags based on content analysis |
| FR-12 | Link following: Optionally follow relevant links up to N hops |
| FR-13 | Rate limiting: Respect robots.txt, configurable delays |
| FR-14 | Batch processing: Queue multiple URLs for sequential processing |

### Non-Functional Requirements

| ID | Requirement |
|----|-------------|
| NFR-1 | Politeness: Default 1s delay between requests, respect robots.txt |
| NFR-2 | Timeout: 30s default per page, configurable |
| NFR-3 | Caching: Cache raw content during review session (1 hour TTL) |
| NFR-4 | Concurrency: Max 3 concurrent scrape jobs per user |

## API Contract

### REST Endpoints

```
POST   /v1/scraper/jobs              → Create scrape job
GET    /v1/scraper/jobs              → List scrape jobs
GET    /v1/scraper/jobs/{job_id}     → Get job status and results
DELETE /v1/scraper/jobs/{job_id}     → Cancel job
POST   /v1/scraper/jobs/{job_id}/review  → Submit review decisions
GET    /v1/scraper/spaces            → List available space_ids for ingestion
```

### Create Scrape Job Request

```json
{
  "mode": "topic|urls",
  "topic": "string — required if mode=topic",
  "urls": ["string — required if mode=urls"],
  "options": {
    "extraction_profile": "documentation|forum|blog|news|generic",
    "max_depth": 0,
    "max_pages": 10,
    "follow_links": false,
    "auth": {
      "type": "none|cookie|header|basic",
      "cookies": {"name": "value"},
      "headers": {"Authorization": "Bearer ..."},
      "basic": {"username": "...", "password": "..."}
    },
    "delay_ms": 1000,
    "timeout_ms": 30000
  },
  "target_space_id": "web-scraper"
}
```

### Job Status Response

```json
{
  "job_id": "uuid",
  "status": "pending|running|awaiting_review|completed|cancelled|failed",
  "created_at": "ISO8601",
  "updated_at": "ISO8601",
  "progress": {
    "total_urls": 5,
    "scraped": 3,
    "failed": 0,
    "pending": 2
  },
  "results": [
    {
      "url": "https://...",
      "title": "Page Title",
      "content_preview": "First 500 chars...",
      "quality_score": 0.85,
      "similar_existing": ["node_id_1"],
      "suggested_tags": ["tag1", "tag2"],
      "summary": "LLM-generated summary",
      "status": "pending_review|approved|rejected|ingested"
    }
  ],
  "target_space_id": "web-scraper",
  "available_spaces": ["web-scraper", "mdemg-dev", "whk-wms"]
}
```

### Review Submission Request

```json
{
  "decisions": [
    {
      "url": "https://...",
      "action": "approve|reject|edit",
      "edited_content": "optional edited content",
      "edited_tags": ["optional", "edited", "tags"],
      "target_space_id": "optional override"
    }
  ]
}
```

### Error Codes

| Code | Meaning |
|------|---------|
| 400 | Invalid request (missing topic/urls, invalid auth) |
| 404 | Job not found |
| 409 | Job already completed or cancelled |
| 429 | Too many concurrent jobs |
| 503 | Scraper module not available |

## Data Model

### Neo4j Schema Additions

```cypher
// Scrape job tracking
CREATE (j:ScrapeJob {
  job_id: "uuid",
  user_id: "string",
  mode: "topic|urls",
  topic: "string",
  status: "string",
  target_space_id: "string",
  options: "json string",
  created_at: datetime(),
  updated_at: datetime()
})

// Scraped content (pending review)
CREATE (c:ScrapedContent {
  content_id: "uuid",
  job_id: "uuid",
  url: "string",
  title: "string",
  content: "string",
  content_hash: "sha256",
  quality_score: 0.85,
  summary: "string",
  suggested_tags: ["array"],
  status: "pending_review|approved|rejected|ingested",
  scraped_at: datetime()
})

// Relationship to job
(c:ScrapedContent)-[:BELONGS_TO]->(j:ScrapeJob)

// Relationship to ingested node (after approval)
(c:ScrapedContent)-[:INGESTED_AS]->(n:MemoryNode)
```

### Go Types

```go
type ScrapeJobRequest struct {
    Mode         string            `json:"mode"` // "topic" or "urls"
    Topic        string            `json:"topic,omitempty"`
    URLs         []string          `json:"urls,omitempty"`
    Options      ScrapeOptions     `json:"options,omitempty"`
    TargetSpaceID string           `json:"target_space_id,omitempty"`
}

type ScrapeOptions struct {
    ExtractionProfile string       `json:"extraction_profile,omitempty"`
    MaxDepth          int          `json:"max_depth,omitempty"`
    MaxPages          int          `json:"max_pages,omitempty"`
    FollowLinks       bool         `json:"follow_links,omitempty"`
    Auth              *ScrapeAuth  `json:"auth,omitempty"`
    DelayMs           int          `json:"delay_ms,omitempty"`
    TimeoutMs         int          `json:"timeout_ms,omitempty"`
}

type ScrapeAuth struct {
    Type     string            `json:"type"` // none|cookie|header|basic
    Cookies  map[string]string `json:"cookies,omitempty"`
    Headers  map[string]string `json:"headers,omitempty"`
    Basic    *BasicAuth        `json:"basic,omitempty"`
}

type ScrapedContent struct {
    ContentID       string   `json:"content_id"`
    JobID           string   `json:"job_id"`
    URL             string   `json:"url"`
    Title           string   `json:"title"`
    Content         string   `json:"content"`
    ContentPreview  string   `json:"content_preview"`
    QualityScore    float64  `json:"quality_score"`
    SimilarExisting []string `json:"similar_existing,omitempty"`
    SuggestedTags   []string `json:"suggested_tags,omitempty"`
    Summary         string   `json:"summary,omitempty"`
    Status          string   `json:"status"`
}
```

## Implementation Plan

### New Files

| File | Purpose |
|------|---------|
| `internal/scraper/service.go` | Core scraping service |
| `internal/scraper/extractor.go` | Content extraction profiles |
| `internal/scraper/quality.go` | Quality scoring |
| `internal/scraper/dedup.go` | Deduplication check |
| `internal/scraper/discovery.go` | Topic-based URL discovery |
| `internal/api/handlers_scraper.go` | REST handlers |
| `internal/api/handlers_scraper_test.go` | Handler tests |
| `plugins/scraper-module/` | Optional: Binary sidecar for scraping |

### Modified Files

| File | Changes |
|------|---------|
| `internal/api/server.go` | Register scraper routes |
| `internal/jobs/tracker.go` | Add scrape job type |
| `internal/config/config.go` | Add scraper config vars |
| `.env.example` | Document new vars |

### Configuration

```bash
# Scraper Configuration
SCRAPER_ENABLED=true
SCRAPER_DEFAULT_SPACE_ID=web-scraper
SCRAPER_MAX_CONCURRENT_JOBS=3
SCRAPER_DEFAULT_DELAY_MS=1000
SCRAPER_DEFAULT_TIMEOUT_MS=30000
SCRAPER_CACHE_TTL_SECONDS=3600
SCRAPER_RESPECT_ROBOTS_TXT=true

# Topic Discovery (optional)
SCRAPER_SEARCH_PROVIDER=google|bing|duckduckgo
SCRAPER_SEARCH_API_KEY=optional
```

## Test Plan

### Unit Tests

- [ ] Extraction profile tests (each profile extracts correctly)
- [ ] Quality scoring tests (various content types)
- [ ] Deduplication tests (similarity detection)
- [ ] Auth handling tests (cookies, headers, basic)

### Integration Tests

- [ ] Full job lifecycle: create → scrape → review → ingest
- [ ] Concurrent job limiting
- [ ] Space selection validation

### UxTS Specs Required

- [ ] UATS: `scraper_create_job.uats.json`
- [ ] UATS: `scraper_get_status.uats.json`
- [ ] UATS: `scraper_review.uats.json`
- [ ] UPTS: `web_content_parser.upts.json` (extraction profiles)

## Acceptance Criteria

- [ ] AC-1: `go build ./...` compiles clean
- [ ] AC-2: `go test ./...` all tests pass
- [ ] AC-3: Topic-based discovery returns relevant URLs
- [ ] AC-4: URL scraping extracts content correctly
- [ ] AC-5: Authenticated scraping works for internal docs
- [ ] AC-6: Review workflow allows edit/approve/reject
- [ ] AC-7: Approved content ingests to selected space
- [ ] AC-8: Quality scores correlate with content value
- [ ] AC-9: Deduplication prevents redundant ingestion

## Dependencies

- **Depends on**: Phase 42 (Self-Ingest), existing ingestion pipeline
- **Optional**: crawl4ai or similar Python scraping library via sidecar

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Rate limiting by sites | Configurable delays; respect robots.txt |
| Legal/TOS concerns | User-triggered only; no automated crawling |
| Low quality content | Quality scoring + user review gate |
| Large content volumes | Pagination; token limits |

---

*Created: 2026-02-06*
