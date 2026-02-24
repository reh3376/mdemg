package cli

import (
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
	return config.FromEnv()
}
