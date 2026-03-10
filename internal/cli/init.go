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
		quick             bool
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
Use --quick for non-interactive setup that also starts Neo4j and the server.

Examples:
  mdemg init                    # Interactive wizard
  mdemg init --defaults         # Non-interactive with defaults
  mdemg init --quick            # Non-interactive + auto-start
  mdemg init --neo4j-uri bolt://db:7687`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if quick {
				defaults = true
			}
			return runInit(initFlags{
				defaults:          defaults,
				quick:             quick,
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
	cmd.Flags().BoolVar(&quick, "quick", false, "Non-interactive setup + auto-start Neo4j and server")
	cmd.Flags().StringVar(&spaceID, "space-id", "", "Override space ID (default: directory name)")
	cmd.Flags().StringVar(&neo4jURI, "neo4j-uri", "", "Override Neo4j URI")
	cmd.Flags().StringVar(&embeddingProvider, "embedding-provider", "", "Override embedding provider (ollama/openai/disabled)")
	cmd.Flags().BoolVar(&noHooks, "no-hooks", false, "Skip git hook installation")
	cmd.Flags().BoolVar(&noIDE, "no-ide", false, "Skip IDE config generation")

	return cmd
}

type initFlags struct {
	defaults          bool
	quick             bool
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

	// Neo4j ports
	if flags.defaults {
		opts.Neo4jBoltPort = 7687
		opts.Neo4jHTTPPort = 7474
	} else {
		boltStr := promptLine("Neo4j bolt port [7687]", "7687")
		if v, err := fmt.Sscanf(boltStr, "%d", &opts.Neo4jBoltPort); v != 1 || err != nil {
			opts.Neo4jBoltPort = 7687
		}
		httpStr := promptLine("Neo4j HTTP port [7474]", "7474")
		if v, err := fmt.Sscanf(httpStr, "%d", &opts.Neo4jHTTPPort); v != 1 || err != nil {
			opts.Neo4jHTTPPort = 7474
		}
	}

	// Server port
	opts.ServerPort = 9999

	// Embedding provider
	hasOpenAIKey := os.Getenv("OPENAI_API_KEY") != ""
	if flags.embeddingProvider != "" {
		opts.EmbeddingProvider = flags.embeddingProvider
	} else if flags.defaults {
		if hasOpenAIKey {
			opts.EmbeddingProvider = "openai"
		} else if env.ollamaReachable {
			opts.EmbeddingProvider = "ollama"
		} else {
			opts.EmbeddingProvider = "disabled"
		}
	} else {
		defaultProvider := "disabled"
		if hasOpenAIKey {
			defaultProvider = "openai"
		} else if env.ollamaReachable {
			defaultProvider = "ollama"
		}
		hint := ""
		if hasOpenAIKey {
			hint = " (OPENAI_API_KEY detected)"
		} else if env.ollamaReachable {
			hint = " (Ollama detected)"
		}
		opts.EmbeddingProvider = promptLine(
			fmt.Sprintf("Embedding provider (ollama/openai/disabled) [%s]%s", defaultProvider, hint),
			defaultProvider,
		)
	}

	// Set embedding defaults based on provider
	var openAIKey string
	switch opts.EmbeddingProvider {
	case "ollama":
		opts.EmbeddingModel = "qwen3-embedding:4b"
		opts.EmbeddingEndpoint = "http://localhost:11434"
		opts.LLMProvider = "ollama"
		opts.LLMModel = "llama3.2:3b-instruct-fp16"
	case "openai":
		opts.EmbeddingModel = "gpt-5-mini"
		opts.LLMProvider = "openai"
		opts.LLMModel = "gpt-5-nano"
		if !flags.defaults && !hasOpenAIKey {
			fmt.Println()
			fmt.Println("  OpenAI requires an API key for embeddings and LLM features.")
			fmt.Println("  The key will be stored in .env (gitignored), NOT in config.yaml.")
			fmt.Println()
			key := promptLine("OpenAI API key (sk-...) [press Enter to skip]", "")
			if key != "" {
				openAIKey = key
			}
		}
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
			if err := InstallGitHook(cwd, opts.SpaceID, false); err != nil {
				fmt.Printf("  Warning: git hook installation failed: %v\n", err)
			} else {
				fmt.Println("  Installed .git/hooks/post-commit")
			}
		}
	}

	// IDE integration
	if !flags.noIDE {
		installIDE := false
		if env.hasCursor || env.hasVSCode || env.hasClaude {
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
				if env.hasClaude {
					ides = append(ides, "Claude Code")
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

	// Create/update .env file with secrets
	envPath := filepath.Join(cwd, ".env")
	envLines := []string{}

	// Read existing .env if present
	if data, err := os.ReadFile(envPath); err == nil {
		envLines = strings.Split(strings.TrimSpace(string(data)), "\n")
	}

	// Add NEO4J_PASS if not already present
	if !envContains(envLines, "NEO4J_PASS") {
		envLines = append(envLines, "NEO4J_PASS=mdemg-dev")
	}

	// Add OPENAI_API_KEY if user provided one and not already present
	if openAIKey != "" && !envContains(envLines, "OPENAI_API_KEY") {
		envLines = append(envLines, fmt.Sprintf("OPENAI_API_KEY=%s", openAIKey))
	}

	if err := os.WriteFile(envPath, []byte(strings.Join(envLines, "\n")+"\n"), 0600); err != nil {
		fmt.Printf("  Warning: could not write .env: %v\n", err)
	} else {
		fmt.Printf("  Updated %s\n", envPath)
	}

	// Print summary
	fmt.Println()
	fmt.Println("Initialization complete!")
	fmt.Println()
	fmt.Println("Config file:   .mdemg/config.yaml")
	fmt.Println("Ignore file:   .mdemgignore")
	fmt.Println("Secrets file:  .env (gitignored)")
	fmt.Printf("Space ID:      %s\n", opts.SpaceID)
	fmt.Println()

	// Auto-start for --quick mode
	if flags.quick {
		fmt.Println("Starting Neo4j and server (--quick mode)...")
		fmt.Println()
		if err := runDBStart(0, 0, "mdemg-dev"); err != nil {
			fmt.Printf("Warning: Neo4j start failed: %v\n", err)
			fmt.Println("You can start it manually: mdemg db start")
		}
		fmt.Println()
		if err := runStart(0, "", true, false, false); err != nil {
			fmt.Printf("Warning: server start failed: %v\n", err)
			fmt.Println("You can start it manually: mdemg start --auto-migrate")
		}
		return nil
	}

	// Post-init auto-start prompt (interactive mode only)
	if !flags.defaults {
		answer := promptLine("Start Neo4j and server now? (yes/no) [yes]", "yes")
		if answer == "yes" {
			fmt.Println()
			if err := runDBStart(0, 0, "mdemg-dev"); err != nil {
				fmt.Printf("Warning: Neo4j start failed: %v\n", err)
				fmt.Println("You can start it manually: mdemg db start")
			}
			fmt.Println()
			if err := runStart(0, "", true, false, false); err != nil {
				fmt.Printf("Warning: server start failed: %v\n", err)
				fmt.Println("You can start it manually: mdemg start --auto-migrate")
			}
			return nil
		}
	}

	// Build "Next steps" for non-auto-start paths
	step := 1
	fmt.Println("Next steps:")
	if openAIKey == "" && opts.EmbeddingProvider == "openai" && !hasOpenAIKey {
		fmt.Printf("  %d. Add your OpenAI API key:  echo 'OPENAI_API_KEY=sk-...' >> .env\n", step)
		step++
	}
	fmt.Printf("  %d. Start Neo4j:            mdemg db start\n", step)
	step++
	fmt.Printf("  %d. Start the server:       mdemg start --auto-migrate\n", step)
	step++
	fmt.Printf("  %d. Ingest your code:       mdemg ingest --path .\n", step)

	return nil
}

// environmentInfo holds detection results.
type environmentInfo struct {
	neo4jReachable  bool
	ollamaReachable bool
	isGitRepo       bool
	hasCursor       bool
	hasVSCode       bool
	hasClaude       bool
}

func detectEnvironment(dir string) environmentInfo {
	env := environmentInfo{}

	// Detect Neo4j
	conn, err := net.DialTimeout("tcp", "localhost:7687", 2*time.Second)
	if err == nil {
		_ = conn.Close()
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
	if _, err := os.Stat(filepath.Join(dir, ".claude")); err == nil {
		env.hasClaude = true
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

	if env.hasClaude {
		mcpPath := filepath.Join(dir, ".claude", "mcp.json")
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

// envContains checks whether a .env line slice already has a key defined.
func envContains(lines []string, key string) bool {
	prefix := key + "="
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			return true
		}
	}
	return false
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

