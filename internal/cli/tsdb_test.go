package cli

import (
	"testing"
)

// ─── tsdbEnv ───

func TestTsdbEnv_Default(t *testing.T) {
	got := tsdbEnv("MDEMG_TEST_TSDB_NONEXISTENT_KEY_XYZ", "fallback_val")
	if got != "fallback_val" {
		t.Errorf("got %q, want %q", got, "fallback_val")
	}
}

func TestTsdbEnv_Override(t *testing.T) {
	t.Setenv("MDEMG_TEST_TSDB_OVERRIDE", "custom_value")
	got := tsdbEnv("MDEMG_TEST_TSDB_OVERRIDE", "fallback_val")
	if got != "custom_value" {
		t.Errorf("got %q, want %q", got, "custom_value")
	}
}

// ─── tsdbEnvInt ───

func TestTsdbEnvInt_Default(t *testing.T) {
	got := tsdbEnvInt("MDEMG_TEST_TSDB_INT_NONEXISTENT_XYZ", 9999)
	if got != 9999 {
		t.Errorf("got %d, want 9999", got)
	}
}

func TestTsdbEnvInt_Valid(t *testing.T) {
	t.Setenv("MDEMG_TEST_TSDB_INT_VALID", "5433")
	got := tsdbEnvInt("MDEMG_TEST_TSDB_INT_VALID", 5432)
	if got != 5433 {
		t.Errorf("got %d, want 5433", got)
	}
}

func TestTsdbEnvInt_Invalid(t *testing.T) {
	t.Setenv("MDEMG_TEST_TSDB_INT_INVALID", "not_a_number")
	got := tsdbEnvInt("MDEMG_TEST_TSDB_INT_INVALID", 5432)
	if got != 5432 {
		t.Errorf("got %d, want 5432 (fallback on bad parse)", got)
	}
}

// ─── tsdbConfigFromEnv ───

func TestTsdbConfigFromEnv_Defaults(t *testing.T) {
	// Clear any env vars that could interfere
	t.Setenv("TSDB_HOST", "")
	t.Setenv("TSDB_PORT", "")
	t.Setenv("TSDB_USER", "")
	t.Setenv("TSDB_PASSWORD", "")
	t.Setenv("TSDB_DATABASE", "")
	t.Setenv("TSDB_SSL_MODE", "")

	cfg := tsdbConfigFromEnv()

	if cfg.Host != "localhost" {
		t.Errorf("Host: got %q, want %q", cfg.Host, "localhost")
	}
	if cfg.Port != 5432 {
		t.Errorf("Port: got %d, want 5432", cfg.Port)
	}
	if cfg.User != "mdemg" {
		t.Errorf("User: got %q, want %q", cfg.User, "mdemg")
	}
	if cfg.Password != "mdemg_metrics" {
		t.Errorf("Password: got %q, want %q", cfg.Password, "mdemg_metrics")
	}
	if cfg.Database != "mdemg_metrics" {
		t.Errorf("Database: got %q, want %q", cfg.Database, "mdemg_metrics")
	}
	if cfg.SSLMode != "disable" {
		t.Errorf("SSLMode: got %q, want %q", cfg.SSLMode, "disable")
	}
}
