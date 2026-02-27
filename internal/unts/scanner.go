package unts

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Scanner discovers and registers tracked files from manifest and specs
type Scanner struct {
	registry *Registry
	basePath string
}

// NewScanner creates a scanner for the given registry
func NewScanner(registry *Registry, basePath string) *Scanner {
	return &Scanner{
		registry: registry,
		basePath: basePath,
	}
}

// uxtsHashSource describes where to find SHA256 hashes in a UxTS framework.
type uxtsHashSource struct {
	framework string
	specDir   string
	suffix    string
	// hashPath is a dot-separated JSON path to the SHA256 field.
	// e.g. "config.sha256", "fixture.sha256"
	hashPath string
}

// uxtsFrameworks lists all UxTS frameworks and their hash field locations.
var uxtsFrameworks = []uxtsHashSource{
	{framework: "uats", specDir: "docs/api/api-spec/uats/specs", suffix: ".uats.json", hashPath: "config.sha256"},
	{framework: "upts", specDir: "docs/lang-parser/lang-parse-spec/upts/specs", suffix: ".upts.json", hashPath: "fixture.sha256"},
	{framework: "ubts", specDir: "docs/tests/ubts/specs", suffix: ".ubts.json", hashPath: "config.sha256"},
	{framework: "usts", specDir: "docs/tests/usts/specs", suffix: ".usts.json", hashPath: "config.sha256"},
	{framework: "uots", specDir: "docs/api/api-spec/uots/specs", suffix: ".uots.json", hashPath: "config.sha256"},
	{framework: "uams", specDir: "docs/tests/uams/specs", suffix: ".uams.json", hashPath: "config.sha256"},
	{framework: "uets", specDir: "docs/tests/uets/specs", suffix: ".uets.json", hashPath: "fixture.sha256"},
	{framework: "uvts", specDir: "docs/tests/uvts/specs", suffix: ".uvts.json", hashPath: "config.sha256"},
}

// ScanAll scans manifest, UDTS specs, and all UxTS framework spec hash fields.
func (s *Scanner) ScanAll() error {
	if err := s.ScanManifest(); err != nil {
		return fmt.Errorf("scan manifest: %w", err)
	}
	if err := s.ScanUDTSSpecs(); err != nil {
		return fmt.Errorf("scan udts: %w", err)
	}
	for _, src := range uxtsFrameworks {
		if err := s.ScanUxTSSpecs(src); err != nil {
			return fmt.Errorf("scan %s: %w", src.framework, err)
		}
	}
	return nil
}

// ScanManifest reads docs/specs/manifest.sha256 and registers all entries
func (s *Scanner) ScanManifest() error {
	manifestPath := filepath.Join(s.basePath, "docs/specs/manifest.sha256")
	f, err := os.Open(manifestPath)
	if os.IsNotExist(err) {
		return nil // No manifest is OK
	}
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Format: <sha256>  <filepath>  (two spaces between)
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 {
			continue
		}

		hash := strings.TrimSpace(parts[0])
		path := strings.TrimSpace(parts[1])

		if len(hash) != 64 {
			continue // Invalid hash
		}

		if err := s.registry.Register(path, "manifest", hash, "docs/specs/manifest.sha256", "manifest"); err != nil {
			return fmt.Errorf("register %s: %w", path, err)
		}
	}

	return scanner.Err()
}

// UDTSSpec represents relevant fields from a UDTS spec file
type UDTSSpec struct {
	Service string `json:"service"`
	Config  struct {
		ProtoSHA256 string `json:"proto_sha256"`
	} `json:"config"`
}

// ScanUDTSSpecs reads all docs/api/api-spec/udts/specs/*.udts.json files
func (s *Scanner) ScanUDTSSpecs() error {
	specsDir := filepath.Join(s.basePath, "docs/api/api-spec/udts/specs")
	entries, err := os.ReadDir(specsDir)
	if os.IsNotExist(err) {
		return nil // No specs dir is OK
	}
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".udts.json") {
			continue
		}

		specPath := filepath.Join(specsDir, entry.Name())
		if err := s.scanUDTSSpec(specPath, entry.Name()); err != nil {
			// Log but continue
			fmt.Printf("warning: scan UDTS spec %s: %v\n", entry.Name(), err)
		}
	}

	return nil
}

func (s *Scanner) scanUDTSSpec(specPath, filename string) error {
	data, err := os.ReadFile(specPath)
	if err != nil {
		return err
	}

	var spec UDTSSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return err
	}

	// Skip if no proto_sha256
	if spec.Config.ProtoSHA256 == "" {
		return nil
	}

	// Derive proto path from service name
	// e.g. mdemg.devspace.v1.DevSpace -> api/proto/devspace.proto
	protoPath := deriveProtoPath(spec.Service)
	if protoPath == "" {
		return nil
	}

	return s.registry.Register(protoPath, "udts", spec.Config.ProtoSHA256, filename, "spec")
}

// deriveProtoPath converts a service name to a proto file path
// e.g. mdemg.devspace.v1.DevSpace -> api/proto/devspace.proto
// e.g. mdemg.transfer.v1.SpaceTransfer -> api/proto/space-transfer.proto
func deriveProtoPath(service string) string {
	if service == "" {
		return ""
	}

	// Split into parts: mdemg.devspace.v1.DevSpace
	parts := strings.Split(service, ".")
	if len(parts) < 4 {
		return ""
	}

	// Use the second part as base (devspace, transfer, unts, etc.)
	base := parts[1]

	// Handle special cases
	switch base {
	case "transfer":
		return "api/proto/space-transfer.proto"
	case "devspace":
		return "api/proto/devspace.proto"
	case "unts":
		return "api/proto/unts.proto"
	case "module":
		return "api/proto/mdemg-module.proto"
	default:
		// Generic: base -> api/proto/base.proto
		return fmt.Sprintf("api/proto/%s.proto", base)
	}
}

// ScanUxTSSpecs scans a UxTS framework's spec directory for SHA256 hash fields.
func (s *Scanner) ScanUxTSSpecs(src uxtsHashSource) error {
	specsDir := filepath.Join(s.basePath, src.specDir)
	entries, err := os.ReadDir(specsDir)
	if os.IsNotExist(err) {
		return nil // Missing dir is OK
	}
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), src.suffix) {
			continue
		}

		specPath := filepath.Join(specsDir, entry.Name())
		data, err := os.ReadFile(specPath)
		if err != nil {
			fmt.Printf("warning: scan %s spec %s: %v\n", src.framework, entry.Name(), err)
			continue
		}

		var doc map[string]any
		if err := json.Unmarshal(data, &doc); err != nil {
			fmt.Printf("warning: parse %s spec %s: %v\n", src.framework, entry.Name(), err)
			continue
		}

		hash := extractHashByPath(doc, src.hashPath)
		if hash == "" || hash == "PENDING" {
			continue
		}

		// For spec-integrity hashes, the tracked path is the spec file itself.
		relPath := filepath.Join(src.specDir, entry.Name())
		if err := s.registry.Register(relPath, src.framework, hash, entry.Name(), "spec"); err != nil {
			fmt.Printf("warning: register %s hash from %s: %v\n", src.framework, entry.Name(), err)
		}
	}

	return nil
}

// extractHashByPath extracts a string value from a nested map using a dot-separated path.
// e.g. extractHashByPath(doc, "config.sha256") → doc["config"]["sha256"]
func extractHashByPath(doc map[string]any, path string) string {
	parts := strings.Split(path, ".")
	var current any = doc
	for _, key := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = m[key]
	}
	s, _ := current.(string)
	return s
}

// ScanResults holds statistics from a scan operation
type ScanResults struct {
	ManifestFiles int
	UDTSFiles     int
	UxTSFiles     int
	Errors        []string
}
