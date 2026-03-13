package retrieval

import "testing"

func TestEscapeLuceneQuery(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text passthrough", "hello world", "hello world"},
		{"colon escaped", "field:value", `field\:value`},
		{"parens escaped", "func(a)", `func\(a\)`},
		{"brackets escaped", "arr[0]", `arr\[0\]`},
		{"double ampersand", "a && b", `a \&\& b`},
		{"double pipe", "a || b", `a \|\| b`},
		{"mixed specials", `path/to/file.go:42 (main)`, `path\/to\/file.go\:42 \(main\)`},
		{"asterisk and question mark", "test*?", `test\*\?`},
		{"tilde and caret", "foo^2 ~0.8", `foo\^2 \~0.8`},
		{"curly braces", "{min TO max}", `\{min TO max\}`},
		{"exclamation mark", "!important", `\!important`},
		{"backslash", `a\b`, `a\\b`},
		{"quotes", `"exact match"`, `\"exact match\"`},
		{"empty string", "", ""},
		{"single ampersand", "a & b", "a & b"},
		{"single pipe", "a | b", "a | b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeLuceneQuery(tt.input)
			if got != tt.want {
				t.Errorf("escapeLuceneQuery(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
