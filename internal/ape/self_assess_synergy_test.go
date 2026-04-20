package ape

import (
	"math"
	"testing"

	"mdemg/internal/config"
)

const synergyFloatTol = 1e-9

func synergyAlmostEqual(a, b float64) bool {
	return math.Abs(a-b) < synergyFloatTol
}

func newSynergyAssessor() *Assessor {
	cfg := config.Config{
		SynergyTargetClaudeLines: 150,
		SynergyTargetMemoryLines: 120,
	}
	return NewAssessor(cfg, nil, nil, nil)
}

func TestScoreSynergy_AllHealthy(t *testing.T) {
	a := newSynergyAssessor()
	r := &SelfAssessmentReport{
		JiminyHealthy:       true,
		SynergyLinesClaude:  100,
		SynergyLinesMemory:  80,
		SynergyOverflowRate: 0,
		SynergyOverlapScore: 0,
	}

	got, _ := a.scoreSynergy(r)
	if !synergyAlmostEqual(got, 1.0) {
		t.Errorf("expected 1.0, got %f", got)
	}
}

func TestScoreSynergy_JiminyUnhealthy(t *testing.T) {
	a := newSynergyAssessor()
	r := &SelfAssessmentReport{
		JiminyHealthy:      false,
		SynergyLinesClaude: 100,
		SynergyLinesMemory: 80,
	}

	got, _ := a.scoreSynergy(r)
	if !synergyAlmostEqual(got, 0.0) {
		t.Errorf("expected 0.0, got %f", got)
	}
}

func TestScoreSynergy_BloatedClaude(t *testing.T) {
	a := newSynergyAssessor()
	// 250 > 150+50=200 → -0.3
	r := &SelfAssessmentReport{
		JiminyHealthy:       true,
		SynergyLinesClaude:  250,
		SynergyLinesMemory:  80,
		SynergyOverflowRate: 0,
		SynergyOverlapScore: 0,
	}

	got, _ := a.scoreSynergy(r)
	want := 0.7 // 1.0 - 0.3
	if !synergyAlmostEqual(got, want) {
		t.Errorf("expected %f, got %f", want, got)
	}
}

func TestScoreSynergy_BloatedMemory(t *testing.T) {
	a := newSynergyAssessor()
	// 250 > 120+60=180 → -0.3
	r := &SelfAssessmentReport{
		JiminyHealthy:       true,
		SynergyLinesClaude:  100,
		SynergyLinesMemory:  250,
		SynergyOverflowRate: 0,
		SynergyOverlapScore: 0,
	}

	got, _ := a.scoreSynergy(r)
	want := 0.7 // 1.0 - 0.3
	if !synergyAlmostEqual(got, want) {
		t.Errorf("expected %f, got %f", want, got)
	}
}

func TestScoreSynergy_HighOverlap(t *testing.T) {
	a := newSynergyAssessor()
	// overlap 0.6 > 0.5 → -0.2
	r := &SelfAssessmentReport{
		JiminyHealthy:       true,
		SynergyLinesClaude:  100,
		SynergyLinesMemory:  80,
		SynergyOverflowRate: 0,
		SynergyOverlapScore: 0.6,
	}

	got, _ := a.scoreSynergy(r)
	want := 0.8 // 1.0 - 0.2
	if !synergyAlmostEqual(got, want) {
		t.Errorf("expected %f, got %f", want, got)
	}
}

func TestScoreSynergy_HighOverflow(t *testing.T) {
	a := newSynergyAssessor()
	// overflow 15 > 10 → -0.3
	r := &SelfAssessmentReport{
		JiminyHealthy:       true,
		SynergyLinesClaude:  100,
		SynergyLinesMemory:  80,
		SynergyOverflowRate: 15,
		SynergyOverlapScore: 0,
	}

	got, _ := a.scoreSynergy(r)
	want := 0.7 // 1.0 - 0.3
	if !synergyAlmostEqual(got, want) {
		t.Errorf("expected %f, got %f", want, got)
	}
}

func TestScoreSynergy_BothFilesZero(t *testing.T) {
	a := newSynergyAssessor()
	r := &SelfAssessmentReport{
		JiminyHealthy:      true,
		SynergyLinesClaude: 0,
		SynergyLinesMemory: 0,
	}

	got, _ := a.scoreSynergy(r)
	if !synergyAlmostEqual(got, 0.0) {
		t.Errorf("expected 0.0 (excluded from formula), got %f", got)
	}
}
