package languages

import (
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&LuaParser{})
}

// LuaParser implements LanguageParser for Lua source files (.lua)
type LuaParser struct{}

func (p *LuaParser) Name() string {
	return "lua"
}

func (p *LuaParser) Extensions() []string {
	return []string{".lua"}
}

func (p *LuaParser) CanParse(path string) bool {
	return HasExtension(path, p.Extensions())
}

func (p *LuaParser) IsTestFile(path string) bool {
	name := filepath.Base(path)
	return strings.HasSuffix(name, "_test.lua") ||
		strings.HasSuffix(name, "_spec.lua") ||
		strings.HasSuffix(name, ".test.lua") ||
		strings.Contains(path, "/test/") ||
		strings.Contains(path, "/tests/") ||
		strings.Contains(path, "/spec/")
}

func (p *LuaParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)

	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	// Build description
	var descParts []string
	descParts = append(descParts, "Lua file: "+fileName)

	funcCount := 0
	classCount := 0
	for _, s := range symbols {
		switch s.Type {
		case "function", "method":
			funcCount++
		case "class":
			classCount++
		}
	}

	if classCount > 0 {
		descParts = append(descParts, "Defines classes/tables")
	}
	if funcCount > 0 {
		descParts = append(descParts, "Defines functions")
	}

	// Include truncated content
	truncated, _ := TruncateContentWithInfo(content, 4000)

	return []CodeElement{{
		Name:        fileName,
		Kind:        "file",
		Path:        "/" + relPath,
		Content:     truncated,
		Summary:     strings.Join(descParts, ". "),
		Package:     "",
		FilePath:    relPath,
		Tags:        []string{"lua"},
		Symbols:     symbols,
		ElementKind: "file",
		StartLine:   1,
		EndLine:     strings.Count(content, "\n") + 1,
	}}, nil
}

func (p *LuaParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	// Regex patterns for Lua constructs
	localConstRe := regexp.MustCompile(`^local\s+([A-Z_][A-Z0-9_]*)\s*=\s*[^{(\s]`)
	globalConstRe := regexp.MustCompile(`^([A-Z_][A-Z0-9_]*)\s*=\s*[^{(\s]`)
	globalFuncRe := regexp.MustCompile(`^function\s+(\w+)\s*\(`)
	localFuncRe := regexp.MustCompile(`^local\s+function\s+(\w+)\s*\(`)
	methodRe := regexp.MustCompile(`^function\s+(\w+):(\w+)\s*\(`)
	moduleFuncRe := regexp.MustCompile(`^function\s+(\w+)\.(\w+)\s*\(`)
	classTableRe := regexp.MustCompile(`^([A-Z][A-Za-z0-9]*)\s*=\s*\{\s*\}`)
	enumTableRe := regexp.MustCompile(`^([A-Z][A-Za-z0-9]*)\s*=\s*\{`)

	// Track which tables are classes (have __index)
	classNames := make(map[string]bool)

	// First pass: identify classes (tables with __index)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, ".__index") {
			parts := strings.Split(trimmed, ".")
			if len(parts) >= 1 {
				name := strings.TrimSpace(parts[0])
				classNames[name] = true
			}
		}
	}

	// Second pass: extract symbols
	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Skip comments and empty lines
		if strings.HasPrefix(trimmed, "--") || trimmed == "" {
			continue
		}

		// Method (function ClassName:methodName)
		if m := methodRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{
				Name:     m[2],
				Type:     "method",
				Line:     lineNum,
				Exported: true,
				Parent:   m[1],
			})
			continue
		}

		// Module function (function M.funcName)
		if m := moduleFuncRe.FindStringSubmatch(trimmed); m != nil {
			// Check if it's a single letter module (like M)
			if len(m[1]) == 1 {
				symbols = append(symbols, Symbol{
					Name:     m[2],
					Type:     "function",
					Line:     lineNum,
					Exported: false, // Module functions are typically local
					Parent:   m[1],
				})
			}
			continue
		}

		// Global function (function name())
		if m := globalFuncRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{
				Name:     m[1],
				Type:     "function",
				Line:     lineNum,
				Exported: true,
			})
			continue
		}

		// Local function (local function name())
		if m := localFuncRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{
				Name:     m[1],
				Type:     "function",
				Line:     lineNum,
				Exported: false,
			})
			continue
		}

		// Local constant (local NAME = value)
		if m := localConstRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{
				Name:     m[1],
				Type:     "constant",
				Line:     lineNum,
				Exported: false,
			})
			continue
		}

		// Class table (Name = {}) - only if it's a known class
		if m := classTableRe.FindStringSubmatch(trimmed); m != nil {
			name := m[1]
			if classNames[name] {
				symbols = append(symbols, Symbol{
					Name:     name,
					Type:     "class",
					Line:     lineNum,
					Exported: true,
				})
				continue
			}
		}

		// Enum-like table (Name = { with content)
		if m := enumTableRe.FindStringSubmatch(trimmed); m != nil {
			name := m[1]
			// Skip if it's a known class
			if classNames[name] {
				symbols = append(symbols, Symbol{
					Name:     name,
					Type:     "class",
					Line:     lineNum,
					Exported: true,
				})
			} else if !strings.HasSuffix(trimmed, "{}") {
				// It's an enum-like table with content
				symbols = append(symbols, Symbol{
					Name:     name,
					Type:     "enum",
					Line:     lineNum,
					Exported: true,
				})
			}
			continue
		}

		// Global constant (NAME = value, uppercase, not a table)
		if m := globalConstRe.FindStringSubmatch(trimmed); m != nil {
			name := m[1]
			// Skip if already handled as class/enum
			alreadyHandled := false
			for _, s := range symbols {
				if s.Name == name {
					alreadyHandled = true
					break
				}
			}
			if !alreadyHandled {
				symbols = append(symbols, Symbol{
					Name:     name,
					Type:     "constant",
					Line:     lineNum,
					Exported: true,
				})
			}
			continue
		}
	}

	return symbols
}
