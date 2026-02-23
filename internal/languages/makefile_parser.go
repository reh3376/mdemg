package languages

import (
	"bufio"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&MakefileParser{})
}

// MakefileParser implements LanguageParser for Makefiles and .mk files
type MakefileParser struct{}

func (p *MakefileParser) Name() string {
	return "makefile"
}

func (p *MakefileParser) Extensions() []string {
	return []string{".mk"}
}

func (p *MakefileParser) CanParse(path string) bool {
	// Check extension
	if HasExtension(path, p.Extensions()) {
		return true
	}
	// Check basename for Makefile, makefile, GNUmakefile (case-insensitive)
	baseName := strings.ToLower(filepath.Base(path))
	return baseName == "makefile" || baseName == "gnumakefile"
}

func (p *MakefileParser) IsTestFile(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.Contains(pathLower, "/fixtures/") ||
		strings.Contains(pathLower, "/testdata/") ||
		strings.Contains(pathLower, "_test.") ||
		strings.Contains(pathLower, "/test/")
}

// Regex patterns for Makefile parsing
var (
	// .PHONY: target1 target2 ...
	makePhonyRegex = regexp.MustCompile(`^\.PHONY\s*:\s*(.+)`)
	// VAR = value, VAR := value, VAR ?= value, VAR += value
	makeVarRegex = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\s*(?::=|\?=|\+=|=)\s*(.*)`)
	// export VAR = value  or  export VAR
	makeExportVarRegex = regexp.MustCompile(`^export\s+([A-Za-z_][A-Za-z0-9_]*)(?:\s*(?::=|\?=|\+=|=)\s*(.*))?$`)
	// target: [deps]  — must start at column 0, no = before :
	makeTargetRegex = regexp.MustCompile(`^([A-Za-z0-9_.%$(){}/<>@*?-][A-Za-z0-9_.%$(){}/<>@*? -]*?)\s*:([^=].*)?$`)
	// define NAME
	makeDefineRegex = regexp.MustCompile(`^define\s+(\S+)`)
	// endef
	makeEndefRegex = regexp.MustCompile(`^endef\s*$`)
	// include file.mk
	makeIncludeRegex = regexp.MustCompile(`^-?include\s+(.+)`)
)

func (p *MakefileParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)

	// Build summary content
	var contentBuilder strings.Builder
	contentBuilder.WriteString("Makefile: " + fileName + "\n")

	// Extract symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	// List targets for summary
	var targets []string
	for _, sym := range symbols {
		if sym.Type == "function" && sym.DocComment != "macro" {
			targets = append(targets, sym.Name)
		}
	}
	if len(targets) > 0 {
		contentBuilder.WriteString("Targets: " + strings.Join(targets, ", ") + "\n")
	}

	contentBuilder.WriteString("\n--- Content ---\n")
	truncated, wasTruncated := TruncateContentWithInfo(content, 4000)
	contentBuilder.WriteString(truncated)

	// Detect concerns
	concerns := DetectConcerns(relPath, content)
	tags := []string{"makefile", "build", "automation"}
	tags = append(tags, concerns...)

	var diagnostics []Diagnostic
	if wasTruncated {
		diagnostics = append(diagnostics, NewDiagnostic("info", "TRUNCATED", "Makefile content truncated to 4000 characters", "makefile"))
	}

	element := CodeElement{
		Name:        fileName,
		Kind:        "makefile",
		Path:        "/" + relPath,
		Content:     contentBuilder.String(),
		Package:     "build",
		FilePath:    relPath,
		Tags:        tags,
		Concerns:    concerns,
		Symbols:     symbols,
		ElementKind: "file",
		Diagnostics: diagnostics,
	}

	return []CodeElement{element}, nil
}

func (p *MakefileParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol

	// First pass: collect .PHONY targets
	phonyTargets := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if matches := makePhonyRegex.FindStringSubmatch(trimmed); matches != nil {
			for _, t := range strings.Fields(matches[1]) {
				phonyTargets[t] = true
			}
		}
	}

	// Second pass: extract all symbols
	scanner = bufio.NewScanner(strings.NewReader(content))
	lineNum := 0
	inDefine := false
	defineStartLine := 0
	defineName := ""
	inContinuation := false

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip recipe lines (tab-indented) unless we are inside a define block
		if len(line) > 0 && line[0] == '\t' && !inDefine {
			continue
		}

		// Handle continuation lines: if previous line ended with \, skip this one
		if inContinuation {
			// Check if this continuation line also continues
			trimmed := strings.TrimSpace(line)
			inContinuation = strings.HasSuffix(trimmed, "\\")
			continue
		}

		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Track if this line has a backslash continuation
		hasContinuation := strings.HasSuffix(trimmed, "\\")

		// Inside a define block, just wait for endef
		if inDefine {
			if makeEndefRegex.MatchString(trimmed) {
				symbols = append(symbols, Symbol{
					Name:       defineName,
					Type:       "function",
					Line:       defineStartLine,
					LineEnd:    lineNum,
					Exported:   false,
					DocComment: "macro",
					Language:   "makefile",
				})
				inDefine = false
				defineName = ""
			}
			continue
		}

		// Check for define block start
		if matches := makeDefineRegex.FindStringSubmatch(trimmed); matches != nil {
			inDefine = true
			defineStartLine = lineNum
			defineName = matches[1]
			continue
		}

		// Check for include statement (metadata only, not a symbol)
		if makeIncludeRegex.MatchString(trimmed) {
			if hasContinuation {
				inContinuation = true
			}
			continue
		}

		// Check for .PHONY (already collected in first pass, skip)
		if makePhonyRegex.MatchString(trimmed) {
			if hasContinuation {
				inContinuation = true
			}
			continue
		}

		// Check for export VAR [= value]
		if matches := makeExportVarRegex.FindStringSubmatch(trimmed); matches != nil {
			name := matches[1]
			value := ""
			if len(matches) > 2 {
				value = strings.TrimSpace(matches[2])
			}
			symbols = append(symbols, Symbol{
				Name:     name,
				Type:     "constant",
				Line:     lineNum,
				Value:    value,
				Exported: true,
				Language: "makefile",
			})
			if hasContinuation {
				inContinuation = true
			}
			continue
		}

		// Check for variable assignment (VAR = value)
		// Must check before target to avoid false positives:
		// a variable line has = before any : (or has no :)
		if matches := makeVarRegex.FindStringSubmatch(trimmed); matches != nil {
			// Ensure this is a variable assignment, not a target.
			// For := the colon is part of the operator, not a target separator.
			eqIdx := strings.IndexAny(trimmed, "=")
			colonIdx := strings.Index(trimmed, ":")
			isVarAssign := colonIdx == -1 || eqIdx < colonIdx
			// Also accept := where colon immediately precedes = (part of := operator)
			if !isVarAssign && colonIdx >= 0 && colonIdx+1 < len(trimmed) && trimmed[colonIdx+1] == '=' {
				isVarAssign = true
			}
			if isVarAssign {
				name := matches[1]
				value := strings.TrimSpace(matches[2])
				// Strip trailing backslash for continued lines
				if strings.HasSuffix(value, "\\") {
					value = strings.TrimSpace(strings.TrimSuffix(value, "\\"))
				}
				symbols = append(symbols, Symbol{
					Name:     name,
					Type:     "constant",
					Line:     lineNum,
					Value:    value,
					Exported: false,
					Language: "makefile",
				})
				if hasContinuation {
					inContinuation = true
				}
				continue
			}
		}

		// Check for target: [deps]
		// Line must start at column 0 (not indented)
		if len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
			if matches := makeTargetRegex.FindStringSubmatch(trimmed); matches != nil {
				targetName := strings.TrimSpace(matches[1])
				// Skip if target name contains = (variable assignment false positive)
				if strings.Contains(targetName, "=") {
					continue
				}
				exported := phonyTargets[targetName]
				symbols = append(symbols, Symbol{
					Name:     targetName,
					Type:     "function",
					Line:     lineNum,
					Exported: exported,
					Language: "makefile",
				})
				if hasContinuation {
					inContinuation = true
				}
				continue
			}
		}

		// Mark continuation for any unrecognized line
		if hasContinuation {
			inContinuation = true
		}
	}

	return symbols
}
