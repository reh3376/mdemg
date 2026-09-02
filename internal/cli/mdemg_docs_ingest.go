// Sprint MDEMG-DOCS-INGEST-001 — ingest MDEMG's own documentation into the
// mdemg-dev substrate as the direct execution of the operator directive
// (2026-08-24): "ingest information into MDEMG graphDB and only fine-tune on
// how to use the mdemg framework".
//
// Mirrors the shipped `mdemg claude-docs-ingest` (CLAUDE-DOCS-INGEST-001)
// POST payload + idempotency semantics: path-keyed dedup via merge +
// SHA256 content_hash for skip-if-unchanged. Once ingested, MDEMG's own
// docs are retrievable via the same RRF+rerank pipeline that serves
// claude.code_knowledge (which itself was ingested via that sprint).
//
// Chunk strategy: read markdown files, split at H2 (`## `) boundaries;
// one MemoryNode per chunk. Files without any H2 become a single whole-
// file chunk. CLI help text is extracted from cobra Long strings via AST.
//
// Architectural framing: the operator's directive says the adapter's job
// is HOW-TO-USE-MDEMG; the substrate's job is FACT-CARRYING. This CLI
// makes MDEMG's own doc surface retrievable so shipped call sites
// (jiminy.synthesize, consulting.classify, etc.) can ground MDEMG-usage
// questions in the actual doc content.
package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"mdemg/internal/config"
)

// mdemgDocsChunk is one ingestable chunk of MDEMG's own documentation.
// One chunk becomes one MemoryNode via POST /v1/memory/ingest.
type mdemgDocsChunk struct {
	Surface       string // "features" | "user" | "api" | "claude" | "cli-help"
	SourceFile    string // repo-relative path
	SectionIndex  int    // 0-based order within the source file
	SectionHeader string // H2 header text, or "(whole file)", or "cli:<command-name>"
	Content       string // markdown / help text — the ingested payload
}

type mdemgDocsIngestConfig struct {
	rootPath      string
	endpoint      string
	spaceID       string
	dryRun        bool
	limit         int
	forceReingest bool
	batchDelayMs  int
	verbose       bool
}

func newMdemgDocsIngestCmd() *cobra.Command {
	c := &mdemgDocsIngestCommand{}
	cmd := &cobra.Command{
		Use:   "mdemg-docs-ingest",
		Short: "Ingest MDEMG's own documentation (docs/features, docs/user, docs/api, CLAUDE.md, CLI help) into the substrate",
		Long: `Ingest MDEMG's own documentation into the mdemg-dev substrate.

Sprint MDEMG-DOCS-INGEST-001 (task #142). Reads five doc surfaces and posts
one MemoryNode per H2 section (or whole file for small files) via POST
/v1/memory/ingest with path-keyed + content-hash dedup:

  1. docs/features/*.md — feature descriptions
  2. docs/user/*.md — CLI usage, multi-instance
  3. docs/api/*.md — API surface + inventories
  4. CLAUDE.md — H2 sections in Architecture Notes + Enforced Protocols
     (excludes session/sprint-record narrative junk per JIMINY-CORPUS-003)
  5. internal/cli/*.go — cobra command Long strings (one node per command)

Idempotent: re-runs on unchanged docs are no-ops (content_hash matched).
Reversible: any node can be archived via is_archived=true.

Architectural framing (operator directive 2026-08-24): the adapter's job is
HOW-TO-USE-MDEMG; the substrate's job is FACT-CARRYING. This CLI makes MDEMG's
own doc surface retrievable so shipped call sites (jiminy.synthesize,
consulting.classify, etc.) can ground MDEMG-usage questions in the actual
doc content instead of the LLM's cached general knowledge.

Directory exclusions (MDEMG-DOCS-INGEST-002, task #147): both the markdown
walker and the CLI Go walker skip subtrees named .venv, __pycache__,
node_modules, *.dist-info, *.egg-info — the operational classes that pollute
the substrate if accidentally nested. Extend the deny set via env
MDEMG_DOCS_INGEST_EXCLUDE_DIRS=name1,name2,... or disable the built-in list
entirely with MDEMG_DOCS_INGEST_EXCLUDE_DIRS=-.`,
		RunE: c.run,
	}
	cmd.Flags().StringVar(&c.opts.rootPath, "root", ".",
		"Repository root (default: current working directory)")
	cmd.Flags().StringVar(&c.opts.endpoint, "endpoint", "",
		"MDEMG endpoint (default: MDEMG_DOCS_INGEST_ENDPOINT env or http://127.0.0.1:9999)")
	cmd.Flags().StringVar(&c.opts.spaceID, "space-id", "",
		"Target MDEMG space ID (default: MDEMG_SPACE_ID env, else mdemg-dev)")
	cmd.Flags().BoolVar(&c.opts.dryRun, "dry-run", false,
		"Print what would be ingested without POSTing")
	cmd.Flags().IntVar(&c.opts.limit, "limit", 0,
		"Cap chunks to ingest (0 = all)")
	cmd.Flags().BoolVar(&c.opts.forceReingest, "force-reingest", false,
		"Ignore content_hash dedup and re-POST every chunk")
	cmd.Flags().IntVar(&c.opts.batchDelayMs, "batch-delay-ms", 0,
		"Delay between requests in ms (default: MDEMG_DOCS_INGEST_BATCH_DELAY_MS env, else 100)")
	cmd.Flags().BoolVar(&c.opts.verbose, "verbose", false, "Verbose per-chunk logging")
	return cmd
}

type mdemgDocsIngestCommand struct {
	opts mdemgDocsIngestConfig
}

func (c *mdemgDocsIngestCommand) run(cmd *cobra.Command, args []string) error {
	if err := c.resolveDefaults(); err != nil {
		return err
	}

	chunks, err := collectMdemgDocsChunks(c.opts.rootPath, c.opts.limit)
	if err != nil {
		return fmt.Errorf("collect chunks: %w", err)
	}
	if len(chunks) == 0 {
		return errors.New("collected 0 chunks — check --root path")
	}

	// Per-surface summary
	perSurface := make(map[string]int)
	for _, ch := range chunks {
		perSurface[ch.Surface]++
	}
	surfaceKeys := make([]string, 0, len(perSurface))
	for k := range perSurface {
		surfaceKeys = append(surfaceKeys, k)
	}
	sort.Strings(surfaceKeys)

	fmt.Printf("mdemg-docs-ingest: %d chunks from %s → %s (space=%s, dry_run=%v, force=%v)\n",
		len(chunks), c.opts.rootPath, c.opts.endpoint, c.opts.spaceID, c.opts.dryRun, c.opts.forceReingest)
	fmt.Printf("  per-surface breakdown:\n")
	for _, k := range surfaceKeys {
		fmt.Printf("    %-12s %d\n", k, perSurface[k])
	}

	client := &http.Client{Timeout: 60 * time.Second}

	var ingested, skipped, errCount int
	for i, ch := range chunks {
		req := buildMdemgDocsIngestRequest(ch, c.opts.spaceID)
		path, _ := req["path"].(string)
		wantHash, _ := req["content_hash"].(string)
		if c.opts.dryRun {
			if c.opts.verbose || i < 5 {
				fmt.Printf("  [dry] %s :: %s (path=%s)\n", ch.SourceFile, ch.SectionHeader, path)
			}
			ingested++
			continue
		}

		// Pre-check dedup — mirrors claude-docs-ingest pattern.
		if !c.opts.forceReingest {
			existing, existErr := getNodeContentHash(client, c.opts.endpoint, c.opts.spaceID, path)
			if existErr == nil && existing == wantHash {
				skipped++
				if c.opts.verbose {
					fmt.Printf("  [skip] %s (content_hash matched pre-check)\n", path)
				}
				if c.opts.batchDelayMs > 0 && i < len(chunks)-1 {
					time.Sleep(time.Duration(c.opts.batchDelayMs) * time.Millisecond)
				}
				continue
			}
		}

		result, postErr := postMdemgDocsIngest(client, c.opts.endpoint, req, c.opts.forceReingest)
		switch {
		case postErr != nil:
			errCount++
			slog.Warn("mdemg-docs-ingest: chunk failed",
				"source_file", ch.SourceFile, "path", path, "error", postErr)
		case result == "skipped":
			skipped++
			if c.opts.verbose {
				fmt.Printf("  [skip] %s (content_hash matched)\n", path)
			}
		default:
			ingested++
			if c.opts.verbose {
				fmt.Printf("  [ok]   %s → %s\n", path, result)
			}
		}

		if c.opts.batchDelayMs > 0 && i < len(chunks)-1 {
			time.Sleep(time.Duration(c.opts.batchDelayMs) * time.Millisecond)
		}
		if (i+1)%50 == 0 {
			fmt.Printf("  progress: %d/%d (ingested=%d skipped=%d errors=%d)\n",
				i+1, len(chunks), ingested, skipped, errCount)
		}
	}

	fmt.Printf("\nmdemg-docs-ingest DONE: ingested=%d skipped=%d errors=%d total=%d\n",
		ingested, skipped, errCount, len(chunks))
	fmt.Fprintf(os.Stderr, "[%s] mdemg-docs-ingest: %d/%d in %s (%d skipped, %d errors)\n",
		time.Now().UTC().Format(time.RFC3339), ingested, len(chunks), c.opts.spaceID, skipped, errCount)
	if errCount > 0 {
		return fmt.Errorf("%d chunks failed to ingest", errCount)
	}
	return nil
}

func (c *mdemgDocsIngestCommand) resolveDefaults() error {
	if c.opts.rootPath == "" {
		c.opts.rootPath = "."
	}
	if c.opts.endpoint == "" {
		if v := os.Getenv("MDEMG_DOCS_INGEST_ENDPOINT"); v != "" {
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
		if v := os.Getenv("MDEMG_DOCS_INGEST_BATCH_DELAY_MS"); v != "" {
			_, _ = fmt.Sscanf(v, "%d", &c.opts.batchDelayMs)
		}
		if c.opts.batchDelayMs == 0 {
			c.opts.batchDelayMs = 100
		}
	}
	return nil
}

// collectMdemgDocsChunks walks the 5 doc surfaces + produces ordered chunks.
func collectMdemgDocsChunks(root string, limit int) ([]mdemgDocsChunk, error) {
	var out []mdemgDocsChunk

	// 1-3: markdown surfaces (features, user, api).
	mdSurfaces := []struct {
		Surface string
		Dir     string
	}{
		{"features", filepath.Join(root, "docs", "features")},
		{"user", filepath.Join(root, "docs", "user")},
		{"api", filepath.Join(root, "docs", "api")},
	}
	for _, s := range mdSurfaces {
		chunks, err := collectMarkdownChunks(s.Surface, s.Dir, root)
		if err != nil {
			return nil, fmt.Errorf("collect %s: %w", s.Surface, err)
		}
		out = append(out, chunks...)
	}

	// 4: CLAUDE.md (H2 slicer with narrative-junk exclusion).
	claudePath := filepath.Join(root, "CLAUDE.md")
	if _, err := os.Stat(claudePath); err == nil {
		chunks, err := collectClaudeMdChunks(claudePath, root)
		if err != nil {
			return nil, fmt.Errorf("collect CLAUDE.md: %w", err)
		}
		out = append(out, chunks...)
	}

	// 5: CLI help text (cobra Long from internal/cli/*.go via AST).
	cliDir := filepath.Join(root, "internal", "cli")
	if _, err := os.Stat(cliDir); err == nil {
		chunks, err := collectCliLongChunks(cliDir, root)
		if err != nil {
			return nil, fmt.Errorf("collect CLI help: %w", err)
		}
		out = append(out, chunks...)
	}

	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// mdemgDocsH2RE matches a markdown H2 header line.
var mdemgDocsH2RE = regexp.MustCompile(`(?m)^## \s*(.+?)\s*$`)

// splitMdemgDocsH2 splits a markdown body at H2 (`## `) boundaries.
// Returns sections as (header, body) pairs. If no H2, returns one entry
// with header="(whole file)" and the full body.
//
// Content BEFORE the first H2 is preserved as a "(preamble)" section so
// file-level context (title, intro) isn't lost.
func splitMdemgDocsH2(body string) []struct{ Header, Body string } {
	indices := mdemgDocsH2RE.FindAllStringSubmatchIndex(body, -1)
	if len(indices) == 0 {
		return []struct{ Header, Body string }{{"(whole file)", strings.TrimSpace(body)}}
	}

	var out []struct{ Header, Body string }
	// Preamble: content before first H2
	if indices[0][0] > 0 {
		pre := strings.TrimSpace(body[:indices[0][0]])
		if pre != "" {
			out = append(out, struct{ Header, Body string }{"(preamble)", pre})
		}
	}
	// Each H2 section runs from its header start to the next H2 (or EOF)
	for i, idx := range indices {
		headerStart := idx[0]
		headerText := strings.TrimSpace(body[idx[2]:idx[3]])
		var sectionEnd int
		if i+1 < len(indices) {
			sectionEnd = indices[i+1][0]
		} else {
			sectionEnd = len(body)
		}
		section := strings.TrimSpace(body[headerStart:sectionEnd])
		if section != "" {
			out = append(out, struct{ Header, Body string }{headerText, section})
		}
	}
	return out
}

// mdemgDocsDefaultExcludeDirs is the built-in deny-set of directory names
// the doc-ingest walkers skip WHOLESALE (via fs.SkipDir on the subtree).
// Sprint MDEMG-DOCS-INGEST-002 (task #147) added these so an accidentally-
// nested Python virtualenv, bytecode cache, npm module tree, or Python
// packaging metadata never lands as a MemoryNode in the substrate.
//
// Extend via `MDEMG_DOCS_INGEST_EXCLUDE_DIRS` (comma-separated). Set to
// `-` to disable the built-in list entirely (advanced users only).
var mdemgDocsDefaultExcludeDirs = []string{
	".venv",
	"__pycache__",
	"node_modules",
	"dist-info",
	"egg-info",
}

// mdemgDocsExcludeDirSet resolves the effective exclusion set from the
// built-in defaults + optional env override. Matches on the directory's
// base name; suffix-match applies to `dist-info` / `egg-info` because
// Python packaging emits `foo-1.2.3.dist-info` / `foo.egg-info` shapes.
func mdemgDocsExcludeDirSet() map[string]struct{} {
	set := make(map[string]struct{}, len(mdemgDocsDefaultExcludeDirs)+4)
	seed := mdemgDocsDefaultExcludeDirs
	if v := strings.TrimSpace(os.Getenv("MDEMG_DOCS_INGEST_EXCLUDE_DIRS")); v != "" {
		if v == "-" {
			seed = nil
		} else {
			seed = append(seed, strings.Split(v, ",")...)
		}
	}
	for _, name := range seed {
		if n := strings.TrimSpace(name); n != "" {
			set[n] = struct{}{}
		}
	}
	return set
}

// mdemgDocsShouldExcludeDir reports whether a directory name matches the
// exclusion set (exact match) or ends in `dist-info` / `egg-info` (Python
// packaging metadata dir suffix — `foo-1.2.3.dist-info`, `foo.egg-info`).
func mdemgDocsShouldExcludeDir(name string, set map[string]struct{}) bool {
	if _, hit := set[name]; hit {
		return true
	}
	// Suffix match for the two Python-packaging patterns; only fires if the
	// suffix is in the active set (so `MDEMG_DOCS_INGEST_EXCLUDE_DIRS=-`
	// truly disables it).
	if _, hit := set["dist-info"]; hit && strings.HasSuffix(name, ".dist-info") {
		return true
	}
	if _, hit := set["egg-info"]; hit && strings.HasSuffix(name, ".egg-info") {
		return true
	}
	return false
}

// collectMarkdownChunks walks a directory and produces one chunk per H2
// section in each *.md file (with preamble captured if any).
//
// Skips subtrees whose directory name matches mdemgDocsExcludeDirSet
// (built-in + MDEMG_DOCS_INGEST_EXCLUDE_DIRS env). Prevents accidental
// ingest of nested venvs / bytecode caches / npm trees / Python packaging
// metadata into the substrate — sprint MDEMG-DOCS-INGEST-002 (task #147).
func collectMarkdownChunks(surface, dir, root string) ([]mdemgDocsChunk, error) {
	var out []mdemgDocsChunk
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil // silently skip if a surface dir doesn't exist
	}
	excludes := mdemgDocsExcludeDirSet()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			// Never skip the root walker dir itself, even if its name
			// somehow matches — an operator pointing --root or a surface
			// dir directly at an excluded name still gets scanned.
			if path != dir && mdemgDocsShouldExcludeDir(d.Name(), excludes) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		sections := splitMdemgDocsH2(string(content))
		for i, s := range sections {
			out = append(out, mdemgDocsChunk{
				Surface:       surface,
				SourceFile:    rel,
				SectionIndex:  i,
				SectionHeader: s.Header,
				Content:       s.Body,
			})
		}
		return nil
	})
	return out, err
}

// mdemgClaudeMdRejectHeadersRE — H2 headers to EXCLUDE from CLAUDE.md ingest.
// These are session/sprint-narrative sections (JIMINY-CORPUS-003 junk class);
// keep durable Architecture Notes + Enforced Protocols only.
var mdemgClaudeMdRejectHeadersRE = regexp.MustCompile(`(?i)^(Session|Sprint |Recent |Session-specific)`)

// collectClaudeMdChunks slices CLAUDE.md at H2, drops narrative/session junk.
func collectClaudeMdChunks(path, root string) ([]mdemgDocsChunk, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	rel, _ := filepath.Rel(root, path)
	sections := splitMdemgDocsH2(string(content))
	var out []mdemgDocsChunk
	kept := 0
	for _, s := range sections {
		if mdemgClaudeMdRejectHeadersRE.MatchString(s.Header) {
			continue
		}
		if s.Header == "(preamble)" || s.Header == "(whole file)" {
			// Preamble of CLAUDE.md is the file-header prose — useful.
			// (whole file) shouldn't happen given CLAUDE.md has H2s, but handle anyway.
		}
		out = append(out, mdemgDocsChunk{
			Surface:       "claude",
			SourceFile:    rel,
			SectionIndex:  kept,
			SectionHeader: s.Header,
			Content:       s.Body,
		})
		kept++
	}
	return out, nil
}

// collectCliLongChunks extracts cobra command Long: strings from Go source
// via AST. One chunk per (file, Long value) pair. Skips empty Longs.
//
// Honors mdemgDocsExcludeDirSet for symmetry with the markdown walker —
// same defensive envelope against accidental nested venv/vendor/etc trees
// (task #147).
func collectCliLongChunks(cliDir, root string) ([]mdemgDocsChunk, error) {
	var out []mdemgDocsChunk
	fset := token.NewFileSet()
	excludes := mdemgDocsExcludeDirSet()
	err := filepath.WalkDir(cliDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if path != cliDir && mdemgDocsShouldExcludeDir(d.Name(), excludes) {
				return fs.SkipDir
			}
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			// Non-fatal: skip files that don't parse (shouldn't happen in-tree)
			slog.Warn("mdemg-docs-ingest: skip unparseable go file", "path", path, "err", err)
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		commandName, longs := extractCobraLongs(file)
		// commandName can be a hint from the first Use field found in-file;
		// if it's empty (file has no cobra.Command literal with Use), fall
		// back to the filename stem.
		if commandName == "" {
			commandName = strings.TrimSuffix(name, ".go")
		}
		for i, longText := range longs {
			out = append(out, mdemgDocsChunk{
				Surface:       "cli-help",
				SourceFile:    rel,
				SectionIndex:  i,
				SectionHeader: "cli:" + commandName,
				Content:       longText,
			})
		}
		return nil
	})
	return out, err
}

// extractCobraLongs walks a Go AST and pulls out the first Use string it
// finds (as the command-name hint) and all non-empty Long values.
func extractCobraLongs(file *ast.File) (commandName string, longs []string) {
	ast.Inspect(file, func(n ast.Node) bool {
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		// Look for &cobra.Command{...} or cobra.Command{...}
		if !isCobraCommandType(cl.Type) {
			return true
		}
		for _, elt := range cl.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			switch key.Name {
			case "Use":
				if commandName == "" {
					if s := stringLiteralValue(kv.Value); s != "" {
						// Take the first token (some Use values are "foo <arg>")
						parts := strings.Fields(s)
						if len(parts) > 0 {
							commandName = parts[0]
						}
					}
				}
			case "Long":
				if s := stringLiteralValue(kv.Value); s != "" {
					longs = append(longs, s)
				}
			}
		}
		return true
	})
	return commandName, longs
}

// isCobraCommandType reports whether the expression is *cobra.Command or cobra.Command.
func isCobraCommandType(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.SelectorExpr:
		if ident, ok := t.X.(*ast.Ident); ok && ident.Name == "cobra" && t.Sel.Name == "Command" {
			return true
		}
	case *ast.StarExpr:
		return isCobraCommandType(t.X)
	}
	return false
}

// stringLiteralValue returns the string value if expr is a plain string
// literal (regular or raw), else "".
func stringLiteralValue(expr ast.Expr) string {
	bl, ok := expr.(*ast.BasicLit)
	if !ok || bl.Kind != token.STRING {
		return ""
	}
	// Handle raw strings (backticks) and regular strings.
	if len(bl.Value) < 2 {
		return ""
	}
	first := bl.Value[0]
	if first == '`' && bl.Value[len(bl.Value)-1] == '`' {
		return bl.Value[1 : len(bl.Value)-1]
	}
	if first == '"' && bl.Value[len(bl.Value)-1] == '"' {
		// Unquote regular string literal.
		var out strings.Builder
		i := 1
		for i < len(bl.Value)-1 {
			ch := bl.Value[i]
			if ch == '\\' && i+1 < len(bl.Value)-1 {
				switch bl.Value[i+1] {
				case 'n':
					out.WriteByte('\n')
				case 't':
					out.WriteByte('\t')
				case '\\':
					out.WriteByte('\\')
				case '"':
					out.WriteByte('"')
				default:
					out.WriteByte(bl.Value[i+1])
				}
				i += 2
				continue
			}
			out.WriteByte(ch)
			i++
		}
		return out.String()
	}
	return ""
}

// mdemgDocsPathSlugRE — trim to lowercase alphanum + dash, cap length.
var mdemgDocsPathSlugRE = regexp.MustCompile(`[^a-z0-9]+`)

func mdemgDocsPathSlug(s string) string {
	slug := strings.ToLower(strings.TrimSpace(s))
	slug = mdemgDocsPathSlugRE.ReplaceAllString(slug, "-")
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

// buildMdemgDocsIngestRequest constructs the /v1/memory/ingest payload for one chunk.
// Mirrors buildClaudeDocsIngestRequest but with mdemg-docs-scoped path prefix + tags.
func buildMdemgDocsIngestRequest(ch mdemgDocsChunk, spaceID string) map[string]any {
	// Filename stem (without extension) is used in the path to keep sibling files
	// with the same header distinct. E.g. docs/features/alert-dispatcher.md +
	// docs/features/hitl-auto-curation.md can both have "## How it works" H2.
	fileStem := strings.TrimSuffix(filepath.Base(ch.SourceFile), filepath.Ext(ch.SourceFile))
	headerSlug := mdemgDocsPathSlug(ch.SectionHeader)
	fileSlug := mdemgDocsPathSlug(fileStem)
	path := fmt.Sprintf("mdemg-docs/%s/%s/%03d__%s",
		ch.Surface, fileSlug, ch.SectionIndex, headerSlug)

	content := ch.Content

	sum := sha256.Sum256([]byte(content))
	contentHash := hex.EncodeToString(sum[:])

	summary := ch.SectionHeader
	if fileStem != "" && fileStem != ch.SectionHeader {
		summary = fileStem + " — " + ch.SectionHeader
	}
	if len(summary) > 500 {
		summary = summary[:500]
	}

	tags := []string{
		"docs:mdemg",
		"docs:mdemg:" + ch.Surface,
		"obs_type:technical_note",
	}

	payload := map[string]any{
		"space_id":     spaceID,
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
		"source":       "mdemg-docs-ingest",
		"content":      content,
		"path":         path,
		"name":         ch.SectionHeader,
		"summary":      summary,
		"tags":         tags,
		"content_hash": contentHash,
	}
	return payload
}

// postMdemgDocsIngest POSTs one chunk to /v1/memory/ingest.
// Returns "skipped" if server signals dedup; otherwise returns the node_id string.
// Mirrors postClaudeDocsIngest exactly.
func postMdemgDocsIngest(client *http.Client, endpoint string, payload map[string]any, forceReingest bool) (string, error) {
	if forceReingest {
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
