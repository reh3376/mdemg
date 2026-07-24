// Package sanitize — rune-safe string cutting primitives
// (RUNE-SAFE-STRINGS-001).
//
// Go string slicing operates on BYTES: s[:n] can land mid-rune on multi-byte
// UTF-8 (CJK, emoji, accents), producing an invalid string. Postgres rejects
// invalid UTF-8 — one poison row fails a whole buffered flush batch (the
// TSDB-WRITER-UTF8-001 incident class) — and Neo4j names / LLM prompts / logs
// degrade to mojibake.
//
// These primitives keep byte-budget semantics (max means "at most max
// bytes" — downstream storage/prompt budgets hold) while backing the cut
// onto a rune boundary (≤3 bytes early, never past max). All string
// truncation in this codebase must route through them; do not hand-roll
// `s[:n]` on content that can contain multi-byte runes.
package sanitize

import "unicode/utf8"

// CutRuneSafe returns s cut to at most maxBytes bytes, never splitting a
// multi-byte rune. maxBytes <= 0 returns s unchanged (no-op convention
// shared with the pre-existing truncation helpers).
func CutRuneSafe(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	return s[:backToRuneStart(s, maxBytes)]
}

// CutRuneSafeSuffix cuts s to at most maxBytes bytes on a rune boundary and
// appends suffix when a cut occurred (matching the widespread
// `s[:n] + "..."` idiom, where the suffix rides on top of the budget).
func CutRuneSafeSuffix(s string, maxBytes int, suffix string) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	return s[:backToRuneStart(s, maxBytes)] + suffix
}

// TailRuneSafe returns the last at-most-maxBytes bytes of s, advancing the
// start forward to a rune boundary (for output-tail capture, e.g. command
// output in error messages).
func TailRuneSafe(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	start := len(s) - maxBytes
	for start < len(s) && !utf8.RuneStart(s[start]) {
		start++
	}
	return s[start:]
}

// backToRuneStart backs cut off to the nearest rune boundary at or before it.
func backToRuneStart(s string, cut int) int {
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return cut
}
