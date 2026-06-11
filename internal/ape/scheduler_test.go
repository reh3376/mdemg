package ape

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	pb "mdemg/api/modulepb"
	"mdemg/internal/plugins"

	"google.golang.org/grpc"
)

// =============================================================================
// Mock APE Module Client for testing
// =============================================================================

// mockAPEModuleClient implements pb.APEModuleClient for testing
type mockAPEModuleClient struct {
	getScheduleResp *pb.GetScheduleResponse
	getScheduleErr  error
	executeResp     *pb.ExecuteResponse
	executeErr      error
	executeCalled   bool
	executeReq      *pb.ExecuteRequest
	mu              sync.Mutex
}

func (m *mockAPEModuleClient) Execute(ctx context.Context, in *pb.ExecuteRequest, opts ...grpc.CallOption) (*pb.ExecuteResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executeCalled = true
	m.executeReq = in
	return m.executeResp, m.executeErr
}

func (m *mockAPEModuleClient) GetSchedule(ctx context.Context, in *pb.GetScheduleRequest, opts ...grpc.CallOption) (*pb.GetScheduleResponse, error) {
	return m.getScheduleResp, m.getScheduleErr
}

func (m *mockAPEModuleClient) wasExecuteCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.executeCalled
}

func (m *mockAPEModuleClient) getExecuteRequest() *pb.ExecuteRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.executeReq
}

// createMockAPEModule creates a mock APE module with the given client
func createMockAPEModule(id string, client pb.APEModuleClient) *plugins.ModuleInfo {
	return &plugins.ModuleInfo{
		Manifest: plugins.Manifest{
			ID:   id,
			Type: "APE",
		},
		State:     plugins.StateReady,
		APEClient: client,
	}
}

// =============================================================================
// parseNextCronRun tests - covers all 9 cron patterns
// =============================================================================

func TestParseNextCronRun_EveryMinute(t *testing.T) {
	s := &Scheduler{}
	before := time.Now()
	next := s.parseNextCronRun("* * * * *")
	after := time.Now()

	// Should be about 1 minute from now, truncated to minute boundary
	expected := before.Add(1 * time.Minute).Truncate(time.Minute)
	if next.Before(expected.Add(-1*time.Second)) || next.After(after.Add(1*time.Minute).Add(1*time.Second)) {
		t.Errorf("parseNextCronRun('* * * * *') = %v, want ~%v", next, expected)
	}
}

func TestParseNextCronRun_Every5Minutes(t *testing.T) {
	s := &Scheduler{}
	now := time.Now()
	next := s.parseNextCronRun("*/5 * * * *")

	// Should be next 5-minute boundary
	expected := now.Truncate(5 * time.Minute).Add(5 * time.Minute)
	if !next.Equal(expected) {
		t.Errorf("parseNextCronRun('*/5 * * * *') = %v, want %v", next, expected)
	}
}

func TestParseNextCronRun_Every15Minutes(t *testing.T) {
	s := &Scheduler{}
	now := time.Now()
	next := s.parseNextCronRun("*/15 * * * *")

	expected := now.Truncate(15 * time.Minute).Add(15 * time.Minute)
	if !next.Equal(expected) {
		t.Errorf("parseNextCronRun('*/15 * * * *') = %v, want %v", next, expected)
	}
}

func TestParseNextCronRun_Every30Minutes(t *testing.T) {
	s := &Scheduler{}
	now := time.Now()
	next := s.parseNextCronRun("*/30 * * * *")

	expected := now.Truncate(30 * time.Minute).Add(30 * time.Minute)
	if !next.Equal(expected) {
		t.Errorf("parseNextCronRun('*/30 * * * *') = %v, want %v", next, expected)
	}
}

func TestParseNextCronRun_Hourly(t *testing.T) {
	s := &Scheduler{}
	now := time.Now()
	next := s.parseNextCronRun("0 * * * *")

	expected := now.Truncate(time.Hour).Add(time.Hour)
	if !next.Equal(expected) {
		t.Errorf("parseNextCronRun('0 * * * *') = %v, want %v", next, expected)
	}
}

func TestParseNextCronRun_Every2Hours(t *testing.T) {
	s := &Scheduler{}
	now := time.Now()
	next := s.parseNextCronRun("0 */2 * * *")

	expected := now.Truncate(2 * time.Hour).Add(2 * time.Hour)
	if !next.Equal(expected) {
		t.Errorf("parseNextCronRun('0 */2 * * *') = %v, want %v", next, expected)
	}
}

func TestParseNextCronRun_Every6Hours(t *testing.T) {
	s := &Scheduler{}
	now := time.Now()
	next := s.parseNextCronRun("0 */6 * * *")

	expected := now.Truncate(6 * time.Hour).Add(6 * time.Hour)
	if !next.Equal(expected) {
		t.Errorf("parseNextCronRun('0 */6 * * *') = %v, want %v", next, expected)
	}
}

func TestParseNextCronRun_DailyMidnight(t *testing.T) {
	s := &Scheduler{}
	now := time.Now()
	next := s.parseNextCronRun("0 0 * * *")

	expected := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	if !next.Equal(expected) {
		t.Errorf("parseNextCronRun('0 0 * * *') = %v, want %v", next, expected)
	}
}

func TestParseNextCronRun_UnknownPattern(t *testing.T) {
	s := &Scheduler{}
	now := time.Now()
	next := s.parseNextCronRun("0 3 * * 1") // Unknown pattern: 3am on Mondays

	// Default: 1 hour from now
	if next.Before(now.Add(59*time.Minute)) || next.After(now.Add(61*time.Minute)) {
		t.Errorf("parseNextCronRun('0 3 * * 1') = %v, want ~1 hour from %v", next, now)
	}
}

// =============================================================================
// NewScheduler tests
// =============================================================================

func TestNewScheduler_WithManager(t *testing.T) {
	mgr := plugins.NewManager("", "", "")
	s := NewScheduler(mgr)

	if s.pluginMgr != mgr {
		t.Error("NewScheduler did not set pluginMgr")
	}
	if s.schedules == nil {
		t.Error("NewScheduler did not initialize schedules map")
	}
	if s.lastRun == nil {
		t.Error("NewScheduler did not initialize lastRun map")
	}
	if s.runningTask == nil {
		t.Error("NewScheduler did not initialize runningTask map")
	}
	if s.ctx == nil {
		t.Error("NewScheduler did not create context")
	}
	if s.cancel == nil {
		t.Error("NewScheduler did not create cancel func")
	}
}

func TestNewScheduler_NilManager(t *testing.T) {
	s := NewScheduler(nil)

	if s.pluginMgr != nil {
		t.Error("NewScheduler should allow nil pluginMgr")
	}
	// Maps should still be initialized
	if s.schedules == nil || s.lastRun == nil || s.runningTask == nil {
		t.Error("NewScheduler should initialize maps even with nil manager")
	}
}

// =============================================================================
// GetStatus tests
// =============================================================================

func TestGetStatus_EmptyScheduler(t *testing.T) {
	mgr := plugins.NewManager("", "", "")
	s := NewScheduler(mgr)

	status := s.GetStatus()

	if status["enabled"] != true {
		t.Errorf("GetStatus enabled = %v, want true", status["enabled"])
	}
	modules, ok := status["modules"].([]map[string]any)
	if !ok {
		t.Error("GetStatus modules should be []map[string]any")
	}
	if len(modules) != 0 {
		t.Errorf("GetStatus modules len = %d, want 0", len(modules))
	}
}

func TestGetStatus_NilManager(t *testing.T) {
	s := NewScheduler(nil)

	status := s.GetStatus()

	if status["enabled"] != false {
		t.Errorf("GetStatus enabled = %v, want false for nil manager", status["enabled"])
	}
}

func TestGetStatus_WithSchedules(t *testing.T) {
	mgr := plugins.NewManager("", "", "")
	s := NewScheduler(mgr)

	// Add a test schedule
	now := time.Now()
	s.schedules["test-module"] = &moduleSchedule{
		ModuleID:       "test-module",
		CronExpression: "0 * * * *",
		EventTriggers:  []string{"ingest", "session_end"},
		MinInterval:    5 * time.Minute,
		nextRun:        now.Add(1 * time.Hour),
	}
	s.lastRun["test-module"] = now.Add(-30 * time.Minute)
	s.runningTask["test-module"] = false

	status := s.GetStatus()

	modules := status["modules"].([]map[string]any)
	if len(modules) != 1 {
		t.Fatalf("GetStatus modules len = %d, want 1", len(modules))
	}

	m := modules[0]
	if m["module_id"] != "test-module" {
		t.Errorf("module_id = %v, want test-module", m["module_id"])
	}
	if m["cron_expression"] != "0 * * * *" {
		t.Errorf("cron_expression = %v, want 0 * * * *", m["cron_expression"])
	}
	if m["running"] != false {
		t.Errorf("running = %v, want false", m["running"])
	}
	if _, ok := m["last_run"]; !ok {
		t.Error("last_run should be present")
	}
	if m["min_interval"] != "5m0s" {
		t.Errorf("min_interval = %v, want 5m0s", m["min_interval"])
	}
}

func TestGetStatus_RunningTask(t *testing.T) {
	mgr := plugins.NewManager("", "", "")
	s := NewScheduler(mgr)

	s.schedules["running-module"] = &moduleSchedule{
		ModuleID:       "running-module",
		CronExpression: "*/5 * * * *",
		nextRun:        time.Now().Add(5 * time.Minute),
	}
	s.runningTask["running-module"] = true

	status := s.GetStatus()
	modules := status["modules"].([]map[string]any)

	if len(modules) != 1 {
		t.Fatalf("Expected 1 module, got %d", len(modules))
	}
	if modules[0]["running"] != true {
		t.Errorf("running = %v, want true", modules[0]["running"])
	}
}

// =============================================================================
// Start/Stop lifecycle tests
// =============================================================================

func TestStart_NilManager(t *testing.T) {
	s := NewScheduler(nil)

	err := s.Start()
	if err != nil {
		t.Errorf("Start with nil manager should not error, got: %v", err)
	}
}

func TestStart_EmptyManager(t *testing.T) {
	mgr := plugins.NewManager("", "", "")
	s := NewScheduler(mgr)

	err := s.Start()
	if err != nil {
		t.Errorf("Start with empty manager should not error, got: %v", err)
	}

	// Clean up
	s.Stop()
}

func TestStop_GracefulShutdown(t *testing.T) {
	mgr := plugins.NewManager("", "", "")
	s := NewScheduler(mgr)

	if err := s.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give scheduler loop time to start
	time.Sleep(50 * time.Millisecond)

	// Stop should complete without hanging
	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Stop did not complete within 2 seconds")
	}
}

func TestStop_ContextCancellation(t *testing.T) {
	mgr := plugins.NewManager("", "", "")
	s := NewScheduler(mgr)

	if err := s.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	s.Stop()

	// Context should be cancelled
	select {
	case <-s.ctx.Done():
		// Expected
	default:
		t.Error("Context should be cancelled after Stop")
	}
}

// =============================================================================
// TriggerEvent tests (extending existing tests)
// =============================================================================

func TestTriggerEventWithContext_ContextPassed(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	s := &Scheduler{
		pluginMgr:   mgr,
		schedules:   make(map[string]*moduleSchedule),
		lastRun:     make(map[string]time.Time),
		runningTask: make(map[string]bool),
	}

	// Register a schedule that listens for "ingest_complete"
	s.schedules["test-module"] = &moduleSchedule{
		ModuleID:      "test-module",
		EventTriggers: []string{"ingest_complete", "source_changed"},
	}

	ctx := map[string]string{
		"space_id":    "test-space",
		"ingest_type": "batch-ingest",
	}

	// TriggerEventWithContext matches the event and spawns a goroutine.
	// The module won't be found (no real plugins loaded), so it logs and returns.
	s.TriggerEventWithContext("ingest_complete", ctx)

	// Give goroutine time to start and exit
	time.Sleep(50 * time.Millisecond)
}

func TestTriggerEventWithContext_NoMatch(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	s := &Scheduler{
		pluginMgr:   mgr,
		schedules:   make(map[string]*moduleSchedule),
		lastRun:     make(map[string]time.Time),
		runningTask: make(map[string]bool),
	}

	// Register a schedule that listens for "session_end" only
	s.schedules["test-module"] = &moduleSchedule{
		ModuleID:      "test-module",
		EventTriggers: []string{"session_end"},
	}

	// Trigger a different event - should not match, no goroutine spawned
	s.TriggerEventWithContext("ingest_complete", map[string]string{
		"space_id": "test-space",
	})

	time.Sleep(50 * time.Millisecond)
}

func TestTriggerEvent_DelegatesToWithContext(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	s := &Scheduler{
		pluginMgr:   mgr,
		schedules:   make(map[string]*moduleSchedule),
		lastRun:     make(map[string]time.Time),
		runningTask: make(map[string]bool),
	}

	// TriggerEvent delegates to TriggerEventWithContext(nil) — should not panic
	s.TriggerEvent("test_event")
	time.Sleep(50 * time.Millisecond)
}

func TestTriggerEventWithContext_MultipleModules(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	s := &Scheduler{
		pluginMgr:   mgr,
		schedules:   make(map[string]*moduleSchedule),
		lastRun:     make(map[string]time.Time),
		runningTask: make(map[string]bool),
	}

	// Register multiple modules listening to the same event
	s.schedules["module-a"] = &moduleSchedule{
		ModuleID:      "module-a",
		EventTriggers: []string{"ingest"},
	}
	s.schedules["module-b"] = &moduleSchedule{
		ModuleID:      "module-b",
		EventTriggers: []string{"ingest", "other"},
	}
	s.schedules["module-c"] = &moduleSchedule{
		ModuleID:      "module-c",
		EventTriggers: []string{"other"},
	}

	// Trigger "ingest" - should match module-a and module-b
	s.TriggerEventWithContext("ingest", nil)
	time.Sleep(50 * time.Millisecond)
}

// =============================================================================
// executeModuleWithContext tests
// =============================================================================

func TestExecuteModuleWithContext_NilContext(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	s := &Scheduler{
		pluginMgr:   mgr,
		schedules:   make(map[string]*moduleSchedule),
		lastRun:     make(map[string]time.Time),
		runningTask: make(map[string]bool),
	}

	// Module won't be found, logs and returns without panic
	s.executeModuleWithContext("nonexistent-module", "test", nil)
}

func TestExecuteModuleWithContext_EmptyContext(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	s := &Scheduler{
		pluginMgr:   mgr,
		schedules:   make(map[string]*moduleSchedule),
		lastRun:     make(map[string]time.Time),
		runningTask: make(map[string]bool),
	}

	s.executeModuleWithContext("nonexistent-module", "test", map[string]string{})
}

func TestExecuteModuleWithContext_AlreadyRunning(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	s := &Scheduler{
		pluginMgr:   mgr,
		schedules:   make(map[string]*moduleSchedule),
		lastRun:     make(map[string]time.Time),
		runningTask: make(map[string]bool),
	}

	// Mark as already running
	s.runningTask["test-module"] = true

	// Should return immediately without error
	s.executeModuleWithContext("test-module", "test", nil)

	// Should still be marked as running (not cleared)
	if !s.runningTask["test-module"] {
		t.Error("runningTask should still be true after skipping execution")
	}
}

func TestExecuteModuleWithContext_SetsAndClearsRunning(t *testing.T) {
	mgr := plugins.NewManager("", "", "")
	ctx, cancel := context.WithCancel(context.Background())

	s := &Scheduler{
		pluginMgr:   mgr,
		schedules:   make(map[string]*moduleSchedule),
		lastRun:     make(map[string]time.Time),
		runningTask: make(map[string]bool),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Execute a nonexistent module - it will fail but should clean up state
	s.executeModuleWithContext("nonexistent-module", "test", nil)

	// After execution completes, runningTask should be cleared
	if s.runningTask["nonexistent-module"] {
		t.Error("runningTask should be false after execution completes")
	}

	// lastRun should be set
	if _, ok := s.lastRun["nonexistent-module"]; !ok {
		t.Error("lastRun should be set after execution")
	}
}

// =============================================================================
// checkScheduledTasks tests
// =============================================================================

func TestCheckScheduledTasks_NoCronSchedule(t *testing.T) {
	mgr := plugins.NewManager("", "", "")
	s := NewScheduler(mgr)

	// Add module with no cron expression
	s.schedules["event-only"] = &moduleSchedule{
		ModuleID:      "event-only",
		EventTriggers: []string{"ingest"},
	}

	// Should not panic or trigger execution
	s.checkScheduledTasks()
}

func TestCheckScheduledTasks_NotYetTime(t *testing.T) {
	mgr := plugins.NewManager("", "", "")
	s := NewScheduler(mgr)

	// Add module with nextRun in the future
	s.schedules["future-task"] = &moduleSchedule{
		ModuleID:       "future-task",
		CronExpression: "0 * * * *",
		nextRun:        time.Now().Add(1 * time.Hour),
	}

	// Should not trigger execution
	s.checkScheduledTasks()
	time.Sleep(50 * time.Millisecond)

	// No lastRun should be set
	if _, ok := s.lastRun["future-task"]; ok {
		t.Error("lastRun should not be set for future task")
	}
}

func TestCheckScheduledTasks_MinIntervalNotMet(t *testing.T) {
	mgr := plugins.NewManager("", "", "")
	s := NewScheduler(mgr)

	now := time.Now()
	s.schedules["frequent-task"] = &moduleSchedule{
		ModuleID:       "frequent-task",
		CronExpression: "* * * * *",
		MinInterval:    10 * time.Minute,
		nextRun:        now.Add(-1 * time.Minute), // Due
	}
	// Last run was 5 minutes ago, but min interval is 10 minutes
	s.lastRun["frequent-task"] = now.Add(-5 * time.Minute)

	// Should not trigger execution due to min interval
	s.checkScheduledTasks()
	time.Sleep(50 * time.Millisecond)

	// lastRun should still be the old time
	if s.lastRun["frequent-task"].After(now.Add(-4 * time.Minute)) {
		t.Error("lastRun should not have been updated")
	}
}

// =============================================================================
// refreshSchedules tests
// =============================================================================

func TestRefreshSchedules_NoModules(t *testing.T) {
	mgr := plugins.NewManager("", "", "")
	s := NewScheduler(mgr)

	err := s.refreshSchedules()
	if err != nil {
		t.Errorf("refreshSchedules with no modules should not error: %v", err)
	}

	if len(s.schedules) != 0 {
		t.Errorf("schedules should be empty, got %d", len(s.schedules))
	}
}

func TestRefreshSchedules_WithMockAPEModules(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	// Create mock APE client that returns a schedule
	mockClient := &mockAPEModuleClient{
		getScheduleResp: &pb.GetScheduleResponse{
			CronExpression:     "*/15 * * * *",
			EventTriggers:      []string{"ingest", "session_end"},
			MinIntervalSeconds: 300,
		},
	}

	// Inject mock module into manager
	mockModule := createMockAPEModule("test-ape-module", mockClient)
	mgr.InjectModuleForTest(mockModule)

	s := NewScheduler(mgr)

	err := s.refreshSchedules()
	if err != nil {
		t.Errorf("refreshSchedules should not error: %v", err)
	}

	if len(s.schedules) != 1 {
		t.Fatalf("expected 1 schedule, got %d", len(s.schedules))
	}

	sched := s.schedules["test-ape-module"]
	if sched == nil {
		t.Fatal("schedule for test-ape-module not found")
	}

	if sched.CronExpression != "*/15 * * * *" {
		t.Errorf("CronExpression = %q, want */15 * * * *", sched.CronExpression)
	}

	if len(sched.EventTriggers) != 2 {
		t.Errorf("EventTriggers len = %d, want 2", len(sched.EventTriggers))
	}

	if sched.MinInterval != 300*time.Second {
		t.Errorf("MinInterval = %v, want 5m0s", sched.MinInterval)
	}

	// nextRun should be set since we have a cron expression
	if sched.nextRun.IsZero() {
		t.Error("nextRun should be set")
	}
}

func TestRefreshSchedules_NilAPEClient(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	// Create module with nil APEClient
	mockModule := &plugins.ModuleInfo{
		Manifest: plugins.Manifest{
			ID:   "nil-client-module",
			Type: "APE",
		},
		State:     plugins.StateReady,
		APEClient: nil, // nil client
	}
	mgr.InjectModuleForTest(mockModule)

	s := NewScheduler(mgr)

	err := s.refreshSchedules()
	if err != nil {
		t.Errorf("refreshSchedules should not error with nil client: %v", err)
	}

	// Should skip the module with nil client
	if len(s.schedules) != 0 {
		t.Errorf("expected 0 schedules (nil client skipped), got %d", len(s.schedules))
	}
}

func TestRefreshSchedules_GetScheduleError(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	// Create mock APE client that returns an error
	mockClient := &mockAPEModuleClient{
		getScheduleErr: errors.New("connection refused"),
	}

	mockModule := createMockAPEModule("error-module", mockClient)
	mgr.InjectModuleForTest(mockModule)

	s := NewScheduler(mgr)

	// Should not return error, just log and continue
	err := s.refreshSchedules()
	if err != nil {
		t.Errorf("refreshSchedules should not propagate GetSchedule error: %v", err)
	}

	// Module should not be added to schedules
	if len(s.schedules) != 0 {
		t.Errorf("expected 0 schedules (error module skipped), got %d", len(s.schedules))
	}
}

func TestRefreshSchedules_MultipleModules(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	// Create multiple mock APE modules
	mockClient1 := &mockAPEModuleClient{
		getScheduleResp: &pb.GetScheduleResponse{
			CronExpression: "0 * * * *",
			EventTriggers:  []string{"ingest"},
		},
	}
	mockClient2 := &mockAPEModuleClient{
		getScheduleResp: &pb.GetScheduleResponse{
			CronExpression: "0 0 * * *",
			EventTriggers:  []string{"daily"},
		},
	}

	mgr.InjectModuleForTest(createMockAPEModule("module-1", mockClient1))
	mgr.InjectModuleForTest(createMockAPEModule("module-2", mockClient2))

	s := NewScheduler(mgr)

	err := s.refreshSchedules()
	if err != nil {
		t.Errorf("refreshSchedules should not error: %v", err)
	}

	if len(s.schedules) != 2 {
		t.Errorf("expected 2 schedules, got %d", len(s.schedules))
	}
}

func TestRefreshSchedules_EventOnlyModule(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	// Create module with no cron expression (event-only)
	mockClient := &mockAPEModuleClient{
		getScheduleResp: &pb.GetScheduleResponse{
			CronExpression: "", // No cron
			EventTriggers:  []string{"ingest"},
		},
	}

	mgr.InjectModuleForTest(createMockAPEModule("event-only-module", mockClient))

	s := NewScheduler(mgr)

	err := s.refreshSchedules()
	if err != nil {
		t.Errorf("refreshSchedules should not error: %v", err)
	}

	sched := s.schedules["event-only-module"]
	if sched == nil {
		t.Fatal("schedule should exist")
	}

	// nextRun should be zero since no cron expression
	if !sched.nextRun.IsZero() {
		t.Error("nextRun should be zero for event-only module")
	}
}

// =============================================================================
// executeModuleWithContext tests with mock modules
// =============================================================================

func TestExecuteModuleWithContext_SuccessWithStats(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	mockClient := &mockAPEModuleClient{
		executeResp: &pb.ExecuteResponse{
			Success: true,
			Message: "completed successfully",
			Stats: &pb.ExecuteStats{
				NodesCreated: 10,
				NodesUpdated: 5,
				EdgesCreated: 8,
				EdgesUpdated: 3,
				DurationMs:   150,
			},
		},
	}

	mockModule := createMockAPEModule("success-module", mockClient)
	mgr.InjectModuleForTest(mockModule)

	ctx, cancel := context.WithCancel(context.Background())
	s := &Scheduler{
		pluginMgr:   mgr,
		schedules:   make(map[string]*moduleSchedule),
		lastRun:     make(map[string]time.Time),
		runningTask: make(map[string]bool),
		ctx:         ctx,
		cancel:      cancel,
	}

	s.executeModuleWithContext("success-module", "test-trigger", map[string]string{
		"space_id": "test-space",
	})

	// Verify execute was called
	if !mockClient.wasExecuteCalled() {
		t.Error("Execute should have been called")
	}

	// Verify request parameters
	req := mockClient.getExecuteRequest()
	if req == nil {
		t.Fatal("Execute request should not be nil")
	}

	if req.Trigger != "test-trigger" {
		t.Errorf("Trigger = %q, want test-trigger", req.Trigger)
	}

	if req.Context["space_id"] != "test-space" {
		t.Errorf("Context[space_id] = %q, want test-space", req.Context["space_id"])
	}

	// Verify lastRun was set
	if _, ok := s.lastRun["success-module"]; !ok {
		t.Error("lastRun should be set after execution")
	}

	// Verify runningTask was cleared
	if s.runningTask["success-module"] {
		t.Error("runningTask should be false after execution")
	}
}

func TestExecuteModuleWithContext_SuccessWithMessage(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	mockClient := &mockAPEModuleClient{
		executeResp: &pb.ExecuteResponse{
			Success: true,
			Message: "processed 100 items",
			Stats:   nil, // No stats, just message
		},
	}

	mockModule := createMockAPEModule("message-module", mockClient)
	mgr.InjectModuleForTest(mockModule)

	ctx, cancel := context.WithCancel(context.Background())
	s := &Scheduler{
		pluginMgr:   mgr,
		schedules:   make(map[string]*moduleSchedule),
		lastRun:     make(map[string]time.Time),
		runningTask: make(map[string]bool),
		ctx:         ctx,
		cancel:      cancel,
	}

	s.executeModuleWithContext("message-module", "schedule", nil)

	if !mockClient.wasExecuteCalled() {
		t.Error("Execute should have been called")
	}
}

func TestExecuteModuleWithContext_ExecuteError(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	mockClient := &mockAPEModuleClient{
		executeErr: errors.New("execution failed"),
	}

	mockModule := createMockAPEModule("error-module", mockClient)
	mgr.InjectModuleForTest(mockModule)

	ctx, cancel := context.WithCancel(context.Background())
	s := &Scheduler{
		pluginMgr:   mgr,
		schedules:   make(map[string]*moduleSchedule),
		lastRun:     make(map[string]time.Time),
		runningTask: make(map[string]bool),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Should not panic
	s.executeModuleWithContext("error-module", "test", nil)

	// lastRun should still be set (defer runs)
	if _, ok := s.lastRun["error-module"]; !ok {
		t.Error("lastRun should be set even after error")
	}

	// runningTask should be cleared
	if s.runningTask["error-module"] {
		t.Error("runningTask should be false after error")
	}
}

func TestExecuteModuleWithContext_ResponseError(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	mockClient := &mockAPEModuleClient{
		executeResp: &pb.ExecuteResponse{
			Success: false,
			Error:   "database connection failed",
		},
	}

	mockModule := createMockAPEModule("resp-error-module", mockClient)
	mgr.InjectModuleForTest(mockModule)

	ctx, cancel := context.WithCancel(context.Background())
	s := &Scheduler{
		pluginMgr:   mgr,
		schedules:   make(map[string]*moduleSchedule),
		lastRun:     make(map[string]time.Time),
		runningTask: make(map[string]bool),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Should handle response error gracefully
	s.executeModuleWithContext("resp-error-module", "test", nil)

	if !mockClient.wasExecuteCalled() {
		t.Error("Execute should have been called")
	}
}

func TestExecuteModuleWithContext_NilAPEClient(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	// Module with nil APEClient
	mockModule := &plugins.ModuleInfo{
		Manifest: plugins.Manifest{
			ID:   "nil-client",
			Type: "APE",
		},
		State:     plugins.StateReady,
		APEClient: nil,
	}
	mgr.InjectModuleForTest(mockModule)

	ctx, cancel := context.WithCancel(context.Background())
	s := &Scheduler{
		pluginMgr:   mgr,
		schedules:   make(map[string]*moduleSchedule),
		lastRun:     make(map[string]time.Time),
		runningTask: make(map[string]bool),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Should return early without panic
	s.executeModuleWithContext("nil-client", "test", nil)

	// lastRun should still be set from defer
	if _, ok := s.lastRun["nil-client"]; !ok {
		t.Error("lastRun should be set")
	}
}

// =============================================================================
// schedulerLoop tests
// =============================================================================

func TestSchedulerLoop_ContextCancellation(t *testing.T) {
	mgr := plugins.NewManager("", "", "")
	s := NewScheduler(mgr)

	// Start the scheduler
	if err := s.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give the loop time to start
	time.Sleep(50 * time.Millisecond)

	// Cancel context via Stop
	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()

	// Should exit promptly
	select {
	case <-done:
		// Success - loop exited on context cancellation
	case <-time.After(2 * time.Second):
		t.Error("schedulerLoop did not exit on context cancellation")
	}
}

// =============================================================================
// checkScheduledTasks tests - execution trigger path
// =============================================================================

func TestCheckScheduledTasks_TriggersExecution(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	mockClient := &mockAPEModuleClient{
		executeResp: &pb.ExecuteResponse{
			Success: true,
			Message: "executed",
		},
	}

	mockModule := createMockAPEModule("due-module", mockClient)
	mgr.InjectModuleForTest(mockModule)

	ctx, cancel := context.WithCancel(context.Background())
	s := &Scheduler{
		pluginMgr:   mgr,
		schedules:   make(map[string]*moduleSchedule),
		lastRun:     make(map[string]time.Time),
		runningTask: make(map[string]bool),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Set up a task that is due (nextRun in the past)
	s.schedules["due-module"] = &moduleSchedule{
		ModuleID:       "due-module",
		CronExpression: "* * * * *",
		nextRun:        time.Now().Add(-1 * time.Minute), // Past due
		MinInterval:    0,
	}

	// Check scheduled tasks - should trigger execution
	s.checkScheduledTasks()

	// Give goroutine time to execute
	time.Sleep(100 * time.Millisecond)

	if !mockClient.wasExecuteCalled() {
		t.Error("Execute should have been called for due task")
	}

	// nextRun should be updated to a future time
	sched := s.schedules["due-module"]
	if sched.nextRun.Before(time.Now()) {
		t.Error("nextRun should be updated to future time after execution")
	}
}

func TestCheckScheduledTasks_MinIntervalMet(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	mockClient := &mockAPEModuleClient{
		executeResp: &pb.ExecuteResponse{
			Success: true,
			Message: "executed",
		},
	}

	mockModule := createMockAPEModule("interval-module", mockClient)
	mgr.InjectModuleForTest(mockModule)

	ctx, cancel := context.WithCancel(context.Background())
	now := time.Now()

	s := &Scheduler{
		pluginMgr:   mgr,
		schedules:   make(map[string]*moduleSchedule),
		lastRun:     make(map[string]time.Time),
		runningTask: make(map[string]bool),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Set up task with min interval that IS met
	s.schedules["interval-module"] = &moduleSchedule{
		ModuleID:       "interval-module",
		CronExpression: "* * * * *",
		nextRun:        now.Add(-1 * time.Minute),
		MinInterval:    5 * time.Minute,
	}
	// Last run was 10 minutes ago, min interval is 5 minutes - should run
	s.lastRun["interval-module"] = now.Add(-10 * time.Minute)

	s.checkScheduledTasks()
	time.Sleep(100 * time.Millisecond)

	if !mockClient.wasExecuteCalled() {
		t.Error("Execute should have been called when min interval is met")
	}
}

// =============================================================================
// Start error path tests
// =============================================================================

func TestStart_RefreshSchedulesError(t *testing.T) {
	// This test verifies that Start handles errors from refreshSchedules gracefully.
	// Since refreshSchedules doesn't return errors to Start (it logs them),
	// we test that Start completes successfully even when modules have issues.

	mgr := plugins.NewManager("", "", "")

	// Create module that will error on GetSchedule
	mockClient := &mockAPEModuleClient{
		getScheduleErr: errors.New("schedule fetch failed"),
	}
	mgr.InjectModuleForTest(createMockAPEModule("failing-module", mockClient))

	s := NewScheduler(mgr)

	// Start should not fail even with refresh errors
	err := s.Start()
	if err != nil {
		t.Errorf("Start should not fail due to refresh errors: %v", err)
	}

	// Clean up
	s.Stop()
}

func TestStart_WithWorkingModules(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	mockClient := &mockAPEModuleClient{
		getScheduleResp: &pb.GetScheduleResponse{
			CronExpression: "*/5 * * * *",
			EventTriggers:  []string{"ingest"},
		},
	}
	mgr.InjectModuleForTest(createMockAPEModule("working-module", mockClient))

	s := NewScheduler(mgr)

	err := s.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Verify schedules were loaded
	s.mu.RLock()
	schedLen := len(s.schedules)
	s.mu.RUnlock()

	if schedLen != 1 {
		t.Errorf("expected 1 schedule, got %d", schedLen)
	}

	s.Stop()
}

// =============================================================================
// TriggerEventWithContext with mock module execution
// =============================================================================

func TestTriggerEventWithContext_ExecutesMatchingModule(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	mockClient := &mockAPEModuleClient{
		executeResp: &pb.ExecuteResponse{
			Success: true,
			Message: "event handled",
		},
	}

	mockModule := createMockAPEModule("event-handler", mockClient)
	mgr.InjectModuleForTest(mockModule)

	ctx, cancel := context.WithCancel(context.Background())
	s := &Scheduler{
		pluginMgr:   mgr,
		schedules:   make(map[string]*moduleSchedule),
		lastRun:     make(map[string]time.Time),
		runningTask: make(map[string]bool),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Register schedule with event trigger
	s.schedules["event-handler"] = &moduleSchedule{
		ModuleID:      "event-handler",
		EventTriggers: []string{"ingest_complete"},
	}

	// Trigger the event
	s.TriggerEventWithContext("ingest_complete", map[string]string{
		"space_id": "my-space",
	})

	// Wait for async execution
	time.Sleep(150 * time.Millisecond)

	if !mockClient.wasExecuteCalled() {
		t.Error("Execute should have been called for matching event")
	}

	req := mockClient.getExecuteRequest()
	if req == nil {
		t.Fatal("Execute request should not be nil")
	}

	if req.Trigger != "event:ingest_complete" {
		t.Errorf("Trigger = %q, want event:ingest_complete", req.Trigger)
	}

	if req.Context["space_id"] != "my-space" {
		t.Errorf("Context should contain space_id")
	}
}

// =============================================================================
// Edge case tests
// =============================================================================

func TestScheduler_ConcurrentExecution(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	// Create a slow mock client
	mockClient := &mockAPEModuleClient{
		executeResp: &pb.ExecuteResponse{
			Success: true,
		},
	}

	mgr.InjectModuleForTest(createMockAPEModule("slow-module", mockClient))

	ctx, cancel := context.WithCancel(context.Background())
	s := &Scheduler{
		pluginMgr:   mgr,
		schedules:   make(map[string]*moduleSchedule),
		lastRun:     make(map[string]time.Time),
		runningTask: make(map[string]bool),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Start first execution
	go s.executeModuleWithContext("slow-module", "first", nil)

	// Give it time to start and set runningTask
	time.Sleep(10 * time.Millisecond)

	// Mark as running to simulate concurrent call
	s.mu.Lock()
	s.runningTask["slow-module"] = true
	s.mu.Unlock()

	// Second call should be skipped
	s.executeModuleWithContext("slow-module", "second", nil)

	// The module should still be marked as running
	s.mu.RLock()
	running := s.runningTask["slow-module"]
	s.mu.RUnlock()

	if !running {
		t.Error("runningTask should still be true while first execution runs")
	}
}

func TestGetStatus_NoLastRun(t *testing.T) {
	mgr := plugins.NewManager("", "", "")
	s := NewScheduler(mgr)

	// Add schedule without lastRun
	s.schedules["new-module"] = &moduleSchedule{
		ModuleID:       "new-module",
		CronExpression: "0 * * * *",
		nextRun:        time.Now().Add(1 * time.Hour),
	}
	s.runningTask["new-module"] = false
	// Note: No s.lastRun["new-module"] set

	status := s.GetStatus()
	modules := status["modules"].([]map[string]any)

	if len(modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(modules))
	}

	// last_run should not be present
	if _, ok := modules[0]["last_run"]; ok {
		t.Error("last_run should not be present for module that never ran")
	}
}

func TestExecuteModuleWithContext_ContextMerge(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	mockClient := &mockAPEModuleClient{
		executeResp: &pb.ExecuteResponse{
			Success: true,
		},
	}

	mgr.InjectModuleForTest(createMockAPEModule("ctx-module", mockClient))

	ctx, cancel := context.WithCancel(context.Background())
	s := &Scheduler{
		pluginMgr:   mgr,
		schedules:   make(map[string]*moduleSchedule),
		lastRun:     make(map[string]time.Time),
		runningTask: make(map[string]bool),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Execute with multiple context values
	s.executeModuleWithContext("ctx-module", "event:test", map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	})

	time.Sleep(50 * time.Millisecond)

	req := mockClient.getExecuteRequest()
	if req == nil {
		t.Fatal("request should not be nil")
	}

	if len(req.Context) != 3 {
		t.Errorf("Context should have 3 entries, got %d", len(req.Context))
	}

	if req.Context["key1"] != "value1" {
		t.Errorf("Context[key1] = %q, want value1", req.Context["key1"])
	}
}

// =============================================================================
// Direct schedulerLoop ticker path test
// =============================================================================

// TestSchedulerLoop_TickerPath tests the ticker-triggered path in schedulerLoop.
// This test directly invokes the scheduler's internal loop behavior.
func TestSchedulerLoop_TickerPath(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	mockClient := &mockAPEModuleClient{
		executeResp: &pb.ExecuteResponse{
			Success: true,
			Message: "ticker executed",
		},
	}

	mockModule := createMockAPEModule("ticker-module", mockClient)
	mgr.InjectModuleForTest(mockModule)

	ctx, cancel := context.WithCancel(context.Background())
	s := &Scheduler{
		pluginMgr:   mgr,
		schedules:   make(map[string]*moduleSchedule),
		lastRun:     make(map[string]time.Time),
		runningTask: make(map[string]bool),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Add a due task
	s.schedules["ticker-module"] = &moduleSchedule{
		ModuleID:       "ticker-module",
		CronExpression: "* * * * *",
		nextRun:        time.Now().Add(-1 * time.Minute),
	}

	// Directly call checkScheduledTasks to simulate what the ticker would do
	// This covers the same code path as the ticker case in schedulerLoop
	s.checkScheduledTasks()

	time.Sleep(100 * time.Millisecond)

	if !mockClient.wasExecuteCalled() {
		t.Error("Execute should have been called via checkScheduledTasks")
	}
}

// TestSchedulerLoopInternal_TickerFires tests the ticker path in schedulerLoop
// by using a very short check interval.
func TestSchedulerLoopInternal_TickerFires(t *testing.T) {
	mgr := plugins.NewManager("", "", "")

	// Create mock with both GetSchedule and Execute responses
	mockClient := &mockAPEModuleClient{
		getScheduleResp: &pb.GetScheduleResponse{
			CronExpression:     "* * * * *",
			EventTriggers:      []string{},
			MinIntervalSeconds: 0,
		},
		executeResp: &pb.ExecuteResponse{
			Success: true,
		},
	}
	mockModule := createMockAPEModule("loop-test-module", mockClient)
	mgr.InjectModuleForTest(mockModule)

	s := NewScheduler(mgr)
	// Set a very fast check interval for testing
	s.SetCheckInterval(10 * time.Millisecond)

	// Start the scheduler - this will call refreshSchedules
	if err := s.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// After refreshSchedules, manually set nextRun to be in the past so the task is due
	s.mu.Lock()
	if sched, ok := s.schedules["loop-test-module"]; ok {
		sched.nextRun = time.Now().Add(-1 * time.Minute)
	}
	s.mu.Unlock()

	// Wait for ticker to fire (should happen within 20ms given 10ms interval)
	time.Sleep(50 * time.Millisecond)

	// Stop scheduler
	s.Stop()

	// Verify execute was called via the ticker path
	if !mockClient.wasExecuteCalled() {
		t.Error("Execute should have been called when ticker fires with due task")
	}
}

// TestSetCheckInterval verifies the SetCheckInterval function
func TestSetCheckInterval(t *testing.T) {
	mgr := plugins.NewManager("", "", "")
	s := NewScheduler(mgr)

	// Default should be 30 seconds
	if s.checkInterval != DefaultCheckInterval {
		t.Errorf("Default checkInterval = %v, want %v", s.checkInterval, DefaultCheckInterval)
	}

	// Set custom interval
	s.SetCheckInterval(5 * time.Second)
	if s.checkInterval != 5*time.Second {
		t.Errorf("checkInterval after set = %v, want 5s", s.checkInterval)
	}
}
