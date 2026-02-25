package guardrail

import (
	"regexp"
	"strings"
)

// DiffContext contains extracted symbols and keywords from a unified diff.
type DiffContext struct {
	AddedLines    []string
	RemovedLines  []string
	FilePaths     []string
	FunctionNames []string
	ImportPaths   []string
	Keywords      []string
	Summary       string // Truncated summary for embedding (max 2000 chars)
}

// Regex patterns for symbol extraction
var (
	// Go: func Name(, func (r *Receiver) Name(
	goFuncRe = regexp.MustCompile(`(?m)^[+-]\s*func\s+(?:\([^)]+\)\s+)?(\w+)\s*\(`)
	// Python: def name(
	pyFuncRe = regexp.MustCompile(`(?m)^[+-]\s*def\s+(\w+)\s*\(`)
	// JS/TS: function name(, const name =, let name =, export function name(
	jsFuncRe = regexp.MustCompile(`(?m)^[+-]\s*(?:export\s+)?(?:function|const|let|var)\s+(\w+)`)
	// Go imports: "path/to/pkg"
	goImportRe = regexp.MustCompile(`(?m)^[+-]\s*"([^"]+)"`)
	// Python imports: import pkg, from pkg import
	pyImportRe = regexp.MustCompile(`(?m)^[+-]\s*(?:from\s+(\S+)|import\s+(\S+))`)
	// JS imports: import ... from 'pkg', import ... from "pkg", require('pkg')
	jsImportRe = regexp.MustCompile(`(?m)^[+-]\s*(?:import\s+.*?\s+from\s+['"]([^'"]+)['"]|require\s*\(\s*['"]([^'"]+)['"]\s*\))`)
	// Hunk header: @@ -a,b +c,d @@ optional context
	hunkRe = regexp.MustCompile(`@@\s*-\d+(?:,\d+)?\s+\+\d+(?:,\d+)?\s*@@\s*(.*)`)
)

// parseDiff extracts symbols, keywords, and context from a unified diff.
func parseDiff(diff string, filesChanged []string) DiffContext {
	ctx := DiffContext{
		FilePaths: filesChanged,
	}

	if diff == "" {
		ctx.AddedLines = []string{}
		ctx.RemovedLines = []string{}
		ctx.FunctionNames = []string{}
		ctx.ImportPaths = []string{}
		ctx.Keywords = []string{}
		ctx.Summary = ""
		return ctx
	}

	lines := strings.Split(diff, "\n")
	seenFuncs := make(map[string]bool)
	seenImports := make(map[string]bool)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "+") && !strings.HasPrefix(trimmed, "+++") {
			ctx.AddedLines = append(ctx.AddedLines, trimmed[1:])
		} else if strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "---") {
			ctx.RemovedLines = append(ctx.RemovedLines, trimmed[1:])
		}
	}

	// Extract function names
	for _, re := range []*regexp.Regexp{goFuncRe, pyFuncRe, jsFuncRe} {
		matches := re.FindAllStringSubmatch(diff, -1)
		for _, m := range matches {
			name := m[1]
			if name != "" && !seenFuncs[name] {
				seenFuncs[name] = true
				ctx.FunctionNames = append(ctx.FunctionNames, name)
			}
		}
	}

	// Extract import paths
	for _, re := range []*regexp.Regexp{goImportRe, pyImportRe, jsImportRe} {
		matches := re.FindAllStringSubmatch(diff, -1)
		for _, m := range matches {
			for i := 1; i < len(m); i++ {
				path := m[i]
				if path != "" && !seenImports[path] {
					seenImports[path] = true
					ctx.ImportPaths = append(ctx.ImportPaths, path)
				}
			}
		}
	}

	// Extract hunk context (function/class names from @@ headers)
	hunkMatches := hunkRe.FindAllStringSubmatch(diff, -1)
	for _, m := range hunkMatches {
		if len(m) > 1 && m[1] != "" {
			hunkCtx := strings.TrimSpace(m[1])
			if hunkCtx != "" && !seenFuncs[hunkCtx] {
				ctx.Keywords = append(ctx.Keywords, hunkCtx)
			}
		}
	}

	// Add file paths as keywords for matching
	for _, fp := range filesChanged {
		parts := strings.Split(fp, "/")
		if len(parts) > 0 {
			ctx.Keywords = append(ctx.Keywords, parts[len(parts)-1])
		}
	}

	// Ensure non-nil slices
	if ctx.AddedLines == nil {
		ctx.AddedLines = []string{}
	}
	if ctx.RemovedLines == nil {
		ctx.RemovedLines = []string{}
	}
	if ctx.FunctionNames == nil {
		ctx.FunctionNames = []string{}
	}
	if ctx.ImportPaths == nil {
		ctx.ImportPaths = []string{}
	}
	if ctx.Keywords == nil {
		ctx.Keywords = []string{}
	}

	// Build summary for embedding
	ctx.Summary = buildDiffSummary(ctx)

	return ctx
}

// buildDiffSummary creates a text summary of the diff context for embedding.
func buildDiffSummary(ctx DiffContext) string {
	var sb strings.Builder

	if len(ctx.FilePaths) > 0 {
		sb.WriteString("Files: ")
		sb.WriteString(strings.Join(ctx.FilePaths, ", "))
		sb.WriteString("\n")
	}

	if len(ctx.FunctionNames) > 0 {
		sb.WriteString("Functions: ")
		sb.WriteString(strings.Join(ctx.FunctionNames, ", "))
		sb.WriteString("\n")
	}

	if len(ctx.ImportPaths) > 0 {
		sb.WriteString("Imports: ")
		sb.WriteString(strings.Join(ctx.ImportPaths, ", "))
		sb.WriteString("\n")
	}

	// Include some added lines for context
	if len(ctx.AddedLines) > 0 {
		sb.WriteString("Added:\n")
		maxLines := 20
		if len(ctx.AddedLines) < maxLines {
			maxLines = len(ctx.AddedLines)
		}
		for i := 0; i < maxLines; i++ {
			sb.WriteString("  + ")
			sb.WriteString(ctx.AddedLines[i])
			sb.WriteString("\n")
		}
	}

	return truncateString(sb.String(), 2000)
}
