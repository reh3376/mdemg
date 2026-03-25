package unts

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	pb "mdemg/api/untspb"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server implements the HashVerification gRPC service
type Server struct {
	pb.UnimplementedHashVerificationServer
	registry *Registry
	scanner  *Scanner
}

// NewServer creates a new UNTS server
func NewServer(basePath string) (*Server, error) {
	registry := NewRegistry(basePath)
	if err := registry.Load(); err != nil {
		return nil, fmt.Errorf("load registry: %w", err)
	}

	scanner := NewScanner(registry, basePath)

	return &Server{
		registry: registry,
		scanner:  scanner,
	}, nil
}

// ScanAndSync scans manifest and UDTS specs, syncing to registry
func (s *Server) ScanAndSync() error {
	if err := s.scanner.ScanAll(); err != nil {
		return err
	}
	return s.registry.Save()
}

// ListVerifiedFiles returns all tracked files with their current hash and status
func (s *Server) ListVerifiedFiles(ctx context.Context, req *pb.ListVerifiedFilesRequest) (*pb.ListVerifiedFilesResponse, error) {
	framework := fromProtoFramework(req.Framework)
	statusFilter := ""
	if req.Status != pb.FileStatus_FILE_STATUS_UNSPECIFIED {
		statusFilter = fromProtoStatus(req.Status)
	}

	files := s.registry.List(framework, statusFilter)

	pbFiles := make([]*pb.VerifiedFileRecord, len(files))
	var verified, mismatch int32
	for i, f := range files {
		pbFiles[i] = f.ToProto()
		switch f.Status {
		case "verified":
			verified++
		case "mismatch":
			mismatch++
		}
	}

	return &pb.ListVerifiedFilesResponse{
		Files:         pbFiles,
		TotalCount:    int32(len(files)),
		VerifiedCount: verified,
		MismatchCount: mismatch,
	}, nil
}

// GetFileStatus returns the status of a single tracked file
func (s *Server) GetFileStatus(ctx context.Context, req *pb.GetFileStatusRequest) (*pb.GetFileStatusResponse, error) {
	if req.Path == "" {
		return nil, status.Error(codes.InvalidArgument, "path is required")
	}

	_, ok := s.registry.Get(req.Path)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "file not tracked: %s", req.Path)
	}

	// Optionally compute actual hash
	var actualHash string
	result, err := s.registry.Verify(req.Path)
	if err == nil && result != nil {
		actualHash = result.ActualHash
	}

	// Re-fetch after verify (it may have updated status)
	f, _ := s.registry.Get(req.Path)

	return &pb.GetFileStatusResponse{
		Record:     f.ToProto(),
		ActualHash: actualHash,
	}, nil
}

// GetHashHistory returns the current hash and last 3 historical hashes
func (s *Server) GetHashHistory(ctx context.Context, req *pb.GetHashHistoryRequest) (*pb.GetHashHistoryResponse, error) {
	if req.Path == "" {
		return nil, status.Error(codes.InvalidArgument, "path is required")
	}

	f, ok := s.registry.Get(req.Path)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "file not tracked: %s", req.Path)
	}

	history := make([]*pb.HashHistoryEntry, len(f.History))
	for i, h := range f.History {
		history[i] = &pb.HashHistoryEntry{
			Hash:      h.Hash,
			UpdatedAt: h.UpdatedAt,
			Source:    toProtoSource(h.Source),
		}
	}

	return &pb.GetHashHistoryResponse{
		Path:        f.Path,
		CurrentHash: f.CurrentHash,
		UpdatedAt:   f.UpdatedAt,
		History:     history,
	}, nil
}

// RevertToPreviousHash sets the expected hash back to a previous value
func (s *Server) RevertToPreviousHash(ctx context.Context, req *pb.RevertToPreviousHashRequest) (*pb.RevertToPreviousHashResponse, error) {
	if req.Path == "" {
		return nil, status.Error(codes.InvalidArgument, "path is required")
	}

	f, ok := s.registry.Get(req.Path)
	if !ok {
		return &pb.RevertToPreviousHashResponse{
			Ok:    false,
			Error: fmt.Sprintf("file not tracked: %s", req.Path),
		}, nil
	}

	var targetHash string
	switch t := req.Target.(type) {
	case *pb.RevertToPreviousHashRequest_TargetHash:
		targetHash = t.TargetHash
	case *pb.RevertToPreviousHashRequest_HistoryIndex:
		idx := int(t.HistoryIndex)
		if idx < 0 || idx >= len(f.History) {
			return &pb.RevertToPreviousHashResponse{
				Ok:    false,
				Error: fmt.Sprintf("history index %d out of range (0-%d)", idx, len(f.History)-1),
			}, nil
		}
		targetHash = f.History[idx].Hash
	default:
		return &pb.RevertToPreviousHashResponse{
			Ok:    false,
			Error: "target hash or history index required",
		}, nil
	}

	if err := s.registry.RevertHash(req.Path, targetHash); err != nil {
		return &pb.RevertToPreviousHashResponse{
			Ok:    false,
			Error: err.Error(),
		}, nil
	}

	// Log audit info
	slog.Info("UNTS revert", "path", req.Path, "target_hash", targetHash, "reverted_by", req.RevertedBy, "reason", req.Reason)

	// Save registry
	if err := s.registry.Save(); err != nil {
		slog.Warn("UNTS save registry after revert failed", "error", err)
	}

	return &pb.RevertToPreviousHashResponse{
		Ok:             true,
		NewCurrentHash: targetHash,
		UpdatedAt:      time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// UpdateHash manually updates the expected hash for a file
func (s *Server) UpdateHash(ctx context.Context, req *pb.UpdateHashRequest) (*pb.UpdateHashResponse, error) {
	if req.Path == "" {
		return nil, status.Error(codes.InvalidArgument, "path is required")
	}
	if req.NewHash == "" {
		return nil, status.Error(codes.InvalidArgument, "new_hash is required")
	}
	if len(req.NewHash) != 64 {
		return nil, status.Error(codes.InvalidArgument, "new_hash must be 64 hex characters")
	}

	source := fromProtoSource(req.Source)
	if source == "" {
		source = "manual"
	}

	if err := s.registry.UpdateHash(req.Path, req.NewHash, source); err != nil {
		return &pb.UpdateHashResponse{
			Ok:    false,
			Error: err.Error(),
		}, nil
	}

	// Log audit info
	slog.Info("UNTS update hash", "path", req.Path, "source", source, "updated_by", req.UpdatedBy, "reason", req.Reason)

	// Save registry
	if err := s.registry.Save(); err != nil {
		slog.Warn("UNTS save registry after update failed", "error", err)
	}

	return &pb.UpdateHashResponse{
		Ok:        true,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// VerifyNow recomputes file hashes and compares to expected values
func (s *Server) VerifyNow(ctx context.Context, req *pb.VerifyNowRequest) (*pb.VerifyNowResponse, error) {
	var results []*VerifyResult

	if req.Path != "" {
		// Single file
		result, err := s.registry.Verify(req.Path)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "verify %s: %v", req.Path, err)
		}
		results = []*VerifyResult{result}
	} else {
		// All files, optionally filtered by framework
		framework := fromProtoFramework(req.Framework)
		results = s.registry.VerifyAll(framework)
	}

	// Save updated status
	if err := s.registry.Save(); err != nil {
		slog.Warn("UNTS save registry after verify failed", "error", err)
	}

	pbResults := make([]*pb.FileVerifyResult, len(results))
	var verified, mismatch, errCount int32
	for i, r := range results {
		pbResults[i] = &pb.FileVerifyResult{
			Path:         r.Path,
			Status:       toProtoStatus(r.Status),
			ExpectedHash: r.ExpectedHash,
			ActualHash:   r.ActualHash,
			UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
			Error:        r.Error,
		}
		switch r.Status {
		case "verified":
			verified++
		case "mismatch":
			mismatch++
		case "unknown":
			errCount++
		}
	}

	return &pb.VerifyNowResponse{
		Results:       pbResults,
		VerifiedCount: verified,
		MismatchCount: mismatch,
		ErrorCount:    errCount,
	}, nil
}

// RegisterTrackedFile adds a new file to the registry
func (s *Server) RegisterTrackedFile(ctx context.Context, req *pb.RegisterTrackedFileRequest) (*pb.RegisterTrackedFileResponse, error) {
	if req.Path == "" {
		return nil, status.Error(codes.InvalidArgument, "path is required")
	}
	if req.InitialHash == "" {
		return nil, status.Error(codes.InvalidArgument, "initial_hash is required")
	}
	if len(req.InitialHash) != 64 {
		return nil, status.Error(codes.InvalidArgument, "initial_hash must be 64 hex characters")
	}

	framework := fromProtoFramework(req.Framework)
	if framework == "" {
		framework = "manifest"
	}

	if err := s.registry.Register(req.Path, framework, req.InitialHash, req.SourceRef, "manual"); err != nil {
		return &pb.RegisterTrackedFileResponse{
			Ok:    false,
			Error: err.Error(),
		}, nil
	}

	// Log audit info
	slog.Info("UNTS register tracked file", "path", req.Path, "framework", framework, "registered_by", req.RegisteredBy)

	// Save registry
	if err := s.registry.Save(); err != nil {
		return &pb.RegisterTrackedFileResponse{
			Ok:    false,
			Error: fmt.Sprintf("save registry: %v", err),
		}, nil
	}

	return &pb.RegisterTrackedFileResponse{
		Ok: true,
	}, nil
}

// Helper to convert proto status to string
func fromProtoStatus(s pb.FileStatus) string {
	switch s {
	case pb.FileStatus_FILE_STATUS_VERIFIED:
		return "verified"
	case pb.FileStatus_FILE_STATUS_MISMATCH:
		return "mismatch"
	case pb.FileStatus_FILE_STATUS_UNKNOWN:
		return "unknown"
	case pb.FileStatus_FILE_STATUS_REVERTED:
		return "reverted"
	default:
		return ""
	}
}
