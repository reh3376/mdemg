# MDEMG Ingestion Guide

## Overview

MDEMG (Multi-Dimensional Emergent Memory Graph) stores knowledge as graph nodes in Neo4j with vector embeddings for semantic search. Ingestion is the process of getting data into this graph.

**Data flow:**
```
Source data --> Parser/Extractor --> Code elements or observations --> MDEMG API --> Neo4j nodes with embeddings
```

Every ingested item becomes a node in the graph with:
- **Content**: the raw text or structured data
- **Embedding**: a vector representation for semantic search (auto-generated if not provided)
- **Tags**: metadata labels for filtering
- **Space ID**: logical namespace for isolation (e.g., `codebase`, `web-scraper`, `linear-issues`)
- **Symbols**: extracted code symbols (functions, classes, constants) for evidence-locked retrieval
- **Path**: source file path for traceability

MDEMG supports 8 ingestion methods, ranging from full codebase walks to real-time file watchers and webhook-driven pipelines.

---

## Method 1: Codebase Ingestion (CLI)

The primary ingestion method. Walks a directory tree, parses source files using language-specific parsers, extracts code elements and symbols, optionally generates LLM summaries, and batch-ingests everything into MDEMG.

### Basic Usage

```bash
# Ingest a codebase (requires running MDEMG server)
mdemg ingest --path /path/to/your/project --space-id myproject

# Ingest the current directory
mdemg ingest --path . --space-id myproject

# With explicit endpoint
mdemg ingest --path ./src --space-id myproject --endpoint http://localhost:9999

# Dry run (preview without ingesting)
mdemg ingest --path ./src --space-id myproject --dry-run

# Verbose output
mdemg ingest --path ./src --space-id myproject --verbose
```

#### PowerShell

```powershell
mdemg ingest --path C:\Users\me\projects\myproject --space-id myproject
mdemg ingest --path .\src --space-id myproject --endpoint http://localhost:9999
mdemg ingest --path .\src --space-id myproject --dry-run
```

### Language Support (27 Languages)

MDEMG includes dedicated parsers for 27 languages and file formats. Each parser extracts structured code elements (functions, classes, structs, modules) and optionally detailed symbols.

List all supported languages:
```bash
mdemg ingest --list-languages
```

**Supported languages and their parsers:**

| Language | Extensions | Parser |
|----------|-----------|--------|
| Go | `.go` | go_parser.go |
| Python | `.py` | python_parser.go |
| TypeScript/JavaScript | `.ts`, `.tsx`, `.js`, `.jsx` | typescript_parser.go |
| Java | `.java` | java_parser.go |
| Rust | `.rs` | rust_parser.go |
| C | `.c`, `.h` | c_parser.go |
| C++ | `.cpp`, `.cxx`, `.cc`, `.hpp` | cpp_parser.go |
| C# | `.cs` | csharp_parser.go |
| CUDA | `.cu`, `.cuh` | cuda_parser.go |
| Kotlin | `.kt`, `.kts` | kotlin_parser.go |
| Lua | `.lua` | lua_parser.go |
| Shell/Bash | `.sh`, `.bash` | shell_parser.go |
| SQL | `.sql` | sql_parser.go |
| Cypher | `.cypher` | cypher_parser.go |
| GraphQL | `.graphql`, `.gql` | graphql_parser.go |
| Protobuf | `.proto` | protobuf_parser.go |
| Terraform | `.tf` | terraform_parser.go |
| Dockerfile | `Dockerfile` | dockerfile_parser.go |
| Makefile | `Makefile` | makefile_parser.go |
| Markdown | `.md` | markdown_parser.go |
| JSON | `.json` | json_parser.go |
| YAML | `.yaml`, `.yml` | yaml_parser.go |
| TOML | `.toml` | toml_parser.go |
| XML | `.xml` | xml_parser.go |
| INI | `.ini`, `.cfg` | ini_parser.go |
| OpenAPI | `openapi.yaml`, `swagger.json` | openapi_parser.go |

Each parser implements the `LanguageParser` interface:
- `CanParse(path)` - determines if the parser handles a given file
- `ParseFile(root, path, extractSymbols)` - extracts code elements
- `IsTestFile(path)` - identifies test files for optional exclusion

Advanced parsers (like CUDA and C/C++) also implement `ContextAwareParser`, which can use build context (compiler flags, include paths, defines) from `CMakeLists.txt`, `Makefile`, etc.

### Symbol Extraction

Symbol extraction creates fine-grained searchable entries for individual code constructs within files. Enabled by default.

```bash
# With symbol extraction (default)
mdemg ingest --path ./src --space-id myproject --extract-symbols=true

# Without symbol extraction (faster, coarser granularity)
mdemg ingest --path ./src --space-id myproject --extract-symbols=false

# Limit symbols per file
mdemg ingest --path ./src --space-id myproject --max-symbols-per-file=500
```

**Symbol types extracted:** `constant`, `function`, `class`, `interface`, `variable`, `struct`, `enum`, `method`, `macro`, `kernel`, `trait`, `field`

Each symbol includes: name, type, line number, exported/private flag, parent scope, signature, value, doc comment, and type annotation.

### LLM Summaries

LLM-generated semantic summaries enhance retrieval quality by providing natural-language descriptions of code elements. Enabled by default.

```bash
# With LLM summaries (default, requires OPENAI_API_KEY)
mdemg ingest --path ./src --space-id myproject --llm-summary

# Using OpenAI (default)
mdemg ingest --path ./src --space-id myproject \
  --llm-summary-provider=openai \
  --llm-summary-model=gpt-4o-mini \
  --llm-summary-batch=10

# Using Ollama (local, no API key needed)
mdemg ingest --path ./src --space-id myproject \
  --llm-summary-provider=ollama \
  --llm-summary-model=llama3

# Disable LLM summaries (faster, structural-only summaries)
mdemg ingest --path ./src --space-id myproject --llm-summary=false
```

**Environment variables:**

```bash
export OPENAI_API_KEY="sk-..."                              # Required for OpenAI provider
export OPENAI_ENDPOINT="https://api.openai.com/v1"          # Custom OpenAI endpoint (default shown)
export OLLAMA_ENDPOINT="http://localhost:11434"              # Ollama server (default shown)
```

Alternatively, set these in a `.env` file in your project root (works the same on all platforms).

#### PowerShell

```powershell
$env:OPENAI_API_KEY = "sk-..."
$env:OPENAI_ENDPOINT = "https://api.openai.com/v1"
$env:OLLAMA_ENDPOINT = "http://localhost:11434"
```

If the API key is not set when using the OpenAI provider, the CLI falls back to structural summaries extracted from docstrings and comments.

### Incremental Mode

Only ingest files that changed since a specific git commit. Dramatically faster for iterative development.

```bash
# Ingest only files changed since the last commit
mdemg ingest --path ./src --space-id myproject --incremental

# Ingest changes since a specific commit
mdemg ingest --path ./src --space-id myproject --incremental --since HEAD~5

# Ingest changes and archive nodes for deleted files
mdemg ingest --path ./src --space-id myproject --incremental --archive-deleted

# Disable archiving of deleted file nodes
mdemg ingest --path ./src --space-id myproject --incremental --archive-deleted=false
```

Incremental mode uses `git diff --name-status` to detect:
- **Added** files: ingested as new nodes
- **Modified** files: re-ingested (updated nodes)
- **Deleted** files: archived (soft-deleted) if `--archive-deleted` is set
- **Renamed** files: treated as modified (new path ingested)

### Presets

Exclusion presets provide pre-configured directory and pattern exclusions for common project types.

```bash
# Default preset
mdemg ingest --path ./src --space-id myproject --preset default

# ML/CUDA project (excludes model checkpoints, datasets, wandb logs)
mdemg ingest --path ./src --space-id myproject --preset ml_cuda

# Web monorepo (excludes .next, coverage, storybook-static, chunk files)
mdemg ingest --path ./src --space-id myproject --preset web_monorepo
```

**Preset details:**

| Preset | Excluded Dirs | Excluded Patterns | Max File Size |
|--------|---------------|-------------------|---------------|
| `default` | `.git`, `node_modules`, `vendor`, `__pycache__`, `.venv`, `venv`, `build`, `dist`, `target` | `*.min.js`, `*.bundle.js`, `*.pyc` | 1 MB |
| `ml_cuda` | All default + `third_party`, `data`, `datasets`, `checkpoints`, `logs`, `wandb`, `outputs`, `.cache` | All default + `*.pt`, `*.pth`, `*.onnx`, `*.bin`, `*.safetensors`, `*.npy`, `*.npz` | 512 KB |
| `web_monorepo` | All default + `.next`, `.nuxt`, `.output`, `coverage`, `storybook-static` | All default + `*.chunk.js`, `*.map` | 1 MB |

### Performance Tuning

```bash
# High-throughput ingestion (large codebase)
mdemg ingest --path ./src --space-id myproject \
  --batch=200 \
  --workers=8 \
  --delay=25 \
  --timeout=600

# Conservative ingestion (shared server)
mdemg ingest --path ./src --space-id myproject \
  --batch=50 \
  --workers=2 \
  --delay=100
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--batch` | 100 | Elements per API batch (optimal: ~100 for 15/s per worker) |
| `--workers` | 4 | Parallel worker goroutines |
| `--delay` | 50 | Milliseconds between batches (backpressure) |
| `--timeout` | 300 | HTTP timeout in seconds |
| `--retries` | 3 | Max retries per batch on failure |
| `--retry-delay` | 2000 | Initial retry delay in ms (doubles each retry) |
| `--max-file-size` | 1048576 | Max file size in bytes (default: 1 MB) |
| `--max-elements-per-file` | 500 | Max elements to extract per file |
| `--max-symbols-per-file` | 1000 | Max symbols to extract per file |
| `--limit` | 0 | Limit total elements ingested (0 = no limit) |

### Language Filtering

```bash
# Include/exclude specific languages
mdemg ingest --path ./src --space-id myproject \
  --include-ts=true \
  --include-py=true \
  --include-java=false \
  --include-rust=false \
  --include-md=true

# Include test files (excluded by default)
mdemg ingest --path ./src --space-id myproject --include-tests

# Custom directory exclusions
mdemg ingest --path ./src --space-id myproject --exclude=".git,vendor,node_modules,generated,proto"
```

### Output Options

```bash
# Quiet mode (suppress all non-error output)
mdemg ingest --path ./src --space-id myproject --quiet

# Log to file
mdemg ingest --path ./src --space-id myproject --log-file=/tmp/ingest.log

# JSON progress events on stdout (logs to stderr)
mdemg ingest --path ./src --space-id myproject --progress-json
```

#### Windows

```powershell
mdemg ingest --path .\src --space-id myproject --log-file=C:\temp\ingest.log
```

JSON progress events emitted:
- `discovery_complete` - files scanned, total element count
- `batch_progress` - current count, rate, errors
- `consolidation_start` - consolidation phase beginning
- `complete` - total, ingested, errors, duration

### Post-Ingest Consolidation

By default, consolidation runs automatically after ingestion. Consolidation builds the hidden layer of the graph -- creating higher-level concept nodes, detecting cross-cutting concerns, building dynamic edges, and running the emergence pipeline.

```bash
# With consolidation (default)
mdemg ingest --path ./src --space-id myproject --consolidate

# Without consolidation (faster, skip post-processing)
mdemg ingest --path ./src --space-id myproject --consolidate=false
```

### .mdemgignore Support

If a `.mdemgignore` file exists in the project root, its patterns are loaded and applied during ingestion. Syntax is similar to `.gitignore`.

---

## Method 2: Codebase Ingestion (API)

Two API endpoints support codebase ingestion. The newer one (`/v1/memory/ingest/trigger`) is preferred.

### Trigger Endpoint (Preferred)

```bash
curl -s -X POST http://localhost:9999/v1/memory/ingest/trigger \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "myproject",
    "path": "/path/to/codebase",
    "batch_size": 100,
    "workers": 4,
    "extract_symbols": true,
    "consolidate": true,
    "include_tests": false,
    "incremental": false,
    "dry_run": false
  }'
```

#### PowerShell

```powershell
$body = @{
    space_id        = "myproject"
    path            = "C:\Users\me\projects\myproject"
    batch_size      = 100
    workers         = 4
    extract_symbols = $true
    consolidate     = $true
    include_tests   = $false
    incremental     = $false
    dry_run         = $false
} | ConvertTo-Json

Invoke-RestMethod -Method Post -Uri "http://localhost:9999/v1/memory/ingest/trigger" `
  -ContentType "application/json" -Body $body
```

Response (HTTP 202 Accepted):
```json
{
  "job_id": "ingest-a1b2c3d4",
  "space_id": "myproject",
  "status": "pending"
}
```

### Check Job Status

```bash
curl -s http://localhost:9999/v1/memory/ingest/status/ingest-a1b2c3d4
```

#### PowerShell

```powershell
Invoke-RestMethod -Uri "http://localhost:9999/v1/memory/ingest/status/ingest-a1b2c3d4"
```

### List All Jobs

```bash
curl -s http://localhost:9999/v1/memory/ingest/jobs
```

### Cancel a Job

```bash
curl -s -X POST http://localhost:9999/v1/memory/ingest/cancel/ingest-a1b2c3d4
```

### Legacy Endpoint (Deprecated)

The `/v1/memory/ingest-codebase` endpoint still works but returns deprecation headers. It supports a richer request structure:

```bash
curl -s -X POST http://localhost:9999/v1/memory/ingest-codebase \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "myproject",
    "path": "/path/to/codebase",
    "source": {
      "type": "local",
      "since": "HEAD~3"
    },
    "languages": {
      "typescript": true,
      "python": true,
      "go": true,
      "markdown": true,
      "include_tests": false
    },
    "symbols": {
      "extract": true,
      "max_per_file": 1000
    },
    "exclusions": {
      "preset": "web_monorepo",
      "directories": ["generated", "proto"],
      "max_file_size": 524288
    },
    "processing": {
      "batch_size": 100,
      "workers": 4,
      "max_elements_per_file": 500,
      "delay_ms": 50
    },
    "llm_summary": {
      "enabled": true,
      "provider": "openai",
      "model": "gpt-4o-mini",
      "batch_size": 10
    },
    "options": {
      "incremental": true,
      "archive_deleted": true,
      "consolidate": true,
      "verbose": false,
      "limit": 0
    },
    "retry": {
      "max_attempts": 3,
      "initial_delay_ms": 2000,
      "timeout_seconds": 300
    }
  }'
```

#### PowerShell

```powershell
$body = @{
    space_id = "myproject"
    path     = "C:\Users\me\projects\myproject"
    source   = @{
        type  = "local"
        since = "HEAD~3"
    }
    languages = @{
        typescript    = $true
        python        = $true
        go            = $true
        markdown      = $true
        include_tests = $false
    }
    symbols = @{
        extract      = $true
        max_per_file = 1000
    }
    exclusions = @{
        preset        = "web_monorepo"
        directories   = @("generated", "proto")
        max_file_size = 524288
    }
    processing = @{
        batch_size            = 100
        workers               = 4
        max_elements_per_file = 500
        delay_ms              = 50
    }
    llm_summary = @{
        enabled    = $true
        provider   = "openai"
        model      = "gpt-4o-mini"
        batch_size = 10
    }
    options = @{
        incremental     = $true
        archive_deleted = $true
        consolidate     = $true
        verbose         = $false
        limit           = 0
    }
    retry = @{
        max_attempts     = 3
        initial_delay_ms = 2000
        timeout_seconds  = 300
    }
} | ConvertTo-Json -Depth 4

Invoke-RestMethod -Method Post -Uri "http://localhost:9999/v1/memory/ingest-codebase" `
  -ContentType "application/json" -Body $body
```

### File-Level Ingestion API

Ingest specific files (used by the file watcher):

```bash
curl -s -X POST http://localhost:9999/v1/memory/ingest/files \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "myproject",
    "files": [
      "/path/to/file1.go",
      "/path/to/file2.ts"
    ],
    "extract_symbols": true,
    "consolidate": false
  }'
```

#### PowerShell

```powershell
$body = @{
    space_id        = "myproject"
    files           = @("C:\Users\me\projects\file1.go", "C:\Users\me\projects\file2.ts")
    extract_symbols = $true
    consolidate     = $false
} | ConvertTo-Json

Invoke-RestMethod -Method Post -Uri "http://localhost:9999/v1/memory/ingest/files" `
  -ContentType "application/json" -Body $body
```

For 50 or fewer files, processing is synchronous. For more than 50 files, a background job is created and a `job_id` is returned.

---

## Method 3: Web Scraping

Scrape web pages and ingest their content into MDEMG. Useful for ingesting documentation sites, wiki pages, or any web content.

**Prerequisites:** Enable the scraper in your environment:

```bash
export SCRAPER_ENABLED=true
```

Or add to your `.env` file:
```
SCRAPER_ENABLED=true
```

#### PowerShell

```powershell
$env:SCRAPER_ENABLED = "true"
```

### Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `SCRAPER_ENABLED` | `false` | Enable the scraper module |
| `SCRAPER_DEFAULT_SPACE_ID` | `web-scraper` | Default target space |
| `SCRAPER_MAX_CONCURRENT_JOBS` | `3` | Max concurrent scrape jobs |
| `SCRAPER_DEFAULT_DELAY_MS` | `1000` | Delay between requests (ms) |
| `SCRAPER_DEFAULT_TIMEOUT_MS` | `30000` | HTTP timeout per page (ms) |
| `SCRAPER_CACHE_TTL_SECONDS` | `3600` | robots.txt cache TTL |
| `SCRAPER_RESPECT_ROBOTS_TXT` | `true` | Respect robots.txt |
| `SCRAPER_MAX_CONTENT_LENGTH_KB` | `500` | Max content length (KB) |

### Creating a Scrape Job

```bash
curl -s -X POST http://localhost:9999/v1/scraper/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "urls": [
      "https://docs.example.com/getting-started",
      "https://docs.example.com/api-reference"
    ],
    "target_space_id": "example-docs",
    "options": {
      "extraction_profile": "documentation",
      "max_depth": 2,
      "max_pages": 50,
      "follow_links": true,
      "delay_ms": 1000,
      "timeout_ms": 30000,
      "auth": {
        "type": "none"
      }
    }
  }'
```

#### PowerShell

```powershell
$body = @{
    urls            = @(
        "https://docs.example.com/getting-started"
        "https://docs.example.com/api-reference"
    )
    target_space_id = "example-docs"
    options         = @{
        extraction_profile = "documentation"
        max_depth          = 2
        max_pages          = 50
        follow_links       = $true
        delay_ms           = 1000
        timeout_ms         = 30000
        auth               = @{ type = "none" }
    }
} | ConvertTo-Json -Depth 3

Invoke-RestMethod -Method Post -Uri "http://localhost:9999/v1/scraper/jobs" `
  -ContentType "application/json" -Body $body
```

Response:
```json
{
  "job_id": "scrape-a1b2c3d4",
  "status": "pending",
  "urls": ["https://docs.example.com/getting-started", "..."],
  "target_space_id": "example-docs",
  "total_urls": 2,
  "processed_urls": 0
}
```

### Checking Job Status

```bash
# Get job details and scraped content
curl -s http://localhost:9999/v1/scraper/jobs/scrape-a1b2c3d4

# List all scrape jobs
curl -s http://localhost:9999/v1/scraper/jobs

# Cancel a running job
curl -s -X DELETE http://localhost:9999/v1/scraper/jobs/scrape-a1b2c3d4
```

### Review and Approve

Scraped content goes through a review step before being committed to the graph. This lets you approve, reject, or edit individual content items.

```bash
curl -s -X POST http://localhost:9999/v1/scraper/jobs/scrape-a1b2c3d4/review \
  -H "Content-Type: application/json" \
  -d '{
    "decisions": [
      {
        "content_id": "content-001",
        "action": "approve"
      },
      {
        "content_id": "content-002",
        "action": "reject"
      },
      {
        "content_id": "content-003",
        "action": "edit",
        "edit_content": "Corrected and cleaned up content here...",
        "edit_tags": ["api", "reference", "v2"],
        "space_id": "custom-space"
      }
    ]
  }'
```

#### PowerShell

```powershell
$body = @{
    decisions = @(
        @{
            content_id = "content-001"
            action     = "approve"
        },
        @{
            content_id = "content-002"
            action     = "reject"
        },
        @{
            content_id   = "content-003"
            action       = "edit"
            edit_content = "Corrected and cleaned up content here..."
            edit_tags    = @("api", "reference", "v2")
            space_id     = "custom-space"
        }
    )
} | ConvertTo-Json -Depth 3

Invoke-RestMethod -Method Post `
  -Uri "http://localhost:9999/v1/scraper/jobs/scrape-a1b2c3d4/review" `
  -ContentType "application/json" -Body $body
```

**Review actions:**
- `approve` - ingest the content as-is into the target space
- `reject` - discard the content
- `edit` - modify the content and/or tags before ingesting; optionally redirect to a different space

### Extraction Profiles

| Profile | Description |
|---------|-------------|
| `documentation` | (Default) Optimized for docs sites. Extracts structured sections, headings, code blocks. |
| `generic` | General-purpose extraction. Raw content with basic cleanup. |

### List Available Spaces

```bash
curl -s http://localhost:9999/v1/scraper/spaces
```

**Authentication options** for scraping protected pages:
- `"type": "none"` - no auth (default)
- `"type": "cookie"` - send cookies via `credentials` map
- `"type": "header"` - custom headers via `credentials` map
- `"type": "basic"` - HTTP Basic Auth via `credentials.username` and `credentials.password`

---

## Method 4: Linear Integration

Integrate with Linear (project management tool) to ingest issues, projects, and comments into MDEMG.

### Setup

Linear integration uses the MDEMG plugin system. The `linear-module` is a gRPC-based plugin that provides both CRUD operations and ingestion capabilities.

**Environment variables:**

```bash
export LINEAR_API_KEY=lin_api_xxxxx        # Your Linear API key
export LINEAR_TEAM_ID=TEAM-ID              # Default team ID for issue creation
export LINEAR_WEBHOOK_SECRET=your-secret   # HMAC-SHA256 signing secret for webhooks
export LINEAR_WEBHOOK_SPACE_ID=linear-dev  # Target space for webhook observations
```

Or add to your `.env` file:
```
LINEAR_API_KEY=lin_api_xxxxx
LINEAR_TEAM_ID=TEAM-ID
LINEAR_WEBHOOK_SECRET=your-secret
LINEAR_WEBHOOK_SPACE_ID=linear-dev
```

#### PowerShell

```powershell
$env:LINEAR_API_KEY = "lin_api_xxxxx"
$env:LINEAR_TEAM_ID = "TEAM-ID"
$env:LINEAR_WEBHOOK_SECRET = "your-secret"
$env:LINEAR_WEBHOOK_SPACE_ID = "linear-dev"
```

### Manual Sync

Trigger a sync to pull issues from Linear into MDEMG:

```bash
curl -s -X POST http://localhost:9999/v1/modules/linear-module/sync \
  -H "Content-Type: application/json" \
  -d '{
    "source_id": "linear-module://issues",
    "ingest": true,
    "space_id": "linear-issues",
    "config": {
      "team": "MY-TEAM"
    }
  }'
```

#### PowerShell

```powershell
$body = @{
    source_id = "linear-module://issues"
    ingest    = $true
    space_id  = "linear-issues"
    config    = @{ team = "MY-TEAM" }
} | ConvertTo-Json -Depth 2

Invoke-RestMethod -Method Post -Uri "http://localhost:9999/v1/modules/linear-module/sync" `
  -ContentType "application/json" -Body $body
```

Without `"ingest": true`, the sync returns observations but does not store them.

### CRUD Operations

MDEMG provides a REST API layer on top of Linear:

```bash
# List issues
curl -s "http://localhost:9999/v1/linear/issues?team=MY-TEAM&state=started&limit=20"

# Get a specific issue
curl -s http://localhost:9999/v1/linear/issues/ISSUE-123

# Create an issue
curl -s -X POST http://localhost:9999/v1/linear/issues \
  -H "Content-Type: application/json" \
  -d '{"title": "Fix authentication bug", "team_id": "TEAM-ID"}'

# Update an issue
curl -s -X PUT http://localhost:9999/v1/linear/issues/ISSUE-123 \
  -H "Content-Type: application/json" \
  -d '{"state": "done"}'

# Add a comment
curl -s -X POST http://localhost:9999/v1/linear/comments \
  -H "Content-Type: application/json" \
  -d '{"issue_id": "ISSUE-123", "body": "Fixed in commit abc123"}'

# List projects
curl -s "http://localhost:9999/v1/linear/projects?limit=10"
```

#### PowerShell

```powershell
# List issues
Invoke-RestMethod -Uri "http://localhost:9999/v1/linear/issues?team=MY-TEAM&state=started&limit=20"

# Get a specific issue
Invoke-RestMethod -Uri "http://localhost:9999/v1/linear/issues/ISSUE-123"

# Create an issue
$body = @{ title = "Fix authentication bug"; team_id = "TEAM-ID" } | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri "http://localhost:9999/v1/linear/issues" `
  -ContentType "application/json" -Body $body

# Update an issue
$body = @{ state = "done" } | ConvertTo-Json
Invoke-RestMethod -Method Put -Uri "http://localhost:9999/v1/linear/issues/ISSUE-123" `
  -ContentType "application/json" -Body $body

# Add a comment
$body = @{ issue_id = "ISSUE-123"; body = "Fixed in commit abc123" } | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri "http://localhost:9999/v1/linear/comments" `
  -ContentType "application/json" -Body $body

# List projects
Invoke-RestMethod -Uri "http://localhost:9999/v1/linear/projects?limit=10"
```

### Real-Time Webhooks

Configure a Linear webhook to push events to MDEMG in real-time:

1. In Linear Settings > API > Webhooks, create a webhook:
   - **URL**: `https://your-mdemg-server.com/v1/webhooks/linear`
   - **Secret**: the value of `LINEAR_WEBHOOK_SECRET`

2. MDEMG processes these event types:
   - **Issue create/update** - parsed and ingested via the `linear-module`
   - **Project update** - parsed and ingested

3. Events are debounced (10-second quiet window per entity) to coalesce rapid field updates.

4. Signature verification uses HMAC-SHA256 (via the `Linear-Signature` header).

---

## Method 5: Git Webhooks (GitHub, GitLab, Bitbucket)

MDEMG provides a generic webhook handler that normalizes payloads from GitHub, GitLab, and Bitbucket and ingests them as observations.

### Configuration

Set the `WEBHOOK_CONFIGS` environment variable:

```bash
# Format: source:secret:space_id (comma-separated for multiple)
export WEBHOOK_CONFIGS="github:ghp_webhook_secret_here:github-events,gitlab:glpat_token_here:gitlab-events,bitbucket:bb_secret:bb-events"
```

Or add to your `.env` file:
```
WEBHOOK_CONFIGS=github:ghp_webhook_secret_here:github-events,gitlab:glpat_token_here:gitlab-events,bitbucket:bb_secret:bb-events
```

#### PowerShell

```powershell
$env:WEBHOOK_CONFIGS = "github:ghp_webhook_secret_here:github-events,gitlab:glpat_token_here:gitlab-events,bitbucket:bb_secret:bb-events"
```

Each entry is `source:secret:space_id`:
- **source**: `github`, `gitlab`, or `bitbucket`
- **secret**: the webhook signing secret
- **space_id**: the MDEMG space to store observations

### Setting Up Webhooks

**GitHub:**
1. Go to repo Settings > Webhooks > Add webhook
2. Payload URL: `https://your-mdemg-server.com/v1/webhooks/github`
3. Content type: `application/json`
4. Secret: the secret from `WEBHOOK_CONFIGS`
5. Events: Push, Pull Request, Issues, etc.

Signature header: `X-Hub-Signature-256` (HMAC-SHA256)

**GitLab:**
1. Go to project Settings > Webhooks
2. URL: `https://your-mdemg-server.com/v1/webhooks/gitlab`
3. Secret token: the secret from `WEBHOOK_CONFIGS`
4. Trigger: Push, Merge Request, Issue, etc.

Signature header: `X-Gitlab-Token` (simple token comparison)

**Bitbucket:**
1. Go to repo Settings > Webhooks > Add webhook
2. URL: `https://your-mdemg-server.com/v1/webhooks/bitbucket`
3. Secret: the secret from `WEBHOOK_CONFIGS`
4. Triggers: Push, Pull Request, etc.

Signature header: `X-Hub-Signature` (HMAC-SHA256)

### Supported Event Types

| Source | Event | Entity Type | Metadata |
|--------|-------|-------------|----------|
| GitHub | `push` | `push` | `commit_sha`, `repository` |
| GitHub | `pull_request` | `pull_request` | `repository` |
| GitHub | `issues` | `issue` | `repository` |
| GitLab | `push` | `push` | `commit_sha`, `repository` |
| GitLab | `merge_request` | `merge_request` | `repository` |
| GitLab | `issue` | `issue` | `repository` |
| Bitbucket | `repo:push` | `push` | `commit_sha`, `repository` |
| Bitbucket | `pullrequest:*` | `pull_request` | `repository` |

### How It Works

1. Webhook payload arrives at `/v1/webhooks/{source}`
2. Signature is verified against the configured secret
3. Payload is normalized into a standard structure (source, action, entity type, entity ID, URL, metadata)
4. Event is debounced (10-second quiet window per entity) to coalesce rapid updates
5. If an ingestion module is configured (`module_id` in config), the payload is parsed by the module
6. Otherwise, the raw payload is stored as a single observation
7. Observations are batch-ingested into the configured space
8. TapRoot freshness is updated and APE events are triggered

### Custom Webhook Sources

Any source can send webhooks. If the source is not `github`, `gitlab`, or `bitbucket`, MDEMG attempts generic JSON parsing with HMAC-SHA256 signature verification. Configure it in `WEBHOOK_CONFIGS` just like the others.

---

## Method 6: File Watcher (Real-Time)

Monitor a directory for file changes and automatically ingest modified files. Two modes: CLI (standalone process) and API (server-managed).

> **Platform note:** `mdemg watch` works on all platforms with the same flags. The underlying file system watcher uses platform-native APIs (FSEvents on macOS, inotify on Linux, ReadDirectoryChangesW on Windows).

### CLI Watcher

A standalone process that monitors a directory and sends changed files to the MDEMG API:

```bash
# Watch current directory
mdemg watch --space-id myproject

# Watch a specific directory
mdemg watch --space-id myproject --path ./src

# Custom settings
mdemg watch --space-id myproject \
  --path ./src \
  --extensions ".go,.py,.ts,.tsx,.js,.jsx,.rs,.md" \
  --exclude "node_modules,.git,vendor,dist,build" \
  --debounce 1000 \
  --endpoint http://localhost:9999
```

#### Windows

```powershell
mdemg watch --space-id myproject --path C:\Users\me\projects\myproject\src

mdemg watch --space-id myproject `
  --path .\src `
  --extensions ".go,.py,.ts,.tsx,.js,.jsx,.rs,.md" `
  --exclude "node_modules,.git,vendor,dist,build" `
  --debounce 1000 `
  --endpoint http://localhost:9999
```

**Default watched extensions:** `.go`, `.py`, `.ts`, `.tsx`, `.js`, `.jsx`, `.rs`, `.java`, `.md`, `.yaml`, `.yml`, `.json`, `.toml`, `.sql`

**Default excluded directories:** `node_modules`, `.git`, `vendor`, `__pycache__`, `.venv`, `dist`, `build`, `.next`

**How it works:**
1. Recursively watches all subdirectories (excluding excluded dirs)
2. Filters events by file extension
3. Debounces changes (default 500ms quiet window) to batch rapid saves
4. Sends changed files to `POST /v1/memory/ingest/files`
5. Retries on server errors (3 attempts with exponential backoff)
6. Automatically watches newly created directories
7. Handles graceful shutdown (`SIGINT`/`SIGTERM` on macOS/Linux, `Ctrl+C` on Windows)

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--space-id` | (required) | Target MDEMG space |
| `--path` | `.` | Directory to watch |
| `--endpoint` | auto-resolved | MDEMG API endpoint |
| `--extensions` | 14 common types | Comma-separated file extensions |
| `--exclude` | 7 common dirs | Comma-separated directories to exclude |
| `--debounce` | 500 | Debounce window in milliseconds |

### API Watcher

Start and manage file watchers through the MDEMG API (in-process, no separate CLI needed):

```bash
# Start watching a directory
curl -s -X POST http://localhost:9999/v1/filewatcher/start \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "myproject",
    "path": "/path/to/src",
    "extensions": [".go", ".py", ".ts", ".tsx"],
    "excludes": ["node_modules", ".git", "vendor"],
    "debounce_ms": 500
  }'
```

#### PowerShell

```powershell
$body = @{
    space_id    = "myproject"
    path        = "C:\Users\me\projects\src"
    extensions  = @(".go", ".py", ".ts", ".tsx")
    excludes    = @("node_modules", ".git", "vendor")
    debounce_ms = 500
} | ConvertTo-Json

Invoke-RestMethod -Method Post -Uri "http://localhost:9999/v1/filewatcher/start" `
  -ContentType "application/json" -Body $body
```

Response:
```json
{
  "space_id": "myproject",
  "path": "/absolute/path/to/src",
  "status": "watching"
}
```

```bash
# Check watcher status
curl -s http://localhost:9999/v1/filewatcher/status
```

Response:
```json
{
  "watchers": [
    {"space_id": "myproject", "path": "/path/to/src", "status": "watching"}
  ],
  "count": 1
}
```

```bash
# Stop watching
curl -s -X POST http://localhost:9999/v1/filewatcher/stop \
  -H "Content-Type: application/json" \
  -d '{"space_id": "myproject"}'
```

#### PowerShell

```powershell
# Check watcher status
Invoke-RestMethod -Uri "http://localhost:9999/v1/filewatcher/status"

# Stop watching
$body = @{ space_id = "myproject" } | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri "http://localhost:9999/v1/filewatcher/stop" `
  -ContentType "application/json" -Body $body
```

**Default extensions** (API watcher): `.go`, `.py`, `.ts`, `.tsx`, `.js`, `.jsx`, `.rs`, `.java`, `.md`, `.yaml`, `.yml`, `.json`, `.toml`, `.sql`

**Default excludes** (API watcher): `node_modules`, `.git`, `vendor`, `__pycache__`, `.venv`, `dist`, `build`, `.next`, `.turbo`, `coverage`

---

## Method 7: Direct Observation API

The lowest-level ingestion method. Ingest individual observations or batches directly via the API. Used by all other methods under the hood.

### Single Observation

```bash
curl -s -X POST http://localhost:9999/v1/memory/ingest \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "myproject",
    "timestamp": "2026-03-10T12:00:00Z",
    "source": "manual",
    "content": "The authentication service uses JWT tokens with a 24-hour expiry. Refresh tokens are stored in Redis with a 30-day TTL.",
    "tags": ["auth", "jwt", "redis"],
    "name": "Auth Token Strategy",
    "path": "architecture/auth",
    "summary": "JWT auth with Redis refresh tokens",
    "sensitivity": "internal",
    "confidence": 0.95
  }'
```

#### PowerShell

```powershell
$body = @{
    space_id    = "myproject"
    timestamp   = "2026-03-10T12:00:00Z"
    source      = "manual"
    content     = "The authentication service uses JWT tokens with a 24-hour expiry. Refresh tokens are stored in Redis with a 30-day TTL."
    tags        = @("auth", "jwt", "redis")
    name        = "Auth Token Strategy"
    path        = "architecture/auth"
    summary     = "JWT auth with Redis refresh tokens"
    sensitivity = "internal"
    confidence  = 0.95
} | ConvertTo-Json

Invoke-RestMethod -Method Post -Uri "http://localhost:9999/v1/memory/ingest" `
  -ContentType "application/json" -Body $body
```

Response:
```json
{
  "space_id": "myproject",
  "node_id": "obs-a1b2c3d4",
  "obs_id": "obs-a1b2c3d4",
  "embedding_dims": 1536,
  "anomalies": []
}
```

**Request fields:**

| Field | Required | Description |
|-------|----------|-------------|
| `space_id` | Yes | Target space (1-256 chars) |
| `timestamp` | Yes | ISO8601 timestamp (or unix/unix_ms/date_only with `timestamp_format`) |
| `source` | Yes | Source identifier (1-64 chars) |
| `content` | Yes | Content (string or structured object) |
| `tags` | No | Array of tag strings |
| `node_id` | No | Explicit node ID (for updates) |
| `path` | No | Source path (max 512 chars) |
| `name` | No | Human-readable name |
| `summary` | No | Brief summary for reranking (max 1000 chars) |
| `sensitivity` | No | `public`, `internal`, or `confidential` |
| `confidence` | No | Confidence score 0.0-1.0 |
| `embedding` | No | Pre-computed embedding vector (auto-generated if omitted) |
| `canonical_time` | No | Content-relevant time (for temporal queries) |
| `timestamp_format` | No | `rfc3339` (default), `unix`, `unix_ms`, `date_only` |

### Batch Ingest

Ingest multiple observations in a single request (up to 2000 per batch):

```bash
curl -s -X POST http://localhost:9999/v1/memory/ingest/batch \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "myproject",
    "observations": [
      {
        "timestamp": "2026-03-10T12:00:00Z",
        "source": "meeting-notes",
        "content": "Decision: migrate from PostgreSQL to CockroachDB for multi-region support",
        "tags": ["decision", "database", "architecture"],
        "name": "DB Migration Decision"
      },
      {
        "timestamp": "2026-03-10T12:30:00Z",
        "source": "meeting-notes",
        "content": "Action item: benchmark CockroachDB write latency vs PostgreSQL",
        "tags": ["action", "benchmark", "database"],
        "name": "DB Benchmark Task"
      }
    ]
  }'
```

#### PowerShell

```powershell
$body = @{
    space_id     = "myproject"
    observations = @(
        @{
            timestamp = "2026-03-10T12:00:00Z"
            source    = "meeting-notes"
            content   = "Decision: migrate from PostgreSQL to CockroachDB for multi-region support"
            tags      = @("decision", "database", "architecture")
            name      = "DB Migration Decision"
        },
        @{
            timestamp = "2026-03-10T12:30:00Z"
            source    = "meeting-notes"
            content   = "Action item: benchmark CockroachDB write latency vs PostgreSQL"
            tags      = @("action", "benchmark", "database")
            name      = "DB Benchmark Task"
        }
    )
} | ConvertTo-Json -Depth 3

Invoke-RestMethod -Method Post -Uri "http://localhost:9999/v1/memory/ingest/batch" `
  -ContentType "application/json" -Body $body
```

Response:
```json
{
  "space_id": "myproject",
  "total_items": 2,
  "success_count": 2,
  "error_count": 0,
  "errors": []
}
```

**Batch items** support the same fields as single observations, plus:
- `symbols` - array of extracted code symbols (used by codebase ingestion)

**Tips:**
- Embeddings are auto-generated for each observation if not provided
- Set `node_id` to update an existing node instead of creating a new one
- Use `canonical_time` for content that refers to a specific point in time (e.g., "deployed v2.0 on Jan 15")
- The batch size limit is configurable via `BATCH_INGEST_MAX_ITEMS` (default: 2000)

---

## Method 8: Git Hooks (Auto-Ingest on Commit)

Automatically run incremental ingestion after every git commit. The post-commit hook runs in the background and does not block your workflow.

> **Platform note:** `mdemg hooks install` works the same on all platforms. Git uses forward slashes internally for hook paths within `.git/hooks/`, so the hook script uses forward slashes regardless of OS. On Windows, the hook script is a shell script executed by Git's bundled bash (Git for Windows includes bash).

### Installing Hooks

```bash
# Navigate to your git repository
cd /path/to/my/repo

# Install with default space ID (uses directory name)
mdemg hooks install

# Install with custom space ID
mdemg hooks install --space-id myproject

# Force overwrite existing hook
mdemg hooks install --force
```

### What the Hook Does

The installed post-commit hook:
1. Finds the `mdemg` binary (in `$PATH` or `$REPO_ROOT/bin/mdemg`)
2. Runs incremental ingestion in the background (with `nohup` on macOS/Linux):
   ```bash
   mdemg ingest \
     --path "$REPO_ROOT" \
     --space-id "$SPACE_ID" \
     --incremental \
     --since "HEAD~1" \
     --archive-deleted \
     --quiet
   ```
3. Only ingests files changed in the latest commit
4. Archives nodes for deleted files
5. Runs silently (output suppressed)

### Disabling Temporarily

```bash
# Disable the hook temporarily
export MDEMG_DISABLED=true
git commit -m "This commit won't trigger ingestion"
unset MDEMG_DISABLED
```

Or override the space ID:
```bash
export MDEMG_SPACE_ID=different-space
```

#### PowerShell

```powershell
$env:MDEMG_DISABLED = "true"
git commit -m "This commit won't trigger ingestion"
Remove-Item Env:\MDEMG_DISABLED
```

Or override the space ID:
```powershell
$env:MDEMG_SPACE_ID = "different-space"
```

### Managing Hooks

```bash
# Check hook status
mdemg hooks list
```

Output:
```
MDEMG Hook Status
=================
  post-commit hook:  installed (mdemg)
  hook script:       present (scripts/mdemg-git-hook)
```

```bash
# Uninstall (only removes MDEMG-installed hooks)
mdemg hooks uninstall
```

The uninstall command only removes hooks with the `# MDEMG` marker comment. Non-MDEMG hooks are never touched.

### Hook Types

| Flag | Description |
|------|-------------|
| `--type git` | (Default) Install git post-commit hook for incremental ingestion |
| `--type claude` | Install Claude Code hooks (CMS recall, Jiminy guidance, session memory) |
| `--type all` | Install all available hook types |

---

## Quick Reference

| Method | Trigger | Best For | Latency |
|--------|---------|----------|---------|
| CLI Ingest | Manual | Initial codebase ingestion, bulk updates | Minutes |
| API Ingest | HTTP POST | CI/CD pipelines, external tools | Minutes |
| Web Scraper | HTTP POST | Documentation sites, wiki pages | Seconds-minutes |
| Linear | Webhook/Manual | Project management data | Real-time/Manual |
| Git Webhooks | Webhook | CI events, PR/issue tracking | Real-time |
| File Watcher | File change | Active development, live reload | Sub-second |
| Direct API | HTTP POST | Custom integrations, scripts, AI agents | Milliseconds |
| Git Hooks | Post-commit | Incremental updates on every commit | Seconds (background) |

## Common Patterns

### Initial Setup

```bash
# 1. Start MDEMG
mdemg start --auto-migrate

# 2. Full codebase ingestion
mdemg ingest --path /path/to/project --space-id myproject

# 3. Install git hooks for ongoing updates
cd /path/to/project && mdemg hooks install --space-id myproject

# 4. (Optional) Start a file watcher for real-time updates
mdemg watch --space-id myproject --path /path/to/project/src
```

#### Windows

```powershell
# 1. Start MDEMG
cd C:\Users\me\mdemg
.\bin\mdemg.exe start --auto-migrate

# 2. Full codebase ingestion
mdemg ingest --path C:\Users\me\projects\myproject --space-id myproject

# 3. Install git hooks for ongoing updates
cd C:\Users\me\projects\myproject
mdemg hooks install --space-id myproject

# 4. (Optional) Start a file watcher for real-time updates
mdemg watch --space-id myproject --path C:\Users\me\projects\myproject\src
```

### CI/CD Integration

```bash
# In your CI pipeline after tests pass
curl -s -X POST http://mdemg-server:9999/v1/memory/ingest/trigger \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "myproject",
    "path": "/workspace",
    "incremental": true,
    "since_commit": "'$CI_COMMIT_BEFORE_SHA'",
    "consolidate": true
  }'
```

#### PowerShell (GitHub Actions / Azure DevOps)

```powershell
$body = @{
    space_id     = "myproject"
    path         = $env:GITHUB_WORKSPACE   # or $env:BUILD_SOURCESDIRECTORY for Azure DevOps
    incremental  = $true
    since_commit = $env:GITHUB_SHA
    consolidate  = $true
} | ConvertTo-Json

Invoke-RestMethod -Method Post -Uri "http://mdemg-server:9999/v1/memory/ingest/trigger" `
  -ContentType "application/json" -Body $body
```

### Multi-Source Knowledge Base

```bash
# Ingest code
mdemg ingest --path ./src --space-id myproject-code

# Ingest documentation site
curl -s -X POST http://localhost:9999/v1/scraper/jobs \
  -H "Content-Type: application/json" \
  -d '{"urls": ["https://docs.myproject.com"], "target_space_id": "myproject-docs", "options": {"follow_links": true, "max_depth": 3}}'

# Ingest from Linear
curl -s -X POST http://localhost:9999/v1/modules/linear-module/sync \
  -H "Content-Type: application/json" \
  -d '{"ingest": true, "space_id": "myproject-linear"}'

# Configure webhooks for real-time updates
export WEBHOOK_CONFIGS="github:secret:myproject-github"
```

#### PowerShell

```powershell
# Ingest code
mdemg ingest --path .\src --space-id myproject-code

# Ingest documentation site
$body = @{
    urls            = @("https://docs.myproject.com")
    target_space_id = "myproject-docs"
    options         = @{ follow_links = $true; max_depth = 3 }
} | ConvertTo-Json -Depth 2
Invoke-RestMethod -Method Post -Uri "http://localhost:9999/v1/scraper/jobs" `
  -ContentType "application/json" -Body $body

# Ingest from Linear
$body = @{ ingest = $true; space_id = "myproject-linear" } | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri "http://localhost:9999/v1/modules/linear-module/sync" `
  -ContentType "application/json" -Body $body

# Configure webhooks for real-time updates
$env:WEBHOOK_CONFIGS = "github:secret:myproject-github"
```
