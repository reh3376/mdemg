package llmclient

import "testing"

func TestStripThinkBlock(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic think block",
			input:    `<think>reasoning here</think>{"result": true}`,
			expected: `{"result": true}`,
		},
		{
			name:     "multiline think block",
			input:    "<think>\nstep 1: analyze\nstep 2: decide\n</think>\n{\"ok\": true}",
			expected: `{"ok": true}`,
		},
		{
			name:     "leading whitespace",
			input:    "  \n <think>thoughts</think> {\"data\": 1}",
			expected: `{"data": 1}`,
		},
		{
			name:     "no think block",
			input:    `{"result": "no think"}`,
			expected: `{"result": "no think"}`,
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "unclosed think tag",
			input:    "<think>unclosed reasoning",
			expected: "<think>unclosed reasoning",
		},
		{
			name:     "think not at start",
			input:    `prefix <think>thoughts</think> suffix`,
			expected: `prefix <think>thoughts</think> suffix`,
		},
		{
			name:     "think block with array response",
			input:    "<think>evaluating scores</think>[0.85, 0.32, 0.71]",
			expected: "[0.85, 0.32, 0.71]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripThinkBlock(tt.input)
			if result != tt.expected {
				t.Errorf("StripThinkBlock() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestSanitizeResponse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "think block + code fence",
			input:    "<think>reasoning</think>```json\n{\"key\": \"value\"}\n```",
			expected: `{"key": "value"}`,
		},
		{
			name:     "think block only",
			input:    "<think>reasoning</think>{\"key\": \"value\"}",
			expected: `{"key": "value"}`,
		},
		{
			name:     "code fence only",
			input:    "```json\n{\"key\": \"value\"}\n```",
			expected: `{"key": "value"}`,
		},
		{
			name:     "plain JSON",
			input:    `{"key": "value"}`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "think + fence + whitespace",
			input:    "  <think>thought</think>  ```json\n[1,2,3]\n```  ",
			expected: "[1,2,3]",
		},
		{
			name:     "empty",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeResponse(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeResponse() = %q, want %q", result, tt.expected)
			}
		})
	}
}
