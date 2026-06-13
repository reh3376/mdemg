package retrieval

import "testing"

// CONTEXT-LIVE-001: classifier→category dispatch contract (the real
// function Retrieve calls pre-CacheKey): first mapped type wins, explicit
// category wins, unmapped types derive nothing.
var deriveCategory = deriveCategoryFromQueryType

func TestCategoryDispatch_Semantics(t *testing.T) {
	m := map[string]string{
		"data_flow":    "data_flow_integration",
		"architecture": "architecture_structure",
		"relationship": "relationship",
	}
	cases := []struct {
		name, existing, queryType, want string
	}{
		{"explicit category wins", "business_logic_constraints", "data_flow", "business_logic_constraints"},
		{"single mapped type", "", "data_flow", "data_flow_integration"},
		{"multi-label first mapped wins", "", "code+architecture+data_flow", "architecture_structure"},
		{"unmapped types derive nothing", "", "code+symbol_lookup+generic", ""},
		{"empty query type", "", "", ""},
		{"deterministic order", "", "relationship+architecture", "relationship"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := deriveCategory(tc.existing, tc.queryType, m); got != tc.want {
				t.Errorf("deriveCategory(%q,%q) = %q; want %q", tc.existing, tc.queryType, got, tc.want)
			}
		})
	}
	if got := deriveCategory("", "data_flow", nil); got != "" {
		t.Errorf("empty map must disable dispatch; got %q", got)
	}
}
