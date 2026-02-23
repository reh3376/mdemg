package languages

import (
	"path/filepath"
	"strings"
)

func init() {
	Register(&ScraperMarkdownParser{})
}

// ScraperMarkdownParser implements LanguageParser for web-scraped markdown content.
// It delegates symbol extraction to MarkdownParser.ExtractSymbols(), validating
// the exact code path used by internal/scraper.Parser via UPTS.
type ScraperMarkdownParser struct {
	md MarkdownParser
}

func (p *ScraperMarkdownParser) Name() string {
	return "scraper-markdown"
}

func (p *ScraperMarkdownParser) Extensions() []string {
	return []string{".scraped.md"}
}

func (p *ScraperMarkdownParser) CanParse(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".scraped.md")
}

func (p *ScraperMarkdownParser) IsTestFile(path string) bool {
	return false
}

func (p *ScraperMarkdownParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	name := filepath.Base(path)
	contentStr := TruncateContent(content, 8000)

	tags := []string{"documentation", "web-scraper", "scraped-markdown"}
	concerns := DetectConcerns(relPath, content)
	tags = append(tags, concerns...)

	var symbols []Symbol
	if extractSymbols {
		symbols = p.md.ExtractSymbols(content)
	}

	element := CodeElement{
		Name:        name,
		Kind:        "documentation",
		Path:        "/" + relPath,
		Content:     contentStr,
		Package:     "web",
		FilePath:    relPath,
		Tags:        tags,
		Concerns:    concerns,
		Symbols:     symbols,
		ElementKind: "file",
	}

	return []CodeElement{element}, nil
}
