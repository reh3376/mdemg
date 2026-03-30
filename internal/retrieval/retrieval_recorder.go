package retrieval

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"
)

// RetrievalEvent captures a single Retrieve() pipeline for contrastive training data.
type RetrievalEvent struct {
	Time              time.Time
	EventID           string
	SpaceID           string
	CallSite          string
	QueryText         string
	QueryHash         string
	RecallNodeIDs     []string
	RecallScores      []float64
	RecallK           int
	BM25NodeIDs       []string
	BM25Scores        []float64
	RerankNodeIDs     []string
	RerankScores      []float64
	RerankModel       string
	ResultNodeIDs     []string
	ResultScores      []float64
	ResultCount       int
	GuidanceID        string
	DownstreamQuality float64
	RecallLatencyMs   int
	RerankLatencyMs   int
	TotalLatencyMs    int
}

// RetrievalEventRecorder records retrieval events for contrastive training data collection.
type RetrievalEventRecorder interface {
	RecordRetrieval(ctx context.Context, event RetrievalEvent)
}

// QueryHash computes SHA-256 of query text for deduplication.
func QueryHash(text string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(text)))
}
