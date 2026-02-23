package scraper

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"mdemg/internal/languages"
)

// ParserConfig controls section chunking thresholds.
type ParserConfig struct {
	MinWordCount      int  // Pages below this word count are not chunked (default 2000)
	MinSectionWords   int  // Sections below this are merged with the next (default 100)
	IncludePageContext bool // Prefix sections with "# PageTitle > SectionTitle" (default true)
}

// DefaultParserConfig returns sensible defaults.
func DefaultParserConfig() ParserConfig {
	return ParserConfig{
		MinWordCount:      2000,
		MinSectionWords:   100,
		IncludePageContext: true,
	}
}

// ParsedSection represents one chunk of a parsed page.
type ParsedSection struct {
	Content      string            // Section content with optional context prefix
	Title        string            // Section heading
	WordCount    int
	Tags         []string          // Section-specific tags
	Symbols      []ParsedSymbol    // Extracted symbols
	SectionIndex int               // Position in document (0-based)
}

// ParsedSymbol represents an extracted symbol from a section.
type ParsedSymbol struct {
	Name   string // Heading text, code language, link text
	Type   string // "heading", "code_block", "link", "section"
	Line   int    // Line number within the section
	Value  string // URL for links, language for code blocks, "hN" for headings
	Parent string // Parent heading for hierarchy
}

// ParseResult is the output of parsing a page.
type ParseResult struct {
	Sections       []ParsedSection
	TotalWordCount int
	WasChunked     bool
}

// Parser chunks scraped web content into focused sections and extracts symbols
// using the UPTS-validated MarkdownParser from cmd/ingest-codebase/languages.
type Parser struct {
	config ParserConfig
	mdParser *languages.MarkdownParser
}

// NewParser creates a new scraper parser.
func NewParser(cfg ParserConfig) *Parser {
	return &Parser{
		config:   cfg,
		mdParser: &languages.MarkdownParser{},
	}
}

// Regex patterns — same as cmd/ingest-codebase/languages/markdown_parser.go
var (
	headingPattern   = regexp.MustCompile(`(?m)^(#{1,6})\s+(.+)$`)
	codeBlockPattern = regexp.MustCompile("(?m)^```(\\w*)\\s*$")
)

// Parse chunks content into sections and extracts symbols via the UPTS markdown parser.
func (p *Parser) Parse(title, content, rawURL string, baseTags []string, qualityScore float64) *ParseResult {
	totalWords := countWords(content)

	result := &ParseResult{
		TotalWordCount: totalWords,
	}

	// Small pages pass through unchanged
	if totalWords < p.config.MinWordCount {
		symbols := p.extractSymbolsViaUPTS(content)
		tags := p.buildTags(content, rawURL, baseTags)

		sec := ParsedSection{
			Content:      p.buildContent(title, "", content),
			Title:        title,
			WordCount:    totalWords,
			Tags:         tags,
			Symbols:      symbols,
			SectionIndex: 0,
		}
		result.Sections = []ParsedSection{sec}
		result.WasChunked = false
		return result
	}

	// Large pages: chunk by headings
	chunks := p.chunkByHeadings(content)
	chunks = p.mergeTinySections(chunks)

	result.WasChunked = true
	for i, chunk := range chunks {
		symbols := p.extractSymbolsViaUPTS(chunk.content)
		tags := p.buildTags(chunk.content, rawURL, baseTags)

		sec := ParsedSection{
			Content:      p.buildContent(title, chunk.heading, chunk.content),
			Title:        chunk.heading,
			WordCount:    countWords(chunk.content),
			Tags:         tags,
			Symbols:      symbols,
			SectionIndex: i,
		}
		result.Sections = append(result.Sections, sec)
	}

	return result
}

// rawChunk is an intermediate representation during chunking.
type rawChunk struct {
	heading string
	content string
}

// chunkByHeadings splits content at ## (level-2+) headings.
func (p *Parser) chunkByHeadings(content string) []rawChunk {
	lines := strings.Split(content, "\n")
	var chunks []rawChunk
	var currentLines []string
	currentHeading := ""

	for _, line := range lines {
		if matches := headingPattern.FindStringSubmatch(line); matches != nil {
			level := len(matches[1])
			if level >= 2 {
				// Flush current chunk
				if len(currentLines) > 0 || currentHeading != "" {
					chunks = append(chunks, rawChunk{
						heading: currentHeading,
						content: strings.TrimSpace(strings.Join(currentLines, "\n")),
					})
				}
				currentHeading = strings.TrimSpace(matches[2])
				currentLines = nil
				continue
			}
		}
		currentLines = append(currentLines, line)
	}

	// Flush last chunk
	if len(currentLines) > 0 || currentHeading != "" {
		chunks = append(chunks, rawChunk{
			heading: currentHeading,
			content: strings.TrimSpace(strings.Join(currentLines, "\n")),
		})
	}

	return chunks
}

// mergeTinySections combines sections < MinSectionWords with the next section.
func (p *Parser) mergeTinySections(chunks []rawChunk) []rawChunk {
	if len(chunks) <= 1 {
		return chunks
	}

	var merged []rawChunk
	i := 0
	for i < len(chunks) {
		chunk := chunks[i]
		words := countWords(chunk.content)

		// If this chunk is tiny and there's a next one, merge forward
		if words < p.config.MinSectionWords && i+1 < len(chunks) {
			next := chunks[i+1]
			combinedHeading := chunk.heading
			if combinedHeading == "" {
				combinedHeading = next.heading
			} else if next.heading != "" {
				combinedHeading = chunk.heading + " / " + next.heading
			}
			combinedContent := chunk.content
			if combinedContent != "" && next.content != "" {
				combinedContent = chunk.content + "\n\n" + next.content
			} else if next.content != "" {
				combinedContent = next.content
			}
			chunks[i+1] = rawChunk{heading: combinedHeading, content: combinedContent}
			i++
			continue
		}

		merged = append(merged, chunk)
		i++
	}

	return merged
}

// extractSymbolsViaUPTS uses the UPTS-validated MarkdownParser for symbol extraction,
// then converts languages.Symbol → ParsedSymbol.
func (p *Parser) extractSymbolsViaUPTS(content string) []ParsedSymbol {
	uptsSymbols := p.mdParser.ExtractSymbols(content)

	var symbols []ParsedSymbol
	for _, s := range uptsSymbols {
		ps := ParsedSymbol{
			Name:   s.Name,
			Type:   s.Type,
			Line:   s.Line,
			Parent: s.Parent,
		}
		// Map UPTS fields to ParsedSymbol.Value
		switch s.Type {
		case "code_block":
			ps.Value = s.Name // language name
		case "link":
			ps.Value = s.Value // URL
		case "section", "heading":
			ps.Value = s.DocComment // "h1", "##", etc.
		}
		symbols = append(symbols, ps)
	}
	return symbols
}

// buildTags generates section-specific tags from content analysis.
func (p *Parser) buildTags(content, rawURL string, baseTags []string) []string {
	tags := make([]string, len(baseTags))
	copy(tags, baseTags)
	seen := make(map[string]bool)
	for _, t := range tags {
		seen[t] = true
	}

	addTag := func(tag string) {
		if !seen[tag] {
			seen[tag] = true
			tags = append(tags, tag)
		}
	}

	// lang:<language> from code blocks
	langs := extractCodeLanguages(content)
	for _, lang := range langs {
		addTag("lang:" + lang)
	}

	// domain:<host> from URL
	if domain := extractDomain(rawURL); domain != "" {
		addTag("domain:" + domain)
	}

	// topic:<term> from headings
	terms := extractKeyTerms(content)
	for _, term := range terms {
		addTag("topic:" + term)
	}

	return tags
}

// buildContent optionally prefixes section content with page/section context.
func (p *Parser) buildContent(pageTitle, sectionTitle, content string) string {
	if !p.config.IncludePageContext {
		return content
	}
	if pageTitle == "" && sectionTitle == "" {
		return content
	}
	var prefix string
	if sectionTitle != "" && pageTitle != "" {
		prefix = fmt.Sprintf("# %s > %s", pageTitle, sectionTitle)
	} else if pageTitle != "" {
		prefix = fmt.Sprintf("# %s", pageTitle)
	} else {
		prefix = fmt.Sprintf("# %s", sectionTitle)
	}
	return prefix + "\n\n" + content
}

// --- Helpers ---

// countWords counts whitespace-delimited words in text.
func countWords(s string) int {
	return len(strings.Fields(s))
}

// extractDomain extracts the host from a URL string.
func extractDomain(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// extractCodeLanguages finds unique code block languages in markdown content.
func extractCodeLanguages(content string) []string {
	matches := codeBlockPattern.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool)
	var langs []string
	inBlock := false
	for _, m := range matches {
		if !inBlock {
			lang := m[1]
			if lang != "" && !seen[lang] {
				seen[lang] = true
				langs = append(langs, lang)
			}
			inBlock = true
		} else {
			inBlock = false
		}
	}
	return langs
}

// extractKeyTerms extracts lowercased heading words as topic terms,
// filtering out common stop words and short words.
func extractKeyTerms(content string) []string {
	stopWords := map[string]bool{
		"the": true, "and": true, "for": true, "with": true, "from": true,
		"this": true, "that": true, "are": true, "was": true, "has": true,
		"have": true, "will": true, "can": true, "all": true, "not": true,
		"but": true, "how": true, "what": true, "when": true, "who": true,
		"its": true, "into": true, "also": true, "more": true, "about": true,
	}

	matches := headingPattern.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool)
	var terms []string
	for _, m := range matches {
		heading := strings.TrimSpace(m[2])
		words := strings.Fields(heading)
		for _, w := range words {
			term := strings.ToLower(strings.Trim(w, ".,;:!?()[]{}\"'`"))
			if len(term) < 3 || stopWords[term] {
				continue
			}
			if !seen[term] {
				seen[term] = true
				terms = append(terms, term)
			}
		}
	}
	return terms
}
