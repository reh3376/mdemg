package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	pb "mdemg/api/modulepb"
)

// Handshake is called immediately after spawn to verify module is ready.
func (h *DocsScraperHandler) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	slog.Info("handshake received", "module", moduleID, "mdemg_version", req.MdemgVersion)

	h.mu.Lock()
	h.config = req.Config
	h.mu.Unlock()

	// Apply config overrides
	h.applyConfig(req.Config)

	return &pb.HandshakeResponse{
		ModuleId:      moduleID,
		ModuleVersion: moduleVersion,
		ModuleType:    pb.ModuleType_MODULE_TYPE_INGESTION,
		Capabilities:  []string{"https://", "http://", "text/html"},
		Ready:         true,
	}, nil
}

// HealthCheck is called periodically to verify module health.
func (h *DocsScraperHandler) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	return &pb.HealthCheckResponse{
		Healthy: true,
		Status:  "ok",
		Metrics: map[string]string{
			"uptime":           time.Since(h.startTime).String(),
			"requests_handled": fmt.Sprintf("%d", atomic.LoadUint64(&h.requestCount)),
			"pages_scraped":    fmt.Sprintf("%d", atomic.LoadUint64(&h.pagesScraped)),
		},
	}, nil
}

// Shutdown is called when MDEMG is stopping or the module is being disabled.
func (h *DocsScraperHandler) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	slog.Info("shutdown requested", "module", moduleID, "reason", req.Reason)
	return &pb.ShutdownResponse{
		Success: true,
		Message: "goodbye",
	}, nil
}
