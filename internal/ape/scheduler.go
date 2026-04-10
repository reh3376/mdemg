package ape

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	pb "mdemg/api/modulepb"
	"mdemg/internal/plugins"
)

// DefaultCheckInterval is the default interval between scheduled task checks.
const DefaultCheckInterval = 30 * time.Second

// Scheduler manages execution of APE (Active Participant Engine) modules.
// It handles scheduled tasks and event-triggered executions.
type Scheduler struct {
	pluginMgr *plugins.Manager

	mu          sync.RWMutex
	schedules   map[string]*moduleSchedule // moduleID -> schedule
	lastRun     map[string]time.Time       // moduleID -> last execution time
	runningTask map[string]bool            // moduleID -> currently running

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// checkInterval is the interval between scheduled task checks.
	// Defaults to DefaultCheckInterval (30 seconds).
	checkInterval time.Duration
}

type moduleSchedule struct {
	ModuleID       string
	CronExpression string   // e.g., "0 * * * *" for hourly
	EventTriggers  []string // e.g., ["ingest", "session_end"]
	MinInterval    time.Duration
	nextRun        time.Time
}

// NewScheduler creates a new APE scheduler
func NewScheduler(pluginMgr *plugins.Manager) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		pluginMgr:     pluginMgr,
		schedules:     make(map[string]*moduleSchedule),
		lastRun:       make(map[string]time.Time),
		runningTask:   make(map[string]bool),
		ctx:           ctx,
		cancel:        cancel,
		checkInterval: DefaultCheckInterval,
	}
}

// SetCheckInterval sets the interval between scheduled task checks.
// This is primarily useful for testing. Must be called before Start().
func (s *Scheduler) SetCheckInterval(d time.Duration) {
	s.checkInterval = d
}

// Start initializes schedules from APE modules and starts the scheduler loop
func (s *Scheduler) Start() error {
	if s.pluginMgr == nil {
		slog.Info("ape: no plugin manager, scheduler disabled")
		return nil
	}

	// Fetch schedules from all APE modules
	if err := s.refreshSchedules(); err != nil {
		slog.Error("ape: failed to refresh schedules", "error", err)
	}

	// Start scheduler loop
	s.wg.Add(1)
	go s.schedulerLoop()

	slog.Info("ape: scheduler started", "module_count", len(s.schedules))
	return nil
}

// Stop gracefully shuts down the scheduler
func (s *Scheduler) Stop() {
	s.cancel()
	s.wg.Wait()
	slog.Info("ape: scheduler stopped")
}

// TriggerEvent triggers all APE modules subscribed to the given event
func (s *Scheduler) TriggerEvent(event string) {
	s.TriggerEventWithContext(event, nil)
}

// TriggerEventWithContext triggers all APE modules subscribed to the given event,
// passing additional context (e.g., space_id, ingest_type) to the module execution.
func (s *Scheduler) TriggerEventWithContext(event string, ctx map[string]string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for moduleID, sched := range s.schedules {
		for _, trigger := range sched.EventTriggers {
			if trigger == event {
				go s.executeModuleWithContext(moduleID, "event:"+event, ctx)
				break
			}
		}
	}
}

// refreshSchedules queries all APE modules for their schedules
func (s *Scheduler) refreshSchedules() error {
	modules := s.pluginMgr.GetAPEModules()
	if len(modules) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, mod := range modules {
		if mod.APEClient == nil {
			continue
		}

		ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
		resp, err := mod.APEClient.GetSchedule(ctx, &pb.GetScheduleRequest{})
		cancel()

		if err != nil {
			slog.Error("ape: failed to get schedule", "module_id", mod.Manifest.ID, "error", err)
			continue
		}

		sched := &moduleSchedule{
			ModuleID:       mod.Manifest.ID,
			CronExpression: resp.CronExpression,
			EventTriggers:  resp.EventTriggers,
			MinInterval:    time.Duration(resp.MinIntervalSeconds) * time.Second,
		}

		// Calculate next run time from cron
		if sched.CronExpression != "" {
			sched.nextRun = s.parseNextCronRun(sched.CronExpression)
		}

		s.schedules[mod.Manifest.ID] = sched
		slog.Info("ape: registered module", "module_id", mod.Manifest.ID, "cron", sched.CronExpression, "events", sched.EventTriggers, "min_interval", sched.MinInterval)
	}

	return nil
}

// schedulerLoop runs the main scheduling loop
func (s *Scheduler) schedulerLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.checkScheduledTasks()
		}
	}
}

// checkScheduledTasks checks if any scheduled tasks should run
func (s *Scheduler) checkScheduledTasks() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()

	for moduleID, sched := range s.schedules {
		// Skip if no cron schedule
		if sched.CronExpression == "" {
			continue
		}

		// Skip if not yet time
		if now.Before(sched.nextRun) {
			continue
		}

		// Check minimum interval
		if lastRun, ok := s.lastRun[moduleID]; ok {
			if now.Sub(lastRun) < sched.MinInterval {
				continue
			}
		}

		// Execute in goroutine
		go s.executeModule(moduleID, "schedule")

		// Update next run time
		sched.nextRun = s.parseNextCronRun(sched.CronExpression)
	}
}

// executeModule runs an APE module with no additional context.
func (s *Scheduler) executeModule(moduleID, trigger string) {
	s.executeModuleWithContext(moduleID, trigger, nil)
}

// executeModuleWithContext runs an APE module, passing eventCtx to the ExecuteRequest.
func (s *Scheduler) executeModuleWithContext(moduleID, trigger string, eventCtx map[string]string) {
	// Check if already running
	s.mu.Lock()
	if s.runningTask[moduleID] {
		s.mu.Unlock()
		slog.Info("ape: skipping module, already running", "module_id", moduleID)
		return
	}
	s.runningTask[moduleID] = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.runningTask[moduleID] = false
		s.lastRun[moduleID] = time.Now()
		s.mu.Unlock()
	}()

	// Get the module
	modInfo, ok := s.pluginMgr.GetModule(moduleID)
	if !ok || modInfo.APEClient == nil {
		slog.Warn("ape: module not found or not APE type", "module_id", moduleID)
		return
	}

	taskID := uuid.New().String()
	slog.Info("ape: executing module", "module_id", moduleID, "task_id", taskID[:8], "trigger", trigger)

	// Merge event context into request context
	reqCtx := make(map[string]string)
	for k, v := range eventCtx {
		reqCtx[k] = v
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Minute)
	defer cancel()

	resp, err := modInfo.APEClient.Execute(ctx, &pb.ExecuteRequest{
		TaskId:  taskID,
		Trigger: trigger,
		Context: reqCtx,
	})

	elapsed := time.Since(start)

	if err != nil {
		slog.Error("ape: module execution failed", "module_id", moduleID, "elapsed", elapsed, "error", err)
		return
	}

	if resp.Error != "" {
		slog.Error("ape: module completed with error", "module_id", moduleID, "elapsed", elapsed, "error", resp.Error)
		return
	}

	if resp.Stats != nil {
		slog.Info("ape: module completed", "module_id", moduleID, "elapsed", elapsed, "nodes_created", resp.Stats.NodesCreated, "nodes_updated", resp.Stats.NodesUpdated, "edges_created", resp.Stats.EdgesCreated, "edges_updated", resp.Stats.EdgesUpdated)
	} else {
		slog.Info("ape: module completed", "module_id", moduleID, "elapsed", elapsed, "message", resp.Message)
	}
}

// parseNextCronRun parses a cron expression and returns the next run time.
// Simplified implementation supporting: minute hour day month weekday
// Examples: "0 * * * *" (hourly), "*/15 * * * *" (every 15 min), "0 0 * * *" (daily)
// Note: server.go has a separate parseCronInterval() that returns a Duration — different purpose.
func (s *Scheduler) parseNextCronRun(cron string) time.Time {
	// Simplified: parse common patterns
	now := time.Now()

	switch {
	case cron == "* * * * *": // Every minute
		return now.Add(1 * time.Minute).Truncate(time.Minute)

	case cron == "*/5 * * * *": // Every 5 minutes
		next := now.Truncate(5 * time.Minute).Add(5 * time.Minute)
		return next

	case cron == "*/15 * * * *": // Every 15 minutes
		next := now.Truncate(15 * time.Minute).Add(15 * time.Minute)
		return next

	case cron == "*/30 * * * *": // Every 30 minutes
		next := now.Truncate(30 * time.Minute).Add(30 * time.Minute)
		return next

	case cron == "0 * * * *": // Every hour
		next := now.Truncate(time.Hour).Add(time.Hour)
		return next

	case cron == "0 */2 * * *": // Every 2 hours
		next := now.Truncate(2 * time.Hour).Add(2 * time.Hour)
		return next

	case cron == "0 */6 * * *": // Every 6 hours
		next := now.Truncate(6 * time.Hour).Add(6 * time.Hour)
		return next

	case cron == "0 0 * * *": // Daily at midnight
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		return next

	default:
		// Default: run in 1 hour
		return now.Add(1 * time.Hour)
	}
}

// GetStatus returns the current scheduler status
func (s *Scheduler) GetStatus() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	modules := make([]map[string]any, 0, len(s.schedules))
	for moduleID, sched := range s.schedules {
		m := map[string]any{
			"module_id":       moduleID,
			"cron_expression": sched.CronExpression,
			"event_triggers":  sched.EventTriggers,
			"min_interval":    sched.MinInterval.String(),
			"next_run":        sched.nextRun.Format(time.RFC3339),
			"running":         s.runningTask[moduleID],
		}
		if lastRun, ok := s.lastRun[moduleID]; ok {
			m["last_run"] = lastRun.Format(time.RFC3339)
		}
		modules = append(modules, m)
	}

	return map[string]any{
		"enabled": s.pluginMgr != nil,
		"modules": modules,
	}
}
