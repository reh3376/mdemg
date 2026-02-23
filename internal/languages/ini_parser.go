package languages

import (
	"bufio"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&INIParser{})
}

// INIParser implements LanguageParser for INI, dotenv, and properties files
type INIParser struct{}

func (p *INIParser) Name() string {
	return "ini"
}

func (p *INIParser) Extensions() []string {
	return []string{".env", ".ini", ".cfg", ".properties"}
}

func (p *INIParser) CanParse(path string) bool {
	pathLower := strings.ToLower(path)
	for _, ext := range p.Extensions() {
		if strings.HasSuffix(pathLower, ext) {
			return true
		}
	}
	// Also match Dockerfile.env, .env.local, etc.
	base := strings.ToLower(filepath.Base(path))
	return strings.HasPrefix(base, ".env") || strings.HasSuffix(base, ".env")
}

func (p *INIParser) IsTestFile(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.Contains(pathLower, "/fixtures/") ||
		strings.Contains(pathLower, "/testdata/") ||
		strings.Contains(pathLower, "_test.")
}

func (p *INIParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
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
	contentBuilder.WriteString("Config file: " + fileName + "\n")
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
	tags := []string{"ini", "config", fileKind}
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

func (p *INIParser) detectFileKind(fileName string) string {
	fileNameLower := strings.ToLower(fileName)

	switch {
	case strings.HasSuffix(fileNameLower, ".properties"):
		return "java-properties"
	case strings.HasSuffix(fileNameLower, ".ini"):
		return "ini-config"
	case strings.HasSuffix(fileNameLower, ".cfg"):
		return "cfg-config"
	case strings.Contains(fileNameLower, ".env"):
		return "dotenv"
	default:
		return "config"
	}
}

// Regex patterns for parsing
var (
	sectionRegex   = regexp.MustCompile(`^\[([^\]]+)\]$`)
	keyValueRegex  = regexp.MustCompile(`^([^=]+)=(.*)$`)
	keyValueSpaced = regexp.MustCompile(`^([^=]+)\s*=\s*(.*)$`)
)

func (p *INIParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol
	currentSection := ""

	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}

		// Check for section header [section]
		if matches := sectionRegex.FindStringSubmatch(trimmed); matches != nil {
			sectionName := matches[1]
			currentSection = sectionName

			symbols = append(symbols, Symbol{
				Name:     sectionName,
				Type:     "section",
				Line:     lineNum,
				Exported: true,
				Language: "ini",
			})
			continue
		}

		// Check for key=value or key = value
		var key, value string
		if matches := keyValueSpaced.FindStringSubmatch(trimmed); matches != nil {
			key = strings.TrimSpace(matches[1])
			value = strings.TrimSpace(matches[2])
		} else if matches := keyValueRegex.FindStringSubmatch(trimmed); matches != nil {
			key = strings.TrimSpace(matches[1])
			value = strings.TrimSpace(matches[2])
		}

		if key != "" {
			// Strip quotes from value
			value = p.stripQuotes(value)

			// Build full name with section prefix
			fullName := key
			parent := ""
			if currentSection != "" {
				fullName = currentSection + "." + key
				parent = currentSection
			}

			// Determine if exported (uppercase = env var style = exported)
			exported := key == strings.ToUpper(key)

			symbols = append(symbols, Symbol{
				Name:     fullName,
				Type:     "constant",
				Line:     lineNum,
				Value:    value,
				Parent:   parent,
				Exported: exported,
				Language: "ini",
			})
		}
	}

	return symbols
}

func (p *INIParser) stripQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
