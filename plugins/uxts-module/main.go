// Package main implements the UxTS Framework REASONING plugin for MDEMG.
// It annotates retrieval candidates with test coverage, spec compliance,
// and drift detection metadata from across all UxTS frameworks.
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

	listener, err := net.Listen("unix", *socketPath)
	if err != nil {
		slog.Error("failed to listen on socket", "error", err)
		os.Exit(1)
	}
	defer listener.Close()
	defer os.Remove(*socketPath)

	slog.Info("listening", "module", moduleID, "socket", *socketPath)

	grpcServer := grpc.NewServer()
	s := newServer()

	pb.RegisterModuleLifecycleServer(grpcServer, s)
	pb.RegisterReasoningModuleServer(grpcServer, s)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-sigChan
		slog.Info("received shutdown signal", "module", moduleID)
		grpcServer.GracefulStop()
	}()

	if err := grpcServer.Serve(listener); err != nil {
		slog.Error("failed to serve", "error", err)
		os.Exit(1)
	}
}
