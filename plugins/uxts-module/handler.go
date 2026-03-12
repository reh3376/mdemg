package main

import (
	"context"
	"log"
	"strconv"
	"sync"
	"time"

	pb "mdemg/api/modulepb"
)

const moduleID = "uxts-module"
const moduleVersion = "1.0.0"

type server struct {
	pb.UnimplementedModuleLifecycleServer
	pb.UnimplementedReasoningModuleServer

	mu              sync.Mutex
	startTime       time.Time
	requestsHandled int64
	specIndex       *SpecIndex
	config          pluginConfig
	resultClient    *ResultClient
	eventHandler    *EventHandler
}

type pluginConfig struct {
	specRoot        string
	frameworks      string
	mdemgEndpoint   string
	spaceID         string
	enableDrift     bool
	maxSpecsPerEvt  int
	coverageBoost   float64
	failurePenalty  float64
}

func newServer() *server {
	return &server{
		startTime: time.Now(),
		specIndex: NewSpecIndex(),
		config: pluginConfig{
			specRoot:        "./docs",
			frameworks:      "uats,udts,upts,ubts,usts,uobs,uots,uets,uvts",
			mdemgEndpoint:   "http://localhost:9999",
			spaceID:         "mdemg-dev",
			enableDrift:     true,
			maxSpecsPerEvt:  50,
			coverageBoost:   0.02,
			failurePenalty:  0.05,
		},
	}
}

func (s *server) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	log.Printf("%s: handshake from MDEMG %s", moduleID, req.MdemgVersion)

	// Parse config from manifest
	if v, ok := req.Config["spec_root"]; ok && v != "" {
		s.config.specRoot = v
	}
	if v, ok := req.Config["frameworks"]; ok && v != "" {
		s.config.frameworks = v
	}
	if v, ok := req.Config["mdemg_endpoint"]; ok && v != "" {
		s.config.mdemgEndpoint = v
	}
	if v, ok := req.Config["space_id"]; ok && v != "" {
		s.config.spaceID = v
	}
	if v, ok := req.Config["enable_drift_check"]; ok {
		s.config.enableDrift = v == "true"
	}
	if v, ok := req.Config["max_specs_per_event"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			s.config.maxSpecsPerEvt = n
		}
	}
	if v, ok := req.Config["coverage_boost"]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			s.config.coverageBoost = f
		}
	}
	if v, ok := req.Config["failure_penalty"]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			s.config.failurePenalty = f
		}
	}

	// Build spec index
	if err := s.specIndex.Load(s.config.specRoot, s.config.frameworks, s.config.enableDrift); err != nil {
		log.Printf("%s: warning: spec index load: %v", moduleID, err)
	}

	log.Printf("%s: loaded %d specs across %d frameworks",
		moduleID, s.specIndex.TotalSpecs(), s.specIndex.FrameworkCount())

	// Initialize result client
	s.resultClient = NewResultClient(s.config.mdemgEndpoint, s.config.spaceID)

	// Start event handler (background polling)
	s.eventHandler = NewEventHandler(s.specIndex, s.resultClient, s.config)
	go s.eventHandler.Start()

	return &pb.HandshakeResponse{
		ModuleId:      moduleID,
		ModuleVersion: moduleVersion,
		ModuleType:    pb.ModuleType_MODULE_TYPE_REASONING,
		Capabilities:  []string{"test_coverage", "spec_compliance", "drift_detection"},
		Ready:         true,
	}, nil
}

func (s *server) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	s.mu.Lock()
	requests := s.requestsHandled
	s.mu.Unlock()

	driftCount := 0
	if s.config.enableDrift {
		driftCount = s.specIndex.DriftCount()
	}

	return &pb.HealthCheckResponse{
		Healthy: true,
		Status:  "ready",
		Metrics: map[string]string{
			"uptime":           time.Since(s.startTime).String(),
			"requests_handled": strconv.FormatInt(requests, 10),
			"total_specs":      strconv.Itoa(s.specIndex.TotalSpecs()),
			"frameworks":       strconv.Itoa(s.specIndex.FrameworkCount()),
			"drift_detected":   strconv.Itoa(driftCount),
		},
	}, nil
}

func (s *server) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	log.Printf("%s: shutdown requested (reason: %s)", moduleID, req.Reason)

	if s.eventHandler != nil {
		s.eventHandler.Stop()
	}

	return &pb.ShutdownResponse{
		Success: true,
		Message: "shutting down gracefully",
	}, nil
}
