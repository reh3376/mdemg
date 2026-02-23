package languages

import (
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&ProtobufParser{})
}

// ProtobufParser implements LanguageParser for Protocol Buffer files (.proto)
type ProtobufParser struct{}

func (p *ProtobufParser) Name() string {
	return "protobuf"
}

func (p *ProtobufParser) Extensions() []string {
	return []string{".proto"}
}

func (p *ProtobufParser) CanParse(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".proto")
}

func (p *ProtobufParser) IsTestFile(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.Contains(pathLower, "/fixtures/") ||
		strings.Contains(pathLower, "/testdata/") ||
		strings.Contains(pathLower, "_test.")
}

func (p *ProtobufParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)

	// Build summary content
	var contentBuilder strings.Builder
	contentBuilder.WriteString("Protocol Buffer file: " + fileName + "\n")

	// Extract symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	// Detect package
	pkg := p.extractPackage(content)
	if pkg != "" {
		contentBuilder.WriteString("Package: " + pkg + "\n")
	}

	// Detect syntax version
	syntax := p.extractSyntax(content)
	if syntax != "" {
		contentBuilder.WriteString("Syntax: " + syntax + "\n")
	}

	contentBuilder.WriteString("\n--- Content ---\n")
	contentBuilder.WriteString(TruncateContent(content, 4000))

	// Detect concerns
	concerns := DetectConcerns(relPath, content)
	tags := []string{"protobuf", "proto", "grpc", "api"}
	if syntax != "" {
		tags = append(tags, syntax)
	}
	tags = append(tags, concerns...)

	element := CodeElement{
		Name:        fileName,
		Kind:        "protobuf",
		Path:        "/" + relPath,
		Content:     contentBuilder.String(),
		Package:     pkg,
		FilePath:    relPath,
		Tags:        tags,
		Concerns:    concerns,
		Symbols:     symbols,
		ElementKind: "file",
	}

	return []CodeElement{element}, nil
}

// Regex patterns for Protocol Buffer parsing
var (
	protoSyntaxPattern  = regexp.MustCompile(`(?m)^\s*syntax\s*=\s*"(proto[23])"\s*;`)
	protoPackagePattern = regexp.MustCompile(`(?m)^\s*package\s+([\w.]+)\s*;`)
	protoImportPattern  = regexp.MustCompile(`(?m)^\s*import\s+(?:public\s+|weak\s+)?"([^"]+)"\s*;`)
	protoOptionPattern  = regexp.MustCompile(`(?m)^\s*option\s+(\w+)\s*=\s*"([^"]+)"\s*;`)
	protoMessagePattern = regexp.MustCompile(`(?m)^\s*message\s+(\w+)\s*\{`)
	protoEnumPattern    = regexp.MustCompile(`(?m)^\s*enum\s+(\w+)\s*\{`)
	protoServicePattern = regexp.MustCompile(`(?m)^\s*service\s+(\w+)\s*\{`)
	protoRpcPattern     = regexp.MustCompile(`(?m)^\s*rpc\s+(\w+)\s*\(\s*(stream\s+)?(\w+)\s*\)\s*returns\s*\(\s*(stream\s+)?(\w+)\s*\)`)
	protoFieldPattern   = regexp.MustCompile(`(?m)^\s*(repeated\s+|optional\s+|required\s+)?(map<[^>]+>|[\w.]+)\s+(\w+)\s*=\s*(\d+)`)
	protoEnumValuePattern = regexp.MustCompile(`(?m)^\s*(\w+)\s*=\s*(-?\d+)\s*;`)
	protoOneofPattern   = regexp.MustCompile(`(?m)^\s*oneof\s+(\w+)\s*\{`)
)

func (p *ProtobufParser) extractSyntax(content string) string {
	if matches := protoSyntaxPattern.FindStringSubmatch(content); matches != nil {
		return matches[1]
	}
	return ""
}

func (p *ProtobufParser) extractPackage(content string) string {
	if matches := protoPackagePattern.FindStringSubmatch(content); matches != nil {
		return matches[1]
	}
	return ""
}

func (p *ProtobufParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	var scopeStack []string
	var scopeTypes []string // "message", "enum", "service", "oneof"
	braceDepth := 0

	for lineNum, line := range lines {
		lineNo := lineNum + 1
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
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

		// Check for package
		if matches := protoPackagePattern.FindStringSubmatch(trimmed); matches != nil {
			symbols = append(symbols, Symbol{
				Name:     matches[1],
				Type:     "namespace",
				Line:     lineNo,
				Exported: true,
				Language: "protobuf",
			})
			continue
		}

		// Check for option
		if matches := protoOptionPattern.FindStringSubmatch(trimmed); matches != nil {
			optName := matches[1]
			optValue := matches[2] // Already captured without quotes
			symbols = append(symbols, Symbol{
				Name:     optName,
				Type:     "constant",
				Value:    optValue,
				Line:     lineNo,
				Parent:   parent,
				Exported: true,
				Language: "protobuf",
			})
			continue
		}

		// Check for message
		if matches := protoMessagePattern.FindStringSubmatch(trimmed); matches != nil {
			msgName := matches[1]
			fullName := msgName
			if parent != "" {
				fullName = parent + "." + msgName
			}
			symbols = append(symbols, Symbol{
				Name:     fullName,
				Type:     "class",
				Line:     lineNo,
				Parent:   parent,
				Exported: true,
				Language: "protobuf",
			})
			if openBraces > 0 {
				scopeStack = append(scopeStack, fullName)
				scopeTypes = append(scopeTypes, "message")
				braceDepth++
			}
			continue
		}

		// Check for enum
		if matches := protoEnumPattern.FindStringSubmatch(trimmed); matches != nil {
			enumName := matches[1]
			fullName := enumName
			if parent != "" {
				fullName = parent + "." + enumName
			}
			symbols = append(symbols, Symbol{
				Name:     fullName,
				Type:     "enum",
				Line:     lineNo,
				Parent:   parent,
				Exported: true,
				Language: "protobuf",
			})
			if openBraces > 0 {
				scopeStack = append(scopeStack, fullName)
				scopeTypes = append(scopeTypes, "enum")
				braceDepth++
			}
			continue
		}

		// Check for service
		if matches := protoServicePattern.FindStringSubmatch(trimmed); matches != nil {
			svcName := matches[1]
			symbols = append(symbols, Symbol{
				Name:     svcName,
				Type:     "interface",
				Line:     lineNo,
				Exported: true,
				Language: "protobuf",
			})
			if openBraces > 0 {
				scopeStack = append(scopeStack, svcName)
				scopeTypes = append(scopeTypes, "service")
				braceDepth++
			}
			continue
		}

		// Check for RPC method
		if matches := protoRpcPattern.FindStringSubmatch(trimmed); matches != nil {
			rpcName := matches[1]
			reqStream := matches[2] != ""
			reqType := matches[3]
			respStream := matches[4] != ""
			respType := matches[5]

			var signature strings.Builder
			signature.WriteString("rpc ")
			signature.WriteString(rpcName)
			signature.WriteString("(")
			if reqStream {
				signature.WriteString("stream ")
			}
			signature.WriteString(reqType)
			signature.WriteString(") returns (")
			if respStream {
				signature.WriteString("stream ")
			}
			signature.WriteString(respType)
			signature.WriteString(")")

			symbols = append(symbols, Symbol{
				Name:      rpcName,
				Type:      "method",
				Signature: signature.String(),
				Line:      lineNo,
				Parent:    parent,
				Exported:  true,
				Language:  "protobuf",
			})
			continue
		}

		// Check for oneof
		if matches := protoOneofPattern.FindStringSubmatch(trimmed); matches != nil {
			oneofName := matches[1]
			symbols = append(symbols, Symbol{
				Name:     oneofName,
				Type:     "section",
				Line:     lineNo,
				Parent:   parent,
				Exported: true,
				Language: "protobuf",
			})
			if openBraces > 0 {
				scopeStack = append(scopeStack, parent+"."+oneofName)
				scopeTypes = append(scopeTypes, "oneof")
				braceDepth++
			}
			continue
		}

		// Check for enum value (inside enum scope)
		if len(scopeTypes) > 0 && scopeTypes[len(scopeTypes)-1] == "enum" {
			if matches := protoEnumValuePattern.FindStringSubmatch(trimmed); matches != nil {
				valueName := matches[1]
				valueNum := matches[2]
				symbols = append(symbols, Symbol{
					Name:     valueName,
					Type:     "enum_value",
					Value:    valueNum,
					Line:     lineNo,
					Parent:   parent,
					Exported: true,
					Language: "protobuf",
				})
				continue
			}
		}

		// Check for field (inside message scope)
		if len(scopeTypes) > 0 && (scopeTypes[len(scopeTypes)-1] == "message" || scopeTypes[len(scopeTypes)-1] == "oneof") {
			if matches := protoFieldPattern.FindStringSubmatch(trimmed); matches != nil {
				modifier := strings.TrimSpace(matches[1])
				fieldType := matches[2]
				fieldName := matches[3]
				fieldNum := matches[4]

				var typeAnnotation string
				if modifier != "" {
					typeAnnotation = modifier + fieldType
				} else {
					typeAnnotation = fieldType
				}

				symbols = append(symbols, Symbol{
					Name:           fieldName,
					Type:           "field",
					TypeAnnotation: typeAnnotation,
					Value:          fieldNum,
					Line:           lineNo,
					Parent:         parent,
					Exported:       true,
					Language:       "protobuf",
				})
				continue
			}
		}

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
