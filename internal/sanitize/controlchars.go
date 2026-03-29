package sanitize

import "strings"

// StripControlChars removes JSON-invalid control characters (U+0000-U+001F)
// except tab (U+0009), newline (U+000A), and carriage return (U+000D).
// This is applied to LLM-generated text to prevent JSON serialization issues.
func StripControlChars(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
			return -1 // drop
		}
		return r
	}, s)
}
