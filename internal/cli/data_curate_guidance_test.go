package cli

import "testing"

func TestNewDataCurateGuidanceCmd(t *testing.T) {
	cmd := newDataCurateGuidanceCmd()
	if cmd.Use != "curate-guidance" {
		t.Errorf("Use = %q, want curate-guidance", cmd.Use)
	}
	// The flags the curation script depends on must exist with sane defaults.
	for _, f := range []struct{ name, def string }{
		{"version", "v1"},
		{"space-id", "mdemg-dev"},
		{"min-label-quality", "real"},
		{"out-dir", "training_data/guidance_corpus"},
	} {
		fl := cmd.Flags().Lookup(f.name)
		if fl == nil {
			t.Errorf("missing flag --%s", f.name)
			continue
		}
		if fl.DefValue != f.def {
			t.Errorf("flag --%s default = %q, want %q", f.name, fl.DefValue, f.def)
		}
	}
	if cmd.Flags().Lookup("lookback-hours") == nil {
		t.Error("missing flag --lookback-hours")
	}
	if cmd.Flags().Lookup("against") == nil {
		t.Error("missing flag --against (leak-audit)")
	}
	// REVIEW-SUGGESTED-GUIDANCE-CONSUME-001: SME-suggestion length gate.
	msl := cmd.Flags().Lookup("min-suggestion-length")
	if msl == nil {
		t.Fatal("missing flag --min-suggestion-length")
	}
	if msl.DefValue != "40" {
		t.Errorf("flag --min-suggestion-length default = %q, want 40", msl.DefValue)
	}
}
