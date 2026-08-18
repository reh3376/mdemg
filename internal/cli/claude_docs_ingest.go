// Sprint CLAUDE-DOCS-INGEST-001 — ingest Claude Code docs corpus into MDEMG substrate.
//
// Reads the curated Q&A JSONL corpus (produced by neural/training/curate_claude_docs.py)
// and writes one MemoryNode per row via POST /v1/memory/ingest. Path-keyed for
// dedup via merge + SHA256 content_hash for skip-if-unchanged. Once ingested,
// the docs are surface-able via MDEMG's 4-5-column retrieval pipeline; RSIC +
// consolidation handle upward-abstraction; consulting/jiminy synthesize into
// task-appropriate guidance at inference.
//
// Architectural framing (Sprint 004 Rule F): fact-recall tasks are substrate-
// ingest problems in MDEMG's architecture, not model-weight-fine-tune problems.
// This CLI is the architecturally-correct successor to CLAUDE-DOCS-TRAINING-001..004.
package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"mdemg/internal/config"
)

// claudeDocsRow mirrors the curated qa.jsonl schema (curate_claude_docs.py + split_claude_docs*.py).
type claudeDocsRow struct {
	RowID         string `json:"row_id"`
	Prompt        string `json:"prompt"`
	Completion    string `json:"completion"`
	SourceURL     string `json:"source_url"`
	SourceSHA256  string `json:"source_sha256"`
	SourceSlug    string `json:"source_slug"`
	DocTitle      string `json:"doc_title"`
	SectionHeader string `json:"section_header"`
	ConceptType   string `json:"concept_type"`
	SectionIndex  int    `json:"section_index"`
	WordCount     int    `json:"word_count"`
	CuratedAtUTC  string `json:"curated_at_utc"`
}

type claudeDocsIngestConfig struct {
	corpusPath    string
	endpoint      string
	spaceID       string
	dryRun        bool
	limit         int
	forceReingest bool
	batchDelayMs  int
	verbose       bool
}

func newClaudeDocsIngestCmd() *cobra.Command {
	cfg := &claudeDocsIngestCommand{}
	cmd := &cobra.Command{
		Use:   "claude-docs-ingest",
		Short: "Ingest curated Claude Code docs corpus into the MDEMG substrate",
		Long: `Ingest the curated Claude Code docs Q&A corpus into MDEMG.

Reads the JSONL corpus produced by neural/training/curate_claude_docs.py, then
POSTs each row to /v1/memory/ingest as a technical_note L0 node. Path-keyed
dedup means re-running the ingest with the same corpus is a no-op (only
content-changed sections re-write). Once ingested, the docs are surface-able
via MDEMG's RRF+rerank retrieval pipeline; RSIC + consolidation handle
upward-abstraction into L1+ emergent concepts.

Architectural framing (CLAUDE-DOCS-TRAINING-004 Rule F): fact-recall tasks
are substrate-ingest problems, not model-weight-fine-tune problems.`,
		RunE: cfg.run,
	}
	cmd.Flags().StringVar(&cfg.opts.corpusPath, "corpus", "training_data/claude-docs/curated/qa.jsonl",
		"Path to the curated Q&A JSONL corpus")
	cmd.Flags().StringVar(&cfg.opts.endpoint, "endpoint", "",
		"MDEMG endpoint (default: from CLAUDE_DOCS_INGEST_ENDPOINT env or LISTEN_ADDR)")
	cmd.Flags().StringVar(&cfg.opts.spaceID, "space-id", "",
		"Target MDEMG space ID (default: MDEMG_SPACE_ID env, else mdemg-dev)")
	cmd.Flags().BoolVar(&cfg.opts.dryRun, "dry-run", false,
		"Print what would be ingested without POSTing")
	cmd.Flags().IntVar(&cfg.opts.limit, "limit", 0,
		"Cap rows to ingest (0 = all)")
	cmd.Flags().BoolVar(&cfg.opts.forceReingest, "force-reingest", false,
		"Ignore content_hash dedup and re-POST every row")
	cmd.Flags().IntVar(&cfg.opts.batchDelayMs, "batch-delay-ms", 0,
		"Delay between requests in ms (default: CLAUDE_DOCS_INGEST_BATCH_DELAY_MS env, else 50)")
	cmd.Flags().BoolVar(&cfg.opts.verbose, "verbose", false, "Verbose per-row logging")
	return cmd
}

type claudeDocsIngestCommand struct {
	opts claudeDocsIngestConfig
}

func (c *claudeDocsIngestCommand) run(cmd *cobra.Command, args []string) error {
	if err := c.resolveDefaults(); err != nil {
		return err
	}

	rows, err := readClaudeDocsCorpus(c.opts.corpusPath, c.opts.limit)
	if err != nil {
		return fmt.Errorf("read corpus: %w", err)
	}
	if len(rows) == 0 {
		return errors.New("corpus produced 0 rows — check --corpus path")
	}

	fmt.Printf("claude-docs-ingest: %d rows from %s → %s (space=%s, dry_run=%v, force=%v)\n",
		len(rows), c.opts.corpusPath, c.opts.endpoint, c.opts.spaceID, c.opts.dryRun, c.opts.forceReingest)

	client := &http.Client{Timeout: 60 * time.Second}

	var ingested, skipped, errs int
	for i, row := range rows {
		req := buildClaudeDocsIngestRequest(row, c.opts.spaceID)
		path, _ := req["path"].(string)
		wantHash, _ := req["content_hash"].(string)
		if c.opts.dryRun {
			if c.opts.verbose || i < 3 {
				fmt.Printf("  [dry] %s :: %s (path=%s)\n", row.SourceSlug, row.SectionHeader, path)
			}
			ingested++
			continue
		}

		// Pre-check dedup: fetch existing node's content_hash; skip if match.
		// Mirrors the ingest-claude-md pattern; the server DOES write on POST
		// even with a matching hash (anomaly reported, node created), so we
		// must gate client-side before POSTing to keep re-runs idempotent.
		if !c.opts.forceReingest {
			existing, err := getNodeContentHash(client, c.opts.endpoint, c.opts.spaceID, path)
			if err == nil && existing == wantHash {
				skipped++
				if c.opts.verbose {
					fmt.Printf("  [skip] %s (content_hash matched pre-check)\n", path)
				}
				if c.opts.batchDelayMs > 0 && i < len(rows)-1 {
					time.Sleep(time.Duration(c.opts.batchDelayMs) * time.Millisecond)
				}
				continue
			}
		}

		result, err := postClaudeDocsIngest(client, c.opts.endpoint, req, c.opts.forceReingest)
		switch {
		case err != nil:
			errs++
			slog.Warn("claude-docs-ingest: row failed",
				"row_id", row.RowID, "path", req["path"], "error", err)
		case result == "skipped":
			skipped++
			if c.opts.verbose {
				fmt.Printf("  [skip] %s (content_hash matched)\n", req["path"])
			}
		default:
			ingested++
			if c.opts.verbose {
				fmt.Printf("  [ok]   %s → %s\n", req["path"], result)
			}
		}

		if c.opts.batchDelayMs > 0 && i < len(rows)-1 {
			time.Sleep(time.Duration(c.opts.batchDelayMs) * time.Millisecond)
		}
		if (i+1)%100 == 0 {
			fmt.Printf("  progress: %d/%d (ingested=%d skipped=%d errors=%d)\n",
				i+1, len(rows), ingested, skipped, errs)
		}
	}

	fmt.Printf("\nclaude-docs-ingest DONE: ingested=%d skipped=%d errors=%d total=%d\n",
		ingested, skipped, errs, len(rows))
	fmt.Fprintf(os.Stderr, "[%s] claude-docs-ingest: %d/%d in %s (%d skipped, %d errors)\n",
		time.Now().UTC().Format(time.RFC3339), ingested, len(rows), c.opts.spaceID, skipped, errs)
	if errs > 0 {
		return fmt.Errorf("%d rows failed to ingest", errs)
	}
	return nil
}

func (c *claudeDocsIngestCommand) resolveDefaults() error {
	if c.opts.endpoint == "" {
		if v := os.Getenv("CLAUDE_DOCS_INGEST_ENDPOINT"); v != "" {
			c.opts.endpoint = v
		} else {
			cfg, _ := config.FromEnv()
			c.opts.endpoint = "http://127.0.0.1" + cfg.ListenAddr
			if cfg.ListenAddr == "" {
				c.opts.endpoint = "http://127.0.0.1:9999"
			}
		}
	}
	if c.opts.spaceID == "" {
		if v := os.Getenv("MDEMG_SPACE_ID"); v != "" {
			c.opts.spaceID = v
		} else {
			c.opts.spaceID = "mdemg-dev"
		}
	}
	if c.opts.batchDelayMs == 0 {
		if v := os.Getenv("CLAUDE_DOCS_INGEST_BATCH_DELAY_MS"); v != "" {
			fmt.Sscanf(v, "%d", &c.opts.batchDelayMs)
		}
		if c.opts.batchDelayMs == 0 {
			c.opts.batchDelayMs = 50
		}
	}
	return nil
}

// readClaudeDocsCorpus parses the qa.jsonl corpus, capping at `limit` rows (0 = all).
func readClaudeDocsCorpus(path string, limit int) ([]claudeDocsRow, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()

	var rows []claudeDocsRow
	dec := json.NewDecoder(fh)
	for dec.More() {
		var row claudeDocsRow
		if err := dec.Decode(&row); err != nil {
			return nil, fmt.Errorf("decode row %d: %w", len(rows)+1, err)
		}
		if row.RowID == "" || row.Prompt == "" || row.Completion == "" {
			return nil, fmt.Errorf("row %d missing required fields (row_id/prompt/completion)", len(rows)+1)
		}
		rows = append(rows, row)
		if limit > 0 && len(rows) >= limit {
			break
		}
	}
	return rows, nil
}

// claudeDocsPathSlug regex — trim to lowercase alphanum + dash, cap length.
var claudeDocsPathSlugRE = regexp.MustCompile(`[^a-z0-9]+`)

func claudeDocsPathSlug(s string) string {
	slug := strings.ToLower(strings.TrimSpace(s))
	slug = claudeDocsPathSlugRE.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if len(slug) > 80 {
		slug = slug[:80]
		slug = strings.TrimRight(slug, "-")
	}
	if slug == "" {
		slug = "unnamed"
	}
	return slug
}

// buildClaudeDocsIngestRequest constructs the /v1/memory/ingest payload for one row.
func buildClaudeDocsIngestRequest(row claudeDocsRow, spaceID string) map[string]any {
	pathSlug := claudeDocsPathSlug(row.SectionHeader)
	path := fmt.Sprintf("claude-docs/%s/%03d__%s", row.SourceSlug, row.SectionIndex, pathSlug)

	content := row.Prompt + "\n\n" + row.Completion

	sum := sha256.Sum256([]byte(content))
	contentHash := hex.EncodeToString(sum[:])

	summary := row.SectionHeader
	if row.DocTitle != "" {
		summary = row.DocTitle + " — " + row.SectionHeader
	}
	if len(summary) > 500 {
		summary = summary[:500]
	}

	tags := []string{"docs:claude-code", "docs:" + row.SourceSlug, "obs_type:technical_note"}
	if row.ConceptType != "" {
		tags = append(tags, "docs:concept:"+row.ConceptType)
	}

	payload := map[string]any{
		"space_id":     spaceID,
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
		"source":       "claude-docs-ingest",
		"content":      content,
		"path":         path,
		"name":         row.SectionHeader,
		"summary":      summary,
		"tags":         tags,
		"content_hash": contentHash,
	}
	return payload
}

// postClaudeDocsIngest POSTs one row. Returns "skipped" if server signals dedup;
// otherwise returns the node_id string.
func postClaudeDocsIngest(client *http.Client, endpoint string, payload map[string]any, forceReingest bool) (string, error) {
	if forceReingest {
		// Neuter the content_hash so the server doesn't dedup on it.
		delete(payload, "content_hash")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	resp, err := client.Post(endpoint+"/v1/memory/ingest", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("ingest returned %d: %s", resp.StatusCode, string(rb))
	}
	var out struct {
		NodeID    string `json:"node_id"`
		Deduped   bool   `json:"deduped"`
		Unchanged bool   `json:"unchanged"`
	}
	_ = json.Unmarshal(rb, &out)
	if out.Deduped || out.Unchanged {
		return "skipped", nil
	}
	if out.NodeID == "" {
		return "ok", nil
	}
	return out.NodeID, nil
}
