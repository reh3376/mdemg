package config

import "testing"

// DASHBOARD-TRUTH-001 Epic 4: the trust-session TTL and gauge significance
// floor default sanely and stay env-overridable (no-hardcoding rule).
func TestJ17TrustGaugeDefaults(t *testing.T) {
	setMinimalEnv(t)
	for _, e := range []string{"J17_TRUST_TTL_HOURS", "J17_TRUST_MIN_FEEDBACK_COUNT"} {
		t.Setenv(e, "")
	}
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.J17TrustTTLHours != 168 {
		t.Errorf("J17TrustTTLHours = %d, want 168 (7-day trust session TTL)", cfg.J17TrustTTLHours)
	}
	if cfg.J17TrustMinFeedbackCount != 5 {
		t.Errorf("J17TrustMinFeedbackCount = %d, want 5 (gauge significance floor)", cfg.J17TrustMinFeedbackCount)
	}
}

func TestJ17TrustGaugeDefaults_Override(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("J17_TRUST_TTL_HOURS", "24")
	t.Setenv("J17_TRUST_MIN_FEEDBACK_COUNT", "0")
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.J17TrustTTLHours != 24 {
		t.Errorf("J17TrustTTLHours = %d, want 24", cfg.J17TrustTTLHours)
	}
	if cfg.J17TrustMinFeedbackCount != 0 {
		t.Errorf("J17TrustMinFeedbackCount = %d, want 0 (floor disabled)", cfg.J17TrustMinFeedbackCount)
	}
}
