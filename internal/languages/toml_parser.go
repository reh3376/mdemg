package languages

import (
	"bufio"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&TOMLParser{})
}

// TOMLParser implements LanguageParser for TOML configuration files
type TOMLParser struct{}

func (p *TOMLParser) Name() string {
	return "toml"
}

func (p *TOMLParser) Extensions() []string {
	return []string{".toml"}
}

func (p *TOMLParser) CanParse(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".toml")
}

func (p *TOMLParser) IsTestFile(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.Contains(pathLower, "/fixtures/") ||
		strings.Contains(pathLower, "/testdata/") ||
		strings.Contains(pathLower, "_test.")
}

func (p *TOMLParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)

	// Detect file type
	fileKind := p.detectFileKind(fileName)

	// Build summary content
	var contentBuilder strings.Builder
	contentBuilder.WriteString("TOML file: " + fileName + "\n")
	contentBuilder.WriteString("Type: " + fileKind + "\n")

	// Extract symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	contentBuilder.WriteString("\n--- Content ---\n")
	contentBuilder.WriteString(TruncateContent(content, 4000))

	// Detect concerns
	concerns := DetectConcerns(relPath, content)
	tags := []string{"toml", "config", fileKind}
	tags = append(tags, concerns...)

	element := CodeElement{
		Name:        fileName,
		Kind:        fileKind,
		Path:        "/" + relPath,
		Content:     contentBuilder.String(),
		Package:     "config",
		FilePath:    relPath,
		Tags:        tags,
		Concerns:    concerns,
		Symbols:     symbols,
		ElementKind: "file",
	}

	return []CodeElement{element}, nil
}

func (p *TOMLParser) detectFileKind(fileName string) string {
	fileNameLower := strings.ToLower(fileName)

	switch {
	case fileNameLower == "cargo.toml":
		return "cargo-manifest"
	case fileNameLower == "pyproject.toml":
		return "pyproject"
	case fileNameLower == "gopkg.toml":
		return "gopkg"
	case fileNameLower == "netlify.toml":
		return "netlify-config"
	case fileNameLower == "rust-toolchain.toml":
		return "rust-toolchain"
	case strings.Contains(fileNameLower, "config"):
		return "toml-config"
	default:
		return "toml"
	}
}

// Regex patterns for TOML parsing
var (
	tomlTableRegex       = regexp.MustCompile(`^\[([^\]]+)\]$`)
	tomlArrayTableRegex  = regexp.MustCompile(`^\[\[([^\]]+)\]\]$`)
	tomlKeyValueRegex    = regexp.MustCompile(`^([^=]+)\s*=\s*(.+)$`)
	tomlMultilineStart   = regexp.MustCompile(`^([^=]+)\s*=\s*"""`)
	tomlArrayStart       = regexp.MustCompile(`^([^=]+)\s*=\s*\[$`)
)

func (p *TOMLParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol
	currentTable := ""
	arrayTableCounts := make(map[string]int)

	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNum := 0
	inMultiline := false
	inArray := false

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Handle multiline strings
		if inMultiline {
			if strings.Contains(trimmed, `"""`) {
				inMultiline = false
			}
			continue
		}

		// Handle multiline arrays
		if inArray {
			if strings.HasSuffix(trimmed, "]") {
				inArray = false
			}
			continue
		}

		// Check for array of tables [[table]]
		if matches := tomlArrayTableRegex.FindStringSubmatch(trimmed); matches != nil {
			tableName := matches[1]
			arrayTableCounts[tableName]++
			currentTable = tableName

			// Only emit section on first occurrence
			if arrayTableCounts[tableName] == 1 {
				symbols = append(symbols, Symbol{
					Name:       tableName,
					Type:       "section",
					Line:       lineNum,
					Exported:   true,
					DocComment: "array-of-tables",
					Language:   "toml",
				})
			}
			continue
		}

		// Check for table [table]
		if matches := tomlTableRegex.FindStringSubmatch(trimmed); matches != nil {
			tableName := matches[1]
			currentTable = tableName

			symbols = append(symbols, Symbol{
				Name:     tableName,
				Type:     "section",
				Line:     lineNum,
				Exported: true,
				Language: "toml",
			})
			continue
		}

		// Check for multiline string start
		if tomlMultilineStart.MatchString(trimmed) && !strings.HasSuffix(trimmed, `"""`) {
			inMultiline = true
			// Still extract the key
			if matches := tomlKeyValueRegex.FindStringSubmatch(trimmed); matches != nil {
				key := strings.TrimSpace(matches[1])
				p.addKeySymbol(&symbols, key, "...", currentTable, lineNum)
			}
			continue
		}

		// Check for multiline array start
		if tomlArrayStart.MatchString(trimmed) {
			inArray = true
			if matches := tomlKeyValueRegex.FindStringSubmatch(trimmed); matches != nil {
				key := strings.TrimSpace(matches[1])
				p.addKeySymbol(&symbols, key, "[...]", currentTable, lineNum)
			}
			continue
		}

		// Check for key = value
		if matches := tomlKeyValueRegex.FindStringSubmatch(trimmed); matches != nil {
			key := strings.TrimSpace(matches[1])
			value := strings.TrimSpace(matches[2])

			// Clean up value
			value = p.cleanValue(value)

			p.addKeySymbol(&symbols, key, value, currentTable, lineNum)
		}
	}

	return symbols
}

func (p *TOMLParser) addKeySymbol(symbols *[]Symbol, key, value, currentTable string, lineNum int) {
	fullKey := key
	parent := ""
	if currentTable != "" {
		fullKey = currentTable + "." + key
		parent = currentTable
	}

	*symbols = append(*symbols, Symbol{
		Name:     fullKey,
		Type:     "constant",
		Line:     lineNum,
		Value:    value,
		Parent:   parent,
		Exported: true,
		Language: "toml",
	})
}

func (p *TOMLParser) cleanValue(value string) string {
	// Remove inline comments
	if idx := strings.Index(value, " #"); idx != -1 {
		value = strings.TrimSpace(value[:idx])
	}

	// Strip quotes
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}

	// Handle inline arrays - just return as-is
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		return value
	}

	return value
}
