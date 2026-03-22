package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFlattenYAML_IngestSpeed(t *testing.T) {
	cfg := YAMLConfig{
		Ingest: IngestYAML{Speed: "fast"},
	}

	m := flattenYAML(cfg)

	got, ok := m["ingest.speed"]
	if !ok {
		t.Fatal("flattenYAML did not produce key \"ingest.speed\"")
	}
	if got != "fast" {
		t.Errorf("ingest.speed = %q, want %q", got, "fast")
	}
}

func TestValidateConfigFile_IngestSpeedValid(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := "ingest:\n  speed: fast\nneo4j:\n  uri: bolt://localhost:7687\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	errs := ValidateConfigFile(cfgPath)

	for _, e := range errs {
		if e.Field == "ingest.speed" && e.Level == "error" {
			t.Errorf("unexpected error for ingest.speed: %s", e.Message)
		}
	}
}

func TestValidateConfigFile_IngestSpeedInvalid(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := "ingest:\n  speed: turbo\nneo4j:\n  uri: bolt://localhost:7687\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	errs := ValidateConfigFile(cfgPath)

	found := false
	for _, e := range errs {
		if e.Field == "ingest.speed" && e.Level == "error" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a validation error with Field=\"ingest.speed\" and Level=\"error\", but none was found")
	}
}
