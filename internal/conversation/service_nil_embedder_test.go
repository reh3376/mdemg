package conversation

import (
	"testing"

	"mdemg/internal/config"
)

// CONVERSATION-EMBEDDER-OPTIONAL-001 — pin tests for the nil-embedder path.
// Pre-sprint, api/server.go skipped constructing the conversation service
// entirely when there was no embedder, leaving `s.conversationSvc == nil` and
// every observe/correct/skill handler returning "conversation service not
// available (embedder required)." That made a fresh install in
// EMBEDDING_PROVIDER=disabled mode effectively read-only — beta blocker B5.
//
// The Service constructor ALREADY handled nil-embedder gracefully (skipping
// SurpriseDetector construction, guarding s.embedder != nil in Observe /
// Retrieve). These tests pin that behavior against regressions.

func TestNewServiceWithConfig_NilEmbedderConstructsCleanly(t *testing.T) {
	// The constructor must NOT panic + must return a non-nil service.
	svc := NewServiceWithConfig(nil, nil, "memNodeEmbedding", config.Config{
		ConstraintDetectionEnabled: true,
		ConstraintMinConfidence:    0.6,
	})
	if svc == nil {
		t.Fatal("NewServiceWithConfig must return a non-nil service even without an embedder — B5 regression")
	}
	if svc.embedder != nil {
		t.Errorf("embedder should be nil when passed nil; got %T", svc.embedder)
	}
	if svc.surpriseDetector != nil {
		t.Errorf("surpriseDetector should be nil when embedder is nil (documented in NewServiceWithConfig:69)")
	}
	// Constraint detection is embedder-independent — should still be enabled.
	if !svc.constraintDetEnabled {
		t.Errorf("constraint detection must remain enabled even without embedder — regex-based, no embedding needed")
	}
	if svc.constraintDetector == nil {
		t.Errorf("constraintDetector must be constructed even without embedder")
	}
}

func TestNewService_NilEmbedderConstructsCleanly(t *testing.T) {
	// The public convenience constructor must also survive nil embedder.
	svc := NewService(nil, nil)
	if svc == nil {
		t.Fatal("NewService(nil, nil) must return a non-nil service — B5 regression")
	}
}

func TestNewServiceWithConfig_NilEmbedderNoConfigStillWorks(t *testing.T) {
	// The no-config overload must also survive.
	svc := NewServiceWithConfig(nil, nil, "memNodeEmbedding")
	if svc == nil {
		t.Fatal("NewServiceWithConfig(_, nil, _) with no config must return non-nil")
	}
	if svc.embedder != nil {
		t.Error("embedder should be nil")
	}
}

func TestNewServiceWithConfig_SetLearningServiceSafeWithoutEmbedder(t *testing.T) {
	// SetLearningService is called unconditionally in api/server.go; must not
	// panic on a nil-embedder service.
	svc := NewServiceWithConfig(nil, nil, "memNodeEmbedding", config.Config{})
	svc.SetLearningService(nil) // real callers pass a non-nil learningService, but the setter itself must be safe
	// Second call replaces; must also be safe.
	svc.SetLearningService(nil)
}
