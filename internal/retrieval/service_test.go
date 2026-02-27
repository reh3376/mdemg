package retrieval

import (
	"testing"

	"mdemg/internal/models"
)

func TestSpaceIDs_WithoutGlobalSpace(t *testing.T) {
	req := models.RetrieveRequest{
		SpaceID:            "my-repo",
		IncludeGlobalSpace: false,
	}

	spaceIDs := []string{req.SpaceID}
	if req.IncludeGlobalSpace {
		spaceIDs = append(spaceIDs, "mdemg-global")
	}

	if len(spaceIDs) != 1 {
		t.Fatalf("expected 1 spaceID, got %d", len(spaceIDs))
	}
	if spaceIDs[0] != "my-repo" {
		t.Errorf("expected spaceIDs[0]=%q, got %q", "my-repo", spaceIDs[0])
	}
}

func TestSpaceIDs_WithGlobalSpace(t *testing.T) {
	req := models.RetrieveRequest{
		SpaceID:            "my-repo",
		IncludeGlobalSpace: true,
	}

	spaceIDs := []string{req.SpaceID}
	if req.IncludeGlobalSpace {
		spaceIDs = append(spaceIDs, "mdemg-global")
	}

	if len(spaceIDs) != 2 {
		t.Fatalf("expected 2 spaceIDs, got %d", len(spaceIDs))
	}
	if spaceIDs[0] != "my-repo" {
		t.Errorf("expected spaceIDs[0]=%q, got %q", "my-repo", spaceIDs[0])
	}
	if spaceIDs[1] != "mdemg-global" {
		t.Errorf("expected spaceIDs[1]=%q, got %q", "mdemg-global", spaceIDs[1])
	}
}

func TestSpaceIDs_GlobalSpaceDefaultsFalse(t *testing.T) {
	req := models.RetrieveRequest{
		SpaceID: "billing-service",
	}

	spaceIDs := []string{req.SpaceID}
	if req.IncludeGlobalSpace {
		spaceIDs = append(spaceIDs, "mdemg-global")
	}

	if len(spaceIDs) != 1 {
		t.Fatalf("expected 1 spaceID when IncludeGlobalSpace omitted, got %d", len(spaceIDs))
	}
}
