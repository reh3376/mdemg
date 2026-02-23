package languages

import (
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&GraphQLParser{})
}

// GraphQLParser implements LanguageParser for GraphQL schema files
type GraphQLParser struct{}

func (p *GraphQLParser) Name() string {
	return "graphql"
}

func (p *GraphQLParser) Extensions() []string {
	return []string{".graphql", ".gql"}
}

func (p *GraphQLParser) CanParse(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.HasSuffix(pathLower, ".graphql") || strings.HasSuffix(pathLower, ".gql")
}

func (p *GraphQLParser) IsTestFile(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.Contains(pathLower, "/fixtures/") ||
		strings.Contains(pathLower, "/testdata/") ||
		strings.Contains(pathLower, "_test.")
}

func (p *GraphQLParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)

	// Build summary content
	var contentBuilder strings.Builder
	contentBuilder.WriteString("GraphQL file: " + fileName + "\n")

	// Detect schema type
	schemaKind := p.detectSchemaKind(content)
	contentBuilder.WriteString("Type: " + schemaKind + "\n")

	// Extract symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	contentBuilder.WriteString("\n--- Content ---\n")
	contentBuilder.WriteString(TruncateContent(content, 4000))

	// Detect concerns
	concerns := DetectConcerns(relPath, content)
	tags := []string{"graphql", "api", "schema", schemaKind}
	tags = append(tags, concerns...)

	element := CodeElement{
		Name:        fileName,
		Kind:        schemaKind,
		Path:        "/" + relPath,
		Content:     contentBuilder.String(),
		Package:     "api",
		FilePath:    relPath,
		Tags:        tags,
		Concerns:    concerns,
		Symbols:     symbols,
		ElementKind: "file",
	}

	return []CodeElement{element}, nil
}

func (p *GraphQLParser) detectSchemaKind(content string) string {
	if strings.Contains(content, "extend type") || strings.Contains(content, "extend schema") {
		return "graphql-extension"
	}
	if strings.Contains(content, "type Query") || strings.Contains(content, "type Mutation") {
		return "graphql-schema"
	}
	if strings.Contains(content, "query ") || strings.Contains(content, "mutation ") {
		return "graphql-operation"
	}
	return "graphql"
}

// Regex patterns for GraphQL parsing
var (
	// Type definitions
	gqlTypePattern      = regexp.MustCompile(`(?m)^\s*type\s+(\w+)(?:\s+implements\s+([\w\s&]+))?\s*\{`)
	gqlInterfacePattern = regexp.MustCompile(`(?m)^\s*interface\s+(\w+)\s*\{`)
	gqlInputPattern     = regexp.MustCompile(`(?m)^\s*input\s+(\w+)\s*\{`)
	gqlEnumPattern      = regexp.MustCompile(`(?m)^\s*enum\s+(\w+)\s*\{`)
	gqlUnionPattern     = regexp.MustCompile(`(?m)^\s*union\s+(\w+)\s*=\s*(.+)`)
	gqlScalarPattern    = regexp.MustCompile(`(?m)^\s*scalar\s+(\w+)`)
	gqlDirectivePattern = regexp.MustCompile(`(?m)^\s*directive\s+@(\w+)`)
	gqlExtendPattern    = regexp.MustCompile(`(?m)^\s*extend\s+type\s+(\w+)\s*\{`)
	gqlSchemaPattern    = regexp.MustCompile(`(?m)^\s*schema\s*\{`)

	// Fields and enum values
	gqlFieldPattern     = regexp.MustCompile(`(?m)^\s*(\w+)(?:\s*\([^)]*\))?\s*:\s*(\[?\w+!?\]?!?)`)
	gqlEnumValuePattern = regexp.MustCompile(`(?m)^\s*([A-Z][A-Z0-9_]*)(?:\s|$)`)
	gqlArgumentPattern  = regexp.MustCompile(`(\w+)\s*:\s*([\w\[\]!]+)(?:\s*=\s*([^,)]+))?`)

	// Query/Mutation operations
	gqlQueryOpPattern    = regexp.MustCompile(`(?m)^\s*query\s+(\w+)`)
	gqlMutationOpPattern = regexp.MustCompile(`(?m)^\s*mutation\s+(\w+)`)
	gqlFragmentPattern   = regexp.MustCompile(`(?m)^\s*fragment\s+(\w+)\s+on\s+(\w+)`)
)

func (p *GraphQLParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	var scopeStack []string
	var scopeTypes []string // "type", "interface", "input", "enum", "schema"
	braceDepth := 0

	for lineNum, line := range lines {
		lineNo := lineNum + 1
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Track brace depth
		openBraces := strings.Count(line, "{")
		closeBraces := strings.Count(line, "}")

		// Get current parent
		parent := ""
		if len(scopeStack) > 0 {
			parent = scopeStack[len(scopeStack)-1]
		}

		// Check for type definition
		if matches := gqlTypePattern.FindStringSubmatch(trimmed); matches != nil {
			typeName := matches[1]
			implements := strings.TrimSpace(matches[2])

			docComment := ""
			if implements != "" {
				docComment = "implements " + implements
			}

			// Determine if it's a special type
			symType := "class"
			if typeName == "Query" || typeName == "Mutation" || typeName == "Subscription" {
				symType = "interface"
			}

			symbols = append(symbols, Symbol{
				Name:       typeName,
				Type:       symType,
				Line:       lineNo,
				DocComment: docComment,
				Exported:   true,
				Language:   "graphql",
			})
			if openBraces > 0 {
				scopeStack = append(scopeStack, typeName)
				scopeTypes = append(scopeTypes, "type")
				braceDepth++
			}
			continue
		}

		// Check for interface definition
		if matches := gqlInterfacePattern.FindStringSubmatch(trimmed); matches != nil {
			intfName := matches[1]
			symbols = append(symbols, Symbol{
				Name:     intfName,
				Type:     "interface",
				Line:     lineNo,
				Exported: true,
				Language: "graphql",
			})
			if openBraces > 0 {
				scopeStack = append(scopeStack, intfName)
				scopeTypes = append(scopeTypes, "interface")
				braceDepth++
			}
			continue
		}

		// Check for input definition
		if matches := gqlInputPattern.FindStringSubmatch(trimmed); matches != nil {
			inputName := matches[1]
			symbols = append(symbols, Symbol{
				Name:     inputName,
				Type:     "class",
				Line:     lineNo,
				Exported: true,
				Language: "graphql",
			})
			if openBraces > 0 {
				scopeStack = append(scopeStack, inputName)
				scopeTypes = append(scopeTypes, "input")
				braceDepth++
			}
			continue
		}

		// Check for enum definition
		if matches := gqlEnumPattern.FindStringSubmatch(trimmed); matches != nil {
			enumName := matches[1]
			symbols = append(symbols, Symbol{
				Name:     enumName,
				Type:     "enum",
				Line:     lineNo,
				Exported: true,
				Language: "graphql",
			})
			if openBraces > 0 {
				scopeStack = append(scopeStack, enumName)
				scopeTypes = append(scopeTypes, "enum")
				braceDepth++
			}
			continue
		}

		// Check for union definition
		if matches := gqlUnionPattern.FindStringSubmatch(trimmed); matches != nil {
			unionName := matches[1]
			unionTypes := strings.TrimSpace(matches[2])
			symbols = append(symbols, Symbol{
				Name:       unionName,
				Type:       "type",
				Line:       lineNo,
				DocComment: unionTypes,
				Exported:   true,
				Language:   "graphql",
			})
			continue
		}

		// Check for scalar definition
		if matches := gqlScalarPattern.FindStringSubmatch(trimmed); matches != nil {
			scalarName := matches[1]
			symbols = append(symbols, Symbol{
				Name:     scalarName,
				Type:     "type",
				Line:     lineNo,
				Exported: true,
				Language: "graphql",
			})
			continue
		}

		// Check for directive definition
		if matches := gqlDirectivePattern.FindStringSubmatch(trimmed); matches != nil {
			directiveName := "@" + matches[1]
			symbols = append(symbols, Symbol{
				Name:     directiveName,
				Type:     "function",
				Line:     lineNo,
				Exported: true,
				Language: "graphql",
			})
			continue
		}

		// Check for extend type
		if matches := gqlExtendPattern.FindStringSubmatch(trimmed); matches != nil {
			typeName := matches[1]
			symbols = append(symbols, Symbol{
				Name:       typeName,
				Type:       "class",
				Line:       lineNo,
				DocComment: "extension",
				Exported:   true,
				Language:   "graphql",
			})
			if openBraces > 0 {
				scopeStack = append(scopeStack, typeName)
				scopeTypes = append(scopeTypes, "type")
				braceDepth++
			}
			continue
		}

		// Check for schema definition
		if gqlSchemaPattern.MatchString(trimmed) {
			symbols = append(symbols, Symbol{
				Name:     "schema",
				Type:     "section",
				Line:     lineNo,
				Exported: true,
				Language: "graphql",
			})
			if openBraces > 0 {
				scopeStack = append(scopeStack, "schema")
				scopeTypes = append(scopeTypes, "schema")
				braceDepth++
			}
			continue
		}

		// Check for query operation
		if matches := gqlQueryOpPattern.FindStringSubmatch(trimmed); matches != nil {
			queryName := matches[1]
			symbols = append(symbols, Symbol{
				Name:     queryName,
				Type:     "function",
				Line:     lineNo,
				Exported: true,
				Language: "graphql",
			})
			continue
		}

		// Check for mutation operation
		if matches := gqlMutationOpPattern.FindStringSubmatch(trimmed); matches != nil {
			mutName := matches[1]
			symbols = append(symbols, Symbol{
				Name:     mutName,
				Type:     "function",
				Line:     lineNo,
				Exported: true,
				Language: "graphql",
			})
			continue
		}

		// Check for fragment
		if matches := gqlFragmentPattern.FindStringSubmatch(trimmed); matches != nil {
			fragName := matches[1]
			onType := matches[2]
			symbols = append(symbols, Symbol{
				Name:       fragName,
				Type:       "function",
				Line:       lineNo,
				DocComment: "on " + onType,
				Exported:   true,
				Language:   "graphql",
			})
			continue
		}

		// Check for enum value (inside enum scope)
		if len(scopeTypes) > 0 && scopeTypes[len(scopeTypes)-1] == "enum" {
			if matches := gqlEnumValuePattern.FindStringSubmatch(trimmed); matches != nil {
				valueName := matches[1]
				// Skip if this looks like a keyword or already processed
				if valueName != "" && !strings.Contains(valueName, ":") {
					symbols = append(symbols, Symbol{
						Name:     valueName,
						Type:     "enum_value",
						Line:     lineNo,
						Parent:   parent,
						Exported: true,
						Language: "graphql",
					})
				}
				continue
			}
		}

		// Check for field (inside type/interface/input scope)
		if len(scopeTypes) > 0 {
			scopeType := scopeTypes[len(scopeTypes)-1]
			if scopeType == "type" || scopeType == "interface" || scopeType == "input" || scopeType == "schema" {
				if matches := gqlFieldPattern.FindStringSubmatch(trimmed); matches != nil {
					fieldName := matches[1]
					fieldType := matches[2]

					// Skip closing braces that might match
					if fieldName == "}" || fieldName == "" {
						goto handleBraces
					}

					symbols = append(symbols, Symbol{
						Name:           fieldName,
						Type:           "field",
						TypeAnnotation: fieldType,
						Line:           lineNo,
						Parent:         parent,
						Exported:       true,
						Language:       "graphql",
					})
					continue
				}
			}
		}

	handleBraces:
		// Handle closing braces - pop scope
		for i := 0; i < closeBraces && len(scopeStack) > 0; i++ {
			braceDepth--
			if braceDepth < len(scopeStack) {
				scopeStack = scopeStack[:len(scopeStack)-1]
				scopeTypes = scopeTypes[:len(scopeTypes)-1]
			}
		}

		// Handle opening braces without matching pattern
		if openBraces > closeBraces {
			braceDepth += openBraces - closeBraces
		}
	}

	return symbols
}
