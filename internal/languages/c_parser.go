package languages

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&CParser{})
}

// CParser implements LanguageParser for C source files
type CParser struct{}

func (p *CParser) Name() string {
	return "c"
}

func (p *CParser) Extensions() []string {
	return []string{".c", ".h"}
}

func (p *CParser) CanParse(path string) bool {
	pathLower := strings.ToLower(path)
	if strings.HasSuffix(pathLower, ".c") {
		return true
	}
	// For .h files, only claim if it's NOT C++ (no classes/namespaces)
	if strings.HasSuffix(pathLower, ".h") {
		return !p.isCppHeader(path)
	}
	return false
}

// isCppHeader detects if a .h file contains C++ code
func (p *CParser) isCppHeader(path string) bool {
	content, err := ReadFileContent(path)
	if err != nil {
		return false
	}
	return strings.Contains(content, "class ") ||
		strings.Contains(content, "namespace ") ||
		strings.Contains(content, "template<") ||
		strings.Contains(content, "template <") ||
		strings.Contains(content, "public:") ||
		strings.Contains(content, "private:")
}

func (p *CParser) IsTestFile(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.Contains(pathLower, "_test.") ||
		strings.Contains(pathLower, "/test/") ||
		strings.Contains(pathLower, "/tests/")
}

func (p *CParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	var elements []CodeElement

	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)

	isHeader := strings.HasSuffix(path, ".h")

	// Extract structs and typedefs
	structs := FindAllMatches(content, `struct\s+(\w+)\s*\{`)
	typedefs := FindAllMatches(content, `typedef\s+(?:struct\s+)?[\w\s*]+\s+(\w+)\s*;`)
	enums := FindAllMatches(content, `enum\s+(\w+)\s*\{`)

	// Extract functions
	functions := FindAllMatches(content, `(?:static\s+)?(?:inline\s+)?[\w*]+\s+(\w+)\s*\([^)]*\)\s*[;{]`)

	// Extract includes
	includes := FindAllMatches(content, `#include\s*[<"]([^>"]+)[>"]`)

	// Build content for embedding
	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("C file: %s\n", fileName))

	if isHeader {
		contentBuilder.WriteString("Type: Header\n")
	} else {
		contentBuilder.WriteString("Type: Source\n")
	}

	if len(structs) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Structs: %s\n", strings.Join(uniqueStrings(structs), ", ")))
	}
	if len(typedefs) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Typedefs: %s\n", strings.Join(uniqueStrings(typedefs), ", ")))
	}
	if len(enums) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Enums: %s\n", strings.Join(uniqueStrings(enums), ", ")))
	}
	if len(functions) > 0 {
		fnList := uniqueStrings(functions)
		fnList = filterCFunctions(fnList)
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
	tags := []string{"c", "module"}
	if isHeader {
		tags = append(tags, "header")
	} else {
		tags = append(tags, "source")
	}
	tags = append(tags, concerns...)

	// Determine file kind
	cKind := "c-source"
	if isHeader {
		cKind = "c-header"
	}

	// Extract symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	// Add file-level element
	elements = append(elements, CodeElement{
		Name:     fileName,
		Kind:     cKind,
		Path:     "/" + relPath,
		Content:  contentBuilder.String(),
		Package:  "",
		FilePath: relPath,
		Tags:     tags,
		Concerns: concerns,
		Symbols:  symbols,
	})

	// Add structs as separate elements
	for _, st := range uniqueStrings(structs) {
		elements = append(elements, CodeElement{
			Name:     st,
			Kind:     "struct",
			Path:     fmt.Sprintf("/%s#%s", relPath, st),
			Content:  fmt.Sprintf("C struct '%s' in file %s", st, fileName),
			Package:  "",
			FilePath: relPath,
			Tags:     append([]string{"c", "struct"}, concerns...),
			Concerns: concerns,
		})
	}

	return elements, nil
}

func (p *CParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	// Patterns for C constructs
	// Simple macro: #define NAME value or #define NAME (no value)
	defineSimplePattern := regexp.MustCompile(`^\s*#define\s+([A-Z_][A-Z0-9_]*)\s*$`)
	defineValuePattern := regexp.MustCompile(`^\s*#define\s+([A-Z_][A-Z0-9_]*)\s+([^(].*)$`)
	// Function-like macro: #define NAME(args) ...
	defineFuncPattern := regexp.MustCompile(`^\s*#define\s+([A-Z_][A-Z0-9_]*)\s*\(`)
	// Typedef: typedef TYPE NAME; (captures the last word before semicolon)
	typedefSimplePattern := regexp.MustCompile(`^\s*typedef\s+[\w\s*]+\s+(\w+)\s*;`)
	// Typedef struct pointer: typedef struct X* Y; (needs special handling)
	typedefStructPtrPattern := regexp.MustCompile(`^\s*typedef\s+struct\s+\w+\s*\*\s*(\w+)\s*;`)
	// Typedef struct alias: typedef struct X X; (needs special handling)
	typedefStructAliasPattern := regexp.MustCompile(`^\s*typedef\s+struct\s+(\w+)\s+(\w+)\s*;`)
	// Typedef struct: typedef struct [Name] { ... } Name;
	typedefStructPattern := regexp.MustCompile(`^\s*typedef\s+struct\s+(\w+)?`)
	// Typedef enum: typedef enum [Name] { ... } Name;
	typedefEnumPattern := regexp.MustCompile(`^\s*typedef\s+enum\s*(\w*)`)
	// Standalone struct: struct Name { or struct Name;
	structPattern := regexp.MustCompile(`^\s*struct\s+(\w+)\s*[{;]`)
	// Standalone enum: enum Name {
	enumPattern := regexp.MustCompile(`^\s*enum\s+(\w+)\s*\{`)
	// Enum values - more lenient pattern (doesn't require trailing comma/brace)
	enumValuePattern := regexp.MustCompile(`^\s*([A-Z][A-Z0-9_]*)\s*(?:=\s*[^,}\n]+)?`)
	// Function declaration/definition
	funcPattern := regexp.MustCompile(`^\s*(?:static\s+)?(?:inline\s+)?(?:extern\s+)?([\w*\s]+?)\s+\*?(\w+)\s*\(([^)]*)\)\s*[;{]`)
	// Const variable
	constPattern := regexp.MustCompile(`^\s*(?:static\s+)?const\s+([\w*]+)\s+([A-Z][A-Z0-9_]*)\s*=\s*(.+?);`)

	inEnum := false
	inStruct := false
	currentTypeName := ""
	braceDepth := 0

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Skip empty and comment lines
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			continue
		}

		// Track brace depth
		braceDepth += strings.Count(line, "{") - strings.Count(line, "}")

		// Handle being inside an enum
		if inEnum {
			if strings.Contains(line, "}") {
				// First check if this line also has a final enum value before the brace
				// e.g., "    LAST_VALUE = 3\n}" or "    LAST_VALUE\n}"
				// We already processed previous lines, but need to check for closing typedef name
				if matches := regexp.MustCompile(`\}\s*(\w+)\s*;`).FindStringSubmatch(line); matches != nil {
					if currentTypeName == "" {
						symbols = append(symbols, Symbol{
							Name:     matches[1],
							Type:     "type",
							Line:     lineNum,
							Exported: true,
							Language: "c",
						})
					}
				}
				inEnum = false
				currentTypeName = ""
			} else if matches := enumValuePattern.FindStringSubmatch(trimmed); matches != nil {
				// Only extract if it looks like an enum value (all caps with underscores)
				name := matches[1]
				if len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z' {
					symbols = append(symbols, Symbol{
						Name:     name,
						Type:     "enum_value",
						Line:     lineNum,
						Exported: true,
						Language: "c",
					})
				}
			}
			continue
		}

		// Handle being inside a struct
		if inStruct {
			if strings.Contains(line, "}") {
				// Check if typedef name follows: } Name;
				if matches := regexp.MustCompile(`\}\s*(\w+)\s*;`).FindStringSubmatch(line); matches != nil {
					symbols = append(symbols, Symbol{
						Name:     matches[1],
						Type:     "type",
						Line:     lineNum,
						Exported: true,
						Language: "c",
					})
				}
				inStruct = false
				currentTypeName = ""
			}
			continue
		}

		// Check for typedef enum
		if matches := typedefEnumPattern.FindStringSubmatch(line); matches != nil {
			if matches[1] != "" {
				symbols = append(symbols, Symbol{
					Name:     matches[1],
					Type:     "enum",
					Line:     lineNum,
					Exported: true,
					Language: "c",
				})
				currentTypeName = matches[1]
			}
			if strings.Contains(line, "{") {
				inEnum = true
			}
			continue
		}

		// Check for typedef struct pointer alias: typedef struct X* Y;
		// Must check BEFORE general typedef struct pattern
		if matches := typedefStructPtrPattern.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:     matches[1],
				Type:     "type",
				Line:     lineNum,
				Exported: true,
				Language: "c",
			})
			continue
		}

		// Check for typedef struct alias: typedef struct X Y; (creates alias Y for struct X)
		// Must check BEFORE general typedef struct pattern
		if matches := typedefStructAliasPattern.FindStringSubmatch(line); matches != nil {
			structName := matches[1]
			aliasName := matches[2]
			// Emit the struct
			symbols = append(symbols, Symbol{
				Name:     structName,
				Type:     "struct",
				Line:     lineNum,
				Exported: true,
				Language: "c",
			})
			// Emit the type alias
			symbols = append(symbols, Symbol{
				Name:     aliasName,
				Type:     "type",
				Line:     lineNum,
				Exported: true,
				Language: "c",
			})
			continue
		}

		// Check for typedef struct with body: typedef struct [Name] { ... } Name;
		if matches := typedefStructPattern.FindStringSubmatch(line); matches != nil {
			structName := matches[1]
			if structName != "" {
				symbols = append(symbols, Symbol{
					Name:     structName,
					Type:     "struct",
					Line:     lineNum,
					Exported: true,
					Language: "c",
				})
				currentTypeName = structName
			}
			if strings.Contains(line, "{") {
				inStruct = true
			}
			continue
		}

		// Check for simple typedef (non-struct, non-enum)
		if !strings.Contains(line, "struct") && !strings.Contains(line, "enum") {
			if matches := typedefSimplePattern.FindStringSubmatch(line); matches != nil {
				symbols = append(symbols, Symbol{
					Name:     matches[1],
					Type:     "type",
					Line:     lineNum,
					Exported: true,
					Language: "c",
				})
				continue
			}
		}

		// Check for standalone struct
		if matches := structPattern.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:     matches[1],
				Type:     "struct",
				Line:     lineNum,
				Exported: true,
				Language: "c",
			})
			if strings.Contains(line, "{") && !strings.Contains(line, "}") {
				inStruct = true
				currentTypeName = matches[1]
			}
			continue
		}

		// Check for standalone enum
		if matches := enumPattern.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:     matches[1],
				Type:     "enum",
				Line:     lineNum,
				Exported: true,
				Language: "c",
			})
			inEnum = true
			currentTypeName = matches[1]
			continue
		}

		// Check for function-like macro
		if matches := defineFuncPattern.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:     matches[1],
				Type:     "macro",
				Line:     lineNum,
				Exported: true,
				Language: "c",
			})
			continue
		}

		// Check for simple #define with value
		if matches := defineValuePattern.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:     matches[1],
				Type:     "macro",
				Value:    CleanValue(matches[2]),
				RawValue: matches[2],
				Line:     lineNum,
				Exported: true,
				Language: "c",
			})
			continue
		}

		// Check for simple #define without value (header guard)
		if matches := defineSimplePattern.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:     matches[1],
				Type:     "macro",
				Line:     lineNum,
				Exported: true,
				Language: "c",
			})
			continue
		}

		// Check for const
		if matches := constPattern.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:     matches[2],
				Type:     "constant",
				Value:    CleanValue(matches[3]),
				RawValue: matches[3],
				Line:     lineNum,
				Exported: !strings.Contains(line, "static"),
				Language: "c",
			})
			continue
		}

		// Check for function declaration/definition
		if matches := funcPattern.FindStringSubmatch(line); matches != nil {
			funcName := matches[2]
			// Skip if function name is a keyword
			if funcName == "if" || funcName == "for" || funcName == "while" || funcName == "switch" || funcName == "return" {
				continue
			}
			returnType := strings.TrimSpace(matches[1])
			params := matches[3]
			signature := fmt.Sprintf("%s %s(%s)", returnType, funcName, params)

			symbols = append(symbols, Symbol{
				Name:           funcName,
				Type:           "function",
				Signature:      signature,
				TypeAnnotation: returnType,
				Line:           lineNum,
				Exported:       !strings.Contains(line, "static"),
				Language:       "c",
			})
		}
	}

	return symbols
}

// filterCFunctions removes common false positives from function names
func filterCFunctions(funcs []string) []string {
	skip := map[string]bool{
		"if": true, "for": true, "while": true, "switch": true,
		"return": true, "sizeof": true, "typeof": true,
	}
	var result []string
	for _, f := range funcs {
		if !skip[f] {
			result = append(result, f)
		}
	}
	return result
}
