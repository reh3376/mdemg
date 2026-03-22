package jiminy

import (
	"testing"
)

func TestExtensionRegistry_NegotiateAllowed(t *testing.T) {
	reg := NewExtensionRegistry([]string{"tier_preference", "abbreviated_ids", "batch_mode", "density_boost"})

	resp := reg.Negotiate(ExtensionRequest{
		SessionID: "sess-1",
		Extension: "tier_preference",
		Value:     "t1_only",
	})

	if !resp.Granted {
		t.Errorf("expected tier_preference to be granted, got reason: %s", resp.Reason)
	}

	// Verify it's active
	if !reg.HasExtension("sess-1", "tier_preference") {
		t.Error("tier_preference should be active for sess-1")
	}
}

func TestExtensionRegistry_NegotiateNotAllowed(t *testing.T) {
	reg := NewExtensionRegistry([]string{"tier_preference"})

	resp := reg.Negotiate(ExtensionRequest{
		SessionID: "sess-1",
		Extension: "custom_extension",
	})

	if resp.Granted {
		t.Error("custom_extension should not be granted")
	}
}

func TestExtensionRegistry_NonNegotiable(t *testing.T) {
	reg := NewExtensionRegistry([]string{"constraint_types"}) // even if "allowed", non-negotiable wins

	resp := reg.Negotiate(ExtensionRequest{
		SessionID: "sess-1",
		Extension: "constraint_types",
	})

	if resp.Granted {
		t.Error("constraint_types is non-negotiable and should not be granted")
	}
}

func TestExtensionRegistry_DuplicateGrant(t *testing.T) {
	reg := NewExtensionRegistry([]string{"tier_preference"})

	reg.Negotiate(ExtensionRequest{SessionID: "sess-1", Extension: "tier_preference"})
	resp := reg.Negotiate(ExtensionRequest{SessionID: "sess-1", Extension: "tier_preference"})

	if !resp.Granted {
		t.Error("duplicate request should still be granted")
	}
	if resp.Reason != "already active" {
		t.Errorf("reason = %q, want 'already active'", resp.Reason)
	}

	// Should still only have one extension
	exts := reg.GetActive("sess-1")
	if len(exts) != 1 {
		t.Errorf("expected 1 active extension, got %d", len(exts))
	}
}

func TestExtensionRegistry_SessionIsolation(t *testing.T) {
	reg := NewExtensionRegistry([]string{"tier_preference", "batch_mode"})

	reg.Negotiate(ExtensionRequest{SessionID: "sess-1", Extension: "tier_preference"})
	reg.Negotiate(ExtensionRequest{SessionID: "sess-2", Extension: "batch_mode"})

	if !reg.HasExtension("sess-1", "tier_preference") {
		t.Error("sess-1 should have tier_preference")
	}
	if reg.HasExtension("sess-1", "batch_mode") {
		t.Error("sess-1 should not have batch_mode")
	}
	if reg.HasExtension("sess-2", "tier_preference") {
		t.Error("sess-2 should not have tier_preference")
	}
	if !reg.HasExtension("sess-2", "batch_mode") {
		t.Error("sess-2 should have batch_mode")
	}
}

func TestExtensionRegistry_ClearSession(t *testing.T) {
	reg := NewExtensionRegistry([]string{"tier_preference"})

	reg.Negotiate(ExtensionRequest{SessionID: "sess-1", Extension: "tier_preference"})
	reg.ClearSession("sess-1")

	if reg.HasExtension("sess-1", "tier_preference") {
		t.Error("sess-1 should have no extensions after clear")
	}

	exts := reg.GetActive("sess-1")
	if len(exts) != 0 {
		t.Errorf("expected 0 active extensions after clear, got %d", len(exts))
	}
}
