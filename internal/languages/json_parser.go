package languages

import (
	"bufio"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&JSONParser{})
}

// JSONParser implements LanguageParser for JSON configuration files
type JSONParser struct{}

func (p *JSONParser) Name() string {
	return "json"
}

func (p *JSONParser) Extensions() []string {
	return []string{".json", ".jsonc", ".json5", ".jsonl", ".ndjson"}
}

func (p *JSONParser) CanParse(path string) bool {
	pathLower := strings.ToLower(path)
	for _, ext := range p.Extensions() {
		if strings.HasSuffix(pathLower, ext) {
			return true
		}
	}
	return false
}

func (p *JSONParser) IsTestFile(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.Contains(pathLower, "/fixtures/") ||
		strings.Contains(pathLower, "/testdata/") ||
		strings.Contains(pathLower, "__mocks__") ||
		strings.HasSuffix(pathLower, ".test.json")
}

func (p *JSONParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)
	pathLower := strings.ToLower(path)

	// Check if this is a JSONL (JSON Lines) file
	isJSONL := strings.HasSuffix(pathLower, ".jsonl") || strings.HasSuffix(pathLower, ".ndjson")

	// Detect file type based on filename
	jsonKind := p.detectJsonKind(fileName)

	// Build content for embedding
	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("JSON file: %s\n", fileName))
	contentBuilder.WriteString(fmt.Sprintf("Type: %s\n", jsonKind))

	// Extract symbols with line numbers
	var symbols []Symbol
	if extractSymbols {
		if isJSONL {
			symbols = p.extractJSONLSymbols(content)
		} else {
			symbols = p.extractSymbols(content)
		}
	}

	contentBuilder.WriteString("\n--- Content ---\n")
	contentBuilder.WriteString(TruncateContent(content, 4000))

	// Detect cross-cutting concerns
	concerns := DetectConcerns(relPath, content)
	tags := []string{"json", "config", jsonKind}
	tags = append(tags, concerns...)

	element := CodeElement{
		Name:        fileName,
		Kind:        jsonKind,
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

func (p *JSONParser) detectJsonKind(fileName string) string {
	fileNameLower := strings.ToLower(fileName)

	// Check for JSONL/NDJSON first
	if strings.HasSuffix(fileNameLower, ".jsonl") || strings.HasSuffix(fileNameLower, ".ndjson") {
		if strings.Contains(fileNameLower, "log") {
			return "json-log"
		}
		if strings.Contains(fileNameLower, "event") {
			return "json-events"
		}
		return "json-lines"
	}

	switch {
	case fileNameLower == "package.json":
		return "npm-package"
	case fileNameLower == "tsconfig.json" || strings.HasPrefix(fileNameLower, "tsconfig."):
		return "typescript-config"
	case fileNameLower == "composer.json":
		return "composer-package"
	case strings.HasPrefix(fileNameLower, ".eslint"):
		return "eslint-config"
	case strings.HasPrefix(fileNameLower, ".prettier"):
		return "prettier-config"
	case fileNameLower == "manifest.json":
		return "manifest"
	case strings.Contains(fileNameLower, "config"):
		return "configuration"
	case strings.Contains(fileNameLower, "settings"):
		return "settings"
	default:
		return "json-data"
	}
}

// Regex patterns for JSON line-by-line parsing
var (
	// Match "key": at start of line (with optional whitespace)
	jsonKeyRegex = regexp.MustCompile(`^\s*"([^"]+)"\s*:`)
	// Match simple values: "key": "value" or "key": number or "key": true/false/null
	jsonStringValueRegex = regexp.MustCompile(`^\s*"([^"]+)"\s*:\s*"([^"]*)"`)
	jsonNumberValueRegex = regexp.MustCompile(`^\s*"([^"]+)"\s*:\s*(-?[\d.]+)`)
	jsonBoolValueRegex   = regexp.MustCompile(`^\s*"([^"]+)"\s*:\s*(true|false|null)`)
	// Match object start: "key": {
	jsonObjectStartRegex = regexp.MustCompile(`^\s*"([^"]+)"\s*:\s*\{`)
	// Match array start: "key": [
	jsonArrayStartRegex = regexp.MustCompile(`^\s*"([^"]+)"\s*:\s*\[`)
)

func (p *JSONParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol
	var sectionStack []string
	braceDepth := 0
	bracketDepth := 0

	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Track brace depth for sections
		openBraces := strings.Count(line, "{")
		closeBraces := strings.Count(line, "}")
		openBrackets := strings.Count(line, "[")
		closeBrackets := strings.Count(line, "]")

		// Pop sections when closing braces
		// braceDepth represents nesting: 1=root, 2=first section, etc.
		// sectionStack has N elements when inside N named sections
		for i := 0; i < closeBraces; i++ {
			if braceDepth > 0 {
				braceDepth--
				// Keep only sections that are at a level <= current brace depth - 1
				// (depth 1 = root, no sections; depth 2 = 1 section; etc.)
				targetLen := braceDepth - 1
				if targetLen < 0 {
					targetLen = 0
				}
				if targetLen < len(sectionStack) {
					sectionStack = sectionStack[:targetLen]
				}
			}
		}

		bracketDepth += openBrackets - closeBrackets
		if bracketDepth < 0 {
			bracketDepth = 0
		}

		// Get current parent
		parent := ""
		if len(sectionStack) > 0 {
			parent = sectionStack[len(sectionStack)-1]
		}

		// Check for object start "key": {
		if matches := jsonObjectStartRegex.FindStringSubmatch(line); matches != nil {
			key := matches[1]
			fullKey := key
			if parent != "" {
				fullKey = parent + "." + key
			}

			symbols = append(symbols, Symbol{
				Name:     fullKey,
				Type:     "section",
				Line:     lineNum,
				Parent:   parent,
				Exported: true,
				Language: "json",
			})

			sectionStack = append(sectionStack, fullKey)
			braceDepth++
			continue
		}

		// Check for array start "key": [
		if matches := jsonArrayStartRegex.FindStringSubmatch(line); matches != nil {
			key := matches[1]
			fullKey := key
			if parent != "" {
				fullKey = parent + "." + key
			}

			symbols = append(symbols, Symbol{
				Name:     fullKey,
				Type:     "section",
				Line:     lineNum,
				Parent:   parent,
				Exported: true,
				Language: "json",
			})
			continue
		}

		// Check for string value
		if matches := jsonStringValueRegex.FindStringSubmatch(line); matches != nil {
			key := matches[1]
			value := matches[2]
			fullKey := key
			if parent != "" {
				fullKey = parent + "." + key
			}

			// Only include reasonably short values
			if len(value) < 500 {
				symbols = append(symbols, Symbol{
					Name:     fullKey,
					Type:     "constant",
					Line:     lineNum,
					Value:    value,
					Parent:   parent,
					Exported: true,
					Language: "json",
				})
			}
			continue
		}

		// Check for number value
		if matches := jsonNumberValueRegex.FindStringSubmatch(line); matches != nil {
			key := matches[1]
			value := matches[2]
			fullKey := key
			if parent != "" {
				fullKey = parent + "." + key
			}

			symbols = append(symbols, Symbol{
				Name:     fullKey,
				Type:     "constant",
				Line:     lineNum,
				Value:    value,
				Parent:   parent,
				Exported: true,
				Language: "json",
			})
			continue
		}

		// Check for bool/null value
		if matches := jsonBoolValueRegex.FindStringSubmatch(line); matches != nil {
			key := matches[1]
			value := matches[2]
			fullKey := key
			if parent != "" {
				fullKey = parent + "." + key
			}

			symbols = append(symbols, Symbol{
				Name:     fullKey,
				Type:     "constant",
				Line:     lineNum,
				Value:    value,
				Parent:   parent,
				Exported: true,
				Language: "json",
			})
			continue
		}

		// Update brace depth for opening braces (that weren't part of object start)
		braceDepth += openBraces
	}

	return symbols
}

func (p *JSONParser) extractJSONLSymbols(content string) []Symbol {
	var symbols []Symbol

	// For JSONL, just extract keys from first line
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) > 0 {
		firstLine := strings.TrimSpace(lines[0])
		if firstLine != "" {
			// Simple key extraction from first record
			for _, match := range jsonKeyRegex.FindAllStringSubmatch(firstLine, -1) {
				symbols = append(symbols, Symbol{
					Name:     match[1],
					Type:     "constant",
					Line:     1,
					Exported: true,
					Language: "json",
				})
			}
		}
	}

	return symbols
}
