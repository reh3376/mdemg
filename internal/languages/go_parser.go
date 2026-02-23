package languages

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&GoParser{})
}

// GoParser implements LanguageParser for Go source files
type GoParser struct {
	fset *token.FileSet
}

func (p *GoParser) Name() string {
	return "go"
}

func (p *GoParser) Extensions() []string {
	return []string{".go"}
}

func (p *GoParser) CanParse(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".go")
}

func (p *GoParser) IsTestFile(path string) bool {
	return strings.HasSuffix(path, "_test.go") ||
		strings.Contains(path, "/testdata/")
}

func (p *GoParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	var elements []CodeElement

	if p.fset == nil {
		p.fset = token.NewFileSet()
	}

	file, err := parser.ParseFile(p.fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	pkgName := file.Name.Name

	// Read file content for concern detection
	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	concerns := DetectConcerns(relPath, content)
	tags := []string{"go", "package", pkgName}
	tags = append(tags, concerns...)

	// Check if this is a configuration file
	kind := "package"
	if IsConfigFile(relPath) {
		tags = append(tags, "config")
		kind = "config"
	}

	// Build content for embedding
	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("Go package %s in file %s\n", pkgName, relPath))
	contentBuilder.WriteString("\n--- Code ---\n")
	truncated, wasTruncated := TruncateContentWithInfo(content, 4000)
	contentBuilder.WriteString(truncated)

	// Collect diagnostics
	var diagnostics []Diagnostic
	if wasTruncated {
		diagnostics = append(diagnostics, NewDiagnosticWithContext(
			"info", "TRUNCATED",
			fmt.Sprintf("Content truncated from %d to 4000 chars", len(content)),
			"go",
			map[string]string{"original_size": fmt.Sprintf("%d", len(content))},
		))
	}

	// Extract code symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	// Add package-level element
	elements = append(elements, CodeElement{
		Name:        pkgName,
		Kind:        kind,
		Path:        "/" + relPath,
		Content:     contentBuilder.String(),
		Package:     pkgName,
		FilePath:    relPath,
		Tags:        tags,
		Concerns:    concerns,
		Symbols:     symbols,
		Diagnostics: diagnostics,
	})

	// Extract declarations
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if elem := p.extractFunction(d, pkgName, relPath); elem != nil {
				elem.Tags = append(elem.Tags, concerns...)
				elem.Concerns = concerns
				elements = append(elements, *elem)
			}
		case *ast.GenDecl:
			for _, elem := range p.extractGenDecl(d, pkgName, relPath) {
				elem.Tags = append(elem.Tags, concerns...)
				elem.Concerns = concerns
				elements = append(elements, elem)
			}
		}
	}

	return elements, nil
}

func (p *GoParser) extractFunction(decl *ast.FuncDecl, pkgName, relPath string) *CodeElement {
	funcName := decl.Name.Name

	// Skip unexported functions
	if funcName[0] < 'A' || funcName[0] > 'Z' {
		return nil
	}

	// Build description from doc comment
	var doc string
	if decl.Doc != nil {
		doc = decl.Doc.Text()
	}

	// Build tags
	tags := []string{"go", "function", pkgName}

	// Check if it's a method
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		tags = append(tags, "method")
	}

	content := fmt.Sprintf("Go function %s.%s\n%s", pkgName, funcName, doc)

	return &CodeElement{
		Name:     funcName,
		Kind:     "function",
		Path:     "/" + relPath + "#" + funcName,
		Content:  content,
		Package:  pkgName,
		FilePath: relPath,
		Tags:     tags,
	}
}

func (p *GoParser) extractGenDecl(decl *ast.GenDecl, pkgName, relPath string) []CodeElement {
	var elements []CodeElement

	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			// Skip unexported types
			if s.Name.Name[0] < 'A' || s.Name.Name[0] > 'Z' {
				continue
			}

			kind := "type"
			tags := []string{"go", pkgName}

			switch s.Type.(type) {
			case *ast.StructType:
				kind = "struct"
				tags = append(tags, "struct")
			case *ast.InterfaceType:
				kind = "interface"
				tags = append(tags, "interface")
			}

			var doc string
			if decl.Doc != nil {
				doc = decl.Doc.Text()
			}

			content := fmt.Sprintf("Go %s %s.%s\n%s", kind, pkgName, s.Name.Name, doc)

			elements = append(elements, CodeElement{
				Name:     s.Name.Name,
				Kind:     kind,
				Path:     "/" + relPath + "#" + s.Name.Name,
				Content:  content,
				Package:  pkgName,
				FilePath: relPath,
				Tags:     tags,
			})
		}
	}

	return elements
}

func (p *GoParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	// Constants
	constPattern := regexp.MustCompile(`^\s*const\s+(\w+)\s*(?:\w+)?\s*=\s*(.+)$`)
	constBlockStart := regexp.MustCompile(`^\s*const\s*\(\s*$`)
	constBlockItem := regexp.MustCompile(`^\s*(\w+)\s*(?:\w+)?\s*=\s*(.+)$`)

	// Type declarations
	typeAliasPattern := regexp.MustCompile(`^\s*type\s+(\w+)\s+(\S+(?:\s*//.*)?)\s*$`)
	structPattern := regexp.MustCompile("^\\s*type\\s+(\\w+)\\s+struct\\s*\\{")
	interfacePattern := regexp.MustCompile("^\\s*type\\s+(\\w+)\\s+interface\\s*\\{")

	// Functions and methods
	funcPattern := regexp.MustCompile("^func\\s+(\\w+)\\s*\\(([^)]*)\\)(?:\\s*\\(([^)]*)\\)|\\s*(\\w+))?\\s*\\{?")
	methodPattern := regexp.MustCompile("^func\\s+\\((\\w+)\\s+\\*?(\\w+)\\)\\s+(\\w+)\\s*\\(([^)]*)\\)(?:\\s*\\(([^)]*)\\)|\\s*(\\w+))?\\s*\\{?")

	inConstBlock := false
	var currentStruct string
	var currentInterface string

	for i, line := range lines {
		lineNum := i + 1
		trimmedLine := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmedLine == "" || strings.HasPrefix(trimmedLine, "//") {
			continue
		}

		// === Const block handling ===
		if constBlockStart.MatchString(line) {
			inConstBlock = true
			continue
		}

		if inConstBlock && trimmedLine == ")" {
			inConstBlock = false
			continue
		}

		if inConstBlock {
			if matches := constBlockItem.FindStringSubmatch(line); matches != nil {
				if isExported(matches[1]) {
					symbols = append(symbols, Symbol{
						Name:       matches[1],
						Type:       "constant",
						Value:      CleanValue(matches[2]),
						RawValue:   matches[2],
						Line: lineNum,
						Exported:   true,
						Language:   "go",
					})
				}
			}
			continue
		}

		// === Single const ===
		if matches := constPattern.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:       matches[1],
				Type:       "constant",
				Value:      CleanValue(matches[2]),
				RawValue:   matches[2],
				Line: lineNum,
				Exported:   isExported(matches[1]),
				Language:   "go",
			})
			continue
		}

		// === Struct definition ===
		if matches := structPattern.FindStringSubmatch(line); matches != nil {
			structName := matches[1]
			currentStruct = structName
			symbols = append(symbols, Symbol{
				Name:       structName,
				Type:       "struct",
				Signature:  fmt.Sprintf("type %s struct", structName),
				Line: lineNum,
				Exported:   isExported(structName),
				Language:   "go",
			})
			continue
		}

		// === Interface definition ===
		if matches := interfacePattern.FindStringSubmatch(line); matches != nil {
			interfaceName := matches[1]
			currentInterface = interfaceName
			symbols = append(symbols, Symbol{
				Name:       interfaceName,
				Type:       "interface",
				Signature:  fmt.Sprintf("type %s interface", interfaceName),
				Line: lineNum,
				Exported:   isExported(interfaceName),
				Language:   "go",
			})
			continue
		}

		// === Type alias (must check after struct/interface) ===
		if matches := typeAliasPattern.FindStringSubmatch(line); matches != nil {
			// Exclude struct and interface (already handled above via continue)
			baseType := strings.TrimSpace(matches[2])
			// Strip trailing inline comments
			if idx := strings.Index(baseType, "//"); idx != -1 {
				baseType = strings.TrimSpace(baseType[:idx])
			}
			if baseType != "struct" && baseType != "interface" &&
				!strings.HasPrefix(baseType, "struct") && !strings.HasPrefix(baseType, "interface") {
				symbols = append(symbols, Symbol{
					Name:           matches[1],
					Type:           "type",
					TypeAnnotation: baseType,
					Signature:      fmt.Sprintf("type %s %s", matches[1], baseType),
					Line:     lineNum,
					Exported:       isExported(matches[1]),
					Language:       "go",
				})
			}
			continue
		}

		// === Method (receiver function) ===
		if matches := methodPattern.FindStringSubmatch(line); matches != nil {
			receiverType := matches[2]
			methodName := matches[3]
			params := matches[4]
			returnType := ""
			if len(matches) > 5 && matches[5] != "" {
				returnType = matches[5]
			} else if len(matches) > 6 && matches[6] != "" {
				returnType = matches[6]
			}

			sig := fmt.Sprintf("func (*%s) %s(%s)", receiverType, methodName, params)
			if returnType != "" {
				sig += " " + returnType
			}

			symbols = append(symbols, Symbol{
				Name:           methodName,
				Type:           "method",
				Parent:         receiverType,
				Signature:      sig,
				TypeAnnotation: returnType,
				Line:     lineNum,
				Exported:       isExported(methodName),
				Language:       "go",
			})
			continue
		}

		// === Standalone function ===
		if matches := funcPattern.FindStringSubmatch(line); matches != nil {
			funcName := matches[1]
			params := matches[2]
			returnType := ""
			if len(matches) > 3 && matches[3] != "" {
				returnType = matches[3]
			} else if len(matches) > 4 && matches[4] != "" {
				returnType = matches[4]
			}

			sig := fmt.Sprintf("func %s(%s)", funcName, params)
			if returnType != "" {
				sig += " " + returnType
			}

			symbols = append(symbols, Symbol{
				Name:           funcName,
				Type:           "function",
				Signature:      sig,
				TypeAnnotation: returnType,
				Line:     lineNum,
				Exported:       isExported(funcName),
				Language:       "go",
			})
			continue
		}

		// Reset struct/interface context at closing brace
		if trimmedLine == "}" {
			if currentStruct != "" {
				currentStruct = ""
			}
			if currentInterface != "" {
				currentInterface = ""
			}
		}
	}

	return symbols
}

// isExported returns true if the name starts with an uppercase letter (Go export rule)
func isExported(name string) bool {
	if len(name) == 0 {
		return false
	}
	return name[0] >= 'A' && name[0] <= 'Z'
}
