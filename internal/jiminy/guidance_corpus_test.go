package jiminy

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateBytes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"under limit unchanged", "hello", 10, "hello"},
		{"exactly at limit unchanged", "hello", 5, "hello"},
		{"max zero returns unchanged", "anything goes", 0, "anything goes"},
		{"max negative returns unchanged", "anything", -1, "anything"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncateBytes(tt.in, tt.max); got != tt.want {
				t.Errorf("truncateBytes(%q,%d) = %q, want %q", tt.in, tt.max, got, tt.want)
			}
		})
	}
}

func TestTruncateBytes_TruncatesAndMarks(t *testing.T) {
	in := strings.Repeat("x", 100)
	got := truncateBytes(in, 10)
	if !strings.HasSuffix(got, "…[truncated]") {
		t.Errorf("expected truncation marker, got %q", got)
	}
	if !strings.HasPrefix(got, "xxxxxxxxxx") {
		t.Errorf("expected first 10 bytes preserved, got %q", got)
	}
}

func TestTruncateBytes_UTF8BoundarySafe(t *testing.T) {
	// "héllo…" with multibyte runes; truncating mid-rune must back off to a
	// valid boundary so the result stays valid UTF-8.
	in := "héllo wörld with ünïcödé that goes on for a while ☃☃☃"
	for max := 1; max < len(in); max++ {
		got := truncateBytes(in, max)
		// Strip the marker before validity check (the marker itself is valid UTF-8).
		body := strings.TrimSuffix(got, "…[truncated]")
		if !utf8.ValidString(body) {
			t.Errorf("truncateBytes at max=%d produced invalid UTF-8: %q", max, body)
		}
	}
}
