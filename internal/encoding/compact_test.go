package encoding

import (
	"strings"
	"testing"
)

func TestTelegraphicCompress(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(string) bool
	}{
		{
			name:  "removes articles",
			input: "the quick brown fox jumps over the lazy dog",
			check: func(s string) bool {
				return !strings.Contains(s, " the ") && strings.Contains(s, "quick") && strings.Contains(s, "fox")
			},
		},
		{
			name:  "removes pronouns",
			input: "I think you should do it yourself",
			check: func(s string) bool {
				return !strings.Contains(s, " I ") && strings.Contains(s, "think")
			},
		},
		{
			name:  "shorter than input",
			input: "This is a very important constraint that should be followed at all times",
			check: func(s string) bool {
				return len(s) < len("This is a very important constraint that should be followed at all times")
			},
		},
		{
			name:  "preserves content words",
			input: "Auth middleware rewrite is compliance-driven not tech-debt",
			check: func(s string) bool {
				return strings.Contains(s, "Auth") && strings.Contains(s, "middleware") &&
					strings.Contains(s, "compliance-driven") && strings.Contains(s, "not")
			},
		},
		{
			name:  "empty input",
			input: "",
			check: func(s string) bool { return s == "" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TelegraphicCompress(tt.input)
			if !tt.check(result) {
				t.Errorf("TelegraphicCompress(%q) = %q, failed check", tt.input, result)
			}
		})
	}
}

func TestAbbreviateNodeID(t *testing.T) {
	tests := []struct {
		nodeID string
		maxLen int
		want   string
	}{
		{"abc123def456", 8, "abc123de"},
		{"short", 8, "short"},
		{"exactly8", 8, "exactly8"},
		{"", 8, ""},
	}

	for _, tt := range tests {
		got := AbbreviateNodeID(tt.nodeID, tt.maxLen)
		if got != tt.want {
			t.Errorf("AbbreviateNodeID(%q, %d) = %q, want %q", tt.nodeID, tt.maxLen, got, tt.want)
		}
	}
}

func TestTelegraphicEncoder_Interface(t *testing.T) {
	var enc CompactEncoder = NewTelegraphicEncoder()
	if enc.Name() != "telegraphic" {
		t.Errorf("Name() = %q, want 'telegraphic'", enc.Name())
	}

	result := enc.Encode("the quick brown fox")
	if strings.Contains(result, "the") {
		t.Errorf("Encode should remove 'the', got %q", result)
	}
}

func TestCodedEncoder(t *testing.T) {
	codes := map[string]string{
		"no-force-push": "NFP",
		"test-first":    "TF",
	}
	enc := NewCodedEncoder(codes)

	if enc.Encode("no-force-push") != "NFP" {
		t.Errorf("Encode('no-force-push') = %q, want 'NFP'", enc.Encode("no-force-push"))
	}

	// Unknown text returns as-is
	if enc.Encode("unknown") != "unknown" {
		t.Errorf("Encode('unknown') = %q, want 'unknown'", enc.Encode("unknown"))
	}

	if enc.Name() != "coded" {
		t.Errorf("Name() = %q, want 'coded'", enc.Name())
	}
}

func TestCompactJSON(t *testing.T) {
	type sample struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	got := CompactJSON(sample{Name: "test", Value: 42})
	if strings.Contains(got, "\n") {
		t.Errorf("CompactJSON should produce single-line JSON, got: %s", got)
	}
	if got != `{"name":"test","value":42}` {
		t.Errorf("unexpected JSON: %s", got)
	}

	// Nil/invalid input
	got = CompactJSON(make(chan int))
	if got != "{}" {
		t.Errorf("expected {} for unmarshalable input, got: %s", got)
	}
}

func TestTruncateAtWord(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"short", "hello", 10, "hello"},
		{"exact", "hello", 5, "hello"},
		{"truncate at word", "hello world foo bar", 15, "hello world..."},
		{"very short max", "hello world", 3, "hel"},
		{"no spaces", "abcdefghijklmnop", 10, "abcdefg..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateAtWord(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("TruncateAtWord(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
			if len(got) > tt.maxLen {
				t.Errorf("result length %d exceeds maxLen %d", len(got), tt.maxLen)
			}
		})
	}
}

func TestCompressSection(t *testing.T) {
	input := "This is a very long summary that should be compressed by removing stop words and then truncated to the specified maximum length"
	result := CompressSection(input, 50)

	if len(result) > 50 {
		t.Errorf("CompressSection result length %d exceeds maxLen 50", len(result))
	}

	// Should not contain common stop words
	if strings.Contains(result, " is ") || strings.Contains(result, " a ") {
		t.Errorf("CompressSection should remove stop words, got: %s", result)
	}

	// Short input should pass through unchanged (after telegraphic compression)
	short := CompressSection("error found", 100)
	if strings.Contains(short, "...") {
		t.Errorf("short input should not be truncated, got: %s", short)
	}
}
