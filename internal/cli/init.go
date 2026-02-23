package cli

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"mdemg/internal/config"
)

func newInitCmd() *cobra.Command {
	var (
		defaults          bool
		spaceID           string
		neo4jURI          string
		embeddingProvider string
		noHooks           bool
		noIDE             bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new MDEMG project",
		Long: `Initialize MDEMG in the current directory.

Creates a .mdemg/ directory with config.yaml and a .mdemgignore file.
By default, runs an interactive wizard to detect your environment and
guide you through configuration.

Use --defaults for non-interactive setup with sensible defaults.

Examples:
  mdemg init                    # Interactive wizard
  mdemg init --defaults         # Non-interactive with defaults
  mdemg init --neo4j-uri bolt://db:7687`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(initFlags{
				defaults:          defaults,
				spaceID:           spaceID,
				neo4jURI:          neo4jURI,
				embeddingProvider: embeddingProvider,
				noHooks:           noHooks,
				noIDE:             noIDE,
			})
		},
	}

	cmd.Flags().BoolVar(&defaults, "defaults", false, "Non-interactive mode with sensible defaults")
	cmd.Flags().BoolVar(&defaults, "yes", false, "Alias for --defaults")
	cmd.Flags().StringVar(&spaceID, "space-id", "", "Override space ID (default: directory name)")
	cmd.Flags().StringVar(&neo4jURI, "neo4j-uri", "", "Override Neo4j URI")
	cmd.Flags().StringVar(&embeddingProvider, "embedding-provider", "", "Override embedding provider (ollama/openai/disabled)")
	cmd.Flags().BoolVar(&noHooks, "no-hooks", false, "Skip git hook installation")
	cmd.Flags().BoolVar(&noIDE, "no-ide", false, "Skip IDE config generation")

	return cmd
}

type initFlags struct {
	defaults          bool
	spaceID           string
	neo4jURI          string
	embeddingProvider string
	noHooks           bool
	noIDE             bool
}

func runInit(flags initFlags) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	// Check for existing .mdemg/
	mdemgDir := filepath.Join(cwd, ".mdemg")
	if _, err := os.Stat(mdemgDir); err == nil {
		if !flags.defaults {
			fmt.Println("Found existing .mdemg/ directory.")
			answer := promptLine("Reconfigure? (yes/no)", "no")
			if answer != "yes" {
				fmt.Println("Aborted.")
				return nil
			}
		}
	}

	fmt.Println("MDEMG Project Initialization")
	fmt.Println("============================")
	fmt.Println()

	// Detect environment
	env := detectEnvironment(cwd)

	// Build options from flags + detection + wizard
	opts := config.InitOptions{
		SchemaVersion: 17, // Current schema version
	}

	// Space ID
	if flags.spaceID != "" {
		opts.SpaceID = flags.spaceID
	} else if flags.defaults {
		opts.SpaceID = filepath.Base(cwd)
	} else {
		defaultSpace := filepath.Base(cwd)
		opts.SpaceID = promptLine(fmt.Sprintf("Space ID [%s]", defaultSpace), defaultSpace)
	}

	// Neo4j URI
	if flags.neo4jURI != "" {
		opts.Neo4jURI = flags.neo4jURI
	} else if flags.defaults {
		opts.Neo4jURI = "bolt://localhost:7687"
	} else {
		defaultURI := "bolt://localhost:7687"
		hint := ""
		if env.neo4jReachable {
			hint = " (detected)"
		}
		opts.Neo4jURI = promptLine(fmt.Sprintf("Neo4j URI [%s]%s", defaultURI, hint), defaultURI)
	}

	// Neo4j credentials
	if flags.defaults {
		opts.Neo4jUser = "neo4j"
		opts.Neo4jPassword = "" // stays in .env
	} else {
		opts.Neo4jUser = promptLine("Neo4j user [neo4j]", "neo4j")
		// Password is NOT prompted — must go in .env for security
	}

	// Server port
	opts.ServerPort = 9999

	// Embedding provider
	if flags.embeddingProvider != "" {
		opts.EmbeddingProvider = flags.embeddingProvider
	} else if flags.defaults {
		if env.ollamaReachable {
			opts.EmbeddingProvider = "ollama"
		} else {
			opts.EmbeddingProvider = "disabled"
		}
	} else {
		defaultProvider := "disabled"
		if env.ollamaReachable {
			defaultProvider = "ollama"
		}
		hint := ""
		if env.ollamaReachable {
			hint = " (detected)"
		}
		opts.EmbeddingProvider = promptLine(
			fmt.Sprintf("Embedding provider (ollama/openai/disabled) [%s]%s", defaultProvider, hint),
			defaultProvider,
		)
	}

	// Set embedding defaults based on provider
	switch opts.EmbeddingProvider {
	case "ollama":
		opts.EmbeddingModel = "nomic-embed-text"
		opts.EmbeddingEndpoint = "http://localhost:11434"
	case "openai":
		opts.EmbeddingModel = "text-embedding-ada-002"
	}

	// Generate files
	fmt.Println()

	// Create .mdemg/ directory
	if err := os.MkdirAll(mdemgDir, 0755); err != nil {
		return fmt.Errorf("create .mdemg/: %w", err)
	}

	// Generate config.yaml
	configData, err := config.GenerateConfigYAML(opts)
	if err != nil {
		return fmt.Errorf("generate config: %w", err)
	}
	configPath := filepath.Join(mdemgDir, "config.yaml")
	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	fmt.Printf("  Created %s\n", configPath)

	// Create .gitkeep
	gitkeepPath := filepath.Join(mdemgDir, ".gitkeep")
	if err := os.WriteFile(gitkeepPath, []byte{}, 0644); err != nil {
		return fmt.Errorf("write .gitkeep: %w", err)
	}

	// Generate .mdemgignore
	ignoreData, err := config.GenerateIgnoreFile(cwd)
	if err != nil {
		return fmt.Errorf("generate ignore file: %w", err)
	}
	ignorePath := filepath.Join(cwd, ".mdemgignore")
	if err := os.WriteFile(ignorePath, ignoreData, 0644); err != nil {
		return fmt.Errorf("write .mdemgignore: %w", err)
	}
	fmt.Printf("  Created %s\n", ignorePath)

	// Git hooks
	if !flags.noHooks && env.isGitRepo {
		installHook := flags.defaults
		if !flags.defaults {
			answer := promptLine("Install git post-commit hook for auto-ingestion? (yes/no) [yes]", "yes")
			installHook = answer == "yes"
		}
		if installHook {
			if err := installGitHook(cwd, opts.SpaceID); err != nil {
				fmt.Printf("  Warning: git hook installation failed: %v\n", err)
			} else {
				fmt.Println("  Installed .git/hooks/post-commit")
			}
		}
	}

	// IDE integration
	if !flags.noIDE {
		installIDE := false
		if env.hasCursor || env.hasVSCode {
			if flags.defaults {
				installIDE = true
			} else {
				ides := []string{}
				if env.hasCursor {
					ides = append(ides, "Cursor")
				}
				if env.hasVSCode {
					ides = append(ides, "VS Code")
				}
				answer := promptLine(
					fmt.Sprintf("Configure MCP for %s? (yes/no) [yes]", strings.Join(ides, ", ")),
					"yes",
				)
				installIDE = answer == "yes"
			}
		}
		if installIDE {
			written := writeIDEConfigs(cwd, opts.ServerPort, env)
			for _, f := range written {
				fmt.Printf("  Updated %s\n", f)
			}
		}
	}

	// Print summary
	fmt.Println()
	fmt.Println("Initialization complete!")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  1. Set Neo4j password:  echo 'NEO4J_PASS=yourpassword' >> .env\n")
	fmt.Printf("  2. Start the server:    mdemg serve\n")
	fmt.Printf("  3. Ingest your code:    mdemg ingest --path .\n")
	fmt.Println()
	fmt.Println("Config file:   .mdemg/config.yaml")
	fmt.Println("Ignore file:   .mdemgignore")
	fmt.Printf("Space ID:      %s\n", opts.SpaceID)

	return nil
}

// environmentInfo holds detection results.
type environmentInfo struct {
	neo4jReachable bool
	ollamaReachable bool
	isGitRepo      bool
	hasCursor      bool
	hasVSCode      bool
}

func detectEnvironment(dir string) environmentInfo {
	env := environmentInfo{}

	// Detect Neo4j
	conn, err := net.DialTimeout("tcp", "localhost:7687", 2*time.Second)
	if err == nil {
		conn.Close()
		env.neo4jReachable = true
	}

	// Detect Ollama
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:11434/api/tags")
	if err == nil {
		resp.Body.Close()
		env.ollamaReachable = resp.StatusCode == http.StatusOK
	}

	// Detect Git
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		env.isGitRepo = true
	}

	// Detect IDEs
	if _, err := os.Stat(filepath.Join(dir, ".cursor")); err == nil {
		env.hasCursor = true
	}
	if _, err := os.Stat(filepath.Join(dir, ".vscode")); err == nil {
		env.hasVSCode = true
	}

	return env
}

func promptLine(prompt, defaultVal string) string {
	fmt.Print(prompt + ": ")
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text != "" {
			return text
		}
	}
	return defaultVal
}

// installGitHook creates a minimal post-commit hook that calls mdemg ingest.
func installGitHook(repoDir, spaceID string) error {
	hookDir := filepath.Join(repoDir, ".git", "hooks")
	hookPath := filepath.Join(hookDir, "post-commit")

	// Don't overwrite existing hooks
	if _, err := os.Stat(hookPath); err == nil {
		return fmt.Errorf("post-commit hook already exists (use scripts/install-git-hook --force to overwrite)")
	}

	hook := fmt.Sprintf(`#!/bin/bash
# MDEMG auto-ingestion hook (installed by mdemg init)
# Set MDEMG_DISABLED=true to temporarily disable

if [ "$MDEMG_DISABLED" = "true" ]; then exit 0; fi

REPO_ROOT=$(git rev-parse --show-toplevel)
SPACE_ID="${MDEMG_SPACE_ID:-%s}"

# Find mdemg binary
if command -v mdemg &> /dev/null; then
    MDEMG_BIN="mdemg"
elif [ -f "$REPO_ROOT/bin/mdemg" ]; then
    MDEMG_BIN="$REPO_ROOT/bin/mdemg"
else
    exit 0  # silently skip if mdemg not found
fi

# Run incremental ingestion in background
nohup "$MDEMG_BIN" ingest \
    --path "$REPO_ROOT" \
    --space-id "$SPACE_ID" \
    --incremental \
    --since "HEAD~1" \
    --archive-deleted \
    --quiet \
    > /dev/null 2>&1 &
`, spaceID)

	if err := os.MkdirAll(hookDir, 0755); err != nil {
		return err
	}
	return os.WriteFile(hookPath, []byte(hook), 0755)
}

// writeIDEConfigs updates MCP configuration for detected IDEs.
// Returns a list of files that were written.
func writeIDEConfigs(dir string, port int, env environmentInfo) []string {
	var written []string
	endpoint := fmt.Sprintf("http://localhost:%d", port)

	mcpConfig := fmt.Sprintf(`{
  "mcpServers": {
    "mdemg": {
      "command": "mdemg",
      "args": ["mcp"],
      "env": {
        "MDEMG_ENDPOINT": "%s"
      }
    }
  }
}
`, endpoint)

	if env.hasCursor {
		mcpPath := filepath.Join(dir, ".cursor", "mcp.json")
		// Only write if it doesn't exist (don't clobber user config)
		if _, err := os.Stat(mcpPath); os.IsNotExist(err) {
			if err := os.WriteFile(mcpPath, []byte(mcpConfig), 0644); err == nil {
				written = append(written, mcpPath)
			}
		}
	}

	if env.hasVSCode {
		mcpPath := filepath.Join(dir, ".vscode", "mcp.json")
		if _, err := os.Stat(mcpPath); os.IsNotExist(err) {
			if err := os.WriteFile(mcpPath, []byte(mcpConfig), 0644); err == nil {
				written = append(written, mcpPath)
			}
		}
	}

	return written
}

// ParseIgnoreFile reads a .mdemgignore file and returns the list of patterns.
func ParseIgnoreFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var patterns []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns, nil
}

// FindIgnoreFile searches for .mdemgignore walking up from the given directory.
func FindIgnoreFile(startDir string) string {
	dir := startDir
	for {
		candidate := filepath.Join(dir, ".mdemgignore")
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

