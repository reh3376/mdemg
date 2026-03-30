package llmclient

import "strings"

// StripThinkBlock removes leading <think>...</think> blocks from LLM responses.
// Qwen3 models in think mode prepend reasoning blocks that break JSON parsing.
// Handles: leading whitespace, multiline content, unclosed tags (returned as-is).
func StripThinkBlock(s string) string {
	trimmed := strings.TrimSpace(s)
	if !strings.HasPrefix(trimmed, "<think>") {
		return s
	}
	_, after, found := strings.Cut(trimmed, "</think>")
	if !found {
		return s // unclosed think block, return original
	}
	return strings.TrimSpace(after)
}

// StripCodeFence removes markdown code fences that LLMs sometimes wrap around JSON.
func StripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if after, ok := strings.CutPrefix(s, "```json"); ok {
		s = strings.TrimSuffix(after, "```")
	} else if after, ok := strings.CutPrefix(s, "```"); ok {
		s = strings.TrimSuffix(after, "```")
	}
	return strings.TrimSpace(s)
}

// SanitizeResponse applies all response sanitization in order:
//  1. StripThinkBlock (remove think blocks)
//  2. StripCodeFence (remove code fences)
//  3. strings.TrimSpace
//
// Call this before json.Unmarshal on any LLM response to handle Qwen3 think
// mode and common markdown wrapping.
func SanitizeResponse(s string) string {
	s = StripThinkBlock(s)
	s = StripCodeFence(s)
	return strings.TrimSpace(s)
}
