package api

import "testing"

func TestShouldUpgradeLabel(t *testing.T) {
	tests := []struct {
		name      string
		oldSource string
		newSource string
		want      bool
	}{
		// Upgrade: artifact label replaced by a real verdict.
		{"blank → llm upgrades", "", "llm", true},
		{"heuristic → llm upgrades", "heuristic", "llm", true},
		{"blank → tier1 upgrades", "", "tier1", true},
		{"heuristic → explicit upgrades", "heuristic", "explicit", true},
		// No upgrade: classifier still fell back to heuristic (LLM unavailable).
		{"heuristic → heuristic no-op", "heuristic", "heuristic", false},
		{"blank → blank no-op", "", "", false},
		{"blank → heuristic no-op", "", "heuristic", false},
		// No upgrade: already a real verdict — never downgrade or churn.
		{"llm → llm not re-upgraded", "llm", "llm", false},
		{"llm → heuristic never downgrades", "llm", "heuristic", false},
		{"tier1 → llm not re-upgraded", "tier1", "llm", false},
		{"explicit → llm not re-upgraded", "explicit", "llm", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldUpgradeLabel(tt.oldSource, tt.newSource); got != tt.want {
				t.Errorf("shouldUpgradeLabel(%q,%q) = %v, want %v", tt.oldSource, tt.newSource, got, tt.want)
			}
		})
	}
}
