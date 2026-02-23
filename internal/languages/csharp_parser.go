package languages

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&CSharpParser{})
}

// CSharpParser implements LanguageParser for C# source files
type CSharpParser struct{}

func (p *CSharpParser) Name() string {
	return "csharp"
}

func (p *CSharpParser) Extensions() []string {
	return []string{".cs"}
}

func (p *CSharpParser) CanParse(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".cs")
}

func (p *CSharpParser) IsTestFile(path string) bool {
	name := filepath.Base(path)
	return strings.HasSuffix(name, "Test.cs") ||
		strings.HasSuffix(name, "Tests.cs") ||
		strings.HasSuffix(name, ".test.cs") ||
		strings.Contains(path, "/test/") ||
		strings.Contains(path, "/tests/") ||
		strings.Contains(path, "/Test/") ||
		strings.Contains(path, "/Tests/")
}

func (p *CSharpParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	var elements []CodeElement
	var diagnostics []Diagnostic

	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)

	// Extract namespace
	namespaceName := ""
	nsMatch := regexp.MustCompile(`(?m)^\s*namespace\s+([\w.]+)`).FindStringSubmatch(content)
	if nsMatch != nil {
		namespaceName = nsMatch[1]
	}

	// Find top-level constructs for summary
	classes := FindAllMatches(content, `(?:public|internal|private|protected)?\s*(?:abstract\s+)?(?:static\s+)?(?:sealed\s+)?(?:partial\s+)?class\s+(\w+)`)
	interfaces := FindAllMatches(content, `(?:public|internal|private|protected)?\s*interface\s+(\w+)`)
	enums := FindAllMatches(content, `(?:public|internal|private|protected)?\s*enum\s+(\w+)`)
	structs := FindAllMatches(content, `(?:public|internal|private|protected)?\s*(?:readonly\s+)?struct\s+(\w+)`)
	records := FindAllMatches(content, `(?:public|internal|private|protected)?\s*record\s+(\w+)`)
	methods := FindAllMatches(content, `(?:public|private|protected|internal)\s+(?:static\s+)?(?:virtual\s+)?(?:override\s+)?(?:abstract\s+)?(?:async\s+)?(?:\w+(?:<[^>]+>)?(?:\?)?)\s+(\w+)\s*\(`)

	// Build content for embedding
	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("C# file: %s\n", fileName))

	if namespaceName != "" {
		contentBuilder.WriteString(fmt.Sprintf("Namespace: %s\n", namespaceName))
	}
	if len(classes) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Classes: %s\n", strings.Join(uniqueStrings(classes), ", ")))
	}
	if len(interfaces) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Interfaces: %s\n", strings.Join(uniqueStrings(interfaces), ", ")))
	}
	if len(structs) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Structs: %s\n", strings.Join(uniqueStrings(structs), ", ")))
	}
	if len(enums) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Enums: %s\n", strings.Join(uniqueStrings(enums), ", ")))
	}
	if len(records) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Records: %s\n", strings.Join(uniqueStrings(records), ", ")))
	}
	if len(methods) > 0 {
		methodList := uniqueStrings(methods)
		if len(methodList) > 15 {
			methodList = methodList[:15]
			contentBuilder.WriteString(fmt.Sprintf("Methods: %s (and more)\n", strings.Join(methodList, ", ")))
		} else {
			contentBuilder.WriteString(fmt.Sprintf("Methods: %s\n", strings.Join(methodList, ", ")))
		}
	}

	// Include actual code content
	contentBuilder.WriteString("\n--- Code ---\n")
	truncatedContent, wasTruncated := TruncateContentWithInfo(content, 4000)
	contentBuilder.WriteString(truncatedContent)

	if wasTruncated {
		diagnostics = append(diagnostics, NewDiagnostic("info", "TRUNCATED", "Content was truncated to 4000 characters", "csharp"))
	}

	// Detect cross-cutting concerns
	concerns := DetectConcerns(relPath, content)
	tags := []string{"csharp", "module"}
	tags = append(tags, concerns...)

	// Determine file kind based on content
	csKind := "csharp-class"
	if len(interfaces) > 0 && len(classes) == 0 && len(structs) == 0 {
		csKind = "csharp-interface"
		tags = append(tags, "interface")
	} else if len(enums) > 0 && len(classes) == 0 && len(structs) == 0 {
		csKind = "csharp-enum"
		tags = append(tags, "enum")
	} else if len(structs) > 0 && len(classes) == 0 {
		csKind = "csharp-struct"
		tags = append(tags, "struct")
	}

	// Extract code symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	// Add file-level element
	elements = append(elements, CodeElement{
		Name:        fileName,
		Kind:        csKind,
		Path:        "/" + relPath,
		Content:     contentBuilder.String(),
		Package:     namespaceName,
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
			Content:  fmt.Sprintf("C# class '%s' in namespace %s (file: %s)", class, namespaceName, fileName),
			Package:  namespaceName,
			FilePath: relPath,
			Tags:     append([]string{"csharp", "class"}, concerns...),
			Concerns: concerns,
		})
	}

	// Add interfaces as separate elements
	for _, iface := range uniqueStrings(interfaces) {
		elements = append(elements, CodeElement{
			Name:     iface,
			Kind:     "interface",
			Path:     fmt.Sprintf("/%s#%s", relPath, iface),
			Content:  fmt.Sprintf("C# interface '%s' in namespace %s (file: %s)", iface, namespaceName, fileName),
			Package:  namespaceName,
			FilePath: relPath,
			Tags:     append([]string{"csharp", "interface"}, concerns...),
			Concerns: concerns,
		})
	}

	// Add structs as separate elements
	for _, st := range uniqueStrings(structs) {
		elements = append(elements, CodeElement{
			Name:     st,
			Kind:     "struct",
			Path:     fmt.Sprintf("/%s#%s", relPath, st),
			Content:  fmt.Sprintf("C# struct '%s' in namespace %s (file: %s)", st, namespaceName, fileName),
			Package:  namespaceName,
			FilePath: relPath,
			Tags:     append([]string{"csharp", "struct"}, concerns...),
			Concerns: concerns,
		})
	}

	// Add enums as separate elements
	for _, en := range uniqueStrings(enums) {
		elements = append(elements, CodeElement{
			Name:     en,
			Kind:     "enum",
			Path:     fmt.Sprintf("/%s#%s", relPath, en),
			Content:  fmt.Sprintf("C# enum '%s' in namespace %s (file: %s)", en, namespaceName, fileName),
			Package:  namespaceName,
			FilePath: relPath,
			Tags:     append([]string{"csharp", "enum"}, concerns...),
			Concerns: concerns,
		})
	}

	return elements, nil
}

func (p *CSharpParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	// Patterns
	namespacePattern := regexp.MustCompile(`^\s*namespace\s+([\w.]+)`)
	classPattern := regexp.MustCompile(`^\s*(?:\[[\w<>,\s]+\]\s*)*(?:public\s+|internal\s+|private\s+|protected\s+)?(?:abstract\s+)?(?:static\s+)?(?:sealed\s+)?(?:partial\s+)?class\s+(\w+)`)
	structPattern := regexp.MustCompile(`^\s*(?:\[[\w<>,\s]+\]\s*)*(?:public\s+|internal\s+|private\s+|protected\s+)?(?:readonly\s+)?struct\s+(\w+)`)
	interfacePattern := regexp.MustCompile(`^\s*(?:\[[\w<>,\s]+\]\s*)*(?:public\s+|internal\s+|private\s+|protected\s+)?interface\s+(\w+)`)
	enumDeclPattern := regexp.MustCompile(`^\s*(?:\[[\w<>,\s]+\]\s*)*(?:public\s+|internal\s+|private\s+|protected\s+)?enum\s+(\w+)`)
	recordPattern := regexp.MustCompile(`^\s*(?:\[[\w<>,\s]+\]\s*)*(?:public\s+|internal\s+|private\s+|protected\s+)?record\s+(\w+)`)
	delegatePattern := regexp.MustCompile(`^\s*(?:public\s+|internal\s+|private\s+|protected\s+)?delegate\s+\S+\s+(\w+)`)

	// Constants: const Type NAME = value;
	constPattern := regexp.MustCompile(`^\s*(?:public\s+|internal\s+|private\s+|protected\s+)?(?:static\s+)?const\s+(\S+)\s+(\w+)\s*=\s*(.+?);`)
	// Static readonly: static readonly Type NAME = value;
	staticReadonlyPattern := regexp.MustCompile(`^\s*(?:public\s+|internal\s+|private\s+|protected\s+)?(?:new\s+)?static\s+readonly\s+(\S+)\s+(\w+)\s*=\s*(.+?);`)
	// Static readonly without initializer: static readonly Type NAME;
	staticReadonlyNoInitPattern := regexp.MustCompile(`^\s*(?:public\s+|internal\s+|private\s+|protected\s+)?(?:new\s+)?static\s+readonly\s+(\S+)\s+(\w+)\s*;`)

	// Properties: [mods] Type Name { get; set; } or { get; init; }
	propertyPattern := regexp.MustCompile(`^\s*(?:public\s+|internal\s+|private\s+|protected\s+)(?:static\s+)?(?:virtual\s+)?(?:override\s+)?(?:abstract\s+)?(?:new\s+)?(\w+(?:<[^>]+>)?(?:\?)?)\s+(\w+)\s*\{\s*(?:get|set|init)`)

	// Methods: [mods] ReturnType MethodName(params) or [mods] async Task<T> MethodName(params)
	methodPattern := regexp.MustCompile(`^\s*(?:\[[\w<>,\s]+\]\s*)*(?:public\s+|internal\s+|private\s+|protected\s+)(?:static\s+)?(?:virtual\s+)?(?:override\s+)?(?:abstract\s+)?(?:async\s+)?(\w+(?:<[^>]+>)?(?:\?)?)\s+(\w+)\s*\(([^)]*)\)`)

	// Interface method: ReturnType MethodName(params);  (inside interface, no access modifier)
	interfaceMethodPattern := regexp.MustCompile(`^\s*(\w+(?:<[^>]+>)?(?:\?)?)\s+(\w+)\s*\(([^)]*)\)\s*;`)

	// Constructor: [mods] ClassName(params)
	constructorPattern := regexp.MustCompile(`^\s*(?:\[[\w<>,\s]+\]\s*)*(?:public\s+|internal\s+|private\s+|protected\s+)(\w+)\s*\(([^)]*)\)\s*(?::\s*(?:base|this)\s*\([^)]*\))?\s*\{?`)

	// Enum values: PascalCase or UPPER_CASE, possibly with = value, before comma or end
	enumValuePattern := regexp.MustCompile(`^\s*([A-Z]\w*)\s*(?:=\s*[^,}]+)?\s*[,}]?\s*$`)
	// Also catch last enum value without trailing comma
	enumValuePatternNoComma := regexp.MustCompile(`^\s*([A-Z]\w*)\s*$`)

	// Attribute pattern for doc comment collection
	attributePattern := regexp.MustCompile(`^\s*\[(\w[\w<>,\s]*)\]\s*$`)

	// Scope tracking for parent assignment
	type scopeEntry struct {
		name  string
		kind  string // "class", "interface", "enum", "struct", "namespace"
		depth int
	}
	var scopeStack []scopeEntry
	braceDepth := 0
	inEnumValues := false
	pendingAttribute := ""

	// C# access modifier keywords (used to identify method/property lines vs random lines)
	csAccessMods := []string{"public", "internal", "private", "protected"}

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and pure comments
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "/*") {
			// Don't clear pendingAttribute for comments/blanks immediately before a declaration
			goto countBraces
		}

		// Check for attribute (store for next symbol)
		if matches := attributePattern.FindStringSubmatch(trimmed); matches != nil {
			pendingAttribute = "[" + matches[1] + "]"
			goto countBraces
		}

		// Get current parent from scope stack (skip namespace entries)
		{
			parent := ""
			currentScopeKind := ""
			for j := len(scopeStack) - 1; j >= 0; j-- {
				if scopeStack[j].kind != "namespace" {
					parent = scopeStack[j].name
					currentScopeKind = scopeStack[j].kind
					break
				}
			}

			// Consume pending attribute
			docComment := pendingAttribute
			pendingAttribute = ""

			// Check for namespace declaration
			if matches := namespacePattern.FindStringSubmatch(trimmed); matches != nil {
				nsName := matches[1]
				exported := true
				symbols = append(symbols, Symbol{
					Name:       nsName,
					Type:       "namespace",
					Line:       lineNum,
					Exported:   exported,
					DocComment: docComment,
					Language:   "csharp",
				})
				// Track namespace brace depth for brace counting, but don't use as parent.
				// Namespace is metadata (Package field), not a parent scope for symbols.
				// For block-scoped namespaces (with {}), we need a scope entry so the
				// brace counter doesn't get confused, but we use a special kind so it's
				// never used as a parent.
				if !strings.HasSuffix(strings.TrimSpace(trimmed), ";") {
					scopeStack = append(scopeStack, scopeEntry{name: nsName, kind: "namespace", depth: braceDepth})
				}
				goto countBraces
			}

			// Check for record declaration
			if matches := recordPattern.FindStringSubmatch(trimmed); matches != nil {
				recName := matches[1]
				exported := p.hasAccessMod(trimmed, "public") || p.hasAccessMod(trimmed, "internal")
				symbols = append(symbols, Symbol{
					Name:       recName,
					Type:       "class",
					Line:       lineNum,
					Parent:     parent,
					Exported:   exported,
					DocComment: docComment,
					Language:   "csharp",
				})
				// Records may have {} or just ;
				if strings.Contains(trimmed, "{") {
					scopeStack = append(scopeStack, scopeEntry{name: recName, kind: "class", depth: braceDepth})
				}
				goto countBraces
			}

			// Check for class declaration
			if matches := classPattern.FindStringSubmatch(trimmed); matches != nil {
				className := matches[1]
				// Make sure this isn't a "new ClassName(" or similar false positive
				if strings.Contains(trimmed, "class "+className) {
					exported := p.hasAccessMod(trimmed, "public") || p.hasAccessMod(trimmed, "internal")
					symbols = append(symbols, Symbol{
						Name:       className,
						Type:       "class",
						Line:       lineNum,
						Parent:     parent,
						Exported:   exported,
						DocComment: docComment,
						Language:   "csharp",
					})
					inEnumValues = false
					scopeStack = append(scopeStack, scopeEntry{name: className, kind: "class", depth: braceDepth})
					goto countBraces
				}
			}

			// Check for struct declaration
			if matches := structPattern.FindStringSubmatch(trimmed); matches != nil {
				structName := matches[1]
				exported := p.hasAccessMod(trimmed, "public") || p.hasAccessMod(trimmed, "internal")
				symbols = append(symbols, Symbol{
					Name:       structName,
					Type:       "struct",
					Line:       lineNum,
					Parent:     parent,
					Exported:   exported,
					DocComment: docComment,
					Language:   "csharp",
				})
				inEnumValues = false
				scopeStack = append(scopeStack, scopeEntry{name: structName, kind: "struct", depth: braceDepth})
				goto countBraces
			}

			// Check for interface declaration
			if matches := interfacePattern.FindStringSubmatch(trimmed); matches != nil {
				ifaceName := matches[1]
				exported := p.hasAccessMod(trimmed, "public") || p.hasAccessMod(trimmed, "internal")
				symbols = append(symbols, Symbol{
					Name:       ifaceName,
					Type:       "interface",
					Line:       lineNum,
					Parent:     parent,
					Exported:   exported,
					DocComment: docComment,
					Language:   "csharp",
				})
				scopeStack = append(scopeStack, scopeEntry{name: ifaceName, kind: "interface", depth: braceDepth})
				goto countBraces
			}

			// Check for enum declaration
			if matches := enumDeclPattern.FindStringSubmatch(trimmed); matches != nil {
				enumName := matches[1]
				exported := p.hasAccessMod(trimmed, "public") || p.hasAccessMod(trimmed, "internal")
				symbols = append(symbols, Symbol{
					Name:       enumName,
					Type:       "enum",
					Line:       lineNum,
					Parent:     parent,
					Exported:   exported,
					DocComment: docComment,
					Language:   "csharp",
				})
				scopeStack = append(scopeStack, scopeEntry{name: enumName, kind: "enum", depth: braceDepth})
				inEnumValues = true
				goto countBraces
			}

			// Check for delegate declaration
			if matches := delegatePattern.FindStringSubmatch(trimmed); matches != nil {
				delName := matches[1]
				exported := p.hasAccessMod(trimmed, "public") || p.hasAccessMod(trimmed, "internal")
				symbols = append(symbols, Symbol{
					Name:       delName,
					Type:       "type",
					Line:       lineNum,
					Parent:     parent,
					Exported:   exported,
					DocComment: docComment,
					Language:   "csharp",
				})
				goto countBraces
			}

			// Extract enum values (inside enum body)
			if inEnumValues {
				if matches := enumValuePattern.FindStringSubmatch(trimmed); matches != nil {
					symbols = append(symbols, Symbol{
						Name:     matches[1],
						Type:     "enum_value",
						Line:     lineNum,
						Parent:   parent,
						Exported: true,
						Language: "csharp",
					})
					goto countBraces
				}
				if matches := enumValuePatternNoComma.FindStringSubmatch(trimmed); matches != nil {
					symbols = append(symbols, Symbol{
						Name:     matches[1],
						Type:     "enum_value",
						Line:     lineNum,
						Parent:   parent,
						Exported: true,
						Language: "csharp",
					})
					goto countBraces
				}
				// Non-enum-value line inside enum -> values section is over
				if trimmed != "{" && trimmed != "}" {
					inEnumValues = false
				}
			}

			// Check for constants (const keyword)
			if matches := constPattern.FindStringSubmatch(trimmed); matches != nil {
				valueStr := CleanValue(matches[3])
				if len(valueStr) > 100 {
					valueStr = valueStr[:100] + "..."
				}
				exported := p.hasAccessMod(trimmed, "public") || p.hasAccessMod(trimmed, "internal")
				symbols = append(symbols, Symbol{
					Name:           matches[2],
					Type:           "constant",
					TypeAnnotation: matches[1],
					Value:          valueStr,
					Line:           lineNum,
					Parent:         parent,
					Exported:       exported,
					DocComment:     docComment,
					Language:       "csharp",
				})
				goto countBraces
			}

			// Check for static readonly with initializer
			if matches := staticReadonlyPattern.FindStringSubmatch(trimmed); matches != nil {
				valueStr := CleanValue(matches[3])
				if len(valueStr) > 100 {
					valueStr = valueStr[:100] + "..."
				}
				exported := p.hasAccessMod(trimmed, "public") || p.hasAccessMod(trimmed, "internal")
				symbols = append(symbols, Symbol{
					Name:           matches[2],
					Type:           "constant",
					TypeAnnotation: matches[1],
					Value:          valueStr,
					Line:           lineNum,
					Parent:         parent,
					Exported:       exported,
					DocComment:     docComment,
					Language:       "csharp",
				})
				goto countBraces
			}

			// Check for static readonly without initializer
			if matches := staticReadonlyNoInitPattern.FindStringSubmatch(trimmed); matches != nil {
				exported := p.hasAccessMod(trimmed, "public") || p.hasAccessMod(trimmed, "internal")
				symbols = append(symbols, Symbol{
					Name:           matches[2],
					Type:           "constant",
					TypeAnnotation: matches[1],
					Line:           lineNum,
					Parent:         parent,
					Exported:       exported,
					DocComment:     docComment,
					Language:       "csharp",
				})
				goto countBraces
			}

			// Check for properties (must be before method check since properties have { get; set; })
			if matches := propertyPattern.FindStringSubmatch(trimmed); matches != nil {
				propType := matches[1]
				propName := matches[2]
				exported := p.hasAccessMod(trimmed, "public") || p.hasAccessMod(trimmed, "internal")
				symbols = append(symbols, Symbol{
					Name:           propName,
					Type:           "method",
					TypeAnnotation: propType,
					Line:           lineNum,
					Parent:         parent,
					Exported:       exported,
					DocComment:     docComment,
					Language:       "csharp",
				})
				goto countBraces
			}

			// Check for constructor (ClassName(params) where ClassName matches current scope)
			if len(scopeStack) > 0 && (currentScopeKind == "class" || currentScopeKind == "struct") {
				if matches := constructorPattern.FindStringSubmatch(trimmed); matches != nil {
					ctorName := matches[1]
					// Verify this is actually a constructor (name matches scope name)
					isConstructor := false
					for j := len(scopeStack) - 1; j >= 0; j-- {
						if scopeStack[j].name == ctorName {
							isConstructor = true
							break
						}
					}
					if isConstructor {
						exported := p.hasAccessMod(trimmed, "public") || p.hasAccessMod(trimmed, "internal")
						symbols = append(symbols, Symbol{
							Name:       ctorName,
							Type:       "method",
							Line:       lineNum,
							Parent:     parent,
							Exported:   exported,
							DocComment: docComment,
							Language:   "csharp",
						})
						goto countBraces
					}
				}
			}

			// Check for methods (with access modifier)
			if matches := methodPattern.FindStringSubmatch(trimmed); matches != nil {
				returnType := matches[1]
				methodName := matches[2]
				params := matches[3]
				// Exclude false positives: keywords that look like method names
				if !p.isKeyword(methodName) {
					signature := fmt.Sprintf("%s %s(%s)", returnType, methodName, params)
					if len(signature) > 150 {
						signature = signature[:150] + "..."
					}
					exported := p.hasAccessMod(trimmed, "public") || p.hasAccessMod(trimmed, "internal")
					symbols = append(symbols, Symbol{
						Name:           methodName,
						Type:           "method",
						Signature:      signature,
						TypeAnnotation: returnType,
						Line:           lineNum,
						Parent:         parent,
						Exported:       exported,
						DocComment:     docComment,
						Language:       "csharp",
					})
					goto countBraces
				}
			}

			// Check for interface methods (no access modifier): ReturnType MethodName(params);
			if currentScopeKind == "interface" {
				if matches := interfaceMethodPattern.FindStringSubmatch(trimmed); matches != nil {
					returnType := matches[1]
					methodName := matches[2]
					// Skip keywords and common false positives
					if !p.isKeyword(methodName) {
						symbols = append(symbols, Symbol{
							Name:           methodName,
							Type:           "method",
							TypeAnnotation: returnType,
							Line:           lineNum,
							Parent:         parent,
							Exported:       true,
							DocComment:     docComment,
							Language:       "csharp",
						})
						goto countBraces
					}
				}
			}

			// Check for abstract/virtual method declarations that end with ;
			// e.g., "public abstract void Validate();"
			if p.hasAnyAccessMod(trimmed, csAccessMods) {
				absMethodPattern := regexp.MustCompile(`^\s*(?:public\s+|internal\s+|private\s+|protected\s+)(?:abstract\s+|virtual\s+)(\w+(?:<[^>]+>)?(?:\?)?)\s+(\w+)\s*\(([^)]*)\)\s*;`)
				if matches := absMethodPattern.FindStringSubmatch(trimmed); matches != nil {
					returnType := matches[1]
					methodName := matches[2]
					params := matches[3]
					if !p.isKeyword(methodName) {
						signature := fmt.Sprintf("%s %s(%s)", returnType, methodName, params)
						exported := p.hasAccessMod(trimmed, "public") || p.hasAccessMod(trimmed, "internal")
						symbols = append(symbols, Symbol{
							Name:           methodName,
							Type:           "method",
							Signature:      signature,
							TypeAnnotation: returnType,
							Line:           lineNum,
							Parent:         parent,
							Exported:       exported,
							DocComment:     docComment,
							Language:       "csharp",
						})
						goto countBraces
					}
				}
			}

			// If we consumed docComment but didn't use it on any symbol, that's OK
			_ = docComment
		}

	countBraces:
		// Track brace depth for scope management
		// Skip braces inside strings and comments
		inString := false
		inChar := false
		escaped := false
		for _, ch := range line {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' && !inChar {
				inString = !inString
				continue
			}
			if ch == '\'' && !inString {
				inChar = !inChar
				continue
			}
			if inString || inChar {
				continue
			}
			switch ch {
			case '{':
				braceDepth++
			case '}':
				braceDepth--
				if braceDepth < 0 {
					braceDepth = 0
				}
				for len(scopeStack) > 0 && scopeStack[len(scopeStack)-1].depth >= braceDepth {
					poppedEntry := scopeStack[len(scopeStack)-1]
					scopeStack = scopeStack[:len(scopeStack)-1]
					if poppedEntry.kind == "enum" {
						inEnumValues = false
					}
				}
			}
		}
	}

	return symbols
}

// hasAccessMod checks if a line contains a specific access modifier as a word
func (p *CSharpParser) hasAccessMod(line, mod string) bool {
	// Check for the modifier as a whole word
	return strings.Contains(" "+line+" ", " "+mod+" ")
}

// hasAnyAccessMod checks if a line contains any of the given access modifiers
func (p *CSharpParser) hasAnyAccessMod(line string, mods []string) bool {
	for _, mod := range mods {
		if p.hasAccessMod(line, mod) {
			return true
		}
	}
	return false
}

// isKeyword checks if a name is a C# keyword that should not be treated as a method/type name
func (p *CSharpParser) isKeyword(name string) bool {
	keywords := map[string]bool{
		"if": true, "else": true, "for": true, "foreach": true, "while": true,
		"do": true, "switch": true, "case": true, "break": true, "continue": true,
		"return": true, "throw": true, "try": true, "catch": true, "finally": true,
		"using": true, "namespace": true, "class": true, "struct": true, "interface": true,
		"enum": true, "delegate": true, "event": true, "new": true, "typeof": true,
		"sizeof": true, "checked": true, "unchecked": true, "default": true,
		"lock": true, "fixed": true, "stackalloc": true, "yield": true,
		"var": true, "dynamic": true, "object": true, "string": true,
		"void": true, "null": true, "true": true, "false": true, "this": true,
		"base": true, "ref": true, "out": true, "in": true, "params": true,
		"get": true, "set": true, "add": true, "remove": true, "value": true,
		"where": true, "select": true, "from": true, "extends": true, "implements": true,
	}
	return keywords[name]
}
