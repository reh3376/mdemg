package sanitize

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// Multi-byte fixtures: é=2 bytes, 世=3 bytes, 🚀=4 bytes.
const (
	twoByte   = "é"
	threeByte = "世"
	fourByte  = "🚀"
)

func TestCutRuneSafe_ASCIIUnchanged(t *testing.T) {
	if got := CutRuneSafe("hello world", 5); got != "hello" {
		t.Fatalf("ASCII cut changed: %q", got)
	}
	if got := CutRuneSafe("hi", 10); got != "hi" {
		t.Fatalf("under-budget must be identity: %q", got)
	}
	if got := CutRuneSafe("hi", 0); got != "hi" {
		t.Fatalf("maxBytes<=0 must be no-op: %q", got)
	}
	if got := CutRuneSafe("hi", -1); got != "hi" {
		t.Fatalf("negative must be no-op: %q", got)
	}
}

// TestCutRuneSafe_EveryStraddle slides the cut across every byte position of
// strings ending in 2/3/4-byte runes and asserts validity + budget at each.
func TestCutRuneSafe_EveryStraddle(t *testing.T) {
	for _, r := range []string{twoByte, threeByte, fourByte} {
		s := "ab" + strings.Repeat(r, 5)
		for max := 1; max <= len(s)+1; max++ {
			got := CutRuneSafe(s, max)
			if !utf8.ValidString(got) {
				t.Fatalf("invalid UTF-8 at max=%d on %q: %q", max, r, got)
			}
			if len(got) > max {
				t.Fatalf("budget exceeded at max=%d: len=%d", max, len(got))
			}
			if !strings.HasPrefix(s, got) {
				t.Fatalf("not a prefix at max=%d: %q", max, got)
			}
		}
	}
}

func TestCutRuneSafe_BacksOffAtMostRuneWidth(t *testing.T) {
	s := "a" + fourByte // len 5
	// Cutting at 2,3,4 all land inside the emoji → back to 1.
	for _, max := range []int{2, 3, 4} {
		if got := CutRuneSafe(s, max); got != "a" {
			t.Fatalf("max=%d: want %q got %q", max, "a", got)
		}
	}
	if got := CutRuneSafe(s, 5); got != s {
		t.Fatalf("exact fit must be identity: %q", got)
	}
}

func TestCutRuneSafeSuffix(t *testing.T) {
	s := "ab" + threeByte + threeByte // len 8
	got := CutRuneSafeSuffix(s, 6, "...")
	// cut=6 lands mid-second-世 → back to 5 → "ab世"+"..."
	if got != "ab"+threeByte+"..." {
		t.Fatalf("suffix cut wrong: %q", got)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("invalid: %q", got)
	}
	// Untruncated: no suffix.
	if got := CutRuneSafeSuffix("short", 10, "..."); got != "short" {
		t.Fatalf("no-cut must not append suffix: %q", got)
	}
	// ASCII behavior byte-identical to the historical s[:n]+"..." idiom.
	if got := CutRuneSafeSuffix("hello world", 5, "..."); got != "hello..." {
		t.Fatalf("ASCII suffix drifted: %q", got)
	}
}

func TestTailRuneSafe(t *testing.T) {
	s := threeByte + threeByte + "xy" // len 8
	got := TailRuneSafe(s, 3)        // start=5 lands mid-second-世 → advance to 6 → "xy"
	if got != "xy" {
		t.Fatalf("tail wrong: %q", got)
	}
	for _, r := range []string{twoByte, threeByte, fourByte} {
		s := strings.Repeat(r, 4) + "end"
		for max := 1; max <= len(s)+1; max++ {
			got := TailRuneSafe(s, max)
			if !utf8.ValidString(got) {
				t.Fatalf("invalid tail at max=%d on %q: %q", max, r, got)
			}
			if len(got) > max {
				t.Fatalf("tail budget exceeded at max=%d: len=%d", max, len(got))
			}
			if !strings.HasSuffix(s, got) {
				t.Fatalf("not a suffix at max=%d: %q", max, got)
			}
		}
	}
	if got := TailRuneSafe("hi", 0); got != "hi" {
		t.Fatalf("maxBytes<=0 must be no-op: %q", got)
	}
}
