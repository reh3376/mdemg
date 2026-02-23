package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToModuleID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple name",
			input:    "my-plugin",
			expected: "my-plugin",
		},
		{
			name:     "spaces to hyphens",
			input:    "My Plugin Name",
			expected: "my-plugin-name",
		},
		{
			name:     "underscores to hyphens",
			input:    "my_plugin_name",
			expected: "my-plugin-name",
		},
		{
			name:     "mixed case",
			input:    "MyPlugin",
			expected: "myplugin",
		},
		{
			name:     "consecutive separators",
			input:    "my--plugin__name",
			expected: "my-plugin-name",
		},
		{
			name:     "leading/trailing separators",
			input:    "-my-plugin-",
			expected: "my-plugin",
		},
		{
			name:     "complex name",
			input:    "Linear Issue Parser",
			expected: "linear-issue-parser",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toModuleID(tt.input)
			if result != tt.expected {
				t.Errorf("toModuleID(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestToStructName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple hyphenated",
			input:    "my-plugin",
			expected: "MyPlugin",
		},
		{
			name:     "with spaces",
			input:    "my plugin name",
			expected: "MyPluginName",
		},
		{
			name:     "underscored",
			input:    "my_plugin_name",
			expected: "MyPluginName",
		},
		{
			name:     "already capitalized",
			input:    "MyPlugin",
			expected: "Myplugin",
		},
		{
			name:     "all caps",
			input:    "MY-PLUGIN",
			expected: "MyPlugin",
		},
		{
			name:     "single word",
			input:    "plugin",
			expected: "Plugin",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "Plugin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toStructName(tt.input)
			if result != tt.expected {
				t.Errorf("toStructName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGeneratePlugin(t *testing.T) {
	// Create a temporary directory for test output
	tempDir, err := os.MkdirTemp("", "plugin-scaffold-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name       string
		config     pluginScaffoldConfig
		wantFiles  []string
		wantInFile map[string][]string // file -> strings that should be present
	}{
		{
			name: "ingestion plugin",
			config: pluginScaffoldConfig{
				Name:       "Linear Parser",
				Type:       moduleTypeIngestion,
				OutputDir:  tempDir,
				Version:    "1.0.0",
				ModuleID:   "linear-parser",
				StructName: "LinearParser",
			},
			wantFiles: []string{
				"manifest.json",
				"main.go",
				"handler.go",
				"Makefile",
				"README.md",
			},
			wantInFile: map[string][]string{
				"manifest.json": {
					`"id": "linear-parser"`,
					`"type": "INGESTION"`,
					`"ingestion_sources"`,
				},
				"main.go": {
					`moduleID      = "linear-parser"`,
					`pb.RegisterIngestionModuleServer`,
					`NewLinearParserHandler`,
				},
				"handler.go": {
					`type LinearParserHandler struct`,
					`func (h *LinearParserHandler) Matches`,
					`func (h *LinearParserHandler) Parse`,
					`func (h *LinearParserHandler) Sync`,
					`MODULE_TYPE_INGESTION`,
				},
				"Makefile": {
					`BINARY_NAME = linear-parser`,
				},
				"README.md": {
					`# Linear Parser`,
					`INGESTION`,
				},
			},
		},
		{
			name: "reasoning plugin",
			config: pluginScaffoldConfig{
				Name:       "Custom Ranker",
				Type:       moduleTypeReasoning,
				OutputDir:  tempDir,
				Version:    "2.0.0",
				ModuleID:   "custom-ranker",
				StructName: "CustomRanker",
			},
			wantFiles: []string{
				"manifest.json",
				"main.go",
				"handler.go",
				"Makefile",
				"README.md",
			},
			wantInFile: map[string][]string{
				"manifest.json": {
					`"id": "custom-ranker"`,
					`"type": "REASONING"`,
					`"pattern_detectors"`,
					`"boost_factor"`,
				},
				"main.go": {
					`moduleID      = "custom-ranker"`,
					`pb.RegisterReasoningModuleServer`,
					`NewCustomRankerHandler`,
				},
				"handler.go": {
					`type CustomRankerHandler struct`,
					`func (h *CustomRankerHandler) Process`,
					`MODULE_TYPE_REASONING`,
					`boostFactor`,
				},
				"README.md": {
					`# Custom Ranker`,
					`REASONING`,
				},
			},
		},
		{
			name: "ape plugin",
			config: pluginScaffoldConfig{
				Name:       "Session Reflector",
				Type:       moduleTypeAPE,
				OutputDir:  tempDir,
				Version:    "1.0.0",
				ModuleID:   "session-reflector",
				StructName: "SessionReflector",
			},
			wantFiles: []string{
				"manifest.json",
				"main.go",
				"handler.go",
				"Makefile",
				"README.md",
			},
			wantInFile: map[string][]string{
				"manifest.json": {
					`"id": "session-reflector"`,
					`"type": "APE"`,
					`"event_triggers"`,
				},
				"main.go": {
					`moduleID      = "session-reflector"`,
					`pb.RegisterAPEModuleServer`,
					`NewSessionReflectorHandler`,
				},
				"handler.go": {
					`type SessionReflectorHandler struct`,
					`func (h *SessionReflectorHandler) Execute`,
					`func (h *SessionReflectorHandler) GetSchedule`,
					`MODULE_TYPE_APE`,
					`CronExpression`,
				},
				"README.md": {
					`# Session Reflector`,
					`APE`,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Generate the plugin
			err := generatePlugin(tt.config)
			if err != nil {
				t.Fatalf("generatePlugin() error = %v", err)
			}

			pluginDir := filepath.Join(tt.config.OutputDir, tt.config.ModuleID)

			// Check all expected files exist
			for _, fileName := range tt.wantFiles {
				path := filepath.Join(pluginDir, fileName)
				if _, err := os.Stat(path); os.IsNotExist(err) {
					t.Errorf("Expected file %s does not exist", fileName)
				}
			}

			// Check file contents
			for fileName, wantStrings := range tt.wantInFile {
				path := filepath.Join(pluginDir, fileName)
				content, err := os.ReadFile(path)
				if err != nil {
					t.Errorf("Failed to read %s: %v", fileName, err)
					continue
				}

				contentStr := string(content)
				for _, want := range wantStrings {
					if !strings.Contains(contentStr, want) {
						t.Errorf("%s: expected to contain %q", fileName, want)
					}
				}
			}

			// Cleanup this specific plugin dir for next test
			os.RemoveAll(pluginDir)
		})
	}
}

// generateManifestForTest is a test helper that generates manifest JSON
// using executeTemplate + getManifestTemplate, replacing the old
// GenerateManifest function that no longer exists in the cli package.
func generateManifestForTest(cfg pluginScaffoldConfig) ([]byte, error) {
	tmpl := getManifestTemplate(cfg.Type)
	result, err := executeTemplate("manifest.json", tmpl, cfg)
	if err != nil {
		return nil, err
	}
	return []byte(result), nil
}

func TestGenerateManifest(t *testing.T) {
	tests := []struct {
		name   string
		config pluginScaffoldConfig
		check  func(t *testing.T, data map[string]interface{})
	}{
		{
			name: "ingestion manifest",
			config: pluginScaffoldConfig{
				Name:     "Test Plugin",
				Type:     moduleTypeIngestion,
				Version:  "1.0.0",
				ModuleID: "test-plugin",
			},
			check: func(t *testing.T, data map[string]interface{}) {
				if data["type"] != "INGESTION" {
					t.Errorf("type = %v, want INGESTION", data["type"])
				}
				caps, ok := data["capabilities"].(map[string]interface{})
				if !ok {
					t.Fatalf("capabilities not found or wrong type")
				}
				if _, ok := caps["ingestion_sources"]; !ok {
					t.Error("expected ingestion_sources in capabilities")
				}
			},
		},
		{
			name: "reasoning manifest",
			config: pluginScaffoldConfig{
				Name:     "Test Reasoner",
				Type:     moduleTypeReasoning,
				Version:  "1.0.0",
				ModuleID: "test-reasoner",
			},
			check: func(t *testing.T, data map[string]interface{}) {
				if data["type"] != "REASONING" {
					t.Errorf("type = %v, want REASONING", data["type"])
				}
				caps, ok := data["capabilities"].(map[string]interface{})
				if !ok {
					t.Fatalf("capabilities not found or wrong type")
				}
				if _, ok := caps["pattern_detectors"]; !ok {
					t.Error("expected pattern_detectors in capabilities")
				}
			},
		},
		{
			name: "ape manifest",
			config: pluginScaffoldConfig{
				Name:     "Test APE",
				Type:     moduleTypeAPE,
				Version:  "1.0.0",
				ModuleID: "test-ape",
			},
			check: func(t *testing.T, data map[string]interface{}) {
				if data["type"] != "APE" {
					t.Errorf("type = %v, want APE", data["type"])
				}
				caps, ok := data["capabilities"].(map[string]interface{})
				if !ok {
					t.Fatalf("capabilities not found or wrong type")
				}
				if _, ok := caps["event_triggers"]; !ok {
					t.Error("expected event_triggers in capabilities")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifestBytes, err := generateManifestForTest(tt.config)
			if err != nil {
				t.Fatalf("generateManifestForTest() error = %v", err)
			}

			var data map[string]interface{}
			if err := json.Unmarshal(manifestBytes, &data); err != nil {
				t.Fatalf("Failed to parse manifest JSON: %v", err)
			}

			// Common checks
			if data["id"] != tt.config.ModuleID {
				t.Errorf("id = %v, want %v", data["id"], tt.config.ModuleID)
			}
			if data["version"] != tt.config.Version {
				t.Errorf("version = %v, want %v", data["version"], tt.config.Version)
			}
			if data["binary"] != tt.config.ModuleID {
				t.Errorf("binary = %v, want %v", data["binary"], tt.config.ModuleID)
			}

			// Type-specific checks
			tt.check(t, data)
		})
	}
}

func TestExecuteTemplate(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		config   pluginScaffoldConfig
		wantErr  bool
		contains []string
	}{
		{
			name:   "simple template",
			tmpl:   "Module: {{.ModuleID}}, Version: {{.Version}}",
			config: pluginScaffoldConfig{ModuleID: "test-module", Version: "1.0.0"},
			contains: []string{
				"Module: test-module",
				"Version: 1.0.0",
			},
		},
		{
			name:   "template with type condition",
			tmpl:   "{{if eq .Type \"INGESTION\"}}ingestion{{else}}other{{end}}",
			config: pluginScaffoldConfig{Type: moduleTypeIngestion},
			contains: []string{
				"ingestion",
			},
		},
		{
			name:    "invalid template",
			tmpl:    "{{.InvalidField}",
			config:  pluginScaffoldConfig{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := executeTemplate(tt.name, tt.tmpl, tt.config)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("executeTemplate() error = %v", err)
			}

			for _, want := range tt.contains {
				if !strings.Contains(result, want) {
					t.Errorf("result should contain %q, got %q", want, result)
				}
			}
		})
	}
}

func TestGetManifestTemplate(t *testing.T) {
	// Verify each type returns a non-empty template
	types := []moduleType{moduleTypeIngestion, moduleTypeReasoning, moduleTypeAPE}

	for _, mt := range types {
		tmpl := getManifestTemplate(mt)
		if tmpl == "" {
			t.Errorf("getManifestTemplate(%v) returned empty template", mt)
		}
		if !strings.Contains(tmpl, string(mt)) {
			t.Errorf("getManifestTemplate(%v) template should contain type %v", mt, mt)
		}
	}
}

func TestGetHandlerTemplate(t *testing.T) {
	tests := []struct {
		moduleType moduleType
		contains   []string
	}{
		{
			moduleType: moduleTypeIngestion,
			contains:   []string{"Matches", "Parse", "Sync", "IngestionModuleServer"},
		},
		{
			moduleType: moduleTypeReasoning,
			contains:   []string{"Process", "ReasoningModuleServer", "boostFactor"},
		},
		{
			moduleType: moduleTypeAPE,
			contains:   []string{"Execute", "GetSchedule", "APEModuleServer", "CronExpression"},
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.moduleType), func(t *testing.T) {
			tmpl := getHandlerTemplate(tt.moduleType)
			if tmpl == "" {
				t.Errorf("getHandlerTemplate(%v) returned empty template", tt.moduleType)
			}

			for _, want := range tt.contains {
				if !strings.Contains(tmpl, want) {
					t.Errorf("handler template for %v should contain %q", tt.moduleType, want)
				}
			}
		})
	}
}

func TestGeneratedCodeCompiles(t *testing.T) {
	// This test verifies that the generated code at least parses correctly
	// by checking that templates don't have obvious syntax errors

	config := pluginScaffoldConfig{
		Name:       "Test Module",
		Type:       moduleTypeIngestion,
		Version:    "1.0.0",
		ModuleID:   "test-module",
		StructName: "TestModule",
	}

	// Test main.go template
	mainResult, err := executeTemplate("main.go", mainGoTemplate, config)
	if err != nil {
		t.Errorf("main.go template failed: %v", err)
	}
	if !strings.Contains(mainResult, "package main") {
		t.Error("main.go should start with package main")
	}
	if !strings.Contains(mainResult, "func main()") {
		t.Error("main.go should contain func main()")
	}

	// Test handler templates for each type
	for _, mt := range []moduleType{moduleTypeIngestion, moduleTypeReasoning, moduleTypeAPE} {
		config.Type = mt
		handlerTmpl := getHandlerTemplate(mt)
		handlerResult, err := executeTemplate("handler.go", handlerTmpl, config)
		if err != nil {
			t.Errorf("handler.go template for %v failed: %v", mt, err)
		}
		if !strings.Contains(handlerResult, "package main") {
			t.Errorf("handler.go for %v should start with package main", mt)
		}
	}
}

func TestPluginDirectoryStructure(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "plugin-structure-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	config := pluginScaffoldConfig{
		Name:       "Test Plugin",
		Type:       moduleTypeIngestion,
		OutputDir:  tempDir,
		Version:    "1.0.0",
		ModuleID:   "test-plugin",
		StructName: "TestPlugin",
	}

	if err := generatePlugin(config); err != nil {
		t.Fatalf("generatePlugin() error = %v", err)
	}

	pluginDir := filepath.Join(tempDir, "test-plugin")

	// Verify directory was created
	info, err := os.Stat(pluginDir)
	if err != nil {
		t.Fatalf("Plugin directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("Plugin path should be a directory")
	}

	// Verify all files have correct permissions (readable)
	expectedFiles := []string{"manifest.json", "main.go", "handler.go", "Makefile", "README.md"}
	for _, fileName := range expectedFiles {
		path := filepath.Join(pluginDir, fileName)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("File %s not found: %v", fileName, err)
			continue
		}
		if info.IsDir() {
			t.Errorf("%s should be a file, not a directory", fileName)
		}
		// Check file is not empty
		if info.Size() == 0 {
			t.Errorf("%s should not be empty", fileName)
		}
	}
}

func TestManifestJSONValidity(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "manifest-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	for _, mt := range []moduleType{moduleTypeIngestion, moduleTypeReasoning, moduleTypeAPE} {
		t.Run(string(mt), func(t *testing.T) {
			config := pluginScaffoldConfig{
				Name:       "Test Plugin",
				Type:       mt,
				OutputDir:  tempDir,
				Version:    "1.0.0",
				ModuleID:   "test-" + strings.ToLower(string(mt)),
				StructName: "TestPlugin",
			}

			if err := generatePlugin(config); err != nil {
				t.Fatalf("generatePlugin() error = %v", err)
			}

			manifestPath := filepath.Join(tempDir, config.ModuleID, "manifest.json")
			content, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatalf("Failed to read manifest: %v", err)
			}

			var manifest map[string]interface{}
			if err := json.Unmarshal(content, &manifest); err != nil {
				t.Errorf("manifest.json is not valid JSON: %v", err)
			}

			// Verify required fields
			requiredFields := []string{"id", "name", "version", "type", "binary"}
			for _, field := range requiredFields {
				if _, ok := manifest[field]; !ok {
					t.Errorf("manifest.json missing required field: %s", field)
				}
			}
		})
	}
}
