# Ingest Performance Tuning Guide

This guide covers how to optimize `mdemg ingest` for different codebases and workflows using speed presets, individual flags, and `.mdemgignore` patterns.

---

## Speed Presets

Speed presets control parallelism, batch sizing, and feature toggles. They do not affect which files are included or excluded -- that is handled separately by exclusion presets (`--preset`) and `.mdemgignore`.

| Setting | `fast` | `balanced` | `thorough` |
|---------|--------|------------|------------|
| Workers | 8 | 4 | 8 |
| Batch size | 250 | 100 | 200 |
| LLM summaries | off | on | on |
| LLM batch | 0 | 10 | 20 |
| Symbol extraction | off | on | on |
| Consolidation | off | on | on |
| Delay (ms) | 0 | 50 | 25 |

When no `--speed` flag is set and no `INGEST_SPEED` environment variable is present, `mdemg ingest` uses its built-in defaults: 4 workers, batch size 100, LLM summaries on, LLM batch 10, symbols on, consolidation on, 50ms delay. This matches the `balanced` preset.

---

## When to Use Each Preset

### fast

First-time ingest of large codebases (50K+ files), CI/CD pipelines, quick re-indexing. Skips LLM summaries, symbol extraction, and consolidation -- raw graph structure only. Uses maximum parallelism (8 workers) with large batches (250) and zero inter-batch delay for peak throughput.

Re-run with `balanced` or `thorough` later to add enrichment (summaries, symbols, hidden layer concepts).

### balanced

Default behavior. Good for regular development workflows and medium codebases (1K-50K files). Full enrichment (LLM summaries, symbol extraction, consolidation) with moderate resource use. The 50ms inter-batch delay avoids overwhelming the MDEMG server or embedding provider.

### thorough

Pre-benchmark runs, small codebases, and situations where retrieval quality matters most. Higher parallelism (8 workers) combined with full enrichment and larger LLM batches (20 files per API call). The 25ms delay balances throughput against API rate limits.

---

## Usage

### CLI Flag

```bash
mdemg ingest --path . --speed fast
mdemg ingest --path . --speed balanced
mdemg ingest --path . --speed thorough
```

### YAML Config

```yaml
# .mdemg/config.yaml
ingest:
  speed: balanced
```

### Environment Variable

```bash
export INGEST_SPEED=fast
mdemg ingest --path .
```

---

## Precedence Rules

Configuration sources are applied in this order (highest wins):

```
Individual CLI flags  >  --speed preset  >  INGEST_SPEED env var  >  config.yaml  >  built-in defaults
```

Individual CLI flags always override preset values. The `--speed` flag applies preset values only for flags that were not explicitly set on the command line.

**Example:** `--speed fast --llm-summary=true` uses all `fast` settings (8 workers, batch 250, 0 delay, no symbols, no consolidation) but keeps LLM summaries enabled because `--llm-summary` was explicitly set.

**Example:** `INGEST_SPEED=thorough` in `.mdemg/config.yaml` with `--speed fast` on the CLI uses `fast` -- the CLI flag wins over the config file.

---

## Combining Speed and Exclusion Presets

Speed presets (`--speed`) and exclusion presets (`--preset`) are orthogonal:

- `--speed` controls **how** files are processed: workers, batch size, LLM, symbols, consolidation, delay
- `--preset` controls **which** files are processed: directory exclusions, pattern exclusions, max file size

Available exclusion presets:

| Preset | Extra Excluded Dirs | Extra Excluded Patterns | Max File Size |
|--------|--------------------|-----------------------|---------------|
| `default` | `.git`, `node_modules`, `vendor`, `__pycache__`, `.venv`, `venv`, `build`, `dist`, `target` | `*.min.js`, `*.bundle.js`, `*.pyc` | 1 MB |
| `ml_cuda` | + `third_party`, `data`, `datasets`, `checkpoints`, `logs`, `wandb`, `outputs`, `.cache` | + `*.pt`, `*.pth`, `*.onnx`, `*.bin`, `*.safetensors`, `*.npy`, `*.npz` | 512 KB |
| `web_monorepo` | + `.next`, `.nuxt`, `.output`, `coverage`, `storybook-static` | + `*.chunk.js`, `*.map` | 1 MB |

Combine them freely:

```bash
# Maximum speed, ML/CUDA exclusions
mdemg ingest --path . --speed fast --preset ml_cuda

# Thorough enrichment, web monorepo exclusions
mdemg ingest --path . --speed thorough --preset web_monorepo
```

---

## Advanced Tuning

Each flag below can be set independently, with or without a speed preset. When used alongside `--speed`, explicitly set flags take priority over the preset value.

### Parallelism and Batching

| Flag | Default | Description |
|------|---------|-------------|
| `--workers` | 4 | Number of parallel HTTP workers sending batches to the MDEMG server. Higher values increase throughput but also increase server and embedding provider load. |
| `--batch` | 100 | Number of code elements per API call. Larger batches reduce HTTP overhead but increase memory usage and per-request latency. |
| `--delay` | 50 | Milliseconds to wait between batches. Set to 0 for maximum throughput. Increase if you see rate-limit errors from the embedding provider. |
| `--timeout` | 300 | HTTP timeout per request in seconds. Increase for very large batches or slow embedding providers. |
| `--retries` | 3 | Maximum retry attempts per failed batch. |
| `--retry-delay` | 2000 | Initial retry delay in milliseconds. Doubles on each subsequent retry (exponential backoff). |

### Enrichment

| Flag | Default | Description |
|------|---------|-------------|
| `--llm-summary` | true | Enable LLM-generated semantic summaries for each file. Requires `OPENAI_API_KEY` or a configured Ollama endpoint. Summaries improve retrieval quality significantly but add latency and cost. |
| `--llm-summary-batch` | 10 | Number of files per LLM API call. Higher values reduce API call count but increase per-call latency and token usage. |
| `--llm-summary-model` | `gpt-4o-mini` | Model to use for generating summaries. |
| `--llm-summary-provider` | `openai` | LLM provider for summaries (`openai` or `ollama`). |
| `--extract-symbols` | true | Run AST parsing to extract functions, classes, constants, and other symbols. Symbols enable evidence-locked retrieval (citations point to exact code locations). |
| `--consolidate` | true | Run hidden layer consolidation after ingestion completes. Creates higher-level concept nodes from the raw observations. Adds time but improves retrieval for conceptual queries. |

### File Filtering

| Flag | Default | Description |
|------|---------|-------------|
| `--exclude` | `.git,vendor,node_modules,.worktrees` | Comma-separated directory names to skip during the walk. |
| `--preset` | (none) | Exclusion preset: `default`, `ml_cuda`, `web_monorepo`. Overrides `--exclude` and `--max-file-size` with curated values. |
| `--max-file-size` | 1048576 (1 MB) | Maximum file size in bytes. Files larger than this are skipped. |
| `--max-elements-per-file` | 500 | Cap on code elements extracted from a single file. |
| `--max-symbols-per-file` | 1000 | Cap on symbols extracted from a single file. |
| `--include-tests` | false | Include test files (`*_test.go`, `*.test.ts`, `*.spec.ts`). Off by default to reduce noise. |
| `--include-md` | true | Include markdown files. |
| `--include-ts` | true | Include TypeScript/JavaScript files. |
| `--include-py` | true | Include Python files. |
| `--include-java` | true | Include Java files. |
| `--include-rust` | true | Include Rust files. |
| `--limit` | 0 | Limit total number of elements to ingest (0 = no limit). Useful for testing. |

### Incremental Ingestion

| Flag | Default | Description |
|------|---------|-------------|
| `--incremental` | false | Only ingest files changed since a given commit (uses `git diff`). |
| `--since` | `HEAD~1` | Git commit to compare against for incremental mode. |
| `--archive-deleted` | true | Archive nodes for files deleted between the baseline commit and HEAD. |

### Output Control

| Flag | Default | Description |
|------|---------|-------------|
| `--dry-run` | false | Print what would be ingested without sending any data. Use this to verify element counts and file selection before committing to a full run. |
| `--verbose` | false | Print detailed per-file processing information. |
| `--quiet` | false | Suppress all non-error output. |
| `--log-file` | (none) | Write logs to a file instead of stderr. |
| `--progress-json` | false | Emit structured JSON progress lines to stdout (logs still go to stderr). Useful for programmatic monitoring. |

---

## Recommended Workflows

### First-time large codebase (50K+ files)

1. Create a `.mdemgignore` file first (see below). This is the single highest-impact optimization.
2. Preview with dry run:
   ```bash
   mdemg ingest --path . --speed fast --dry-run
   ```
   Check the element count. If it exceeds 100K elements, tighten your `.mdemgignore`.
3. Ingest structure only:
   ```bash
   mdemg ingest --path . --speed fast
   ```
4. Add enrichment:
   ```bash
   mdemg ingest --path . --speed thorough
   ```
   The second pass re-ingests with LLM summaries, symbols, and consolidation.

### Regular development

Use `balanced` (or omit `--speed` entirely, since the defaults match `balanced`):

```bash
mdemg ingest --path .
```

For incremental updates after a few commits:

```bash
mdemg ingest --path . --incremental --since HEAD~5
```

### Pre-benchmark

Use `thorough` to maximize retrieval quality:

```bash
mdemg ingest --path . --speed thorough
```

### CI/CD pipeline

Use `fast` with `--quiet` and `--progress-json` for machine-readable output:

```bash
mdemg ingest --path . --speed fast --quiet --progress-json
```

---

## .mdemgignore

`.mdemgignore` is the first line of defense for ingest performance. A well-crafted ignore file can reduce element count by 80% or more by excluding directories and file patterns that add noise without value.

### Syntax

`.mdemgignore` uses `.gitignore` syntax:

- Lines starting with `#` are comments
- Empty lines are ignored
- Lines starting with `!` negate a pattern (re-include something previously excluded)
- Lines ending with `/` match directories only
- `*` matches anything except `/`
- `**` matches any number of path segments

### Location

Place `.mdemgignore` at the root of your project (next to `.git/`). MDEMG searches upward from the ingested directory to find it.

### Example

```gitignore
# Dependencies and build output
node_modules/
vendor/
__pycache__/
build/
dist/
target/
.venv/

# Large data directories
data/
datasets/
checkpoints/

# Binary and generated files
*.min.js
*.bundle.js
*.map
*.pyc
*.so
*.dylib

# IDE and OS
.idea/
.vscode/
.DS_Store

# Secrets
.env
.env.*

# MDEMG runtime
.mdemg/backups/
.mdemg/logs/
```

### Measuring Impact

Use `--dry-run` to compare element counts with and without your ignore file:

```bash
# Without .mdemgignore (rename it temporarily)
mv .mdemgignore .mdemgignore.bak
mdemg ingest --path . --speed fast --dry-run 2>&1 | tail -5

# With .mdemgignore
mv .mdemgignore.bak .mdemgignore
mdemg ingest --path . --speed fast --dry-run 2>&1 | tail -5
```

The difference in element count shows exactly how much work `.mdemgignore` saves.

---

## Troubleshooting

### Ingestion is slow

1. Check element count with `--dry-run`. If it is unexpectedly high, tighten `.mdemgignore`.
2. If LLM summaries are the bottleneck (visible in verbose output), try `--llm-summary-batch 20` to reduce API calls, or switch to a faster model with `--llm-summary-model gpt-4o-mini`.
3. Increase `--workers` if the MDEMG server and embedding provider can handle the load.
4. Reduce `--delay` to 0 if you are not hitting rate limits.

### Rate limit errors from embedding provider

1. Increase `--delay` (e.g., `--delay 100` or `--delay 200`).
2. Reduce `--workers` to 2.
3. Reduce `--batch` to 50.
4. The exponential backoff (`--retries 3 --retry-delay 2000`) handles transient rate limits automatically.

### Out of memory

1. Reduce `--batch` to 50 or lower.
2. Reduce `--workers` to 2.
3. Reduce `--max-elements-per-file` if individual files generate thousands of elements.

### Server timeouts

1. Increase `--timeout` (e.g., `--timeout 600` for 10 minutes).
2. Reduce `--batch` so each request processes fewer elements.
