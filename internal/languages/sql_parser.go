package languages

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&SQLParser{})
}

// SQLParser implements LanguageParser for SQL files
type SQLParser struct{}

func (p *SQLParser) Name() string {
	return "sql"
}

func (p *SQLParser) Extensions() []string {
	return []string{".sql"}
}

func (p *SQLParser) CanParse(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".sql")
}

func (p *SQLParser) IsTestFile(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.Contains(pathLower, "/test/") ||
		strings.Contains(pathLower, "/tests/") ||
		strings.Contains(pathLower, "_test.sql") ||
		strings.Contains(pathLower, "/fixtures/")
}

func (p *SQLParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	var elements []CodeElement

	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)
	contentUpper := strings.ToUpper(content)

	// Extract SQL objects
	tables := p.extractTables(content)
	views := p.extractViews(content)
	procedures := p.extractProcedures(content)
	functions := p.extractFunctions(content)
	triggers := p.extractTriggers(content)
	indexes := p.extractIndexes(content)

	// Detect SQL dialect
	dialect := p.detectDialect(content)

	// Detect file type (migration, seed, etc.)
	fileType := p.detectFileType(fileName, content)

	// Build content for embedding
	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("SQL file: %s\n", fileName))
	contentBuilder.WriteString(fmt.Sprintf("Dialect: %s\n", dialect))
	contentBuilder.WriteString(fmt.Sprintf("Type: %s\n", fileType))

	if len(tables) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Tables: %s\n", strings.Join(uniqueStrings(tables), ", ")))
	}
	if len(views) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Views: %s\n", strings.Join(uniqueStrings(views), ", ")))
	}
	if len(procedures) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Procedures: %s\n", strings.Join(uniqueStrings(procedures), ", ")))
	}
	if len(functions) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Functions: %s\n", strings.Join(uniqueStrings(functions), ", ")))
	}
	if len(triggers) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Triggers: %s\n", strings.Join(uniqueStrings(triggers), ", ")))
	}
	if len(indexes) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Indexes: %s\n", strings.Join(uniqueStrings(indexes), ", ")))
	}

	// Count statement types
	selectCount := strings.Count(contentUpper, "SELECT ")
	insertCount := strings.Count(contentUpper, "INSERT ")
	updateCount := strings.Count(contentUpper, "UPDATE ")
	deleteCount := strings.Count(contentUpper, "DELETE ")

	if selectCount+insertCount+updateCount+deleteCount > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Statements: SELECT=%d, INSERT=%d, UPDATE=%d, DELETE=%d\n",
			selectCount, insertCount, updateCount, deleteCount))
	}

	// Include actual code content
	contentBuilder.WriteString("\n--- SQL ---\n")
	contentBuilder.WriteString(TruncateContent(content, 4000))

	// Detect cross-cutting concerns
	concerns := DetectConcerns(relPath, content)
	tags := []string{"sql", dialect, fileType}
	tags = append(tags, concerns...)

	// Extract symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	// Determine element name - use parent dir for migrations to avoid duplicates
	// (Prisma migrations are all named migration.sql in different timestamped folders)
	elementName := fileName
	if fileType == "migration" && fileName == "migration.sql" {
		parentDir := filepath.Dir(relPath)
		migrationName := filepath.Base(parentDir)
		if migrationName != "" && migrationName != "." {
			elementName = migrationName
		}
	}

	// Add file-level element
	elements = append(elements, CodeElement{
		Name:     elementName,
		Kind:     fileType,
		Path:     "/" + relPath,
		Content:  contentBuilder.String(),
		Package:  "database",
		FilePath: relPath,
		Tags:     tags,
		Concerns: concerns,
		Symbols:  symbols,
	})

	// Add tables as separate elements
	for _, table := range uniqueStrings(tables) {
		elements = append(elements, CodeElement{
			Name:     table,
			Kind:     "table",
			Path:     fmt.Sprintf("/%s#%s", relPath, table),
			Content:  fmt.Sprintf("SQL table '%s' defined in %s", table, fileName),
			Package:  "database",
			FilePath: relPath,
			Tags:     append([]string{"sql", "table", dialect}, concerns...),
			Concerns: concerns,
		})
	}

	// Add views as separate elements
	for _, view := range uniqueStrings(views) {
		elements = append(elements, CodeElement{
			Name:     view,
			Kind:     "view",
			Path:     fmt.Sprintf("/%s#%s", relPath, view),
			Content:  fmt.Sprintf("SQL view '%s' defined in %s", view, fileName),
			Package:  "database",
			FilePath: relPath,
			Tags:     append([]string{"sql", "view", dialect}, concerns...),
			Concerns: concerns,
		})
	}

	// Add procedures as separate elements
	for _, proc := range uniqueStrings(procedures) {
		elements = append(elements, CodeElement{
			Name:     proc,
			Kind:     "procedure",
			Path:     fmt.Sprintf("/%s#%s", relPath, proc),
			Content:  fmt.Sprintf("SQL stored procedure '%s' defined in %s", proc, fileName),
			Package:  "database",
			FilePath: relPath,
			Tags:     append([]string{"sql", "procedure", dialect}, concerns...),
			Concerns: concerns,
		})
	}

	return elements, nil
}

func (p *SQLParser) extractTables(content string) []string {
	// CREATE TABLE name
	pattern := `(?i)CREATE\s+(?:TEMP(?:ORARY)?\s+)?TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?(?:[\w.]+\.)?(\w+)`
	return FindAllMatches(content, pattern)
}

func (p *SQLParser) extractViews(content string) []string {
	// CREATE VIEW name
	pattern := `(?i)CREATE\s+(?:OR\s+REPLACE\s+)?(?:TEMP(?:ORARY)?\s+)?(?:MATERIALIZED\s+)?VIEW\s+(?:IF\s+NOT\s+EXISTS\s+)?(?:[\w.]+\.)?(\w+)`
	return FindAllMatches(content, pattern)
}

func (p *SQLParser) extractProcedures(content string) []string {
	// CREATE PROCEDURE name
	pattern := `(?i)CREATE\s+(?:OR\s+REPLACE\s+)?PROC(?:EDURE)?\s+(?:[\w.]+\.)?(\w+)`
	return FindAllMatches(content, pattern)
}

func (p *SQLParser) extractFunctions(content string) []string {
	// CREATE FUNCTION name
	pattern := `(?i)CREATE\s+(?:OR\s+REPLACE\s+)?FUNCTION\s+(?:[\w.]+\.)?(\w+)`
	return FindAllMatches(content, pattern)
}

func (p *SQLParser) extractTriggers(content string) []string {
	// CREATE TRIGGER name
	pattern := `(?i)CREATE\s+(?:OR\s+REPLACE\s+)?TRIGGER\s+(?:[\w.]+\.)?(\w+)`
	return FindAllMatches(content, pattern)
}

func (p *SQLParser) extractIndexes(content string) []string {
	// CREATE INDEX name
	pattern := `(?i)CREATE\s+(?:UNIQUE\s+)?INDEX\s+(?:IF\s+NOT\s+EXISTS\s+)?(?:CONCURRENTLY\s+)?(\w+)`
	return FindAllMatches(content, pattern)
}

func (p *SQLParser) detectDialect(content string) string {
	contentUpper := strings.ToUpper(content)

	// Check for PostgreSQL-specific syntax
	if strings.Contains(contentUpper, "::") ||
		strings.Contains(contentUpper, "RETURNING") ||
		strings.Contains(contentUpper, "SERIAL") ||
		strings.Contains(contentUpper, "ILIKE") {
		return "postgresql"
	}

	// Check for MySQL-specific syntax
	if strings.Contains(contentUpper, "AUTO_INCREMENT") ||
		strings.Contains(contentUpper, "ENGINE=") ||
		strings.Contains(content, "``") {
		return "mysql"
	}

	// Check for SQL Server-specific syntax
	if strings.Contains(contentUpper, "IDENTITY(") ||
		strings.Contains(contentUpper, "NVARCHAR") ||
		strings.Contains(contentUpper, "GO\n") ||
		strings.Contains(contentUpper, "[") {
		return "sqlserver"
	}

	// Check for SQLite-specific syntax
	if strings.Contains(contentUpper, "AUTOINCREMENT") ||
		strings.Contains(contentUpper, "SQLITE") {
		return "sqlite"
	}

	// Check for Oracle-specific syntax
	if strings.Contains(contentUpper, "VARCHAR2") ||
		strings.Contains(contentUpper, "NUMBER(") ||
		strings.Contains(contentUpper, "DBMS_") {
		return "oracle"
	}

	return "sql"
}

func (p *SQLParser) detectFileType(fileName string, content string) string {
	fileNameLower := strings.ToLower(fileName)
	contentUpper := strings.ToUpper(content)

	// Check filename patterns
	if strings.Contains(fileNameLower, "migration") ||
		regexp.MustCompile(`^\d{14}`).MatchString(fileName) ||
		regexp.MustCompile(`^V\d+`).MatchString(fileName) {
		return "migration"
	}

	if strings.Contains(fileNameLower, "seed") ||
		strings.Contains(fileNameLower, "fixture") {
		return "seed"
	}

	if strings.Contains(fileNameLower, "schema") {
		return "schema"
	}

	// Check content patterns
	if strings.Contains(contentUpper, "CREATE TABLE") ||
		strings.Contains(contentUpper, "ALTER TABLE") {
		if strings.Contains(contentUpper, "INSERT") {
			return "seed"
		}
		return "schema"
	}

	if strings.Contains(contentUpper, "INSERT") &&
		!strings.Contains(contentUpper, "CREATE") {
		return "seed"
	}

	return "query"
}

func (p *SQLParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	// Regex patterns for SQL constructs
	tablePattern := regexp.MustCompile(`(?i)^\s*CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?(?:[\w.]+\.)?(\w+)\s*\(`)
	columnPattern := regexp.MustCompile(`(?i)^\s*(\w+)\s+([\w()]+(?:\s*\([^)]*\))?)`)
	defaultPattern := regexp.MustCompile(`(?i)DEFAULT\s+('[^']*'|[\w()]+)`)
	indexPattern := regexp.MustCompile(`(?i)^\s*CREATE\s+(?:UNIQUE\s+)?INDEX\s+(?:IF\s+NOT\s+EXISTS\s+)?(?:CONCURRENTLY\s+)?(\w+)\s+ON\s+(\w+)`)
	viewPattern := regexp.MustCompile(`(?i)^\s*CREATE\s+(?:OR\s+REPLACE\s+)?(?:TEMP(?:ORARY)?\s+)?(?:MATERIALIZED\s+)?VIEW\s+(?:IF\s+NOT\s+EXISTS\s+)?(?:[\w.]+\.)?(\w+)`)
	functionPattern := regexp.MustCompile(`(?i)^\s*CREATE\s+(?:OR\s+REPLACE\s+)?FUNCTION\s+(?:[\w.]+\.)?(\w+)`)
	triggerPattern := regexp.MustCompile(`(?i)^\s*CREATE\s+(?:OR\s+REPLACE\s+)?TRIGGER\s+(?:[\w.]+\.)?(\w+)`)
	enumPattern := regexp.MustCompile(`(?i)^\s*CREATE\s+TYPE\s+(\w+)\s+AS\s+ENUM`)
	sequencePattern := regexp.MustCompile(`(?i)^\s*CREATE\s+SEQUENCE\s+(?:IF\s+NOT\s+EXISTS\s+)?(\w+)`)

	var currentTable string
	inTableDef := false
	parenDepth := 0

	for lineNum, line := range lines {
		lineNo := lineNum + 1
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}

		// Check for table start
		if matches := tablePattern.FindStringSubmatch(line); matches != nil {
			tableName := matches[1]
			currentTable = tableName
			inTableDef = true
			parenDepth = strings.Count(line, "(") - strings.Count(line, ")")

			symbols = append(symbols, Symbol{
				Name:     tableName,
				Type:     "table",
				Line:     lineNo,
				Exported: true,
				Language: "sql",
			})
			continue
		}

		// Process table columns
		if inTableDef && currentTable != "" {
			parenDepth += strings.Count(line, "(") - strings.Count(line, ")")

			// Check for column definition
			if matches := columnPattern.FindStringSubmatch(trimmed); matches != nil {
				colName := matches[1]
				colType := matches[2]

				// Skip keywords
				if !isKeyword(colName) {
					// Check for DEFAULT value and PRIMARY KEY
					var defaultVal string
					var docComment string
					upperTrimmed := strings.ToUpper(trimmed)
					if strings.Contains(upperTrimmed, "PRIMARY KEY") {
						docComment = "PRIMARY KEY"
					}
					if defMatch := defaultPattern.FindStringSubmatch(trimmed); defMatch != nil {
						defaultVal = strings.Trim(defMatch[1], "'")
						if docComment != "" {
							docComment += " DEFAULT " + defMatch[1]
						} else {
							docComment = "DEFAULT " + defMatch[1]
						}
					}

					symbols = append(symbols, Symbol{
						Name:           fmt.Sprintf("%s.%s", currentTable, colName),
						Type:           "column",
						TypeAnnotation: colType,
						Line:           lineNo,
						Parent:         currentTable,
						Value:          defaultVal,
						DocComment:     docComment,
						Exported:       true,
						Language:       "sql",
					})
				}
			}

			// End of table definition
			if parenDepth <= 0 || strings.HasSuffix(trimmed, ";") {
				inTableDef = false
				currentTable = ""
			}
			continue
		}

		// Check for index
		if matches := indexPattern.FindStringSubmatch(line); matches != nil {
			indexName := matches[1]
			tableName := matches[2]
			symbols = append(symbols, Symbol{
				Name:     indexName,
				Type:     "index",
				Line:     lineNo,
				Parent:   tableName,
				Exported: true,
				Language: "sql",
			})
			continue
		}

		// Check for view
		if matches := viewPattern.FindStringSubmatch(line); matches != nil {
			viewName := matches[1]
			symbols = append(symbols, Symbol{
				Name:     viewName,
				Type:     "view",
				Line:     lineNo,
				Exported: true,
				Language: "sql",
			})
			continue
		}

		// Check for function
		if matches := functionPattern.FindStringSubmatch(line); matches != nil {
			funcName := matches[1]
			// Try to detect return type
			var signature string
			if strings.Contains(strings.ToUpper(line), "RETURNS TRIGGER") {
				signature = "RETURNS TRIGGER"
			} else if strings.Contains(strings.ToUpper(line), "RETURNS TABLE") {
				signature = "RETURNS TABLE"
			}
			symbols = append(symbols, Symbol{
				Name:      funcName,
				Type:      "function",
				Signature: signature,
				Line:      lineNo,
				Exported:  true,
				Language:  "sql",
			})
			continue
		}

		// Check for trigger
		if matches := triggerPattern.FindStringSubmatch(line); matches != nil {
			triggerName := matches[1]
			symbols = append(symbols, Symbol{
				Name:     triggerName,
				Type:     "trigger",
				Line:     lineNo,
				Exported: true,
				Language: "sql",
			})
			continue
		}

		// Check for enum
		if matches := enumPattern.FindStringSubmatch(line); matches != nil {
			enumName := matches[1]
			symbols = append(symbols, Symbol{
				Name:     enumName,
				Type:     "enum",
				Line:     lineNo,
				Exported: true,
				Language: "sql",
			})
			continue
		}

		// Check for sequence
		if matches := sequencePattern.FindStringSubmatch(line); matches != nil {
			seqName := matches[1]
			symbols = append(symbols, Symbol{
				Name:     seqName,
				Type:     "constant",
				Line:     lineNo,
				Exported: true,
				Language: "sql",
			})
			continue
		}
	}

	return symbols
}

func isKeyword(s string) bool {
	keywords := map[string]bool{
		"PRIMARY": true, "KEY": true, "FOREIGN": true, "REFERENCES": true,
		"UNIQUE": true, "INDEX": true, "CONSTRAINT": true, "CHECK": true,
	}
	return keywords[strings.ToUpper(s)]
}
