package main

import (
	"flag"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	pb "mdemg/api/modulepb"
)

const (
	moduleID      = "test-ingestion-plugin"
	moduleVersion = "1.0.0"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	socketPath := flag.String("socket", "", "Unix socket path")
	flag.Parse()

	if *socketPath == "" {
		slog.Error("missing required flag", "flag", "--socket")
		os.Exit(1)
	}

	// Remove stale socket
	os.Remove(*socketPath)

	// Create Unix socket listener
	listener, err := net.Listen("unix", *socketPath)
	if err != nil {
		slog.Error("failed to listen", "error", err)
		os.Exit(1)
	}
	defer listener.Close()
	defer os.Remove(*socketPath)

	slog.Info("listening", "module", moduleID, "socket", *socketPath)

	// Create gRPC server
	grpcServer := grpc.NewServer()

	// Create and register module handler
	handler := NewTestIngestionPluginHandler()
	pb.RegisterModuleLifecycleServer(grpcServer, handler)
	pb.RegisterIngestionModuleServer(grpcServer, handler)

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-sigChan
		slog.Info("received shutdown signal", "module", moduleID)
		grpcServer.GracefulStop()
	}()

	// Start serving
	if err := grpcServer.Serve(listener); err != nil {
		slog.Error("failed to serve", "error", err)
		os.Exit(1)
	}
}
