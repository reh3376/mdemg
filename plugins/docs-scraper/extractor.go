package main

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Extractor converts raw HTML into structured content.
type Extractor struct{}

// ExtractedContent holds the result of HTML extraction.
type ExtractedContent struct {
	Title       string
	Content     string
	ContentHash string
	WordCount   int
	Links       []string
}

// NewExtractor creates a new HTML extractor.
func NewExtractor() *Extractor {
	return &Extractor{}
}

// Extract parses HTML and returns structured content based on the extraction profile.
func (e *Extractor) Extract(body []byte, baseURL, profile string) (*ExtractedContent, error) {
	html := string(body)

	title := extractTitle(html)
	var content string

	switch profile {
	case "documentation":
		content = extractDocumentation(html)
	default:
		content = extractGeneric(html)
	}

	// Clean up whitespace
	content = cleanWhitespace(content)

	// Compute hash
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

	// Count words
	wordCount := len(strings.Fields(content))

	// Extract links
	links := extractLinks(html, baseURL)

	return &ExtractedContent{
		Title:       title,
		Content:     content,
		ContentHash: hash,
		WordCount:   wordCount,
		Links:       links,
	}, nil
}

// extractTitle extracts the <title> tag content.
func extractTitle(html string) string {
	re := regexp.MustCompile(`(?i)<title[^>]*>(.*?)</title>`)
	m := re.FindStringSubmatch(html)
	if len(m) > 1 {
		return decodeHTMLEntities(strings.TrimSpace(stripHTML(m[1])))
	}
	// Fallback to h1
	re = regexp.MustCompile(`(?i)<h1[^>]*>(.*?)</h1>`)
	m = re.FindStringSubmatch(html)
	if len(m) > 1 {
		return decodeHTMLEntities(strings.TrimSpace(stripHTML(m[1])))
	}
	return "Untitled"
}

// extractDocumentation removes nav/footer/sidebar and focuses on main content.
func extractDocumentation(html string) string {
	// Remove script and style tags
	html = removeTagsContent(html, "script")
	html = removeTagsContent(html, "style")
	html = removeTagsContent(html, "nav")
	html = removeTagsContent(html, "footer")
	html = removeTagsContent(html, "header")

	// Remove common non-content elements by class/id
	html = removeByAttr(html, "sidebar")
	html = removeByAttr(html, "nav")
	html = removeByAttr(html, "menu")
	html = removeByAttr(html, "breadcrumb")
	html = removeByAttr(html, "cookie")
	html = removeByAttr(html, "banner")

	// Try to extract main content area
	mainContent := extractTagContent(html, "main")
	if mainContent == "" {
		mainContent = extractTagContent(html, "article")
	}
	if mainContent == "" {
		// Fallback: extract <body> content
		mainContent = extractTagContent(html, "body")
	}
	if mainContent == "" {
		mainContent = html
	}

	return htmlToMarkdown(mainContent)
}

// extractGeneric does minimal filtering on HTML.
func extractGeneric(html string) string {
	// Remove script and style
	html = removeTagsContent(html, "script")
	html = removeTagsContent(html, "style")

	bodyContent := extractTagContent(html, "body")
	if bodyContent == "" {
		bodyContent = html
	}

	return htmlToMarkdown(bodyContent)
}

// htmlToMarkdown converts HTML to a simple markdown representation.
func htmlToMarkdown(html string) string {
	var sb strings.Builder

	// Convert headings
	for i := 6; i >= 1; i-- {
		tag := fmt.Sprintf("h%d", i)
		prefix := strings.Repeat("#", i)
		re := regexp.MustCompile(fmt.Sprintf(`(?i)<%s[^>]*>(.*?)</%s>`, tag, tag))
		html = re.ReplaceAllStringFunc(html, func(match string) string {
			m := re.FindStringSubmatch(match)
			if len(m) > 1 {
				text := strings.TrimSpace(stripHTML(m[1]))
				return fmt.Sprintf("\n%s %s\n", prefix, text)
			}
			return match
		})
	}

	// Convert code blocks
	preRe := regexp.MustCompile(`(?is)<pre[^>]*><code[^>]*(?:class="[^"]*language-(\w+)[^"]*")?[^>]*>(.*?)</code></pre>`)
	html = preRe.ReplaceAllStringFunc(html, func(match string) string {
		m := preRe.FindStringSubmatch(match)
		lang := ""
		content := match
		if len(m) > 2 {
			lang = m[1]
			content = m[2]
		} else if len(m) > 1 {
			content = m[1]
		}
		content = decodeHTMLEntities(strings.TrimSpace(stripHTML(content)))
		return fmt.Sprintf("\n```%s\n%s\n```\n", lang, content)
	})

	// Convert inline code
	codeRe := regexp.MustCompile(`(?i)<code[^>]*>(.*?)</code>`)
	html = codeRe.ReplaceAllString(html, "`$1`")

	// Convert links
	linkRe := regexp.MustCompile(`(?i)<a[^>]*href="([^"]*)"[^>]*>(.*?)</a>`)
	html = linkRe.ReplaceAllString(html, "[$2]($1)")

	// Convert lists
	html = regexp.MustCompile(`(?i)<li[^>]*>`).ReplaceAllString(html, "\n- ")
	html = regexp.MustCompile(`(?i)</li>`).ReplaceAllString(html, "")

	// Convert paragraphs and breaks
	html = regexp.MustCompile(`(?i)<p[^>]*>`).ReplaceAllString(html, "\n\n")
	html = regexp.MustCompile(`(?i)</p>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?i)<br\s*/?\s*>`).ReplaceAllString(html, "\n")

	// Convert bold/italic
	html = regexp.MustCompile(`(?i)<(?:strong|b)[^>]*>(.*?)</(?:strong|b)>`).ReplaceAllString(html, "**$1**")
	html = regexp.MustCompile(`(?i)<(?:em|i)[^>]*>(.*?)</(?:em|i)>`).ReplaceAllString(html, "*$1*")

	// Strip remaining HTML tags
	result := stripHTML(html)

	sb.WriteString(result)
	return sb.String()
}

// --- helper functions ---

func stripHTML(s string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	return re.ReplaceAllString(s, "")
}

func removeTagsContent(html, tag string) string {
	re := regexp.MustCompile(fmt.Sprintf(`(?is)<%s[^>]*>.*?</%s>`, tag, tag))
	return re.ReplaceAllString(html, "")
}

func removeByAttr(html, keyword string) string {
	re := regexp.MustCompile(fmt.Sprintf(`(?is)<\w+[^>]*(?:class|id)="[^"]*%s[^"]*"[^>]*>.*?</\w+>`, keyword))
	return re.ReplaceAllString(html, "")
}

func extractTagContent(html, tag string) string {
	re := regexp.MustCompile(fmt.Sprintf(`(?is)<%s[^>]*>(.*)</%s>`, tag, tag))
	m := re.FindStringSubmatch(html)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

func extractLinks(html, baseURL string) []string {
	re := regexp.MustCompile(`(?i)<a[^>]*href="([^"]*)"`)
	matches := re.FindAllStringSubmatch(html, -1)

	seen := make(map[string]bool)
	var links []string

	base, _ := url.Parse(baseURL)

	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		href := strings.TrimSpace(m[1])
		// SEC-TRANCHE-3: also reject data: and vbscript: schemes
		// (XSS-adjacent). Normalize to lowercase before prefix-checking so
		// `JavaScript:` etc. cannot slip past.
		lowerHref := strings.ToLower(href)
		if href == "" || strings.HasPrefix(href, "#") ||
			strings.HasPrefix(lowerHref, "javascript:") ||
			strings.HasPrefix(lowerHref, "data:") ||
			strings.HasPrefix(lowerHref, "vbscript:") {
			continue
		}

		// Resolve relative URLs
		if base != nil {
			if parsed, err := url.Parse(href); err == nil {
				href = base.ResolveReference(parsed).String()
			}
		}

		if !seen[href] {
			seen[href] = true
			links = append(links, href)
		}
	}
	return links
}

func cleanWhitespace(s string) string {
	// Replace multiple blank lines with two
	re := regexp.MustCompile(`\n{3,}`)
	s = re.ReplaceAllString(s, "\n\n")
	// Trim leading/trailing whitespace from lines
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func decodeHTMLEntities(s string) string {
	replacer := strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&#39;", "'",
		"&apos;", "'",
		"&#x27;", "'",
		"&nbsp;", " ",
	)
	return replacer.Replace(s)
}
