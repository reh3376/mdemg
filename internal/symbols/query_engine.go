package symbols

import (
	"embed"
	"fmt"
	"log/slog"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

//go:embed queries
var queriesFS embed.FS

// RelationshipQuery holds a compiled tree-sitter query for relationship extraction.
type RelationshipQuery struct {
	Language      Language
	RelType       string        // "IMPORTS", "CALLS", "EXTENDS", "IMPLEMENTS"
	Tier          int           // 1=import, 2=inheritance, 3=call
	SourceCapture string        // @source capture name in query
	TargetCapture string        // @target capture name in query
	Query         *sitter.Query // Compiled tree-sitter query
}

// QueryEngine extracts relationships from ASTs using tree-sitter queries.
type QueryEngine struct {
	queries   map[Language][]RelationshipQuery
	languages map[Language]*sitter.Language
}

// NewQueryEngine creates a query engine by loading and compiling .scm files
// from the embedded queries directory. It receives the same languages map
// from NewParser() to avoid duplication.
func NewQueryEngine(languages map[Language]*sitter.Language) (*QueryEngine, error) {
	qe := &QueryEngine{
		queries:   make(map[Language][]RelationshipQuery),
		languages: languages,
	}

	// Define which .scm files to load for each language
	// Format: language/reltype.scm
	queryFiles := []struct {
		lang    Language
		relType string
		tier    int
		file    string
		source  string // @source capture
		target  string // @target capture
	}{
		// Go
		{LangGo, RelImports, 1, "queries/go/imports.scm", "source_file", "import_path"},
		{LangGo, RelCalls, 3, "queries/go/calls.scm", "caller", "callee"},
		// Python
		{LangPython, RelImports, 1, "queries/python/imports.scm", "source_file", "import_path"},
		{LangPython, RelExtends, 2, "queries/python/inheritance.scm", "child", "parent"},
		{LangPython, RelCalls, 3, "queries/python/calls.scm", "caller", "callee"},
		// TypeScript
		{LangTypeScript, RelImports, 1, "queries/typescript/imports.scm", "source_file", "import_path"},
		{LangTypeScript, RelExtends, 2, "queries/typescript/inheritance.scm", "child", "parent"},
		{LangTypeScript, RelImplements, 2, "queries/typescript/implements.scm", "child", "interface"},
		{LangTypeScript, RelCalls, 3, "queries/typescript/calls.scm", "caller", "callee"},
		// Rust
		{LangRust, RelImports, 1, "queries/rust/imports.scm", "source_file", "import_path"},
		{LangRust, RelImplements, 2, "queries/rust/inheritance.scm", "impl_type", "trait"},
		{LangRust, RelCalls, 3, "queries/rust/calls.scm", "caller", "callee"},
		// Java
		{LangJava, RelImports, 1, "queries/java/imports.scm", "source_file", "import_path"},
		{LangJava, RelExtends, 2, "queries/java/inheritance.scm", "child", "parent"},
		{LangJava, RelCalls, 3, "queries/java/calls.scm", "caller", "callee"},
		// C
		{LangC, RelImports, 1, "queries/c/imports.scm", "source_file", "import_path"},
		{LangC, RelCalls, 3, "queries/c/calls.scm", "caller", "callee"},
		// C++
		{LangCPP, RelImports, 1, "queries/cpp/imports.scm", "source_file", "import_path"},
		{LangCPP, RelExtends, 2, "queries/cpp/inheritance.scm", "child", "parent"},
		{LangCPP, RelCalls, 3, "queries/cpp/calls.scm", "caller", "callee"},
	}

	for _, qf := range queryFiles {
		tsLang, ok := languages[qf.lang]
		if !ok {
			continue // Skip languages without grammar loaded
		}

		content, err := queriesFS.ReadFile(qf.file)
		if err != nil {
			slog.Warn("query file not found, skipping", "file", qf.file)
			continue
		}

		query, err := sitter.NewQuery(content, tsLang)
		if err != nil {
			return nil, fmt.Errorf("failed to compile query %s: %w", qf.file, err)
		}

		qe.queries[qf.lang] = append(qe.queries[qf.lang], RelationshipQuery{
			Language:      qf.lang,
			RelType:       qf.relType,
			Tier:          qf.tier,
			SourceCapture: qf.source,
			TargetCapture: qf.target,
			Query:         query,
		})
	}

	return qe, nil
}

// ExtractRelationships runs all queries for the given language against the AST
// and returns discovered relationships.
func (qe *QueryEngine) ExtractRelationships(lang Language, root *sitter.Node, content []byte, filePath string) []Relationship {
	if qe == nil {
		return nil
	}

	queries, ok := qe.queries[lang]
	if !ok {
		return nil
	}

	var rels []Relationship
	for _, rq := range queries {
		cursor := sitter.NewQueryCursor()
		cursor.Exec(rq.Query, root)

		callCount := 0
		maxCalls := 50 // Cap per function to prevent noise

		for {
			match, ok := cursor.NextMatch()
			if !ok {
				break
			}
			match = cursor.FilterPredicates(match, content)

			var source, target string
			var line int

			for _, capture := range match.Captures {
				captureName := rq.Query.CaptureNameForId(capture.Index)
				nodeText := capture.Node.Content(content)

				switch captureName {
				case rq.SourceCapture:
					source = nodeText
					line = int(capture.Node.StartPoint().Row) + 1 // 0-indexed to 1-indexed
				case rq.TargetCapture:
					target = nodeText
					if line == 0 {
						line = int(capture.Node.StartPoint().Row) + 1
					}
				}
			}

			if source == "" || target == "" {
				continue
			}

			// Clean import paths (remove quotes)
			if rq.RelType == RelImports {
				target = strings.Trim(target, "\"'`")
				source = filePath // For imports, source is the file
			}

			// Cap calls per function
			if rq.RelType == RelCalls {
				callCount++
				if callCount > maxCalls {
					continue
				}
			}

			rels = append(rels, Relationship{
				Source:     source,
				Relation:   rq.RelType,
				Target:     target,
				SourceFile: filePath,
				Line:       line,
				Confidence: 0.9, // Tree-sitter AST queries are high confidence
				Tier:       rq.Tier,
			})
		}
	}

	return rels
}
