package cli

import "testing"

func TestDataCleanCmd_Defaults(t *testing.T) {
	cmd := newDataCleanCmd()

	if cmd.Use != "clean" {
		t.Errorf("Use = %q, want %q", cmd.Use, "clean")
	}

	// Verify flag defaults.
	tests := []struct {
		flag     string
		wantDef  string
	}{
		{"space-id", "mdemg-dev"},
		{"dry-run", "true"},
		{"force", "false"},
		{"limit", "0"},
		{"min-response-len", "10"},
		{"json", "false"},
	}
	for _, tt := range tests {
		f := cmd.Flags().Lookup(tt.flag)
		if f == nil {
			t.Errorf("flag %q not found", tt.flag)
			continue
		}
		if f.DefValue != tt.wantDef {
			t.Errorf("flag %q default = %q, want %q", tt.flag, f.DefValue, tt.wantDef)
		}
	}
}

func TestDataCleanCmd_RequiresForce(t *testing.T) {
	cmd := newDataCleanCmd()
	cmd.SetArgs([]string{"--dry-run=false"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --dry-run=false without --force")
	}
}
