package llmclient

import (
	"testing"
)

func TestScrub_APIKeys(t *testing.T) {
	rec := &InteractionRecord{
		UserPrompt: "use key sk-abc12345678901234567890 here",
	}
	Scrub(rec)
	want := "use key [REDACTED_KEY] here"
	if rec.UserPrompt != want {
		t.Errorf("APIKeys: got %q, want %q", rec.UserPrompt, want)
	}
}

func TestScrub_AWSKeys(t *testing.T) {
	rec := &InteractionRecord{
		UserPrompt: "aws key AKIAIOSFODNN7EXAMPLE is exposed",
	}
	Scrub(rec)
	want := "aws key [REDACTED_KEY] is exposed"
	if rec.UserPrompt != want {
		t.Errorf("AWSKeys: got %q, want %q", rec.UserPrompt, want)
	}
}

func TestScrub_BearerTokens(t *testing.T) {
	rec := &InteractionRecord{
		UserPrompt: "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.abc",
	}
	Scrub(rec)
	want := "Authorization: [REDACTED_KEY]"
	if rec.UserPrompt != want {
		t.Errorf("BearerTokens: got %q, want %q", rec.UserPrompt, want)
	}
}

func TestScrub_GitHubTokens(t *testing.T) {
	rec := &InteractionRecord{
		UserPrompt: "token ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij is here",
	}
	Scrub(rec)
	want := "token [REDACTED_KEY] is here"
	if rec.UserPrompt != want {
		t.Errorf("GitHubTokens: got %q, want %q", rec.UserPrompt, want)
	}
}

func TestScrub_AbsolutePaths(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "macOS path",
			input: "file at /Users/reh3376/mdemg/internal/api/server.go found",
			want:  "file at /[PATH]/api/server.go found",
		},
		{
			name:  "Linux path",
			input: "see /home/deploy/app/config.yaml for details",
			want:  "see /[PATH]/app/config.yaml for details",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &InteractionRecord{UserPrompt: tt.input}
			Scrub(rec)
			if rec.UserPrompt != tt.want {
				t.Errorf("got %q, want %q", rec.UserPrompt, tt.want)
			}
		})
	}
}

func TestScrub_EnvSecrets(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "PASSWORD equals",
			input: "set PASSWORD=hunter2 in env",
			want:  "set PASSWORD=[REDACTED] in env",
		},
		{
			name:  "SECRET colon",
			input: `SECRET: "my-secret-value" is set`,
			want:  `SECRET=[REDACTED] is set`,
		},
		{
			name:  "API_KEY",
			input: "API_KEY=abcdef123456 exported",
			want:  "API_KEY=[REDACTED] exported",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &InteractionRecord{UserPrompt: tt.input}
			Scrub(rec)
			if rec.UserPrompt != tt.want {
				t.Errorf("got %q, want %q", rec.UserPrompt, tt.want)
			}
		})
	}
}

func TestScrub_Emails(t *testing.T) {
	rec := &InteractionRecord{
		UserPrompt: "contact user@example.com for help",
	}
	Scrub(rec)
	want := "contact [EMAIL] for help"
	if rec.UserPrompt != want {
		t.Errorf("Emails: got %q, want %q", rec.UserPrompt, want)
	}
}

func TestScrub_Neo4jCreds(t *testing.T) {
	rec := &InteractionRecord{
		UserPrompt: "connect to neo4j://user:pass@host:7687/db",
	}
	Scrub(rec)
	want := "connect to neo4j://[REDACTED]@host:7687/db"
	if rec.UserPrompt != want {
		t.Errorf("Neo4jCreds: got %q, want %q", rec.UserPrompt, want)
	}
}

func TestScrub_PreservesCleanText(t *testing.T) {
	clean := `func main() {
	fmt.Println("hello world")
	x := 42
}`
	rec := &InteractionRecord{
		UserPrompt: clean,
		Response:   clean,
	}
	Scrub(rec)
	if rec.UserPrompt != clean {
		t.Errorf("UserPrompt modified: got %q", rec.UserPrompt)
	}
	if rec.Response != clean {
		t.Errorf("Response modified: got %q", rec.Response)
	}
}

func TestScrub_EmptyString(t *testing.T) {
	rec := &InteractionRecord{}
	Scrub(rec)
	if rec.UserPrompt != "" || rec.Response != "" || rec.SystemPrompt != "" || rec.ThinkContent != "" {
		t.Error("empty fields should remain empty")
	}
}

func TestScrub_ThinkContent(t *testing.T) {
	rec := &InteractionRecord{
		ThinkContent: "the user's email is admin@secret.org and key is sk-AAAABBBBCCCCDDDDEEEEFFFFF",
	}
	Scrub(rec)
	want := "the user's email is [EMAIL] and key is [REDACTED_KEY]"
	if rec.ThinkContent != want {
		t.Errorf("ThinkContent: got %q, want %q", rec.ThinkContent, want)
	}
}

func TestScrub_MultiByte(t *testing.T) {
	input := "用户 user@example.com 的路径 /Users/reh3376/mdemg/cmd/main.go 含密码 PASSWORD=秘密"
	rec := &InteractionRecord{UserPrompt: input}
	Scrub(rec)

	// Email should be redacted
	if want := "[EMAIL]"; !contains(rec.UserPrompt, want) {
		t.Errorf("expected %q in result, got %q", want, rec.UserPrompt)
	}
	// Path should be redacted
	if want := "/[PATH]/cmd/main.go"; !contains(rec.UserPrompt, want) {
		t.Errorf("expected %q in result, got %q", want, rec.UserPrompt)
	}
	// PASSWORD value should be redacted
	if want := "PASSWORD=[REDACTED]"; !contains(rec.UserPrompt, want) {
		t.Errorf("expected %q in result, got %q", want, rec.UserPrompt)
	}
	// Unicode text outside sensitive patterns should be preserved
	if want := "用户"; !contains(rec.UserPrompt, want) {
		t.Errorf("expected Unicode %q preserved in result, got %q", want, rec.UserPrompt)
	}
}

func TestScrubStringExcluding_SkipAbsPath(t *testing.T) {
	input := "query for /Users/reh3376/mdemg/internal/retrieval/pipeline.go"
	result := ScrubStringExcluding(input, []string{"abs_path"})
	// abs_path should NOT be redacted
	if result != input {
		t.Errorf("abs_path should be skipped, got %q", result)
	}
}

func TestScrubStringExcluding_APIKeyStillCaughtWhenAbsPathSkipped(t *testing.T) {
	input := "/Users/reh3376/project/main.go sk-abc12345678901234567890"
	result := ScrubStringExcluding(input, []string{"abs_path"})
	// abs_path should be preserved, but API key should still be caught
	if !contains(result, "/Users/reh3376/project/main.go") {
		t.Errorf("abs_path should be preserved, got %q", result)
	}
	if !contains(result, "[REDACTED_KEY]") {
		t.Errorf("API key should still be caught, got %q", result)
	}
}

func TestScrubStringExcluding_NilSkipEquivalentToScrubString(t *testing.T) {
	input := "email user@example.com at /Users/reh3376/code/main.go with PASSWORD=secret"
	got := ScrubStringExcluding(input, nil)
	want := ScrubString(input)
	if got != want {
		t.Errorf("nil skip should equal ScrubString\ngot:  %q\nwant: %q", got, want)
	}
}

func TestScrubStringExcluding_EmptySkipEquivalentToScrubString(t *testing.T) {
	input := "email user@example.com at /Users/reh3376/code/main.go with PASSWORD=secret"
	got := ScrubStringExcluding(input, []string{})
	want := ScrubString(input)
	if got != want {
		t.Errorf("empty skip should equal ScrubString\ngot:  %q\nwant: %q", got, want)
	}
}

func TestScrubStringExcluding_MultipleSkips(t *testing.T) {
	input := "user@example.com at /Users/reh3376/code/main.go"
	result := ScrubStringExcluding(input, []string{"abs_path", "email"})
	// Both patterns skipped — input should be unchanged
	if result != input {
		t.Errorf("both patterns skipped should return original, got %q", result)
	}
}

func TestScrub_GoldenRegression(t *testing.T) {
	// Golden test: verify Scrub() produces known output for known input.
	// This catches any inadvertent change from the patternEntry refactor.
	rec := &InteractionRecord{
		SystemPrompt: "You are a helpful assistant. Contact admin@corp.com for support.",
		UserPrompt:   "file at /Users/reh3376/mdemg/internal/api/server.go found with key sk-abc12345678901234567890",
		Response:     "Set PASSWORD=hunter2 in env. See neo4j://admin:secret@localhost:7687 for DB.",
		ThinkContent: "The user has token ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij",
	}
	Scrub(rec)

	wantSystem := "You are a helpful assistant. Contact [EMAIL] for support."
	wantUser := "file at /[PATH]/api/server.go found with key [REDACTED_KEY]"
	wantResponse := "Set PASSWORD=[REDACTED] in env. See neo4j://[REDACTED]@localhost:7687 for DB."
	wantThink := "The user has token [REDACTED_KEY]"

	if rec.SystemPrompt != wantSystem {
		t.Errorf("SystemPrompt:\n  got:  %q\n  want: %q", rec.SystemPrompt, wantSystem)
	}
	if rec.UserPrompt != wantUser {
		t.Errorf("UserPrompt:\n  got:  %q\n  want: %q", rec.UserPrompt, wantUser)
	}
	if rec.Response != wantResponse {
		t.Errorf("Response:\n  got:  %q\n  want: %q", rec.Response, wantResponse)
	}
	if rec.ThinkContent != wantThink {
		t.Errorf("ThinkContent:\n  got:  %q\n  want: %q", rec.ThinkContent, wantThink)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
