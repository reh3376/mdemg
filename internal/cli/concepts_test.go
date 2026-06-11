package cli

import (
	"strings"
	"testing"
)

// Safety pins: repair must default to preview mode, and the childless
// predicate must include ABSTRACTS_TO — the hidden layer's actual
// grounding edge (a GENERALIZES-only predicate over-counts 19k vs 10k
// live; verified against mdemg-dev during PR-B).
func TestConceptsRepair_DefaultsToDryRun(t *testing.T) {
	cmd := newConceptsRepairCmd()
	if v, _ := cmd.Flags().GetBool("dry-run"); !v {
		t.Error("repair must default to dry-run=true")
	}
	if v, _ := cmd.Flags().GetInt("limit"); v != 0 {
		t.Errorf("limit default = %d, want 0", v)
	}
}

func TestConceptChildEdges_IncludesAbstractsTo(t *testing.T) {
	for _, want := range []string{"ABSTRACTS_TO", "GENERALIZES"} {
		if !strings.Contains(conceptChildEdges, want) {
			t.Errorf("conceptChildEdges missing %s", want)
		}
	}
}

func TestConceptsCmd_HasSubcommands(t *testing.T) {
	cmd := newConceptsCmd()
	names := map[string]bool{}
	for _, c := range cmd.Commands() {
		names[c.Name()] = true
	}
	for _, want := range []string{"repair", "trace"} {
		if !names[want] {
			t.Errorf("concepts missing subcommand %s", want)
		}
	}
}
