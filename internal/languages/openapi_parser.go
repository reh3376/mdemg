package languages

import (
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&OpenAPIParser{})
}

// OpenAPIParser implements LanguageParser for OpenAPI/Swagger specification files
type OpenAPIParser struct{}

func (p *OpenAPIParser) Name() string {
	return "openapi"
}

func (p *OpenAPIParser) Extensions() []string {
	// OpenAPI specs are typically YAML or JSON, but we detect by content
	// Return empty to avoid conflicts with yaml/json parsers
	return []string{}
}

func (p *OpenAPIParser) CanParse(path string) bool {
	// Check if file could be OpenAPI (yaml/json extension)
	pathLower := strings.ToLower(path)
	if !strings.HasSuffix(pathLower, ".yaml") &&
		!strings.HasSuffix(pathLower, ".yml") &&
		!strings.HasSuffix(pathLower, ".json") {
		return false
	}

	// Read content to check for OpenAPI markers
	content, err := ReadFileContent(path)
	if err != nil {
		return false
	}

	return p.isOpenAPISpec(content)
}

func (p *OpenAPIParser) isOpenAPISpec(content string) bool {
	// Check for OpenAPI 3.x or Swagger 2.x markers
	contentLower := strings.ToLower(content)

	// OpenAPI 3.x
	if strings.Contains(contentLower, "openapi:") ||
		strings.Contains(contentLower, `"openapi":`) {
		return true
	}

	// Swagger 2.x
	if strings.Contains(contentLower, "swagger:") ||
		strings.Contains(contentLower, `"swagger":`) {
		return true
	}

	return false
}

func (p *OpenAPIParser) IsTestFile(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.Contains(pathLower, "/fixtures/") ||
		strings.Contains(pathLower, "/testdata/") ||
		strings.Contains(pathLower, "_test.")
}

func (p *OpenAPIParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)

	// Build summary content
	var contentBuilder strings.Builder
	contentBuilder.WriteString("OpenAPI specification: " + fileName + "\n")

	// Detect version
	version := p.detectVersion(content)
	if version != "" {
		contentBuilder.WriteString("Version: " + version + "\n")
	}

	// Extract API info
	info := p.extractInfo(content)
	if info != "" {
		contentBuilder.WriteString("API: " + info + "\n")
	}

	// Extract symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	contentBuilder.WriteString("\n--- Content ---\n")
	contentBuilder.WriteString(TruncateContent(content, 4000))

	// Detect concerns
	concerns := DetectConcerns(relPath, content)
	tags := []string{"openapi", "api", "rest", "swagger"}
	if version != "" {
		tags = append(tags, version)
	}
	tags = append(tags, concerns...)

	element := CodeElement{
		Name:        fileName,
		Kind:        "openapi",
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

// Regex patterns for OpenAPI parsing
var (
	// Version detection
	openapiVersionPattern = regexp.MustCompile(`(?m)^openapi:\s*["']?([0-9.]+)["']?`)
	swaggerVersionPattern = regexp.MustCompile(`(?m)^swagger:\s*["']?([0-9.]+)["']?`)

	// Info section
	openapiTitlePattern   = regexp.MustCompile(`(?m)^\s{2}title:\s*["']?([^"'\n]+)["']?`)
	openapiVersionInfoPattern = regexp.MustCompile(`(?m)^\s{2}version:\s*["']?([^"'\n]+)["']?`)

	// Paths - look for path patterns at correct indentation
	openapiPathPattern = regexp.MustCompile(`(?m)^  (/[^:\n]*):`)

	// HTTP methods under paths
	openapiMethodPattern = regexp.MustCompile(`(?m)^    (get|post|put|delete|patch|options|head):\s*$`)

	// Operation ID
	openapiOperationIdPattern = regexp.MustCompile(`(?m)^\s+operationId:\s*["']?([^"'\n]+)["']?`)

	// Tags on operations
	openapiTagPattern = regexp.MustCompile(`(?m)^\s+tags:\s*$`)

	// Parameters
	openapiParameterNamePattern = regexp.MustCompile(`(?m)^\s+-\s+name:\s*["']?([^"'\n]+)["']?`)
	openapiParameterInPattern   = regexp.MustCompile(`(?m)^\s+in:\s*["']?([^"'\n]+)["']?`)

	// Schemas in components
	openapiSchemaPattern = regexp.MustCompile(`(?m)^    (\w+):\s*$`)

	// Security schemes
	openapiSecuritySchemePattern = regexp.MustCompile(`(?m)^    (\w+):\s*$`)

	// Server URLs
	openapiServerUrlPattern = regexp.MustCompile(`(?m)^\s+-?\s*url:\s*["']?([^"'\n]+)["']?`)
)

func (p *OpenAPIParser) detectVersion(content string) string {
	if matches := openapiVersionPattern.FindStringSubmatch(content); matches != nil {
		return "openapi-" + matches[1]
	}
	if matches := swaggerVersionPattern.FindStringSubmatch(content); matches != nil {
		return "swagger-" + matches[1]
	}
	return ""
}

func (p *OpenAPIParser) extractInfo(content string) string {
	var title, version string
	if matches := openapiTitlePattern.FindStringSubmatch(content); matches != nil {
		title = strings.TrimSpace(matches[1])
	}
	if matches := openapiVersionInfoPattern.FindStringSubmatch(content); matches != nil {
		version = strings.TrimSpace(matches[1])
	}
	if title != "" && version != "" {
		return title + " v" + version
	}
	if title != "" {
		return title
	}
	return ""
}

func (p *OpenAPIParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	var currentSection string // "paths", "components", "servers", etc.
	var currentPath string
	var currentMethod string
	var inParameters bool
	var inSecuritySchemes bool
	var inSchemas bool

	for lineNum, line := range lines {
		lineNo := lineNum + 1
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Detect top-level sections
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			if strings.HasPrefix(trimmed, "paths:") {
				currentSection = "paths"
				currentPath = ""
				currentMethod = ""
				continue
			}
			if strings.HasPrefix(trimmed, "components:") {
				currentSection = "components"
				currentPath = ""
				currentMethod = ""
				continue
			}
			if strings.HasPrefix(trimmed, "servers:") {
				currentSection = "servers"
				symbols = append(symbols, Symbol{
					Name:     "servers",
					Type:     "section",
					Line:     lineNo,
					Exported: true,
					Language: "openapi",
				})
				continue
			}
			if strings.HasPrefix(trimmed, "security:") {
				currentSection = "security"
				continue
			}
			if strings.HasPrefix(trimmed, "tags:") {
				currentSection = "tags"
				continue
			}
			if strings.HasPrefix(trimmed, "info:") {
				currentSection = "info"
				// Extract info as a symbol
				symbols = append(symbols, Symbol{
					Name:     "info",
					Type:     "section",
					Line:     lineNo,
					Exported: true,
					Language: "openapi",
				})
				continue
			}
			if strings.HasPrefix(trimmed, "openapi:") || strings.HasPrefix(trimmed, "swagger:") {
				currentSection = "version"
				continue
			}
		}

		// Handle paths section
		if currentSection == "paths" {
			// Check for path definition (2 spaces indent)
			if strings.HasPrefix(line, "  /") && !strings.HasPrefix(line, "    ") {
				if matches := openapiPathPattern.FindStringSubmatch(line); matches != nil {
					currentPath = matches[1]
					currentMethod = ""
					inParameters = false
					symbols = append(symbols, Symbol{
						Name:     currentPath,
						Type:     "endpoint",
						Line:     lineNo,
						Exported: true,
						Language: "openapi",
					})
					continue
				}
			}

			// Check for HTTP method (4 spaces indent)
			if currentPath != "" && strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "      ") {
				methodLower := strings.ToLower(strings.TrimSuffix(trimmed, ":"))
				if isHTTPMethod(methodLower) {
					currentMethod = strings.ToUpper(methodLower)
					inParameters = false
					symbols = append(symbols, Symbol{
						Name:       currentMethod,
						Type:       "method",
						Line:       lineNo,
						Parent:     currentPath,
						Exported:   true,
						Language:   "openapi",
					})
					continue
				}
			}

			// Check for operationId
			if currentPath != "" && currentMethod != "" {
				if matches := openapiOperationIdPattern.FindStringSubmatch(line); matches != nil {
					opId := strings.TrimSpace(matches[1])
					symbols = append(symbols, Symbol{
						Name:       opId,
						Type:       "function",
						Line:       lineNo,
						Parent:     currentPath + "." + currentMethod,
						DocComment: "operationId",
						Exported:   true,
						Language:   "openapi",
					})
					continue
				}
			}

			// Check for parameters section
			if strings.Contains(trimmed, "parameters:") {
				inParameters = true
				continue
			}

			// Check for parameter name
			if inParameters && currentPath != "" {
				if matches := openapiParameterNamePattern.FindStringSubmatch(line); matches != nil {
					paramName := strings.TrimSpace(matches[1])
					parent := currentPath
					if currentMethod != "" {
						parent = currentPath + "." + currentMethod
					}
					symbols = append(symbols, Symbol{
						Name:     paramName,
						Type:     "parameter",
						Line:     lineNo,
						Parent:   parent,
						Exported: true,
						Language: "openapi",
					})
					continue
				}
			}
		}

		// Handle components section
		if currentSection == "components" {
			// Check for schemas subsection
			if strings.HasPrefix(line, "  schemas:") {
				inSchemas = true
				inSecuritySchemes = false
				continue
			}

			// Check for securitySchemes subsection
			if strings.HasPrefix(line, "  securitySchemes:") {
				inSecuritySchemes = true
				inSchemas = false
				continue
			}

			// Extract schema names (4 spaces indent under schemas)
			if inSchemas && strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "      ") {
				schemaName := strings.TrimSuffix(strings.TrimSpace(line), ":")
				if schemaName != "" && !strings.Contains(schemaName, " ") {
					symbols = append(symbols, Symbol{
						Name:     schemaName,
						Type:     "class",
						Line:     lineNo,
						Parent:   "components.schemas",
						Exported: true,
						Language: "openapi",
					})
					continue
				}
			}

			// Extract security scheme names
			if inSecuritySchemes && strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "      ") {
				schemeName := strings.TrimSuffix(strings.TrimSpace(line), ":")
				if schemeName != "" && !strings.Contains(schemeName, " ") {
					symbols = append(symbols, Symbol{
						Name:     schemeName,
						Type:     "constant",
						Line:     lineNo,
						Parent:   "components.securitySchemes",
						Exported: true,
						Language: "openapi",
					})
					continue
				}
			}
		}

		// Handle servers section
		if currentSection == "servers" {
			if matches := openapiServerUrlPattern.FindStringSubmatch(line); matches != nil {
				serverUrl := strings.TrimSpace(matches[1])
				symbols = append(symbols, Symbol{
					Name:     serverUrl,
					Type:     "constant",
					Line:     lineNo,
					Parent:   "servers",
					Exported: true,
					Language: "openapi",
				})
				continue
			}
		}
	}

	return symbols
}

func isHTTPMethod(s string) bool {
	methods := []string{"get", "post", "put", "delete", "patch", "options", "head", "trace"}
	for _, m := range methods {
		if s == m {
			return true
		}
	}
	return false
}
