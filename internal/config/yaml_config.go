package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// YAMLConfig represents the curated settings exposed in .mdemg/config.yaml.
// Only ~20 commonly-adjusted settings are included; advanced users use env vars directly.
type YAMLConfig struct {
	Neo4j     Neo4jYAML     `yaml:"neo4j"`
	Server    ServerYAML    `yaml:"server"`
	LLM       LLMYAML       `yaml:"llm,omitempty"`
	Embedding EmbeddingYAML `yaml:"embedding"`
	Backup    BackupYAML    `yaml:"backup,omitempty"`
	Retrieval RetrievalYAML `yaml:"retrieval,omitempty"`
	Learning  LearningYAML  `yaml:"learning,omitempty"`
	Plugins   PluginsYAML   `yaml:"plugins,omitempty"`
	Schema    SchemaYAML    `yaml:"schema,omitempty"`
	Jiminy    JiminyYAML    `yaml:"jiminy"`
	Ingest    IngestYAML    `yaml:"ingest,omitempty"`
}

// IngestYAML holds ingestion speed preset settings.
type IngestYAML struct {
	Speed string `yaml:"speed,omitempty"` // Speed preset: fast, balanced, thorough
}

// BackupYAML holds periodic backup and retention settings.
type BackupYAML struct {
	Enabled        bool   `yaml:"enabled,omitempty"`         // Enable automatic backups (default: true for project repos)
	StorageDir     string `yaml:"storage_dir,omitempty"`     // Backup directory (default: .mdemg/backups)
	IntervalHours  int    `yaml:"interval_hours,omitempty"`  // Hours between partial backups (default: 24)
	RetentionCount int    `yaml:"retention_count,omitempty"` // Keep last N backups per type (default: 2)
}

// LLMYAML holds top-level LLM text-generation settings that cascade to all features.
type LLMYAML struct {
	Provider string `yaml:"provider,omitempty"` // "ollama" or "openai" (default: "openai")
	Model    string `yaml:"model,omitempty"`    // default: "gpt-5-nano"
}

// Neo4jYAML holds Neo4j connection settings.
type Neo4jYAML struct {
	URI      string `yaml:"uri,omitempty"`
	User     string `yaml:"user,omitempty"`
	Password string `yaml:"password,omitempty"`
	BoltPort int    `yaml:"bolt_port,omitempty"` // Preferred bolt port (default: 7687)
	HTTPPort int    `yaml:"http_port,omitempty"` // Preferred HTTP port (default: 7474)
}

// ServerYAML holds HTTP server settings.
type ServerYAML struct {
	Port int `yaml:"port,omitempty"`
}

// EmbeddingYAML holds embedding provider settings.
type EmbeddingYAML struct {
	Provider string `yaml:"provider,omitempty"` // "ollama", "openai", or "" (disabled)
	Model    string `yaml:"model,omitempty"`
	Endpoint string `yaml:"endpoint,omitempty"`
	// OpenAI-specific (secrets should stay in .env, but supported here for completeness)
	OpenAIKey string `yaml:"openai_key,omitempty"`
}

// RetrievalYAML holds retrieval tuning settings.
type RetrievalYAML struct {
	CandidateK int `yaml:"candidate_k,omitempty"`
	TopK       int `yaml:"top_k,omitempty"`
	HopDepth   int `yaml:"hop_depth,omitempty"`
}

// LearningYAML holds Hebbian learning settings.
type LearningYAML struct {
	Eta           float64 `yaml:"eta,omitempty"`
	DecayPerDay   float64 `yaml:"decay_per_day,omitempty"`
	MaxEdgesPerNode int   `yaml:"max_edges_per_node,omitempty"`
}

// PluginsYAML holds plugin system settings.
type PluginsYAML struct {
	Enabled bool   `yaml:"enabled,omitempty"`
	Dir     string `yaml:"dir,omitempty"`
}

// SchemaYAML holds schema version settings.
type SchemaYAML struct {
	Version int `yaml:"version,omitempty"`
}

// JiminyYAML holds Jiminy inner-voice guidance settings.
type JiminyYAML struct {
	Enabled             *bool  `yaml:"enabled,omitempty"`               // Enable Jiminy guidance (default: true; nil = defer to env/default)
	SynthesisEnabled    bool   `yaml:"synthesis_enabled,omitempty"`     // Enable LLM synthesis (default: true)
	SynthesisProvider   string `yaml:"synthesis_provider,omitempty"`    // LLM provider for synthesis (inherits llm.provider)
	SynthesisModel      string `yaml:"synthesis_model,omitempty"`       // LLM model for synthesis (inherits llm.model)
	EvaluateEnabled     bool   `yaml:"evaluate_enabled,omitempty"`      // Enable agent output evaluation (default: true)
	EvaluateLLMEnabled  bool   `yaml:"evaluate_llm_enabled,omitempty"`  // Enable LLM Tier 2 evaluation (default: false)
	EvaluateLLMProvider string `yaml:"evaluate_llm_provider,omitempty"` // LLM provider for evaluation (inherits llm.provider)
	EvaluateLLMModel    string `yaml:"evaluate_llm_model,omitempty"`    // LLM model for evaluation (inherits llm.model)
	OutcomeLLMEnabled          bool `yaml:"outcome_llm_enabled,omitempty"`            // Enable LLM outcome classification (default: false)
	GuidanceContextMaxChars    int  `yaml:"guidance_context_max_chars,omitempty"`     // J16: max chars for agent context in guidance (default: 200000)
	GuidanceOutputMaxChars     int  `yaml:"guidance_output_max_chars,omitempty"`      // J16: max chars for agent output in guidance (default: 200000)
	EvaluateOutputMaxChars     int  `yaml:"evaluate_output_max_chars,omitempty"`      // J16: max chars for agent output in eval (default: 200000)
	EvaluateItemMaxChars       int  `yaml:"evaluate_item_max_chars,omitempty"`        // J16: max chars per constraint/correction in eval (default: 0 = unlimited)
	J17Enabled                 bool `yaml:"j17_enabled,omitempty"`                    // J17: enable AI-to-AI protocol (default: false)
	J17TicketTTLHours          int  `yaml:"j17_ticket_ttl_hours,omitempty"`           // J17: session ticket TTL in hours (default: 4)
}

// yamlEnvMapping defines the mapping from YAML dot-path to environment variable.
// The special prefix "PORT:" indicates the value needs int→":PORT" conversion.
var yamlEnvMapping = []struct {
	yamlPath string
	envVar   string
	convert  func(string) string // optional conversion, nil = identity
}{
	{"neo4j.uri", "NEO4J_URI", nil},
	{"neo4j.user", "NEO4J_USER", nil},
	{"neo4j.password", "NEO4J_PASS", nil},
	{"neo4j.bolt_port", "NEO4J_BOLT_PORT", nil},
	{"neo4j.http_port", "NEO4J_HTTP_PORT", nil},
	{"server.port", "LISTEN_ADDR", convertPort},
	{"llm.provider", "LLM_PROVIDER", nil},
	{"llm.model", "LLM_MODEL", nil},
	{"embedding.provider", "EMBEDDING_PROVIDER", nil},
	{"embedding.model", "OLLAMA_MODEL", nil},
	{"embedding.endpoint", "OLLAMA_ENDPOINT", nil},
	{"embedding.openai_key", "OPENAI_API_KEY", nil},
	{"retrieval.candidate_k", "DEFAULT_CANDIDATE_K", nil},
	{"retrieval.top_k", "DEFAULT_TOP_K", nil},
	{"retrieval.hop_depth", "DEFAULT_HOP_DEPTH", nil},
	{"learning.eta", "LEARNING_ETA", nil},
	{"learning.decay_per_day", "LEARNING_DECAY_PER_DAY", nil},
	{"learning.max_edges_per_node", "LEARNING_MAX_EDGES_PER_NODE", nil},
	{"emergence.provider", "EMERGENCE_PROVIDER", nil},
	{"emergence.model", "EMERGENCE_MODEL", nil},
	{"summary.provider", "LLM_SUMMARY_PROVIDER", nil},
	{"summary.model", "LLM_SUMMARY_MODEL", nil},
	{"reranker.provider", "RERANK_PROVIDER", nil},
	{"reranker.model", "RERANK_MODEL", nil},
	{"plugins.enabled", "PLUGINS_ENABLED", nil},
	{"plugins.dir", "PLUGINS_DIR", nil},
	{"schema.version", "REQUIRED_SCHEMA_VERSION", nil},
	{"backup.enabled", "BACKUP_ENABLED", nil},
	{"backup.storage_dir", "BACKUP_STORAGE_DIR", nil},
	{"backup.interval_hours", "BACKUP_PARTIAL_INTERVAL_HOURS", nil},
	{"backup.retention_count", "BACKUP_RETENTION_FULL_COUNT", nil},
	{"backup.retention_count", "BACKUP_RETENTION_PARTIAL_COUNT", nil},
	// Jiminy inner-voice
	{"jiminy.enabled", "JIMINY_ENABLED", nil},
	{"jiminy.synthesis_enabled", "JIMINY_SYNTHESIS_ENABLED", nil},
	{"jiminy.synthesis_provider", "JIMINY_SYNTHESIS_PROVIDER", nil},
	{"jiminy.synthesis_model", "JIMINY_SYNTHESIS_MODEL", nil},
	{"jiminy.evaluate_enabled", "JIMINY_EVALUATE_ENABLED", nil},
	{"jiminy.evaluate_llm_enabled", "JIMINY_EVALUATE_LLM_ENABLED", nil},
	{"jiminy.evaluate_llm_provider", "JIMINY_EVALUATE_LLM_PROVIDER", nil},
	{"jiminy.evaluate_llm_model", "JIMINY_EVALUATE_LLM_MODEL", nil},
	{"jiminy.outcome_llm_enabled", "JIMINY_OUTCOME_LLM_ENABLED", nil},
	{"jiminy.guidance_context_max_chars", "JIMINY_GUIDANCE_CONTEXT_MAX_CHARS", nil},
	{"jiminy.guidance_output_max_chars", "JIMINY_GUIDANCE_OUTPUT_MAX_CHARS", nil},
	{"jiminy.evaluate_output_max_chars", "JIMINY_EVALUATE_OUTPUT_MAX_CHARS", nil},
	{"jiminy.evaluate_item_max_chars", "JIMINY_EVALUATE_ITEM_MAX_CHARS", nil},
	// J17: AI-to-AI Communication Protocol
	{"jiminy.j17_enabled", "J17_ENABLED", nil},
	{"jiminy.j17_ticket_ttl_hours", "J17_TICKET_TTL_HOURS", nil},
	// Ingest
	{"ingest.speed", "INGEST_SPEED", nil},
}

// convertPort converts a port number string to ":PORT" format for LISTEN_ADDR.
func convertPort(v string) string {
	if v == "" || v == "0" {
		return ""
	}
	return ":" + v
}

// FindConfigFile searches for .mdemg/config.yaml walking up from the current
// working directory. Returns the absolute path or "" if not found.
func FindConfigFile() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(dir, ".mdemg", "config.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// LoadYAMLConfig reads a YAML config file and sets environment variables for
// any values not already set. This preserves the priority: env > yaml > defaults.
// It should be called before godotenv.Load() and config.FromEnv().
func LoadYAMLConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	var cfg YAMLConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	flat := flattenYAML(cfg)

	for _, m := range yamlEnvMapping {
		val, ok := flat[m.yamlPath]
		if !ok || val == "" {
			continue
		}
		// Only set if the env var is not already set (env > yaml priority)
		if os.Getenv(m.envVar) != "" {
			continue
		}
		if m.convert != nil {
			val = m.convert(val)
		}
		if val != "" {
			if err := os.Setenv(m.envVar, val); err != nil {
				return fmt.Errorf("set %s: %w", m.envVar, err)
			}
		}
	}

	return nil
}

// flattenYAML converts a YAMLConfig struct into a flat map of dot-path → string value.
func flattenYAML(cfg YAMLConfig) map[string]string {
	m := make(map[string]string)

	// Neo4j
	setIfNonEmpty(m, "neo4j.uri", cfg.Neo4j.URI)
	setIfNonEmpty(m, "neo4j.user", cfg.Neo4j.User)
	setIfNonEmpty(m, "neo4j.password", cfg.Neo4j.Password)
	if cfg.Neo4j.BoltPort > 0 {
		m["neo4j.bolt_port"] = strconv.Itoa(cfg.Neo4j.BoltPort)
	}
	if cfg.Neo4j.HTTPPort > 0 {
		m["neo4j.http_port"] = strconv.Itoa(cfg.Neo4j.HTTPPort)
	}

	// Server
	if cfg.Server.Port > 0 {
		m["server.port"] = strconv.Itoa(cfg.Server.Port)
	}

	// LLM
	setIfNonEmpty(m, "llm.provider", cfg.LLM.Provider)
	setIfNonEmpty(m, "llm.model", cfg.LLM.Model)

	// Embedding
	setIfNonEmpty(m, "embedding.provider", cfg.Embedding.Provider)
	setIfNonEmpty(m, "embedding.model", cfg.Embedding.Model)
	setIfNonEmpty(m, "embedding.endpoint", cfg.Embedding.Endpoint)
	setIfNonEmpty(m, "embedding.openai_key", cfg.Embedding.OpenAIKey)

	// Retrieval
	if cfg.Retrieval.CandidateK > 0 {
		m["retrieval.candidate_k"] = strconv.Itoa(cfg.Retrieval.CandidateK)
	}
	if cfg.Retrieval.TopK > 0 {
		m["retrieval.top_k"] = strconv.Itoa(cfg.Retrieval.TopK)
	}
	if cfg.Retrieval.HopDepth > 0 {
		m["retrieval.hop_depth"] = strconv.Itoa(cfg.Retrieval.HopDepth)
	}

	// Learning
	if cfg.Learning.Eta > 0 {
		m["learning.eta"] = strconv.FormatFloat(cfg.Learning.Eta, 'f', -1, 64)
	}
	if cfg.Learning.DecayPerDay > 0 {
		m["learning.decay_per_day"] = strconv.FormatFloat(cfg.Learning.DecayPerDay, 'f', -1, 64)
	}
	if cfg.Learning.MaxEdgesPerNode > 0 {
		m["learning.max_edges_per_node"] = strconv.Itoa(cfg.Learning.MaxEdgesPerNode)
	}

	// Plugins
	if cfg.Plugins.Enabled {
		m["plugins.enabled"] = "true"
	}
	setIfNonEmpty(m, "plugins.dir", cfg.Plugins.Dir)

	// Backup
	if cfg.Backup.Enabled {
		m["backup.enabled"] = "true"
	}
	setIfNonEmpty(m, "backup.storage_dir", cfg.Backup.StorageDir)
	if cfg.Backup.IntervalHours > 0 {
		m["backup.interval_hours"] = strconv.Itoa(cfg.Backup.IntervalHours)
	}
	if cfg.Backup.RetentionCount > 0 {
		m["backup.retention_count"] = strconv.Itoa(cfg.Backup.RetentionCount)
	}

	// Schema
	if cfg.Schema.Version > 0 {
		m["schema.version"] = strconv.Itoa(cfg.Schema.Version)
	}

	// Jiminy — only emit enabled when explicitly set in YAML (nil = absent, not false)
	if cfg.Jiminy.Enabled != nil {
		m["jiminy.enabled"] = strconv.FormatBool(*cfg.Jiminy.Enabled)
	}
	if cfg.Jiminy.SynthesisEnabled {
		m["jiminy.synthesis_enabled"] = "true"
	}
	setIfNonEmpty(m, "jiminy.synthesis_provider", cfg.Jiminy.SynthesisProvider)
	setIfNonEmpty(m, "jiminy.synthesis_model", cfg.Jiminy.SynthesisModel)
	if cfg.Jiminy.EvaluateEnabled {
		m["jiminy.evaluate_enabled"] = "true"
	}
	if cfg.Jiminy.EvaluateLLMEnabled {
		m["jiminy.evaluate_llm_enabled"] = "true"
	}
	setIfNonEmpty(m, "jiminy.evaluate_llm_provider", cfg.Jiminy.EvaluateLLMProvider)
	setIfNonEmpty(m, "jiminy.evaluate_llm_model", cfg.Jiminy.EvaluateLLMModel)
	if cfg.Jiminy.OutcomeLLMEnabled {
		m["jiminy.outcome_llm_enabled"] = "true"
	}
	if cfg.Jiminy.GuidanceContextMaxChars > 0 {
		m["jiminy.guidance_context_max_chars"] = strconv.Itoa(cfg.Jiminy.GuidanceContextMaxChars)
	}
	if cfg.Jiminy.GuidanceOutputMaxChars > 0 {
		m["jiminy.guidance_output_max_chars"] = strconv.Itoa(cfg.Jiminy.GuidanceOutputMaxChars)
	}
	if cfg.Jiminy.EvaluateOutputMaxChars > 0 {
		m["jiminy.evaluate_output_max_chars"] = strconv.Itoa(cfg.Jiminy.EvaluateOutputMaxChars)
	}
	if cfg.Jiminy.EvaluateItemMaxChars > 0 {
		m["jiminy.evaluate_item_max_chars"] = strconv.Itoa(cfg.Jiminy.EvaluateItemMaxChars)
	}

	// Ingest
	setIfNonEmpty(m, "ingest.speed", cfg.Ingest.Speed)

	return m
}

func setIfNonEmpty(m map[string]string, key, val string) {
	if val != "" {
		m[key] = val
	}
}

// UpdateNeo4jURI reads the existing .mdemg/config.yaml, updates the neo4j.uri
// field, and writes it back. If the config file does not exist, it creates a
// minimal one. Other fields are preserved.
func UpdateNeo4jURI(configPath, uri string) error {
	// Ensure the parent directory exists
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	var cfg YAMLConfig
	if data, err := os.ReadFile(configPath); err == nil {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("parse existing config: %w", err)
		}
	}

	cfg.Neo4j.URI = uri

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	header := "# MDEMG Project Configuration\n" +
		"# Updated by: mdemg db start\n\n"

	return os.WriteFile(configPath, []byte(header+string(data)), 0o644)
}

// ConfigError represents a validation error in the config file.
type ConfigError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Level   string `json:"level"` // "error" or "warning"
}

func (e ConfigError) String() string {
	return fmt.Sprintf("[%s] %s: %s", e.Level, e.Field, e.Message)
}

// ValidateConfigFile reads and validates a YAML config file.
// Returns a list of errors and warnings.
func ValidateConfigFile(path string) []ConfigError {
	var errs []ConfigError

	data, err := os.ReadFile(path)
	if err != nil {
		return []ConfigError{{Field: "file", Message: fmt.Sprintf("cannot read: %v", err), Level: "error"}}
	}

	var cfg YAMLConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return []ConfigError{{Field: "yaml", Message: fmt.Sprintf("parse error: %v", err), Level: "error"}}
	}

	// Validate Neo4j settings
	if cfg.Neo4j.URI != "" {
		if !strings.HasPrefix(cfg.Neo4j.URI, "bolt://") && !strings.HasPrefix(cfg.Neo4j.URI, "neo4j://") && !strings.HasPrefix(cfg.Neo4j.URI, "neo4j+s://") {
			errs = append(errs, ConfigError{Field: "neo4j.uri", Message: "must start with bolt://, neo4j://, or neo4j+s://", Level: "error"})
		}
	}

	// Validate server port
	if cfg.Server.Port != 0 && (cfg.Server.Port < 1 || cfg.Server.Port > 65535) {
		errs = append(errs, ConfigError{Field: "server.port", Message: "must be between 1 and 65535", Level: "error"})
	}

	// Validate embedding provider
	if cfg.Embedding.Provider != "" {
		switch cfg.Embedding.Provider {
		case "ollama", "openai":
			// valid
		default:
			errs = append(errs, ConfigError{Field: "embedding.provider", Message: "must be 'ollama', 'openai', or empty (disabled)", Level: "error"})
		}
	}

	// Validate schema version
	if cfg.Schema.Version != 0 && cfg.Schema.Version < 1 {
		errs = append(errs, ConfigError{Field: "schema.version", Message: "must be >= 1", Level: "error"})
	}

	// Validate ingest speed preset
	if cfg.Ingest.Speed != "" {
		switch cfg.Ingest.Speed {
		case "fast", "balanced", "thorough":
			// valid
		default:
			errs = append(errs, ConfigError{Field: "ingest.speed", Message: "must be 'fast', 'balanced', 'thorough', or empty", Level: "error"})
		}
	}

	// Warnings for missing recommended values
	if cfg.Neo4j.URI == "" && os.Getenv("NEO4J_URI") == "" {
		errs = append(errs, ConfigError{Field: "neo4j.uri", Message: "not set (required for server to start)", Level: "warning"})
	}
	if cfg.Schema.Version == 0 && os.Getenv("REQUIRED_SCHEMA_VERSION") == "" {
		errs = append(errs, ConfigError{Field: "schema.version", Message: "not set (will auto-detect from embedded migrations)", Level: "info"})
	}

	return errs
}

// InitOptions holds the answers from the init wizard for config generation.
type InitOptions struct {
	SpaceID           string
	Neo4jURI          string
	Neo4jUser         string
	Neo4jPassword     string
	Neo4jBoltPort     int
	Neo4jHTTPPort     int
	ServerPort        int
	LLMProvider       string
	LLMModel          string
	EmbeddingProvider string
	EmbeddingModel    string
	EmbeddingEndpoint string
	SchemaVersion     int
	BackupEnabled     bool
	BackupStorageDir  string
	BackupInterval    int
	BackupRetention   int
	PluginsEnabled    bool
	PluginsDir        string
	JiminyEnabled     bool
	JiminyModel       string
	JiminyProvider    string
}

// GenerateConfigYAML produces a config.yaml file from wizard answers.
func GenerateConfigYAML(opts InitOptions) ([]byte, error) {
	cfg := YAMLConfig{
		Neo4j: Neo4jYAML{
			URI:      opts.Neo4jURI,
			User:     opts.Neo4jUser,
			BoltPort: opts.Neo4jBoltPort,
			HTTPPort: opts.Neo4jHTTPPort,
			// Password intentionally omitted — stays in .env or env vars
		},
		Server: ServerYAML{
			Port: opts.ServerPort,
		},
		Schema: SchemaYAML{
			Version: opts.SchemaVersion,
		},
	}

	if opts.LLMProvider != "" {
		cfg.LLM = LLMYAML{
			Provider: opts.LLMProvider,
			Model:    opts.LLMModel,
		}
	}

	if opts.EmbeddingProvider != "" && opts.EmbeddingProvider != "disabled" {
		cfg.Embedding = EmbeddingYAML{
			Provider: opts.EmbeddingProvider,
			Model:    opts.EmbeddingModel,
			Endpoint: opts.EmbeddingEndpoint,
		}
	}

	if opts.BackupEnabled {
		cfg.Backup = BackupYAML{
			Enabled:        true,
			StorageDir:     opts.BackupStorageDir,
			IntervalHours:  opts.BackupInterval,
			RetentionCount: opts.BackupRetention,
		}
	}

	if opts.PluginsEnabled {
		cfg.Plugins = PluginsYAML{
			Enabled: true,
			Dir:     opts.PluginsDir,
		}
	}

	// Always emit jiminy section when wizard runs (explicit user choice)
	jiminyEnabled := opts.JiminyEnabled
	cfg.Jiminy = JiminyYAML{
		Enabled: &jiminyEnabled,
	}
	if opts.JiminyEnabled && opts.JiminyModel != "" {
		cfg.Jiminy.SynthesisEnabled = true
		cfg.Jiminy.SynthesisProvider = opts.JiminyProvider
		cfg.Jiminy.SynthesisModel = opts.JiminyModel
		cfg.Jiminy.EvaluateEnabled = true
		cfg.Jiminy.EvaluateLLMProvider = opts.JiminyProvider
		cfg.Jiminy.EvaluateLLMModel = opts.JiminyModel
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}

	// Add header comment
	header := "# MDEMG Project Configuration\n" +
		"# Generated by: mdemg init\n" +
		"#\n" +
		"# This file sets common configuration values. For the full 160+ env vars,\n" +
		"# see .env.example or docs/features/unified-cli.md.\n" +
		"#\n" +
		"# Priority (lowest to highest):\n" +
		"#   1. Defaults (in FromEnv)\n" +
		"#   2. This file (.mdemg/config.yaml)\n" +
		"#   3. .env file\n" +
		"#   4. Environment variables\n" +
		"#   5. CLI flags\n" +
		"#\n" +
		"# IMPORTANT: Do NOT put secrets (passwords, API keys) in this file.\n" +
		"# Use .env (gitignored) or environment variables for secrets.\n\n"

	return []byte(header + string(data)), nil
}

// GenerateIgnoreFile creates a default .mdemgignore file, optionally seeding
// from an existing .gitignore in the same directory.
func GenerateIgnoreFile(projectDir string) ([]byte, error) {
	var lines []string

	lines = append(lines,
		"# MDEMG Ingestion Ignore Patterns",
		"# Syntax: same as .gitignore (line-based, # comments, ! negation)",
		"",
		"# Version control",
		".git/",
		".svn/",
		"",
		"# Dependencies",
		"node_modules/",
		"vendor/",
		"__pycache__/",
		".venv/",
		"venv/",
		"",
		"# Build artifacts",
		"build/",
		"dist/",
		"target/",
		"bin/",
		"",
		"# IDE/editor",
		".idea/",
		".vscode/",
		".cursor/",
		"*.swp",
		"*.swo",
		"",
		"# OS files",
		".DS_Store",
		"Thumbs.db",
		"",
		"# Binary / large files",
		"*.exe",
		"*.dll",
		"*.so",
		"*.dylib",
		"*.zip",
		"*.tar.gz",
		"*.whl",
		"*.pyc",
		"*.min.js",
		"*.bundle.js",
		"*.map",
		"",
		"# Secrets / environment",
		".env",
		".env.*",
		"",
		"# MDEMG runtime",
		".mdemg/backups/",
		".mdemg/logs/",
	)

	// Seed from .gitignore if it exists
	gitignorePath := filepath.Join(projectDir, ".gitignore")
	if data, err := os.ReadFile(gitignorePath); err == nil {
		gitLines := strings.Split(string(data), "\n")
		seeded := false
		for _, line := range gitLines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			if !seeded {
				lines = append(lines, "", "# From .gitignore")
				seeded = true
			}
			lines = append(lines, trimmed)
		}
	}

	lines = append(lines, "") // trailing newline
	return []byte(strings.Join(lines, "\n")), nil
}

// FindIgnoreFile searches for .mdemgignore walking up from startDir.
// Returns the absolute path or "" if not found.
func FindIgnoreFile(startDir string) string {
	dir := startDir
	for {
		candidate := filepath.Join(dir, ".mdemgignore")
		if _, err := os.Stat(candidate); err == nil {
			abs, _ := filepath.Abs(candidate)
			return abs
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// IgnorePattern represents a single pattern from .mdemgignore.
type IgnorePattern struct {
	Pattern  string // the raw pattern (without leading !)
	Negated  bool   // true if the line started with !
	DirOnly  bool   // true if the pattern ends with /
}

// ParseIgnoreFile reads a .mdemgignore file and returns the parsed patterns.
// Syntax follows .gitignore conventions:
//   - Lines starting with # are comments
//   - Empty lines are ignored
//   - Lines starting with ! negate the pattern
//   - Lines ending with / match directories only
//   - * matches anything except /
//   - ** matches any path segment(s)
func ParseIgnoreFile(path string) ([]IgnorePattern, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var patterns []IgnorePattern
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		p := IgnorePattern{}
		if strings.HasPrefix(line, "!") {
			p.Negated = true
			line = line[1:]
		}
		if strings.HasSuffix(line, "/") {
			p.DirOnly = true
			line = strings.TrimSuffix(line, "/")
		}
		p.Pattern = line
		patterns = append(patterns, p)
	}

	return patterns, nil
}

// MatchesIgnorePatterns checks whether a given path (relative to project root)
// should be ignored based on the parsed .mdemgignore patterns.
// isDir should be true if the path is a directory.
func MatchesIgnorePatterns(relPath string, isDir bool, patterns []IgnorePattern) bool {
	ignored := false
	name := filepath.Base(relPath)

	for _, p := range patterns {
		if p.DirOnly && !isDir {
			continue
		}

		matched := matchPattern(p.Pattern, relPath, name)
		if matched {
			ignored = !p.Negated
		}
	}

	return ignored
}

// matchPattern checks if a gitignore-style pattern matches the given path.
// It tries matching against both the full relative path and just the basename.
func matchPattern(pattern, relPath, name string) bool {
	// If the pattern contains a slash, match against the full path
	if strings.Contains(pattern, "/") {
		return matchGlob(pattern, relPath)
	}
	// Otherwise match against the basename
	return matchGlob(pattern, name)
}

// matchGlob matches a gitignore-style glob pattern against a string.
// Handles ** (any path segments) and delegates to filepath.Match for the rest.
func matchGlob(pattern, s string) bool {
	// Handle ** patterns
	if strings.Contains(pattern, "**") {
		// Split pattern on **
		parts := strings.Split(pattern, "**")
		if len(parts) == 2 {
			prefix := strings.TrimSuffix(parts[0], "/")
			suffix := strings.TrimPrefix(parts[1], "/")

			if prefix == "" && suffix == "" {
				return true // ** matches everything
			}
			if prefix == "" {
				// **/suffix — match suffix at any depth
				segments := strings.Split(s, string(filepath.Separator))
				for i := range segments {
					tail := strings.Join(segments[i:], string(filepath.Separator))
					if matched, _ := filepath.Match(suffix, tail); matched {
						return true
					}
				}
				return false
			}
			if suffix == "" {
				// prefix/** — match prefix as directory prefix
				return strings.HasPrefix(s, prefix+string(filepath.Separator)) || s == prefix
			}
		}
	}

	// Simple glob via filepath.Match
	if matched, _ := filepath.Match(pattern, s); matched {
		return true
	}
	return false
}

// ConfigSource describes where a configuration value came from.
type ConfigSource struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Source string `json:"source"` // "yaml", "env", "default", "flag"
	Masked bool   `json:"masked"` // true for secrets
}

// EffectiveConfig reads the effective configuration and returns each key's
// value and source. Used by `mdemg config show`.
func EffectiveConfig(yamlPath string) []ConfigSource {
	var result []ConfigSource

	// Load YAML if present
	var yamlCfg YAMLConfig
	var yamlFlat map[string]string
	if yamlPath != "" {
		if data, err := os.ReadFile(yamlPath); err == nil {
			if err := yaml.Unmarshal(data, &yamlCfg); err == nil {
				yamlFlat = flattenYAML(yamlCfg)
			}
		}
	}
	if yamlFlat == nil {
		yamlFlat = make(map[string]string)
	}

	// For each mapping, determine source (deduplicate by YAML path so entries
	// like backup.retention_count that map to multiple env vars appear only once)
	seen := make(map[string]bool)
	for _, m := range yamlEnvMapping {
		if seen[m.yamlPath] {
			continue
		}
		seen[m.yamlPath] = true

		envVal := os.Getenv(m.envVar)
		yamlVal := yamlFlat[m.yamlPath]

		cs := ConfigSource{
			Key:    m.yamlPath,
			Masked: isSensitive(m.envVar),
		}

		if envVal != "" {
			cs.Value = envVal
			cs.Source = "env"
		} else if yamlVal != "" {
			cs.Value = yamlVal
			cs.Source = "yaml"
		} else {
			cs.Value = ""
			cs.Source = "default"
		}

		if cs.Masked && cs.Value != "" {
			cs.Value = "****"
		}

		result = append(result, cs)
	}

	return result
}

// isSensitive returns true for env vars that contain secrets.
func isSensitive(envVar string) bool {
	switch envVar {
	case "NEO4J_PASS", "OPENAI_API_KEY":
		return true
	}
	return false
}
