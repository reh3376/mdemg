package cli

import (
	"log/slog"
	"os"

	"github.com/joho/godotenv"
	"mdemg/internal/config"
	"mdemg/internal/secrets"
)

// loadConfig loads the full MDEMG configuration using the layered priority:
//  1. Defaults (in FromEnv)
//  2. .mdemg/config.yaml (sets env vars for unset keys)
//  3. System keychain secrets (opportunistic)
//  4. .env file (godotenv)
//  5. Environment variables (already set)
//  6. CLI flags (applied after this function returns)
func loadConfig() (config.Config, error) {
	// Layer 2: YAML config (only sets env vars not already present)
	if cfgPath := config.FindConfigFile(); cfgPath != "" {
		_ = config.LoadYAMLConfig(cfgPath)
	}

	// Layer 2.5: Keychain secrets (opportunistic — errors silently ignored)
	secrets.ResolveSecrets()

	// Layer 3: .env file
	_ = godotenv.Load()

	// Layer 1+4: Defaults + env vars
	cfg, err := config.FromEnv()
	if err != nil {
		return cfg, err
	}

	// Auto-detect project-scoped Neo4j container name for backups.
	// If still the default "mdemg-neo4j", override with the project-scoped name
	// so backup full commands target the correct container.
	if cfg.BackupNeo4jContainer == "mdemg-neo4j" {
		if cwd, cwdErr := os.Getwd(); cwdErr == nil {
			cfg.BackupNeo4jContainer = ContainerNameForProject(cwd)
		}
	}

	// Cross-field validation (warn-not-error by default)
	if warnings, vErr := cfg.Validate(); vErr != nil {
		return cfg, vErr
	} else {
		for _, w := range warnings {
			slog.Warn("config validation", "warning", w)
		}
	}

	return cfg, nil
}
