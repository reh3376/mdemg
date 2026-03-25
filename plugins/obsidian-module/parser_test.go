package main

import (
	"strings"
	"testing"
)

func TestParseNote_Wikilinks(t *testing.T) {
	content := `# My Note

This links to [[Another Page]] and [[Folder/Nested Page]].
Also [[Page With Alias|display text]] and [[Page#Section]].
And [[Full Link#Section|Alias]].
`
	note := ParseNote(content, "my-note.md")

	if len(note.Wikilinks) != 5 {
		t.Fatalf("expected 5 wikilinks, got %d", len(note.Wikilinks))
	}

	tests := []struct {
		idx     int
		target  string
		section string
		alias   string
	}{
		{0, "Another Page", "", ""},
		{1, "Folder/Nested Page", "", ""},
		{2, "Page With Alias", "", "display text"},
		{3, "Page", "Section", ""},
		{4, "Full Link", "Section", "Alias"},
	}

	for _, tc := range tests {
		wl := note.Wikilinks[tc.idx]
		if wl.Target != tc.target {
			t.Errorf("wikilink[%d] target: got %q, want %q", tc.idx, wl.Target, tc.target)
		}
		if wl.Section != tc.section {
			t.Errorf("wikilink[%d] section: got %q, want %q", tc.idx, wl.Section, tc.section)
		}
		if wl.Alias != tc.alias {
			t.Errorf("wikilink[%d] alias: got %q, want %q", tc.idx, wl.Alias, tc.alias)
		}
	}
}

func TestParseNote_Tags(t *testing.T) {
	content := `---
tags: [project, active]
---

# Tagged Note

This has #inline-tag and #nested/tag references.
Also #another_tag here.
`
	note := ParseNote(content, "tagged.md")

	expected := []string{"project", "active", "inline-tag", "nested/tag", "another_tag"}
	if len(note.Tags) != len(expected) {
		t.Fatalf("expected %d tags, got %d: %v", len(expected), len(note.Tags), note.Tags)
	}
	for i, exp := range expected {
		if note.Tags[i] != exp {
			t.Errorf("tag[%d]: got %q, want %q", i, note.Tags[i], exp)
		}
	}
}

func TestParseNote_Frontmatter(t *testing.T) {
	content := `---
title: My Custom Title
author: John Doe
status: draft
aliases: [alias1, alias2]
---

# Heading

Body content here.
`
	note := ParseNote(content, "note.md")

	if note.Title != "My Custom Title" {
		t.Errorf("title: got %q, want %q", note.Title, "My Custom Title")
	}
	if note.Frontmatter["author"] != "John Doe" {
		t.Errorf("author: got %q, want %q", note.Frontmatter["author"], "John Doe")
	}
	if note.Frontmatter["status"] != "draft" {
		t.Errorf("status: got %q, want %q", note.Frontmatter["status"], "draft")
	}
	// Content should not include frontmatter
	if len(note.Content) == 0 {
		t.Error("content should not be empty")
	}
	if note.Content[0] == '-' {
		t.Error("content should not start with frontmatter delimiter")
	}
}

func TestParseNote_Embeds(t *testing.T) {
	content := `# Note with embeds

Check this image: ![[screenshot.png]]
And this note embed: ![[embedded-note]]
Regular link: [[not-an-embed]]
`
	note := ParseNote(content, "embeds.md")

	if len(note.Embeds) != 2 {
		t.Fatalf("expected 2 embeds, got %d: %v", len(note.Embeds), note.Embeds)
	}
	if note.Embeds[0] != "screenshot.png" {
		t.Errorf("embed[0]: got %q, want %q", note.Embeds[0], "screenshot.png")
	}
	if note.Embeds[1] != "embedded-note" {
		t.Errorf("embed[1]: got %q, want %q", note.Embeds[1], "embedded-note")
	}

	// Embeds should not appear as wikilinks
	for _, wl := range note.Wikilinks {
		if wl.Target == "screenshot.png" || wl.Target == "embedded-note" {
			t.Errorf("embed %q should not appear as wikilink", wl.Target)
		}
	}
	// Regular link should still appear
	if len(note.Wikilinks) != 1 || note.Wikilinks[0].Target != "not-an-embed" {
		t.Errorf("expected 1 wikilink (not-an-embed), got %v", note.Wikilinks)
	}
}

func TestParseNote_TitleFallback(t *testing.T) {
	// No frontmatter title, no H1 — should use filename
	content := "Just some text without a heading.\n"
	note := ParseNote(content, "my-file.md")

	if note.Title != "my-file" {
		t.Errorf("title fallback: got %q, want %q", note.Title, "my-file")
	}
}

func TestParseNote_TitleFromH1(t *testing.T) {
	content := "# First Heading\n\nBody text.\n"
	note := ParseNote(content, "file.md")

	if note.Title != "First Heading" {
		t.Errorf("title from H1: got %q, want %q", note.Title, "First Heading")
	}
}

func TestParseNote_NoFrontmatter(t *testing.T) {
	content := "# Simple Note\n\nNo frontmatter here.\n"
	note := ParseNote(content, "simple.md")

	if len(note.Frontmatter) != 0 {
		t.Errorf("expected empty frontmatter, got %v", note.Frontmatter)
	}
	if note.Content != content {
		t.Errorf("content should be unchanged when no frontmatter")
	}
}

func TestParseNote_DuplicateTags(t *testing.T) {
	content := `---
tags: [important]
---

This is #important and also #important again.
`
	note := ParseNote(content, "dupes.md")

	count := 0
	for _, tag := range note.Tags {
		if tag == "important" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("tag 'important' should appear once, appeared %d times in %v", count, note.Tags)
	}
}

func TestParseNote_YAMLFrontmatter(t *testing.T) {
	content := `---
title: Nested YAML Test
author:
  name: Jane Doe
  email: jane@example.com
priority: 1
---

# Content

Body here.
`
	note := ParseNote(content, "nested.md")

	if note.Title != "Nested YAML Test" {
		t.Errorf("title: got %q, want %q", note.Title, "Nested YAML Test")
	}
	// yaml.v3 should parse nested maps as map[string]interface{}
	author, ok := note.Frontmatter["author"]
	if !ok {
		t.Fatal("expected 'author' in frontmatter")
	}
	authorMap, ok := author.(map[string]interface{})
	if !ok {
		t.Fatalf("expected author to be a map, got %T", author)
	}
	if authorMap["name"] != "Jane Doe" {
		t.Errorf("author.name: got %v, want %q", authorMap["name"], "Jane Doe")
	}
	// Numeric value should be parsed
	if note.Frontmatter["priority"] != 1 {
		t.Errorf("priority: got %v, want 1", note.Frontmatter["priority"])
	}
}

func TestParseNote_MalformedFrontmatter(t *testing.T) {
	content := `---
title: Good Title
bad yaml: [unclosed
also: fine
---

Body content.
`
	// Should gracefully handle malformed YAML via fallback
	note := ParseNote(content, "malformed.md")
	// Fallback should still extract simple key:value pairs
	if note.Frontmatter["title"] == nil {
		t.Error("expected title to be extracted even with malformed YAML")
	}
}

func TestParseNote_Aliases(t *testing.T) {
	content := `---
title: Main Title
aliases:
  - Alias One
  - Alias Two
  - Alias Three
---

Content here.
`
	note := ParseNote(content, "aliases.md")

	if len(note.Aliases) != 3 {
		t.Fatalf("expected 3 aliases, got %d: %v", len(note.Aliases), note.Aliases)
	}
	expected := []string{"Alias One", "Alias Two", "Alias Three"}
	for i, exp := range expected {
		if note.Aliases[i] != exp {
			t.Errorf("alias[%d]: got %q, want %q", i, note.Aliases[i], exp)
		}
	}
}

func TestParseNote_CleanContent(t *testing.T) {
	content := `# Test Note

This links to [[Another Page]] and shows [[Page With Alias|display text]].
Also embeds ![[screenshot.png]] inline.
`
	note := ParseNote(content, "clean.md")
	clean := note.CleanContent()

	if strings.Contains(clean, "[[") {
		t.Errorf("CleanContent should strip wikilink brackets, got: %s", clean)
	}
	if !strings.Contains(clean, "Another Page") {
		t.Error("CleanContent should keep target name for simple wikilinks")
	}
	if !strings.Contains(clean, "display text") {
		t.Error("CleanContent should use alias for aliased wikilinks")
	}
	if !strings.Contains(clean, "screenshot.png") {
		t.Error("CleanContent should replace embed with filename")
	}
}

func TestParseNote_CreatedAt(t *testing.T) {
	content := `---
title: Dated Note
created: "2026-01-15"
---

Content.
`
	note := ParseNote(content, "dated.md")
	if note.CreatedAt != "2026-01-15" {
		t.Errorf("CreatedAt: got %q, want %q", note.CreatedAt, "2026-01-15")
	}

	// Also test the "date" fallback — yaml.v3 parses unquoted dates as time.Time
	content2 := `---
date: 2026-02-20
---

Content.
`
	note2 := ParseNote(content2, "dated2.md")
	if !strings.Contains(note2.CreatedAt, "2026-02-20") {
		t.Errorf("CreatedAt from date: got %q, want to contain %q", note2.CreatedAt, "2026-02-20")
	}
}

func TestFrontmatterStrings(t *testing.T) {
	content := `---
title: String Test
tags:
  - alpha
  - beta
priority: 5
---

Content.
`
	note := ParseNote(content, "fmstrings.md")
	fms := note.FrontmatterStrings()

	if fms["title"] != "String Test" {
		t.Errorf("title: got %q, want %q", fms["title"], "String Test")
	}
	if fms["tags"] != "alpha, beta" {
		t.Errorf("tags: got %q, want %q", fms["tags"], "alpha, beta")
	}
	if fms["priority"] != "5" {
		t.Errorf("priority: got %q, want %q", fms["priority"], "5")
	}
}

func TestObsidianNodeID_Deterministic(t *testing.T) {
	id1 := obsidianNodeID("My Note.md")
	id2 := obsidianNodeID("my note.md")
	id3 := obsidianNodeID("My Note")

	if id1 != id2 {
		t.Errorf("node IDs should be case-insensitive: %q != %q", id1, id2)
	}
	if id1 != id3 {
		t.Errorf("node IDs should strip .md: %q != %q", id1, id3)
	}
}
