package main

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// ObsidianNote holds the parsed structure of an Obsidian markdown file.
type ObsidianNote struct {
	Frontmatter map[string]interface{} // YAML frontmatter key-value pairs
	Tags        []string               // #tag and #nested/tag references
	Aliases     []string               // extracted from frontmatter aliases key
	Wikilinks   []Wikilink             // [[page]] and [[page|alias]] references
	Embeds      []string               // ![[file]] embedded references
	Title       string                 // First H1 heading or filename
	Content     string                 // Full text content (without frontmatter)
	CreatedAt   string                 // extracted from frontmatter created or date key
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
		Frontmatter: make(map[string]interface{}),
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
		switch v := fmTags.(type) {
		case []interface{}:
			for _, item := range v {
				t := strings.TrimSpace(fmt.Sprintf("%v", item))
				if t != "" {
					note.Tags = append(note.Tags, t)
				}
			}
		case string:
			for _, t := range splitYAMLList(v) {
				t = strings.TrimSpace(t)
				if t != "" {
					note.Tags = append(note.Tags, t)
				}
			}
		}
	}

	// Extract aliases from frontmatter
	if fmAliases, ok := note.Frontmatter["aliases"]; ok {
		switch v := fmAliases.(type) {
		case []interface{}:
			for _, item := range v {
				a := strings.TrimSpace(fmt.Sprintf("%v", item))
				if a != "" {
					note.Aliases = append(note.Aliases, a)
				}
			}
		case string:
			for _, a := range splitYAMLList(v) {
				a = strings.TrimSpace(a)
				if a != "" {
					note.Aliases = append(note.Aliases, a)
				}
			}
		}
	}

	// Extract created date from frontmatter
	if created, ok := note.Frontmatter["created"]; ok {
		note.CreatedAt = fmt.Sprintf("%v", created)
	} else if date, ok := note.Frontmatter["date"]; ok {
		note.CreatedAt = fmt.Sprintf("%v", date)
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
	if t, ok := note.Frontmatter["title"]; ok {
		title := fmt.Sprintf("%v", t)
		if title != "" {
			note.Title = title
		}
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

// parseFrontmatter parses YAML frontmatter using yaml.v3 with graceful fallback.
func parseFrontmatter(raw string, out map[string]interface{}) {
	if err := yaml.Unmarshal([]byte(raw), &out); err != nil {
		// Graceful fallback: try simple key:value parsing
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
			val = strings.Trim(val, "\"'")
			if key != "" {
				out[key] = val
			}
		}
	}
}

// FrontmatterStrings flattens the interface map for proto metadata.
func (n ObsidianNote) FrontmatterStrings() map[string]string {
	result := make(map[string]string, len(n.Frontmatter))
	for k, v := range n.Frontmatter {
		switch val := v.(type) {
		case string:
			result[k] = val
		case []interface{}:
			parts := make([]string, 0, len(val))
			for _, item := range val {
				parts = append(parts, fmt.Sprintf("%v", item))
			}
			result[k] = strings.Join(parts, ", ")
		default:
			result[k] = fmt.Sprintf("%v", val)
		}
	}
	return result
}

// CleanContent strips wikilink syntax for embeddings.
func (n ObsidianNote) CleanContent() string {
	result := n.Content
	// Replace ![[embed]] -> embed
	result = embedRe.ReplaceAllString(result, "$1")
	// Replace [[Page|Display]] -> Display, [[Page]] -> Page
	result = wikilinkRe.ReplaceAllStringFunc(result, func(match string) string {
		parts := wikilinkRe.FindStringSubmatch(match)
		if len(parts) > 3 && parts[3] != "" {
			return parts[3] // Use alias/display text
		}
		return parts[1] // Use target name
	})
	return result
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
