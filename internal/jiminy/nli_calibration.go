package jiminy

import (
	"math"
	"sync"
	"time"
)

// NLICalibrationTracker compares NLI and heuristic scores to detect systematic bias.
//
// DASHBOARD-TRUTH-001: the window is LIKE-FOR-LIKE. NLI comprehension scores
// ("did the agent understand the constraint?") and the compliance heuristic
// ("did the agent follow it?") measure different things for `ignored` outcomes:
// NLI legitimately scores an ignored-but-understood constraint high (neutral →
// 0.5, contradiction → 1.0 "understood but violated") while the heuristic maps
// ignored → 0.0. In any sub-~85%-follow regime that divergence-by-design pushed
// MeanBias ≈ 0.5 ≫ threshold forever — a permanently-red alert that carried no
// calibration signal. `ignored` samples are therefore NOT admitted to the ring
// buffer (Track drops them); followed/partial/contradicted samples — where NLI
// and the heuristic are estimating the same quantity — are kept. Ignored
// outcomes still flow to RecordOutcomeWithTier and the guidance corpus, so no
// data is lost; they are only excluded from the *calibration* comparison.
type NLICalibrationTracker struct {
	mu            sync.Mutex
	scores        []nliCalibrationEntry
	head          int
	count         int
	maxSize       int
	biasThreshold float64
	minSamples    int
}

type nliCalibrationEntry struct {
	NLIScore  float64
	Heuristic float64
	Timestamp time.Time
}

// NLICalibrationReport summarizes NLI-vs-heuristic calibration over a sliding window.
type NLICalibrationReport struct {
	MeanNLI       float64 `json:"mean_nli"`
	MeanHeuristic float64 `json:"mean_heuristic"`
	MeanBias      float64 `json:"mean_bias"`
	StdDevNLI     float64 `json:"stddev_nli"`
	WindowSize    int     `json:"window_size"`
	BiasAlert     bool    `json:"bias_alert"`
	// DASHBOARD-TRUTH-001: min-sample floor. When the like-for-like window holds
	// fewer than MinSamples entries the report is "insufficient data" — BiasAlert
	// is forced false and consumers (gauge emitters, the RSIC drift insight) must
	// NOT treat the means as a calibration verdict. Prevents a 16-sample window
	// that resets on every restart from firing a bias alarm.
	InsufficientSamples bool `json:"insufficient_samples"`
	MinSamples          int  `json:"min_samples"`
}

// NewNLICalibrationTracker creates a calibration tracker with the given window
// size, bias threshold, and minimum-sample floor (minSamples <= 0 disables the
// floor). biasThreshold comes from J17_NLI_CALIBRATION_BIAS_THRESHOLD (default
// 0.15) and now applies to the like-for-like (ignored-excluded) window.
func NewNLICalibrationTracker(windowSize int, biasThreshold float64, minSamples int) *NLICalibrationTracker {
	if windowSize <= 0 {
		windowSize = 500
	}
	if minSamples < 0 {
		minSamples = 0
	}
	return &NLICalibrationTracker{
		scores:        make([]nliCalibrationEntry, windowSize),
		maxSize:       windowSize,
		biasThreshold: biasThreshold,
		minSamples:    minSamples,
	}
}

// Track records a paired NLI and heuristic score into the ring buffer.
// Samples whose outcome is OutcomeIgnored are excluded — see the type comment:
// NLI-comprehension vs compliance-heuristic divergence on ignored outcomes is
// by-design, not miscalibration. The ring-buffer cap is preserved.
func (t *NLICalibrationTracker) Track(nliScore, heuristicScore float64, outcome GuidanceOutcome) {
	if outcome == OutcomeIgnored {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.scores[t.head] = nliCalibrationEntry{
		NLIScore:  nliScore,
		Heuristic: heuristicScore,
		Timestamp: time.Now(),
	}
	t.head = (t.head + 1) % t.maxSize
	if t.count < t.maxSize {
		t.count++
	}
}

// Report computes calibration statistics over the current window.
// Below the min-sample floor the means are still computed (informational) but
// BiasAlert is forced false and InsufficientSamples is set — sub-floor windows
// are "insufficient data", not a calibration verdict.
func (t *NLICalibrationTracker) Report() *NLICalibrationReport {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.count == 0 {
		return &NLICalibrationReport{
			InsufficientSamples: t.minSamples > 0,
			MinSamples:          t.minSamples,
		}
	}

	var sumNLI, sumHeuristic float64
	for i := range t.count {
		sumNLI += t.scores[i].NLIScore
		sumHeuristic += t.scores[i].Heuristic
	}

	n := float64(t.count)
	meanNLI := sumNLI / n
	meanHeuristic := sumHeuristic / n
	meanBias := meanNLI - meanHeuristic

	// Standard deviation of NLI scores
	var varianceSum float64
	for i := range t.count {
		diff := t.scores[i].NLIScore - meanNLI
		varianceSum += diff * diff
	}
	stdDevNLI := math.Sqrt(varianceSum / n)

	insufficient := t.count < t.minSamples

	return &NLICalibrationReport{
		MeanNLI:             meanNLI,
		MeanHeuristic:       meanHeuristic,
		MeanBias:            meanBias,
		StdDevNLI:           stdDevNLI,
		WindowSize:          t.count,
		InsufficientSamples: insufficient,
		MinSamples:          t.minSamples,
		// biasThreshold is J17_NLI_CALIBRATION_BIAS_THRESHOLD — config-driven,
		// no hardcoded compare. It now applies to the like-for-like window.
		BiasAlert: !insufficient && math.Abs(meanBias) > t.biasThreshold,
	}
}
