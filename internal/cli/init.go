package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
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
		SchemaVersion:    18, // Current schema version (V0018 vector 3072)
		BackupEnabled:    true,
		BackupStorageDir: ".mdemg/backups",
		BackupInterval:   24,  // daily partial backups
		BackupRetention:  2,   // keep 2 most recent per type
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
		} else {
			opts.EmbeddingProvider = "openai" // OpenAI is the default; user will be prompted for key
		}
	} else {
		defaultProvider := "openai"
		hint := ""
		if hasOpenAIKey {
			hint = " (OPENAI_API_KEY detected)"
		}
		for {
			opts.EmbeddingProvider = promptLine(
				fmt.Sprintf("Embedding provider (ollama/openai/disabled) [%s]%s", defaultProvider, hint),
				defaultProvider,
			)
			switch opts.EmbeddingProvider {
			case "ollama", "openai", "disabled":
				// valid
			default:
				fmt.Printf("  Invalid provider %q — must be ollama, openai, or disabled.\n", opts.EmbeddingProvider)
				continue
			}
			break
		}
	}

	// Set embedding defaults based on provider
	var openAIKey string
	switch opts.EmbeddingProvider {
	case "ollama":
		if flags.defaults {
			opts.EmbeddingModel = "qwen3-embedding:4b"
			opts.LLMModel = "llama3.2:3b-instruct-fp16"
		} else {
			opts.EmbeddingModel = promptLine("Embedding model [qwen3-embedding:4b]", "qwen3-embedding:4b")
			opts.LLMModel = promptLine("Naming/LLM model [llama3.2:3b-instruct-fp16]", "llama3.2:3b-instruct-fp16")
		}
		opts.EmbeddingEndpoint = "http://localhost:11434"
		opts.LLMProvider = "ollama"
	case "openai":
		if flags.defaults {
			opts.EmbeddingModel = "text-embedding-3-large"
			opts.LLMModel = "gpt-5-nano"
		} else {
			opts.EmbeddingModel = promptLine("Embedding model [text-embedding-3-large]", "text-embedding-3-large")
			opts.LLMModel = promptLine("Naming/LLM model [gpt-5-nano]", "gpt-5-nano")
		}
		opts.LLMProvider = "openai"
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

	// Create .mdemg/ directory and subdirectories
	if err := os.MkdirAll(mdemgDir, 0755); err != nil {
		return fmt.Errorf("create .mdemg/: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(mdemgDir, "backups"), 0755); err != nil {
		return fmt.Errorf("create .mdemg/backups/: %w", err)
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
		if env.hasCursor || env.hasVSCode || env.hasClaude || env.hasRootMCP {
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
				if env.hasRootMCP {
					ides = append(ides, "Claude Code (.mcp.json)")
				} else if env.hasClaude {
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

	// Load .env into current process so spawned daemon inherits secrets
	// Use Overload (not Load) to ensure values are set even if env vars exist as empty
	_ = godotenv.Overload(envPath)

	// Auto-start for --quick mode
	if flags.quick {
		fmt.Println("Starting Neo4j and server (--quick mode)...")
		fmt.Println()
		if err := runDBStart(0, 0, "mdemg-dev"); err != nil {
			fmt.Printf("Warning: Neo4j start failed: %v\n", err)
			fmt.Println("You can start it manually: mdemg db start")
		}
		// Reload config.yaml into env — runDBStart may have updated neo4j.uri with dynamic port
		reloadConfigEnv()
		// Wait for Neo4j bolt protocol to be fully ready (TCP open != bolt ready)
		waitForBoltReady()
		fmt.Println()
		if err := runStart(0, "", true, false, false); err != nil {
			fmt.Printf("Warning: server start failed: %v\n", err)
			fmt.Println("You can start it manually: mdemg start --auto-migrate")
		}
		// Run initial ingest so the graph isn't empty
		runInitialIngest(cwd, opts.SpaceID)
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
			// Reload config.yaml into env — runDBStart may have updated neo4j.uri with dynamic port
			reloadConfigEnv()
			// Wait for Neo4j bolt protocol to be fully ready (TCP open != bolt ready)
			waitForBoltReady()
			fmt.Println()
			if err := runStart(0, "", true, false, false); err != nil {
				fmt.Printf("Warning: server start failed: %v\n", err)
				fmt.Println("You can start it manually: mdemg start --auto-migrate")
			}
			// Run initial ingest so the graph isn't empty
			runInitialIngest(cwd, opts.SpaceID)
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
	neo4jReachable bool
	ollamaReachable bool
	isGitRepo       bool
	hasCursor       bool
	hasVSCode       bool
	hasClaude       bool
	hasRootMCP      bool // .mcp.json exists at repo root (Claude Code project-scoped config)
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

	// Detect existing .mcp.json (Claude Code project-scoped config)
	if _, err := os.Stat(filepath.Join(dir, ".mcp.json")); err == nil {
		env.hasRootMCP = true
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

// mdemgMCPEntry returns the MCP server config block for mdemg.
func mdemgMCPEntry(endpoint string) map[string]interface{} {
	return map[string]interface{}{
		"command": "mdemg",
		"args":    []string{"mcp"},
		"env": map[string]string{
			"MDEMG_ENDPOINT": endpoint,
		},
	}
}

// mergeMCPConfig reads an existing mcp.json, adds/updates the "mdemg" server entry,
// and writes it back. Returns true if the file was modified.
func mergeMCPConfig(mcpPath, endpoint string) (bool, error) {
	var root map[string]interface{}

	data, err := os.ReadFile(mcpPath)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", mcpPath, err)
	}
	if err := json.Unmarshal(data, &root); err != nil {
		return false, fmt.Errorf("parse %s: %w", mcpPath, err)
	}

	servers, ok := root["mcpServers"].(map[string]interface{})
	if !ok {
		servers = make(map[string]interface{})
		root["mcpServers"] = servers
	}

	// Skip if mdemg entry already exists
	if _, exists := servers["mdemg"]; exists {
		return false, nil
	}

	servers["mdemg"] = mdemgMCPEntry(endpoint)

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return false, fmt.Errorf("marshal %s: %w", mcpPath, err)
	}
	out = append(out, '\n')
	if err := os.WriteFile(mcpPath, out, 0644); err != nil {
		return false, fmt.Errorf("write %s: %w", mcpPath, err)
	}
	return true, nil
}

// writeIDEConfigs updates MCP configuration for detected IDEs.
// Prefers merging into existing .mcp.json at repo root (used by Claude Code for
// project-scoped servers) over creating .claude/mcp.json. For Cursor and VS Code,
// creates new files only if they don't already exist; merges if they do.
// Returns a list of files that were written.
func writeIDEConfigs(dir string, port int, env environmentInfo) []string {
	var written []string
	endpoint := fmt.Sprintf("http://localhost:%d", port)

	freshConfig := func() []byte {
		out, _ := json.MarshalIndent(map[string]interface{}{
			"mcpServers": map[string]interface{}{
				"mdemg": mdemgMCPEntry(endpoint),
			},
		}, "", "  ")
		return append(out, '\n')
	}

	// writeOrMerge handles both new-file creation and merging into existing files.
	writeOrMerge := func(mcpPath string) {
		if _, err := os.Stat(mcpPath); os.IsNotExist(err) {
			if err := os.WriteFile(mcpPath, freshConfig(), 0644); err == nil {
				written = append(written, mcpPath)
			}
		} else {
			if merged, err := mergeMCPConfig(mcpPath, endpoint); err != nil {
				fmt.Printf("  Warning: could not merge into %s: %v\n", mcpPath, err)
			} else if merged {
				written = append(written, mcpPath)
			}
		}
	}

	if env.hasCursor {
		writeOrMerge(filepath.Join(dir, ".cursor", "mcp.json"))
	}

	if env.hasVSCode {
		writeOrMerge(filepath.Join(dir, ".vscode", "mcp.json"))
	}

	// Claude Code: prefer .mcp.json at repo root (project-scoped, shared config)
	// over .claude/mcp.json. Claude Code loads .mcp.json for project servers.
	rootMCP := filepath.Join(dir, ".mcp.json")
	if _, err := os.Stat(rootMCP); err == nil {
		// Existing .mcp.json found — merge into it
		writeOrMerge(rootMCP)
	} else if env.hasClaude {
		// No .mcp.json — fall back to .claude/mcp.json
		writeOrMerge(filepath.Join(dir, ".claude", "mcp.json"))
	}

	return written
}

// waitForBoltReady waits until Neo4j bolt protocol is fully ready to serve queries.
// WaitForPort only checks TCP connectivity, but Neo4j may not be ready for bolt queries yet.
func waitForBoltReady() {
	uri := os.Getenv("NEO4J_URI")
	if uri == "" {
		uri = "bolt://localhost:7687"
	}
	pass := os.Getenv("NEO4J_PASS")
	if pass == "" {
		pass = "mdemg-dev" //nolint:gosec // G101: default dev password, not a credential
	}

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		driver, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth("neo4j", pass, ""))
		if err != nil {
			cancel()
			time.Sleep(1 * time.Second)
			continue
		}
		err = driver.VerifyConnectivity(ctx)
		driver.Close(ctx)
		cancel()
		if err == nil {
			return
		}
		time.Sleep(1 * time.Second)
	}
}

// reloadConfigEnv re-reads config.yaml and forces env vars to the current YAML values.
// This is needed after runDBStart which may update neo4j.uri with a dynamic port.
// Without this, the spawned daemon inherits stale env vars (e.g., bolt://localhost:7687
// instead of the actual port like bolt://localhost:7688).
func reloadConfigEnv() {
	cfgPath := config.FindConfigFile()
	if cfgPath == "" {
		return
	}
	// Clear NEO4J_URI so LoadYAMLConfig will set it from the updated config.yaml
	_ = os.Unsetenv("NEO4J_URI")
	_ = config.LoadYAMLConfig(cfgPath)
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

// runInitialIngest performs a full ingest of the codebase after init.
// Uses the provided spaceID (from opts.SpaceID) and leaves sinceCommit empty
// so runIngest performs a full (non-incremental) ingest.
func runInitialIngest(cwd, spaceID string) {
	fmt.Println()
	fmt.Println("Running initial ingest...")
	if err := runIngest(&ingestConfig{
		codebasePath:       cwd,
		spaceID:            spaceID,
		batchSize:          100,
		workers:            4,
		timeout:            300,
		delay:              50,
		maxRetries:         3,
		retryDelay:         2000,
		consolidate:        true,
		extractSymbols:     true,
		includeMd:          true,
		includeTS:          true,
		includePy:          true,
		includeJava:        true,
		includeRust:        true,
		excludeDirs:        ".git,vendor,node_modules,.worktrees",
		archiveDeleted:     true,
		llmSummary:         true,
		llmSummaryModel:    "gpt-4o-mini",
		llmSummaryProvider: "openai",
		llmSummaryBatch:    10,
	}); err != nil {
		fmt.Printf("Warning: initial ingest failed: %v\n", err)
		fmt.Println("You can run it manually: mdemg ingest --path .")
	}
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

