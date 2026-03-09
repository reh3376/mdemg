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
