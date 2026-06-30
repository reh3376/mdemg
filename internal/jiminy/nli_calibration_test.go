package jiminy

import "testing"

func TestCalibrationTracker_Track(t *testing.T) {
	tracker := NewNLICalibrationTracker(100, 0.15)
	tracker.Track(0.8, 0.7)
	tracker.Track(0.9, 0.85)

	report := tracker.Report()
	if report.WindowSize != 2 {
		t.Errorf("window size = %d, want 2", report.WindowSize)
	}
}

// ALERT-TRUTH-001: GetNLICalibrationReport must return nil when the NLI sidecar
// isn't operational, even with a stale-but-biased window — otherwise the phantom
// mean-bias pins a continuously-firing nli_bias_alert (live: 0.638 at 0 requests).
func TestGetNLICalibrationReport_GatedOnOperational(t *testing.T) {
	tracker := NewNLICalibrationTracker(10, 0.05)
	tracker.Track(0.9, 0.2) // big bias → BiasAlert would be true if surfaced
	tracker.Track(0.85, 0.25)

	// Sanity: the tracker itself does report a bias.
	if rep := tracker.Report(); rep == nil || !rep.BiasAlert {
		t.Fatalf("precondition: tracker should report a bias alert")
	}

	// Non-operational scorer (enabled but no sidecar URL) → nil report.
	svc := &Service{calibrationTracker: tracker, nliScorer: &NLIComprehensionScorer{enabled: true}}
	if rep := svc.GetNLICalibrationReport(); rep != nil {
		t.Errorf("sidecar off → want nil report, got mean_bias=%v alert=%v", rep.MeanBias, rep.BiasAlert)
	}

	// Operational scorer → the real report flows through.
	svc.nliScorer = &NLIComprehensionScorer{enabled: true, sidecarURL: "http://127.0.0.1:8101"}
	if rep := svc.GetNLICalibrationReport(); rep == nil {
		t.Errorf("sidecar operational → want a report, got nil")
	}

	// Nil scorer → nil report (no panic).
	svc.nliScorer = nil
	if rep := svc.GetNLICalibrationReport(); rep != nil {
		t.Errorf("nil scorer → want nil report")
	}
}

func TestCalibrationTracker_Report_NoBias(t *testing.T) {
	tracker := NewNLICalibrationTracker(100, 0.15)
	for range 10 {
		tracker.Track(0.8, 0.8)
	}

	report := tracker.Report()
	if report.MeanBias != 0 {
		t.Errorf("mean bias = %f, want 0", report.MeanBias)
	}
	if report.BiasAlert {
		t.Error("bias alert should be false when no bias")
	}
}

func TestCalibrationTracker_Report_PositiveBias(t *testing.T) {
	tracker := NewNLICalibrationTracker(100, 0.15)
	for range 10 {
		tracker.Track(0.9, 0.6) // NLI consistently higher
	}

	report := tracker.Report()
	expectedBias := 0.3
	if report.MeanBias < expectedBias-0.01 || report.MeanBias > expectedBias+0.01 {
		t.Errorf("mean bias = %f, want ~%.1f", report.MeanBias, expectedBias)
	}
}

func TestCalibrationTracker_Report_BiasAlert(t *testing.T) {
	tracker := NewNLICalibrationTracker(100, 0.15)
	for range 20 {
		tracker.Track(0.9, 0.5) // 0.4 bias > 0.15 threshold
	}

	report := tracker.Report()
	if !report.BiasAlert {
		t.Error("bias alert should be true when bias exceeds threshold")
	}
	if report.MeanBias < 0.39 || report.MeanBias > 0.41 {
		t.Errorf("mean bias = %f, want ~0.4", report.MeanBias)
	}
}

func TestCalibrationTracker_RingBufferWrap(t *testing.T) {
	tracker := NewNLICalibrationTracker(5, 0.15) // small window
	for i := range 10 {
		tracker.Track(float64(i)*0.1, 0.5)
	}

	report := tracker.Report()
	if report.WindowSize != 5 {
		t.Errorf("window size = %d, want 5 (capped at max)", report.WindowSize)
	}
	// After wrapping, the last 5 entries are i=5..9 → nli=0.5..0.9
	// Mean NLI = (0.5+0.6+0.7+0.8+0.9)/5 = 0.7
	if report.MeanNLI < 0.69 || report.MeanNLI > 0.71 {
		t.Errorf("mean NLI = %f, want ~0.7 (ring buffer should evict oldest)", report.MeanNLI)
	}
}

func TestCalibrationTracker_EmptyReport(t *testing.T) {
	tracker := NewNLICalibrationTracker(100, 0.15)

	report := tracker.Report()
	if report.WindowSize != 0 {
		t.Errorf("window size = %d, want 0", report.WindowSize)
	}
	if report.MeanNLI != 0 {
		t.Errorf("mean NLI = %f, want 0", report.MeanNLI)
	}
	if report.BiasAlert {
		t.Error("bias alert should be false on empty report")
	}
}
