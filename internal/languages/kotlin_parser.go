package languages

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&KotlinParser{})
}

// KotlinParser implements LanguageParser for Kotlin source files (.kt, .kts)
type KotlinParser struct{}

func (p *KotlinParser) Name() string {
	return "kotlin"
}

func (p *KotlinParser) Extensions() []string {
	return []string{".kt", ".kts"}
}

func (p *KotlinParser) CanParse(path string) bool {
	return HasExtension(path, p.Extensions())
}

func (p *KotlinParser) IsTestFile(path string) bool {
	name := filepath.Base(path)
	return strings.HasSuffix(name, "Test.kt") ||
		strings.HasSuffix(name, "Tests.kt") ||
		strings.HasSuffix(name, "Spec.kt") ||
		strings.HasSuffix(name, "Test.kts") ||
		strings.Contains(path, "/test/") ||
		strings.Contains(path, "/tests/") ||
		strings.Contains(path, "/androidTest/")
}

func (p *KotlinParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	var elements []CodeElement

	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)

	// Extract package name
	packageName := ""
	packageMatch := regexp.MustCompile(`^\s*package\s+([\w.]+)`).FindStringSubmatch(content)
	if packageMatch != nil {
		packageName = packageMatch[1]
	}

	// Find top-level declarations for summary
	classes := FindAllMatches(content, `(?:data\s+|sealed\s+|abstract\s+|open\s+|inner\s+)?class\s+(\w+)`)
	objects := FindAllMatches(content, `(?:^|\s)object\s+(\w+)`)
	interfaces := FindAllMatches(content, `(?:^|\s)interface\s+(\w+)`)
	enums := FindAllMatches(content, `enum\s+class\s+(\w+)`)
	functions := FindAllMatches(content, `(?:^|\s)fun\s+(?:\w+\.)?(\w+)\s*\(`)
	imports := FindAllMatches(content, `^\s*import\s+([\w.*]+)`)

	// Build content for embedding
	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("Kotlin file: %s\n", fileName))

	if packageName != "" {
		contentBuilder.WriteString(fmt.Sprintf("Package: %s\n", packageName))
	}
	if len(classes) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Classes: %s\n", strings.Join(uniqueStrings(classes), ", ")))
	}
	if len(objects) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Objects: %s\n", strings.Join(uniqueStrings(objects), ", ")))
	}
	if len(interfaces) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Interfaces: %s\n", strings.Join(uniqueStrings(interfaces), ", ")))
	}
	if len(enums) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Enums: %s\n", strings.Join(uniqueStrings(enums), ", ")))
	}
	if len(functions) > 0 {
		funcList := uniqueStrings(functions)
		if len(funcList) > 15 {
			funcList = funcList[:15]
			contentBuilder.WriteString(fmt.Sprintf("Functions: %s (and more)\n", strings.Join(funcList, ", ")))
		} else {
			contentBuilder.WriteString(fmt.Sprintf("Functions: %s\n", strings.Join(funcList, ", ")))
		}
	}
	contentBuilder.WriteString(fmt.Sprintf("Imports: %d\n", len(imports)))

	// Include actual code content
	contentBuilder.WriteString("\n--- Code ---\n")
	truncated, wasTruncated := TruncateContentWithInfo(content, 4000)
	contentBuilder.WriteString(truncated)

	// Collect diagnostics
	var diagnostics []Diagnostic
	if wasTruncated {
		diagnostics = append(diagnostics, NewDiagnosticWithContext(
			"info", "TRUNCATED",
			fmt.Sprintf("Content truncated from %d to 4000 chars", len(content)),
			"kotlin",
			map[string]string{"original_size": fmt.Sprintf("%d", len(content))},
		))
	}

	// Detect cross-cutting concerns
	concerns := DetectConcerns(relPath, content)
	tags := []string{"kotlin", "module"}
	tags = append(tags, concerns...)

	// Determine file kind based on content
	kotlinKind := "kotlin-class"
	if len(interfaces) > 0 && len(classes) == 0 && len(objects) == 0 {
		kotlinKind = "kotlin-interface"
		tags = append(tags, "interface")
	} else if len(enums) > 0 && len(classes) == 0 {
		kotlinKind = "kotlin-enum"
		tags = append(tags, "enum")
	} else if len(objects) > 0 && len(classes) == 0 {
		kotlinKind = "kotlin-object"
		tags = append(tags, "object")
	}

	// Detect Kotlin script
	if strings.HasSuffix(path, ".kts") {
		tags = append(tags, "script")
	}

	// Extract code symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	// Add file-level element
	elements = append(elements, CodeElement{
		Name:        fileName,
		Kind:        kotlinKind,
		Path:        "/" + relPath,
		Content:     contentBuilder.String(),
		Package:     packageName,
		FilePath:    relPath,
		Tags:        tags,
		Concerns:    concerns,
		Symbols:     symbols,
		Diagnostics: diagnostics,
	})

	// Add classes as separate elements
	for _, class := range uniqueStrings(classes) {
		elements = append(elements, CodeElement{
			Name:     class,
			Kind:     "class",
			Path:     fmt.Sprintf("/%s#%s", relPath, class),
			Content:  fmt.Sprintf("Kotlin class '%s' in package %s (file: %s)", class, packageName, fileName),
			Package:  packageName,
			FilePath: relPath,
			Tags:     append([]string{"kotlin", "class"}, concerns...),
			Concerns: concerns,
		})
	}

	// Add interfaces as separate elements
	for _, iface := range uniqueStrings(interfaces) {
		elements = append(elements, CodeElement{
			Name:     iface,
			Kind:     "interface",
			Path:     fmt.Sprintf("/%s#%s", relPath, iface),
			Content:  fmt.Sprintf("Kotlin interface '%s' in package %s (file: %s)", iface, packageName, fileName),
			Package:  packageName,
			FilePath: relPath,
			Tags:     append([]string{"kotlin", "interface"}, concerns...),
			Concerns: concerns,
		})
	}

	// Add objects as separate elements
	for _, obj := range uniqueStrings(objects) {
		elements = append(elements, CodeElement{
			Name:     obj,
			Kind:     "class",
			Path:     fmt.Sprintf("/%s#%s", relPath, obj),
			Content:  fmt.Sprintf("Kotlin object '%s' in package %s (file: %s)", obj, packageName, fileName),
			Package:  packageName,
			FilePath: relPath,
			Tags:     append([]string{"kotlin", "object"}, concerns...),
			Concerns: concerns,
		})
	}

	return elements, nil
}

func (p *KotlinParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	// --- Compiled patterns ---
	packagePattern := regexp.MustCompile(`^\s*package\s+[\w.]+`)
	importPattern := regexp.MustCompile(`^\s*import\s+`)
	annotationPattern := regexp.MustCompile(`^\s*@(\w+)`)

	typealiasPattern := regexp.MustCompile(`^\s*(?:(?:public|private|internal)\s+)?typealias\s+(\w+)\s*=\s*(.+)`)

	// Class patterns: data class, sealed class, abstract class, open class, inner class, plain class
	classPattern := regexp.MustCompile(`^\s*(?:@\w+\s+)*(?:(?:public|private|internal|protected)\s+)?(?:(?:data|sealed|abstract|open|inner)\s+)*class\s+(\w+)`)
	objectPattern := regexp.MustCompile(`^\s*(?:@\w+\s+)*(?:(?:public|private|internal|protected)\s+)?object\s+(\w+)`)
	companionPattern := regexp.MustCompile(`^\s*companion\s+object`)
	interfacePattern := regexp.MustCompile(`^\s*(?:@\w+\s+)*(?:(?:public|private|internal|protected)\s+)?interface\s+(\w+)`)
	enumClassPattern := regexp.MustCompile(`^\s*(?:@\w+\s+)*(?:(?:public|private|internal|protected)\s+)?enum\s+class\s+(\w+)`)

	// Enum value pattern: UPPERCASE_NAME optionally followed by (args) or { ... }
	enumValuePattern := regexp.MustCompile(`^\s*([A-Z][A-Z0-9_]*)\s*(?:\([^)]*\))?\s*[,;{]`)

	// Function pattern: [modifiers] fun [ReceiverType.]functionName(params)[: ReturnType]
	funPattern := regexp.MustCompile(`^\s*(?:@\w+\s+)*(?:(?:public|private|internal|protected|override|open|abstract|final|inline|suspend|tailrec|operator|infix|external)\s+)*fun\s+(?:(?:\w+(?:<[^>]*>)?)\.)?\s*(\w+)\s*\(([^)]*)\)(?:\s*:\s*(\S+))?`)

	// Constant pattern: const val NAME = value
	constValPattern := regexp.MustCompile(`^\s*(?:(?:public|private|internal|protected|override)\s+)*const\s+val\s+(\w+)\s*(?::\s*\w+)?\s*=\s*(.+)`)

	// Top-level val/var pattern (captured only at top level, braceDepth == 0)
	topLevelValPattern := regexp.MustCompile(`^\s*(?:(?:public|private|internal|protected|override)\s+)*(?:val|var)\s+(\w+)\s*(?::\s*[^=]+)?\s*=\s*(.+)`)

	// Scope tracking
	type scopeEntry struct {
		name string
		kind string // "class", "interface", "enum", "object", "companion"
		depth int
	}
	var scopeStack []scopeEntry
	braceDepth := 0
	parenDepth := 0
	inEnumValues := false
	pendingAnnotation := ""

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and pure comments
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			goto countBraces
		}

		// Skip package and import declarations
		if packagePattern.MatchString(trimmed) || importPattern.MatchString(trimmed) {
			goto countBraces
		}

		// Skip lines inside multi-line parenthetical blocks (e.g., data class constructor params)
		if parenDepth > 0 {
			goto countBraces
		}

		// Capture annotations for the next declaration
		if annotationPattern.MatchString(trimmed) && !funPattern.MatchString(trimmed) && !classPattern.MatchString(trimmed) {
			matches := annotationPattern.FindStringSubmatch(trimmed)
			if matches != nil {
				pendingAnnotation = "@" + matches[1]
			}
			// If the annotation line also has something else, keep processing
			// But if it's annotation-only, skip
			if !strings.Contains(trimmed, "class ") && !strings.Contains(trimmed, "fun ") &&
				!strings.Contains(trimmed, "val ") && !strings.Contains(trimmed, "var ") &&
				!strings.Contains(trimmed, "object ") && !strings.Contains(trimmed, "interface ") {
				goto countBraces
			}
		}

		{
			// Get current parent from scope stack
			parent := ""
			if len(scopeStack) > 0 {
				parent = scopeStack[len(scopeStack)-1].name
			}

			// Determine visibility from leading modifiers only (before the declaration keyword).
			// We extract the prefix before any keyword (class, fun, val, var, object, interface, etc.)
			// to avoid false positives from "private" inside constructor parameters.
			exported := kotlinIsExported(trimmed)

			// Check for typealias (top-level only)
			if matches := typealiasPattern.FindStringSubmatch(trimmed); matches != nil {
				sym := Symbol{
					Name:     matches[1],
					Type:     "type",
					Line:     lineNum,
					Parent:   parent,
					Exported: exported,
					Language: "kotlin",
				}
				if pendingAnnotation != "" {
					sym.DocComment = pendingAnnotation
					pendingAnnotation = ""
				}
				symbols = append(symbols, sym)
				goto countBraces
			}

			// Helper: check if line contains an opening brace (indicates a body block)
			lineHasBrace := strings.Contains(line, "{")

			// Check for enum class (must be before class pattern)
			if matches := enumClassPattern.FindStringSubmatch(trimmed); matches != nil {
				enumName := matches[1]
				sym := Symbol{
					Name:     enumName,
					Type:     "enum",
					Line:     lineNum,
					Parent:   parent,
					Exported: exported,
					Language: "kotlin",
				}
				if pendingAnnotation != "" {
					sym.DocComment = pendingAnnotation
					pendingAnnotation = ""
				}
				symbols = append(symbols, sym)
				if lineHasBrace {
					scopeStack = append(scopeStack, scopeEntry{name: enumName, kind: "enum", depth: braceDepth})
					inEnumValues = true
				}
				goto countBraces
			}

			// Check for companion object (scope container, not a symbol)
			if companionPattern.MatchString(trimmed) {
				// Push companion scope; parent stays the enclosing class
				if lineHasBrace {
					scopeStack = append(scopeStack, scopeEntry{name: parent, kind: "companion", depth: braceDepth})
				}
				pendingAnnotation = ""
				goto countBraces
			}

			// Check for object declaration
			if matches := objectPattern.FindStringSubmatch(trimmed); matches != nil {
				objName := matches[1]
				sym := Symbol{
					Name:     objName,
					Type:     "class",
					Line:     lineNum,
					Parent:   parent,
					Exported: exported,
					Language: "kotlin",
				}
				if pendingAnnotation != "" {
					sym.DocComment = pendingAnnotation
					pendingAnnotation = ""
				}
				symbols = append(symbols, sym)
				if lineHasBrace {
					scopeStack = append(scopeStack, scopeEntry{name: objName, kind: "object", depth: braceDepth})
				}
				goto countBraces
			}

			// Check for interface declaration
			if matches := interfacePattern.FindStringSubmatch(trimmed); matches != nil {
				ifaceName := matches[1]
				sym := Symbol{
					Name:     ifaceName,
					Type:     "interface",
					Line:     lineNum,
					Parent:   parent,
					Exported: exported,
					Language: "kotlin",
				}
				if pendingAnnotation != "" {
					sym.DocComment = pendingAnnotation
					pendingAnnotation = ""
				}
				symbols = append(symbols, sym)
				if lineHasBrace {
					scopeStack = append(scopeStack, scopeEntry{name: ifaceName, kind: "interface", depth: braceDepth})
				}
				goto countBraces
			}

			// Check for class declaration (data class, sealed class, abstract class, etc.)
			if matches := classPattern.FindStringSubmatch(trimmed); matches != nil {
				className := matches[1]
				sym := Symbol{
					Name:     className,
					Type:     "class",
					Line:     lineNum,
					Parent:   parent,
					Exported: exported,
					Language: "kotlin",
				}
				if pendingAnnotation != "" {
					sym.DocComment = pendingAnnotation
					pendingAnnotation = ""
				}
				symbols = append(symbols, sym)
				inEnumValues = false
				if lineHasBrace {
					scopeStack = append(scopeStack, scopeEntry{name: className, kind: "class", depth: braceDepth})
				}
				goto countBraces
			}

			// Extract enum values (inside enum body, before semicolon or method)
			if inEnumValues {
				if matches := enumValuePattern.FindStringSubmatch(trimmed); matches != nil {
					symbols = append(symbols, Symbol{
						Name:     matches[1],
						Type:     "enum_value",
						Line:     lineNum,
						Parent:   parent,
						Exported: true,
						Language: "kotlin",
					})
					// Check if this line ends the enum value section
					if strings.Contains(trimmed, ";") {
						inEnumValues = false
					}
					pendingAnnotation = ""
					goto countBraces
				}
				// Non-enum-value line inside enum -> values section is over
				if !strings.HasPrefix(trimmed, "//") && trimmed != "" {
					inEnumValues = false
				}
			}

			// Check for const val (constant)
			if matches := constValPattern.FindStringSubmatch(trimmed); matches != nil {
				valueStr := CleanValue(matches[2])
				if len(valueStr) > 100 {
					valueStr = valueStr[:100] + "..."
				}
				sym := Symbol{
					Name:     matches[1],
					Type:     "constant",
					Value:    valueStr,
					Line:     lineNum,
					Parent:   parent,
					Exported: exported,
					Language: "kotlin",
				}
				if pendingAnnotation != "" {
					sym.DocComment = pendingAnnotation
					pendingAnnotation = ""
				}
				symbols = append(symbols, sym)
				goto countBraces
			}

			// Check for top-level val/var (only at brace depth 0 and not inside parens)
			if braceDepth == 0 && parenDepth == 0 {
				if matches := topLevelValPattern.FindStringSubmatch(trimmed); matches != nil {
					valueStr := CleanValue(matches[2])
					if len(valueStr) > 100 {
						valueStr = valueStr[:100] + "..."
					}
					sym := Symbol{
						Name:     matches[1],
						Type:     "constant",
						Value:    valueStr,
						Line:     lineNum,
						Parent:   parent,
						Exported: exported,
						Language: "kotlin",
					}
					if pendingAnnotation != "" {
						sym.DocComment = pendingAnnotation
						pendingAnnotation = ""
					}
					symbols = append(symbols, sym)
					goto countBraces
				}
			}

			// Check for function/method declarations
			if matches := funPattern.FindStringSubmatch(trimmed); matches != nil {
				funcName := matches[1]
				params := matches[2]
				returnType := matches[3]

				symType := "function"
				if parent != "" {
					symType = "method"
				}

				signature := fmt.Sprintf("fun %s(%s)", funcName, params)
				if returnType != "" {
					signature = fmt.Sprintf("fun %s(%s): %s", funcName, params, returnType)
				}
				if len(signature) > 150 {
					signature = signature[:150] + "..."
				}

				sym := Symbol{
					Name:           funcName,
					Type:           symType,
					Signature:      signature,
					TypeAnnotation: returnType,
					Line:           lineNum,
					Parent:         parent,
					Exported:       exported,
					Language:       "kotlin",
				}
				if pendingAnnotation != "" {
					sym.DocComment = pendingAnnotation
					pendingAnnotation = ""
				}
				symbols = append(symbols, sym)
				goto countBraces
			}
		}

	countBraces:
		// Track brace and paren depth for scope management
		for _, ch := range line {
			switch ch {
			case '(':
				parenDepth++
			case ')':
				if parenDepth > 0 {
					parenDepth--
				}
			case '{':
				braceDepth++
			case '}':
				braceDepth--
				for len(scopeStack) > 0 && scopeStack[len(scopeStack)-1].depth >= braceDepth {
					scopeStack = scopeStack[:len(scopeStack)-1]
				}
			}
		}
	}

	return symbols
}

// kotlinIsExported examines the leading modifiers of a Kotlin declaration line
// and returns true if the declaration is exported (not private or internal).
// Kotlin defaults to public visibility, so we only check for explicit restrictors.
func kotlinIsExported(trimmed string) bool {
	// Find the prefix before the first declaration keyword.
	// Declaration keywords: class, fun, val, var, object, interface, enum, typealias, const
	keywords := []string{"class ", "fun ", "val ", "var ", "object ", "interface ", "enum ", "typealias ", "const "}
	prefix := trimmed
	for _, kw := range keywords {
		if idx := strings.Index(trimmed, kw); idx >= 0 {
			candidate := trimmed[:idx]
			if len(candidate) < len(prefix) {
				prefix = candidate
			}
		}
	}
	// Check the prefix (modifiers before keyword) for visibility restrictors
	return !strings.Contains(prefix, "private") && !strings.Contains(prefix, "internal")
}
