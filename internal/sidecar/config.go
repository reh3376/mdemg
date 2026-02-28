package sidecar

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	configFileName = "sidecar.yaml"
	mdemgDir       = ".mdemg"
)

// FindConfigFile walks up the directory tree looking for .mdemg/sidecar.yaml.
// Returns the full path or "" if not found.
func FindConfigFile() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return FindConfigFileFrom(dir)
}

// FindConfigFileFrom walks up from the given directory looking for .mdemg/sidecar.yaml.
func FindConfigFileFrom(startDir string) string {
	dir := startDir
	for {
		candidate := filepath.Join(dir, mdemgDir, configFileName)
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

// LoadConfig reads and unmarshals a sidecar.yaml file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// ConfigError represents a single validation issue.
type ConfigError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Level   string `json:"level"` // "error" or "warning"
}

// ValidateConfig checks a Config for correctness per the schema contract.
// Returns a slice of ConfigErrors (may be empty).
func ValidateConfig(cfg *Config) []ConfigError {
	var errs []ConfigError

	// version
	if cfg.Version != ConfigVersion {
		errs = append(errs, ConfigError{
			Field:   "version",
			Message: fmt.Sprintf("unsupported version %q, expected %q", cfg.Version, ConfigVersion),
			Level:   "error",
		})
	}

	// profile
	if !cfg.Profile.IsValid() {
		errs = append(errs, ConfigError{
			Field:   "profile",
			Message: fmt.Sprintf("unknown profile %q, must be one of: local, studio-remote", cfg.Profile),
			Level:   "error",
		})
	}

	// endpoint
	if strings.TrimSpace(cfg.Runtime.Endpoint) == "" {
		errs = append(errs, ConfigError{
			Field:   "runtime.endpoint",
			Message: "endpoint is required",
			Level:   "error",
		})
	}

	// studio-remote requires host
	if cfg.Profile == ProfileStudioRemote && strings.TrimSpace(cfg.Runtime.Remote.Host) == "" {
		errs = append(errs, ConfigError{
			Field:   "runtime.remote.host",
			Message: "host is required when profile is studio-remote",
			Level:   "error",
		})
	}

	// transport
	switch cfg.Runtime.Remote.Transport {
	case TransportDockerContext, TransportSSHExec, "":
		// valid (empty is ok for local profile)
	default:
		errs = append(errs, ConfigError{
			Field:   "runtime.remote.transport",
			Message: fmt.Sprintf("unknown transport %q, must be one of: docker-context, ssh-exec", cfg.Runtime.Remote.Transport),
			Level:   "error",
		})
	}

	// adapter uniqueness
	seen := make(map[AdapterName]bool)
	for i, a := range cfg.Adapters {
		switch a.Name {
		case AdapterClaudeCode, AdapterCodex:
			// valid
		default:
			errs = append(errs, ConfigError{
				Field:   fmt.Sprintf("adapters[%d].name", i),
				Message: fmt.Sprintf("unknown adapter %q, must be one of: claude-code, codex", a.Name),
				Level:   "error",
			})
		}
		if seen[a.Name] {
			errs = append(errs, ConfigError{
				Field:   fmt.Sprintf("adapters[%d].name", i),
				Message: fmt.Sprintf("duplicate adapter name %q", a.Name),
				Level:   "error",
			})
		}
		seen[a.Name] = true
	}

	// space_id_strategy
	if strings.TrimSpace(cfg.Hooks.SpaceIDStrategy) == "" {
		errs = append(errs, ConfigError{
			Field:   "hooks.space_id_strategy",
			Message: "space_id_strategy is required",
			Level:   "error",
		})
	}

	return errs
}

// HasErrors returns true if any ConfigError has level "error".
func HasErrors(errs []ConfigError) bool {
	for _, e := range errs {
		if e.Level == "error" {
			return true
		}
	}
	return false
}

// GenerateConfig builds a Config struct with defaults for the given profile and adapters.
func GenerateConfig(profile Profile, adapters []AdapterName, endpoint, host string) *Config {
	if endpoint == "" {
		endpoint = "http://localhost:9999"
	}

	transport := Transport("")
	if profile == ProfileStudioRemote {
		transport = TransportDockerContext
	}

	adapterConfigs := make([]AdapterConfig, 0, len(adapters))
	for _, name := range adapters {
		adapterConfigs = append(adapterConfigs, AdapterConfig{
			Name:    name,
			Enabled: true,
		})
	}

	return &Config{
		Version: ConfigVersion,
		Profile: profile,
		Runtime: RuntimeConfig{
			Endpoint: endpoint,
			Remote: RemoteConfig{
				Host:      host,
				Transport: transport,
			},
		},
		Adapters: adapterConfigs,
		Hooks: HooksConfig{
			SpaceIDStrategy: "repo-basename",
		},
		Install: InstallConfig{
			AutoFix: true,
		},
	}
}

// MarshalConfigYAML serializes a Config to YAML with a header comment.
func MarshalConfigYAML(cfg *Config) ([]byte, error) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	header := "# MDEMG Sidecar Configuration\n# Generated by: mdemg sidecar init\n# Schema version: " + ConfigVersion + "\n\n"
	return append([]byte(header), data...), nil
}

// ResolveSpaceID derives a space_id from the config's strategy and project directory.
func ResolveSpaceID(cfg *Config, projectDir string) string {
	switch cfg.Hooks.SpaceIDStrategy {
	case "repo-basename":
		return strings.ToLower(filepath.Base(projectDir))
	default:
		return strings.ToLower(filepath.Base(projectDir))
	}
}

// HashConfig returns the SHA-256 hex digest of raw config bytes for drift detection.
func HashConfig(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}
