package jiminy

import "testing"

func TestCalibrationTracker_Track(t *testing.T) {
	tracker := NewNLICalibrationTracker(100, 0.15, 0)
	tracker.Track(0.8, 0.7, OutcomeFollowed)
	tracker.Track(0.9, 0.85, OutcomeFollowed)

	report := tracker.Report()
	if report.WindowSize != 2 {
		t.Errorf("window size = %d, want 2", report.WindowSize)
	}
}

// ALERT-TRUTH-001: GetNLICalibrationReport must return nil when the NLI sidecar
// isn't operational, even with a stale-but-biased window — otherwise the phantom
// mean-bias pins a continuously-firing nli_bias_alert (live: 0.638 at 0 requests).
func TestGetNLICalibrationReport_GatedOnOperational(t *testing.T) {
	tracker := NewNLICalibrationTracker(10, 0.05, 0)
	tracker.Track(0.9, 0.2, OutcomeFollowed) // big bias → BiasAlert would be true if surfaced
	tracker.Track(0.85, 0.25, OutcomeFollowed)

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
	tracker := NewNLICalibrationTracker(100, 0.15, 0)
	for range 10 {
		tracker.Track(0.8, 0.8, OutcomeFollowed)
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
	tracker := NewNLICalibrationTracker(100, 0.15, 0)
	for range 10 {
		tracker.Track(0.9, 0.6, OutcomeFollowed) // NLI consistently higher
	}

	report := tracker.Report()
	expectedBias := 0.3
	if report.MeanBias < expectedBias-0.01 || report.MeanBias > expectedBias+0.01 {
		t.Errorf("mean bias = %f, want ~%.1f", report.MeanBias, expectedBias)
	}
}

func TestCalibrationTracker_Report_BiasAlert(t *testing.T) {
	tracker := NewNLICalibrationTracker(100, 0.15, 0)
	for range 20 {
		tracker.Track(0.9, 0.5, OutcomeFollowed) // 0.4 bias > 0.15 threshold
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
	tracker := NewNLICalibrationTracker(5, 0.15, 0) // small window
	for i := range 10 {
		tracker.Track(float64(i)*0.1, 0.5, OutcomeFollowed)
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
	tracker := NewNLICalibrationTracker(100, 0.15, 0)

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

// DASHBOARD-TRUTH-001 (a): a mostly-`ignored` regime must NOT fire the bias
// alert. NLI comprehension legitimately scores ignored-but-understood guidance
// high (neutral → 0.5, contradiction → 1.0) while the compliance heuristic maps
// ignored → 0.0; that divergence is by design, not miscalibration. Pre-fix,
// ~80% ignored pinned MeanBias ≈ 0.5 ≫ 0.15 forever.
func TestCalibrationTracker_IgnoredExcluded_NoAlert(t *testing.T) {
	tracker := NewNLICalibrationTracker(500, 0.15, 50)

	// 80% ignored: NLI ~0.68 vs heuristic 0.0 — the permanently-red regime.
	for range 160 {
		tracker.Track(0.68, 0.0, OutcomeIgnored)
	}
	// 20% followed, well-calibrated: NLI ≈ heuristic.
	for range 60 {
		tracker.Track(0.95, 1.0, OutcomeFollowed)
	}

	report := tracker.Report()
	if report.WindowSize != 60 {
		t.Errorf("window size = %d, want 60 (ignored samples must not be admitted)", report.WindowSize)
	}
	if report.InsufficientSamples {
		t.Errorf("60 like-for-like samples ≥ floor 50 → want sufficient, got insufficient")
	}
	if report.BiasAlert {
		t.Errorf("bias alert fired in a mostly-ignored, well-calibrated regime (mean_bias=%f)", report.MeanBias)
	}
	if report.MeanBias < -0.06 || report.MeanBias > -0.04 {
		t.Errorf("mean bias = %f, want ~-0.05 (followed-only window)", report.MeanBias)
	}
}

// DASHBOARD-TRUTH-001 (b): anti-over-correction guard — a genuinely divergent
// followed/partial window above the floor MUST still fire the alert. The
// ignored-exclusion must not lobotomize the detector.
func TestCalibrationTracker_GenuineDivergence_StillAlerts(t *testing.T) {
	tracker := NewNLICalibrationTracker(500, 0.15, 50)

	// Genuine miscalibration: agent follows guidance but NLI scores comprehension low.
	for range 40 {
		tracker.Track(0.5, 1.0, OutcomeFollowed) // bias -0.5
	}
	for range 20 {
		tracker.Track(0.4, 0.7, OutcomePartialCompliance) // bias -0.3
	}
	// Ignored noise mixed in — must not dilute or inflate the signal.
	for range 100 {
		tracker.Track(0.68, 0.0, OutcomeIgnored)
	}

	report := tracker.Report()
	if report.WindowSize != 60 {
		t.Errorf("window size = %d, want 60", report.WindowSize)
	}
	if !report.BiasAlert {
		t.Errorf("bias alert must fire on genuine followed/partial divergence (mean_bias=%f)", report.MeanBias)
	}
	// Expected bias: (40*(-0.5) + 20*(-0.3)) / 60 ≈ -0.433
	if report.MeanBias > -0.42 || report.MeanBias < -0.45 {
		t.Errorf("mean bias = %f, want ~-0.433", report.MeanBias)
	}
}

// DASHBOARD-TRUTH-001 (c): below J17_NLI_CALIBRATION_MIN_SAMPLES no alert may
// fire regardless of bias — a small restart-reset window is "insufficient
// data", not a calibration verdict.
func TestCalibrationTracker_BelowMinSamples_NoAlert(t *testing.T) {
	tracker := NewNLICalibrationTracker(500, 0.15, 50)

	// 16 maximally-divergent samples (the pre-fix live window size).
	for range 16 {
		tracker.Track(1.0, 0.0, OutcomeFollowed)
	}

	report := tracker.Report()
	if !report.InsufficientSamples {
		t.Errorf("16 < floor 50 → want InsufficientSamples=true")
	}
	if report.MinSamples != 50 {
		t.Errorf("MinSamples = %d, want 50", report.MinSamples)
	}
	if report.BiasAlert {
		t.Errorf("bias alert fired below the min-sample floor (window=%d, bias=%f)", report.WindowSize, report.MeanBias)
	}

	// Crossing the floor with the same divergence → alert becomes eligible.
	for range 34 {
		tracker.Track(1.0, 0.0, OutcomeFollowed)
	}
	report = tracker.Report()
	if report.InsufficientSamples {
		t.Errorf("50 samples = floor → want sufficient")
	}
	if !report.BiasAlert {
		t.Errorf("bias alert should fire once the floor is reached (bias=%f)", report.MeanBias)
	}
}

// DASHBOARD-TRUTH-001: the RSIC adapter path treats a sub-floor report as
// no-data. Verify the report itself carries the flag with BiasAlert false so
// consumers can gate on it.
func TestCalibrationTracker_MinSamplesZero_DisablesFloor(t *testing.T) {
	tracker := NewNLICalibrationTracker(100, 0.15, 0)
	tracker.Track(1.0, 0.0, OutcomeFollowed)

	report := tracker.Report()
	if report.InsufficientSamples {
		t.Errorf("minSamples=0 disables the floor → want InsufficientSamples=false")
	}
	if !report.BiasAlert {
		t.Errorf("with the floor disabled a divergent sample should alert (legacy behavior)")
	}
}
