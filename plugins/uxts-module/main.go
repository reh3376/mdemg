// Package main implements the UxTS Framework REASONING plugin for MDEMG.
// It annotates retrieval candidates with test coverage, spec compliance,
// and drift detection metadata from across all UxTS frameworks.
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

func main() {
	socketPath := flag.String("socket", "", "Unix socket path")
	flag.Parse()

	if *socketPath == "" {
		log.Fatal("--socket flag is required")
	}

	// Remove stale socket
	os.Remove(*socketPath)

	listener, err := net.Listen("unix", *socketPath)
	if err != nil {
		log.Fatalf("Failed to listen on socket: %v", err)
	}
	defer listener.Close()
	defer os.Remove(*socketPath)

	log.Printf("%s: listening on %s", moduleID, *socketPath)

	grpcServer := grpc.NewServer()
	s := newServer()

	pb.RegisterModuleLifecycleServer(grpcServer, s)
	pb.RegisterReasoningModuleServer(grpcServer, s)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-sigChan
		log.Printf("%s: received shutdown signal", moduleID)
		grpcServer.GracefulStop()
	}()

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
