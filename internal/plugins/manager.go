package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"path/filepath"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "mdemg/api/modulepb"
)

const (
	defaultHealthCheckInterval = 5 * time.Second
	defaultStartupTimeout      = 10 * time.Second
	defaultShutdownTimeout     = 5 * time.Second
	maxRestartAttempts         = 3
	restartBackoffBase         = 2 * time.Second
)

// Manager handles discovery, lifecycle, and communication with plugin modules
type Manager struct {
	pluginsDir string
	socketDir  string
	mdemgVer   string

	mu      sync.RWMutex
	modules map[string]*moduleInstance

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type moduleInstance struct {
	info           *ModuleInfo
	cmd            *exec.Cmd
	conn           *grpc.ClientConn
	restartCount   int
	lastRestartAt  time.Time
	healthTicker   *time.Ticker
	stopHealthLoop chan struct{}
}

// NewManager creates a new plugin manager
func NewManager(pluginsDir, socketDir, mdemgVersion string) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		pluginsDir: pluginsDir,
		socketDir:  socketDir,
		mdemgVer:   mdemgVersion,
		modules:    make(map[string]*moduleInstance),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Start scans the plugins directory and starts all discovered modules
func (m *Manager) Start() error {
	// Ensure socket directory exists
	if err := os.MkdirAll(m.socketDir, 0755); err != nil {
		return fmt.Errorf("failed to create socket dir: %w", err)
	}

	// Scan plugins directory
	entries, err := os.ReadDir(m.pluginsDir)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Info("plugins: directory does not exist, no modules to load", "dir", m.pluginsDir)
			return nil
		}
		return fmt.Errorf("failed to read plugins dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if entry.Name() == ".disabled" {
			continue
		}

		moduleDir := filepath.Join(m.pluginsDir, entry.Name())
		if err := m.loadModule(moduleDir); err != nil {
			slog.Error("plugins: failed to load module", "module", entry.Name(), "error", err)
			// Continue loading other modules
		}
	}

	return nil
}

// Stop gracefully shuts down all modules
func (m *Manager) Stop() error {
	m.cancel()

	m.mu.Lock()
	defer m.mu.Unlock()

	var lastErr error
	for id, inst := range m.modules {
		if err := m.stopModuleInstance(inst); err != nil {
			slog.Error("plugins: error stopping module", "module", id, "error", err)
			lastErr = err
		}
	}

	m.wg.Wait()
	return lastErr
}

// ListModules returns status of all loaded modules
func (m *Manager) ListModules() []ModuleStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]ModuleStatus, 0, len(m.modules))
	for _, inst := range m.modules {
		info := inst.info
		status := ModuleStatus{
			ID:          info.Manifest.ID,
			Name:        info.Manifest.Name,
			Version:     info.Manifest.Version,
			Type:        info.Manifest.Type,
			State:       string(info.State),
			SocketPath:  info.SocketPath,
			PID:         info.PID,
			LastError:   info.LastError,
			Metrics:     info.Metrics,
		}
		if !info.StartedAt.IsZero() {
			status.StartedAt = info.StartedAt.Format(time.RFC3339)
		}
		if !info.LastHealthy.IsZero() {
			status.LastHealthy = info.LastHealthy.Format(time.RFC3339)
		}

		// Collect capabilities
		caps := info.Manifest.Capabilities
		status.Capabilities = append(status.Capabilities, caps.IngestionSources...)
		status.Capabilities = append(status.Capabilities, caps.ContentTypes...)
		status.Capabilities = append(status.Capabilities, caps.PatternDetectors...)
		status.Capabilities = append(status.Capabilities, caps.EventTriggers...)

		result = append(result, status)
	}
	return result
}

// GetModule returns a specific module by ID
func (m *Manager) GetModule(id string) (*ModuleInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	inst, ok := m.modules[id]
	if !ok {
		return nil, false
	}
	return inst.info, true
}

// GetIngestionModules returns all modules that can handle ingestion
func (m *Manager) GetIngestionModules() []*ModuleInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*ModuleInfo
	for _, inst := range m.modules {
		if inst.info.Manifest.Type == "INGESTION" && inst.info.State == StateReady {
			result = append(result, inst.info)
		}
	}
	return result
}

// GetReasoningModules returns all modules that can process retrieval results
func (m *Manager) GetReasoningModules() []*ModuleInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*ModuleInfo
	for _, inst := range m.modules {
		if inst.info.Manifest.Type == "REASONING" && inst.info.State == StateReady {
			result = append(result, inst.info)
		}
	}
	return result
}

// GetAPEModules returns all APE (Active Participant Engine) modules
func (m *Manager) GetAPEModules() []*ModuleInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*ModuleInfo
	for _, inst := range m.modules {
		if inst.info.Manifest.Type == "APE" && inst.info.State == StateReady {
			result = append(result, inst.info)
		}
	}
	return result
}

// GetCRUDModules returns all modules that support CRUD operations.
// This includes modules with type "CRUD" and modules that declare "CRUD"
// in their additional_services (e.g., an INGESTION module with CRUD capability).
func (m *Manager) GetCRUDModules() []*ModuleInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*ModuleInfo
	for _, inst := range m.modules {
		if inst.info.State != StateReady {
			continue
		}
		if inst.info.CRUDClient != nil {
			result = append(result, inst.info)
		}
	}
	return result
}

// loadModule loads a single module from its directory
func (m *Manager) loadModule(moduleDir string) error {
	// Read manifest
	manifestPath := filepath.Join(moduleDir, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Validate manifest
	if manifest.ID == "" {
		return fmt.Errorf("manifest missing id")
	}
	if manifest.Binary == "" {
		return fmt.Errorf("manifest missing binary")
	}

	// Set defaults
	if manifest.HealthCheckIntervalMs <= 0 {
		manifest.HealthCheckIntervalMs = int(defaultHealthCheckInterval / time.Millisecond)
	}
	if manifest.StartupTimeoutMs <= 0 {
		manifest.StartupTimeoutMs = int(defaultStartupTimeout / time.Millisecond)
	}

	// Check binary exists
	binaryPath := filepath.Join(moduleDir, manifest.Binary)
	if _, err := os.Stat(binaryPath); err != nil {
		return fmt.Errorf("binary not found: %s", binaryPath)
	}

	// Create module info
	socketPath := filepath.Join(m.socketDir, fmt.Sprintf("mdemg-%s.sock", manifest.ID))
	info := &ModuleInfo{
		Manifest:   manifest,
		State:      StateStarting,
		SocketPath: socketPath,
	}

	inst := &moduleInstance{
		info:           info,
		stopHealthLoop: make(chan struct{}),
	}

	m.mu.Lock()
	m.modules[manifest.ID] = inst
	m.mu.Unlock()

	// Start the module
	if err := m.startModuleInstance(inst, binaryPath); err != nil {
		inst.info.State = StateCrashed
		inst.info.LastError = err.Error()
		return err
	}

	return nil
}

// startModuleInstance spawns the module binary and performs handshake
// reapOrphanPluginProcesses kills any process from a previous server
// generation still holding this module's socket path (MCP-REVIVE-001
// hygiene). `launchctl kickstart -k` kills the server without running
// Manager.Stop, orphaning plugin children — 3 stale generations (Apr 30,
// May 1, May 7) were observed live. Matching the FULL socket path via
// pgrep -f keeps the kill surgical: only plugin binaries carry it on
// their command line. Called before spawning, so the new child never
// matches its own reap.
func reapOrphanPluginProcesses(socketPath string) {
	out, err := exec.Command("pgrep", "-f", socketPath).Output()
	if err != nil {
		// pgrep exits 1 on no-match — the common, healthy case.
		return
	}
	for _, pidStr := range strings.Fields(string(out)) {
		pid, convErr := strconv.Atoi(pidStr)
		if convErr != nil || pid <= 1 || pid == os.Getpid() {
			continue
		}
		slog.Warn("plugins: reaping orphaned plugin process from a previous server generation",
			"pid", pid, "socket", socketPath)
		if proc, findErr := os.FindProcess(pid); findErr == nil {
			_ = proc.Kill()
		}
	}
}

func (m *Manager) startModuleInstance(inst *moduleInstance, binaryPath string) error {
	info := inst.info

	// Reap orphans from prior server generations, then remove the stale socket
	reapOrphanPluginProcesses(info.SocketPath)
	os.Remove(info.SocketPath)

	// Spawn binary
	cmd := exec.CommandContext(m.ctx, binaryPath, "--socket", info.SocketPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start binary: %w", err)
	}

	inst.cmd = cmd
	info.PID = cmd.Process.Pid
	info.StartedAt = time.Now()

	slog.Info("plugins: started module", "module", info.Manifest.ID, "pid", info.PID, "socket", info.SocketPath)

	// Wait for socket to become available
	timeout := time.Duration(info.Manifest.StartupTimeoutMs) * time.Millisecond
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(info.SocketPath); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Connect via gRPC
	conn, err := grpc.NewClient(
		"unix://"+info.SocketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		cmd.Process.Kill()
		return fmt.Errorf("failed to connect to module: %w", err)
	}
	inst.conn = conn

	// Create clients based on module type
	info.LifecycleClient = pb.NewModuleLifecycleClient(conn)
	switch info.Manifest.Type {
	case "INGESTION":
		info.IngestionClient = pb.NewIngestionModuleClient(conn)
	case "REASONING":
		info.ReasoningClient = pb.NewReasoningModuleClient(conn)
	case "APE":
		info.APEClient = pb.NewAPEModuleClient(conn)
	case "CRUD":
		info.CRUDClient = pb.NewCRUDModuleClient(conn)
	}

	// Wire additional services declared in manifest
	for _, svc := range info.Manifest.AdditionalServices {
		switch svc {
		case "CRUD":
			if info.CRUDClient == nil {
				info.CRUDClient = pb.NewCRUDModuleClient(conn)
			}
		}
	}

	// Perform handshake
	ctx, cancel := context.WithTimeout(m.ctx, timeout)
	defer cancel()

	resp, err := info.LifecycleClient.Handshake(ctx, &pb.HandshakeRequest{
		MdemgVersion: m.mdemgVer,
		SocketPath:   info.SocketPath,
		Config:       info.Manifest.Config,
	})
	if err != nil {
		_ = conn.Close()
		_ = cmd.Process.Kill()
		return fmt.Errorf("handshake failed: %w", err)
	}

	if !resp.Ready {
		_ = conn.Close()
		_ = cmd.Process.Kill()
		return fmt.Errorf("module not ready: %s", resp.Error)
	}

	info.State = StateReady
	info.LastHealthy = time.Now()
	slog.Info("plugins: module ready", "module", info.Manifest.ID, "version", resp.ModuleVersion, "capabilities", resp.Capabilities)

	// Start health check loop
	m.startHealthLoop(inst)

	// Monitor process exit
	m.wg.Add(1)
	go m.monitorProcess(inst)

	return nil
}

// stopModuleInstance gracefully stops a module
func (m *Manager) stopModuleInstance(inst *moduleInstance) error {
	info := inst.info
	info.State = StateStopping

	// Stop health loop
	close(inst.stopHealthLoop)

	// Send shutdown request
	if info.LifecycleClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), defaultShutdownTimeout)
		defer cancel()

		_, err := info.LifecycleClient.Shutdown(ctx, &pb.ShutdownRequest{
			TimeoutMs: int32(defaultShutdownTimeout / time.Millisecond),
			Reason:    "server_stop",
		})
		if err != nil {
			slog.Warn("plugins: shutdown request failed", "module", info.Manifest.ID, "error", err)
		}
	}

	// Close gRPC connection
	if inst.conn != nil {
		_ = inst.conn.Close()
	}

	// Kill process if still running
	if inst.cmd != nil && inst.cmd.Process != nil {
		inst.cmd.Process.Kill()
	}

	// Remove socket
	os.Remove(info.SocketPath)

	info.State = StateStopped
	return nil
}

// StopModuleByID gracefully stops a single module by ID.
func (m *Manager) StopModuleByID(id string) error {
	m.mu.Lock()
	inst, ok := m.modules[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("module %q not found", id)
	}
	m.mu.Unlock()

	if inst.info.State == StateStopped || inst.info.State == StateStopping {
		return nil
	}
	return m.stopModuleInstance(inst)
}

// StartModuleByID starts (or restarts) a single module by ID.
// The module must have been previously loaded.
func (m *Manager) StartModuleByID(id string) error {
	m.mu.RLock()
	inst, ok := m.modules[id]
	if !ok {
		m.mu.RUnlock()
		return fmt.Errorf("module %q not found", id)
	}
	m.mu.RUnlock()

	if inst.info.State == StateReady || inst.info.State == StateStarting {
		return nil // already running
	}

	binaryPath := filepath.Join(m.pluginsDir, inst.info.Manifest.ID, inst.info.Manifest.Binary)
	inst.stopHealthLoop = make(chan struct{})
	return m.startModuleInstance(inst, binaryPath)
}

// RestartModuleByID stops then starts a module by ID.
func (m *Manager) RestartModuleByID(id string) error {
	if err := m.StopModuleByID(id); err != nil {
		return fmt.Errorf("stop failed: %w", err)
	}
	// Brief pause for socket cleanup
	time.Sleep(200 * time.Millisecond)
	return m.StartModuleByID(id)
}

// startHealthLoop starts periodic health checks for a module
func (m *Manager) startHealthLoop(inst *moduleInstance) {
	interval := time.Duration(inst.info.Manifest.HealthCheckIntervalMs) * time.Millisecond
	inst.healthTicker = time.NewTicker(interval)

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer inst.healthTicker.Stop()

		for {
			select {
			case <-inst.stopHealthLoop:
				return
			case <-inst.healthTicker.C:
				m.checkHealth(inst)
			}
		}
	}()
}

// checkHealth performs a single health check
func (m *Manager) checkHealth(inst *moduleInstance) {
	info := inst.info
	if info.LifecycleClient == nil {
		return
	}

	ctx, cancel := context.WithTimeout(m.ctx, 2*time.Second)
	defer cancel()

	resp, err := info.LifecycleClient.HealthCheck(ctx, &pb.HealthCheckRequest{})
	if err != nil {
		info.State = StateUnhealthy
		info.LastError = fmt.Sprintf("health check failed: %v", err)
		slog.Warn("plugins: health check failed", "module", info.Manifest.ID, "error", err)
		return
	}

	if !resp.Healthy {
		info.State = StateUnhealthy
		info.LastError = resp.Status
		return
	}

	info.State = StateReady
	info.LastHealthy = time.Now()
	info.Metrics = resp.Metrics
	info.LastError = ""
}

// monitorProcess watches for process exit and handles restarts
func (m *Manager) monitorProcess(inst *moduleInstance) {
	defer m.wg.Done()

	if inst.cmd == nil {
		return
	}

	err := inst.cmd.Wait()
	info := inst.info

	// Don't restart if we're shutting down
	select {
	case <-m.ctx.Done():
		info.State = StateStopped
		return
	default:
	}

	info.State = StateCrashed
	if err != nil {
		info.LastError = fmt.Sprintf("process exited: %v", err)
	} else {
		info.LastError = "process exited unexpectedly"
	}
	slog.Error("plugins: module crashed", "module", info.Manifest.ID, "error", info.LastError)

	// Attempt restart with backoff
	if inst.restartCount < maxRestartAttempts {
		inst.restartCount++
		backoff := restartBackoffBase * time.Duration(inst.restartCount)

		slog.Info("plugins: restarting module", "module", info.Manifest.ID, "backoff", backoff, "attempt", inst.restartCount, "max_attempts", maxRestartAttempts)

		time.Sleep(backoff)

		binaryPath := filepath.Join(m.pluginsDir, info.Manifest.ID, info.Manifest.Binary)
		if err := m.startModuleInstance(inst, binaryPath); err != nil {
			slog.Error("plugins: restart failed", "module", info.Manifest.ID, "error", err)
			info.LastError = fmt.Sprintf("restart failed: %v", err)
		} else {
			inst.lastRestartAt = time.Now()
		}
	} else {
		slog.Error("plugins: module exceeded max restart attempts, giving up", "module", info.Manifest.ID)
	}
}

// MatchIngestionModule finds a module that can handle the given source
func (m *Manager) MatchIngestionModule(ctx context.Context, sourceURI, contentType string) (*ModuleInfo, float32, error) {
	modules := m.GetIngestionModules()
	if len(modules) == 0 {
		return nil, 0, nil
	}

	var bestModule *ModuleInfo
	var bestConfidence float32

	for _, mod := range modules {
		if mod.IngestionClient == nil {
			continue
		}

		resp, err := mod.IngestionClient.Matches(ctx, &pb.MatchRequest{
			SourceUri:   sourceURI,
			ContentType: contentType,
		})
		if err != nil {
			slog.Warn("plugins: match check failed", "module", mod.Manifest.ID, "error", err)
			continue
		}

		if resp.Matches && resp.Confidence > bestConfidence {
			bestModule = mod
			bestConfidence = resp.Confidence
		}
	}

	return bestModule, bestConfidence, nil
}
