package cli

import "testing"

// TestIsFtLoopStage pins the accepted stage set (FT-RECURSIVE-001 Epic 4).
func TestIsFtLoopStage(t *testing.T) {
	for _, ok := range []string{"capture", "curate", "train", "benchmark", "gate", "promote"} {
		if !isFtLoopStage(ok) {
			t.Errorf("%q should be a valid stage", ok)
		}
	}
	for _, bad := range []string{"", "deploy", "Capture", "trainn", "rollback"} {
		if isFtLoopStage(bad) {
			t.Errorf("%q should NOT be a valid stage", bad)
		}
	}
	if len(ftLoopStages) != 6 {
		t.Errorf("expected 6 stages, got %d", len(ftLoopStages))
	}
}

// TestFtLoopReportStageCmd_Validation: invalid stage/status are rejected before
// any side effect; required flags enforced.
func TestFtLoopReportStageCmd_Validation(t *testing.T) {
	cmd := newFtLoopReportStageCmd()
	cmd.SetArgs([]string{"--stage", "bogus", "--status", "success"})
	if err := cmd.Execute(); err == nil {
		t.Error("invalid --stage should error")
	}
	cmd = newFtLoopReportStageCmd()
	cmd.SetArgs([]string{"--stage", "train", "--status", "maybe"})
	if err := cmd.Execute(); err == nil {
		t.Error("invalid --status should error")
	}
}
