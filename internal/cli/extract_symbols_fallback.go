// extract_symbols_fallback.go
// Fallback to config parsers when tree-sitter grammar is unavailable
// This file contains all fallback parser implementations for extract-symbols command

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// fallbackSymbol matches the output format expected by UPTS
type fallbackSymbol struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Line       int    `json:"line"`
	Exported   bool   `json:"exported"`
	Parent     string `json:"parent,omitempty"`
	Signature  string `json:"signature,omitempty"`
	Value      string `json:"value,omitempty"`
	DocComment string `json:"doc_comment,omitempty"`
}

// TryFallbackParser attempts to parse using config parsers
// Returns (symbols, handled, error)
func tryFallbackParser(filePath string) ([]fallbackSymbol, bool, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	base := strings.ToLower(filepath.Base(filePath))

	// Route to appropriate parser
	switch {
	case ext == ".yaml" || ext == ".yml":
		// Check if this is an OpenAPI spec
		content, err := os.ReadFile(filePath)
		if err == nil {
			contentStr := string(content)
			if strings.Contains(contentStr, "openapi:") || strings.Contains(contentStr, "swagger:") {
				return parseOpenAPI(filePath, contentStr)
			}
		}
		return parseYAML(filePath)
	case ext == ".toml":
		return parseTOML(filePath)
	case ext == ".json" || ext == ".jsonc":
		return parseJSON(filePath)
	case ext == ".env" || ext == ".ini" || ext == ".cfg" || ext == ".properties":
		return parseINI(filePath)
	case ext == ".sh" || ext == ".bash" || ext == ".zsh":
		return parseShell(filePath)
	case ext == ".sql":
		return parseSQL(filePath)
	case ext == ".cypher" || ext == ".cql":
		return parseCypher(filePath)
	case strings.HasPrefix(base, "dockerfile") || ext == ".dockerfile":
		return parseDockerfile(filePath)
	// New parsers for Phase 4 and Phase 5
	case ext == ".cs":
		return parseCSharp(filePath)
	case ext == ".kt" || ext == ".kts":
		return parseKotlin(filePath)
	case ext == ".tf" || ext == ".tfvars":
		return parseTerraform(filePath)
	case base == "makefile" || base == "gnumakefile" || ext == ".mk":
		return parseMakefile(filePath)
	case ext == ".proto":
		return parseProtobuf(filePath)
	case ext == ".graphql" || ext == ".gql":
		return parseGraphQL(filePath)
	case ext == ".md" || ext == ".markdown":
		return parseMarkdown(filePath)
	case ext == ".xml" || ext == ".xsd" || ext == ".xsl" || ext == ".xslt" || ext == ".wsdl" ||
		ext == ".svg" || ext == ".xhtml" || ext == ".plist" || ext == ".csproj" ||
		ext == ".vbproj" || ext == ".fsproj" || ext == ".vcxproj" || ext == ".props" ||
		ext == ".targets" || ext == ".nuspec" || ext == ".resx" || ext == ".xaml" ||
		ext == ".config" || ext == ".manifest":
		return parseXML(filePath)
	case ext == ".c" || ext == ".h":
		return parseC(filePath)
	case ext == ".cpp" || ext == ".cc" || ext == ".cxx" || ext == ".hpp" || ext == ".hh" || ext == ".hxx":
		return parseCPP(filePath)
	case ext == ".cu" || ext == ".cuh":
		return parseCUDA(filePath)
	default:
		return nil, false, nil // Not handled
	}
}

// ============================================================
// YAML Parser
// ============================================================

func parseYAML(filePath string) ([]fallbackSymbol, bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, true, err
	}

	var symbols []fallbackSymbol
	lines := strings.Split(string(content), "\n")

	// Track current section context
	var currentSection string
	sectionIndent := -1

	keyPattern := regexp.MustCompile(`^(\s*)([a-zA-Z_][a-zA-Z0-9_.-]*):\s*(.*)$`)
	anchorPattern := regexp.MustCompile(`&(\w+)`)

	for i, line := range lines {
		lineNum := i + 1

		// Skip comments and empty lines
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		match := keyPattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		indent := len(match[1])
		key := match[2]
		value := strings.TrimSpace(match[3])

		// Detect section vs value
		isSection := value == "" || strings.HasPrefix(value, "&") || strings.HasPrefix(value, "|") || strings.HasPrefix(value, ">")

		// Update section context
		if indent <= sectionIndent || sectionIndent == -1 {
			if isSection {
				currentSection = key
				sectionIndent = indent
			} else {
				currentSection = ""
				sectionIndent = -1
			}
		}

		// Check for anchor
		docComment := ""
		if anchorMatch := anchorPattern.FindStringSubmatch(value); anchorMatch != nil {
			docComment = "anchor: " + anchorMatch[1]
		}

		sym := fallbackSymbol{
			Name:     key,
			Line:     lineNum,
			Exported: true,
		}

		if isSection {
			sym.Type = "section"
			if currentSection != "" && key != currentSection {
				sym.Name = currentSection + "." + key
				sym.Parent = currentSection
			}
		} else {
			sym.Type = "constant"
			sym.Value = cleanYAMLValue(value)
			if currentSection != "" {
				sym.Name = currentSection + "." + key
				sym.Parent = currentSection
			}
		}

		if docComment != "" {
			sym.DocComment = docComment
		}

		symbols = append(symbols, sym)
	}

	return symbols, true, nil
}

func cleanYAMLValue(value string) string {
	// Remove quotes
	if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
		(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
		return value[1 : len(value)-1]
	}
	// Remove inline comments
	if idx := strings.Index(value, " #"); idx > 0 {
		return strings.TrimSpace(value[:idx])
	}
	return value
}

// ============================================================
// TOML Parser
// ============================================================

func parseTOML(filePath string) ([]fallbackSymbol, bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, true, err
	}

	var symbols []fallbackSymbol
	lines := strings.Split(string(content), "\n")

	var currentSection string
	tablePattern := regexp.MustCompile(`^\[([^\]]+)\]$`)
	arrayTablePattern := regexp.MustCompile(`^\[\[([^\]]+)\]\]$`)
	keyValuePattern := regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_.-]*)\s*=\s*(.+)$`)

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Skip comments and empty lines
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Array of tables [[name]]
		if match := arrayTablePattern.FindStringSubmatch(trimmed); match != nil {
			currentSection = match[1]
			symbols = append(symbols, fallbackSymbol{
				Name:       currentSection,
				Type:       "section",
				Line:       lineNum,
				Exported:   true,
				DocComment: "array of tables",
			})
			continue
		}

		// Table [name]
		if match := tablePattern.FindStringSubmatch(trimmed); match != nil {
			currentSection = match[1]
			symbols = append(symbols, fallbackSymbol{
				Name:     currentSection,
				Type:     "section",
				Line:     lineNum,
				Exported: true,
			})
			continue
		}

		// Key = value
		if match := keyValuePattern.FindStringSubmatch(trimmed); match != nil {
			key := match[1]
			value := cleanTOMLValue(match[2])

			sym := fallbackSymbol{
				Name:     key,
				Type:     "constant",
				Line:     lineNum,
				Value:    value,
				Exported: true,
			}

			if currentSection != "" {
				sym.Name = currentSection + "." + key
				sym.Parent = currentSection
			}

			symbols = append(symbols, sym)
		}
	}

	return symbols, true, nil
}

func cleanTOMLValue(value string) string {
	value = strings.TrimSpace(value)
	// Remove quotes
	if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
		(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
		return value[1 : len(value)-1]
	}
	return value
}

// ============================================================
// JSON Parser
// ============================================================

func parseJSON(filePath string) ([]fallbackSymbol, bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, true, err
	}

	// Strip JSONC comments
	jsonContent := stripJSONComments(string(content))

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonContent), &data); err != nil {
		return nil, true, fmt.Errorf("JSON parse error: %w", err)
	}

	var symbols []fallbackSymbol
	lines := strings.Split(string(content), "\n")

	// Walk JSON structure
	walkJSON(data, "", &symbols, lines)

	return symbols, true, nil
}

func walkJSON(data map[string]interface{}, prefix string, symbols *[]fallbackSymbol, lines []string) {
	for key, value := range data {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		lineNum := findJSONKeyLine(key, lines)

		switch v := value.(type) {
		case map[string]interface{}:
			*symbols = append(*symbols, fallbackSymbol{
				Name:     fullKey,
				Type:     "section",
				Line:     lineNum,
				Parent:   prefix,
				Exported: true,
			})
			walkJSON(v, fullKey, symbols, lines)
		case []interface{}:
			*symbols = append(*symbols, fallbackSymbol{
				Name:     fullKey,
				Type:     "constant",
				Line:     lineNum,
				Parent:   prefix,
				Exported: true,
				Value:    fmt.Sprintf("[%d items]", len(v)),
			})
		default:
			*symbols = append(*symbols, fallbackSymbol{
				Name:     fullKey,
				Type:     "constant",
				Line:     lineNum,
				Parent:   prefix,
				Exported: true,
				Value:    fmt.Sprintf("%v", v),
			})
		}
	}
}

func findJSONKeyLine(key string, lines []string) int {
	pattern := regexp.MustCompile(fmt.Sprintf(`"(%s)"\s*:`, regexp.QuoteMeta(key)))
	for i, line := range lines {
		if pattern.MatchString(line) {
			return i + 1
		}
	}
	return 1
}

func stripJSONComments(content string) string {
	// Remove // comments but NOT inside strings
	// Simple approach: only remove // at start of line (with optional whitespace)
	singleLineComment := regexp.MustCompile(`^\s*//.*$`)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = singleLineComment.ReplaceAllString(line, "")
	}
	return strings.Join(lines, "\n")
}

// ============================================================
// INI/dotenv Parser
// ============================================================

func parseINI(filePath string) ([]fallbackSymbol, bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, true, err
	}

	var symbols []fallbackSymbol
	lines := strings.Split(string(content), "\n")

	var currentSection string
	sectionPattern := regexp.MustCompile(`^\[([^\]]+)\]$`)
	keyValuePattern := regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_.-]*)\s*=\s*(.*)$`)

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Skip comments and empty lines
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}

		// Section [name]
		if match := sectionPattern.FindStringSubmatch(trimmed); match != nil {
			currentSection = match[1]
			symbols = append(symbols, fallbackSymbol{
				Name:     currentSection,
				Type:     "section",
				Line:     lineNum,
				Exported: true,
			})
			continue
		}

		// Key = value
		if match := keyValuePattern.FindStringSubmatch(trimmed); match != nil {
			key := match[1]
			value := cleanINIValue(match[2])

			// Determine if exported (uppercase = env var style)
			exported := key == strings.ToUpper(key)

			sym := fallbackSymbol{
				Name:     key,
				Type:     "constant",
				Line:     lineNum,
				Value:    value,
				Exported: exported,
			}

			if currentSection != "" {
				sym.Name = currentSection + "." + key
				sym.Parent = currentSection
			}

			symbols = append(symbols, sym)
		}
	}

	return symbols, true, nil
}

func cleanINIValue(value string) string {
	value = strings.TrimSpace(value)
	// Remove quotes
	if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
		(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
		return value[1 : len(value)-1]
	}
	return value
}

// ============================================================
// Shell Parser
// ============================================================

func parseShell(filePath string) ([]fallbackSymbol, bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, true, err
	}

	var symbols []fallbackSymbol
	lines := strings.Split(string(content), "\n")

	exportPattern := regexp.MustCompile(`^export\s+([A-Za-z_][A-Za-z0-9_]*)=(.*)$`)
	readonlyPattern := regexp.MustCompile(`^readonly\s+([A-Za-z_][A-Za-z0-9_]*)=(.*)$`)
	funcPattern := regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_]*)\s*\(\)\s*\{?`)
	funcPattern2 := regexp.MustCompile(`^function\s+([a-zA-Z_][a-zA-Z0-9_]*)`)

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Skip comments and empty lines
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// export VAR=value
		if match := exportPattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "constant",
				Line:     lineNum,
				Value:    cleanShellValue(match[2]),
				Exported: true,
			})
			continue
		}

		// readonly VAR=value
		if match := readonlyPattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "constant",
				Line:     lineNum,
				Value:    cleanShellValue(match[2]),
				Exported: false,
			})
			continue
		}

		// function name() or name()
		if match := funcPattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "function",
				Line:     lineNum,
				Exported: true,
			})
			continue
		}

		if match := funcPattern2.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "function",
				Line:     lineNum,
				Exported: true,
			})
			continue
		}
	}

	return symbols, true, nil
}

func cleanShellValue(value string) string {
	value = strings.TrimSpace(value)
	// Remove quotes
	if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
		(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
		return value[1 : len(value)-1]
	}
	return value
}

// ============================================================
// Dockerfile Parser
// ============================================================

func parseDockerfile(filePath string) ([]fallbackSymbol, bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, true, err
	}

	var symbols []fallbackSymbol
	lines := strings.Split(string(content), "\n")

	argPattern := regexp.MustCompile(`^ARG\s+([A-Za-z_][A-Za-z0-9_]*)(?:=(.*))?$`)
	envPattern := regexp.MustCompile(`^ENV\s+([A-Za-z_][A-Za-z0-9_]*)(?:=(.*))?$`)
	fromPattern := regexp.MustCompile(`^FROM\s+(\S+)(?:\s+AS\s+(\S+))?`)
	exposePattern := regexp.MustCompile(`^EXPOSE\s+(\d+)`)
	volumePattern := regexp.MustCompile(`^VOLUME\s+(\S+)`)
	entrypointPattern := regexp.MustCompile(`^ENTRYPOINT\s+(.+)$`)
	cmdPattern := regexp.MustCompile(`^CMD\s+(.+)$`)

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Skip comments and empty lines
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// ARG name=value
		if match := argPattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "constant",
				Line:     lineNum,
				Value:    match[2],
				Exported: true,
			})
			continue
		}

		// ENV name=value
		if match := envPattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "constant",
				Line:     lineNum,
				Value:    match[2],
				Exported: true,
			})
			continue
		}

		// FROM image AS stage
		if match := fromPattern.FindStringSubmatch(trimmed); match != nil {
			stageName := match[2]
			if stageName == "" {
				stageName = "base"
			}
			symbols = append(symbols, fallbackSymbol{
				Name:       stageName,
				Type:       "section",
				Line:       lineNum,
				Exported:   true,
				DocComment: "FROM " + match[1],
			})
			continue
		}

		// EXPOSE port
		if match := exposePattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     "EXPOSE:" + match[1],
				Type:     "constant",
				Line:     lineNum,
				Value:    match[1],
				Exported: true,
			})
			continue
		}

		// VOLUME path
		if match := volumePattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     "VOLUME:" + match[1],
				Type:     "constant",
				Line:     lineNum,
				Value:    match[1],
				Exported: true,
			})
			continue
		}

		// ENTRYPOINT
		if match := entrypointPattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:      "ENTRYPOINT",
				Type:      "function",
				Line:      lineNum,
				Signature: cleanDockerCommand(match[1]),
				Exported:  true,
			})
			continue
		}

		// CMD
		if match := cmdPattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:      "CMD",
				Type:      "function",
				Line:      lineNum,
				Signature: cleanDockerCommand(match[1]),
				Exported:  true,
			})
			continue
		}
	}

	return symbols, true, nil
}

func cleanDockerCommand(cmd string) string {
	// Parse JSON array format ["cmd", "arg1", "arg2"]
	if strings.HasPrefix(cmd, "[") {
		var parts []string
		if err := json.Unmarshal([]byte(cmd), &parts); err == nil {
			return strings.Join(parts, " ")
		}
	}
	return cmd
}

// ============================================================
// SQL Parser
// ============================================================

func parseSQL(filePath string) ([]fallbackSymbol, bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, true, err
	}

	var symbols []fallbackSymbol
	lines := strings.Split(string(content), "\n")

	// Patterns (case-insensitive)
	tablePattern := regexp.MustCompile(`(?i)CREATE\s+TABLE\s+(\w+)`)
	indexPattern := regexp.MustCompile(`(?i)CREATE\s+INDEX\s+(\w+)\s+.*ON\s+(\w+)`)
	viewPattern := regexp.MustCompile(`(?i)CREATE\s+(?:OR\s+REPLACE\s+)?VIEW\s+(\w+)`)
	funcPattern := regexp.MustCompile(`(?i)CREATE\s+(?:OR\s+REPLACE\s+)?FUNCTION\s+(\w+)`)
	triggerPattern := regexp.MustCompile(`(?i)CREATE\s+TRIGGER\s+(\w+).*ON\s+(\w+)`)
	typePattern := regexp.MustCompile(`(?i)CREATE\s+TYPE\s+(\w+)\s+AS\s+ENUM`)
	seqPattern := regexp.MustCompile(`(?i)CREATE\s+SEQUENCE\s+(\w+)`)

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Skip comments
		if strings.HasPrefix(trimmed, "--") {
			continue
		}

		// CREATE TABLE
		if match := tablePattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "table",
				Line:     lineNum,
				Exported: true,
			})
			continue
		}

		// CREATE INDEX
		if match := indexPattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "index",
				Line:     lineNum,
				Parent:   match[2],
				Exported: true,
			})
			continue
		}

		// CREATE VIEW
		if match := viewPattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "view",
				Line:     lineNum,
				Exported: true,
			})
			continue
		}

		// CREATE FUNCTION
		if match := funcPattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "function",
				Line:     lineNum,
				Exported: true,
			})
			continue
		}

		// CREATE TRIGGER
		if match := triggerPattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "trigger",
				Line:     lineNum,
				Parent:   match[2],
				Exported: true,
			})
			continue
		}

		// CREATE TYPE AS ENUM
		if match := typePattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "enum",
				Line:     lineNum,
				Exported: true,
			})
			continue
		}

		// CREATE SEQUENCE
		if match := seqPattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "constant",
				Line:     lineNum,
				Exported: true,
			})
			continue
		}
	}

	// Also extract columns (need to re-scan within table blocks)
	symbols = append(symbols, extractSQLColumns(string(content), lines)...)

	return symbols, true, nil
}

func extractSQLColumns(content string, lines []string) []fallbackSymbol {
	var symbols []fallbackSymbol
	
	// Find CREATE TABLE blocks and extract columns with defaults
	tableBlockPattern := regexp.MustCompile(`(?is)CREATE\s+TABLE\s+(\w+)\s*\((.*?)\);`)
	columnPattern := regexp.MustCompile(`(?i)(\w+)\s+\w+.*?DEFAULT\s+([^,\n]+)`)

	for _, match := range tableBlockPattern.FindAllStringSubmatch(content, -1) {
		tableName := match[1]
		tableBody := match[2]
		
		// Find line number for table
		tableLine := 1
		for i, line := range lines {
			if strings.Contains(strings.ToLower(line), "create table "+strings.ToLower(tableName)) {
				tableLine = i + 1
				break
			}
		}

		// Extract columns with defaults
		for _, colMatch := range columnPattern.FindAllStringSubmatch(tableBody, -1) {
			colName := colMatch[1]
			defaultVal := strings.TrimSpace(colMatch[2])
			
			// Find approximate line for column
			colLine := tableLine
			for i := tableLine; i < len(lines) && i < tableLine+20; i++ {
				if strings.Contains(strings.ToLower(lines[i]), strings.ToLower(colName)) {
					colLine = i + 1
					break
				}
			}

			symbols = append(symbols, fallbackSymbol{
				Name:       tableName + "." + colName,
				Type:       "column",
				Line:       colLine,
				Parent:     tableName,
				Value:      cleanSQLDefault(defaultVal),
				DocComment: "DEFAULT " + defaultVal,
				Exported:   true,
			})
		}
	}

	return symbols
}

func cleanSQLDefault(value string) string {
	// Remove trailing comma or parenthesis
	value = strings.TrimRight(value, ",)")
	value = strings.TrimSpace(value)
	// Remove quotes
	if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
		return value[1 : len(value)-1]
	}
	return value
}

// ============================================================
// Cypher Parser
// ============================================================

func parseCypher(filePath string) ([]fallbackSymbol, bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, true, err
	}

	var symbols []fallbackSymbol
	lines := strings.Split(string(content), "\n")

	// Track seen items to avoid duplicates
	seenLabels := make(map[string]bool)
	seenRelTypes := make(map[string]bool)

	constraintPattern := regexp.MustCompile(`(?i)CREATE\s+CONSTRAINT\s+(\w+).*FOR\s+\(\w+:(\w+)\)`)
	indexPattern := regexp.MustCompile(`(?i)CREATE\s+INDEX\s+(\w+).*FOR\s+\(\w+:(\w+)\)`)
	createNodePattern := regexp.MustCompile(`CREATE\s+\(\w+:(\w+)`)
	relPattern := regexp.MustCompile(`-\[:(\w+)`)

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Skip comments
		if strings.HasPrefix(trimmed, "//") {
			continue
		}

		// Constraints
		if match := constraintPattern.FindStringSubmatch(trimmed); match != nil {
			constraintName := match[1]
			labelName := match[2]

			symbols = append(symbols, fallbackSymbol{
				Name:     constraintName,
				Type:     "constraint",
				Line:     lineNum,
				Parent:   labelName,
				Exported: true,
			})

			if !seenLabels[labelName] {
				seenLabels[labelName] = true
				symbols = append(symbols, fallbackSymbol{
					Name:     labelName,
					Type:     "label",
					Line:     lineNum,
					Exported: true,
				})
			}
			continue
		}

		// Indexes
		if match := indexPattern.FindStringSubmatch(trimmed); match != nil {
			indexName := match[1]
			labelName := match[2]

			symbols = append(symbols, fallbackSymbol{
				Name:     indexName,
				Type:     "index",
				Line:     lineNum,
				Parent:   labelName,
				Exported: true,
			})

			if !seenLabels[labelName] {
				seenLabels[labelName] = true
				symbols = append(symbols, fallbackSymbol{
					Name:     labelName,
					Type:     "label",
					Line:     lineNum,
					Exported: true,
				})
			}
			continue
		}

		// CREATE (n:Label {...})
		for _, match := range createNodePattern.FindAllStringSubmatch(trimmed, -1) {
			labelName := match[1]
			if !seenLabels[labelName] {
				seenLabels[labelName] = true
				symbols = append(symbols, fallbackSymbol{
					Name:     labelName,
					Type:     "label",
					Line:     lineNum,
					Exported: true,
				})
			}
		}

		// Relationship types -[:TYPE]-
		for _, match := range relPattern.FindAllStringSubmatch(trimmed, -1) {
			relType := match[1]
			if !seenRelTypes[relType] {
				seenRelTypes[relType] = true
				symbols = append(symbols, fallbackSymbol{
					Name:     relType,
					Type:     "relationship_type",
					Line:     lineNum,
					Exported: true,
				})
			}
		}
	}

	return symbols, true, nil
}

// ============================================================
// C# Parser
// ============================================================

func parseCSharp(filePath string) ([]fallbackSymbol, bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, true, err
	}

	var symbols []fallbackSymbol
	lines := strings.Split(string(content), "\n")

	// Patterns for C#
	namespacePattern := regexp.MustCompile(`^\s*namespace\s+([\w.]+)`)
	classPattern := regexp.MustCompile(`^\s*(public|private|protected|internal)?\s*(static|abstract|sealed|partial)?\s*(class|struct|interface|enum|record)\s+(\w+)`)
	methodPattern := regexp.MustCompile(`^\s*(public|private|protected|internal)?\s*(static|virtual|override|abstract|async)?\s*[\w<>\[\],\s]+\s+(\w+)\s*\(`)
	propertyPattern := regexp.MustCompile(`^\s*(public|private|protected|internal)?\s*(static|virtual|override|abstract)?\s*[\w<>\[\],\s]+\s+(\w+)\s*\{\s*(get|set)`)
	constPattern := regexp.MustCompile(`^\s*(public|private|protected|internal)?\s*(const|static\s+readonly)\s+[\w<>\[\],\s]+\s+(\w+)\s*=\s*(.+?);`)
	enumValuePattern := regexp.MustCompile(`^\s*(\w+)\s*(?:=\s*(\d+))?\s*,?\s*$`)
	attributePattern := regexp.MustCompile(`^\s*\[(\w+)`)

	var currentClass string
	var inEnum bool
	var braceDepth int
	var pendingAttribute string

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		// Track brace depth
		braceDepth += strings.Count(line, "{") - strings.Count(line, "}")

		// Capture attributes for next symbol
		if match := attributePattern.FindStringSubmatch(trimmed); match != nil {
			pendingAttribute = match[1]
			continue
		}

		// Namespace
		if match := namespacePattern.FindStringSubmatch(line); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "namespace",
				Line:     lineNum,
				Exported: true,
			})
			continue
		}

		// Class/struct/interface/enum/record
		if match := classPattern.FindStringSubmatch(line); match != nil {
			visibility := match[1]
			kind := match[3]
			name := match[4]
			exported := visibility == "public" || visibility == "internal" || visibility == ""

			symType := kind
			if kind == "record" {
				symType = "class"
			}

			sym := fallbackSymbol{
				Name:     name,
				Type:     symType,
				Line:     lineNum,
				Exported: exported,
			}
			if pendingAttribute != "" {
				sym.DocComment = "[" + pendingAttribute + "]"
				pendingAttribute = ""
			}
			symbols = append(symbols, sym)

			currentClass = name
			inEnum = kind == "enum"
			continue
		}

		// Enum values (only inside enum)
		if inEnum && braceDepth > 0 {
			if match := enumValuePattern.FindStringSubmatch(trimmed); match != nil {
				name := match[1]
				// Skip common false positives
				if name != "" && name != "{" && name != "}" && !strings.HasPrefix(name, "//") {
					sym := fallbackSymbol{
						Name:     name,
						Type:     "enum_value",
						Line:     lineNum,
						Parent:   currentClass,
						Exported: true,
					}
					if match[2] != "" {
						sym.Value = match[2]
					}
					symbols = append(symbols, sym)
				}
			}
			continue
		}

		// Constants
		if match := constPattern.FindStringSubmatch(line); match != nil {
			visibility := match[1]
			name := match[3]
			value := strings.TrimSpace(match[4])
			exported := visibility == "public" || visibility == "internal"

			sym := fallbackSymbol{
				Name:     name,
				Type:     "constant",
				Line:     lineNum,
				Value:    value,
				Exported: exported,
			}
			if currentClass != "" && braceDepth > 0 {
				sym.Parent = currentClass
			}
			symbols = append(symbols, sym)
			continue
		}

		// Properties
		if match := propertyPattern.FindStringSubmatch(line); match != nil {
			visibility := match[1]
			name := match[3]
			exported := visibility == "public" || visibility == "internal"

			sym := fallbackSymbol{
				Name:     name,
				Type:     "method",
				Line:     lineNum,
				Exported: exported,
			}
			if currentClass != "" && braceDepth > 0 {
				sym.Parent = currentClass
			}
			if pendingAttribute != "" {
				sym.DocComment = "[" + pendingAttribute + "]"
				pendingAttribute = ""
			}
			symbols = append(symbols, sym)
			continue
		}

		// Methods
		if match := methodPattern.FindStringSubmatch(line); match != nil {
			visibility := match[1]
			name := match[3]
			// Skip common false positives
			if name == "if" || name == "for" || name == "while" || name == "switch" || name == "catch" || name == "using" {
				continue
			}
			exported := visibility == "public" || visibility == "internal"

			sym := fallbackSymbol{
				Name:     name,
				Type:     "method",
				Line:     lineNum,
				Exported: exported,
			}
			if currentClass != "" && braceDepth > 0 {
				sym.Parent = currentClass
			}
			if pendingAttribute != "" {
				sym.DocComment = "[" + pendingAttribute + "]"
				pendingAttribute = ""
			}
			symbols = append(symbols, sym)
		}

		// Reset enum flag when leaving enum block
		if inEnum && braceDepth == 0 {
			inEnum = false
		}
	}

	return symbols, true, nil
}

// ============================================================
// Kotlin Parser
// ============================================================

func parseKotlin(filePath string) ([]fallbackSymbol, bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, true, err
	}

	var symbols []fallbackSymbol
	lines := strings.Split(string(content), "\n")

	// Patterns for Kotlin
	packagePattern := regexp.MustCompile(`^\s*package\s+([\w.]+)`)
	classPattern := regexp.MustCompile(`^\s*(public|private|protected|internal)?\s*(data|sealed|abstract|open|inner)?\s*(class|object|interface)\s+(\w+)`)
	enumClassPattern := regexp.MustCompile(`^\s*(public|private|protected|internal)?\s*enum\s+class\s+(\w+)`)
	funPattern := regexp.MustCompile(`^\s*(public|private|protected|internal)?\s*(suspend|inline|override|open)?\s*fun\s+(?:<[^>]+>\s*)?(\w+)\s*\(`)
	constValPattern := regexp.MustCompile(`^\s*(public|private|protected|internal)?\s*const\s+val\s+(\w+)\s*(?::\s*\w+)?\s*=\s*(.+)`)
	companionPattern := regexp.MustCompile(`^\s*companion\s+object`)
	typealiasPattern := regexp.MustCompile(`^\s*(public|private|protected|internal)?\s*typealias\s+(\w+)\s*=`)
	annotationPattern := regexp.MustCompile(`^\s*@(\w+)`)
	enumValuePattern := regexp.MustCompile(`^\s*(\w+)(?:\([^)]*\))?\s*,?\s*$`)

	var currentClass string
	var inEnum bool
	var braceDepth int
	var pendingAnnotation string

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		// Track brace depth
		braceDepth += strings.Count(line, "{") - strings.Count(line, "}")

		// Capture annotations for next symbol
		if match := annotationPattern.FindStringSubmatch(trimmed); match != nil {
			pendingAnnotation = match[1]
			continue
		}

		// Package (metadata, not a symbol)
		if match := packagePattern.FindStringSubmatch(line); match != nil {
			continue
		}

		// Enum class
		if match := enumClassPattern.FindStringSubmatch(line); match != nil {
			visibility := match[1]
			name := match[2]
			exported := visibility != "private" && visibility != "internal"

			sym := fallbackSymbol{
				Name:     name,
				Type:     "enum",
				Line:     lineNum,
				Exported: exported,
			}
			if pendingAnnotation != "" {
				sym.DocComment = "@" + pendingAnnotation
				pendingAnnotation = ""
			}
			symbols = append(symbols, sym)
			currentClass = name
			inEnum = true
			continue
		}

		// Class/object/interface
		if match := classPattern.FindStringSubmatch(line); match != nil {
			visibility := match[1]
			modifier := match[2]
			kind := match[3]
			name := match[4]
			exported := visibility != "private" && visibility != "internal"

			symType := kind
			if kind == "object" {
				symType = "class"
			}

			sym := fallbackSymbol{
				Name:     name,
				Type:     symType,
				Line:     lineNum,
				Exported: exported,
			}
			if modifier != "" {
				sym.DocComment = modifier
			}
			if pendingAnnotation != "" {
				if sym.DocComment != "" {
					sym.DocComment = "@" + pendingAnnotation + " " + sym.DocComment
				} else {
					sym.DocComment = "@" + pendingAnnotation
				}
				pendingAnnotation = ""
			}
			symbols = append(symbols, sym)
			currentClass = name
			inEnum = false
			continue
		}

		// Companion object
		if companionPattern.MatchString(line) {
			// Just track that we're in companion context
			continue
		}

		// Enum values (only inside enum)
		if inEnum && braceDepth > 0 && !strings.Contains(line, "fun ") && !strings.Contains(line, "val ") {
			if match := enumValuePattern.FindStringSubmatch(trimmed); match != nil {
				name := match[1]
				// Skip common false positives
				if name != "" && name != "{" && name != "}" && !strings.HasPrefix(name, "//") && name != ";" {
					symbols = append(symbols, fallbackSymbol{
						Name:     name,
						Type:     "enum_value",
						Line:     lineNum,
						Parent:   currentClass,
						Exported: true,
					})
				}
			}
			continue
		}

		// Typealias
		if match := typealiasPattern.FindStringSubmatch(line); match != nil {
			visibility := match[1]
			name := match[2]
			exported := visibility != "private" && visibility != "internal"

			symbols = append(symbols, fallbackSymbol{
				Name:     name,
				Type:     "type",
				Line:     lineNum,
				Exported: exported,
			})
			continue
		}

		// const val
		if match := constValPattern.FindStringSubmatch(line); match != nil {
			visibility := match[1]
			name := match[2]
			value := strings.TrimSpace(match[3])
			exported := visibility != "private" && visibility != "internal"

			sym := fallbackSymbol{
				Name:     name,
				Type:     "constant",
				Line:     lineNum,
				Value:    value,
				Exported: exported,
			}
			if currentClass != "" && braceDepth > 0 {
				sym.Parent = currentClass
			}
			symbols = append(symbols, sym)
			continue
		}

		// Functions
		if match := funPattern.FindStringSubmatch(line); match != nil {
			visibility := match[1]
			name := match[3]
			exported := visibility != "private" && visibility != "internal"

			sym := fallbackSymbol{
				Name:     name,
				Type:     "function",
				Line:     lineNum,
				Exported: exported,
			}
			if currentClass != "" && braceDepth > 0 {
				sym.Parent = currentClass
				sym.Type = "method"
			}
			if pendingAnnotation != "" {
				sym.DocComment = "@" + pendingAnnotation
				pendingAnnotation = ""
			}
			symbols = append(symbols, sym)
		}

		// Reset enum flag when leaving enum block
		if inEnum && braceDepth == 0 {
			inEnum = false
		}
	}

	return symbols, true, nil
}

// ============================================================
// Terraform/HCL Parser
// ============================================================

func parseTerraform(filePath string) ([]fallbackSymbol, bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, true, err
	}

	var symbols []fallbackSymbol
	lines := strings.Split(string(content), "\n")

	// Patterns for Terraform
	resourcePattern := regexp.MustCompile(`^\s*resource\s+"([^"]+)"\s+"([^"]+)"`)
	dataPattern := regexp.MustCompile(`^\s*data\s+"([^"]+)"\s+"([^"]+)"`)
	modulePattern := regexp.MustCompile(`^\s*module\s+"([^"]+)"`)
	providerPattern := regexp.MustCompile(`^\s*provider\s+"([^"]+)"`)
	variablePattern := regexp.MustCompile(`^\s*variable\s+"([^"]+)"`)
	outputPattern := regexp.MustCompile(`^\s*output\s+"([^"]+)"`)
	localsPattern := regexp.MustCompile(`^\s*locals\s*\{`)
	localKeyPattern := regexp.MustCompile(`^\s*(\w+)\s*=`)
	terraformPattern := regexp.MustCompile(`^\s*terraform\s*\{`)
	defaultPattern := regexp.MustCompile(`^\s*default\s*=\s*(.+)`)
	descriptionPattern := regexp.MustCompile(`^\s*description\s*=\s*"([^"]*)"`)
	valuePattern := regexp.MustCompile(`^\s*value\s*=\s*(.+)`)

	var inLocals bool
	var inVariable bool
	var inOutput bool
	var currentVarName string
	var currentVarLine int
	var currentDefault string
	var currentDescription string
	var braceDepth int

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Skip comments and empty lines
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}

		// Track brace depth
		openBraces := strings.Count(line, "{")
		closeBraces := strings.Count(line, "}")
		braceDepth += openBraces - closeBraces

		// Resource
		if match := resourcePattern.FindStringSubmatch(line); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[2],
				Type:     "section",
				Line:     lineNum,
				Value:    match[1],
				Exported: true,
			})
			continue
		}

		// Data source
		if match := dataPattern.FindStringSubmatch(line); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[2],
				Type:     "section",
				Line:     lineNum,
				Value:    match[1],
				Exported: true,
			})
			continue
		}

		// Module
		if match := modulePattern.FindStringSubmatch(line); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "section",
				Line:     lineNum,
				Exported: true,
			})
			continue
		}

		// Provider
		if match := providerPattern.FindStringSubmatch(line); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "section",
				Line:     lineNum,
				Exported: true,
			})
			continue
		}

		// Variable
		if match := variablePattern.FindStringSubmatch(line); match != nil {
			inVariable = true
			currentVarName = match[1]
			currentVarLine = lineNum
			currentDefault = ""
			currentDescription = ""
			continue
		}

		// Inside variable block
		if inVariable {
			if match := defaultPattern.FindStringSubmatch(line); match != nil {
				currentDefault = strings.TrimSpace(match[1])
			}
			if match := descriptionPattern.FindStringSubmatch(line); match != nil {
				currentDescription = match[1]
			}
			if closeBraces > 0 && braceDepth == 0 {
				sym := fallbackSymbol{
					Name:     currentVarName,
					Type:     "constant",
					Line:     currentVarLine,
					Value:    currentDefault,
					Exported: true,
				}
				if currentDescription != "" {
					sym.DocComment = currentDescription
				}
				symbols = append(symbols, sym)
				inVariable = false
			}
			continue
		}

		// Output
		if match := outputPattern.FindStringSubmatch(line); match != nil {
			inOutput = true
			currentVarName = match[1]
			currentVarLine = lineNum
			currentDefault = ""
			currentDescription = ""
			continue
		}

		// Inside output block
		if inOutput {
			if match := valuePattern.FindStringSubmatch(line); match != nil {
				currentDefault = strings.TrimSpace(match[1])
			}
			if match := descriptionPattern.FindStringSubmatch(line); match != nil {
				currentDescription = match[1]
			}
			if closeBraces > 0 && braceDepth == 0 {
				sym := fallbackSymbol{
					Name:     currentVarName,
					Type:     "constant",
					Line:     currentVarLine,
					Value:    currentDefault,
					Exported: true,
				}
				if currentDescription != "" {
					sym.DocComment = currentDescription
				}
				symbols = append(symbols, sym)
				inOutput = false
			}
			continue
		}

		// Locals block
		if localsPattern.MatchString(line) {
			inLocals = true
			continue
		}

		// Inside locals block
		if inLocals && braceDepth > 0 {
			if match := localKeyPattern.FindStringSubmatch(line); match != nil {
				symbols = append(symbols, fallbackSymbol{
					Name:     match[1],
					Type:     "constant",
					Line:     lineNum,
					Exported: true,
				})
			}
		}

		if inLocals && braceDepth == 0 {
			inLocals = false
		}

		// Terraform block
		if terraformPattern.MatchString(line) {
			symbols = append(symbols, fallbackSymbol{
				Name:     "terraform",
				Type:     "section",
				Line:     lineNum,
				Exported: true,
			})
		}
	}

	return symbols, true, nil
}

// ============================================================
// Makefile Parser
// ============================================================

func parseMakefile(filePath string) ([]fallbackSymbol, bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, true, err
	}

	var symbols []fallbackSymbol
	lines := strings.Split(string(content), "\n")

	// Patterns for Makefile
	targetPattern := regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_.-]*)\s*:([^=]|$)`)
	varPattern := regexp.MustCompile(`^([A-Z_][A-Z0-9_]*)\s*[:?+]?=\s*(.*)$`)
	phonyPattern := regexp.MustCompile(`^\.PHONY\s*:\s*(.+)$`)
	definePattern := regexp.MustCompile(`^define\s+(\w+)`)
	exportPattern := regexp.MustCompile(`^export\s+([A-Z_][A-Z0-9_]*)`)

	phonyTargets := make(map[string]bool)

	// First pass: collect .PHONY targets
	for _, line := range lines {
		if match := phonyPattern.FindStringSubmatch(line); match != nil {
			targets := strings.Fields(match[1])
			for _, t := range targets {
				phonyTargets[t] = true
			}
		}
	}

	var inDefine bool
	var defineStartLine int
	var defineName string

	for i, line := range lines {
		lineNum := i + 1

		// Skip comments and empty lines
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Skip recipe lines (tab-indented)
		if strings.HasPrefix(line, "\t") {
			continue
		}

		// Define block
		if match := definePattern.FindStringSubmatch(line); match != nil {
			inDefine = true
			defineName = match[1]
			defineStartLine = lineNum
			continue
		}

		if inDefine {
			if trimmed == "endef" {
				symbols = append(symbols, fallbackSymbol{
					Name:     defineName,
					Type:     "function",
					Line:     defineStartLine,
					Exported: true,
				})
				inDefine = false
			}
			continue
		}

		// Export
		if match := exportPattern.FindStringSubmatch(line); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "constant",
				Line:     lineNum,
				Exported: true,
			})
			continue
		}

		// Variables
		if match := varPattern.FindStringSubmatch(line); match != nil {
			name := match[1]
			value := strings.TrimSpace(match[2])
			symbols = append(symbols, fallbackSymbol{
				Name:     name,
				Type:     "constant",
				Line:     lineNum,
				Value:    value,
				Exported: true,
			})
			continue
		}

		// Targets
		if match := targetPattern.FindStringSubmatch(line); match != nil {
			name := match[1]
			// Skip special targets
			if strings.HasPrefix(name, ".") && name != ".PHONY" {
				continue
			}
			exported := phonyTargets[name]
			symbols = append(symbols, fallbackSymbol{
				Name:     name,
				Type:     "function",
				Line:     lineNum,
				Exported: exported,
			})
		}
	}

	return symbols, true, nil
}

// ============================================================
// Protocol Buffers Parser
// ============================================================

func parseProtobuf(filePath string) ([]fallbackSymbol, bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, true, err
	}

	var symbols []fallbackSymbol
	lines := strings.Split(string(content), "\n")

	// Patterns for Protocol Buffers
	packagePattern := regexp.MustCompile(`^\s*package\s+([\w.]+)\s*;`)
	optionPattern := regexp.MustCompile(`^\s*option\s+(\w+)\s*=\s*"([^"]+)"\s*;`)
	messagePattern := regexp.MustCompile(`^\s*message\s+(\w+)`)
	enumPattern := regexp.MustCompile(`^\s*enum\s+(\w+)`)
	servicePattern := regexp.MustCompile(`^\s*service\s+(\w+)`)
	rpcPattern := regexp.MustCompile(`^\s*rpc\s+(\w+)\s*\(([^)]*)\)\s*returns\s*\(([^)]*)\)`)
	fieldPattern := regexp.MustCompile(`^\s*(repeated|optional|required)?\s*([\w.]+)\s+(\w+)\s*=\s*(\d+)`)
	enumValuePattern := regexp.MustCompile(`^\s*(\w+)\s*=\s*(\d+)\s*;`)
	oneofPattern := regexp.MustCompile(`^\s*oneof\s+(\w+)`)

	var currentScope string
	var scopeStack []string
	var inEnum bool
	var braceDepth int

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Skip comments and empty lines
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}

		// Track brace depth
		openBraces := strings.Count(line, "{")
		closeBraces := strings.Count(line, "}")

		// Package
		if match := packagePattern.FindStringSubmatch(line); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "namespace",
				Line:     lineNum,
				Exported: true,
			})
			continue
		}

		// Option
		if match := optionPattern.FindStringSubmatch(line); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "constant",
				Line:     lineNum,
				Value:    match[2],
				Exported: true,
			})
			continue
		}

		// Message
		if match := messagePattern.FindStringSubmatch(line); match != nil {
			name := match[1]
			sym := fallbackSymbol{
				Name:     name,
				Type:     "struct",
				Line:     lineNum,
				Exported: true,
			}
			if currentScope != "" {
				sym.Parent = currentScope
			}
			symbols = append(symbols, sym)
			scopeStack = append(scopeStack, currentScope)
			currentScope = name
			inEnum = false
			braceDepth += openBraces
			continue
		}

		// Enum
		if match := enumPattern.FindStringSubmatch(line); match != nil {
			name := match[1]
			sym := fallbackSymbol{
				Name:     name,
				Type:     "enum",
				Line:     lineNum,
				Exported: true,
			}
			if currentScope != "" {
				sym.Parent = currentScope
			}
			symbols = append(symbols, sym)
			scopeStack = append(scopeStack, currentScope)
			currentScope = name
			inEnum = true
			braceDepth += openBraces
			continue
		}

		// Service
		if match := servicePattern.FindStringSubmatch(line); match != nil {
			name := match[1]
			symbols = append(symbols, fallbackSymbol{
				Name:     name,
				Type:     "interface",
				Line:     lineNum,
				Exported: true,
			})
			scopeStack = append(scopeStack, currentScope)
			currentScope = name
			inEnum = false
			braceDepth += openBraces
			continue
		}

		// RPC
		if match := rpcPattern.FindStringSubmatch(line); match != nil {
			name := match[1]
			input := strings.TrimSpace(match[2])
			output := strings.TrimSpace(match[3])
			signature := "(" + input + ") returns (" + output + ")"
			symbols = append(symbols, fallbackSymbol{
				Name:      name,
				Type:      "method",
				Line:      lineNum,
				Parent:    currentScope,
				Signature: signature,
				Exported:  true,
			})
			continue
		}

		// Oneof
		if match := oneofPattern.FindStringSubmatch(line); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "section",
				Line:     lineNum,
				Parent:   currentScope,
				Exported: true,
			})
			braceDepth += openBraces
			continue
		}

		// Enum values
		if inEnum && braceDepth > 0 {
			if match := enumValuePattern.FindStringSubmatch(line); match != nil {
				symbols = append(symbols, fallbackSymbol{
					Name:     match[1],
					Type:     "enum_value",
					Line:     lineNum,
					Parent:   currentScope,
					Value:    match[2],
					Exported: true,
				})
				continue
			}
		}

		// Fields (in messages)
		if !inEnum && braceDepth > 0 {
			if match := fieldPattern.FindStringSubmatch(line); match != nil {
				fieldType := match[2]
				fieldName := match[3]
				sym := fallbackSymbol{
					Name:     fieldName,
					Type:     "constant",
					Line:     lineNum,
					Parent:   currentScope,
					Value:    fieldType,
					Exported: true,
				}
				if match[1] != "" {
					sym.DocComment = match[1]
				}
				symbols = append(symbols, sym)
				continue
			}
		}

		// Handle closing braces - pop scope
		braceDepth += openBraces - closeBraces
		if closeBraces > 0 && braceDepth >= 0 {
			for j := 0; j < closeBraces && len(scopeStack) > 0; j++ {
				currentScope = scopeStack[len(scopeStack)-1]
				scopeStack = scopeStack[:len(scopeStack)-1]
				if currentScope == "" {
					inEnum = false
				}
			}
		}
	}

	return symbols, true, nil
}

// ============================================================
// GraphQL Parser
// ============================================================

func parseGraphQL(filePath string) ([]fallbackSymbol, bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, true, err
	}

	var symbols []fallbackSymbol
	lines := strings.Split(string(content), "\n")

	// Patterns for GraphQL
	typePattern := regexp.MustCompile(`^\s*type\s+(\w+)`)
	interfacePattern := regexp.MustCompile(`^\s*interface\s+(\w+)`)
	inputPattern := regexp.MustCompile(`^\s*input\s+(\w+)`)
	enumPattern := regexp.MustCompile(`^\s*enum\s+(\w+)`)
	unionPattern := regexp.MustCompile(`^\s*union\s+(\w+)`)
	scalarPattern := regexp.MustCompile(`^\s*scalar\s+(\w+)`)
	directivePattern := regexp.MustCompile(`^\s*directive\s+@(\w+)`)
	fieldPattern := regexp.MustCompile(`^\s*(\w+)(?:\([^)]*\))?\s*:\s*([\w\[\]!]+)`)
	enumValuePattern := regexp.MustCompile(`^\s*(\w+)\s*$`)
	extendPattern := regexp.MustCompile(`^\s*extend\s+type\s+(\w+)`)

	var currentScope string
	var scopeType string // "type", "interface", "input", "enum", etc.
	var braceDepth int

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Skip comments and empty lines
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Track brace depth
		openBraces := strings.Count(line, "{")
		closeBraces := strings.Count(line, "}")

		// Extend type
		if match := extendPattern.FindStringSubmatch(line); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:       "extend " + match[1],
				Type:       "type",
				Line:       lineNum,
				Exported:   true,
				DocComment: "extends",
			})
			currentScope = match[1]
			scopeType = "type"
			braceDepth += openBraces
			continue
		}

		// Type (including Query, Mutation, Subscription)
		if match := typePattern.FindStringSubmatch(line); match != nil {
			name := match[1]
			symType := "type"
			if name == "Query" || name == "Mutation" || name == "Subscription" {
				symType = "interface"
			}
			symbols = append(symbols, fallbackSymbol{
				Name:     name,
				Type:     symType,
				Line:     lineNum,
				Exported: true,
			})
			currentScope = name
			scopeType = "type"
			braceDepth += openBraces
			continue
		}

		// Interface
		if match := interfacePattern.FindStringSubmatch(line); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "interface",
				Line:     lineNum,
				Exported: true,
			})
			currentScope = match[1]
			scopeType = "interface"
			braceDepth += openBraces
			continue
		}

		// Input
		if match := inputPattern.FindStringSubmatch(line); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "struct",
				Line:     lineNum,
				Exported: true,
			})
			currentScope = match[1]
			scopeType = "input"
			braceDepth += openBraces
			continue
		}

		// Enum
		if match := enumPattern.FindStringSubmatch(line); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "enum",
				Line:     lineNum,
				Exported: true,
			})
			currentScope = match[1]
			scopeType = "enum"
			braceDepth += openBraces
			continue
		}

		// Union
		if match := unionPattern.FindStringSubmatch(line); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "type",
				Line:     lineNum,
				Exported: true,
			})
			continue
		}

		// Scalar
		if match := scalarPattern.FindStringSubmatch(line); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "type",
				Line:     lineNum,
				Exported: true,
			})
			continue
		}

		// Directive
		if match := directivePattern.FindStringSubmatch(line); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "function",
				Line:     lineNum,
				Exported: true,
			})
			continue
		}

		// Inside a scope
		if braceDepth > 0 && currentScope != "" {
			// Enum values
			if scopeType == "enum" {
				if match := enumValuePattern.FindStringSubmatch(trimmed); match != nil {
					name := match[1]
					if name != "{" && name != "}" {
						symbols = append(symbols, fallbackSymbol{
							Name:     name,
							Type:     "enum_value",
							Line:     lineNum,
							Parent:   currentScope,
							Exported: true,
						})
					}
				}
			} else {
				// Fields
				if match := fieldPattern.FindStringSubmatch(line); match != nil {
					name := match[1]
					fieldType := match[2]
					symType := "constant"
					if currentScope == "Query" || currentScope == "Mutation" || currentScope == "Subscription" {
						symType = "method"
					}
					symbols = append(symbols, fallbackSymbol{
						Name:     name,
						Type:     symType,
						Line:     lineNum,
						Parent:   currentScope,
						Value:    fieldType,
						Exported: true,
					})
				}
			}
		}

		// Handle closing braces
		braceDepth += openBraces - closeBraces
		if braceDepth == 0 {
			currentScope = ""
			scopeType = ""
		}
	}

	return symbols, true, nil
}

// ============================================================
// OpenAPI/Swagger Parser
// ============================================================

func parseOpenAPI(filePath string, content string) ([]fallbackSymbol, bool, error) {
	var symbols []fallbackSymbol
	lines := strings.Split(content, "\n")

	// Patterns for OpenAPI - match UPTS spec expectations
	pathPattern := regexp.MustCompile(`^  (/[^:]*):`)                            // Paths start with 2 spaces
	methodPattern := regexp.MustCompile(`^\s{4}(get|post|put|patch|delete|head|options):`) // Methods at 4 spaces
	operationIdPattern := regexp.MustCompile(`^\s+operationId:\s*(\S+)`)
	schemaPattern := regexp.MustCompile(`^\s{4}(\w+):\s*$`)                      // Schema names at 4 spaces under schemas:
	parameterNamePattern := regexp.MustCompile(`^\s+-\s*name:\s*(\S+)`)          // Parameter names in arrays
	securitySchemePattern := regexp.MustCompile(`^\s{4}(\w+):\s*$`)              // Security scheme names
	serverUrlPattern := regexp.MustCompile(`^\s+-\s*url:\s*(\S+)`)               // Server URLs

	var inPaths bool
	var inSchemas bool
	var inSecuritySchemes bool
	var inServers bool
	var inParameters bool
	var currentPath string
	var currentMethod string

	for i, line := range lines {
		lineNum := i + 1
		trimmedLine := strings.TrimRight(line, " \t\r")

		// Top-level section detection
		if strings.HasPrefix(trimmedLine, "info:") && !strings.HasPrefix(trimmedLine, " ") {
			symbols = append(symbols, fallbackSymbol{
				Name:     "info",
				Type:     "section",
				Line:     lineNum,
				Exported: true,
			})
			inPaths = false
			inSchemas = false
			inSecuritySchemes = false
			inServers = false
			continue
		}

		if strings.HasPrefix(trimmedLine, "servers:") && !strings.HasPrefix(trimmedLine, " ") {
			symbols = append(symbols, fallbackSymbol{
				Name:     "servers",
				Type:     "section",
				Line:     lineNum,
				Exported: true,
			})
			inServers = true
			inPaths = false
			inSchemas = false
			inSecuritySchemes = false
			continue
		}

		if strings.HasPrefix(trimmedLine, "paths:") && !strings.HasPrefix(trimmedLine, " ") {
			inPaths = true
			inSchemas = false
			inSecuritySchemes = false
			inServers = false
			continue
		}

		if strings.HasPrefix(trimmedLine, "components:") && !strings.HasPrefix(trimmedLine, " ") {
			inPaths = false
			inServers = false
			continue
		}

		if strings.HasPrefix(trimmedLine, "  schemas:") {
			inSchemas = true
			inSecuritySchemes = false
			continue
		}

		if strings.HasPrefix(trimmedLine, "  securitySchemes:") {
			inSecuritySchemes = true
			inSchemas = false
			continue
		}

		// Servers section - extract URLs with parent "servers"
		if inServers {
			if match := serverUrlPattern.FindStringSubmatch(line); match != nil {
				symbols = append(symbols, fallbackSymbol{
					Name:     match[1],
					Type:     "constant",
					Line:     lineNum,
					Parent:   "servers",
					Exported: true,
				})
				continue
			}
			// Exit servers section when we hit another top-level section
			if len(line) > 0 && line[0] != ' ' && line[0] != '#' {
				inServers = false
			}
		}

		// Paths section
		if inPaths {
			// New path - type "endpoint"
			if match := pathPattern.FindStringSubmatch(line); match != nil {
				currentPath = match[1]
				currentMethod = ""
				inParameters = false
				symbols = append(symbols, fallbackSymbol{
					Name:     currentPath,
					Type:     "endpoint",
					Line:     lineNum,
					Exported: true,
				})
				continue
			}

			// HTTP Method under path - just the method name, parent is the path
			if match := methodPattern.FindStringSubmatch(line); match != nil {
				currentMethod = strings.ToUpper(match[1])
				inParameters = false
				symbols = append(symbols, fallbackSymbol{
					Name:     currentMethod,
					Type:     "method",
					Line:     lineNum,
					Parent:   currentPath,
					Exported: true,
				})
				continue
			}

			// Operation ID - parent is "path.METHOD"
			if match := operationIdPattern.FindStringSubmatch(line); match != nil && currentPath != "" && currentMethod != "" {
				symbols = append(symbols, fallbackSymbol{
					Name:       match[1],
					Type:       "function",
					Line:       lineNum,
					Parent:     currentPath + "." + currentMethod,
					DocComment: "operationId",
					Exported:   true,
				})
				continue
			}

			// Parameters section marker
			if strings.Contains(line, "parameters:") {
				inParameters = true
				continue
			}

			// Parameter names - type "parameter", parent is "path.METHOD"
			if inParameters {
				if match := parameterNamePattern.FindStringSubmatch(line); match != nil && currentPath != "" && currentMethod != "" {
					symbols = append(symbols, fallbackSymbol{
						Name:     match[1],
						Type:     "parameter",
						Line:     lineNum,
						Parent:   currentPath + "." + currentMethod,
						Exported: true,
					})
					continue
				}
			}

			// Exit parameters when we see responses, requestBody, or another method
			if strings.Contains(line, "responses:") || strings.Contains(line, "requestBody:") {
				inParameters = false
			}
		}

		// Schemas section - type "class" with parent "components.schemas"
		if inSchemas {
			if match := schemaPattern.FindStringSubmatch(line); match != nil {
				// Skip sub-properties like "type:", "properties:", etc.
				name := match[1]
				if name != "type" && name != "properties" && name != "required" && name != "items" && name != "format" && name != "description" {
					symbols = append(symbols, fallbackSymbol{
						Name:     name,
						Type:     "class",
						Line:     lineNum,
						Parent:   "components.schemas",
						Exported: true,
					})
				}
				continue
			}
		}

		// Security schemes section - parent "components.securitySchemes"
		if inSecuritySchemes {
			if match := securitySchemePattern.FindStringSubmatch(line); match != nil {
				name := match[1]
				// Skip sub-properties
				if name != "type" && name != "scheme" && name != "in" && name != "name" && name != "bearerFormat" {
					symbols = append(symbols, fallbackSymbol{
						Name:     name,
						Type:     "constant",
						Line:     lineNum,
						Parent:   "components.securitySchemes",
						Exported: true,
					})
				}
				continue
			}
		}
	}

	return symbols, true, nil
}

// ============================================================
// Markdown Parser
// ============================================================

func parseMarkdown(filePath string) ([]fallbackSymbol, bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, true, err
	}

	var symbols []fallbackSymbol
	lines := strings.Split(string(content), "\n")

	// Patterns for Markdown
	headingPattern := regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
	codeBlockPattern := regexp.MustCompile("^```(\\w*)\\s*$")
	linkPattern := regexp.MustCompile(`^\[([^\]]+)\]\(([^)]+)\)`)

	var inCodeBlock bool
	var codeBlockLang string
	var headingStack []string

	for i, line := range lines {
		lineNum := i + 1

		// Track code block state
		if match := codeBlockPattern.FindStringSubmatch(line); match != nil {
			if !inCodeBlock {
				inCodeBlock = true
				codeBlockLang = match[1]
				if codeBlockLang != "" {
					symbols = append(symbols, fallbackSymbol{
						Name:     codeBlockLang,
						Type:     "section",
						Line:     lineNum,
						Exported: true,
					})
				}
			} else {
				inCodeBlock = false
			}
			continue
		}

		// Skip content inside code blocks
		if inCodeBlock {
			continue
		}

		// Headings
		if match := headingPattern.FindStringSubmatch(line); match != nil {
			level := len(match[1])
			heading := strings.TrimSpace(match[2])

			// Clean heading (remove trailing anchors)
			if idx := strings.Index(heading, " {#"); idx > 0 {
				heading = heading[:idx]
			}

			// Determine parent
			parent := ""
			if level > 1 && len(headingStack) > 0 {
				parent = headingStack[len(headingStack)-1]
			}

			// Update heading stack
			if level <= len(headingStack) {
				headingStack = headingStack[:level-1]
			}
			headingStack = append(headingStack, heading)

			symType := "heading"
			if level == 1 {
				symType = "section"
			}

			symbols = append(symbols, fallbackSymbol{
				Name:       heading,
				Type:       symType,
				Line:       lineNum,
				Parent:     parent,
				DocComment: strings.Repeat("#", level),
				Exported:   true,
			})
			continue
		}

		// Links at start of line
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			if match := linkPattern.FindStringSubmatch(trimmed); match != nil {
				linkText := match[1]
				linkURL := match[2]
				if strings.HasPrefix(linkURL, "http") || strings.HasPrefix(linkURL, "/") {
					symbols = append(symbols, fallbackSymbol{
						Name:     linkText,
						Type:     "constant",
						Line:     lineNum,
						Value:    linkURL,
						Exported: true,
					})
				}
			}
		}
	}

	return symbols, true, nil
}

// ============================================================
// XML Parser
// ============================================================

func parseXML(filePath string) ([]fallbackSymbol, bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, true, err
	}

	var symbols []fallbackSymbol
	lines := strings.Split(string(content), "\n")
	contentStr := string(content)

	// Detect XML kind
	ext := strings.ToLower(filepath.Ext(filePath))
	base := strings.ToLower(filepath.Base(filePath))
	xmlKind := detectXMLKind(base, ext, contentStr)

	// Patterns
	dependencyPattern := regexp.MustCompile(`<dependency>`)
	groupIdPattern := regexp.MustCompile(`<groupId>([^<]+)</groupId>`)
	artifactIdPattern := regexp.MustCompile(`<artifactId>([^<]+)</artifactId>`)
	versionPattern := regexp.MustCompile(`<version>([^<]+)</version>`)
	packageRefPattern := regexp.MustCompile(`<PackageReference\s+Include="([^"]+)"(?:\s+Version="([^"]+)")?`)
	targetPattern := regexp.MustCompile(`<Target\s+Name="([^"]+)"`)
	propertyGroupPattern := regexp.MustCompile(`<PropertyGroup`)
	itemGroupPattern := regexp.MustCompile(`<ItemGroup`)
	xsdElementPattern := regexp.MustCompile(`<(?:xs|xsd):element\s+name="([^"]+)"`)
	xsdComplexPattern := regexp.MustCompile(`<(?:xs|xsd):complexType\s+name="([^"]+)"`)

	// Track state for multi-line patterns
	var inDependency bool
	var depGroupId, depArtifactId, depVersion string
	var depStartLine int

	for i, line := range lines {
		lineNum := i + 1

		switch xmlKind {
		case "maven-pom":
			// Track dependency blocks
			if dependencyPattern.MatchString(line) {
				inDependency = true
				depStartLine = lineNum
				depGroupId = ""
				depArtifactId = ""
				depVersion = ""
			}
			if inDependency {
				if match := groupIdPattern.FindStringSubmatch(line); match != nil {
					depGroupId = match[1]
				}
				if match := artifactIdPattern.FindStringSubmatch(line); match != nil {
					depArtifactId = match[1]
				}
				if match := versionPattern.FindStringSubmatch(line); match != nil {
					depVersion = match[1]
				}
				if strings.Contains(line, "</dependency>") {
					if depGroupId != "" && depArtifactId != "" {
						symbols = append(symbols, fallbackSymbol{
							Name:     depGroupId + ":" + depArtifactId,
							Type:     "constant",
							Line:     depStartLine,
							Value:    depVersion,
							Exported: true,
						})
					}
					inDependency = false
				}
			}

		case "dotnet-project":
			// Package references
			if match := packageRefPattern.FindStringSubmatch(line); match != nil {
				version := ""
				if len(match) > 2 {
					version = match[2]
				}
				symbols = append(symbols, fallbackSymbol{
					Name:     match[1],
					Type:     "constant",
					Line:     lineNum,
					Value:    version,
					Exported: true,
				})
			}
			// Targets
			if match := targetPattern.FindStringSubmatch(line); match != nil {
				symbols = append(symbols, fallbackSymbol{
					Name:     match[1],
					Type:     "function",
					Line:     lineNum,
					Exported: true,
				})
			}
			// PropertyGroup and ItemGroup sections
			if propertyGroupPattern.MatchString(line) {
				symbols = append(symbols, fallbackSymbol{
					Name:     "PropertyGroup",
					Type:     "section",
					Line:     lineNum,
					Exported: true,
				})
			}
			if itemGroupPattern.MatchString(line) {
				symbols = append(symbols, fallbackSymbol{
					Name:     "ItemGroup",
					Type:     "section",
					Line:     lineNum,
					Exported: true,
				})
			}

		case "xsd-schema":
			// Element definitions
			if match := xsdElementPattern.FindStringSubmatch(line); match != nil {
				symbols = append(symbols, fallbackSymbol{
					Name:     match[1],
					Type:     "constant",
					Line:     lineNum,
					Exported: true,
				})
			}
			// Complex type definitions
			if match := xsdComplexPattern.FindStringSubmatch(line); match != nil {
				symbols = append(symbols, fallbackSymbol{
					Name:     match[1],
					Type:     "type",
					Line:     lineNum,
					Exported: true,
				})
			}
		}
	}

	return symbols, true, nil
}

func detectXMLKind(base, ext, content string) string {
	switch {
	case base == "pom.xml":
		return "maven-pom"
	case ext == ".csproj" || ext == ".vbproj" || ext == ".fsproj":
		return "dotnet-project"
	case ext == ".xsd":
		return "xsd-schema"
	case strings.Contains(content, "<project") && strings.Contains(content, "maven"):
		return "maven-pom"
	case strings.Contains(content, "<Project") && strings.Contains(content, "Sdk="):
		return "dotnet-project"
	case strings.Contains(content, "<xs:schema") || strings.Contains(content, "<xsd:schema"):
		return "xsd-schema"
	default:
		return "xml-data"
	}
}

// ============================================================
// C Parser
// ============================================================

func parseC(filePath string) ([]fallbackSymbol, bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, true, err
	}

	var symbols []fallbackSymbol
	lines := strings.Split(string(content), "\n")

	// Patterns for C
	definePattern := regexp.MustCompile(`^\s*#define\s+([A-Za-z_][A-Za-z0-9_]*)\s*(?:\([^)]*\))?\s*(.*)$`)
	typedefSimplePattern := regexp.MustCompile(`^\s*typedef\s+[\w\s\*]+\s+(\w+)\s*;`)
	typedefEnumStartPattern := regexp.MustCompile(`^\s*typedef\s+enum`)
	typedefStructStartPattern := regexp.MustCompile(`^\s*typedef\s+struct`)
	enumNamePattern := regexp.MustCompile(`enum\s+(\w+)`)
	structNamePattern := regexp.MustCompile(`struct\s+(\w+)`)
	structForwardPattern := regexp.MustCompile(`^\s*struct\s+(\w+)\s*;`)
	// Function pattern: capture the name immediately before opening paren
	funcPattern := regexp.MustCompile(`\b([a-zA-Z_][a-zA-Z0-9_]*)\s*\([^)]*\)\s*[;{]`)

	var inTypedefEnum bool
	var inTypedefStruct bool
	var typedefStartLine int
	var braceDepth int
	seenFunctions := make(map[string]bool)

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			continue
		}

		// Track brace depth for typedef blocks
		openBraces := strings.Count(line, "{")
		closeBraces := strings.Count(line, "}")

		// #define macros
		if match := definePattern.FindStringSubmatch(line); match != nil {
			name := match[1]
			value := strings.TrimSpace(match[2])
			// Remove trailing comments
			if idx := strings.Index(value, "//"); idx > 0 {
				value = strings.TrimSpace(value[:idx])
			}
			sym := fallbackSymbol{
				Name:     name,
				Type:     "macro",
				Line:     lineNum,
				Exported: true,
			}
			if value != "" {
				sym.Value = value
			}
			symbols = append(symbols, sym)
			continue
		}

		// typedef enum start
		if typedefEnumStartPattern.MatchString(line) {
			inTypedefEnum = true
			typedefStartLine = lineNum
						braceDepth = openBraces

			// Check for named enum: typedef enum EnumName {
			if match := enumNamePattern.FindStringSubmatch(line); match != nil {
				symbols = append(symbols, fallbackSymbol{
					Name:     match[1],
					Type:     "enum",
					Line:     lineNum,
					Exported: true,
				})
			}
			continue
		}

		// typedef struct start
		if typedefStructStartPattern.MatchString(line) {
			inTypedefStruct = true
			typedefStartLine = lineNum
						braceDepth = openBraces

			// Check for named struct: typedef struct StructName {
			if match := structNamePattern.FindStringSubmatch(line); match != nil {
				symbols = append(symbols, fallbackSymbol{
					Name:     match[1],
					Type:     "struct",
					Line:     lineNum,
					Exported: true,
				})
			}
			continue
		}

		// Handle closing brace of typedef enum
		if inTypedefEnum {
			braceDepth += openBraces - closeBraces
			if braceDepth <= 0 && closeBraces > 0 {
				// Extract typedef name from closing line: } TypeName;
				typedefNamePattern := regexp.MustCompile(`\}\s*(\w+)\s*;`)
				if match := typedefNamePattern.FindStringSubmatch(line); match != nil {
					symbols = append(symbols, fallbackSymbol{
						Name:     match[1],
						Type:     "type",
						Line:     typedefStartLine,
						Exported: true,
					})
				}
				inTypedefEnum = false
			}
			continue
		}

		// Handle closing brace of typedef struct
		if inTypedefStruct {
			braceDepth += openBraces - closeBraces
			if braceDepth <= 0 && closeBraces > 0 {
				// Extract typedef name from closing line: } TypeName;
				typedefNamePattern := regexp.MustCompile(`\}\s*(\w+)\s*;`)
				if match := typedefNamePattern.FindStringSubmatch(line); match != nil {
					// Don't add duplicate if struct name was already added
					alreadyExists := false
					for _, s := range symbols {
						if s.Name == match[1] && (s.Type == "struct" || s.Type == "type") {
							alreadyExists = true
							break
						}
					}
					if !alreadyExists {
						symbols = append(symbols, fallbackSymbol{
							Name:     match[1],
							Type:     "struct",
							Line:     typedefStartLine,
							Exported: true,
						})
					}
				}
				inTypedefStruct = false
			}
			continue
		}

		// Forward struct declaration (not typedef)
		if match := structForwardPattern.FindStringSubmatch(line); match != nil {
			if !strings.Contains(line, "typedef") {
				symbols = append(symbols, fallbackSymbol{
					Name:     match[1],
					Type:     "struct",
					Line:     lineNum,
					Exported: true,
				})
			}
			continue
		}

		// Simple typedef (type alias) - not struct/enum
		if strings.HasPrefix(trimmed, "typedef") && !strings.Contains(line, "struct") && !strings.Contains(line, "enum") {
			if match := typedefSimplePattern.FindStringSubmatch(line); match != nil {
				symbols = append(symbols, fallbackSymbol{
					Name:     match[1],
					Type:     "type",
					Line:     lineNum,
					Exported: true,
				})
				continue
			}
		}

		// Skip struct/enum/typedef keywords
		if strings.HasPrefix(trimmed, "typedef") || strings.HasPrefix(trimmed, "struct") || strings.HasPrefix(trimmed, "enum") {
			continue
		}

		// Function declarations and definitions
		// Look for pattern: name(params) followed by ; or {
		if strings.Contains(line, "(") && (strings.Contains(line, ";") || strings.Contains(line, "{")) {
			if match := funcPattern.FindStringSubmatch(line); match != nil {
				name := match[1]
				// Skip control flow and other keywords
				skipKeywords := map[string]bool{
					"if": true, "for": true, "while": true, "switch": true, "return": true,
					"sizeof": true, "typedef": true, "struct": true, "enum": true,
					"malloc": true, "free": true, "printf": true, "snprintf": true,
					"strchr": true, "time_t": true, "size_t": true, "NULL": true,
				}
				if skipKeywords[name] {
					continue
				}

				// Skip if already seen (declarations vs definitions)
				if seenFunctions[name] {
					continue
				}
				seenFunctions[name] = true

				isStatic := strings.Contains(line, "static ")
				isInline := strings.Contains(line, "inline ")

				symbols = append(symbols, fallbackSymbol{
					Name:     name,
					Type:     "function",
					Line:     lineNum,
					Exported: !isStatic || isInline,
				})
			}
		}
	}

	return symbols, true, nil
}

// ============================================================
// C++ Parser
// ============================================================

func parseCPP(filePath string) ([]fallbackSymbol, bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, true, err
	}

	var symbols []fallbackSymbol
	lines := strings.Split(string(content), "\n")

	// Patterns for C++
	constexprPattern := regexp.MustCompile(`^\s*(?:inline\s+)?constexpr\s+[\w<>:]+\s+(\w+)\s*=\s*(.+?);`)
	staticConstPattern := regexp.MustCompile(`^\s*static\s+const\s+[\w<>:]+\s+(\w+)\s*=\s*(.+?);`)
	usingPattern := regexp.MustCompile(`^\s*using\s+(\w+)\s*=`)
	namespacePattern := regexp.MustCompile(`^\s*namespace\s+(\w+)`)
	enumClassPattern := regexp.MustCompile(`^\s*enum\s+class\s+(\w+)`)
	enumPattern := regexp.MustCompile(`^\s*enum\s+(\w+)`)
	classPattern := regexp.MustCompile(`^\s*(template\s*<[^>]*>\s*)?(class|struct)\s+(\w+)`)
	methodDeclPattern := regexp.MustCompile(`^\s*(virtual\s+)?(static\s+)?([\w<>:*&\s]+)\s+(\w+)\s*\([^)]*\)`)
	funcDefPattern := regexp.MustCompile(`^([\w<>:*&\s]+)\s+(\w+)\s*\([^)]*\)\s*(?:const\s*)?\{`)
	publicPattern := regexp.MustCompile(`^\s*public:`)
	privatePattern := regexp.MustCompile(`^\s*private:`)
	protectedPattern := regexp.MustCompile(`^\s*protected:`)

	var currentClass string
	var classStack []string
	var inClass bool
	var isPublic bool
	var braceDepth int
	var classStartDepth int

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		// Track brace depth
		openBraces := strings.Count(line, "{")
		closeBraces := strings.Count(line, "}")

		// Constexpr constants
		if match := constexprPattern.FindStringSubmatch(line); match != nil {
			value := strings.TrimSpace(match[2])
			// Remove trailing semicolon and quotes
			value = strings.TrimSuffix(value, ";")
			if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
				value = value[1 : len(value)-1]
			}
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "constant",
				Line:     lineNum,
				Value:    value,
				Exported: true,
			})
			continue
		}

		// Static const
		if match := staticConstPattern.FindStringSubmatch(line); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "constant",
				Line:     lineNum,
				Value:    strings.TrimSpace(match[2]),
				Exported: true,
			})
			continue
		}

		// Using type alias
		if match := usingPattern.FindStringSubmatch(line); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "type",
				Line:     lineNum,
				Exported: true,
			})
			continue
		}

		// Namespace
		if match := namespacePattern.FindStringSubmatch(line); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "namespace",
				Line:     lineNum,
				Exported: true,
			})
			braceDepth += openBraces
			continue
		}

		// Enum class
		if match := enumClassPattern.FindStringSubmatch(line); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "enum",
				Line:     lineNum,
				Exported: true,
			})
			braceDepth += openBraces
			continue
		}

		// Enum
		if match := enumPattern.FindStringSubmatch(line); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "enum",
				Line:     lineNum,
				Exported: true,
			})
			braceDepth += openBraces
			continue
		}

		// Class or struct
		if match := classPattern.FindStringSubmatch(line); match != nil {
			className := match[3]
			symbols = append(symbols, fallbackSymbol{
				Name:     className,
				Type:     "class",
				Line:     lineNum,
				Exported: true,
			})
			if openBraces > 0 {
				classStack = append(classStack, currentClass)
				currentClass = className
				inClass = true
				isPublic = match[2] == "struct" // structs default to public
				classStartDepth = braceDepth
			}
			braceDepth += openBraces
			continue
		}

		// Visibility specifiers
		if publicPattern.MatchString(line) {
			isPublic = true
			continue
		}
		if privatePattern.MatchString(line) || protectedPattern.MatchString(line) {
			isPublic = false
			continue
		}

		// Method declarations inside class
		if inClass && braceDepth > classStartDepth {
			if match := methodDeclPattern.FindStringSubmatch(line); match != nil {
				name := match[4]
				// Skip keywords
				if name == "if" || name == "for" || name == "while" || name == "switch" || name == "return" || name == "delete" || name == "default" {
					braceDepth += openBraces - closeBraces
					goto checkClassEnd
				}

				symbols = append(symbols, fallbackSymbol{
					Name:     name,
					Type:     "method",
					Line:     lineNum,
					Parent:   currentClass,
					Exported: isPublic,
				})
				braceDepth += openBraces - closeBraces
				goto checkClassEnd
			}
		}

		// Standalone function (outside class)
		if !inClass && strings.Contains(line, "(") {
			if match := funcDefPattern.FindStringSubmatch(line); match != nil {
				name := match[2]
				// Skip keywords and control flow
				if name == "if" || name == "for" || name == "while" || name == "switch" || name == "return" {
					braceDepth += openBraces - closeBraces
					continue
				}
				// Skip class constructors/destructors (these are detected as methods)
				alreadyExists := false
				for _, s := range symbols {
					if s.Name == name && s.Line == lineNum {
						alreadyExists = true
						break
					}
				}
				if !alreadyExists {
					symbols = append(symbols, fallbackSymbol{
						Name:     name,
						Type:     "function",
						Line:     lineNum,
						Exported: true,
					})
				}
			}
		}

		braceDepth += openBraces - closeBraces

	checkClassEnd:
		// Check if we've left the current class
		if inClass && braceDepth <= classStartDepth {
			if len(classStack) > 0 {
				currentClass = classStack[len(classStack)-1]
				classStack = classStack[:len(classStack)-1]
				if currentClass == "" {
					inClass = false
				}
			} else {
				currentClass = ""
				inClass = false
			}
			isPublic = false
		}
	}

	return symbols, true, nil
}

// ============================================================
// CUDA Parser
// ============================================================

func parseCUDA(filePath string) ([]fallbackSymbol, bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, true, err
	}

	var symbols []fallbackSymbol
	lines := strings.Split(string(content), "\n")

	// Patterns for CUDA
	definePattern := regexp.MustCompile(`^\s*#define\s+([A-Za-z_][A-Za-z0-9_]*)\s*(?:\([^)]*\))?\s*(.*)$`)
	constexprPattern := regexp.MustCompile(`^\s*constexpr\s+[\w<>:]+\s+(\w+)\s*=\s*(.+?);`)
	staticConstPattern := regexp.MustCompile(`^\s*static\s+const\s+[\w<>:]+\s+(\w+)\s*=\s*(.+?);`)
	constantMemPattern := regexp.MustCompile(`^\s*__constant__\s+[\w<>:*\s]+\s+(\w+)`)
	globalKernelPattern := regexp.MustCompile(`^\s*__global__\s+[\w\s]+\s+(\w+)\s*\(`)
	deviceFuncPattern := regexp.MustCompile(`^\s*__device__\s+(?:__forceinline__\s+)?[\w\s<>:*&]+\s+(\w+)\s*\(`)
	hostDeviceFuncPattern := regexp.MustCompile(`^\s*__host__\s+__device__\s+[\w\s<>:*&]+\s+(\w+)\s*\(`)
	sharedVarPattern := regexp.MustCompile(`^\s*__shared__\s+[\w<>:*\s]+\s+(\w+)`)
	hostFuncPattern := regexp.MustCompile(`^[\w\s<>:*&]+\s+(\w+)\s*\([^)]*\)\s*\{`)

	// Track if we're inside a kernel or device function (for shared variables)
	var inCUDAFunction bool
	var braceDepth int

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		// Track brace depth
		openBraces := strings.Count(line, "{")
		closeBraces := strings.Count(line, "}")

		// #define macros
		if match := definePattern.FindStringSubmatch(line); match != nil {
			name := match[1]
			value := strings.TrimSpace(match[2])
			// Remove trailing comments
			if idx := strings.Index(value, "//"); idx > 0 {
				value = strings.TrimSpace(value[:idx])
			}
			sym := fallbackSymbol{
				Name:     name,
				Type:     "macro",
				Line:     lineNum,
				Exported: true,
			}
			if value != "" {
				sym.Value = value
			}
			symbols = append(symbols, sym)
			continue
		}

		// Constexpr constants
		if match := constexprPattern.FindStringSubmatch(line); match != nil {
			value := strings.TrimSpace(match[2])
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "constant",
				Line:     lineNum,
				Value:    value,
				Exported: true,
			})
			continue
		}

		// Static const
		if match := staticConstPattern.FindStringSubmatch(line); match != nil {
			continue // Skip static const in CUDA - not part of expected symbols
		}

		// __constant__ memory
		if match := constantMemPattern.FindStringSubmatch(line); match != nil {
			continue // Skip __constant__ for now - not in expected symbols
		}

		// __global__ kernel functions
		if match := globalKernelPattern.FindStringSubmatch(line); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "kernel",
				Line:     lineNum,
				Exported: true,
			})
			inCUDAFunction = true
			braceDepth += openBraces
			continue
		}

		// __host__ __device__ functions (treat as regular function)
		if match := hostDeviceFuncPattern.FindStringSubmatch(line); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "function",
				Line:     lineNum,
				Exported: true,
			})
			inCUDAFunction = true
			braceDepth += openBraces
			continue
		}

		// __device__ functions
		if match := deviceFuncPattern.FindStringSubmatch(line); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "device_function",
				Line:     lineNum,
				Exported: true,
			})
			inCUDAFunction = true
			braceDepth += openBraces
			continue
		}

		// __shared__ variables inside kernels
		if match := sharedVarPattern.FindStringSubmatch(line); match != nil {
			symbols = append(symbols, fallbackSymbol{
				Name:     match[1],
				Type:     "variable",
				Line:     lineNum,
				Exported: false,
			})
			braceDepth += openBraces - closeBraces
			continue
		}

		// Regular host functions
		if !inCUDAFunction && strings.Contains(line, "(") && strings.Contains(line, "{") {
			// Skip if line contains CUDA qualifiers
			if strings.Contains(line, "__global__") || strings.Contains(line, "__device__") || strings.Contains(line, "__host__") {
				braceDepth += openBraces - closeBraces
				continue
			}
			if match := hostFuncPattern.FindStringSubmatch(line); match != nil {
				name := match[1]
				// Skip keywords
				if name == "if" || name == "for" || name == "while" || name == "switch" || name == "return" {
					braceDepth += openBraces - closeBraces
					continue
				}
				// Skip constructors and common patterns
				if name == "struct" || name == "class" || name == "enum" {
					braceDepth += openBraces - closeBraces
					continue
				}
				// Only add if not already added
				alreadyExists := false
				for _, s := range symbols {
					if s.Name == name {
						alreadyExists = true
						break
					}
				}
				if !alreadyExists {
					symbols = append(symbols, fallbackSymbol{
						Name:     name,
						Type:     "function",
						Line:     lineNum,
						Exported: true,
					})
				}
			}
		}

		braceDepth += openBraces - closeBraces
		if braceDepth <= 0 {
			inCUDAFunction = false
		}
	}

	return symbols, true, nil
}
