package main

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
	moduleID      = "test-reasoning-plugin"
	moduleVersion = "1.0.0"
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
	handler := NewTestReasoningPluginHandler()
	pb.RegisterModuleLifecycleServer(grpcServer, handler)
	pb.RegisterReasoningModuleServer(grpcServer, handler)

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
