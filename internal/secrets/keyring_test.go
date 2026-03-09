package secrets

import (
	"os"
	"testing"
)

func TestIsKnown(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"neo4j-password", true},
		{"openai-api-key", true},
		{"jwt-secret", true},
		{"linear-webhook", true},
		{"unknown-key", false},
		{"", false},
	}
	for _, tt := range tests {
		got := IsKnown(tt.key)
		if got != tt.want {
			t.Errorf("IsKnown(%q) = %v, want %v", tt.key, got, tt.want)
		}
	}
}

func TestKnownSecrets_EnvVarMappings(t *testing.T) {
	expected := map[string]string{
		"neo4j-password": "NEO4J_PASS",
		"openai-api-key": "OPENAI_API_KEY",
		"jwt-secret":     "AUTH_JWT_SECRET",
		"linear-webhook": "LINEAR_WEBHOOK_SECRET",
	}
	for key, envVar := range expected {
		got, ok := KnownSecrets[key]
		if !ok {
			t.Errorf("KnownSecrets missing key %q", key)
			continue
		}
		if got != envVar {
			t.Errorf("KnownSecrets[%q] = %q, want %q", key, got, envVar)
		}
	}
	if len(KnownSecrets) != len(expected) {
		t.Errorf("KnownSecrets has %d entries, want %d", len(KnownSecrets), len(expected))
	}
}

func TestErrNotFound(t *testing.T) {
	if ErrNotFound == nil {
		t.Fatal("ErrNotFound should not be nil")
	}
	if ErrNotFound.Error() != "secret not found" {
		t.Errorf("ErrNotFound = %q, want %q", ErrNotFound.Error(), "secret not found")
	}
}

func TestServiceName(t *testing.T) {
	if serviceName != "mdemg" {
		t.Errorf("serviceName = %q, want %q", serviceName, "mdemg")
	}
}

func TestResolveSecrets_SkipsExistingEnvVars(t *testing.T) {
	// Set an env var that corresponds to a known secret
	key := "NEO4J_PASS"
	original := os.Getenv(key)
	os.Setenv(key, "already-set")
	defer func() {
		if original != "" {
			os.Setenv(key, original)
		} else {
			os.Unsetenv(key)
		}
	}()

	// ResolveSecrets should NOT overwrite the existing env var
	// (Even if keychain has a different value, env takes priority)
	ResolveSecrets()

	if got := os.Getenv(key); got != "already-set" {
		t.Errorf("NEO4J_PASS = %q after ResolveSecrets, want %q (should not overwrite)", got, "already-set")
	}
}
