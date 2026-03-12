package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FrameworkDef defines a UxTS framework's spec location and hashing.
type FrameworkDef struct {
	Name      string
	SpecDir   string
	Glob      string
	HashField []string // path to hash field, e.g. ["config", "sha256"]
}

// SpecEntry represents a single parsed UxTS spec.
type SpecEntry struct {
	Framework  string
	FilePath   string
	FileName   string
	Tags       []string
	HTTPPath   string            // UATS: request.path
	Service    string            // UDTS: service
	Method     string            // UDTS: method
	Compliance string            // "pass", "fail", "unknown"
	DriftState string            // "clean" or "drift_detected"
	RawSpec    map[string]any    // Full spec for hash computation
	Metadata   map[string]string // Extracted metadata
}

// SpecIndex holds all parsed specs with lookup maps.
type SpecIndex struct {
	mu         sync.RWMutex
	entries    []*SpecEntry
	byPath     map[string][]*SpecEntry // HTTP path → specs (UATS)
	byService  map[string][]*SpecEntry // service.method → specs (UDTS)
	byTag      map[string][]*SpecEntry // tag → specs (all)
	byFramework map[string][]*SpecEntry
	frameworks []string
	driftCount int
}

var frameworkDefs = map[string]FrameworkDef{
	"uats": {Name: "uats", SpecDir: "api/api-spec/uats/specs", Glob: "*.uats.json", HashField: []string{"config", "sha256"}},
	"udts": {Name: "udts", SpecDir: "api/api-spec/udts/specs", Glob: "*.udts.json", HashField: []string{"config", "proto_sha256"}},
	"upts": {Name: "upts", SpecDir: "lang-parser/lang-parse-spec/upts/specs", Glob: "*.upts.json", HashField: []string{"fixture", "sha256"}},
	"ubts": {Name: "ubts", SpecDir: "tests/ubts/specs", Glob: "*.ubts.json", HashField: []string{"config", "sha256"}},
	"usts": {Name: "usts", SpecDir: "tests/usts/specs", Glob: "*.usts.json", HashField: []string{"config", "sha256"}},
	"uams": {Name: "uams", SpecDir: "tests/uams/specs", Glob: "*.uams.json", HashField: []string{"config", "sha256"}},
	"uobs": {Name: "uobs", SpecDir: "tests/uobs/specs", Glob: "*.uobs.json", HashField: []string{"config", "sha256"}},
	"uots": {Name: "uots", SpecDir: "api/api-spec/uots/specs", Glob: "*.uots.json", HashField: []string{"config", "sha256"}},
	"uvts": {Name: "uvts", SpecDir: "tests/uvts/specs", Glob: "*.uvts.json", HashField: []string{"config", "sha256"}},
	"uets": {Name: "uets", SpecDir: "tests/uets/specs", Glob: "*.uets.json", HashField: []string{"fixture", "sha256"}},
}

func NewSpecIndex() *SpecIndex {
	return &SpecIndex{
		byPath:      make(map[string][]*SpecEntry),
		byService:   make(map[string][]*SpecEntry),
		byTag:       make(map[string][]*SpecEntry),
		byFramework: make(map[string][]*SpecEntry),
	}
}

// Load reads specs from disk for the given frameworks (comma-separated).
func (idx *SpecIndex) Load(specRoot, frameworks string, enableDrift bool) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.entries = nil
	idx.byPath = make(map[string][]*SpecEntry)
	idx.byService = make(map[string][]*SpecEntry)
	idx.byTag = make(map[string][]*SpecEntry)
	idx.byFramework = make(map[string][]*SpecEntry)
	idx.driftCount = 0

	fwList := strings.Split(frameworks, ",")
	idx.frameworks = fwList

	for _, fw := range fwList {
		fw = strings.TrimSpace(fw)
		def, ok := frameworkDefs[fw]
		if !ok {
			log.Printf("%s: unknown framework: %s", moduleID, fw)
			continue
		}

		specDir := filepath.Join(specRoot, def.SpecDir)
		pattern := filepath.Join(specDir, def.Glob)

		files, err := filepath.Glob(pattern)
		if err != nil {
			log.Printf("%s: glob error for %s: %v", moduleID, fw, err)
			continue
		}

		for _, f := range files {
			entry, err := parseSpecFile(f, fw, def, enableDrift)
			if err != nil {
				log.Printf("%s: parse error %s: %v", moduleID, f, err)
				continue
			}
			idx.entries = append(idx.entries, entry)
			idx.byFramework[fw] = append(idx.byFramework[fw], entry)

			// Index by HTTP path (UATS)
			if entry.HTTPPath != "" {
				idx.byPath[entry.HTTPPath] = append(idx.byPath[entry.HTTPPath], entry)
			}

			// Index by service.method (UDTS)
			if entry.Service != "" && entry.Method != "" {
				key := entry.Service + "." + entry.Method
				idx.byService[key] = append(idx.byService[key], entry)
			}

			// Index by tags
			for _, tag := range entry.Tags {
				idx.byTag[tag] = append(idx.byTag[tag], entry)
			}

			if entry.DriftState == "drift_detected" {
				idx.driftCount++
			}
		}
	}

	return nil
}

func parseSpecFile(path, framework string, def FrameworkDef, enableDrift bool) (*SpecEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	entry := &SpecEntry{
		Framework:  framework,
		FilePath:   path,
		FileName:   filepath.Base(path),
		Compliance: "unknown",
		DriftState: "clean",
		RawSpec:    raw,
		Metadata:   make(map[string]string),
	}

	// Extract tags from metadata
	if meta, ok := raw["metadata"].(map[string]any); ok {
		if tags, ok := meta["tags"].([]any); ok {
			for _, t := range tags {
				if s, ok := t.(string); ok {
					entry.Tags = append(entry.Tags, s)
				}
			}
		}
		if desc, ok := meta["description"].(string); ok {
			entry.Metadata["description"] = desc
		}
	}

	// Extract config tags
	if cfg, ok := raw["config"].(map[string]any); ok {
		if tags, ok := cfg["tags"].([]any); ok {
			for _, t := range tags {
				if s, ok := t.(string); ok {
					entry.Tags = append(entry.Tags, s)
				}
			}
		}
	}

	// Framework-specific extraction
	switch framework {
	case "uats":
		extractUATSFields(raw, entry)
	case "udts":
		extractUDTSFields(raw, entry)
	}

	// Drift detection
	if enableDrift {
		entry.DriftState = checkDrift(raw, def.HashField)
	}

	return entry, nil
}

func extractUATSFields(raw map[string]any, entry *SpecEntry) {
	if req, ok := raw["request"].(map[string]any); ok {
		if path, ok := req["path"].(string); ok {
			// Strip template variables for indexing: /v1/memory/nodes/{{id}} → /v1/memory/nodes/
			cleaned := stripTemplateVars(path)
			entry.HTTPPath = cleaned
		}
	}
	if api, ok := raw["api"].(map[string]any); ok {
		if tags, ok := api["tags"].([]any); ok {
			for _, t := range tags {
				if s, ok := t.(string); ok {
					entry.Tags = append(entry.Tags, s)
				}
			}
		}
	}
}

func extractUDTSFields(raw map[string]any, entry *SpecEntry) {
	if svc, ok := raw["service"].(string); ok {
		entry.Service = svc
	}
	if method, ok := raw["method"].(string); ok {
		entry.Method = method
	}
}

func stripTemplateVars(path string) string {
	// Replace {{...}} with empty string for indexing
	result := path
	for {
		start := strings.Index(result, "{{")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "}}")
		if end == -1 {
			break
		}
		result = result[:start] + result[start+end+2:]
	}
	// Collapse double slashes from template removal
	for strings.Contains(result, "//") {
		result = strings.ReplaceAll(result, "//", "/")
	}
	// Remove trailing slashes from stripping
	result = strings.TrimRight(result, "/")
	if result == "" {
		result = "/"
	}
	return result
}

// Lookup methods

func (idx *SpecIndex) FindByPath(path string) []*SpecEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	// Try exact match first
	if entries, ok := idx.byPath[path]; ok {
		return entries
	}

	// Try prefix match (strip path params)
	cleaned := stripTemplateVars(path)
	if entries, ok := idx.byPath[cleaned]; ok {
		return entries
	}

	// Try partial path matching
	var matches []*SpecEntry
	for indexedPath, entries := range idx.byPath {
		if strings.HasPrefix(path, indexedPath) || strings.HasPrefix(indexedPath, path) {
			matches = append(matches, entries...)
		}
	}
	return matches
}

func (idx *SpecIndex) FindByService(service, method string) []*SpecEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	key := service + "." + method
	return idx.byService[key]
}

func (idx *SpecIndex) FindByTag(tag string) []*SpecEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.byTag[tag]
}

func (idx *SpecIndex) FindByText(text string) []*SpecEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	textLower := strings.ToLower(text)
	var matches []*SpecEntry
	seen := make(map[string]bool)

	for _, entry := range idx.entries {
		if seen[entry.FilePath] {
			continue
		}
		// Match on filename, tags, description, path, service
		if strings.Contains(strings.ToLower(entry.FileName), textLower) ||
			strings.Contains(strings.ToLower(entry.HTTPPath), textLower) ||
			strings.Contains(strings.ToLower(entry.Service), textLower) ||
			strings.Contains(strings.ToLower(entry.Metadata["description"]), textLower) {
			matches = append(matches, entry)
			seen[entry.FilePath] = true
			continue
		}
		for _, tag := range entry.Tags {
			if strings.Contains(strings.ToLower(tag), textLower) {
				matches = append(matches, entry)
				seen[entry.FilePath] = true
				break
			}
		}
	}
	return matches
}

func (idx *SpecIndex) TotalSpecs() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.entries)
}

func (idx *SpecIndex) FrameworkCount() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	count := 0
	for _, fw := range idx.frameworks {
		if len(idx.byFramework[fw]) > 0 {
			count++
		}
	}
	return count
}

func (idx *SpecIndex) DriftCount() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.driftCount
}

// FrameworkStats returns spec counts per framework.
func (idx *SpecIndex) FrameworkStats() map[string]int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	stats := make(map[string]int)
	for fw, entries := range idx.byFramework {
		stats[fw] = len(entries)
	}
	return stats
}
