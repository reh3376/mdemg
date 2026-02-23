package cli

import (
	"github.com/joho/godotenv"
	"mdemg/internal/config"
)

// loadConfig loads the full MDEMG configuration using the layered priority:
//  1. Defaults (in FromEnv)
//  2. .mdemg/config.yaml (sets env vars for unset keys)
//  3. .env file (godotenv)
//  4. Environment variables (already set)
//  5. CLI flags (applied after this function returns)
func loadConfig() (config.Config, error) {
	// Layer 2: YAML config (only sets env vars not already present)
	if cfgPath := config.FindConfigFile(); cfgPath != "" {
		_ = config.LoadYAMLConfig(cfgPath)
	}

	// Layer 3: .env file
	_ = godotenv.Load()

	// Layer 1+4: Defaults + env vars
	return config.FromEnv()
}
