package jiminy

import (
	"math"
	"sync"
	"time"
)

// NLICalibrationTracker compares NLI and heuristic scores to detect systematic bias.
type NLICalibrationTracker struct {
	mu            sync.Mutex
	scores        []nliCalibrationEntry
	head          int
	count         int
	maxSize       int
	biasThreshold float64
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
}

// NewNLICalibrationTracker creates a calibration tracker with the given window size and bias threshold.
func NewNLICalibrationTracker(windowSize int, biasThreshold float64) *NLICalibrationTracker {
	if windowSize <= 0 {
		windowSize = 500
	}
	return &NLICalibrationTracker{
		scores:        make([]nliCalibrationEntry, windowSize),
		maxSize:       windowSize,
		biasThreshold: biasThreshold,
	}
}

// Track records a paired NLI and heuristic score into the ring buffer.
func (t *NLICalibrationTracker) Track(nliScore, heuristicScore float64) {
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
func (t *NLICalibrationTracker) Report() *NLICalibrationReport {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.count == 0 {
		return &NLICalibrationReport{}
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

	return &NLICalibrationReport{
		MeanNLI:       meanNLI,
		MeanHeuristic: meanHeuristic,
		MeanBias:      meanBias,
		StdDevNLI:     stdDevNLI,
		WindowSize:    t.count,
		BiasAlert:     math.Abs(meanBias) > t.biasThreshold,
	}
}
