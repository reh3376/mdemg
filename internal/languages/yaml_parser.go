package languages

import (
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func init() {
	Register(&YAMLParser{})
}

// YAMLParser implements LanguageParser for YAML configuration files
type YAMLParser struct{}

func (p *YAMLParser) Name() string {
	return "yaml"
}

func (p *YAMLParser) Extensions() []string {
	return []string{".yml", ".yaml"}
}

func (p *YAMLParser) CanParse(path string) bool {
	pathLower := strings.ToLower(path)
	if !strings.HasSuffix(pathLower, ".yml") && !strings.HasSuffix(pathLower, ".yaml") {
		return false
	}

	// Skip OpenAPI/Swagger specs - they have their own parser
	content, err := ReadFileContent(path)
	if err != nil {
		return true // Let it try if we can't read
	}
	contentLower := strings.ToLower(content)
	if strings.Contains(contentLower, "openapi:") ||
		strings.Contains(contentLower, `"openapi":`) ||
		strings.Contains(contentLower, "swagger:") ||
		strings.Contains(contentLower, `"swagger":`) {
		return false // Let OpenAPI parser handle it
	}

	return true
}

func (p *YAMLParser) IsTestFile(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.Contains(pathLower, "/fixtures/") ||
		strings.Contains(pathLower, "/testdata/") ||
		strings.Contains(pathLower, "_test.")
}

func (p *YAMLParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)

	// Detect file type based on filename and content
	fileKind := p.detectFileKind(fileName, content)

	// Build summary content
	var contentBuilder strings.Builder
	contentBuilder.WriteString("YAML file: " + fileName + "\n")
	contentBuilder.WriteString("Type: " + fileKind + "\n")

	// Extract symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	contentBuilder.WriteString("\n--- Content ---\n")
	contentBuilder.WriteString(TruncateContent(content, 4000))

	// Detect concerns
	concerns := DetectConcerns(relPath, content)
	tags := []string{"yaml", "config", fileKind}
	tags = append(tags, concerns...)

	element := CodeElement{
		Name:        fileName,
		Kind:        fileKind,
		Path:        "/" + relPath,
		Content:     contentBuilder.String(),
		Package:     "config",
		FilePath:    relPath,
		Tags:        tags,
		Concerns:    concerns,
		Symbols:     symbols,
		ElementKind: "file",
	}

	return []CodeElement{element}, nil
}

func (p *YAMLParser) detectFileKind(fileName string, content string) string {
	fileNameLower := strings.ToLower(fileName)
	contentLower := strings.ToLower(content)

	// Check for GitHub Actions
	if strings.Contains(fileNameLower, "workflow") || strings.Contains(content, "runs-on:") {
		return "github-actions"
	}

	// Check for Kubernetes
	if strings.Contains(contentLower, "apiversion:") && strings.Contains(contentLower, "kind:") {
		return "kubernetes"
	}

	// Check for Docker Compose
	if fileNameLower == "docker-compose.yml" || fileNameLower == "docker-compose.yaml" ||
		fileNameLower == "compose.yml" || fileNameLower == "compose.yaml" ||
		strings.Contains(contentLower, "services:") {
		return "docker-compose"
	}

	// Check for Ansible
	if strings.Contains(contentLower, "hosts:") && strings.Contains(contentLower, "tasks:") {
		return "ansible"
	}

	// Check for CI configs
	if fileNameLower == ".gitlab-ci.yml" || fileNameLower == "azure-pipelines.yml" {
		return "ci-config"
	}

	// Check for Helm
	if fileNameLower == "chart.yaml" || fileNameLower == "values.yaml" {
		return "helm"
	}

	return "yaml-config"
}

func (p *YAMLParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol

	// Parse YAML with line number preservation
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(content), &root); err != nil {
		return symbols
	}

	// Root is typically a document node containing the actual content
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		p.walkNode(root.Content[0], "", &symbols)
	}

	return symbols
}

func (p *YAMLParser) walkNode(node *yaml.Node, prefix string, symbols *[]Symbol) {
	if node == nil {
		return
	}

	switch node.Kind {
	case yaml.MappingNode:
		// Process key-value pairs
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]

			key := keyNode.Value
			fullKey := key
			if prefix != "" {
				fullKey = prefix + "." + key
			}

			// Check for anchor
			docComment := ""
			if keyNode.Anchor != "" {
				docComment = "anchor: " + keyNode.Anchor
			} else if valueNode.Anchor != "" {
				docComment = "anchor: " + valueNode.Anchor
			}

			// Check for alias
			if valueNode.Kind == yaml.AliasNode {
				docComment = "merges: " + valueNode.Value
			}

			switch valueNode.Kind {
			case yaml.ScalarNode:
				// Simple value - extract as constant
				*symbols = append(*symbols, Symbol{
					Name:       fullKey,
					Type:       "constant",
					Line:       keyNode.Line,
					Value:      valueNode.Value,
					Parent:     prefix,
					Exported:   true,
					DocComment: docComment,
					Language:   "yaml",
				})

			case yaml.MappingNode:
				// Nested mapping - extract as section
				*symbols = append(*symbols, Symbol{
					Name:       fullKey,
					Type:       "section",
					Line:       keyNode.Line,
					Parent:     prefix,
					Exported:   true,
					DocComment: docComment,
					Language:   "yaml",
				})
				// Recurse into nested mapping
				p.walkNode(valueNode, fullKey, symbols)

			case yaml.SequenceNode:
				// Array - extract as constant (array type)
				*symbols = append(*symbols, Symbol{
					Name:       fullKey,
					Type:       "constant",
					Line:       keyNode.Line,
					Parent:     prefix,
					Exported:   true,
					DocComment: docComment,
					Language:   "yaml",
				})
				// Check if array contains mappings (e.g., GHA steps)
				for _, item := range valueNode.Content {
					if item.Kind == yaml.MappingNode {
						// For named items in arrays, check for 'name' field
						for j := 0; j < len(item.Content); j += 2 {
							if item.Content[j].Value == "name" {
								itemName := item.Content[j+1].Value
								itemKey := fullKey + "[" + itemName + "]"
								*symbols = append(*symbols, Symbol{
									Name:     itemKey,
									Type:     "section",
									Line:     item.Line,
									Parent:   fullKey,
									Exported: true,
									Language: "yaml",
								})
								break
							}
						}
					}
				}

			case yaml.AliasNode:
				// Alias reference - extract section with merge info
				*symbols = append(*symbols, Symbol{
					Name:       fullKey,
					Type:       "section",
					Line:       keyNode.Line,
					Parent:     prefix,
					Exported:   true,
					DocComment: "merges: " + valueNode.Value,
					Language:   "yaml",
				})
			}
		}
	}
}
