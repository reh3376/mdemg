package languages

import (
	"bufio"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&ShellParser{})
}

// ShellParser implements LanguageParser for shell scripts (bash, sh, zsh)
type ShellParser struct{}

func (p *ShellParser) Name() string {
	return "shell"
}

func (p *ShellParser) Extensions() []string {
	return []string{".sh", ".bash", ".zsh"}
}

func (p *ShellParser) CanParse(path string) bool {
	pathLower := strings.ToLower(path)
	for _, ext := range p.Extensions() {
		if strings.HasSuffix(pathLower, ext) {
			return true
		}
	}
	return false
}

func (p *ShellParser) IsTestFile(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.Contains(pathLower, "/fixtures/") ||
		strings.Contains(pathLower, "/testdata/") ||
		strings.Contains(pathLower, "_test.") ||
		strings.Contains(pathLower, "/test/")
}

func (p *ShellParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)

	// Detect shell type
	shellType := p.detectShellType(content, path)

	// Build summary content
	var contentBuilder strings.Builder
	contentBuilder.WriteString("Shell script: " + fileName + "\n")
	contentBuilder.WriteString("Type: " + shellType + "\n")

	// Extract symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	// List functions for summary
	var funcs []string
	for _, sym := range symbols {
		if sym.Type == "function" {
			funcs = append(funcs, sym.Name)
		}
	}
	if len(funcs) > 0 {
		contentBuilder.WriteString("Functions: " + strings.Join(funcs, ", ") + "\n")
	}

	contentBuilder.WriteString("\n--- Content ---\n")
	contentBuilder.WriteString(TruncateContent(content, 4000))

	// Detect concerns
	concerns := DetectConcerns(relPath, content)
	tags := []string{"shell", shellType, "script"}
	tags = append(tags, concerns...)

	element := CodeElement{
		Name:        fileName,
		Kind:        "shell-script",
		Path:        "/" + relPath,
		Content:     contentBuilder.String(),
		Package:     "scripts",
		FilePath:    relPath,
		Tags:        tags,
		Concerns:    concerns,
		Symbols:     symbols,
		ElementKind: "file",
	}

	return []CodeElement{element}, nil
}

func (p *ShellParser) detectShellType(content, path string) string {
	// Check shebang first
	if strings.HasPrefix(content, "#!/") {
		firstLine := strings.SplitN(content, "\n", 2)[0]
		if strings.Contains(firstLine, "bash") {
			return "bash"
		}
		if strings.Contains(firstLine, "zsh") {
			return "zsh"
		}
		if strings.Contains(firstLine, "/sh") {
			return "sh"
		}
	}

	// Check extension
	pathLower := strings.ToLower(path)
	if strings.HasSuffix(pathLower, ".bash") {
		return "bash"
	}
	if strings.HasSuffix(pathLower, ".zsh") {
		return "zsh"
	}

	return "sh"
}

// Regex patterns for shell parsing
var (
	// function name() { or function name {
	funcKeywordRegex = regexp.MustCompile(`^function\s+(\w+)\s*(?:\(\))?\s*\{?`)
	// name() {
	funcParenRegex = regexp.MustCompile(`^(\w+)\s*\(\)\s*\{?`)
	// export VAR=value
	exportRegex = regexp.MustCompile(`^export\s+(\w+)(?:=(.*))?`)
	// VAR=value (uppercase = constant)
	assignRegex = regexp.MustCompile(`^([A-Z][A-Z0-9_]*)=(.*)`)
	// readonly VAR=value
	readonlyRegex = regexp.MustCompile(`^readonly\s+(\w+)(?:=(.*))?`)
	// local VAR=value (inside functions)
	localRegex = regexp.MustCompile(`^\s*local\s+(\w+)(?:=(.*))?`)
	// source or . include
	sourceRegex = regexp.MustCompile(`^(?:source|\.\s+)(.+)`)
	// alias name='command'
	aliasRegex = regexp.MustCompile(`^alias\s+(\w+)=(.*)`)
)

func (p *ShellParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol
	currentFunction := ""
	inFunction := false
	braceDepth := 0

	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Track brace depth for function boundaries
		braceDepth += strings.Count(trimmed, "{") - strings.Count(trimmed, "}")
		if inFunction && braceDepth == 0 {
			inFunction = false
			currentFunction = ""
		}

		// Check for function with 'function' keyword
		if matches := funcKeywordRegex.FindStringSubmatch(trimmed); matches != nil {
			funcName := matches[1]
			symbols = append(symbols, Symbol{
				Name:     funcName,
				Type:     "function",
				Line:     lineNum,
				Exported: true,
				Language: "shell",
			})
			currentFunction = funcName
			inFunction = strings.Contains(trimmed, "{")
			if !inFunction {
				braceDepth = 0
			}
			continue
		}

		// Check for function with parens syntax
		if matches := funcParenRegex.FindStringSubmatch(trimmed); matches != nil {
			funcName := matches[1]
			// Skip if it looks like a command (common false positives)
			if !isShellKeyword(funcName) {
				symbols = append(symbols, Symbol{
					Name:     funcName,
					Type:     "function",
					Line:     lineNum,
					Exported: true,
					Language: "shell",
				})
				currentFunction = funcName
				inFunction = strings.Contains(trimmed, "{")
			}
			continue
		}

		// Check for export
		if matches := exportRegex.FindStringSubmatch(trimmed); matches != nil {
			name := matches[1]
			value := ""
			if len(matches) > 2 {
				value = p.cleanValue(matches[2])
			}

			symbols = append(symbols, Symbol{
				Name:     name,
				Type:     "constant",
				Line:     lineNum,
				Value:    value,
				Parent:   currentFunction,
				Exported: true,
				Language: "shell",
			})
			continue
		}

		// Check for readonly
		if matches := readonlyRegex.FindStringSubmatch(trimmed); matches != nil {
			name := matches[1]
			value := ""
			if len(matches) > 2 {
				value = p.cleanValue(matches[2])
			}

			symbols = append(symbols, Symbol{
				Name:       name,
				Type:       "constant",
				Line:       lineNum,
				Value:      value,
				Parent:     currentFunction,
				Exported:   true,
				DocComment: "readonly",
				Language:   "shell",
			})
			continue
		}

		// Check for uppercase assignment (module-level constant)
		if !inFunction {
			if matches := assignRegex.FindStringSubmatch(trimmed); matches != nil {
				name := matches[1]
				value := p.cleanValue(matches[2])

				symbols = append(symbols, Symbol{
					Name:     name,
					Type:     "constant",
					Line:     lineNum,
					Value:    value,
					Exported: false, // Not exported unless 'export' keyword
					Language: "shell",
				})
				continue
			}
		}

		// Check for alias
		if matches := aliasRegex.FindStringSubmatch(trimmed); matches != nil {
			name := matches[1]
			value := p.cleanValue(matches[2])

			symbols = append(symbols, Symbol{
				Name:       name,
				Type:       "function",
				Line:       lineNum,
				Value:      value,
				Exported:   true,
				DocComment: "alias",
				Language:   "shell",
			})
			continue
		}

		// Check for source/include
		if matches := sourceRegex.FindStringSubmatch(trimmed); matches != nil {
			sourcePath := p.cleanValue(matches[1])

			symbols = append(symbols, Symbol{
				Name:       "source:" + sourcePath,
				Type:       "constant",
				Line:       lineNum,
				Value:      sourcePath,
				Exported:   true,
				DocComment: "include",
				Language:   "shell",
			})
			continue
		}
	}

	return symbols
}

func (p *ShellParser) cleanValue(value string) string {
	value = strings.TrimSpace(value)

	// Remove trailing comment
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

	return value
}

func isShellKeyword(name string) bool {
	keywords := map[string]bool{
		"if": true, "then": true, "else": true, "elif": true, "fi": true,
		"case": true, "esac": true, "for": true, "while": true, "until": true,
		"do": true, "done": true, "in": true, "select": true,
		"time": true, "coproc": true,
	}
	return keywords[name]
}
