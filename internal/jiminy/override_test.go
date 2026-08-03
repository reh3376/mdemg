package jiminy

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// JIMINY-ENFORCE-003 — OverrideManager pin tests.

func TestOverrideManager_ApplyValidation(t *testing.T) {
	m := NewOverrideManager("")
	cases := []struct {
		name    string
		session string
		code    string
		reason  string
		dur     time.Duration
	}{
		{"empty session", "", "c1", "r", time.Minute},
		{"empty code", "s1", "", "r", time.Minute},
		{"empty reason", "s1", "c1", "", time.Minute},
		{"zero duration", "s1", "c1", "r", 0},
		{"negative duration", "s1", "c1", "r", -time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := m.Apply(tc.session, tc.code, tc.reason, tc.dur); err == nil {
				t.Errorf("expected validation error for %q", tc.name)
			}
		})
	}
}

func TestOverrideManager_ApplyGetRevoke(t *testing.T) {
	m := NewOverrideManager("")
	entry, err := m.Apply("s1", "c1", "false-positive spike", 5*time.Minute)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if entry.SessionID != "s1" || entry.ConstraintCode != "c1" || entry.Reason != "false-positive spike" {
		t.Errorf("entry fields wrong: %+v", entry)
	}
	got := m.Get("s1", "c1")
	if got == nil || got.ConstraintCode != "c1" {
		t.Errorf("get after apply broken: %+v", got)
	}

	// Revoke removes it.
	rev := m.Revoke("s1", "c1")
	if rev == nil {
		t.Fatal("revoke returned nil for existing entry")
	}
	if got := m.Get("s1", "c1"); got != nil {
		t.Errorf("get after revoke must be nil, got %+v", got)
	}
	// Second revoke → nil.
	if rev := m.Revoke("s1", "c1"); rev != nil {
		t.Error("double-revoke must return nil")
	}
}

func TestOverrideManager_ExpiryLazyPurge(t *testing.T) {
	m := NewOverrideManager("")
	_, err := m.Apply("s1", "c1", "brief", time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if got := m.Get("s1", "c1"); got != nil {
		t.Errorf("expired override must be purged on Get, got %+v", got)
	}
	// Re-apply after expiry works.
	if _, err := m.Apply("s1", "c1", "again", time.Minute); err != nil {
		t.Fatalf("re-apply after expiry: %v", err)
	}
	if got := m.Get("s1", "c1"); got == nil {
		t.Error("re-apply after expiry must succeed")
	}
}

func TestOverrideManager_SessionIsolation(t *testing.T) {
	m := NewOverrideManager("")
	_, _ = m.Apply("s1", "c1", "r1", time.Minute)
	_, _ = m.Apply("s2", "c1", "r2", time.Minute) // same constraint, different session

	if got := m.Get("s1", "c1"); got == nil || got.Reason != "r1" {
		t.Errorf("session s1 override broken: %+v", got)
	}
	if got := m.Get("s2", "c1"); got == nil || got.Reason != "r2" {
		t.Errorf("session s2 override broken: %+v", got)
	}
	// Revoking s1 must not affect s2.
	m.Revoke("s1", "c1")
	if got := m.Get("s2", "c1"); got == nil {
		t.Error("revoking s1 wiped s2 — session isolation broken")
	}
}

func TestOverrideManager_ListFiltersBySession(t *testing.T) {
	m := NewOverrideManager("")
	_, _ = m.Apply("s1", "c1", "r", time.Minute)
	_, _ = m.Apply("s1", "c2", "r", time.Minute)
	_, _ = m.Apply("s2", "c1", "r", time.Minute)

	if got := len(m.List("")); got != 3 {
		t.Errorf("unfiltered list: expected 3, got %d", got)
	}
	if got := len(m.List("s1")); got != 2 {
		t.Errorf("s1 list: expected 2, got %d", got)
	}
	if got := len(m.List("s2")); got != 1 {
		t.Errorf("s2 list: expected 1, got %d", got)
	}
	if got := len(m.List("nosuch")); got != 0 {
		t.Errorf("nosuch list: expected 0, got %d", got)
	}
}

func TestOverrideManager_AuditLogWritesOnApplyAndRevoke(t *testing.T) {
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.jsonl")
	m := NewOverrideManager(auditPath)
	_, _ = m.Apply("s1", "c1", "reason-a", time.Minute)
	m.Revoke("s1", "c1")

	f, err := os.Open(auditPath)
	if err != nil {
		t.Fatalf("audit file missing: %v", err)
	}
	defer f.Close()

	ops := []string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var rec map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			t.Errorf("audit line not JSON: %s", scanner.Text())
			continue
		}
		if op, ok := rec["op"].(string); ok {
			ops = append(ops, op)
		}
	}
	if len(ops) != 2 || ops[0] != "apply" || ops[1] != "revoke" {
		t.Errorf("audit ops = %v; want [apply revoke]", ops)
	}
}

func TestOverrideManager_AuditFailsOpen(t *testing.T) {
	// Unwritable path — must not crash the caller.
	m := NewOverrideManager("/nonexistent/nested/should/fail.jsonl")
	if _, err := m.Apply("s1", "c1", "r", time.Minute); err != nil {
		t.Errorf("apply must succeed even when audit unwritable: %v", err)
	}
}

// JIMINY-ENFORCE-003 — StrictClassifier suppression pin.
// Full-suppression path: verdict deny with all-violated codes overridden → pass.
func TestStrictClassifier_OverrideSuppressesDeny(t *testing.T) {
	// Directly exercise the classifier's suppression logic via a fake evaluator
	// isn't wired up here (evaluator is production-heavy). This test pins the
	// override manager's Get() semantics that the classifier consumes; the full
	// integration path is covered by the live Tier-3 smoke.
	m := NewOverrideManager("")
	_, err := m.Apply("s1", "MYRULE", "false-positive", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Get("s1", "MYRULE"); got == nil {
		t.Error("override must be retrievable — classifier suppression relies on this")
	}
	if got := m.Get("s1", "OTHERRULE"); got != nil {
		t.Error("different rule must not match — classifier suppression depends on per-code matching")
	}
}
