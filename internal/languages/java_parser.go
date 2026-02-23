package languages

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&JavaParser{})
}

// JavaParser implements LanguageParser for Java source files
type JavaParser struct{}

func (p *JavaParser) Name() string {
	return "java"
}

func (p *JavaParser) Extensions() []string {
	return []string{".java"}
}

func (p *JavaParser) CanParse(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".java")
}

func (p *JavaParser) IsTestFile(path string) bool {
	name := filepath.Base(path)
	return strings.HasSuffix(name, "Test.java") ||
		strings.HasSuffix(name, "Tests.java") ||
		strings.HasSuffix(name, "IT.java") ||
		strings.Contains(path, "/test/") ||
		strings.Contains(path, "/tests/")
}

func (p *JavaParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	var elements []CodeElement

	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)

	// Extract package name
	packageName := ""
	packageMatch := regexp.MustCompile(`^\s*package\s+([\w.]+)\s*;`).FindStringSubmatch(content)
	if packageMatch != nil {
		packageName = packageMatch[1]
	}

	// Find classes and interfaces
	classes := FindAllMatches(content, `(?:public\s+)?(?:abstract\s+)?(?:final\s+)?class\s+(\w+)`)
	interfaces := FindAllMatches(content, `(?:public\s+)?interface\s+(\w+)`)
	enums := FindAllMatches(content, `(?:public\s+)?enum\s+(\w+)`)

	// Find methods
	methods := FindAllMatches(content, `(?:public|private|protected)?\s*(?:static\s+)?(?:final\s+)?(?:synchronized\s+)?(?:\w+(?:<[^>]+>)?)\s+(\w+)\s*\([^)]*\)\s*(?:throws\s+[\w,\s]+)?\s*\{`)

	// Find imports
	imports := FindAllMatches(content, `^\s*import\s+([\w.*]+)\s*;`)

	// Build content for embedding
	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("Java file: %s\n", fileName))

	if packageName != "" {
		contentBuilder.WriteString(fmt.Sprintf("Package: %s\n", packageName))
	}
	if len(classes) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Classes: %s\n", strings.Join(uniqueStrings(classes), ", ")))
	}
	if len(interfaces) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Interfaces: %s\n", strings.Join(uniqueStrings(interfaces), ", ")))
	}
	if len(enums) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Enums: %s\n", strings.Join(uniqueStrings(enums), ", ")))
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
	contentBuilder.WriteString(fmt.Sprintf("Imports: %d\n", len(imports)))

	// Include actual code content
	contentBuilder.WriteString("\n--- Code ---\n")
	contentBuilder.WriteString(TruncateContent(content, 4000))

	// Detect cross-cutting concerns
	concerns := DetectConcerns(relPath, content)
	tags := []string{"java", "module"}
	tags = append(tags, concerns...)

	// Determine file kind based on content
	javaKind := "java-class"
	if len(interfaces) > 0 && len(classes) == 0 {
		javaKind = "java-interface"
		tags = append(tags, "interface")
	} else if len(enums) > 0 && len(classes) == 0 {
		javaKind = "java-enum"
		tags = append(tags, "enum")
	}

	// Extract code symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	// Add file-level element
	elements = append(elements, CodeElement{
		Name:     fileName,
		Kind:     javaKind,
		Path:     "/" + relPath,
		Content:  contentBuilder.String(),
		Package:  packageName,
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
			Content:  fmt.Sprintf("Java class '%s' in package %s (file: %s)", class, packageName, fileName),
			Package:  packageName,
			FilePath: relPath,
			Tags:     append([]string{"java", "class"}, concerns...),
			Concerns: concerns,
		})
	}

	// Add interfaces as separate elements
	for _, iface := range uniqueStrings(interfaces) {
		elements = append(elements, CodeElement{
			Name:     iface,
			Kind:     "interface",
			Path:     fmt.Sprintf("/%s#%s", relPath, iface),
			Content:  fmt.Sprintf("Java interface '%s' in package %s (file: %s)", iface, packageName, fileName),
			Package:  packageName,
			FilePath: relPath,
			Tags:     append([]string{"java", "interface"}, concerns...),
			Concerns: concerns,
		})
	}

	return elements, nil
}

func (p *JavaParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	// Patterns for Java constructs
	// Class: [modifiers] class Name [extends X] [implements Y]
	classPattern := regexp.MustCompile(`^\s*(public\s+)?(abstract\s+)?(final\s+)?class\s+(\w+)`)
	// Interface: [modifiers] interface Name [extends X]
	interfacePattern := regexp.MustCompile(`^\s*(public\s+)?interface\s+(\w+)`)
	// Enum: [modifiers] enum Name
	enumPattern := regexp.MustCompile(`^\s*(public\s+)?enum\s+(\w+)`)
	// Record: [modifiers] record Name(params)
	recordPattern := regexp.MustCompile(`^\s*(public\s+)?record\s+(\w+)\s*\(`)
	// Constant: [modifiers] static final TYPE NAME = value
	constPattern := regexp.MustCompile(`^\s*(?:public|private|protected)?\s*static\s+final\s+(\w+)\s+([A-Z][A-Z0-9_]*)\s*=\s*(.+?);`)
	// Method/Constructor: [modifiers] [TYPE] name(params)
	methodPattern := regexp.MustCompile(`^\s*(public|private|protected)?\s*(abstract\s+)?(static\s+)?(final\s+)?(synchronized\s+)?(?:(\w+(?:<[^>]+>)?)\s+)?(\w+)\s*\(([^)]*)\)`)
	// Enum values
	enumValuePattern := regexp.MustCompile(`^\s*([A-Z][A-Z0-9_]*)\s*(?:\([^)]*\))?\s*[,;]`)

	// Track context for parent assignment
	var scopeStack []string
	var braceDepth int
	var scopeDepths []int
	inEnum := false
	currentEnumName := ""

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		// Track brace depth
		openBraces := strings.Count(line, "{")
		closeBraces := strings.Count(line, "}")

		// Check for scope exit
		for closeBraces > 0 && len(scopeStack) > 0 && len(scopeDepths) > 0 {
			if braceDepth-closeBraces < scopeDepths[len(scopeDepths)-1] {
				scopeStack = scopeStack[:len(scopeStack)-1]
				scopeDepths = scopeDepths[:len(scopeDepths)-1]
				if inEnum && len(scopeStack) == 0 || (len(scopeStack) > 0 && scopeStack[len(scopeStack)-1] != currentEnumName) {
					inEnum = false
					currentEnumName = ""
				}
			}
			closeBraces--
		}
		braceDepth += openBraces - strings.Count(line, "}")

		// Check for class declaration
		if matches := classPattern.FindStringSubmatch(line); matches != nil {
			className := matches[4]
			isPublic := matches[1] != ""
			symbols = append(symbols, Symbol{
				Name:     className,
				Type:     "class",
				Line:     lineNum,
				Exported: isPublic,
				Language: "java",
			})
			if strings.Contains(line, "{") {
				scopeStack = append(scopeStack, className)
				scopeDepths = append(scopeDepths, braceDepth)
			}
			continue
		}

		// Check for interface declaration
		if matches := interfacePattern.FindStringSubmatch(line); matches != nil {
			ifaceName := matches[2]
			isPublic := matches[1] != ""
			symbols = append(symbols, Symbol{
				Name:     ifaceName,
				Type:     "interface",
				Line:     lineNum,
				Exported: isPublic,
				Language: "java",
			})
			if strings.Contains(line, "{") {
				scopeStack = append(scopeStack, ifaceName)
				scopeDepths = append(scopeDepths, braceDepth)
			}
			continue
		}

		// Check for enum declaration
		if matches := enumPattern.FindStringSubmatch(line); matches != nil {
			enumName := matches[2]
			isPublic := matches[1] != ""
			symbols = append(symbols, Symbol{
				Name:     enumName,
				Type:     "enum",
				Line:     lineNum,
				Exported: isPublic,
				Language: "java",
			})
			if strings.Contains(line, "{") {
				scopeStack = append(scopeStack, enumName)
				scopeDepths = append(scopeDepths, braceDepth)
				inEnum = true
				currentEnumName = enumName
			}
			continue
		}

		// Check for record declaration
		if matches := recordPattern.FindStringSubmatch(line); matches != nil {
			recordName := matches[2]
			isPublic := matches[1] != ""
			symbols = append(symbols, Symbol{
				Name:     recordName,
				Type:     "class",
				Line:     lineNum,
				Exported: isPublic,
				Language: "java",
			})
			if strings.Contains(line, "{") {
				scopeStack = append(scopeStack, recordName)
				scopeDepths = append(scopeDepths, braceDepth)
			}
			continue
		}

		// Extract enum values
		if inEnum {
			if matches := enumValuePattern.FindStringSubmatch(line); matches != nil {
				parent := ""
				if len(scopeStack) > 0 {
					parent = scopeStack[len(scopeStack)-1]
				}
				symbols = append(symbols, Symbol{
					Name:     matches[1],
					Type:     "enum_value",
					Parent:   parent,
					Line:     lineNum,
					Exported: true,
					Language: "java",
				})
			}
			// Continue to also check for methods inside enum
		}

		// Check for constants (public static final)
		if matches := constPattern.FindStringSubmatch(line); matches != nil {
			valueStr := strings.TrimSpace(matches[3])
			if len(valueStr) > 100 {
				valueStr = valueStr[:100] + "..."
			}
			parent := ""
			if len(scopeStack) > 0 {
				parent = scopeStack[len(scopeStack)-1]
			}
			symbols = append(symbols, Symbol{
				Name:           matches[2],
				Type:           "constant",
				TypeAnnotation: matches[1],
				Value:          valueStr,
				Parent:         parent,
				Line:           lineNum,
				Exported:       strings.Contains(line, "public"),
				Language:       "java",
			})
			continue
		}

		// Check for methods and constructors
		if matches := methodPattern.FindStringSubmatch(line); matches != nil {
			visibility := matches[1]
			returnType := matches[6]
			methodName := matches[7]
			params := matches[8]

			// Skip if this looks like a control statement
			if methodName == "if" || methodName == "while" || methodName == "for" || methodName == "switch" || methodName == "catch" {
				continue
			}

			parent := ""
			if len(scopeStack) > 0 {
				parent = scopeStack[len(scopeStack)-1]
			}

			// Determine if this is a constructor (no return type and name matches parent)
			symType := "method"
			if returnType == "" && parent == methodName {
				symType = "method" // Constructor is still a method type
				returnType = methodName
			}

			signature := fmt.Sprintf("%s(%s)", methodName, params)
			if returnType != "" && returnType != methodName {
				signature = fmt.Sprintf("%s %s(%s)", returnType, methodName, params)
			}
			if len(signature) > 150 {
				signature = signature[:150] + "..."
			}

			symbols = append(symbols, Symbol{
				Name:           methodName,
				Type:           symType,
				Parent:         parent,
				Signature:      signature,
				TypeAnnotation: returnType,
				Line:           lineNum,
				Exported:       visibility == "public",
				Language:       "java",
			})
		}
	}

	return symbols
}
