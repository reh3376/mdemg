package ape

import (
	"math"
	"sync"
	"testing"
)

// Helper function for floating point comparison
func almostEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

// =============================================================================
// NewSignalLearner tests
// =============================================================================

func TestNewSignalLearner_DefaultRates(t *testing.T) {
	// Test with zero values - should use defaults
	sl := NewSignalLearner(0, 0)

	if sl.decayRate != 0.05 {
		t.Errorf("decayRate = %f, want 0.05", sl.decayRate)
	}
	if sl.boostRate != 0.1 {
		t.Errorf("boostRate = %f, want 0.1", sl.boostRate)
	}
	if sl.signals == nil {
		t.Error("signals map should be initialized")
	}
}

func TestNewSignalLearner_CustomRates(t *testing.T) {
	// Test with custom values
	sl := NewSignalLearner(0.03, 0.15)

	if sl.decayRate != 0.03 {
		t.Errorf("decayRate = %f, want 0.03", sl.decayRate)
	}
	if sl.boostRate != 0.15 {
		t.Errorf("boostRate = %f, want 0.15", sl.boostRate)
	}
}

func TestNewSignalLearner_NegativeRates(t *testing.T) {
	// Negative values should trigger defaults
	sl := NewSignalLearner(-0.05, -0.1)

	if sl.decayRate != 0.05 {
		t.Errorf("decayRate = %f, want 0.05 (default for negative)", sl.decayRate)
	}
	if sl.boostRate != 0.1 {
		t.Errorf("boostRate = %f, want 0.1 (default for negative)", sl.boostRate)
	}
}

// =============================================================================
// RecordEmission tests
// =============================================================================

func TestRecordEmission_NewSignal(t *testing.T) {
	sl := NewSignalLearner(0.05, 0.1)

	sl.RecordEmission("TEST_001")

	// Verify signal was created
	s, ok := sl.signals["TEST_001"]
	if !ok {
		t.Fatal("signal TEST_001 should be created")
	}

	// Initial strength is 0.5, decay is 0.05 → should be 0.45
	expectedStrength := 0.5 - 0.05
	if s.Strength != expectedStrength {
		t.Errorf("strength = %f, want %f", s.Strength, expectedStrength)
	}

	if s.Emissions != 1 {
		t.Errorf("emissions = %d, want 1", s.Emissions)
	}

	if s.Responses != 0 {
		t.Errorf("responses = %d, want 0", s.Responses)
	}
}

func TestRecordEmission_ExistingSignal(t *testing.T) {
	sl := NewSignalLearner(0.05, 0.1)

	// First emission
	sl.RecordEmission("TEST_002")
	// Second emission
	sl.RecordEmission("TEST_002")

	s := sl.signals["TEST_002"]

	// 0.5 - 0.05 = 0.45 (first)
	// 0.45 - 0.05 = 0.40 (second)
	expectedStrength := 0.5 - 0.05 - 0.05
	if s.Strength != expectedStrength {
		t.Errorf("strength = %f, want %f", s.Strength, expectedStrength)
	}

	if s.Emissions != 2 {
		t.Errorf("emissions = %d, want 2", s.Emissions)
	}
}

func TestRecordEmission_StrengthFloor(t *testing.T) {
	sl := NewSignalLearner(0.05, 0.1)

	// Emit many times to hit the floor
	for i := 0; i < 20; i++ {
		sl.RecordEmission("TEST_FLOOR")
	}

	s := sl.signals["TEST_FLOOR"]

	// Strength should never go below 0.1
	if s.Strength != 0.1 {
		t.Errorf("strength = %f, want 0.1 (floor)", s.Strength)
	}

	if s.Emissions != 20 {
		t.Errorf("emissions = %d, want 20", s.Emissions)
	}
}

func TestRecordEmission_DecayToFloor(t *testing.T) {
	sl := NewSignalLearner(0.05, 0.1)

	// Starting from 0.5, need 8 emissions to reach floor:
	// 0.50 → 0.45 → 0.40 → 0.35 → 0.30 → 0.25 → 0.20 → 0.15 → 0.10
	for i := 0; i < 8; i++ {
		sl.RecordEmission("TEST_DECAY")
	}

	s := sl.signals["TEST_DECAY"]

	if !almostEqual(s.Strength, 0.1, 0.0001) {
		t.Errorf("strength = %f, want 0.1", s.Strength)
	}

	// One more emission should still be at floor
	sl.RecordEmission("TEST_DECAY")
	if !almostEqual(s.Strength, 0.1, 0.0001) {
		t.Errorf("strength = %f, want 0.1 (still at floor)", s.Strength)
	}
}

// =============================================================================
// RecordResponse tests
// =============================================================================

func TestRecordResponse_NewSignal(t *testing.T) {
	sl := NewSignalLearner(0.05, 0.1)

	sl.RecordResponse("RESP_001")

	s, ok := sl.signals["RESP_001"]
	if !ok {
		t.Fatal("signal RESP_001 should be created")
	}

	// Initial strength is 0.5, boost is 0.1 + 0.05 (decay) = 0.15
	// 0.5 + 0.15 = 0.65
	expectedStrength := 0.5 + 0.1 + 0.05
	if s.Strength != expectedStrength {
		t.Errorf("strength = %f, want %f", s.Strength, expectedStrength)
	}

	if s.Responses != 1 {
		t.Errorf("responses = %d, want 1", s.Responses)
	}

	if s.Emissions != 0 {
		t.Errorf("emissions = %d, want 0", s.Emissions)
	}
}

func TestRecordResponse_ExistingSignal(t *testing.T) {
	sl := NewSignalLearner(0.05, 0.1)

	// Set up initial state
	sl.RecordEmission("RESP_002")
	// Respond
	sl.RecordResponse("RESP_002")

	s := sl.signals["RESP_002"]

	// Started at 0.5
	// After emission: 0.5 - 0.05 = 0.45
	// After response: 0.45 + 0.1 + 0.05 = 0.60
	expectedStrength := 0.5 - 0.05 + 0.1 + 0.05
	if !almostEqual(s.Strength, expectedStrength, 0.0001) {
		t.Errorf("strength = %f, want %f", s.Strength, expectedStrength)
	}

	if s.Emissions != 1 {
		t.Errorf("emissions = %d, want 1", s.Emissions)
	}

	if s.Responses != 1 {
		t.Errorf("responses = %d, want 1", s.Responses)
	}
}

func TestRecordResponse_StrengthCeiling(t *testing.T) {
	sl := NewSignalLearner(0.05, 0.1)

	// Respond many times to hit the ceiling
	for i := 0; i < 10; i++ {
		sl.RecordResponse("RESP_CEILING")
	}

	s := sl.signals["RESP_CEILING"]

	// Strength should never go above 1.0
	if s.Strength != 1.0 {
		t.Errorf("strength = %f, want 1.0 (ceiling)", s.Strength)
	}

	if s.Responses != 10 {
		t.Errorf("responses = %d, want 10", s.Responses)
	}
}

func TestRecordResponse_BoostToCeiling(t *testing.T) {
	sl := NewSignalLearner(0.05, 0.1)

	// Starting from 0.5, each boost is 0.15
	// 0.50 → 0.65 → 0.80 → 0.95 → 1.00 (4 responses to reach ceiling)
	for i := 0; i < 4; i++ {
		sl.RecordResponse("RESP_BOOST")
	}

	s := sl.signals["RESP_BOOST"]

	if s.Strength != 1.0 {
		t.Errorf("strength = %f, want 1.0", s.Strength)
	}

	// One more response should still be at ceiling
	sl.RecordResponse("RESP_BOOST")
	if s.Strength != 1.0 {
		t.Errorf("strength = %f, want 1.0 (still at ceiling)", s.Strength)
	}
}

// =============================================================================
// GetStrength tests
// =============================================================================

func TestGetStrength_UnknownSignal(t *testing.T) {
	sl := NewSignalLearner(0.05, 0.1)

	strength := sl.GetStrength("UNKNOWN")

	// Default for unknown signals is 0.5
	if strength != 0.5 {
		t.Errorf("strength = %f, want 0.5 (default)", strength)
	}
}

func TestGetStrength_KnownSignal(t *testing.T) {
	sl := NewSignalLearner(0.05, 0.1)

	sl.RecordEmission("KNOWN")
	strength := sl.GetStrength("KNOWN")

	expectedStrength := 0.5 - 0.05
	if strength != expectedStrength {
		t.Errorf("strength = %f, want %f", strength, expectedStrength)
	}
}

func TestGetStrength_AfterEmissionAndResponse(t *testing.T) {
	sl := NewSignalLearner(0.05, 0.1)

	sl.RecordEmission("MIXED")
	sl.RecordResponse("MIXED")

	strength := sl.GetStrength("MIXED")

	// 0.5 - 0.05 + 0.15 = 0.60
	expectedStrength := 0.5 - 0.05 + 0.1 + 0.05
	if !almostEqual(strength, expectedStrength, 0.0001) {
		t.Errorf("strength = %f, want %f", strength, expectedStrength)
	}
}

// =============================================================================
// GetAllEffectiveness tests
// =============================================================================

func TestGetAllEffectiveness_EmptyLearner(t *testing.T) {
	sl := NewSignalLearner(0.05, 0.1)

	result := sl.GetAllEffectiveness()

	if len(result) != 0 {
		t.Errorf("len(result) = %d, want 0", len(result))
	}
}

func TestGetAllEffectiveness_SingleSignal(t *testing.T) {
	sl := NewSignalLearner(0.05, 0.1)

	sl.RecordEmission("SINGLE")
	sl.RecordResponse("SINGLE")

	result := sl.GetAllEffectiveness()

	if len(result) != 1 {
		t.Fatalf("len(result) = %d, want 1", len(result))
	}

	eff := result[0]
	if eff.Code != "SINGLE" {
		t.Errorf("Code = %q, want SINGLE", eff.Code)
	}
	if eff.Emissions != 1 {
		t.Errorf("Emissions = %d, want 1", eff.Emissions)
	}
	if eff.Responses != 1 {
		t.Errorf("Responses = %d, want 1", eff.Responses)
	}

	expectedStrength := 0.5 - 0.05 + 0.1 + 0.05
	if !almostEqual(eff.Strength, expectedStrength, 0.0001) {
		t.Errorf("Strength = %f, want %f", eff.Strength, expectedStrength)
	}

	expectedRate := 1.0 / 1.0
	if eff.ResponseRate != expectedRate {
		t.Errorf("ResponseRate = %f, want %f", eff.ResponseRate, expectedRate)
	}
}

func TestGetAllEffectiveness_MultipleSignals(t *testing.T) {
	sl := NewSignalLearner(0.05, 0.1)

	// Signal A: 3 emissions, 2 responses
	sl.RecordEmission("A")
	sl.RecordEmission("A")
	sl.RecordEmission("A")
	sl.RecordResponse("A")
	sl.RecordResponse("A")

	// Signal B: 2 emissions, 1 response
	sl.RecordEmission("B")
	sl.RecordEmission("B")
	sl.RecordResponse("B")

	result := sl.GetAllEffectiveness()

	if len(result) != 2 {
		t.Fatalf("len(result) = %d, want 2", len(result))
	}

	// Find each signal in results
	var effA, effB *SignalEffectiveness
	for i := range result {
		if result[i].Code == "A" {
			effA = &result[i]
		} else if result[i].Code == "B" {
			effB = &result[i]
		}
	}

	if effA == nil {
		t.Fatal("signal A not found in results")
	}
	if effB == nil {
		t.Fatal("signal B not found in results")
	}

	// Verify signal A
	if effA.Emissions != 3 {
		t.Errorf("A.Emissions = %d, want 3", effA.Emissions)
	}
	if effA.Responses != 2 {
		t.Errorf("A.Responses = %d, want 2", effA.Responses)
	}
	expectedRateA := 2.0 / 3.0
	if effA.ResponseRate != expectedRateA {
		t.Errorf("A.ResponseRate = %f, want %f", effA.ResponseRate, expectedRateA)
	}

	// Verify signal B
	if effB.Emissions != 2 {
		t.Errorf("B.Emissions = %d, want 2", effB.Emissions)
	}
	if effB.Responses != 1 {
		t.Errorf("B.Responses = %d, want 1", effB.Responses)
	}
	expectedRateB := 1.0 / 2.0
	if effB.ResponseRate != expectedRateB {
		t.Errorf("B.ResponseRate = %f, want %f", effB.ResponseRate, expectedRateB)
	}
}

func TestGetAllEffectiveness_ResponseRateCalculation(t *testing.T) {
	tests := []struct {
		name         string
		emissions    int
		responses    int
		expectedRate float64
	}{
		{"no emissions", 0, 0, 0.0},
		{"no responses", 5, 0, 0.0},
		{"perfect rate", 4, 4, 1.0},
		{"half rate", 10, 5, 0.5},
		{"more responses than emissions", 2, 3, 1.5}, // Edge case - response without emission
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sl := NewSignalLearner(0.05, 0.1)

			code := "TEST_" + tt.name
			for i := 0; i < tt.emissions; i++ {
				sl.RecordEmission(code)
			}
			for i := 0; i < tt.responses; i++ {
				sl.RecordResponse(code)
			}

			result := sl.GetAllEffectiveness()

			// For zero emissions case, no signal is created
			if tt.emissions == 0 && tt.responses == 0 {
				if len(result) != 0 {
					t.Errorf("expected no signals, got %d", len(result))
				}
				return
			}

			if len(result) != 1 {
				t.Fatalf("len(result) = %d, want 1", len(result))
			}

			if result[0].ResponseRate != tt.expectedRate {
				t.Errorf("ResponseRate = %f, want %f", result[0].ResponseRate, tt.expectedRate)
			}
		})
	}
}

// =============================================================================
// Hebbian learning cycle tests
// =============================================================================

func TestHebbianCycle_DecayWithoutResponse(t *testing.T) {
	sl := NewSignalLearner(0.05, 0.1)

	// Emit multiple times without response
	initialStrength := 0.5
	for i := 0; i < 3; i++ {
		sl.RecordEmission("HEBBIAN_DECAY")
	}

	s := sl.signals["HEBBIAN_DECAY"]

	// Each emission decays by 0.05
	expectedStrength := initialStrength - 0.05 - 0.05 - 0.05
	if s.Strength != expectedStrength {
		t.Errorf("strength = %f, want %f", s.Strength, expectedStrength)
	}
}

func TestHebbianCycle_BoostAfterDecay(t *testing.T) {
	sl := NewSignalLearner(0.05, 0.1)

	// Emit, then respond
	sl.RecordEmission("HEBBIAN_BOOST")
	strengthAfterEmit := sl.GetStrength("HEBBIAN_BOOST")

	sl.RecordResponse("HEBBIAN_BOOST")
	strengthAfterResp := sl.GetStrength("HEBBIAN_BOOST")

	// Response should increase strength
	if strengthAfterResp <= strengthAfterEmit {
		t.Errorf("strength after response (%f) should be greater than after emission (%f)",
			strengthAfterResp, strengthAfterEmit)
	}

	// Response boost should be greater than emission decay
	boost := strengthAfterResp - strengthAfterEmit
	expectedBoost := 0.1 + 0.05 // boostRate + decayRate
	if !almostEqual(boost, expectedBoost, 0.0001) {
		t.Errorf("boost = %f, want %f", boost, expectedBoost)
	}
}

func TestHebbianCycle_EmitResponseEmitResponse(t *testing.T) {
	sl := NewSignalLearner(0.05, 0.1)

	// Cycle: emit → respond → emit → respond
	strengthHistory := []float64{0.5} // initial

	sl.RecordEmission("CYCLE")
	strengthHistory = append(strengthHistory, sl.GetStrength("CYCLE"))

	sl.RecordResponse("CYCLE")
	strengthHistory = append(strengthHistory, sl.GetStrength("CYCLE"))

	sl.RecordEmission("CYCLE")
	strengthHistory = append(strengthHistory, sl.GetStrength("CYCLE"))

	sl.RecordResponse("CYCLE")
	strengthHistory = append(strengthHistory, sl.GetStrength("CYCLE"))

	// Verify progression
	// 0: 0.50 (initial)
	// 1: 0.45 (emit)
	// 2: 0.60 (respond)
	// 3: 0.55 (emit)
	// 4: 0.70 (respond)

	expected := []float64{0.5, 0.45, 0.60, 0.55, 0.70}
	for i, exp := range expected {
		if !almostEqual(strengthHistory[i], exp, 0.0001) {
			t.Errorf("strength[%d] = %f, want %f", i, strengthHistory[i], exp)
		}
	}
}

func TestHebbianCycle_NoResponseLeadsToWeakening(t *testing.T) {
	sl := NewSignalLearner(0.05, 0.1)

	initialStrength := sl.GetStrength("WEAK")

	// Many emissions with no response
	for i := 0; i < 10; i++ {
		sl.RecordEmission("WEAK")
	}

	finalStrength := sl.GetStrength("WEAK")

	if finalStrength >= initialStrength {
		t.Errorf("finalStrength (%f) should be less than initial (%f)", finalStrength, initialStrength)
	}

	// Should hit floor
	if finalStrength != 0.1 {
		t.Errorf("finalStrength = %f, want 0.1 (floor)", finalStrength)
	}
}

func TestHebbianCycle_ConsistentResponseLeadsToStrengthening(t *testing.T) {
	sl := NewSignalLearner(0.05, 0.1)

	initialStrength := sl.GetStrength("STRONG")

	// Emit and respond consistently
	for i := 0; i < 5; i++ {
		sl.RecordEmission("STRONG")
		sl.RecordResponse("STRONG")
	}

	finalStrength := sl.GetStrength("STRONG")

	if finalStrength <= initialStrength {
		t.Errorf("finalStrength (%f) should be greater than initial (%f)", finalStrength, initialStrength)
	}

	// Each cycle: -0.05 (emit) +0.15 (respond) = +0.10 net gain
	// 5 cycles: 0.5 + 0.1*5 = 1.0
	if finalStrength != 1.0 {
		t.Errorf("finalStrength = %f, want 1.0 (ceiling)", finalStrength)
	}
}

// =============================================================================
// Concurrent safety tests
// =============================================================================

func TestConcurrentEmissions(t *testing.T) {
	sl := NewSignalLearner(0.05, 0.1)

	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sl.RecordEmission("CONCURRENT_EMIT")
		}()
	}

	wg.Wait()

	s := sl.signals["CONCURRENT_EMIT"]
	if s.Emissions != numGoroutines {
		t.Errorf("emissions = %d, want %d", s.Emissions, numGoroutines)
	}
}

func TestConcurrentResponses(t *testing.T) {
	sl := NewSignalLearner(0.05, 0.1)

	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sl.RecordResponse("CONCURRENT_RESP")
		}()
	}

	wg.Wait()

	s := sl.signals["CONCURRENT_RESP"]
	if s.Responses != numGoroutines {
		t.Errorf("responses = %d, want %d", s.Responses, numGoroutines)
	}
}

func TestConcurrentMixedOperations(t *testing.T) {
	sl := NewSignalLearner(0.05, 0.1)

	var wg sync.WaitGroup
	numOpsPerType := 50

	// Concurrent emissions
	for i := 0; i < numOpsPerType; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sl.RecordEmission("MIXED")
		}()
	}

	// Concurrent responses
	for i := 0; i < numOpsPerType; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sl.RecordResponse("MIXED")
		}()
	}

	// Concurrent reads
	for i := 0; i < numOpsPerType; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = sl.GetStrength("MIXED")
		}()
	}

	wg.Wait()

	// No panics = success
	s := sl.signals["MIXED"]
	if s.Emissions != numOpsPerType {
		t.Errorf("emissions = %d, want %d", s.Emissions, numOpsPerType)
	}
	if s.Responses != numOpsPerType {
		t.Errorf("responses = %d, want %d", s.Responses, numOpsPerType)
	}
}

func TestConcurrentGetAllEffectiveness(t *testing.T) {
	sl := NewSignalLearner(0.05, 0.1)

	// Set up some signals
	sl.RecordEmission("SIG1")
	sl.RecordResponse("SIG1")
	sl.RecordEmission("SIG2")

	var wg sync.WaitGroup
	numReaders := 50

	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := sl.GetAllEffectiveness()
			if len(result) < 1 {
				t.Error("GetAllEffectiveness should return at least 1 signal")
			}
		}()
	}

	wg.Wait()
}

func TestConcurrentMultipleSignals(t *testing.T) {
	sl := NewSignalLearner(0.05, 0.1)

	var wg sync.WaitGroup
	signals := []string{"A", "B", "C", "D", "E"}
	numOpsPerSignal := 20

	for _, sig := range signals {
		for i := 0; i < numOpsPerSignal; i++ {
			wg.Add(2)
			s := sig // Capture for closure
			go func() {
				defer wg.Done()
				sl.RecordEmission(s)
			}()
			go func() {
				defer wg.Done()
				sl.RecordResponse(s)
			}()
		}
	}

	wg.Wait()

	// Verify all signals were tracked
	result := sl.GetAllEffectiveness()
	if len(result) != len(signals) {
		t.Errorf("len(result) = %d, want %d", len(result), len(signals))
	}

	// Each signal should have correct counts
	for _, eff := range result {
		if eff.Emissions != numOpsPerSignal {
			t.Errorf("%s: emissions = %d, want %d", eff.Code, eff.Emissions, numOpsPerSignal)
		}
		if eff.Responses != numOpsPerSignal {
			t.Errorf("%s: responses = %d, want %d", eff.Code, eff.Responses, numOpsPerSignal)
		}
	}
}

// =============================================================================
// Strength bounds tests
// =============================================================================

func TestStrengthBounds_NeverBelowFloor(t *testing.T) {
	sl := NewSignalLearner(0.05, 0.1)

	// Extreme decay attempt
	for i := 0; i < 1000; i++ {
		sl.RecordEmission("FLOOR_TEST")
	}

	strength := sl.GetStrength("FLOOR_TEST")
	if strength < 0.1 {
		t.Errorf("strength = %f, should never be below 0.1", strength)
	}
	if strength != 0.1 {
		t.Errorf("strength = %f, should be exactly 0.1 after extreme decay", strength)
	}
}

func TestStrengthBounds_NeverAboveCeiling(t *testing.T) {
	sl := NewSignalLearner(0.05, 0.1)

	// Extreme boost attempt
	for i := 0; i < 1000; i++ {
		sl.RecordResponse("CEILING_TEST")
	}

	strength := sl.GetStrength("CEILING_TEST")
	if strength > 1.0 {
		t.Errorf("strength = %f, should never be above 1.0", strength)
	}
	if strength != 1.0 {
		t.Errorf("strength = %f, should be exactly 1.0 after extreme boost", strength)
	}
}

func TestStrengthBounds_AlternatingOperations(t *testing.T) {
	sl := NewSignalLearner(0.05, 0.1)

	// Alternate between decay and boost many times
	for i := 0; i < 100; i++ {
		sl.RecordEmission("ALTERNATING")
		sl.RecordResponse("ALTERNATING")
	}

	strength := sl.GetStrength("ALTERNATING")

	// Should be within valid bounds
	if strength < 0.1 || strength > 1.0 {
		t.Errorf("strength = %f, should be between 0.1 and 1.0", strength)
	}
}

func TestStrengthBounds_CustomHighDecay(t *testing.T) {
	// Test with high decay rate
	sl := NewSignalLearner(0.2, 0.1)

	// With 0.2 decay, should hit floor in 2 emissions from 0.5
	// 0.5 → 0.3 → 0.1
	sl.RecordEmission("HIGH_DECAY")
	sl.RecordEmission("HIGH_DECAY")

	strength := sl.GetStrength("HIGH_DECAY")
	if strength != 0.1 {
		t.Errorf("strength = %f, want 0.1 (floor)", strength)
	}
}

func TestStrengthBounds_CustomHighBoost(t *testing.T) {
	// Test with high boost rate
	sl := NewSignalLearner(0.05, 0.5)

	// With 0.5 boost + 0.05 decay = 0.55 total boost
	// 0.5 → 1.05 (capped to 1.0) in one response
	sl.RecordResponse("HIGH_BOOST")

	strength := sl.GetStrength("HIGH_BOOST")
	if strength != 1.0 {
		t.Errorf("strength = %f, want 1.0 (ceiling)", strength)
	}
}

func TestStrengthBounds_ZeroEmissionsZeroResponses(t *testing.T) {
	sl := NewSignalLearner(0.05, 0.1)

	// Get strength for signal that never had any operations
	strength := sl.GetStrength("NEVER_USED")

	if strength != 0.5 {
		t.Errorf("strength = %f, want 0.5 (default)", strength)
	}
}
