package cli

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/joho/godotenv"
	"mdemg/internal/config"
	"mdemg/internal/secrets"
)

// loadYAMLConfigOrWarn loads a DISCOVERED yaml config file, warning loudly
// on failure instead of silently proceeding with env/defaults
// (CONFIG-DEADFLAG-001: nine call sites blank-assigned the error, so a
// malformed .mdemg/config.yaml — a typo'd key, broken indent — degraded
// every command to defaults with zero operator signal). The file is known
// to exist (FindConfigFile found it), so any error here is real. Warn
// rather than hard-fail: a corrupt yaml must not brick every CLI command,
// but it must be visible.
func loadYAMLConfigOrWarn(cfgPath string) {
	if err := config.LoadYAMLConfig(cfgPath); err != nil {
		fmt.Fprintf(os.Stderr, "WARN: config file %s could not be loaded (continuing with env/defaults): %v\n", cfgPath, err)
	}
}

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
		loadYAMLConfigOrWarn(cfgPath)
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

	// Inject build-time version/commit when not overridden by MDEMG_VERSION/MDEMG_COMMIT.
	// config.FromEnv defaults these to "" so cli's ldflags-injected values win unless the
	// operator sets the env var explicitly. /healthz and /readyz read from cfg.Mdemg{Version,Commit}.
	if cfg.MdemgVersion == "" {
		cfg.MdemgVersion = Version
	}
	if cfg.MdemgCommit == "" {
		cfg.MdemgCommit = Commit
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
