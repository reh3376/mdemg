package main

import (
	"regexp"
	"strings"
)

// ObsidianNote holds the parsed structure of an Obsidian markdown file.
type ObsidianNote struct {
	Frontmatter map[string]string // YAML frontmatter key-value pairs
	Tags        []string          // #tag and #nested/tag references
	Wikilinks   []Wikilink        // [[page]] and [[page|alias]] references
	Embeds      []string          // ![[file]] embedded references
	Title       string            // First H1 heading or filename
	Content     string            // Full text content (without frontmatter)
}

// Wikilink represents an Obsidian wikilink reference.
type Wikilink struct {
	Target  string // The linked page name (e.g., "My Page" or "folder/page")
	Alias   string // Display text if specified (e.g., in [[page|alias]])
	Section string // Section anchor if specified (e.g., in [[page#section]])
}

var (
	// [[target]] or [[target|alias]] or [[target#section]] or [[target#section|alias]]
	wikilinkRe = regexp.MustCompile(`\[\[([^\]|#]+)(?:#([^\]|]+))?(?:\|([^\]]+))?\]\]`)

	// ![[embedded file]]
	embedRe = regexp.MustCompile(`!\[\[([^\]]+)\]\]`)

	// #tag or #nested/tag (not inside code blocks, not hex colors)
	tagRe = regexp.MustCompile(`(?:^|\s)#([a-zA-Z][a-zA-Z0-9_/-]*)`)

	// YAML frontmatter between --- delimiters
	frontmatterRe = regexp.MustCompile(`(?s)\A---\n(.+?)\n---\n`)
)

// ParseNote parses an Obsidian markdown file into structured data.
func ParseNote(content, filename string) ObsidianNote {
	note := ObsidianNote{
		Frontmatter: make(map[string]string),
	}

	body := content

	// Extract YAML frontmatter
	if match := frontmatterRe.FindStringSubmatch(content); len(match) > 1 {
		parseFrontmatter(match[1], note.Frontmatter)
		body = content[len(match[0]):]
	}
	note.Content = body

	// Extract tags from frontmatter
	if fmTags, ok := note.Frontmatter["tags"]; ok {
		for _, t := range splitYAMLList(fmTags) {
			t = strings.TrimSpace(t)
			if t != "" {
				note.Tags = append(note.Tags, t)
			}
		}
	}

	// Extract inline tags from body
	for _, match := range tagRe.FindAllStringSubmatch(body, -1) {
		if len(match) > 1 {
			tag := match[1]
			if !containsStr(note.Tags, tag) {
				note.Tags = append(note.Tags, tag)
			}
		}
	}

	// Extract embeds first (before wikilinks, since ![[x]] contains [[x]])
	for _, match := range embedRe.FindAllStringSubmatch(body, -1) {
		if len(match) > 1 {
			note.Embeds = append(note.Embeds, match[1])
		}
	}

	// Extract wikilinks (exclude embeds by stripping ![[...]] first)
	bodyNoEmbeds := embedRe.ReplaceAllString(body, "")
	for _, match := range wikilinkRe.FindAllStringSubmatch(bodyNoEmbeds, -1) {
		wl := Wikilink{Target: strings.TrimSpace(match[1])}
		if len(match) > 2 && match[2] != "" {
			wl.Section = strings.TrimSpace(match[2])
		}
		if len(match) > 3 && match[3] != "" {
			wl.Alias = strings.TrimSpace(match[3])
		}
		note.Wikilinks = append(note.Wikilinks, wl)
	}

	// Extract title: first H1 heading, or frontmatter title, or filename
	if t, ok := note.Frontmatter["title"]; ok && t != "" {
		note.Title = t
	} else if idx := strings.Index(body, "\n# "); idx >= 0 {
		end := strings.Index(body[idx+3:], "\n")
		if end >= 0 {
			note.Title = strings.TrimSpace(body[idx+3 : idx+3+end])
		} else {
			note.Title = strings.TrimSpace(body[idx+3:])
		}
	} else if strings.HasPrefix(body, "# ") {
		end := strings.Index(body[2:], "\n")
		if end >= 0 {
			note.Title = strings.TrimSpace(body[2 : 2+end])
		} else {
			note.Title = strings.TrimSpace(body[2:])
		}
	} else {
		// Use filename without extension
		note.Title = strings.TrimSuffix(filename, ".md")
		note.Title = strings.TrimSuffix(note.Title, ".markdown")
	}

	return note
}

// parseFrontmatter does simple YAML key: value parsing (single-level only).
func parseFrontmatter(raw string, out map[string]string) {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		// Strip surrounding quotes
		val = strings.Trim(val, "\"'")
		if key != "" {
			out[key] = val
		}
	}
}

// splitYAMLList handles both inline [a, b, c] and multi-line YAML list values.
func splitYAMLList(s string) []string {
	s = strings.TrimSpace(s)
	// Inline list: [tag1, tag2]
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		s = s[1 : len(s)-1]
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.TrimPrefix(p, "- ")
		p = strings.Trim(p, "\"' ")
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
