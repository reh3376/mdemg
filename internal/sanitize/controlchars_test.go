package sanitize

import "testing"

func TestStripControlChars(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		{
			name: "already clean string",
			in:   "Hello, world!",
			want: "Hello, world!",
		},
		{
			name: "null byte removed",
			in:   "hello\x00world",
			want: "helloworld",
		},
		{
			name: "various control chars removed",
			in:   "\x01\x02\x03\x04\x05\x06\x07\x08text\x0B\x0C\x0E\x0F\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1A\x1B\x1C\x1D\x1E\x1F",
			want: "text",
		},
		{
			name: "tab preserved",
			in:   "col1\tcol2\tcol3",
			want: "col1\tcol2\tcol3",
		},
		{
			name: "newline preserved",
			in:   "line1\nline2\nline3",
			want: "line1\nline2\nline3",
		},
		{
			name: "carriage return preserved",
			in:   "line1\r\nline2\r\n",
			want: "line1\r\nline2\r\n",
		},
		{
			name: "multi-byte unicode preserved emoji",
			in:   "Hello 🌍 World 🚀",
			want: "Hello 🌍 World 🚀",
		},
		{
			name: "multi-byte unicode preserved CJK",
			in:   "日本語テスト",
			want: "日本語テスト",
		},
		{
			name: "mixed content control chars between normal text",
			in:   "abc\x01def\x02ghi",
			want: "abcdefghi",
		},
		{
			name: "all control chars yields empty string",
			in:   "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x0B\x0C\x0E\x0F\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1A\x1B\x1C\x1D\x1E\x1F",
			want: "",
		},
		{
			name: "whitespace trio preserved among control chars",
			in:   "\x00\t\x01\n\x02\r\x03",
			want: "\t\n\r",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripControlChars(tt.in)
			if got != tt.want {
				t.Errorf("StripControlChars(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
