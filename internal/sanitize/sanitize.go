// Package sanitize strips prompt injection patterns from user context
// before it is sent to LLMs. It removes known injection vectors such as
// system prompt overrides, role injection lines, code fence escapes, and
// excessive repetition, then truncates the result to a maximum length.
package sanitize

import (
	"regexp"
	"strings"
)

// injectionPhrases are case-insensitive substrings that indicate a prompt
// injection attempt. Any line containing one of these is removed entirely.
var injectionPhrases = []string{
	"ignore previous instructions",
	"ignore all previous instructions",
	"ignore all prior instructions",
	"ignore your instructions",
	"disregard previous instructions",
	"disregard all previous instructions",
	"disregard your instructions",
	"forget your instructions",
	"forget all previous instructions",
	"override your instructions",
	"you are now",
	"you must now",
	"pretend you are",
	"act as if you are",
	"from now on you are",
	"new instructions:",
	"new system prompt:",
	"begin new conversation",
}

// roleLineRe matches lines that start with a chat-role prefix
// (e.g. "Human:", "Assistant:", "System:") with optional leading whitespace.
var roleLineRe = regexp.MustCompile(`(?i)^\s*(human|assistant|system|user)\s*:`)

// standaloneRolePrefixRe matches bare "system:" or "assistant:" at the
// start of a line even when followed by content (covers non-conversational
// injection like "system: you are a helpful assistant").
var standaloneRolePrefixRe = regexp.MustCompile(`(?i)^\s*(system|assistant)\s*:`)

// codeFenceRe matches triple-backtick fences that could break prompt
// structure (e.g. ``` or ```json).
var codeFenceRe = regexp.MustCompile("^\\s*`{3,}")

// SanitizeUserContext removes prompt injection patterns from input and
// truncates the result to maxLen bytes. A maxLen of 0 or negative means
// no length limit is applied.
//
// The function processes the input line by line and:
//   - Removes lines containing known injection phrases (case insensitive)
//   - Removes lines starting with chat-role prefixes (Human:/Assistant:/System:)
//   - Strips triple-backtick code fences
//   - Collapses runs of more than 5 consecutive identical lines to 5
//   - Truncates the final result to maxLen bytes
func SanitizeUserContext(input string, maxLen int) string {
	if input == "" {
		return ""
	}

	lines := strings.Split(input, "\n")
	out := make([]string, 0, len(lines))

	var prevLine string
	var repeatCount int

	for _, line := range lines {
		// --- Injection phrase check ---
		if containsInjectionPhrase(line) {
			continue
		}

		// --- Role-line check ---
		if roleLineRe.MatchString(line) {
			continue
		}

		// --- Standalone system:/assistant: prefix ---
		if standaloneRolePrefixRe.MatchString(line) {
			continue
		}

		// --- Code fence stripping ---
		if codeFenceRe.MatchString(line) {
			continue
		}

		// --- Excessive repetition (>5 identical consecutive lines) ---
		trimmed := strings.TrimSpace(line)
		if trimmed == prevLine && trimmed != "" {
			repeatCount++
			if repeatCount > 5 {
				continue
			}
		} else {
			prevLine = trimmed
			repeatCount = 1
		}

		out = append(out, line)
	}

	result := strings.Join(out, "\n")

	// --- Length truncation (rune-safe: never split a multi-byte rune) ---
	result = CutRuneSafeSuffix(result, maxLen, "...")

	return result
}

// containsInjectionPhrase returns true if line contains any known
// injection phrase (case-insensitive comparison).
func containsInjectionPhrase(line string) bool {
	lower := strings.ToLower(line)
	for _, phrase := range injectionPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}
