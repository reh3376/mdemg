package tsdb

import (
	"strings"
	"testing"
)

func TestConfig_DSN(t *testing.T) {
	tests := []struct {
		name       string
		cfg        Config
		wantHost   string
		wantPort   string
		wantUser   string
		wantDB     string
		wantSSL    string
	}{
		{
			name: "default dev config",
			cfg: Config{
				Host:     "localhost",
				Port:     5432,
				User:     "mdemg",
				Password: "mdemg_metrics",
				Database: "mdemg_metrics",
				SSLMode:  "disable",
			},
			wantHost: "localhost",
			wantPort: "5432",
			wantUser: "mdemg",
			wantDB:   "mdemg_metrics",
			wantSSL:  "sslmode=disable",
		},
		{
			name: "custom host and port",
			cfg: Config{
				Host:     "db.example.com",
				Port:     5433,
				User:     "admin",
				Password: "secret",
				Database: "prod_metrics",
				SSLMode:  "require",
			},
			wantHost: "db.example.com",
			wantPort: "5433",
			wantUser: "admin",
			wantDB:   "prod_metrics",
			wantSSL:  "sslmode=require",
		},
		{
			name: "ssl verify-full",
			cfg: Config{
				Host:     "secure.host",
				Port:     5432,
				User:     "secure_user",
				Password: "pass",
				Database: "secure_db",
				SSLMode:  "verify-full",
			},
			wantHost: "secure.host",
			wantPort: "5432",
			wantUser: "secure_user",
			wantDB:   "secure_db",
			wantSSL:  "sslmode=verify-full",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dsn := tt.cfg.DSN()

			if !strings.Contains(dsn, tt.wantHost) {
				t.Errorf("DSN %q does not contain host %q", dsn, tt.wantHost)
			}
			if !strings.Contains(dsn, tt.wantPort) {
				t.Errorf("DSN %q does not contain port %q", dsn, tt.wantPort)
			}
			if !strings.Contains(dsn, tt.wantUser) {
				t.Errorf("DSN %q does not contain user %q", dsn, tt.wantUser)
			}
			if !strings.Contains(dsn, tt.wantDB) {
				t.Errorf("DSN %q does not contain database %q", dsn, tt.wantDB)
			}
			if !strings.Contains(dsn, tt.wantSSL) {
				t.Errorf("DSN %q does not contain ssl %q", dsn, tt.wantSSL)
			}

			// Verify it starts with postgres://
			if !strings.HasPrefix(dsn, "postgres://") {
				t.Errorf("DSN %q does not start with postgres://", dsn)
			}
		})
	}
}
