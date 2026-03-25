package main

import (
	"testing"
)

func TestIssueToEdges_Parent(t *testing.T) {
	m := &LinearModule{}
	issue := linearIssue{
		ID:         "issue-1",
		Identifier: "ENG-100",
	}
	issue.Parent = &struct {
		ID         string
		Identifier string
		Title      string
	}{
		ID:         "issue-0",
		Identifier: "ENG-50",
		Title:      "Parent Issue",
	}

	edges := m.issueToEdges(issue)
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}

	e := edges[0]
	if e.FromNodeId != "linear-issue-issue-1" {
		t.Errorf("FromNodeId: got %q, want %q", e.FromNodeId, "linear-issue-issue-1")
	}
	if e.ToNodeId != "linear-issue-issue-0" {
		t.Errorf("ToNodeId: got %q, want %q", e.ToNodeId, "linear-issue-issue-0")
	}
	if e.RelType != "PARENT_OF" {
		t.Errorf("RelType: got %q, want %q", e.RelType, "PARENT_OF")
	}
	if e.Weight != 0.9 {
		t.Errorf("Weight: got %f, want %f", e.Weight, 0.9)
	}
	if e.Properties["source"] != "linear" {
		t.Errorf("Properties[source]: got %q, want %q", e.Properties["source"], "linear")
	}
}

func TestIssueToEdges_Children(t *testing.T) {
	m := &LinearModule{}
	issue := linearIssue{
		ID:         "parent-1",
		Identifier: "ENG-100",
	}
	issue.Children = []struct {
		ID         string
		Identifier string
		Title      string
	}{
		{ID: "child-1", Identifier: "ENG-101", Title: "Child 1"},
		{ID: "child-2", Identifier: "ENG-102", Title: "Child 2"},
	}

	edges := m.issueToEdges(issue)
	if len(edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(edges))
	}

	// Children emit edges FROM child TO parent
	for i, e := range edges {
		if e.ToNodeId != "linear-issue-parent-1" {
			t.Errorf("edge[%d] ToNodeId: got %q, want %q", i, e.ToNodeId, "linear-issue-parent-1")
		}
		if e.RelType != "PARENT_OF" {
			t.Errorf("edge[%d] RelType: got %q, want %q", i, e.RelType, "PARENT_OF")
		}
	}

	if edges[0].FromNodeId != "linear-issue-child-1" {
		t.Errorf("edge[0] FromNodeId: got %q, want %q", edges[0].FromNodeId, "linear-issue-child-1")
	}
	if edges[1].FromNodeId != "linear-issue-child-2" {
		t.Errorf("edge[1] FromNodeId: got %q, want %q", edges[1].FromNodeId, "linear-issue-child-2")
	}
}

func TestIssueToEdges_Relations(t *testing.T) {
	m := &LinearModule{}
	issue := linearIssue{
		ID:         "issue-1",
		Identifier: "ENG-100",
	}
	issue.Relations = []struct {
		Type         string
		RelatedIssue struct {
			ID         string
			Identifier string
		}
	}{
		{Type: "blocks", RelatedIssue: struct {
			ID         string
			Identifier string
		}{ID: "issue-2", Identifier: "ENG-200"}},
		{Type: "related", RelatedIssue: struct {
			ID         string
			Identifier string
		}{ID: "issue-3", Identifier: "ENG-300"}},
		{Type: "duplicate", RelatedIssue: struct {
			ID         string
			Identifier string
		}{ID: "issue-4", Identifier: "ENG-400"}},
	}

	edges := m.issueToEdges(issue)
	if len(edges) != 3 {
		t.Fatalf("expected 3 edges, got %d", len(edges))
	}

	expectedTypes := []string{"BLOCKS", "RELATED_TO", "DUPLICATE_OF"}
	for i, e := range edges {
		if e.RelType != expectedTypes[i] {
			t.Errorf("edge[%d] RelType: got %q, want %q", i, e.RelType, expectedTypes[i])
		}
		if e.FromNodeId != "linear-issue-issue-1" {
			t.Errorf("edge[%d] FromNodeId: got %q, want %q", i, e.FromNodeId, "linear-issue-issue-1")
		}
		if e.Properties["source"] != "linear" {
			t.Errorf("edge[%d] Properties[source]: got %q, want %q", i, e.Properties["source"], "linear")
		}
	}
}

func TestIssueToEdges_NoRelationships(t *testing.T) {
	m := &LinearModule{}
	issue := linearIssue{
		ID:         "issue-1",
		Identifier: "ENG-100",
	}

	edges := m.issueToEdges(issue)
	if len(edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(edges))
	}
}

func TestMapLinearRelationType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"blocks", "BLOCKS"},
		{"is-blocked-by", "BLOCKED_BY"},
		{"related", "RELATED_TO"},
		{"duplicate", "DUPLICATE_OF"},
		{"unknown-type", "RELATED_TO"},
		{"", "RELATED_TO"},
	}

	for _, tc := range tests {
		got := mapLinearRelationType(tc.input)
		if got != tc.expected {
			t.Errorf("mapLinearRelationType(%q): got %q, want %q", tc.input, got, tc.expected)
		}
	}
}
