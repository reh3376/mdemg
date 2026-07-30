package retrieval

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"mdemg/internal/models"

	neo4j "github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// RETRIEVAL-REVERSE-LOOKUP-001 (RQA-001 cluster A): "what consumes X?" queries
// fail because MDEMG's node summaries are function-level abstractions that
// don't include the SQL/symbol strings from function bodies. No substrate-side
// indexing solves this — the answer isn't in the substrate at all.
//
// Solution: at query time, extract candidate symbols from the query, walk the
// workspace filesystem for file-content matches, look up matching files as
// MemoryNodes, inject them into the RRF pool via the same post-rerank quota
// mechanism RETRIEVAL-LAYER-BALANCE-001 uses.
//
// SECURITY: no shell-out. Uses Go-native filepath.Walk + regexp.MatchString
// only. Reads files under `workspaceRoot` only; refuses symlinks that escape
// the root. File-count cap prevents pathological workspace scans.

// symbolRegex matches identifier-shaped tokens (letters, digits, underscore).
var symbolRegex = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

// reverseLookupShapeVerbs are verbs whose presence in a query strongly
// signals a reverse-lookup intent ("what X the Y?" / "which files X Y?" /
// "who X Y?"). The `IsReverseLookupQuery` gate uses this list to decide
// whether to activate the reverse-ref quota — activating on every query
// caused live-caught regressions (q10 5/5→2/5 when quota promoted
// grep-matched irrelevant files over natural top-K).
var reverseLookupShapeVerbs = map[string]struct{}{
	"consumes": {}, "consume": {}, "consumed": {}, "consumer": {}, "consumers": {},
	"uses": {}, "use": {}, "used": {}, "using": {}, "usage": {}, "users": {},
	"reads": {}, "read": {}, "reading": {},
	"writes": {}, "write": {}, "writing": {}, "writers": {},
	"calls": {}, "call": {}, "calling": {},
	"invokes": {}, "invoke": {}, "invoked": {},
	"references": {}, "reference": {}, "referenced": {}, "referencing": {},
	"depends": {}, "depend": {}, "dependency": {}, "dependencies": {}, "depending": {},
	"imports": {}, "import": {}, "imported": {}, "importers": {},
	"queries": {}, "query": {}, "querying": {},
	"joins": {}, "join": {}, "joining": {},
	"selects": {}, "select": {}, "selecting": {},
	"inserts": {}, "insert": {}, "inserting": {},
	"updates": {}, "update": {}, "updating": {},
	"deletes": {}, "delete": {}, "deleting": {},
	"affects": {}, "affect": {}, "affected": {}, "affecting": {},
	"produces": {}, "produce": {}, "produced": {}, "producer": {}, "producers": {},
	"triggers": {}, "trigger": {}, "triggered": {}, "triggering": {},
	"where": {}, "who": {}, // "where is X used?", "who calls X?"
}

// IsReverseLookupQuery returns true when the query text contains a
// reverse-lookup verb — a signal that the caller is asking "what/who X the
// Y?" rather than "what IS Y?". The reverse-ref quota promoter fires only
// when this returns true, so semantic-similarity queries (which are the
// vast majority) aren't perturbed by grep-matched candidates.
func IsReverseLookupQuery(queryText string) bool {
	// Cheap tokenize: lowercase + regex-split on identifier boundaries.
	tokens := symbolRegex.FindAllString(strings.ToLower(queryText), -1)
	for _, t := range tokens {
		if _, ok := reverseLookupShapeVerbs[t]; ok {
			return true
		}
	}
	return false
}

// reverseRefStopWords are query-shaped English words that carry no reverse-
// lookup signal. Keep this list small — over-filtering hides real symbols.
// Case-insensitive match.
var reverseRefStopWords = map[string]struct{}{
	"about": {}, "after": {}, "again": {}, "against": {}, "along": {},
	"already": {}, "another": {}, "any": {}, "are": {}, "because": {},
	"been": {}, "before": {}, "being": {}, "between": {}, "both": {},
	"could": {}, "did": {}, "does": {}, "doing": {}, "done": {},
	"each": {}, "either": {}, "else": {}, "every": {}, "from": {},
	"further": {}, "had": {}, "has": {}, "have": {}, "having": {},
	"here": {}, "how": {}, "into": {}, "just": {}, "like": {},
	"more": {}, "most": {}, "much": {}, "must": {}, "not": {},
	"now": {}, "off": {}, "once": {}, "only": {}, "other": {},
	"our": {}, "out": {}, "over": {}, "own": {}, "same": {},
	"should": {}, "since": {}, "some": {}, "such": {}, "than": {},
	"that": {}, "the": {}, "their": {}, "them": {}, "then": {},
	"there": {}, "these": {}, "they": {}, "this": {}, "those": {},
	"through": {}, "under": {}, "until": {}, "using": {}, "very": {},
	"was": {}, "were": {}, "what": {}, "when": {}, "where": {},
	"which": {}, "while": {}, "who": {}, "whom": {}, "whose": {},
	"why": {}, "will": {}, "with": {}, "would": {}, "you": {}, "your": {},
	// Query-shape stop words specific to reverse-lookup phrasing
	"consumes": {}, "consume": {}, "consumed": {}, "consumer": {}, "consumers": {},
	"uses": {}, "used": {}, "usage": {},
	"reads": {}, "read": {}, "writes": {}, "write": {},
	"calls": {}, "call": {}, "invokes": {}, "invoke": {},
	"references": {}, "reference": {},
	"table": {}, "tables": {}, "field": {}, "fields": {},
	"function": {}, "functions": {}, "method": {}, "methods": {},
	"query": {}, "queries": {}, "value": {}, "values": {},
	"default": {}, "config": {}, "setting": {}, "settings": {},
	"error": {}, "errors": {},
}

// extractCandidateSymbols returns distinct identifier tokens from queryText
// that (a) are at least minLen chars, (b) aren't in the stop-word list.
// The returned order is FIRST-OCCURRENCE preserved so downstream ranking
// can prefer earlier-mentioned symbols if it wants to.
func extractCandidateSymbols(queryText string, minLen int) []string {
	if minLen <= 0 {
		minLen = 5
	}
	matches := symbolRegex.FindAllString(queryText, -1)
	seen := map[string]struct{}{}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < minLen {
			continue
		}
		low := strings.ToLower(m)
		if _, stop := reverseRefStopWords[low]; stop {
			continue
		}
		if _, dup := seen[low]; dup {
			continue
		}
		seen[low] = struct{}{}
		// Preserve the ORIGINAL casing of the first occurrence for the grep —
		// case-sensitive match reduces false-positives (e.g. "table" vs "Table"
		// as a Go type name).
		out = append(out, m)
	}
	return out
}

// grepHit represents a file in the workspace whose content matches at least
// one candidate symbol. HitCount aggregates counts across all matched symbols.
type grepHit struct {
	AbsPath  string
	RelPath  string
	HitCount int
}

// grepReferences walks workspaceRoot for files matching allowExts, counts
// occurrences of each symbol (case-sensitive `\bSYMBOL\b`), and returns the
// top-K hits ranked by HitCount DESC.
//
// SECURITY: pure Go, no shell-out. Skips directories in excludeDirs, symlinks
// escaping workspaceRoot, and stops early after maxFilesScanned files.
func grepReferences(workspaceRoot string, symbols []string, allowExts, excludeDirs []string, maxFilesScanned, topK int) ([]grepHit, error) {
	if workspaceRoot == "" || len(symbols) == 0 || topK <= 0 {
		return nil, nil
	}
	absRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("workspaceRoot is not a directory")
	}
	extSet := make(map[string]struct{}, len(allowExts))
	for _, e := range allowExts {
		e = strings.ToLower(strings.TrimSpace(e))
		if e == "" {
			continue
		}
		if !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		extSet[e] = struct{}{}
	}
	excludeSet := make(map[string]struct{}, len(excludeDirs))
	for _, d := range excludeDirs {
		d = strings.TrimSpace(d)
		if d != "" {
			excludeSet[d] = struct{}{}
		}
	}
	patterns := make([]*regexp.Regexp, 0, len(symbols))
	for _, s := range symbols {
		if s == "" {
			continue
		}
		// Word-boundary match per symbol; RegExp.MustCompile inputs are
		// pre-filtered identifier tokens so metacharacter escaping is a
		// defense-in-depth (regexp.QuoteMeta) rather than a strict need.
		pat, perr := regexp.Compile(`\b` + regexp.QuoteMeta(s) + `\b`)
		if perr != nil {
			continue
		}
		patterns = append(patterns, pat)
	}
	if len(patterns) == 0 {
		return nil, nil
	}
	hits := map[string]*grepHit{}
	filesScanned := 0

	walkFn := func(path string, d fs.DirEntry, wErr error) error {
		if wErr != nil {
			return nil // skip unreadable entries; don't abort the whole walk
		}
		if d.IsDir() {
			base := d.Name()
			if _, skip := excludeSet[base]; skip {
				return filepath.SkipDir
			}
			// Skip hidden dirs (.git, .venv, etc.) by convention unless at root.
			if strings.HasPrefix(base, ".") && path != absRoot {
				return filepath.SkipDir
			}
			return nil
		}
		// Regular file. Extension filter.
		ext := strings.ToLower(filepath.Ext(path))
		if _, ok := extSet[ext]; !ok {
			return nil
		}
		if filesScanned >= maxFilesScanned {
			return fs.SkipAll
		}
		filesScanned++
		// Symlink safety: resolve and check the target is inside absRoot.
		if lstat, lerr := os.Lstat(path); lerr == nil && (lstat.Mode()&os.ModeSymlink) != 0 {
			resolved, rerr := filepath.EvalSymlinks(path)
			if rerr != nil {
				return nil
			}
			absResolved, aerr := filepath.Abs(resolved)
			if aerr != nil {
				return nil
			}
			if !strings.HasPrefix(absResolved, absRoot+string(filepath.Separator)) && absResolved != absRoot {
				return nil // symlink escapes root
			}
			path = absResolved
		}
		body, rerr := os.ReadFile(path) //nolint:gosec // path is workspace-scoped
		if rerr != nil {
			return nil
		}
		text := string(body)
		total := 0
		for _, pat := range patterns {
			total += len(pat.FindAllStringIndex(text, -1))
		}
		if total == 0 {
			return nil
		}
		rel, _ := filepath.Rel(absRoot, path)
		if rel == "" {
			rel = path
		}
		// Cap per-file HitCount to reduce writer/definer bias: a file that
		// declares the symbol as a var + uses it 20 times shouldn't dominate
		// consumer files that just have 1-2 real reference sites. Empirically
		// on q11: writer files (constraint_outcomes_writer.go) had ~15 hits;
		// consumer files (dataset_builder.go, alert/rules.go) had 1-3 hits.
		// Capping at 3 flattens the bias so consumers can rank alongside
		// writers instead of always losing to them.
		const perFileHitCap = 3
		if total > perFileHitCap {
			total = perFileHitCap
		}
		if existing, ok := hits[path]; ok {
			existing.HitCount += total
			if existing.HitCount > perFileHitCap {
				existing.HitCount = perFileHitCap
			}
		} else {
			hits[path] = &grepHit{AbsPath: path, RelPath: rel, HitCount: total}
		}
		return nil
	}

	if err := filepath.WalkDir(absRoot, walkFn); err != nil && !errors.Is(err, fs.SkipAll) {
		return nil, err
	}
	if len(hits) == 0 {
		return nil, nil
	}
	out := make([]grepHit, 0, len(hits))
	for _, h := range hits {
		out = append(out, *h)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].HitCount != out[j].HitCount {
			return out[i].HitCount > out[j].HitCount
		}
		// Deterministic tie-break: shorter paths first (typically the more
		// canonical / less deeply-nested file wins on ties), then lexical.
		if len(out[i].RelPath) != len(out[j].RelPath) {
			return len(out[i].RelPath) < len(out[j].RelPath)
		}
		return out[i].RelPath < out[j].RelPath
	})
	if len(out) > topK {
		out = out[:topK]
	}
	return out, nil
}

// fetchReverseRefResults extracts candidate symbols from the query, greps the
// workspace, and looks up matching files as MemoryNodes in the given space.
// Returns []Candidate ready for injection via ApplyConcreteQuotaWithExtra
// (RETRIEVAL-LAYER-BALANCE-001 mechanism reused). Fail-open on any error.
func (s *Service) fetchReverseRefResults(ctx context.Context, spaceIDs []string, queryText string) []models.RetrieveResult {
	if s.driver == nil || queryText == "" || len(spaceIDs) == 0 {
		return nil
	}
	workspaceRoot := s.cfg.RetrievalReverseRefWorkspaceRoot
	if workspaceRoot == "" {
		return nil
	}
	symbols := extractCandidateSymbols(queryText, s.cfg.RetrievalReverseRefMinSymbolLen)
	if len(symbols) == 0 {
		return nil
	}
	// Bounded — never let one query kick off an unbounded number of regex
	// matches (each symbol × each file). The most-likely user-intent symbols
	// appear first (query text is left-to-right); take the first N.
	maxSymbols := 4
	if len(symbols) > maxSymbols {
		symbols = symbols[:maxSymbols]
	}
	allowExts := parseCSVList(s.cfg.RetrievalReverseRefExtensions)
	if len(allowExts) == 0 {
		allowExts = []string{".go", ".py", ".sql", ".md", ".yml", ".yaml", ".json", ".ts", ".tsx", ".js"}
	}
	excludeDirs := parseCSVList(s.cfg.RetrievalReverseRefExcludeDirs)
	if len(excludeDirs) == 0 {
		excludeDirs = []string{"node_modules", ".git", ".venv", "dist", "build", "vendor", ".mypy_cache"}
	}
	hits, err := grepReferences(workspaceRoot, symbols,
		allowExts, excludeDirs,
		s.cfg.RetrievalReverseRefMaxFilesScanned,
		s.cfg.RetrievalReverseRefTopK)
	if err != nil {
		slog.Warn("retrieval: reverse-ref grep failed (fail-open)", "error", err.Error())
		return nil
	}
	if len(hits) == 0 {
		slog.Debug("retrieval: reverse-ref no filesystem matches",
			"symbols", symbols,
			"workspace_root", workspaceRoot)
		return nil
	}
	// Look up each matched file as a MemoryNode by its relative path
	// (MDEMG ingest stores paths as `/relative/path.go`).
	paths := make([]string, 0, len(hits))
	for _, h := range hits {
		p := "/" + h.RelPath
		if strings.HasPrefix(h.RelPath, "/") {
			p = h.RelPath
		}
		paths = append(paths, p)
	}
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx) //nolint:errcheck
	cypher := `
	MATCH (n:MemoryNode)
	WHERE n.space_id IN $spaceIds
	  AND NOT coalesce(n.is_archived, false)
	  AND (n.path IN $paths OR
	       any(p IN $paths WHERE n.path STARTS WITH p + '#'))
	RETURN n.node_id AS nodeId, coalesce(n.name, '') AS name,
	       coalesce(n.path, '') AS path,
	       coalesce(n.summary, '') AS summary,
	       coalesce(n.role_type, '') AS roleType,
	       coalesce(n.obs_type, '') AS obsType,
	       coalesce(n.layer, 0) AS layer,
	       coalesce(n.confidence, 0.5) AS confidence`
	params := map[string]any{"spaceIds": spaceIDs, "paths": paths}
	out, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, txErr := tx.Run(ctx, cypher, params)
		if txErr != nil {
			return nil, txErr
		}
		// Pre-index hit ranks by relPath for score derivation.
		rankByPath := map[string]int{}
		for i, h := range hits {
			rankByPath[h.RelPath] = i
		}
		var results []models.RetrieveResult
		for res.Next(ctx) {
			rec := res.Record()
			getStr := func(k string) string {
				v, _ := rec.Get(k)
				if s, ok := v.(string); ok {
					return s
				}
				return ""
			}
			nodeID := getStr("nodeId")
			if nodeID == "" {
				continue
			}
			path := getStr("path")
			// Derive a score: rank 0 → 1.0, rank 1 → 0.9, etc. Cap at 0.1.
			// The quota promoter treats higher scores first when choosing which
			// extra to promote.
			rel := strings.TrimPrefix(path, "/")
			// Strip a symbol-anchor suffix like `#DatasetBuilder` since grep hits
			// were file-level.
			if idx := strings.Index(rel, "#"); idx >= 0 {
				rel = rel[:idx]
			}
			rank := rankByPath[rel]
			// Rank-derived score: rank 0 → 1.0, rank 1 → 0.9, …, floored at
			// a small positive so lower-ranked reverse-ref hits still sort
			// above zero-scored default results. This is a rank-derived
			// PRESENTATION score, not a threshold gate — the reverse-ref quota
			// promoter uses hit-order (input array position) for slot
			// selection, not this score. RRF-SCALE-001-safe.
			const reverseRefMinScore = 0.1
			score := 1.0 - 0.1*float64(rank)
			if score < reverseRefMinScore {
				score = reverseRefMinScore
			}
			layerVal, _ := rec.Get("layer")
			layer := 0
			switch v := layerVal.(type) {
			case int64:
				layer = int(v)
			case int:
				layer = v
			case float64:
				layer = int(v)
			}
			results = append(results, models.RetrieveResult{
				NodeID:   nodeID,
				Path:     path,
				Name:     getStr("name"),
				Summary:  getStr("summary"),
				RoleType: getStr("roleType"),
				ObsType:  getStr("obsType"),
				Layer:    layer,
				Score:    score,
			})
		}
		return results, res.Err()
	})
	if err != nil {
		slog.Warn("retrieval: reverse-ref Neo4j lookup failed (fail-open)", "error", err.Error())
		return nil
	}
	results, _ := out.([]models.RetrieveResult)
	slog.Info("retrieval: reverse-ref executed",
		"symbols", symbols,
		"grep_hits", len(hits),
		"node_matches", len(results))
	return results
}

func parseCSVList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}
