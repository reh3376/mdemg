package scraper

import (
	"testing"

	pb "mdemg/api/modulepb"
)

// --- normalizeURL tests ---

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain URL", "https://example.com/path", "https://example.com/path"},
		{"strip fragment", "https://example.com/page#section", "https://example.com/page"},
		{"strip trailing slash", "https://example.com/path/", "https://example.com/path"},
		{"strip both", "https://example.com/path/#frag", "https://example.com/path"},
		{"no change needed", "https://example.com", "https://example.com"},
		{"query preserved", "https://example.com/search?q=test", "https://example.com/search?q=test"},
		{"invalid URL passthrough", "not a url %%", "not a url %%"},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeURL(tt.input)
			if got != tt.want {
				t.Errorf("normalizeURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- extractDiscoveredLinks tests ---

func TestExtractDiscoveredLinks(t *testing.T) {
	obs := &pb.Observation{
		Metadata: map[string]string{
			"discovered_links": `["https://a.com","https://b.com"]`,
		},
	}
	links := extractDiscoveredLinks(obs)
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d", len(links))
	}
	if links[0] != "https://a.com" {
		t.Errorf("links[0] = %q, want %q", links[0], "https://a.com")
	}
}

func TestExtractDiscoveredLinks_NoMetadata(t *testing.T) {
	obs := &pb.Observation{Metadata: map[string]string{}}
	links := extractDiscoveredLinks(obs)
	if links != nil {
		t.Errorf("expected nil, got %v", links)
	}
}

func TestExtractDiscoveredLinks_EmptyValue(t *testing.T) {
	obs := &pb.Observation{Metadata: map[string]string{"discovered_links": ""}}
	links := extractDiscoveredLinks(obs)
	if links != nil {
		t.Errorf("expected nil, got %v", links)
	}
}

func TestExtractDiscoveredLinks_InvalidJSON(t *testing.T) {
	obs := &pb.Observation{Metadata: map[string]string{"discovered_links": "not json"}}
	links := extractDiscoveredLinks(obs)
	if links != nil {
		t.Errorf("expected nil for invalid JSON, got %v", links)
	}
}

func TestExtractDiscoveredLinks_EmptyArray(t *testing.T) {
	obs := &pb.Observation{Metadata: map[string]string{"discovered_links": "[]"}}
	links := extractDiscoveredLinks(obs)
	if len(links) != 0 {
		t.Errorf("expected 0 links, got %d", len(links))
	}
}

// --- type constant tests ---

func TestJobStatus_Constants(t *testing.T) {
	if StatusPending != "pending" {
		t.Errorf("StatusPending = %q", StatusPending)
	}
	if StatusRunning != "running" {
		t.Errorf("StatusRunning = %q", StatusRunning)
	}
	if StatusCompleted != "completed" {
		t.Errorf("StatusCompleted = %q", StatusCompleted)
	}
	if StatusFailed != "failed" {
		t.Errorf("StatusFailed = %q", StatusFailed)
	}
}

func TestContentStatus_Constants(t *testing.T) {
	if ContentPendingReview != "pending_review" {
		t.Errorf("ContentPendingReview = %q", ContentPendingReview)
	}
	if ContentApproved != "approved" {
		t.Errorf("ContentApproved = %q", ContentApproved)
	}
	if ContentRejected != "rejected" {
		t.Errorf("ContentRejected = %q", ContentRejected)
	}
}

// --- NewDedupChecker tests ---

func TestNewDedupChecker_DefaultThreshold(t *testing.T) {
	dc := NewDedupChecker(nil, nil, 0)
	if dc.threshold != 0.85 {
		t.Errorf("expected default threshold 0.85, got %f", dc.threshold)
	}
}

func TestNewDedupChecker_NegativeThreshold(t *testing.T) {
	dc := NewDedupChecker(nil, nil, -1.0)
	if dc.threshold != 0.85 {
		t.Errorf("expected default threshold 0.85 for negative input, got %f", dc.threshold)
	}
}

func TestNewDedupChecker_CustomThreshold(t *testing.T) {
	dc := NewDedupChecker(nil, nil, 0.9)
	if dc.threshold != 0.9 {
		t.Errorf("expected threshold 0.9, got %f", dc.threshold)
	}
}

// --- NewOrchestrator tests ---

func TestNewOrchestrator(t *testing.T) {
	cfg := Config{DefaultDelayMs: 100, DefaultTimeoutMs: 5000}
	orch := NewOrchestrator(nil, nil, nil, nil, cfg)
	if orch == nil {
		t.Fatal("NewOrchestrator returned nil")
	}
	if orch.cfg.DefaultDelayMs != 100 {
		t.Errorf("expected DefaultDelayMs 100, got %d", orch.cfg.DefaultDelayMs)
	}
	if orch.cfg.DefaultTimeoutMs != 5000 {
		t.Errorf("expected DefaultTimeoutMs 5000, got %d", orch.cfg.DefaultTimeoutMs)
	}
}

// --- NewReviewer tests ---

func TestNewReviewer(t *testing.T) {
	rev := NewReviewer(nil, nil)
	if rev == nil {
		t.Fatal("NewReviewer returned nil")
	}
}

// --- Service constructor and getter tests ---

func TestNewService_NilDeps(t *testing.T) {
	cfg := Config{Enabled: true, DefaultSpaceID: "test-space"}
	svc := NewService(cfg, nil, nil, nil)
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
	if svc.store == nil {
		t.Error("NewService should create a Store")
	}
}

func TestService_GetConfig(t *testing.T) {
	cfg := Config{
		Enabled:        true,
		DefaultSpaceID: "my-space",
		MaxConcurrentJobs: 5,
	}
	svc := NewService(cfg, nil, nil, nil)
	got := svc.GetConfig()
	if got.DefaultSpaceID != "my-space" {
		t.Errorf("GetConfig().DefaultSpaceID = %q, want %q", got.DefaultSpaceID, "my-space")
	}
	if got.MaxConcurrentJobs != 5 {
		t.Errorf("GetConfig().MaxConcurrentJobs = %d, want 5", got.MaxConcurrentJobs)
	}
}

func TestService_GetStore(t *testing.T) {
	svc := NewService(Config{}, nil, nil, nil)
	store := svc.GetStore()
	if store == nil {
		t.Error("GetStore() returned nil")
	}
}

func TestService_GetOrchestrator_BeforeWiring(t *testing.T) {
	svc := NewService(Config{}, nil, nil, nil)
	if svc.GetOrchestrator() != nil {
		t.Error("GetOrchestrator() should be nil before SetConversationService")
	}
}

func TestService_GetReviewer_BeforeWiring(t *testing.T) {
	svc := NewService(Config{}, nil, nil, nil)
	if svc.GetReviewer() != nil {
		t.Error("GetReviewer() should be nil before SetConversationService")
	}
}

func TestService_SetConversationService(t *testing.T) {
	svc := NewService(Config{}, nil, nil, nil)
	svc.SetConversationService(nil)
	if svc.GetOrchestrator() == nil {
		t.Error("GetOrchestrator() should be non-nil after SetConversationService")
	}
	if svc.GetReviewer() == nil {
		t.Error("GetReviewer() should be non-nil after SetConversationService")
	}
}
