package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"mdemg/internal/config"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage MDEMG configuration",
		Long:  "View and validate MDEMG project configuration.",
	}

	cmd.AddCommand(newConfigShowCmd())
	cmd.AddCommand(newConfigValidateCmd())
	cmd.AddCommand(newConfigSetSecretCmd())
	cmd.AddCommand(newConfigGetSecretCmd())
	cmd.AddCommand(newConfigListSecretsCmd())

	return cmd
}

// --- config show ---

func newConfigShowCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Display effective configuration with sources",
		Long: `Show the effective MDEMG configuration.

For each setting, displays the value and where it came from:
  yaml    - from .mdemg/config.yaml
  env     - from environment variable or .env file
  default - built-in default value

Secrets are masked with **** in the output.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigShow(jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")

	return cmd
}

func runConfigShow(jsonOutput bool) error {
	// Load config layers (YAML → keychain → .env) so env vars are populated
	// before building the effective config. Without this, os.Getenv calls
	// miss values that only exist in .env when run as a subprocess.
	_, _ = loadConfig()

	yamlPath := config.FindConfigFile()

	entries := config.EffectiveConfig(yamlPath)

	if jsonOutput {
		data, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal json: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	// Group by section
	if yamlPath != "" {
		fmt.Printf("Config file: %s\n\n", yamlPath)
	} else {
		fmt.Println("Config file: (none found)")
		fmt.Println()
	}

	fmt.Printf("%-30s %-30s %s\n", "SETTING", "VALUE", "SOURCE")
	fmt.Printf("%-30s %-30s %s\n", "-------", "-----", "------")

	for _, e := range entries {
		val := e.Value
		if val == "" {
			val = "(not set)"
		}
		fmt.Printf("%-30s %-30s %s\n", e.Key, val, e.Source)
	}

	return nil
}

// --- config validate ---

func newConfigValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Check configuration validity",
		Long: `Validate the MDEMG configuration file and test connectivity.

Checks:
  - .mdemg/config.yaml syntax and field values
  - Neo4j reachability (if configured)
  - Embedding provider reachability (if configured)

Exit code 0 = valid, 1 = errors found.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigValidate()
		},
	}

	return cmd
}

func runConfigValidate() error {
	fmt.Println("MDEMG Configuration Validation")
	fmt.Println("==============================")
	fmt.Println()

	// Load config layers (YAML → keychain → .env) so env vars are populated
	// before connectivity checks. Without this, os.Getenv calls below would
	// miss values that only exist in config.yaml or .env.
	_, _ = loadConfig()

	hasErrors := false

	// Check for config file
	yamlPath := config.FindConfigFile()
	if yamlPath != "" {
		fmt.Printf("Config file: %s\n", yamlPath)

		errs := config.ValidateConfigFile(yamlPath)
		if len(errs) == 0 {
			fmt.Println("  YAML syntax:    OK")
			fmt.Println("  Field values:   OK")
		} else {
			for _, e := range errs {
				prefix := "  WARNING"
				if e.Level == "error" {
					prefix = "  ERROR"
					hasErrors = true
				}
				fmt.Printf("%s: %s — %s\n", prefix, e.Field, e.Message)
			}
		}
	} else {
		fmt.Println("Config file: (none found)")
		fmt.Println("  Tip: Run 'mdemg init' to create one")
	}
	fmt.Println()

	// CONFIG-VALIDATE-TRANSIENT-DISTINGUISH-001 (2026-08-05): distinguish
	// "containers not started yet" (transient — next step is `docker compose
	// up -d`, config is fine) from "container up but service broken" (real
	// error). Pre-sprint, a fresh `mdemg init` immediately followed by
	// `mdemg config validate` reported FAILED — beta-tester reads that as
	// "MDEMG is broken" when the actual state is "user hasn't started the
	// services yet." Now: NOT STARTED = PASSED with next-step hint; real
	// UNREACHABLE = FAILED.
	composeUp := composeStackRunning()
	var hasTransient bool

	// Test Neo4j connectivity
	neo4jURI := os.Getenv("NEO4J_URI")
	if neo4jURI == "" {
		fmt.Println("Neo4j:    (not configured)")
	} else {
		fmt.Printf("Neo4j:    %s ... ", neo4jURI)
		if testNeo4jReachable(neo4jURI) {
			fmt.Println("OK")
		} else if !composeUp {
			fmt.Println("NOT STARTED")
			hasTransient = true
		} else {
			fmt.Println("UNREACHABLE (containers up but Bolt port not responding)")
			hasErrors = true
		}
	}

	// Test embedding provider
	provider := os.Getenv("EMBEDDING_PROVIDER")
	switch provider {
	case "", "disabled":
		fmt.Println("Embedding: (disabled)")
	case "ollama":
		endpoint := os.Getenv("OLLAMA_ENDPOINT")
		if endpoint == "" {
			endpoint = "http://localhost:11434"
		}
		fmt.Printf("Embedding: ollama at %s ... ", endpoint)
		if testHTTPReachable(endpoint + "/api/tags") {
			fmt.Println("OK")
		} else {
			// Ollama runs OUTSIDE the mdemg docker stack (native install)
			// so composeUp doesn't gate this transient state. Still, treat
			// the "not running yet" case as transient rather than a config
			// error: the operator's next step is `ollama serve &`, not "fix
			// your config."
			fmt.Println("NOT STARTED (run: ollama serve &)")
			hasTransient = true
		}
	case "openai":
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey != "" {
			fmt.Println("Embedding: openai (API key set)")
		} else {
			fmt.Println("Embedding: openai (WARNING: OPENAI_API_KEY not set)")
			// Not an error (the config is valid; the operator just hasn't
			// exported the key yet). Warning already surfaced above.
		}
	default:
		fmt.Printf("Embedding: unknown provider %q\n", provider)
		hasErrors = true
	}

	fmt.Println()

	if hasErrors {
		fmt.Println("Validation: FAILED (errors found)")
		os.Exit(1)
	}

	if hasTransient {
		fmt.Println("Validation: PASSED (services not started — run: docker compose up -d)")
		return nil
	}

	fmt.Println("Validation: PASSED")
	return nil
}

// composeStackRunning returns true when at least one Docker Compose service
// under the current project has a running container. Used to distinguish
// "containers not up yet" (transient — reachable=NO but ok) from "containers
// up but service is broken" (real error). Never blocks: bounded 2-second
// timeout, and any failure (docker not installed, permission denied, etc.)
// returns false — which flips the caller into the "not started" branch, which
// is the safe default when we can't tell.
func composeStackRunning() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// `docker compose ps -q` returns container IDs (one per line) for every
	// service in the current project that has a container. Empty output = no
	// containers = stack not up.
	cmd := exec.CommandContext(ctx, "docker", "compose", "ps", "-q")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(out))) > 0
}

// testNeo4jReachable tests TCP connectivity to the Neo4j bolt port.
func testNeo4jReachable(uri string) bool {
	// Extract host:port from bolt://host:port
	host := uri
	for _, prefix := range []string{"bolt://", "neo4j://", "neo4j+s://"} {
		host = trimPrefix(host, prefix)
	}
	conn, err := net.DialTimeout("tcp", host, 3*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// trimPrefix removes prefix if present (strings.TrimPrefix but local to avoid import collisions).
func trimPrefix(s, prefix string) string {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}

// testHTTPReachable tests HTTP connectivity to a URL.
func testHTTPReachable(url string) bool {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}
