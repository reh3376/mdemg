package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/spf13/cobra"

	"mdemg/internal/plugins"
)

// newPluginCmd creates the plugin command group
func newPluginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage MDEMG plugins",
		Long:  `Commands for creating, validating, and managing MDEMG plugins.`,
	}

	cmd.AddCommand(newPluginScaffoldCmd())
	cmd.AddCommand(newPluginValidateCmd())

	return cmd
}

// ============ Plugin Scaffold Command ============

// moduleType represents the type of MDEMG module
type moduleType string

const (
	moduleTypeIngestion moduleType = "INGESTION"
	moduleTypeReasoning moduleType = "REASONING"
	moduleTypeAPE       moduleType = "APE"
)

// pluginScaffoldConfig holds the configuration for scaffold generation
type pluginScaffoldConfig struct {
	Name       string
	Type       moduleType
	OutputDir  string
	Version    string
	ModuleID   string // Lowercase, hyphenated version of Name
	StructName string // PascalCase version for Go structs
}

func newPluginScaffoldCmd() *cobra.Command {
	var (
		name      string
		pluginType string
		outputDir string
		version   string
	)

	cmd := &cobra.Command{
		Use:   "scaffold",
		Short: "Generate a new plugin scaffold",
		Long: `Generate a complete plugin scaffold with manifest, handler, and build files.

This command creates all necessary files for a new MDEMG plugin including:
- manifest.json (plugin metadata)
- main.go (plugin entrypoint)
- handler.go (business logic)
- Makefile (build automation)
- README.md (documentation)`,
		Example: `  # Create an INGESTION plugin
  mdemg plugin scaffold --name="My Parser" --type=INGESTION

  # Create a REASONING plugin
  mdemg plugin scaffold --name="Custom Ranker" --type=REASONING --output=./my-plugins

  # Create an APE plugin with specific version
  mdemg plugin scaffold --name="Background Task" --type=APE --version=2.0.0`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}

			if pluginType == "" {
				return fmt.Errorf("--type is required")
			}

			mt := moduleType(strings.ToUpper(pluginType))
			if mt != moduleTypeIngestion && mt != moduleTypeReasoning && mt != moduleTypeAPE {
				return fmt.Errorf("--type must be one of: INGESTION, REASONING, APE (got: %s)", pluginType)
			}

			cfg := pluginScaffoldConfig{
				Name:       name,
				Type:       mt,
				OutputDir:  outputDir,
				Version:    version,
				ModuleID:   toModuleID(name),
				StructName: toStructName(name),
			}

			if err := generatePlugin(cfg); err != nil {
				return fmt.Errorf("failed to generate plugin: %w", err)
			}

			fmt.Printf("Successfully generated plugin scaffold at %s/%s\n", cfg.OutputDir, cfg.ModuleID)
			fmt.Println("\nNext steps:")
			fmt.Println("  1. cd", filepath.Join(cfg.OutputDir, cfg.ModuleID))
			fmt.Println("  2. Review and customize handler.go with your logic")
			fmt.Println("  3. Run 'make build' to compile the plugin")
			fmt.Println("  4. Restart MDEMG server to auto-discover the plugin")

			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Plugin name (required)")
	cmd.Flags().StringVar(&pluginType, "type", "", "Module type: INGESTION, REASONING, or APE (required)")
	cmd.Flags().StringVar(&outputDir, "output", "./plugins", "Output directory for the plugin")
	cmd.Flags().StringVar(&version, "version", "1.0.0", "Initial version for the plugin")

	cmd.MarkFlagRequired("name")
	cmd.MarkFlagRequired("type")

	return cmd
}

// toModuleID converts a plugin name to a lowercase, hyphenated module ID
func toModuleID(name string) string {
	// Replace spaces and underscores with hyphens
	result := strings.ReplaceAll(name, " ", "-")
	result = strings.ReplaceAll(result, "_", "-")
	// Convert to lowercase
	result = strings.ToLower(result)
	// Remove any consecutive hyphens
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	// Trim leading/trailing hyphens
	result = strings.Trim(result, "-")
	return result
}

// toStructName converts a plugin name to a PascalCase struct name
func toStructName(name string) string {
	// Split on hyphens, underscores, and spaces
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	words := strings.Fields(name)

	var result strings.Builder
	for _, word := range words {
		if len(word) > 0 {
			result.WriteString(strings.ToUpper(word[:1]))
			if len(word) > 1 {
				result.WriteString(strings.ToLower(word[1:]))
			}
		}
	}

	// Ensure it starts with a letter
	s := result.String()
	if len(s) == 0 {
		return "Plugin"
	}
	return s
}

// generatePlugin creates all scaffold files for the plugin
func generatePlugin(cfg pluginScaffoldConfig) error {
	pluginDir := filepath.Join(cfg.OutputDir, cfg.ModuleID)

	// Create plugin directory
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return fmt.Errorf("failed to create plugin directory: %w", err)
	}

	// Generate each file
	files := []struct {
		name     string
		template string
		perm     os.FileMode
	}{
		{"manifest.json", getManifestTemplate(cfg.Type), 0644},
		{"main.go", mainGoTemplate, 0644},
		{"handler.go", getHandlerTemplate(cfg.Type), 0644},
		{"Makefile", makefileTemplate, 0644},
		{"README.md", readmeTemplate, 0644},
	}

	for _, f := range files {
		path := filepath.Join(pluginDir, f.name)
		content, err := executeTemplate(f.name, f.template, cfg)
		if err != nil {
			return fmt.Errorf("failed to generate %s: %w", f.name, err)
		}
		if err := os.WriteFile(path, []byte(content), f.perm); err != nil {
			return fmt.Errorf("failed to write %s: %w", f.name, err)
		}
	}

	return nil
}

// executeTemplate renders a template with the given config
func executeTemplate(name, tmplStr string, cfg pluginScaffoldConfig) (string, error) {
	tmpl, err := template.New(name).Parse(tmplStr)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, cfg); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// getManifestTemplate returns the manifest.json template for the given module type
func getManifestTemplate(mt moduleType) string {
	switch mt {
	case moduleTypeIngestion:
		return manifestIngestionTemplate
	case moduleTypeReasoning:
		return manifestReasoningTemplate
	case moduleTypeAPE:
		return manifestAPETemplate
	default:
		return manifestIngestionTemplate
	}
}

// getHandlerTemplate returns the handler.go template for the given module type
func getHandlerTemplate(mt moduleType) string {
	switch mt {
	case moduleTypeIngestion:
		return handlerIngestionTemplate
	case moduleTypeReasoning:
		return handlerReasoningTemplate
	case moduleTypeAPE:
		return handlerAPETemplate
	default:
		return handlerIngestionTemplate
	}
}

// ============ Plugin Validate Command ============

// Color codes for terminal output
var (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorBold   = "\033[1m"
)

func newPluginValidateCmd() *cobra.Command {
	var (
		pluginPath    string
		socketPath    string
		manifestOnly  bool
		protoOnly     bool
		healthOnly    bool
		lifecycleOnly bool
		jsonOutput    bool
		verbose       bool
		noColor       bool
	)

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a plugin for correctness",
		Long: `Validate a plugin's manifest, proto compliance, health, and lifecycle.

This command performs comprehensive validation including:
- Manifest structure and required fields
- gRPC proto service implementation
- Health check responsiveness
- Complete lifecycle (handshake, health, shutdown)`,
		Example: `  # Validate all aspects of a plugin
  mdemg plugin validate --plugin=./plugins/my-plugin

  # Validate only the manifest
  mdemg plugin validate --plugin=./plugins/my-plugin --manifest-only

  # Validate a running plugin's health
  mdemg plugin validate --socket=/var/run/mdemg/mdemg-my-plugin.sock --health-only

  # Output results as JSON
  mdemg plugin validate --plugin=./plugins/my-plugin --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Disable colors if requested or if not a terminal
			if noColor || os.Getenv("NO_COLOR") != "" || !isTerminal() {
				disableColors()
			}

			// Validate flags
			if pluginPath == "" && socketPath == "" {
				return fmt.Errorf("--plugin or --socket is required")
			}

			if (healthOnly || lifecycleOnly) && socketPath == "" {
				return fmt.Errorf("--socket is required for health or lifecycle validation")
			}

			// Resolve paths
			if pluginPath != "" {
				absPath, err := filepath.Abs(pluginPath)
				if err != nil {
					return fmt.Errorf("failed to resolve path: %w", err)
				}
				pluginPath = absPath
			}

			// Run validations
			var exitCode int
			result := &plugins.PluginValidation{
				PluginPath: pluginPath,
				Valid:      true,
			}

			if socketPath != "" {
				result.PluginPath = socketPath
			}

			// Manifest validation
			if pluginPath != "" && !protoOnly && !healthOnly && !lifecycleOnly {
				if !jsonOutput {
					printSection("Manifest Validation")
				}

				manifestResult, err := plugins.ValidateManifest(pluginPath)
				if err != nil {
					if jsonOutput {
						outputJSONError(err)
					} else {
						return fmt.Errorf("manifest validation failed: %w", err)
					}
					os.Exit(1)
				}

				result.Manifest = manifestResult
				if !manifestResult.Valid {
					result.Valid = false
				}

				if !jsonOutput {
					printManifestResult(manifestResult, verbose)
				}

				if manifestOnly {
					if jsonOutput {
						outputJSON(result)
					}
					if !result.Valid {
						exitCode = 1
					}
					os.Exit(exitCode)
				}
			}

			// Proto compliance validation
			if pluginPath != "" && !manifestOnly && !healthOnly && !lifecycleOnly {
				if result.Manifest == nil || result.Manifest.Manifest == nil {
					// Need to read manifest first
					manifestResult, err := plugins.ValidateManifest(pluginPath)
					if err != nil || !manifestResult.Valid {
						if !jsonOutput {
							return fmt.Errorf("cannot validate proto compliance without valid manifest")
						}
						os.Exit(1)
					}
					result.Manifest = manifestResult
				}

				if !jsonOutput {
					printSection("Proto Compliance Validation")
				}

				binaryPath := filepath.Join(pluginPath, result.Manifest.Manifest.Binary)
				protoResult, err := plugins.ValidateProtoCompliance(binaryPath, result.Manifest.Manifest.Type)
				if err != nil {
					if jsonOutput {
						outputJSONError(err)
					} else {
						return fmt.Errorf("proto validation failed: %w", err)
					}
					os.Exit(1)
				}

				result.Proto = protoResult
				if !protoResult.Valid {
					result.Valid = false
				}

				if !jsonOutput {
					printProtoResult(protoResult, verbose)
				}

				if protoOnly {
					if jsonOutput {
						outputJSON(result)
					}
					if !result.Valid {
						exitCode = 1
					}
					os.Exit(exitCode)
				}
			}

			// Health check validation
			if socketPath != "" && (healthOnly || (!manifestOnly && !protoOnly && !lifecycleOnly)) {
				if !jsonOutput {
					printSection("Health Check Validation")
				}

				healthResult, err := plugins.ValidateHealthCheck(socketPath)
				if err != nil {
					if jsonOutput {
						outputJSONError(err)
					} else {
						return fmt.Errorf("health check validation failed: %w", err)
					}
					os.Exit(1)
				}

				result.Health = healthResult
				if !healthResult.Valid {
					result.Valid = false
				}

				if !jsonOutput {
					printHealthResult(healthResult, verbose)
				}

				if healthOnly {
					if jsonOutput {
						outputJSON(result)
					}
					if !result.Valid {
						exitCode = 1
					}
					os.Exit(exitCode)
				}
			}

			// Lifecycle validation
			if socketPath != "" && (lifecycleOnly || (!manifestOnly && !protoOnly && !healthOnly)) {
				if !jsonOutput {
					printSection("Lifecycle Validation")
				}

				lifecycleResult, err := plugins.ValidateLifecycle(socketPath)
				if err != nil {
					if jsonOutput {
						outputJSONError(err)
					} else {
						return fmt.Errorf("lifecycle validation failed: %w", err)
					}
					os.Exit(1)
				}

				result.Lifecycle = lifecycleResult
				if !lifecycleResult.Valid {
					result.Valid = false
				}

				if !jsonOutput {
					printLifecycleResult(lifecycleResult, verbose)
				}
			}

			// Output final results
			if jsonOutput {
				outputJSON(result)
			} else {
				printSummary(result)
			}

			if !result.Valid {
				exitCode = 1
			}
			os.Exit(exitCode)

			return nil
		},
	}

	cmd.Flags().StringVar(&pluginPath, "plugin", "", "Path to plugin directory")
	cmd.Flags().StringVar(&socketPath, "socket", "", "Path to running plugin socket (for health/lifecycle validation)")
	cmd.Flags().BoolVar(&manifestOnly, "manifest-only", false, "Only validate manifest.json")
	cmd.Flags().BoolVar(&protoOnly, "proto-only", false, "Only validate proto compliance")
	cmd.Flags().BoolVar(&healthOnly, "health-only", false, "Only validate health check (requires --socket)")
	cmd.Flags().BoolVar(&lifecycleOnly, "lifecycle-only", false, "Only validate lifecycle (requires --socket)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output results as JSON")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show detailed output")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colored output")

	return cmd
}

func printSection(title string) {
	fmt.Printf("\n%s%s=== %s ===%s\n\n", colorBold, colorBlue, title, colorReset)
}

func printManifestResult(result *plugins.ManifestValidation, verbose bool) {
	if result.Valid {
		fmt.Printf("%s[PASS]%s Manifest validation passed\n", colorGreen, colorReset)
	} else {
		fmt.Printf("%s[FAIL]%s Manifest validation failed\n", colorRed, colorReset)
	}

	if result.Manifest != nil && verbose {
		fmt.Printf("\n  Plugin ID:    %s\n", result.Manifest.ID)
		fmt.Printf("  Plugin Name:  %s\n", result.Manifest.Name)
		fmt.Printf("  Version:      %s\n", result.Manifest.Version)
		fmt.Printf("  Type:         %s\n", result.Manifest.Type)
		fmt.Printf("  Binary:       %s\n", result.Manifest.Binary)
	}

	if len(result.Errors) > 0 {
		fmt.Printf("\n  %sErrors:%s\n", colorRed, colorReset)
		for _, e := range result.Errors {
			fmt.Printf("    - %s\n", e)
		}
	}

	if len(result.Warnings) > 0 {
		fmt.Printf("\n  %sWarnings:%s\n", colorYellow, colorReset)
		for _, w := range result.Warnings {
			fmt.Printf("    - %s\n", w)
		}
	}
}

func printProtoResult(result *plugins.ProtoValidation, verbose bool) {
	if result.Valid {
		fmt.Printf("%s[PASS]%s Proto compliance validation passed\n", colorGreen, colorReset)
	} else {
		fmt.Printf("%s[FAIL]%s Proto compliance validation failed\n", colorRed, colorReset)
	}

	if verbose {
		if len(result.ServicesRegistered) > 0 {
			fmt.Printf("\n  Services Registered:\n")
			for _, s := range result.ServicesRegistered {
				fmt.Printf("    - %s\n", s)
			}
		}

		if len(result.RPCsImplemented) > 0 {
			fmt.Printf("\n  RPCs Implemented:\n")
			for _, r := range result.RPCsImplemented {
				fmt.Printf("    - %s%s%s\n", colorGreen, r, colorReset)
			}
		}
	}

	if len(result.RPCsMissing) > 0 {
		fmt.Printf("\n  %sRPCs Missing:%s\n", colorRed, colorReset)
		for _, r := range result.RPCsMissing {
			fmt.Printf("    - %s\n", r)
		}
	}

	if len(result.Errors) > 0 {
		fmt.Printf("\n  %sErrors:%s\n", colorRed, colorReset)
		for _, e := range result.Errors {
			fmt.Printf("    - %s\n", e)
		}
	}

	if len(result.Warnings) > 0 {
		fmt.Printf("\n  %sWarnings:%s\n", colorYellow, colorReset)
		for _, w := range result.Warnings {
			fmt.Printf("    - %s\n", w)
		}
	}
}

func printHealthResult(result *plugins.HealthValidation, verbose bool) {
	if result.Valid {
		fmt.Printf("%s[PASS]%s Health check validation passed\n", colorGreen, colorReset)
	} else {
		fmt.Printf("%s[FAIL]%s Health check validation failed\n", colorRed, colorReset)
	}

	if verbose || !result.Healthy {
		fmt.Printf("\n  Healthy:       %v\n", result.Healthy)
		fmt.Printf("  Status:        %s\n", result.Status)
		fmt.Printf("  Response Time: %dms\n", result.ResponseTimeMs)

		if len(result.Metrics) > 0 {
			fmt.Printf("\n  Metrics:\n")
			for k, v := range result.Metrics {
				fmt.Printf("    %s: %s\n", k, v)
			}
		}
	}

	if len(result.Errors) > 0 {
		fmt.Printf("\n  %sErrors:%s\n", colorRed, colorReset)
		for _, e := range result.Errors {
			fmt.Printf("    - %s\n", e)
		}
	}

	if len(result.Warnings) > 0 {
		fmt.Printf("\n  %sWarnings:%s\n", colorYellow, colorReset)
		for _, w := range result.Warnings {
			fmt.Printf("    - %s\n", w)
		}
	}
}

func printLifecycleResult(result *plugins.LifecycleValidation, verbose bool) {
	if result.Valid {
		fmt.Printf("%s[PASS]%s Lifecycle validation passed\n", colorGreen, colorReset)
	} else {
		fmt.Printf("%s[FAIL]%s Lifecycle validation failed\n", colorRed, colorReset)
	}

	if verbose {
		fmt.Printf("\n  Module ID:      %s\n", result.ModuleID)
		fmt.Printf("  Module Version: %s\n", result.ModuleVersion)
		fmt.Printf("  Total Duration: %dms\n", result.TotalDurationMs)
	}

	// Always show step results
	fmt.Printf("\n  Lifecycle Steps:\n")
	printStepResult("Handshake", result.HandshakeOK)
	printStepResult("Health Check", result.HealthOK)
	printStepResult("Shutdown", result.ShutdownOK)

	if len(result.Errors) > 0 {
		fmt.Printf("\n  %sErrors:%s\n", colorRed, colorReset)
		for _, e := range result.Errors {
			fmt.Printf("    - %s\n", e)
		}
	}

	if len(result.Warnings) > 0 {
		fmt.Printf("\n  %sWarnings:%s\n", colorYellow, colorReset)
		for _, w := range result.Warnings {
			fmt.Printf("    - %s\n", w)
		}
	}
}

func printStepResult(name string, ok bool) {
	if ok {
		fmt.Printf("    %s[OK]%s %s\n", colorGreen, colorReset, name)
	} else {
		fmt.Printf("    %s[FAIL]%s %s\n", colorRed, colorReset, name)
	}
}

func printSummary(result *plugins.PluginValidation) {
	fmt.Printf("\n%s=== Summary ===%s\n\n", colorBold, colorReset)

	var passed, failed, warnings int

	if result.Manifest != nil {
		if result.Manifest.Valid {
			passed++
		} else {
			failed++
		}
		warnings += len(result.Manifest.Warnings)
	}

	if result.Proto != nil {
		if result.Proto.Valid {
			passed++
		} else {
			failed++
		}
		warnings += len(result.Proto.Warnings)
	}

	if result.Health != nil {
		if result.Health.Valid {
			passed++
		} else {
			failed++
		}
		warnings += len(result.Health.Warnings)
	}

	if result.Lifecycle != nil {
		if result.Lifecycle.Valid {
			passed++
		} else {
			failed++
		}
		warnings += len(result.Lifecycle.Warnings)
	}

	fmt.Printf("  Validations Passed:  %s%d%s\n", colorGreen, passed, colorReset)
	fmt.Printf("  Validations Failed:  %s%d%s\n", colorRed, failed, colorReset)
	fmt.Printf("  Total Warnings:      %s%d%s\n", colorYellow, warnings, colorReset)

	fmt.Println()
	if result.Valid {
		fmt.Printf("%s%sPlugin validation PASSED%s\n", colorBold, colorGreen, colorReset)
	} else {
		fmt.Printf("%s%sPlugin validation FAILED%s\n", colorBold, colorRed, colorReset)
	}
	fmt.Println()
}

func outputJSON(result *plugins.PluginValidation) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(result)
}

func outputJSONError(err error) {
	result := map[string]interface{}{
		"valid": false,
		"error": err.Error(),
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(result)
}

func isTerminal() bool {
	fileInfo, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

func disableColors() {
	colorReset = ""
	colorRed = ""
	colorGreen = ""
	colorYellow = ""
	colorBlue = ""
	colorBold = ""
}

// ============ Template Definitions ============

const manifestIngestionTemplate = `{
  "id": "{{.ModuleID}}",
  "name": "{{.Name}}",
  "version": "{{.Version}}",
  "type": "INGESTION",
  "binary": "{{.ModuleID}}",
  "capabilities": {
    "ingestion_sources": ["{{.ModuleID}}://"],
    "content_types": ["application/x-{{.ModuleID}}"]
  },
  "health_check_interval_ms": 5000,
  "startup_timeout_ms": 10000
}
`

const manifestReasoningTemplate = `{
  "id": "{{.ModuleID}}",
  "name": "{{.Name}}",
  "version": "{{.Version}}",
  "type": "REASONING",
  "binary": "{{.ModuleID}}",
  "capabilities": {
    "pattern_detectors": ["custom_ranking"]
  },
  "health_check_interval_ms": 5000,
  "startup_timeout_ms": 10000,
  "config": {
    "boost_factor": "0.2"
  }
}
`

const manifestAPETemplate = `{
  "id": "{{.ModuleID}}",
  "name": "{{.Name}}",
  "version": "{{.Version}}",
  "type": "APE",
  "binary": "{{.ModuleID}}",
  "capabilities": {
    "event_triggers": ["session_end", "consolidate"]
  },
  "health_check_interval_ms": 5000,
  "startup_timeout_ms": 10000
}
`

const mainGoTemplate = `package main

import (
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	pb "mdemg/api/modulepb"
)

const (
	moduleID      = "{{.ModuleID}}"
	moduleVersion = "{{.Version}}"
)

func main() {
	socketPath := flag.String("socket", "", "Unix socket path")
	flag.Parse()

	if *socketPath == "" {
		log.Fatal("--socket is required")
	}

	// Remove stale socket
	os.Remove(*socketPath)

	// Create Unix socket listener
	listener, err := net.Listen("unix", *socketPath)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()
	defer os.Remove(*socketPath)

	log.Printf("%s: listening on %s", moduleID, *socketPath)

	// Create gRPC server
	grpcServer := grpc.NewServer()

	// Create and register module handler
	handler := New{{.StructName}}Handler()
	pb.RegisterModuleLifecycleServer(grpcServer, handler)
{{if eq .Type "INGESTION"}}	pb.RegisterIngestionModuleServer(grpcServer, handler)
{{else if eq .Type "REASONING"}}	pb.RegisterReasoningModuleServer(grpcServer, handler)
{{else if eq .Type "APE"}}	pb.RegisterAPEModuleServer(grpcServer, handler)
{{end}}
	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-sigChan
		log.Printf("%s: received shutdown signal", moduleID)
		grpcServer.GracefulStop()
	}()

	// Start serving
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
`

const handlerIngestionTemplate = `package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pb "mdemg/api/modulepb"
)

// {{.StructName}}Handler implements the INGESTION module interfaces
type {{.StructName}}Handler struct {
	pb.UnimplementedModuleLifecycleServer
	pb.UnimplementedIngestionModuleServer

	mu        sync.Mutex
	startTime time.Time
	config    map[string]string
}

var requestCounter uint64

// New{{.StructName}}Handler creates a new handler instance
func New{{.StructName}}Handler() *{{.StructName}}Handler {
	return &{{.StructName}}Handler{
		startTime: time.Now(),
		config:    make(map[string]string),
	}
}

// ============ Lifecycle RPCs (Required for ALL modules) ============

// Handshake is called immediately after spawn to verify module is ready.
func (h *{{.StructName}}Handler) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	log.Printf("%s: handshake from MDEMG %s", moduleID, req.MdemgVersion)

	// Store configuration from manifest
	h.mu.Lock()
	h.config = req.Config
	h.mu.Unlock()

	return &pb.HandshakeResponse{
		ModuleId:      moduleID,
		ModuleVersion: moduleVersion,
		ModuleType:    pb.ModuleType_MODULE_TYPE_INGESTION,
		Capabilities:  []string{"{{.ModuleID}}://", "application/x-{{.ModuleID}}"},
		Ready:         true,
	}, nil
}

// HealthCheck is called periodically to verify module health.
func (h *{{.StructName}}Handler) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	return &pb.HealthCheckResponse{
		Healthy: true,
		Status:  "ok",
		Metrics: map[string]string{
			"uptime":           time.Since(h.startTime).String(),
			"requests_handled": fmt.Sprintf("%d", atomic.LoadUint64(&requestCounter)),
		},
	}, nil
}

// Shutdown is called when MDEMG is stopping or the module is being disabled.
func (h *{{.StructName}}Handler) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	log.Printf("%s: shutdown requested (reason: %s)", moduleID, req.Reason)
	return &pb.ShutdownResponse{
		Success: true,
		Message: "goodbye",
	}, nil
}

// ============ Ingestion RPCs ============

// Matches checks if this module can handle the given source.
func (h *{{.StructName}}Handler) Matches(ctx context.Context, req *pb.MatchRequest) (*pb.MatchResponse, error) {
	atomic.AddUint64(&requestCounter, 1)

	// TODO: Customize matching logic for your data source
	matches := strings.HasPrefix(req.SourceUri, "{{.ModuleID}}://") ||
		req.ContentType == "application/x-{{.ModuleID}}"

	confidence := float32(0.0)
	reason := "not a supported source"
	if matches {
		confidence = 1.0
		reason = "matches {{.ModuleID}}:// or application/x-{{.ModuleID}}"
	}

	return &pb.MatchResponse{
		Matches:    matches,
		Confidence: confidence,
		Reason:     reason,
	}, nil
}

// Parse converts source content into MDEMG observations.
func (h *{{.StructName}}Handler) Parse(ctx context.Context, req *pb.ParseRequest) (*pb.ParseResponse, error) {
	atomic.AddUint64(&requestCounter, 1)

	// TODO: Implement your parsing logic here
	// Convert the raw content into structured observations

	obs := &pb.Observation{
		NodeId:      fmt.Sprintf("%s-%d", moduleID, time.Now().UnixNano()),
		Path:        req.SourceUri,
		Name:        "Parsed Item", // TODO: Extract a meaningful name
		Content:     string(req.Content),
		ContentType: req.ContentType,
		Tags:        []string{moduleID}, // TODO: Add relevant tags
		Metadata:    map[string]string{"source_uri": req.SourceUri},
		Timestamp:   time.Now().Format(time.RFC3339),
		Source:      moduleID,
	}

	return &pb.ParseResponse{
		Observations: []*pb.Observation{obs},
		Metadata: map[string]string{
			"parsed_at": time.Now().Format(time.RFC3339),
		},
	}, nil
}

// Sync performs incremental synchronization with an external source.
func (h *{{.StructName}}Handler) Sync(req *pb.SyncRequest, stream pb.IngestionModule_SyncServer) error {
	atomic.AddUint64(&requestCounter, 1)

	// TODO: Implement incremental sync logic
	// Use req.Cursor to resume from the last position
	// Fetch items from your external source in batches

	// Example: Send a single response (replace with your sync logic)
	obs := &pb.Observation{
		NodeId:      fmt.Sprintf("%s-sync-%d", moduleID, time.Now().UnixNano()),
		Path:        fmt.Sprintf("%s://sync", moduleID),
		Name:        "sync-observation",
		Content:     "TODO: Replace with synced content",
		ContentType: "text/plain",
		Tags:        []string{moduleID, "sync"},
		Timestamp:   time.Now().Format(time.RFC3339),
		Source:      moduleID,
	}

	if err := stream.Send(&pb.SyncResponse{
		Observations: []*pb.SyncResponse_Observation{obs},
		Cursor:       "cursor-1", // TODO: Return actual cursor for incremental sync
		HasMore:      false,
		Stats: &pb.SyncStats{
			ItemsProcessed: 1,
			ItemsCreated:   1,
		},
	}); err != nil {
		return err
	}

	return nil
}
`

const handlerReasoningTemplate = `package main

import (
	"context"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	pb "mdemg/api/modulepb"
)

// {{.StructName}}Handler implements the REASONING module interfaces
type {{.StructName}}Handler struct {
	pb.UnimplementedModuleLifecycleServer
	pb.UnimplementedReasoningModuleServer

	mu              sync.Mutex
	startTime       time.Time
	requestsHandled int64
	boostFactor     float64
}

// New{{.StructName}}Handler creates a new handler instance
func New{{.StructName}}Handler() *{{.StructName}}Handler {
	return &{{.StructName}}Handler{
		startTime:   time.Now(),
		boostFactor: 0.2, // Default boost factor
	}
}

// ============ Lifecycle RPCs (Required for ALL modules) ============

// Handshake is called immediately after spawn to verify module is ready.
func (h *{{.StructName}}Handler) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	log.Printf("%s: handshake from MDEMG %s", moduleID, req.MdemgVersion)

	// Parse configuration
	if factor, ok := req.Config["boost_factor"]; ok {
		if f, err := strconv.ParseFloat(factor, 64); err == nil {
			h.boostFactor = f
			log.Printf("%s: boost_factor set to %.2f", moduleID, h.boostFactor)
		}
	}

	return &pb.HandshakeResponse{
		ModuleId:      moduleID,
		ModuleVersion: moduleVersion,
		ModuleType:    pb.ModuleType_MODULE_TYPE_REASONING,
		Capabilities:  []string{"custom_ranking"},
		Ready:         true,
	}, nil
}

// HealthCheck is called periodically to verify module health.
func (h *{{.StructName}}Handler) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	h.mu.Lock()
	requests := h.requestsHandled
	h.mu.Unlock()

	return &pb.HealthCheckResponse{
		Healthy: true,
		Status:  "ready",
		Metrics: map[string]string{
			"uptime":           time.Since(h.startTime).String(),
			"requests_handled": strconv.FormatInt(requests, 10),
			"boost_factor":     strconv.FormatFloat(h.boostFactor, 'f', 2, 64),
		},
	}, nil
}

// Shutdown is called when MDEMG is stopping or the module is being disabled.
func (h *{{.StructName}}Handler) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	log.Printf("%s: shutdown requested (reason: %s)", moduleID, req.Reason)
	return &pb.ShutdownResponse{
		Success: true,
		Message: "goodbye",
	}, nil
}

// ============ Reasoning RPCs ============

// Process takes retrieval candidates and returns refined results.
func (h *{{.StructName}}Handler) Process(ctx context.Context, req *pb.ProcessRequest) (*pb.ProcessResponse, error) {
	h.mu.Lock()
	h.requestsHandled++
	h.mu.Unlock()

	if len(req.Candidates) == 0 {
		return &pb.ProcessResponse{Results: req.Candidates}, nil
	}

	log.Printf("%s: processing %d candidates for: %s",
		moduleID, len(req.Candidates), truncate(req.QueryText, 50))

	// TODO: Implement your re-ranking/filtering logic here
	// Example: Simple keyword-based boosting

	// Extract keywords from query
	keywords := extractKeywords(req.QueryText)

	// Score and boost candidates
	type scored struct {
		candidate *pb.RetrievalCandidate
		boost     float64
	}

	results := make([]scored, len(req.Candidates))
	for i, c := range req.Candidates {
		// Calculate boost based on keyword matches
		boost := calculateBoost(c.Name, c.Summary, keywords) * h.boostFactor

		// Apply boost to score
		c.Score = c.Score + float32(boost)

		results[i] = scored{candidate: c, boost: boost}
	}

	// Sort by new score (descending)
	sort.Slice(results, func(i, j int) bool {
		return results[i].candidate.Score > results[j].candidate.Score
	})

	// Build output
	output := make([]*pb.RetrievalCandidate, len(results))
	for i, r := range results {
		output[i] = r.candidate
	}

	// Apply top_k limit
	if req.TopK > 0 && int(req.TopK) < len(output) {
		output = output[:req.TopK]
	}

	return &pb.ProcessResponse{
		Results: output,
		Metadata: map[string]string{
			"keywords":     strings.Join(keywords, ","),
			"boost_factor": strconv.FormatFloat(h.boostFactor, 'f', 2, 64),
		},
	}, nil
}

// ============ Helper Functions ============

// extractKeywords tokenizes the query into keywords (customize as needed)
func extractKeywords(query string) []string {
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true,
		"of": true, "at": true, "by": true, "for": true, "with": true,
		"to": true, "from": true, "in": true, "out": true, "on": true,
		"and": true, "or": true, "but": true, "if": true, "what": true,
		"this": true, "that": true, "how": true, "why": true, "where": true,
	}

	words := strings.Fields(strings.ToLower(query))
	var keywords []string
	for _, word := range words {
		word = strings.Trim(word, ".,!?;:\"'()[]{}/-")
		if len(word) >= 2 && !stopWords[word] {
			keywords = append(keywords, word)
		}
	}
	return keywords
}

// calculateBoost scores how well a candidate matches the keywords
func calculateBoost(name, summary string, keywords []string) float64 {
	if len(keywords) == 0 {
		return 0
	}

	combined := strings.ToLower(name + " " + summary)
	matchCount := 0

	for _, kw := range keywords {
		if strings.Contains(combined, kw) {
			matchCount++
		}
	}

	return float64(matchCount) / float64(len(keywords))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
`

const handlerAPETemplate = `package main

import (
	"context"
	"log"
	"strconv"
	"sync"
	"time"

	pb "mdemg/api/modulepb"
)

// {{.StructName}}Handler implements the APE module interfaces
type {{.StructName}}Handler struct {
	pb.UnimplementedModuleLifecycleServer
	pb.UnimplementedAPEModuleServer

	mu              sync.Mutex
	startTime       time.Time
	executionsTotal int64
	lastExecution   time.Time
}

// New{{.StructName}}Handler creates a new handler instance
func New{{.StructName}}Handler() *{{.StructName}}Handler {
	return &{{.StructName}}Handler{
		startTime: time.Now(),
	}
}

// ============ Lifecycle RPCs (Required for ALL modules) ============

// Handshake is called immediately after spawn to verify module is ready.
func (h *{{.StructName}}Handler) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	log.Printf("%s: handshake from MDEMG %s", moduleID, req.MdemgVersion)

	return &pb.HandshakeResponse{
		ModuleId:      moduleID,
		ModuleVersion: moduleVersion,
		ModuleType:    pb.ModuleType_MODULE_TYPE_APE,
		Capabilities:  []string{"background_task"}, // TODO: Describe your capabilities
		Ready:         true,
	}, nil
}

// HealthCheck is called periodically to verify module health.
func (h *{{.StructName}}Handler) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	h.mu.Lock()
	executions := h.executionsTotal
	lastExec := h.lastExecution
	h.mu.Unlock()

	metrics := map[string]string{
		"uptime":           time.Since(h.startTime).String(),
		"executions_total": strconv.FormatInt(executions, 10),
	}
	if !lastExec.IsZero() {
		metrics["last_execution"] = lastExec.Format(time.RFC3339)
	}

	return &pb.HealthCheckResponse{
		Healthy: true,
		Status:  "ready",
		Metrics: metrics,
	}, nil
}

// Shutdown is called when MDEMG is stopping or the module is being disabled.
func (h *{{.StructName}}Handler) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	log.Printf("%s: shutdown requested (reason: %s)", moduleID, req.Reason)
	return &pb.ShutdownResponse{
		Success: true,
		Message: "goodbye",
	}, nil
}

// ============ APE RPCs ============

// GetSchedule returns the module's preferred execution schedule.
func (h *{{.StructName}}Handler) GetSchedule(ctx context.Context, req *pb.GetScheduleRequest) (*pb.GetScheduleResponse, error) {
	return &pb.GetScheduleResponse{
		// TODO: Customize your schedule
		CronExpression:     "0 * * * *", // Every hour
		EventTriggers:      []string{"session_end", "consolidate"},
		MinIntervalSeconds: 300, // Minimum 5 minutes between runs
	}, nil
}

// Execute runs a background task.
func (h *{{.StructName}}Handler) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
	start := time.Now()

	h.mu.Lock()
	h.executionsTotal++
	h.lastExecution = start
	execNum := h.executionsTotal
	h.mu.Unlock()

	log.Printf("%s: executing task %s (trigger=%s, execution #%d)",
		moduleID, req.TaskId, req.Trigger, execNum)

	// TODO: Implement your background task logic here
	var message string
	var stats *pb.ExecuteStats

	switch req.Trigger {
	case "event:session_end":
		// TODO: Handle session end event
		message, stats = h.handleSessionEnd(ctx)

	case "event:consolidate":
		// TODO: Handle consolidation event
		message, stats = h.handleConsolidate(ctx)

	case "schedule":
		// TODO: Handle scheduled execution
		message, stats = h.handleScheduled(ctx)

	default:
		message = "Unknown trigger, no action taken"
		stats = &pb.ExecuteStats{DurationMs: time.Since(start).Milliseconds()}
	}

	stats.DurationMs = time.Since(start).Milliseconds()

	log.Printf("%s: task %s completed in %v", moduleID, req.TaskId, time.Since(start))

	return &pb.ExecuteResponse{
		Success: true,
		Message: message,
		Stats:   stats,
	}, nil
}

// ============ Task Handlers (Customize these) ============

func (h *{{.StructName}}Handler) handleSessionEnd(ctx context.Context) (string, *pb.ExecuteStats) {
	// TODO: Implement session reflection logic
	// - Query recent observations
	// - Generate summary nodes
	// - Create relationship edges

	return "Session processing completed", &pb.ExecuteStats{
		NodesCreated: 0,
		NodesUpdated: 0,
		EdgesCreated: 0,
	}
}

func (h *{{.StructName}}Handler) handleConsolidate(ctx context.Context) (string, *pb.ExecuteStats) {
	// TODO: Implement post-consolidation logic

	return "Post-consolidation processing completed", &pb.ExecuteStats{
		NodesUpdated: 0,
	}
}

func (h *{{.StructName}}Handler) handleScheduled(ctx context.Context) (string, *pb.ExecuteStats) {
	// TODO: Implement periodic maintenance
	// - Cleanup stale data
	// - Recalculate statistics
	// - Detect patterns

	return "Scheduled maintenance completed", &pb.ExecuteStats{
		NodesUpdated: 0,
	}
}
`

const makefileTemplate = `.PHONY: build clean test

BINARY_NAME = {{.ModuleID}}

build:
	go build -o $(BINARY_NAME) .

clean:
	rm -f $(BINARY_NAME)

test:
	go test -v ./...

run: build
	./$(BINARY_NAME) --socket /tmp/$(BINARY_NAME).sock
`

const readmeTemplate = `# {{.Name}}

{{if eq .Type "INGESTION"}}An INGESTION module for MDEMG that parses external sources into observations.{{else if eq .Type "REASONING"}}A REASONING module for MDEMG that re-ranks and filters retrieval results.{{else if eq .Type "APE"}}An APE (Active Participant Engine) module for MDEMG that performs background maintenance tasks.{{end}}

## Overview

- **Module ID**: ` + "`{{.ModuleID}}`" + `
- **Type**: {{.Type}}
- **Version**: {{.Version}}

## Building

` + "```bash" + `
make build
` + "```" + `

## Development

1. Edit ` + "`handler.go`" + ` to implement your custom logic
2. Update ` + "`manifest.json`" + ` with your capabilities
3. Build with ` + "`make build`" + `
4. Test locally: ` + "`make run`" + `

## Testing Locally

` + "```bash" + `
# Start the module
./{{.ModuleID}} --socket /tmp/{{.ModuleID}}.sock

# In another terminal, test with grpcurl
grpcurl -plaintext -unix /tmp/{{.ModuleID}}.sock \
    mdemg.module.v1.ModuleLifecycle/HealthCheck
` + "```" + `

## Deployment

Place the built binary and ` + "`manifest.json`" + ` in the MDEMG plugins directory:

` + "```" + `
plugins/
  {{.ModuleID}}/
    {{.ModuleID}}    # binary
    manifest.json
` + "```" + `

MDEMG will auto-discover the plugin on startup.

{{if eq .Type "INGESTION"}}## Ingestion Module

This module implements:
- **Matches**: Determine if this module can handle a given source
- **Parse**: Convert raw content into MDEMG observations
- **Sync**: Incrementally sync with external sources

### Customization

Edit the ` + "`Matches`" + ` function to define which sources this module handles.
Edit the ` + "`Parse`" + ` function to implement your parsing logic.
{{else if eq .Type "REASONING"}}## Reasoning Module

This module implements:
- **Process**: Re-rank/filter retrieval candidates

### Customization

Edit the ` + "`Process`" + ` function to implement your ranking algorithm.
The default implementation uses keyword-based boosting.
{{else if eq .Type "APE"}}## APE Module

This module implements:
- **GetSchedule**: Define when this module should run
- **Execute**: Perform background maintenance tasks

### Customization

Edit ` + "`GetSchedule`" + ` to define your cron schedule and event triggers.
Edit the handler functions (` + "`handleSessionEnd`" + `, etc.) to implement your logic.
{{end}}

## Configuration

Configuration is passed via ` + "`manifest.json`" + `:

` + "```json" + `
{
  "config": {
    "key": "value"
  }
}
` + "```" + `

Access configuration in your handler via ` + "`req.Config`" + ` in the Handshake RPC.
`
