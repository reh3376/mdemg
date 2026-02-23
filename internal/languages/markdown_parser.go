package languages

import (
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&MarkdownParser{})
}

// MarkdownParser implements LanguageParser for Markdown documentation files
type MarkdownParser struct{}

func (p *MarkdownParser) Name() string {
	return "markdown"
}

func (p *MarkdownParser) Extensions() []string {
	return []string{".md", ".markdown"}
}

func (p *MarkdownParser) CanParse(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.HasSuffix(pathLower, ".md") || strings.HasSuffix(pathLower, ".markdown")
}

func (p *MarkdownParser) IsTestFile(path string) bool {
	pathLower := strings.ToLower(path)
	return strings.Contains(pathLower, "/fixtures/") ||
		strings.Contains(pathLower, "/testdata/") ||
		strings.Contains(pathLower, "_test.")
}

func (p *MarkdownParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	name := filepath.Base(path)

	// Truncate content if too long
	contentStr := TruncateContent(content, 4000)

	// Detect cross-cutting concerns from docs
	concerns := DetectConcerns(relPath, content)
	tags := []string{"documentation", "markdown"}
	tags = append(tags, concerns...)

	// Check if this is a configuration doc file
	mdKind := "documentation"
	if IsConfigFile(relPath) {
		tags = append(tags, "config")
		mdKind = "config"
	}

	// Extract symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.ExtractSymbols(content)
	}

	element := CodeElement{
		Name:        name,
		Kind:        mdKind,
		Path:        "/" + relPath,
		Content:     contentStr,
		Package:     "docs",
		FilePath:    relPath,
		Tags:        tags,
		Concerns:    concerns,
		Symbols:     symbols,
		ElementKind: "file",
	}

	return []CodeElement{element}, nil
}

// Regex patterns for Markdown parsing
var (
	// Headings: # H1, ## H2, etc.
	mdHeadingPattern = regexp.MustCompile(`(?m)^(#{1,6})\s+(.+)$`)

	// Code blocks: ```language
	mdCodeBlockPattern = regexp.MustCompile("(?m)^```(\\w*)\\s*$")

	// Links: [text](url)
	mdLinkPattern = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

	// Frontmatter: ---\n...\n---
	mdFrontmatterPattern = regexp.MustCompile(`(?s)^---\n(.+?)\n---`)
)

// ExtractSymbols parses markdown content and returns UPTS-validated symbols.
// Exported so that internal/scraper can reuse the same UPTS-validated extraction.
func (p *MarkdownParser) ExtractSymbols(content string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	var inCodeBlock bool
	var codeBlockLang string
	var headingStack []string // For tracking hierarchy

	for lineNum, line := range lines {
		lineNo := lineNum + 1

		// Track code block state
		if matches := mdCodeBlockPattern.FindStringSubmatch(line); matches != nil {
			if !inCodeBlock {
				// Starting a code block
				inCodeBlock = true
				codeBlockLang = matches[1]
				if codeBlockLang != "" {
					symbols = append(symbols, Symbol{
						Name:     codeBlockLang,
						Type:     "code_block",
						Line:     lineNo,
						Exported: true,
						Language: "markdown",
					})
				}
			} else {
				// Ending a code block
				inCodeBlock = false
				codeBlockLang = ""
			}
			continue
		}

		// Skip content inside code blocks
		if inCodeBlock {
			continue
		}

		// Check for headings
		if matches := mdHeadingPattern.FindStringSubmatch(line); matches != nil {
			level := len(matches[1])
			heading := strings.TrimSpace(matches[2])

			// Clean heading text (remove trailing anchors like {#id})
			if idx := strings.Index(heading, " {#"); idx > 0 {
				heading = heading[:idx]
			}

			// Determine parent from heading stack
			parent := ""
			if level > 1 && len(headingStack) > 0 {
				// Find the nearest higher-level heading
				for i := len(headingStack) - 1; i >= 0; i-- {
					parent = headingStack[i]
					break
				}
			}

			// Update heading stack
			// Trim stack to current level
			if level <= len(headingStack) {
				headingStack = headingStack[:level-1]
			}
			headingStack = append(headingStack, heading)

			// Determine symbol type based on level
			symType := "heading"
			if level == 1 {
				symType = "section"
			}

			symbols = append(symbols, Symbol{
				Name:       heading,
				Type:       symType,
				Line:       lineNo,
				Parent:     parent,
				DocComment: strings.Repeat("#", level),
				Exported:   true,
				Language:   "markdown",
			})
			continue
		}

		// Check for links (only extract significant links, not inline ones)
		// We'll extract links that appear at the start of a line or are references
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.Contains(trimmed, "](") {
			if matches := mdLinkPattern.FindStringSubmatch(trimmed); matches != nil {
				linkText := matches[1]
				linkURL := matches[2]

				// Only extract links that look like references (not inline text links)
				if strings.HasPrefix(linkURL, "http") || strings.HasPrefix(linkURL, "/") {
					symbols = append(symbols, Symbol{
						Name:     linkText,
						Type:     "link",
						Line:     lineNo,
						Value:    linkURL,
						Exported: true,
						Language: "markdown",
					})
				}
			}
		}
	}

	return symbols
}
