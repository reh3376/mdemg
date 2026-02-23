package languages

import (
	"bufio"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&TerraformParser{})
}

// TerraformParser implements LanguageParser for Terraform/HCL files
type TerraformParser struct{}

func (p *TerraformParser) Name() string {
	return "terraform"
}

func (p *TerraformParser) Extensions() []string {
	return []string{".tf", ".tfvars"}
}

func (p *TerraformParser) CanParse(path string) bool {
	return HasExtension(path, p.Extensions())
}

func (p *TerraformParser) IsTestFile(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.Contains(pathLower, "/fixtures/") ||
		strings.Contains(pathLower, "/testdata/") ||
		strings.Contains(pathLower, "_test.")
}

func (p *TerraformParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)

	// Detect file kind based on extension
	fileKind := p.detectFileKind(path)

	// Build summary content
	var contentBuilder strings.Builder
	contentBuilder.WriteString("Terraform file: " + fileName + "\n")
	contentBuilder.WriteString("Type: " + fileKind + "\n")

	// Extract symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	// Summarize top-level blocks
	var blocks []string
	for _, sym := range symbols {
		if sym.Type == "section" {
			blocks = append(blocks, sym.Name)
		}
	}
	if len(blocks) > 0 {
		contentBuilder.WriteString("Blocks: " + strings.Join(blocks, ", ") + "\n")
	}

	contentBuilder.WriteString("\n--- Content ---\n")
	truncated, wasTruncated := TruncateContentWithInfo(content, 4000)
	contentBuilder.WriteString(truncated)

	// Collect diagnostics
	var diagnostics []Diagnostic
	if wasTruncated {
		diagnostics = append(diagnostics, NewDiagnosticWithContext(
			"info", "TRUNCATED",
			fmt.Sprintf("Content truncated from %d to 4000 chars", len(content)),
			"terraform",
			map[string]string{"original_size": fmt.Sprintf("%d", len(content))},
		))
	}

	// Detect concerns
	concerns := DetectConcerns(relPath, content)
	tags := []string{"terraform", "hcl", "iac", fileKind}
	tags = append(tags, concerns...)

	element := CodeElement{
		Name:        fileName,
		Kind:        fileKind,
		Path:        "/" + relPath,
		Content:     contentBuilder.String(),
		Package:     "infrastructure",
		FilePath:    relPath,
		Tags:        tags,
		Concerns:    concerns,
		Symbols:     symbols,
		ElementKind: "file",
		Diagnostics: diagnostics,
	}

	return []CodeElement{element}, nil
}

func (p *TerraformParser) detectFileKind(path string) string {
	pathLower := strings.ToLower(path)
	baseName := strings.ToLower(filepath.Base(path))

	if strings.HasSuffix(pathLower, ".tfvars") {
		return "terraform-vars"
	}

	// Detect by filename convention
	switch {
	case baseName == "main.tf":
		return "terraform-main"
	case baseName == "variables.tf":
		return "terraform-variables"
	case baseName == "outputs.tf":
		return "terraform-outputs"
	case baseName == "providers.tf":
		return "terraform-providers"
	case baseName == "backend.tf":
		return "terraform-backend"
	case baseName == "versions.tf":
		return "terraform-versions"
	default:
		return "terraform-config"
	}
}

// Regex patterns for Terraform/HCL block detection
var (
	// resource "type" "name" {
	tfResourceRegex = regexp.MustCompile(`^resource\s+"([^"]+)"\s+"([^"]+)"\s*\{`)
	// data "type" "name" {
	tfDataRegex = regexp.MustCompile(`^data\s+"([^"]+)"\s+"([^"]+)"\s*\{`)
	// module "name" {
	tfModuleRegex = regexp.MustCompile(`^module\s+"([^"]+)"\s*\{`)
	// provider "name" {
	tfProviderRegex = regexp.MustCompile(`^provider\s+"([^"]+)"\s*\{`)
	// variable "name" {
	tfVariableRegex = regexp.MustCompile(`^variable\s+"([^"]+)"\s*\{`)
	// output "name" {
	tfOutputRegex = regexp.MustCompile(`^output\s+"([^"]+)"\s*\{`)
	// locals {
	tfLocalsRegex = regexp.MustCompile(`^locals\s*\{`)
	// terraform {
	tfTerraformRegex = regexp.MustCompile(`^terraform\s*\{`)
	// key = value (inside blocks)
	tfAttrRegex = regexp.MustCompile(`^\s*(\w+)\s*=\s*(.+)`)
	// description = "..."
	tfDescriptionRegex = regexp.MustCompile(`^\s*description\s*=\s*"([^"]*)"`)
	// default = value
	tfDefaultRegex = regexp.MustCompile(`^\s*default\s*=\s*(.+)`)
	// value = expression
	tfValueRegex = regexp.MustCompile(`^\s*value\s*=\s*(.+)`)
)

func (p *TerraformParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol

	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNum := 0
	braceDepth := 0

	// State tracking
	type blockState struct {
		kind       string // "variable", "output", "locals", "resource", etc.
		name       string
		startLine  int
		depth      int // brace depth when block was entered
		desc       string
		value      string
		hasDefault bool
	}

	var currentBlock *blockState
	inLocals := false
	localsDepth := 0
	// Track inner brace depth for multi-line values inside locals
	localInnerDepth := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}

		// Count braces on this line
		openBraces := strings.Count(trimmed, "{")
		closeBraces := strings.Count(trimmed, "}")
		braceDepth += openBraces - closeBraces

		// Check for top-level blocks (only at depth 0 before this line's braces)
		priorDepth := braceDepth - openBraces + closeBraces

		if priorDepth == 0 {
			// terraform {
			if tfTerraformRegex.MatchString(trimmed) {
				symbols = append(symbols, Symbol{
					Name:     "terraform",
					Type:     "section",
					Line:     lineNum,
					Exported: true,
					Language: "terraform",
				})
				continue
			}

			// provider "name" {
			if matches := tfProviderRegex.FindStringSubmatch(trimmed); matches != nil {
				symbols = append(symbols, Symbol{
					Name:     "provider." + matches[1],
					Type:     "section",
					Line:     lineNum,
					Exported: true,
					Value:    matches[1],
					Language: "terraform",
				})
				continue
			}

			// resource "type" "name" {
			if matches := tfResourceRegex.FindStringSubmatch(trimmed); matches != nil {
				symbols = append(symbols, Symbol{
					Name:     matches[1] + "." + matches[2],
					Type:     "section",
					Line:     lineNum,
					Exported: true,
					Value:    matches[1],
					Language: "terraform",
				})
				continue
			}

			// data "type" "name" {
			if matches := tfDataRegex.FindStringSubmatch(trimmed); matches != nil {
				symbols = append(symbols, Symbol{
					Name:     "data." + matches[1] + "." + matches[2],
					Type:     "section",
					Line:     lineNum,
					Exported: true,
					Value:    matches[1],
					Language: "terraform",
				})
				continue
			}

			// module "name" {
			if matches := tfModuleRegex.FindStringSubmatch(trimmed); matches != nil {
				symbols = append(symbols, Symbol{
					Name:     "module." + matches[1],
					Type:     "section",
					Line:     lineNum,
					Exported: true,
					Language: "terraform",
				})
				continue
			}

			// variable "name" {
			if matches := tfVariableRegex.FindStringSubmatch(trimmed); matches != nil {
				currentBlock = &blockState{
					kind:      "variable",
					name:      matches[1],
					startLine: lineNum,
					depth:     braceDepth,
				}
				continue
			}

			// output "name" {
			if matches := tfOutputRegex.FindStringSubmatch(trimmed); matches != nil {
				currentBlock = &blockState{
					kind:      "output",
					name:      matches[1],
					startLine: lineNum,
					depth:     braceDepth,
				}
				continue
			}

			// locals {
			if tfLocalsRegex.MatchString(trimmed) {
				inLocals = true
				localsDepth = braceDepth
				localInnerDepth = 0
				continue
			}
		}

		// Process inside a variable or output block
		if currentBlock != nil {
			// Check if the block has closed
			if braceDepth < currentBlock.depth {
				// Block closed — emit symbol
				sym := Symbol{
					Name:       currentBlock.name,
					Type:       "constant",
					Line:       currentBlock.startLine,
					Exported:   true,
					DocComment: currentBlock.desc,
					Language:   "terraform",
				}
				if currentBlock.kind == "variable" {
					sym.Value = p.cleanTFValue(currentBlock.value)
				} else if currentBlock.kind == "output" {
					sym.Value = p.cleanTFValue(currentBlock.value)
				}
				symbols = append(symbols, sym)
				currentBlock = nil
			} else {
				// Extract description
				if matches := tfDescriptionRegex.FindStringSubmatch(trimmed); matches != nil {
					currentBlock.desc = matches[1]
				}
				// Extract default (variable)
				if currentBlock.kind == "variable" {
					if matches := tfDefaultRegex.FindStringSubmatch(trimmed); matches != nil {
						currentBlock.value = strings.TrimSpace(matches[1])
						currentBlock.hasDefault = true
					}
				}
				// Extract value (output)
				if currentBlock.kind == "output" {
					if matches := tfValueRegex.FindStringSubmatch(trimmed); matches != nil {
						currentBlock.value = strings.TrimSpace(matches[1])
					}
				}
			}
			continue
		}

		// Process inside locals block
		if inLocals {
			if braceDepth < localsDepth {
				// Locals block has closed
				inLocals = false
				localInnerDepth = 0
				continue
			}

			// Only extract top-level assignments within locals (depth == localsDepth)
			if localInnerDepth == 0 && braceDepth >= localsDepth {
				if matches := tfAttrRegex.FindStringSubmatch(trimmed); matches != nil {
					key := matches[1]
					rawValue := strings.TrimSpace(matches[2])

					symbols = append(symbols, Symbol{
						Name:     key,
						Type:     "constant",
						Line:     lineNum,
						Exported: true,
						Value:    p.cleanTFValue(rawValue),
						Parent:   "locals",
						Language: "terraform",
					})

					// If the value opens more braces than it closes, track inner depth
					innerOpen := strings.Count(rawValue, "{") + strings.Count(rawValue, "[")
					innerClose := strings.Count(rawValue, "}") + strings.Count(rawValue, "]")
					if innerOpen > innerClose {
						localInnerDepth = innerOpen - innerClose
					}
				}
			} else if localInnerDepth > 0 {
				// Track nested braces/brackets to know when to resume
				innerOpen := strings.Count(trimmed, "{") + strings.Count(trimmed, "[")
				innerClose := strings.Count(trimmed, "}") + strings.Count(trimmed, "]")
				localInnerDepth += innerOpen - innerClose
				if localInnerDepth < 0 {
					localInnerDepth = 0
				}
			}
			continue
		}
	}

	// Flush any pending block at end of file
	if currentBlock != nil {
		sym := Symbol{
			Name:       currentBlock.name,
			Type:       "constant",
			Line:       currentBlock.startLine,
			Exported:   true,
			DocComment: currentBlock.desc,
			Language:   "terraform",
		}
		if currentBlock.kind == "variable" {
			sym.Value = p.cleanTFValue(currentBlock.value)
		} else if currentBlock.kind == "output" {
			sym.Value = p.cleanTFValue(currentBlock.value)
		}
		symbols = append(symbols, sym)
	}

	return symbols
}

// cleanTFValue cleans a Terraform value expression for storage.
func (p *TerraformParser) cleanTFValue(value string) string {
	value = strings.TrimSpace(value)

	// Strip surrounding quotes
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		return value[1 : len(value)-1]
	}

	// Strip trailing braces/brackets from incomplete multi-line values
	value = strings.TrimRight(value, "{[")
	value = strings.TrimSpace(value)

	return value
}
