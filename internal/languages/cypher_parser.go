package languages

import (
	"bufio"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&CypherParser{})
}

// CypherParser implements LanguageParser for Cypher query files (Neo4j)
type CypherParser struct{}

func (p *CypherParser) Name() string {
	return "cypher"
}

func (p *CypherParser) Extensions() []string {
	return []string{".cypher", ".cql"}
}

func (p *CypherParser) CanParse(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.HasSuffix(pathLower, ".cypher") || strings.HasSuffix(pathLower, ".cql")
}

func (p *CypherParser) IsTestFile(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.Contains(pathLower, "/fixtures/") ||
		strings.Contains(pathLower, "/testdata/") ||
		strings.Contains(pathLower, "_test.")
}

func (p *CypherParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)

	// Detect file type
	fileKind := p.detectFileKind(content)

	// Build summary content
	var contentBuilder strings.Builder
	contentBuilder.WriteString("Cypher file: " + fileName + "\n")
	contentBuilder.WriteString("Type: " + fileKind + "\n")

	// Extract symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	// Summarize labels and relationships
	labels := p.extractLabels(content)
	rels := p.extractRelationshipTypes(content)
	if len(labels) > 0 {
		contentBuilder.WriteString("Labels: " + strings.Join(labels, ", ") + "\n")
	}
	if len(rels) > 0 {
		contentBuilder.WriteString("Relationships: " + strings.Join(rels, ", ") + "\n")
	}

	contentBuilder.WriteString("\n--- Content ---\n")
	contentBuilder.WriteString(TruncateContent(content, 4000))

	// Detect concerns
	concerns := DetectConcerns(relPath, content)
	tags := []string{"cypher", "neo4j", "graph", fileKind}
	tags = append(tags, concerns...)

	element := CodeElement{
		Name:        fileName,
		Kind:        fileKind,
		Path:        "/" + relPath,
		Content:     contentBuilder.String(),
		Package:     "database",
		FilePath:    relPath,
		Tags:        tags,
		Concerns:    concerns,
		Symbols:     symbols,
		ElementKind: "file",
	}

	return []CodeElement{element}, nil
}

func (p *CypherParser) detectFileKind(content string) string {
	contentUpper := strings.ToUpper(content)

	if strings.Contains(contentUpper, "CREATE CONSTRAINT") || strings.Contains(contentUpper, "CREATE INDEX") {
		return "cypher-schema"
	}
	if strings.Contains(contentUpper, "CREATE (") || strings.Contains(contentUpper, "MERGE (") {
		return "cypher-data"
	}
	if strings.Contains(contentUpper, "MATCH (") {
		return "cypher-query"
	}
	return "cypher"
}

// Regex patterns for Cypher parsing
var (
	// Node labels: (n:Label) or (:Label) - captures only the label after colon
	labelRegex = regexp.MustCompile(`\([\w]*:([\w]+)`)
	// Multiple labels: (:Label1:Label2) or (n:Label1:Label2) - captures all colons and labels
	// We'll filter out variable names in code
	multiLabelRegex = regexp.MustCompile(`\([\w]*:([\w]+(?::[\w]+)+)`)
	// Relationship types: -[:TYPE]-> or -[r:TYPE]-> or -[:TYPE {...}]-> or -[:TYPE*1..3]->
	relTypeRegex = regexp.MustCompile(`-\[[\w]*:([\w]+)[^\]]*\]->?`)
	// CREATE CONSTRAINT - Neo4j syntax: CREATE CONSTRAINT name [IF NOT EXISTS] FOR/ON
	constraintRegex = regexp.MustCompile(`(?i)CREATE\s+CONSTRAINT\s+(\w+)\s+(?:IF\s+NOT\s+EXISTS\s+)?(?:ON|FOR)`)
	// CREATE INDEX - Neo4j syntax: CREATE [type] INDEX name [IF NOT EXISTS] FOR/ON
	indexRegex = regexp.MustCompile(`(?i)CREATE\s+(?:BTREE\s+|FULLTEXT\s+|POINT\s+|RANGE\s+|TEXT\s+)?INDEX\s+(\w+)\s+(?:IF\s+NOT\s+EXISTS\s+)?(?:ON|FOR)`)
	// Property keys in {key: value}
	propertyRegex = regexp.MustCompile(`\{([^}]+)\}`)
	// CALL procedure
	callRegex = regexp.MustCompile(`(?i)CALL\s+([\w.]+)`)
)

func (p *CypherParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol
	seenLabels := make(map[string]bool)
	seenRels := make(map[string]bool)
	seenConstraints := make(map[string]bool)
	seenIndexes := make(map[string]bool)

	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}

		// Extract labels from this line
		for _, matches := range labelRegex.FindAllStringSubmatch(line, -1) {
			label := matches[1]
			if !seenLabels[label] {
				seenLabels[label] = true
				symbols = append(symbols, Symbol{
					Name:     label,
					Type:     "label", // Cypher node labels
					Line:     lineNum,
					Exported: true,
					Language: "cypher",
				})
			}
		}

		// Extract multi-labels
		for _, matches := range multiLabelRegex.FindAllStringSubmatch(line, -1) {
			labels := strings.Split(matches[1], ":")
			for _, label := range labels {
				if !seenLabels[label] {
					seenLabels[label] = true
					symbols = append(symbols, Symbol{
						Name:     label,
						Type:     "label",
						Line:     lineNum,
						Exported: true,
						Language: "cypher",
					})
				}
			}
		}

		// Extract relationship types
		for _, matches := range relTypeRegex.FindAllStringSubmatch(line, -1) {
			relType := matches[1]
			if !seenRels[relType] {
				seenRels[relType] = true
				symbols = append(symbols, Symbol{
					Name:     relType,
					Type:     "relationship_type",
					Line:     lineNum,
					Exported: true,
					Language: "cypher",
				})
			}
		}

		// Extract constraints
		if matches := constraintRegex.FindStringSubmatch(trimmed); matches != nil {
			name := matches[1]
			if name == "" {
				name = "unnamed_constraint_" + string(rune(lineNum))
			}
			if !seenConstraints[name] {
				seenConstraints[name] = true
				symbols = append(symbols, Symbol{
					Name:     name,
					Type:     "constraint",
					Line:     lineNum,
					Exported: true,
					Language: "cypher",
				})
			}
		}

		// Extract indexes
		if matches := indexRegex.FindStringSubmatch(trimmed); matches != nil {
			name := matches[1]
			if name == "" {
				name = "unnamed_index_" + string(rune(lineNum))
			}
			if !seenIndexes[name] {
				seenIndexes[name] = true
				symbols = append(symbols, Symbol{
					Name:     name,
					Type:     "index",
					Line:     lineNum,
					Exported: true,
					Language: "cypher",
				})
			}
		}

		// Extract procedure calls
		if matches := callRegex.FindStringSubmatch(trimmed); matches != nil {
			procName := matches[1]
			symbols = append(symbols, Symbol{
				Name:       procName,
				Type:       "function",
				Line:       lineNum,
				Exported:   true,
				DocComment: "procedure call",
				Language:   "cypher",
			})
		}
	}

	return symbols
}

func (p *CypherParser) extractLabels(content string) []string {
	seen := make(map[string]bool)
	var labels []string

	for _, matches := range labelRegex.FindAllStringSubmatch(content, -1) {
		label := matches[1]
		if !seen[label] {
			seen[label] = true
			labels = append(labels, label)
		}
	}

	return labels
}

func (p *CypherParser) extractRelationshipTypes(content string) []string {
	seen := make(map[string]bool)
	var types []string

	for _, matches := range relTypeRegex.FindAllStringSubmatch(content, -1) {
		relType := matches[1]
		if !seen[relType] {
			seen[relType] = true
			types = append(types, relType)
		}
	}

	return types
}
