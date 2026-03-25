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
	moduleID      = "obsidian-module"
	moduleVersion = "1.0.0"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	socketPath := flag.String("socket", "", "Unix socket path")
	flag.Parse()

	if *socketPath == "" {
		slog.Error("--socket is required")
		os.Exit(1)
	}

	os.Remove(*socketPath)

	listener, err := net.Listen("unix", *socketPath)
	if err != nil {
		slog.Error("failed to listen", "error", err)
		os.Exit(1)
	}
	defer listener.Close()
	defer os.Remove(*socketPath)

	slog.Info("obsidian module listening", "socket", *socketPath)

	grpcServer := grpc.NewServer()

	handler := NewObsidianHandler()
	pb.RegisterModuleLifecycleServer(grpcServer, handler)
	pb.RegisterIngestionModuleServer(grpcServer, handler)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-sigChan
		slog.Info("received shutdown signal")
		grpcServer.GracefulStop()
	}()

	if err := grpcServer.Serve(listener); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
