package symbols

import "testing"

// GO-IMPLEMENTS-002 — pin tests for the generated-protobuf filter.

func TestIsGeneratedProtobuf_MatchesExpectedShapes(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		// Positive matches — generated protobuf/gRPC.
		{"/Users/x/mdemg/api/modulepb/mdemg-module.pb.go", true},
		{"/Users/x/mdemg/api/modulepb/mdemg-module_grpc.pb.go", true},
		{"api/devspacepb/devspace.pb.go", true},
		{"api/devspacepb/devspace_grpc.pb.go", true},
		// Negative — regular Go files.
		{"/Users/x/mdemg/internal/api/server.go", false},
		{"internal/symbols/go_types.go", false},
		{"main.go", false},
		// Edge — a file named like a pb.go but NOT actually so (must still match
		// because the .pb.go SUFFIX is the invariant; a hand-authored file
		// intentionally ending in .pb.go would be an operator anti-pattern).
		{"weird_manual.pb.go", true},
		// Edge — .go inside a pb directory is NOT excluded.
		{"api/pb/helper.go", false},
	}
	for _, c := range cases {
		got := isGeneratedProtobuf(c.path)
		if got != c.want {
			t.Errorf("isGeneratedProtobuf(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

// TestIsGeneratedProtobuf_EmptyPath — defensive: no crash on empty input.
func TestIsGeneratedProtobuf_EmptyPath(t *testing.T) {
	if isGeneratedProtobuf("") {
		t.Error("empty path must not match")
	}
}
