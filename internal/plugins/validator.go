package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "mdemg/api/modulepb"
)

// ValidationResult contains the base validation results
type ValidationResult struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// ManifestValidation contains manifest validation results
type ManifestValidation struct {
	ValidationResult
	Manifest *Manifest `json:"manifest,omitempty"`
}

// ProtoValidation contains proto compliance validation results
type ProtoValidation struct {
	ValidationResult
	ServicesRegistered []string `json:"services_registered,omitempty"`
	RPCsImplemented    []string `json:"rpcs_implemented,omitempty"`
	RPCsMissing        []string `json:"rpcs_missing,omitempty"`
}

// HealthValidation contains health check validation results
type HealthValidation struct {
	ValidationResult
	Healthy       bool              `json:"healthy"`
	Status        string            `json:"status,omitempty"`
	Metrics       map[string]string `json:"metrics,omitempty"`
	ResponseTimeMs int64            `json:"response_time_ms"`
}

// LifecycleValidation contains full lifecycle validation results
type LifecycleValidation struct {
	ValidationResult
	HandshakeOK  bool   `json:"handshake_ok"`
	HealthOK     bool   `json:"health_ok"`
	ShutdownOK   bool   `json:"shutdown_ok"`
	ModuleID     string `json:"module_id,omitempty"`
	ModuleVersion string `json:"module_version,omitempty"`
	TotalDurationMs int64 `json:"total_duration_ms"`
}

// PluginValidation contains all validation results for a plugin
type PluginValidation struct {
	PluginPath  string               `json:"plugin_path"`
	Valid       bool                 `json:"valid"`
	Manifest    *ManifestValidation  `json:"manifest,omitempty"`
	Proto       *ProtoValidation     `json:"proto,omitempty"`
	Health      *HealthValidation    `json:"health,omitempty"`
	Lifecycle   *LifecycleValidation `json:"lifecycle,omitempty"`
}

// semverRegex matches semver version strings (e.g., 1.0.0, 2.1.3-beta)
var semverRegex = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)

// validModuleTypes are the allowed module types
var validModuleTypes = map[string]bool{
	"INGESTION": true,
	"REASONING": true,
	"APE":       true,
}

// knownManifestFields are the known fields in manifest.json
var knownManifestFields = map[string]bool{
	"id":                       true,
	"name":                     true,
	"version":                  true,
	"type":                     true,
	"binary":                   true,
	"capabilities":             true,
	"health_check_interval_ms": true,
	"startup_timeout_ms":       true,
	"config":                   true,
}

// knownCapabilitiesFields are the known fields in capabilities
var knownCapabilitiesFields = map[string]bool{
	"ingestion_sources":  true,
	"content_types":      true,
	"pattern_detectors":  true,
	"event_triggers":     true,
}

// ValidateManifest validates a plugin's manifest.json file
func ValidateManifest(path string) (*ManifestValidation, error) {
	result := &ManifestValidation{
		ValidationResult: ValidationResult{
			Valid:    true,
			Errors:   []string{},
			Warnings: []string{},
		},
	}

	// Check if path is a directory or file
	info, err := os.Stat(path)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("path not found: %v", err))
		return result, nil
	}

	manifestPath := path
	var pluginDir string
	if info.IsDir() {
		manifestPath = filepath.Join(path, "manifest.json")
		pluginDir = path
	} else {
		pluginDir = filepath.Dir(path)
	}

	// Read manifest file
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("failed to read manifest.json: %v", err))
		return result, nil
	}

	// Parse manifest for unknown field detection
	var rawManifest map[string]interface{}
	if err := json.Unmarshal(data, &rawManifest); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("invalid JSON: %v", err))
		return result, nil
	}

	// Check for unknown fields
	for key := range rawManifest {
		if !knownManifestFields[key] {
			result.Warnings = append(result.Warnings, fmt.Sprintf("unknown field in manifest: %q", key))
		}
	}

	// Check for unknown capability fields
	if caps, ok := rawManifest["capabilities"].(map[string]interface{}); ok {
		for key := range caps {
			if !knownCapabilitiesFields[key] {
				result.Warnings = append(result.Warnings, fmt.Sprintf("unknown capability field: %q", key))
			}
		}
	}

	// Parse manifest into struct
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("failed to parse manifest: %v", err))
		return result, nil
	}

	// Validate required fields
	if manifest.ID == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "missing required field: id")
	}

	if manifest.Name == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "missing required field: name")
	}

	if manifest.Version == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "missing required field: version")
	} else if !semverRegex.MatchString(manifest.Version) {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("version %q does not follow semver format (e.g., 1.0.0)", manifest.Version))
	}

	if manifest.Type == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "missing required field: type")
	} else if !validModuleTypes[manifest.Type] {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("invalid module type %q: must be one of INGESTION, REASONING, APE", manifest.Type))
	}

	if manifest.Binary == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "missing required field: binary")
	} else {
		// Check if entrypoint (binary) exists
		binaryPath := filepath.Join(pluginDir, manifest.Binary)
		if fi, err := os.Stat(binaryPath); err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("entrypoint binary not found: %s", binaryPath))
		} else if fi.IsDir() {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("entrypoint is a directory, not a file: %s", binaryPath))
		} else if fi.Mode()&0111 == 0 {
			result.Warnings = append(result.Warnings, fmt.Sprintf("entrypoint binary may not be executable: %s", binaryPath))
		}
	}

	// Validate capability consistency with module type
	switch manifest.Type {
	case "INGESTION":
		if len(manifest.Capabilities.PatternDetectors) > 0 {
			result.Warnings = append(result.Warnings, "pattern_detectors are typically for REASONING modules, not INGESTION")
		}
		if len(manifest.Capabilities.EventTriggers) > 0 {
			result.Warnings = append(result.Warnings, "event_triggers are typically for APE modules, not INGESTION")
		}
	case "REASONING":
		if len(manifest.Capabilities.IngestionSources) > 0 {
			result.Warnings = append(result.Warnings, "ingestion_sources are typically for INGESTION modules, not REASONING")
		}
		if len(manifest.Capabilities.EventTriggers) > 0 {
			result.Warnings = append(result.Warnings, "event_triggers are typically for APE modules, not REASONING")
		}
	case "APE":
		if len(manifest.Capabilities.IngestionSources) > 0 {
			result.Warnings = append(result.Warnings, "ingestion_sources are typically for INGESTION modules, not APE")
		}
		if len(manifest.Capabilities.PatternDetectors) > 0 {
			result.Warnings = append(result.Warnings, "pattern_detectors are typically for REASONING modules, not APE")
		}
	}

	// Validate health check interval
	if manifest.HealthCheckIntervalMs < 0 {
		result.Warnings = append(result.Warnings, "health_check_interval_ms should be positive")
	} else if manifest.HealthCheckIntervalMs > 0 && manifest.HealthCheckIntervalMs < 1000 {
		result.Warnings = append(result.Warnings, "health_check_interval_ms less than 1000ms may cause excessive load")
	}

	// Validate startup timeout
	if manifest.StartupTimeoutMs < 0 {
		result.Warnings = append(result.Warnings, "startup_timeout_ms should be positive")
	} else if manifest.StartupTimeoutMs > 0 && manifest.StartupTimeoutMs < 1000 {
		result.Warnings = append(result.Warnings, "startup_timeout_ms less than 1000ms may be too short for some modules")
	}

	result.Manifest = &manifest
	return result, nil
}

// ValidateProtoCompliance tests that a plugin binary implements the required gRPC services
func ValidateProtoCompliance(binaryPath string, moduleType string) (*ProtoValidation, error) {
	result := &ProtoValidation{
		ValidationResult: ValidationResult{
			Valid:    true,
			Errors:   []string{},
			Warnings: []string{},
		},
		ServicesRegistered: []string{},
		RPCsImplemented:    []string{},
		RPCsMissing:        []string{},
	}

	// Validate module type
	if !validModuleTypes[moduleType] {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("invalid module type: %s", moduleType))
		return result, nil
	}

	// Normalize the binary path — filepath.Clean strips `..` traversal
	// tokens and normalizes separators. The path is operator-provided (via
	// the `mdemg module validate <path>` CLI) so this is defense-in-depth
	// against typos, not a security boundary — the operator's intent IS to
	// exec this binary.
	binaryPath = filepath.Clean(binaryPath)

	// Check binary exists and is executable
	if _, err := os.Stat(binaryPath); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("binary not found: %v", err))
		return result, nil
	}

	// Create temporary socket
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("mdemg-validate-%d.sock", time.Now().UnixNano()))
	defer os.Remove(socketPath)

	// Start the plugin binary
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// #nosec G204 — CodeQL go/command-injection false-positive-with-context.
	// binaryPath is operator-provided via the `mdemg module validate <path>`
	// CLI or programmatic caller. This function's ENTIRE purpose is to exec
	// operator-specified plugin binaries + probe their gRPC surface. It is
	// not reachable from any network-facing HTTP handler. The trust model is
	// "operator ran a local CLI intending to launch this binary" — same
	// level as `go run <path>` or `./<path>`. filepath.Clean above catches
	// typo-class path issues; a malicious path would already require local
	// shell access, at which point cmd.Exec is not the attack surface.
	cmd := exec.CommandContext(ctx, binaryPath, "--socket", socketPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("failed to start binary: %v", err))
		return result, nil
	}

	// Ensure process is killed on exit
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()

	// Wait for socket to be created
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if _, err := os.Stat(socketPath); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, "plugin did not create socket within timeout")
		return result, nil
	}

	// Connect to the plugin
	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("failed to connect to plugin: %v", err))
		return result, nil
	}
	defer func() { _ = conn.Close() }()

	// Test ModuleLifecycle service (required for all modules)
	lifecycleClient := pb.NewModuleLifecycleClient(conn)

	// Test Handshake RPC
	handshakeResp, err := lifecycleClient.Handshake(ctx, &pb.HandshakeRequest{
		MdemgVersion: "1.0.0-test",
		SocketPath:   socketPath,
	})
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("Handshake RPC not implemented or failed: %v", err))
	} else {
		result.RPCsImplemented = append(result.RPCsImplemented, "ModuleLifecycle.Handshake")
		if !handshakeResp.Ready {
			result.Warnings = append(result.Warnings, fmt.Sprintf("module reported not ready: %s", handshakeResp.Error))
		}
	}

	// Test HealthCheck RPC
	_, err = lifecycleClient.HealthCheck(ctx, &pb.HealthCheckRequest{})
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("HealthCheck RPC not implemented or failed: %v", err))
	} else {
		result.RPCsImplemented = append(result.RPCsImplemented, "ModuleLifecycle.HealthCheck")
	}

	result.ServicesRegistered = append(result.ServicesRegistered, "ModuleLifecycle")

	// Test type-specific services
	switch moduleType {
	case "INGESTION":
		ingestionClient := pb.NewIngestionModuleClient(conn)

		// Test Matches RPC
		_, err = ingestionClient.Matches(ctx, &pb.MatchRequest{
			SourceUri:   "test://validation",
			ContentType: "text/plain",
		})
		if err != nil {
			if strings.Contains(err.Error(), "Unimplemented") {
				result.RPCsMissing = append(result.RPCsMissing, "IngestionModule.Matches")
				result.Valid = false
				result.Errors = append(result.Errors, "IngestionModule.Matches RPC not implemented")
			} else {
				// RPC is implemented but returned an error - that's OK for validation
				result.RPCsImplemented = append(result.RPCsImplemented, "IngestionModule.Matches")
			}
		} else {
			result.RPCsImplemented = append(result.RPCsImplemented, "IngestionModule.Matches")
		}

		// Test Parse RPC
		_, err = ingestionClient.Parse(ctx, &pb.ParseRequest{
			SourceUri:   "test://validation",
			ContentType: "text/plain",
			Content:     []byte("test content"),
		})
		if err != nil {
			if strings.Contains(err.Error(), "Unimplemented") {
				result.RPCsMissing = append(result.RPCsMissing, "IngestionModule.Parse")
				result.Valid = false
				result.Errors = append(result.Errors, "IngestionModule.Parse RPC not implemented")
			} else {
				result.RPCsImplemented = append(result.RPCsImplemented, "IngestionModule.Parse")
			}
		} else {
			result.RPCsImplemented = append(result.RPCsImplemented, "IngestionModule.Parse")
		}

		// Test Sync RPC (streaming)
		syncClient, err := ingestionClient.Sync(ctx, &pb.SyncRequest{
			SourceId: "test-validation",
		})
		if err != nil {
			if strings.Contains(err.Error(), "Unimplemented") {
				result.RPCsMissing = append(result.RPCsMissing, "IngestionModule.Sync")
				result.Valid = false
				result.Errors = append(result.Errors, "IngestionModule.Sync RPC not implemented")
			} else {
				result.RPCsImplemented = append(result.RPCsImplemented, "IngestionModule.Sync")
			}
		} else {
			// Try to receive at least one response to verify streaming works
			_, err = syncClient.Recv()
			if err != nil && !strings.Contains(err.Error(), "EOF") {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Sync streaming may have issues: %v", err))
			}
			result.RPCsImplemented = append(result.RPCsImplemented, "IngestionModule.Sync")
		}

		result.ServicesRegistered = append(result.ServicesRegistered, "IngestionModule")

	case "REASONING":
		reasoningClient := pb.NewReasoningModuleClient(conn)

		// Test Process RPC
		_, err = reasoningClient.Process(ctx, &pb.ProcessRequest{
			QueryText: "test query",
			Candidates: []*pb.RetrievalCandidate{
				{NodeId: "test-1", Name: "Test", Score: 0.5},
			},
			TopK: 10,
		})
		if err != nil {
			if strings.Contains(err.Error(), "Unimplemented") {
				result.RPCsMissing = append(result.RPCsMissing, "ReasoningModule.Process")
				result.Valid = false
				result.Errors = append(result.Errors, "ReasoningModule.Process RPC not implemented")
			} else {
				result.RPCsImplemented = append(result.RPCsImplemented, "ReasoningModule.Process")
			}
		} else {
			result.RPCsImplemented = append(result.RPCsImplemented, "ReasoningModule.Process")
		}

		result.ServicesRegistered = append(result.ServicesRegistered, "ReasoningModule")

	case "APE":
		apeClient := pb.NewAPEModuleClient(conn)

		// Test GetSchedule RPC
		_, err = apeClient.GetSchedule(ctx, &pb.GetScheduleRequest{})
		if err != nil {
			if strings.Contains(err.Error(), "Unimplemented") {
				result.RPCsMissing = append(result.RPCsMissing, "APEModule.GetSchedule")
				result.Valid = false
				result.Errors = append(result.Errors, "APEModule.GetSchedule RPC not implemented")
			} else {
				result.RPCsImplemented = append(result.RPCsImplemented, "APEModule.GetSchedule")
			}
		} else {
			result.RPCsImplemented = append(result.RPCsImplemented, "APEModule.GetSchedule")
		}

		// Test Execute RPC
		_, err = apeClient.Execute(ctx, &pb.ExecuteRequest{
			TaskId:  "test-validation",
			Trigger: "validation",
		})
		if err != nil {
			if strings.Contains(err.Error(), "Unimplemented") {
				result.RPCsMissing = append(result.RPCsMissing, "APEModule.Execute")
				result.Valid = false
				result.Errors = append(result.Errors, "APEModule.Execute RPC not implemented")
			} else {
				result.RPCsImplemented = append(result.RPCsImplemented, "APEModule.Execute")
			}
		} else {
			result.RPCsImplemented = append(result.RPCsImplemented, "APEModule.Execute")
		}

		result.ServicesRegistered = append(result.ServicesRegistered, "APEModule")
	}

	// Test Shutdown RPC (do this last)
	_, err = lifecycleClient.Shutdown(ctx, &pb.ShutdownRequest{
		TimeoutMs: 5000,
		Reason:    "validation_complete",
	})
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Shutdown RPC failed: %v", err))
	} else {
		result.RPCsImplemented = append(result.RPCsImplemented, "ModuleLifecycle.Shutdown")
	}

	return result, nil
}

// ValidateHealthCheck tests the health check response of a running plugin
func ValidateHealthCheck(socketPath string) (*HealthValidation, error) {
	result := &HealthValidation{
		ValidationResult: ValidationResult{
			Valid:    true,
			Errors:   []string{},
			Warnings: []string{},
		},
	}

	// Check socket exists
	if _, err := os.Stat(socketPath); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("socket not found: %v", err))
		return result, nil
	}

	// Connect to the plugin
	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("failed to connect: %v", err))
		return result, nil
	}
	defer func() { _ = conn.Close() }()

	client := pb.NewModuleLifecycleClient(conn)

	// Call health check with timing
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	resp, err := client.HealthCheck(ctx, &pb.HealthCheckRequest{})
	result.ResponseTimeMs = time.Since(start).Milliseconds()

	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("health check failed: %v", err))
		return result, nil
	}

	result.Healthy = resp.Healthy
	result.Status = resp.Status
	result.Metrics = resp.Metrics

	if !resp.Healthy {
		result.Warnings = append(result.Warnings, fmt.Sprintf("module reported unhealthy: %s", resp.Status))
	}

	// Check response time
	if result.ResponseTimeMs > 100 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("health check response time (%dms) exceeds recommended 100ms", result.ResponseTimeMs))
	}

	return result, nil
}

// ValidateLifecycle tests the full lifecycle of a plugin (handshake -> health -> shutdown)
func ValidateLifecycle(socketPath string) (*LifecycleValidation, error) {
	result := &LifecycleValidation{
		ValidationResult: ValidationResult{
			Valid:    true,
			Errors:   []string{},
			Warnings: []string{},
		},
	}

	start := time.Now()
	defer func() {
		result.TotalDurationMs = time.Since(start).Milliseconds()
	}()

	// Check socket exists
	if _, err := os.Stat(socketPath); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("socket not found: %v", err))
		return result, nil
	}

	// Connect to the plugin
	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("failed to connect: %v", err))
		return result, nil
	}
	defer func() { _ = conn.Close() }()

	client := pb.NewModuleLifecycleClient(conn)

	// Step 1: Handshake
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	handshakeResp, err := client.Handshake(ctx, &pb.HandshakeRequest{
		MdemgVersion: "1.0.0-test",
		SocketPath:   socketPath,
	})
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("handshake failed: %v", err))
		return result, nil
	}

	result.HandshakeOK = handshakeResp.Ready
	result.ModuleID = handshakeResp.ModuleId
	result.ModuleVersion = handshakeResp.ModuleVersion

	if !handshakeResp.Ready {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("module not ready after handshake: %s", handshakeResp.Error))
		return result, nil
	}

	// Step 2: Health Check
	healthResp, err := client.HealthCheck(ctx, &pb.HealthCheckRequest{})
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("health check failed: %v", err))
		return result, nil
	}

	result.HealthOK = healthResp.Healthy
	if !healthResp.Healthy {
		result.Warnings = append(result.Warnings, fmt.Sprintf("module unhealthy: %s", healthResp.Status))
	}

	// Step 3: Shutdown
	shutdownResp, err := client.Shutdown(ctx, &pb.ShutdownRequest{
		TimeoutMs: 5000,
		Reason:    "lifecycle_validation",
	})
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("shutdown request failed: %v", err))
		result.ShutdownOK = false
	} else {
		result.ShutdownOK = shutdownResp.Success
		if !shutdownResp.Success {
			result.Warnings = append(result.Warnings, fmt.Sprintf("shutdown reported failure: %s", shutdownResp.Message))
		}
	}

	return result, nil
}

// ValidatePlugin performs comprehensive validation of a plugin directory
func ValidatePlugin(pluginPath string) (*PluginValidation, error) {
	result := &PluginValidation{
		PluginPath: pluginPath,
		Valid:      true,
	}

	// Step 1: Validate manifest
	manifestResult, err := ValidateManifest(pluginPath)
	if err != nil {
		return nil, fmt.Errorf("manifest validation error: %w", err)
	}
	result.Manifest = manifestResult

	if !manifestResult.Valid {
		result.Valid = false
		return result, nil
	}

	// Step 2: Validate proto compliance (requires starting the binary)
	binaryPath := filepath.Join(pluginPath, manifestResult.Manifest.Binary)
	protoResult, err := ValidateProtoCompliance(binaryPath, manifestResult.Manifest.Type)
	if err != nil {
		return nil, fmt.Errorf("proto validation error: %w", err)
	}
	result.Proto = protoResult

	if !protoResult.Valid {
		result.Valid = false
	}

	return result, nil
}

// ValidatePluginRunning validates a running plugin via its socket
func ValidatePluginRunning(socketPath string) (*PluginValidation, error) {
	result := &PluginValidation{
		PluginPath: socketPath,
		Valid:      true,
	}

	// Validate health check
	healthResult, err := ValidateHealthCheck(socketPath)
	if err != nil {
		return nil, fmt.Errorf("health validation error: %w", err)
	}
	result.Health = healthResult

	if !healthResult.Valid {
		result.Valid = false
	}

	// Validate lifecycle
	lifecycleResult, err := ValidateLifecycle(socketPath)
	if err != nil {
		return nil, fmt.Errorf("lifecycle validation error: %w", err)
	}
	result.Lifecycle = lifecycleResult

	if !lifecycleResult.Valid {
		result.Valid = false
	}

	return result, nil
}
