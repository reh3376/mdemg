package languages

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&CppParser{})
}

// CppParser implements LanguageParser for C++ source files
type CppParser struct{}

func (p *CppParser) Name() string {
	return "cpp"
}

func (p *CppParser) Extensions() []string {
	return []string{".cpp", ".cxx", ".cc", ".hpp", ".hxx", ".h"}
}

func (p *CppParser) CanParse(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.HasSuffix(pathLower, ".cpp") ||
		strings.HasSuffix(pathLower, ".cxx") ||
		strings.HasSuffix(pathLower, ".cc") ||
		strings.HasSuffix(pathLower, ".hpp") ||
		strings.HasSuffix(pathLower, ".hxx") ||
		(strings.HasSuffix(pathLower, ".h") && p.isCppHeader(path))
}

// isCppHeader tries to detect if a .h file is C++ (has classes/namespaces)
// If unsure, defaults to C (handled by c_parser.go)
func (p *CppParser) isCppHeader(path string) bool {
	content, err := ReadFileContent(path)
	if err != nil {
		return false
	}
	// Check for C++ indicators
	return strings.Contains(content, "class ") ||
		strings.Contains(content, "namespace ") ||
		strings.Contains(content, "template<") ||
		strings.Contains(content, "template <") ||
		strings.Contains(content, "public:") ||
		strings.Contains(content, "private:") ||
		strings.Contains(content, "protected:")
}

func (p *CppParser) IsTestFile(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.Contains(pathLower, "_test.") ||
		strings.Contains(pathLower, "_tests.") ||
		strings.Contains(pathLower, "/test/") ||
		strings.Contains(pathLower, "/tests/") ||
		strings.HasSuffix(pathLower, "_unittest.cpp") ||
		strings.HasSuffix(pathLower, "_test.cpp")
}

func (p *CppParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	var elements []CodeElement

	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)

	// Detect if header or implementation
	isHeader := strings.HasSuffix(path, ".h") || strings.HasSuffix(path, ".hpp") || strings.HasSuffix(path, ".hxx")

	// Extract namespaces
	namespaces := FindAllMatches(content, `namespace\s+(\w+)\s*\{`)

	// Extract classes and structs
	classes := FindAllMatches(content, `class\s+(\w+)(?:\s*:\s*(?:public|private|protected)\s+[\w:]+)?(?:\s*\{)?`)
	structs := FindAllMatches(content, `struct\s+(\w+)(?:\s*\{)?`)

	// Extract templates
	templates := FindAllMatches(content, `template\s*<[^>]+>\s*(?:class|struct)\s+(\w+)`)

	// Extract functions
	functions := FindAllMatches(content, `(?:[\w:]+\s+)?(\w+)\s*\([^)]*\)\s*(?:const)?\s*(?:override)?\s*(?:noexcept)?\s*(?:=\s*(?:default|delete|0))?\s*[;{]`)

	// Extract includes
	includes := FindAllMatches(content, `#include\s*[<"]([^>"]+)[>"]`)

	// Build content for embedding
	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("C++ file: %s\n", fileName))

	if isHeader {
		contentBuilder.WriteString("Type: Header\n")
	} else {
		contentBuilder.WriteString("Type: Implementation\n")
	}

	if len(namespaces) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Namespaces: %s\n", strings.Join(uniqueStrings(namespaces), ", ")))
	}
	if len(classes) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Classes: %s\n", strings.Join(uniqueStrings(classes), ", ")))
	}
	if len(structs) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Structs: %s\n", strings.Join(uniqueStrings(structs), ", ")))
	}
	if len(templates) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Templates: %s\n", strings.Join(uniqueStrings(templates), ", ")))
	}
	if len(functions) > 0 {
		fnList := uniqueStrings(functions)
		// Filter out common false positives
		fnList = filterCppFunctions(fnList)
		if len(fnList) > 15 {
			fnList = fnList[:15]
			contentBuilder.WriteString(fmt.Sprintf("Functions: %s (and more)\n", strings.Join(fnList, ", ")))
		} else if len(fnList) > 0 {
			contentBuilder.WriteString(fmt.Sprintf("Functions: %s\n", strings.Join(fnList, ", ")))
		}
	}
	contentBuilder.WriteString(fmt.Sprintf("Includes: %d\n", len(includes)))

	// Include actual code content
	contentBuilder.WriteString("\n--- Code ---\n")
	contentBuilder.WriteString(TruncateContent(content, 4000))

	// Detect cross-cutting concerns
	concerns := DetectConcerns(relPath, content)
	tags := []string{"cpp", "module"}
	if isHeader {
		tags = append(tags, "header")
	} else {
		tags = append(tags, "implementation")
	}
	tags = append(tags, concerns...)

	// Determine file kind
	cppKind := "cpp-source"
	if isHeader {
		cppKind = "cpp-header"
	}

	// Extract symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	// Add file-level element
	elements = append(elements, CodeElement{
		Name:     fileName,
		Kind:     cppKind,
		Path:     "/" + relPath,
		Content:  contentBuilder.String(),
		Package:  strings.Join(uniqueStrings(namespaces), "::"),
		FilePath: relPath,
		Tags:     tags,
		Concerns: concerns,
		Symbols:  symbols,
	})

	// Add classes as separate elements
	for _, class := range uniqueStrings(classes) {
		elements = append(elements, CodeElement{
			Name:     class,
			Kind:     "class",
			Path:     fmt.Sprintf("/%s#%s", relPath, class),
			Content:  fmt.Sprintf("C++ class '%s' in file %s", class, fileName),
			Package:  strings.Join(uniqueStrings(namespaces), "::"),
			FilePath: relPath,
			Tags:     append([]string{"cpp", "class"}, concerns...),
			Concerns: concerns,
		})
	}

	// Add structs as separate elements
	for _, st := range uniqueStrings(structs) {
		elements = append(elements, CodeElement{
			Name:     st,
			Kind:     "struct",
			Path:     fmt.Sprintf("/%s#%s", relPath, st),
			Content:  fmt.Sprintf("C++ struct '%s' in file %s", st, fileName),
			Package:  strings.Join(uniqueStrings(namespaces), "::"),
			FilePath: relPath,
			Tags:     append([]string{"cpp", "struct"}, concerns...),
			Concerns: concerns,
		})
	}

	return elements, nil
}

func (p *CppParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	// Patterns for C++ constructs
	// Constants: constexpr TYPE NAME = value; or const TYPE NAME = value;
	constPattern := regexp.MustCompile(`^\s*(?:inline\s+)?(?:constexpr|const)\s+(?:const\s+)?([^\s=]+(?:\s*\*)?)\s+([A-Z][A-Z0-9_]*)\s*=\s*(.+?);`)
	staticConstPattern := regexp.MustCompile(`^\s*static\s+(?:const|constexpr)\s+([^\s=]+)\s+([A-Z][A-Z0-9_]*)\s*=\s*(.+?);`)
	// Type alias: using Name = Type;
	usingPattern := regexp.MustCompile(`^\s*using\s+(\w+)\s*=`)
	// Namespace: namespace Name {
	namespacePattern := regexp.MustCompile(`^\s*namespace\s+(\w+)\s*\{`)
	// Enum: enum [class] Name {
	enumPattern := regexp.MustCompile(`^\s*enum\s+(?:class\s+)?(\w+)\s*(?:\{|:)`)
	// Class/struct: [template<...>] class Name [: inheritance] {
	classPattern := regexp.MustCompile(`^\s*(?:template\s*<[^>]*>\s*)?class\s+(\w+)`)
	structPattern := regexp.MustCompile(`^\s*(?:template\s*<[^>]*>\s*)?struct\s+(\w+)`)
	// Method/function: [virtual] [static] [const] RetType [&*] Name(params) [const] [override] [noexcept] [= 0/default/delete] [;{]
	// Handle return types like: void, int, const UserId&, std::optional<User>, const std::string&
	methodPattern := regexp.MustCompile(`^\s*(?:virtual\s+)?(?:static\s+)?(?:explicit\s+)?(?:inline\s+)?((?:const\s+)?[\w:]+(?:<[^>]*>)?(?:\s*[*&])?)\s+(\w+)\s*\(([^)]*)\)\s*(?:const)?\s*(?:override)?\s*(?:noexcept)?`)
	// Pure virtual or deleted: = 0, = default, = delete
	pureVirtualPattern := regexp.MustCompile(`=\s*(?:0|default|delete)\s*;`)

	// Scope tracking
	var scopeStack []string
	var scopeDepths []int
	braceDepth := 0
	inPrivate := false
	_ = inPrivate // will be used for export tracking

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			continue
		}

		// Check for scope changes (public/private/protected)
		if strings.Contains(trimmed, "private:") {
			inPrivate = true
		} else if strings.Contains(trimmed, "protected:") || strings.Contains(trimmed, "public:") {
			inPrivate = false
		}

		// Track brace depth - but process scope exit AFTER pattern matching
		// This ensures inline methods like `int getValue() const { return v_; }`
		// are properly associated with their parent class
		openBraces := strings.Count(line, "{")
		closeBraces := strings.Count(line, "}")

		// Check for namespace
		if matches := namespacePattern.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:     matches[1],
				Type:     "namespace",
				Line:     lineNum,
				Exported: true,
				Language: "cpp",
			})
			continue
		}

		// Check for using type alias
		if matches := usingPattern.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:     matches[1],
				Type:     "type",
				Line:     lineNum,
				Exported: true,
				Language: "cpp",
			})
			continue
		}

		// Check for static const
		if matches := staticConstPattern.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:           matches[2],
				Type:           "constant",
				TypeAnnotation: matches[1],
				Value:          CleanValue(matches[3]),
				RawValue:       matches[3],
				Line:           lineNum,
				Exported:       !inPrivate,
				Language:       "cpp",
			})
			continue
		}

		// Check for const/constexpr
		if matches := constPattern.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:           matches[2],
				Type:           "constant",
				TypeAnnotation: matches[1],
				Value:          CleanValue(matches[3]),
				RawValue:       matches[3],
				Line:           lineNum,
				Exported:       !inPrivate,
				Language:       "cpp",
			})
			continue
		}

		// Check for enum
		if matches := enumPattern.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:     matches[1],
				Type:     "enum",
				Line:     lineNum,
				Exported: true,
				Language: "cpp",
			})
			continue
		}

		// Check for class
		if matches := classPattern.FindStringSubmatch(line); matches != nil {
			className := matches[1]
			symbols = append(symbols, Symbol{
				Name:     className,
				Type:     "class",
				Line:     lineNum,
				Exported: true,
				Language: "cpp",
			})
			if strings.Contains(line, "{") {
				scopeStack = append(scopeStack, className)
				scopeDepths = append(scopeDepths, braceDepth)
			}
			continue
		}

		// Check for struct (treated as class in C++)
		if matches := structPattern.FindStringSubmatch(line); matches != nil {
			structName := matches[1]
			symbols = append(symbols, Symbol{
				Name:     structName,
				Type:     "class",
				Line:     lineNum,
				Exported: true,
				Language: "cpp",
			})
			if strings.Contains(line, "{") {
				scopeStack = append(scopeStack, structName)
				scopeDepths = append(scopeDepths, braceDepth)
			}
			continue
		}

		// Check for method/function
		if matches := methodPattern.FindStringSubmatch(line); matches != nil {
			returnType := strings.TrimSpace(matches[1])
			funcName := matches[2]
			params := matches[3]

			// Skip destructors
			if strings.HasPrefix(funcName, "~") {
				continue
			}
			// Skip keywords
			if funcName == "if" || funcName == "for" || funcName == "while" || funcName == "switch" || funcName == "return" || funcName == "catch" {
				continue
			}
			// Skip member variable initializers (like id_(std::move(...)))
			if strings.Contains(funcName, "_") && strings.HasSuffix(funcName, "_") {
				continue
			}

			// Determine if this is a method (inside a class) or function
			symType := "function"
			parent := ""
			if len(scopeStack) > 0 {
				symType = "method"
				parent = scopeStack[len(scopeStack)-1]
			}

			// Check if this is a pure virtual, default, or deleted function (declaration only)
			isPure := pureVirtualPattern.MatchString(line)
			if !isPure && !strings.Contains(line, "{") && !strings.Contains(line, ";") {
				continue
			}

			signature := fmt.Sprintf("%s %s(%s)", returnType, funcName, params)
			if len(signature) > 150 {
				signature = signature[:150] + "..."
			}

			symbols = append(symbols, Symbol{
				Name:           funcName,
				Type:           symType,
				Parent:         parent,
				Signature:      signature,
				TypeAnnotation: returnType,
				Line:           lineNum,
				Exported:       !inPrivate,
				Language:       "cpp",
			})
		}

		// Update brace depth and check for scope exit AFTER pattern matching
		// This ensures inline methods are associated with their parent before scope changes
		braceDepth += openBraces
		for closeBraces > 0 {
			braceDepth--
			if len(scopeStack) > 0 && len(scopeDepths) > 0 && braceDepth < scopeDepths[len(scopeDepths)-1] {
				scopeStack = scopeStack[:len(scopeStack)-1]
				scopeDepths = scopeDepths[:len(scopeDepths)-1]
				inPrivate = false
			}
			closeBraces--
		}
	}

	return symbols
}

// filterCppFunctions removes common false positives from function names
func filterCppFunctions(funcs []string) []string {
	skip := map[string]bool{
		"if": true, "for": true, "while": true, "switch": true,
		"return": true, "sizeof": true, "typeof": true, "alignof": true,
		"new": true, "delete": true, "throw": true, "catch": true,
	}
	var result []string
	for _, f := range funcs {
		if !skip[f] {
			result = append(result, f)
		}
	}
	return result
}
