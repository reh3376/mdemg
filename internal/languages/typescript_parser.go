package languages

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&TypeScriptParser{})
}

// TypeScriptParser implements LanguageParser for TypeScript/JavaScript files
type TypeScriptParser struct{}

func (p *TypeScriptParser) Name() string {
	return "typescript"
}

func (p *TypeScriptParser) Extensions() []string {
	return []string{".ts", ".tsx", ".js", ".jsx"}
}

func (p *TypeScriptParser) CanParse(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.HasSuffix(pathLower, ".ts") ||
		strings.HasSuffix(pathLower, ".tsx") ||
		strings.HasSuffix(pathLower, ".js") ||
		strings.HasSuffix(pathLower, ".jsx")
}

func (p *TypeScriptParser) IsTestFile(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.HasSuffix(pathLower, ".test.ts") ||
		strings.HasSuffix(pathLower, ".test.tsx") ||
		strings.HasSuffix(pathLower, ".test.js") ||
		strings.HasSuffix(pathLower, ".test.jsx") ||
		strings.HasSuffix(pathLower, ".spec.ts") ||
		strings.HasSuffix(pathLower, ".spec.tsx") ||
		strings.HasSuffix(pathLower, ".spec.js") ||
		strings.HasSuffix(pathLower, ".spec.jsx") ||
		strings.Contains(path, "/__tests__/")
}

func (p *TypeScriptParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	var elements []CodeElement

	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)

	// Extract module name
	moduleName := strings.TrimSuffix(fileName, filepath.Ext(fileName))

	// Find classes, interfaces, functions
	classes := FindAllMatches(content, `(?:export\s+)?class\s+(\w+)`)
	interfaces := FindAllMatches(content, `(?:export\s+)?interface\s+(\w+)`)
	functions := FindAllMatches(content, `(?:export\s+)?(?:async\s+)?function\s+(\w+)`)
	arrowFuncs := FindAllMatches(content, `(?:export\s+)?const\s+(\w+)\s*=\s*(?:async\s+)?\([^)]*\)\s*(?::\s*[^=]+)?\s*=>`)
	imports := FindAllMatches(content, `import\s+.*?\s+from\s+['"]([^'"]+)['"]`)

	// Check for NestJS decorators
	decorators := FindAllMatches(content, `@(\w+)\(`)

	// Build content for embedding
	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("TypeScript/JavaScript module: %s\n", moduleName))
	contentBuilder.WriteString(fmt.Sprintf("File: %s\n", relPath))

	if len(classes) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Classes: %s\n", strings.Join(uniqueStrings(classes), ", ")))
	}
	if len(interfaces) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Interfaces: %s\n", strings.Join(uniqueStrings(interfaces), ", ")))
	}

	allFuncs := append(uniqueStrings(functions), uniqueStrings(arrowFuncs)...)
	if len(allFuncs) > 0 {
		if len(allFuncs) > 15 {
			allFuncs = allFuncs[:15]
			contentBuilder.WriteString(fmt.Sprintf("Functions: %s (and more)\n", strings.Join(allFuncs, ", ")))
		} else {
			contentBuilder.WriteString(fmt.Sprintf("Functions: %s\n", strings.Join(allFuncs, ", ")))
		}
	}

	if len(decorators) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Decorators: %s\n", strings.Join(uniqueStrings(decorators), ", ")))
	}
	contentBuilder.WriteString(fmt.Sprintf("Imports: %d\n", len(imports)))

	// Include actual code content
	contentBuilder.WriteString("\n--- Code ---\n")
	contentBuilder.WriteString(TruncateContent(content, 4000))

	// Detect cross-cutting concerns
	concerns := DetectConcerns(relPath, content)
	tags := []string{"typescript", "module"}
	tags = append(tags, concerns...)

	// Add framework-specific tags
	if containsAny(decorators, []string{"Controller", "Injectable", "Module", "Guard"}) {
		tags = append(tags, "nestjs")
	}
	if containsAny(content, []string{"React", "useState", "useEffect"}) {
		tags = append(tags, "react")
	}
	if containsAny(content, []string{"@angular", "NgModule"}) {
		tags = append(tags, "angular")
	}

	// Extract code symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	// Add file-level element
	elements = append(elements, CodeElement{
		Name:     moduleName,
		Kind:     "module",
		Path:     "/" + relPath,
		Content:  contentBuilder.String(),
		Package:  moduleName,
		FilePath: relPath,
		Tags:     tags,
		Concerns: concerns,
		Symbols:  symbols,
	})

	// Add classes as separate elements
	for _, cls := range uniqueStrings(classes) {
		elements = append(elements, CodeElement{
			Name:     cls,
			Kind:     "class",
			Path:     fmt.Sprintf("/%s#%s", relPath, cls),
			Content:  fmt.Sprintf("TypeScript class '%s' in module %s", cls, moduleName),
			Package:  moduleName,
			FilePath: relPath,
			Tags:     append([]string{"typescript", "class"}, concerns...),
			Concerns: concerns,
		})
	}

	// Add interfaces as separate elements
	for _, iface := range uniqueStrings(interfaces) {
		elements = append(elements, CodeElement{
			Name:     iface,
			Kind:     "interface",
			Path:     fmt.Sprintf("/%s#%s", relPath, iface),
			Content:  fmt.Sprintf("TypeScript interface '%s' in module %s", iface, moduleName),
			Package:  moduleName,
			FilePath: relPath,
			Tags:     append([]string{"typescript", "interface"}, concerns...),
			Concerns: concerns,
		})
	}

	return elements, nil
}

func (p *TypeScriptParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	// Patterns for various TypeScript constructs
	// Constants: const NAME = value (UPPER_CASE or camelCase exported)
	constPattern := regexp.MustCompile(`^(?:export\s+)?const\s+([A-Z][A-Z0-9_]*)\s*(?::\s*[^=]+)?\s*=\s*(.+)$`)
	// Functions: export function name(args): Type
	fnPattern := regexp.MustCompile(`^(?:export\s+)?(?:async\s+)?function\s+(\w+)\s*\(([^)]*)\)(?:\s*:\s*([^\{]+))?`)
	// Arrow functions: export const name = (args): Type =>
	arrowPattern := regexp.MustCompile(`^(?:export\s+)?const\s+(\w+)\s*=\s*(?:async\s+)?\(([^)]*)\)(?:\s*:\s*([^=]+))?\s*=>`)
	// Classes: export class ClassName extends/implements
	classPattern := regexp.MustCompile(`^(?:@\w+\([^)]*\)\s*)*(?:export\s+)?(?:abstract\s+)?class\s+(\w+)(?:\s+extends\s+(\w+))?(?:\s+implements\s+([^{]+))?`)
	// Interfaces: export interface InterfaceName extends
	interfacePattern := regexp.MustCompile(`^(?:export\s+)?interface\s+(\w+)(?:\s+extends\s+([^{]+))?`)
	// Type aliases: export type TypeName = ...
	typePattern := regexp.MustCompile(`^(?:export\s+)?type\s+(\w+)(?:<[^>]+>)?\s*=`)
	// Enums: export enum EnumName
	enumPattern := regexp.MustCompile(`^(?:export\s+)?(?:const\s+)?enum\s+(\w+)`)
	// Methods: async methodName(args): Type or methodName(args): Type
	methodPattern := regexp.MustCompile(`^\s+(?:public\s+|private\s+|protected\s+)?(?:static\s+)?(?:async\s+)?(\w+)\s*\(([^)]*)\)(?:\s*:\s*([^{]+))?(?:\s*\{)?$`)
	// Decorators on next line's class/method
	decoratorPattern := regexp.MustCompile(`^\s*@(\w+)\(`)

	var currentClass string
	var pendingDecorators []string

	for i, line := range lines {
		lineNum := i + 1
		trimmedLine := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmedLine == "" || strings.HasPrefix(trimmedLine, "//") || strings.HasPrefix(trimmedLine, "/*") || strings.HasPrefix(trimmedLine, "*") {
			continue
		}

		// Track decorators for the next declaration
		if matches := decoratorPattern.FindStringSubmatch(trimmedLine); matches != nil {
			pendingDecorators = append(pendingDecorators, matches[1])
			continue
		}

		// Extract classes
		if matches := classPattern.FindStringSubmatch(trimmedLine); matches != nil {
			className := matches[1]
			extends := ""
			implements := ""
			if len(matches) > 2 && matches[2] != "" {
				extends = matches[2]
			}
			if len(matches) > 3 && matches[3] != "" {
				implements = strings.TrimSpace(matches[3])
			}

			docComment := ""
			if len(pendingDecorators) > 0 {
				docComment = "Decorators: @" + strings.Join(pendingDecorators, ", @")
			}

			sym := Symbol{
				Name:       className,
				Type:       "class",
				Signature:  fmt.Sprintf("class %s", className),
				DocComment: docComment,
				Line: lineNum,
				Exported:   strings.Contains(trimmedLine, "export"),
				Language:   "typescript",
			}
			if extends != "" {
				sym.Signature += " extends " + extends
			}
			if implements != "" {
				sym.Signature += " implements " + implements
			}
			symbols = append(symbols, sym)
			currentClass = className
			pendingDecorators = nil
			continue
		}

		// Extract interfaces
		if matches := interfacePattern.FindStringSubmatch(trimmedLine); matches != nil {
			interfaceName := matches[1]
			extends := ""
			if len(matches) > 2 && matches[2] != "" {
				extends = strings.TrimSpace(matches[2])
			}

			sym := Symbol{
				Name:       interfaceName,
				Type:       "interface",
				Signature:  fmt.Sprintf("interface %s", interfaceName),
				Line: lineNum,
				Exported:   strings.Contains(trimmedLine, "export"),
				Language:   "typescript",
			}
			if extends != "" {
				sym.Signature += " extends " + extends
			}
			symbols = append(symbols, sym)
			pendingDecorators = nil
			continue
		}

		// Extract type aliases
		if matches := typePattern.FindStringSubmatch(trimmedLine); matches != nil {
			sym := Symbol{
				Name:       matches[1],
				Type:       "type",
				Signature:  fmt.Sprintf("type %s", matches[1]),
				Line: lineNum,
				Exported:   strings.Contains(trimmedLine, "export"),
				Language:   "typescript",
			}
			symbols = append(symbols, sym)
			pendingDecorators = nil
			continue
		}

		// Extract enums
		if matches := enumPattern.FindStringSubmatch(trimmedLine); matches != nil {
			sym := Symbol{
				Name:       matches[1],
				Type:       "enum",
				Signature:  fmt.Sprintf("enum %s", matches[1]),
				Line: lineNum,
				Exported:   strings.Contains(trimmedLine, "export"),
				Language:   "typescript",
			}
			symbols = append(symbols, sym)
			pendingDecorators = nil
			continue
		}

		// Extract constants (UPPER_CASE)
		if matches := constPattern.FindStringSubmatch(trimmedLine); matches != nil {
			sym := Symbol{
				Name:       matches[1],
				Type:       "constant",
				Value:      CleanValue(matches[2]),
				RawValue:   matches[2],
				Line: lineNum,
				Exported:   strings.HasPrefix(trimmedLine, "export"),
				Language:   "typescript",
			}
			symbols = append(symbols, sym)
			pendingDecorators = nil
			continue
		}

		// Extract function definitions (top-level)
		if matches := fnPattern.FindStringSubmatch(trimmedLine); matches != nil {
			returnType := ""
			if len(matches) > 3 && matches[3] != "" {
				returnType = strings.TrimSpace(matches[3])
			}

			sym := Symbol{
				Name:           matches[1],
				Type:           "function",
				Signature:      fmt.Sprintf("function %s(%s)%s", matches[1], matches[2], formatTSReturn(returnType)),
				TypeAnnotation: returnType,
				Line:     lineNum,
				Exported:       strings.HasPrefix(trimmedLine, "export"),
				Language:       "typescript",
			}
			symbols = append(symbols, sym)
			pendingDecorators = nil
			continue
		}

		// Extract arrow functions
		if matches := arrowPattern.FindStringSubmatch(trimmedLine); matches != nil {
			returnType := ""
			if len(matches) > 3 && matches[3] != "" {
				returnType = strings.TrimSpace(matches[3])
			}

			sym := Symbol{
				Name:           matches[1],
				Type:           "function",
				Signature:      fmt.Sprintf("const %s = (%s)%s =>", matches[1], matches[2], formatTSReturn(returnType)),
				TypeAnnotation: returnType,
				Line:     lineNum,
				Exported:       strings.HasPrefix(trimmedLine, "export"),
				Language:       "typescript",
			}
			symbols = append(symbols, sym)
			pendingDecorators = nil
			continue
		}

		// Extract methods (inside classes) - must start with whitespace
		if currentClass != "" && strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t") {
			if matches := methodPattern.FindStringSubmatch(line); matches != nil {
				methodName := matches[1]
				// Skip constructor and common non-method patterns
				if methodName == "constructor" || methodName == "if" || methodName == "for" || methodName == "while" || methodName == "switch" || methodName == "return" || methodName == "throw" {
					pendingDecorators = nil
					continue
				}

				returnType := ""
				if len(matches) > 3 && matches[3] != "" {
					returnType = strings.TrimSpace(matches[3])
				}

				docComment := ""
				if len(pendingDecorators) > 0 {
					docComment = "Decorators: @" + strings.Join(pendingDecorators, ", @")
				}

				sym := Symbol{
					Name:           methodName,
					Type:           "method",
					Parent:         currentClass,
					Signature:      fmt.Sprintf("%s.%s(%s)%s", currentClass, methodName, matches[2], formatTSReturn(returnType)),
					TypeAnnotation: returnType,
					DocComment:     docComment,
					Line:     lineNum,
					Exported:       true, // Methods are accessible via class
					Language:       "typescript",
				}
				symbols = append(symbols, sym)
				pendingDecorators = nil
			}
		}

		// Reset class context at closing brace (simple heuristic)
		if trimmedLine == "}" && currentClass != "" {
			// Check if this might be end of class (no indentation)
			if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				currentClass = ""
			}
		}

		// Clear pending decorators if we hit a line that didn't use them
		if len(pendingDecorators) > 0 && !strings.HasPrefix(trimmedLine, "@") {
			pendingDecorators = nil
		}
	}

	return symbols
}

// containsAny checks if any of the patterns appear in the string
func containsAny(s interface{}, patterns []string) bool {
	var str string
	switch v := s.(type) {
	case string:
		str = v
	case []string:
		str = strings.Join(v, " ")
	default:
		return false
	}
	for _, pattern := range patterns {
		if strings.Contains(str, pattern) {
			return true
		}
	}
	return false
}

// formatTSReturn formats a return type for display
func formatTSReturn(returnType string) string {
	if returnType == "" {
		return ""
	}
	return ": " + returnType
}
