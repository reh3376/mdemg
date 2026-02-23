package languages

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&PythonParser{})
}

// PythonParser implements LanguageParser for Python source files
type PythonParser struct{}

func (p *PythonParser) Name() string {
	return "python"
}

func (p *PythonParser) Extensions() []string {
	return []string{".py"}
}

func (p *PythonParser) CanParse(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".py")
}

func (p *PythonParser) IsTestFile(path string) bool {
	name := filepath.Base(path)
	return strings.HasPrefix(name, "test_") ||
		strings.HasSuffix(name, "_test.py") ||
		strings.Contains(path, "/tests/") ||
		strings.Contains(path, "/test/")
}

func (p *PythonParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	var elements []CodeElement

	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)

	// Extract module name
	moduleName := strings.TrimSuffix(fileName, ".py")

	// Find classes, functions, and imports
	// Use (?m)^ to match at line start, require colon or parenthesis after name
	// This avoids matching "class that" in docstrings like "A dummy class that..."
	classes := FindAllMatches(content, `(?m)^\s*class\s+(\w+)\s*[:\(]`)
	functions := FindAllMatches(content, `(?m)^\s*def\s+(\w+)\s*\(`)
	imports := FindAllMatches(content, `^(?:from\s+[\w.]+\s+)?import\s+([\w.]+)`)

	// Build content for embedding
	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("Python module: %s\n", moduleName))
	contentBuilder.WriteString(fmt.Sprintf("File: %s\n", relPath))

	if len(classes) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Classes: %s\n", strings.Join(uniqueStrings(classes), ", ")))
	}
	if len(functions) > 0 {
		fnList := uniqueStrings(functions)
		// Filter out dunder methods for summary
		publicFuncs := filterDunderMethods(fnList)
		if len(publicFuncs) > 0 {
			if len(publicFuncs) > 15 {
				publicFuncs = publicFuncs[:15]
				contentBuilder.WriteString(fmt.Sprintf("Functions: %s (and more)\n", strings.Join(publicFuncs, ", ")))
			} else {
				contentBuilder.WriteString(fmt.Sprintf("Functions: %s\n", strings.Join(publicFuncs, ", ")))
			}
		}
	}
	contentBuilder.WriteString(fmt.Sprintf("Imports: %d\n", len(imports)))

	// Include actual code content
	contentBuilder.WriteString("\n--- Code ---\n")
	contentBuilder.WriteString(TruncateContent(content, 4000))

	// Detect cross-cutting concerns
	concerns := DetectConcerns(relPath, content)
	tags := []string{"python", "module"}
	tags = append(tags, concerns...)

	// Determine file kind
	pyKind := "module"
	if fileName == "__init__.py" {
		pyKind = "package"
		tags = append(tags, "package")
	} else if fileName == "__main__.py" {
		pyKind = "entrypoint"
		tags = append(tags, "entrypoint")
	}

	// Extract code symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	// Add file-level element
	elements = append(elements, CodeElement{
		Name:     moduleName,
		Kind:     pyKind,
		Path:     "/" + relPath,
		Content:  contentBuilder.String(),
		Package:  moduleName,
		FilePath: relPath,
		Tags:     tags,
		Concerns: concerns,
		Symbols:  symbols,
	})

	// Add classes as separate elements with enhanced content for better retrieval
	for _, cls := range uniqueStrings(classes) {
		// Try to extract class definition and docstring for better embedding
		classContent := extractClassContent(content, cls, moduleName)
		elements = append(elements, CodeElement{
			Name:     cls,
			Kind:     "class",
			Path:     fmt.Sprintf("/%s#%s", relPath, cls),
			Content:  classContent,
			Package:  moduleName,
			FilePath: relPath,
			Tags:     append([]string{"python", "class"}, concerns...),
			Concerns: concerns,
		})
	}

	return elements, nil
}

func (p *PythonParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	// Patterns for various Python constructs
	// Constants: UPPER_CASE = value (at module level)
	constPattern := regexp.MustCompile(`^([A-Z][A-Z0-9_]*)\s*=\s*(.+)$`)
	// Private constants: _UPPER_CASE = value (at module level)
	privateConstPattern := regexp.MustCompile(`^(_[A-Z][A-Z0-9_]*)\s*=\s*(.+)$`)
	// Type aliases: TypeName = SomeType (CamelCase = type at module level)
	typeAliasPattern := regexp.MustCompile(`^([A-Z][a-zA-Z0-9]*)\s*=\s*(str|int|float|bool|List\[|Dict\[|Set\[|Tuple\[|Optional\[|Union\[)`)
	// Function: def function_name(args) -> Type: or async def function_name(args) -> Type:
	fnPattern := regexp.MustCompile(`^(async\s+)?def\s+(\w+)\s*\(([^)]*)\)(?:\s*->\s*([^:]+))?:`)
	// Class: class ClassName(base):
	classPattern := regexp.MustCompile(`^class\s+(\w+)(?:\s*\(([^)]*)\))?:`)
	// Decorator: @decorator_name or @decorator_name(args)
	decoratorPattern := regexp.MustCompile(`^@(\w+)(?:\(|$)`)

	var currentClass string
	var classIndent int
	var pendingDecorators []string

	for i, line := range lines {
		lineNum := i + 1
		trimmedLine := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmedLine == "" || strings.HasPrefix(trimmedLine, "#") {
			continue
		}

		// Calculate indentation
		indent := len(line) - len(strings.TrimLeft(line, " \t"))

		// Track decorators
		if matches := decoratorPattern.FindStringSubmatch(trimmedLine); matches != nil {
			pendingDecorators = append(pendingDecorators, matches[1])
			continue
		}

		// Check if we've exited the current class (dedented to class level or lower)
		if currentClass != "" && indent <= classIndent && !strings.HasPrefix(trimmedLine, "@") {
			currentClass = ""
			classIndent = 0
		}

		// Extract type aliases (at module level, CamelCase = Type)
		if indent == 0 {
			if matches := typeAliasPattern.FindStringSubmatch(trimmedLine); matches != nil {
				// Don't capture if it matches class pattern
				if !classPattern.MatchString(trimmedLine) {
					sym := Symbol{
						Name:       matches[1],
						Type:       "type",
						Line: lineNum,
						Exported:   !strings.HasPrefix(matches[1], "_"),
						Language:   "python",
					}
					symbols = append(symbols, sym)
					pendingDecorators = nil
					continue
				}
			}
		}

		// Extract module-level constants (must not be indented)
		if indent == 0 {
			if matches := constPattern.FindStringSubmatch(trimmedLine); matches != nil {
				// Skip if it looks like a class or function definition
				if strings.Contains(matches[2], "class") || strings.Contains(matches[2], "def") {
					continue
				}
				sym := Symbol{
					Name:       matches[1],
					Type:       "constant",
					Value:      CleanValue(matches[2]),
					RawValue:   matches[2],
					Line: lineNum,
					Exported:   true,
					Language:   "python",
				}
				symbols = append(symbols, sym)
				pendingDecorators = nil
				continue
			}
			// Also capture private constants
			if matches := privateConstPattern.FindStringSubmatch(trimmedLine); matches != nil {
				if strings.Contains(matches[2], "class") || strings.Contains(matches[2], "def") {
					continue
				}
				sym := Symbol{
					Name:       matches[1],
					Type:       "constant",
					Value:      CleanValue(matches[2]),
					RawValue:   matches[2],
					Line: lineNum,
					Exported:   false,
					Language:   "python",
				}
				symbols = append(symbols, sym)
				pendingDecorators = nil
				continue
			}
		}

		// Extract class definitions
		if matches := classPattern.FindStringSubmatch(trimmedLine); matches != nil {
			className := matches[1]
			baseClasses := ""
			if len(matches) > 2 && matches[2] != "" {
				baseClasses = matches[2]
			}

			// Determine symbol type based on base class
			symType := "class"
			if strings.Contains(baseClasses, "Enum") {
				symType = "enum"
			} else if strings.Contains(baseClasses, "Protocol") {
				symType = "interface"
			}

			// Build doc comment from decorators
			docComment := ""
			if len(pendingDecorators) > 0 {
				docComment = "Decorators: @" + strings.Join(pendingDecorators, ", @")
			}

			sym := Symbol{
				Name:       className,
				Type:       symType,
				Parent:     baseClasses,
				DocComment: docComment,
				Line: lineNum,
				Exported:   !strings.HasPrefix(className, "_"),
				Language:   "python",
			}
			symbols = append(symbols, sym)

			// Track class context for method detection
			currentClass = className
			classIndent = indent
			pendingDecorators = nil
			continue
		}

		// Extract function/method definitions
		if matches := fnPattern.FindStringSubmatch(trimmedLine); matches != nil {
			isAsync := matches[1] != ""
			funcName := matches[2]
			params := matches[3]
			returnType := ""
			if len(matches) > 4 && matches[4] != "" {
				returnType = strings.TrimSpace(matches[4])
			}

			// Skip dunder methods except __init__
			if strings.HasPrefix(funcName, "__") && funcName != "__init__" {
				pendingDecorators = nil
				continue
			}

			// Determine if this is a method (inside a class) or function
			symType := "function"
			parent := ""
			if currentClass != "" && indent > classIndent {
				symType = "method"
				parent = currentClass
			}

			// Build signature
			prefix := "def "
			if isAsync {
				prefix = "async def "
			}
			sig := fmt.Sprintf("%s%s(%s)%s", prefix, funcName, params, formatPythonReturn(returnType))

			// Build doc comment from decorators
			docComment := ""
			if len(pendingDecorators) > 0 {
				docComment = "Decorators: @" + strings.Join(pendingDecorators, ", @")
			}

			sym := Symbol{
				Name:           funcName,
				Type:           symType,
				Parent:         parent,
				Signature:      sig,
				TypeAnnotation: returnType,
				DocComment:     docComment,
				Line:     lineNum,
				Exported:       !strings.HasPrefix(funcName, "_"),
				Language:       "python",
			}
			symbols = append(symbols, sym)
			pendingDecorators = nil
			continue
		}

		// Clear pending decorators if we hit a line that didn't use them
		if len(pendingDecorators) > 0 && !strings.HasPrefix(trimmedLine, "@") {
			pendingDecorators = nil
		}
	}

	return symbols
}

// filterDunderMethods removes __dunder__ methods from a list
func filterDunderMethods(funcs []string) []string {
	var result []string
	for _, f := range funcs {
		if !strings.HasPrefix(f, "__") {
			result = append(result, f)
		}
	}
	return result
}

// formatPythonReturn formats a return type for display
func formatPythonReturn(returnType string) string {
	if returnType == "" {
		return ""
	}
	return " -> " + returnType
}

// extractClassContent extracts a class definition with docstring for better embedding.
// Returns enhanced content that includes class signature, docstring, and key attributes.
func extractClassContent(content, className, moduleName string) string {
	var result strings.Builder
	result.WriteString(fmt.Sprintf("Python class %s in module %s\n", className, moduleName))

	// Find class definition
	classPattern := regexp.MustCompile(fmt.Sprintf(`(?m)^class\s+%s\s*(?:\(([^)]*)\))?\s*:`, regexp.QuoteMeta(className)))
	match := classPattern.FindStringSubmatchIndex(content)
	if match == nil {
		return result.String()
	}

	// Extract base classes if present
	if match[2] != -1 && match[3] != -1 {
		bases := content[match[2]:match[3]]
		if bases != "" {
			result.WriteString(fmt.Sprintf("Inherits from: %s\n", bases))
		}
	}

	// Find class body start
	classStart := match[0]
	lines := strings.Split(content[classStart:], "\n")

	// Extract docstring if present (first string after class:)
	docLines := []string{}
	inDocstring := false
	docstringDelim := ""
	for i := 1; i < len(lines) && i < 30; i++ {
		line := strings.TrimSpace(lines[i])
		if !inDocstring {
			if strings.HasPrefix(line, `"""`) || strings.HasPrefix(line, `'''`) {
				inDocstring = true
				docstringDelim = line[:3]
				docLine := strings.TrimPrefix(line, docstringDelim)
				if strings.HasSuffix(docLine, docstringDelim) {
					// Single-line docstring
					docLines = append(docLines, strings.TrimSuffix(docLine, docstringDelim))
					break
				}
				docLines = append(docLines, docLine)
			} else if line != "" && !strings.HasPrefix(line, "#") {
				// Not a docstring, move on
				break
			}
		} else {
			if strings.HasSuffix(line, docstringDelim) {
				docLines = append(docLines, strings.TrimSuffix(line, docstringDelim))
				break
			}
			docLines = append(docLines, line)
		}
	}

	if len(docLines) > 0 {
		docstring := strings.Join(docLines, " ")
		if len(docstring) > 500 {
			docstring = docstring[:500] + "..."
		}
		result.WriteString(fmt.Sprintf("Description: %s\n", docstring))
	}

	// Extract class attributes (lines with : type annotation)
	attrPattern := regexp.MustCompile(`^\s{4}(\w+)\s*:\s*(.+?)(?:\s*=.*)?$`)
	attrs := []string{}
	for i := 1; i < len(lines) && i < 50 && len(attrs) < 10; i++ {
		if attrMatch := attrPattern.FindStringSubmatch(lines[i]); attrMatch != nil {
			attrs = append(attrs, fmt.Sprintf("%s: %s", attrMatch[1], attrMatch[2]))
		}
	}
	if len(attrs) > 0 {
		result.WriteString(fmt.Sprintf("Key attributes: %s\n", strings.Join(attrs, ", ")))
	}

	return result.String()
}
