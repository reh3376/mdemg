package llmclient

import "testing"

// SCRUB-ENV-REF-001 — pin tests for the env-secret refinement that preserves
// shell env-var REFERENCES (which are pointers, not values) while continuing
// to redact literal secret values.

func TestScrubEnvSecret_LiteralValueRedacted(t *testing.T) {
	// Baseline regression: literal secret values still get redacted.
	cases := []struct {
		in, want string
	}{
		{"PASSWORD=hunter2", "PASSWORD=[REDACTED]"},
		{"SECRET=abc123", "SECRET=[REDACTED]"},
		{"API_KEY=sk_test_xyz", "API_KEY=[REDACTED]"},
		{"TOKEN=eyJhbG.abc.def", "TOKEN=[REDACTED]"},
		{`PASSWORD="quoted-value"`, `PASSWORD=[REDACTED]`}, // opening quote is stripped by the outer regex, closing quote survives replacement
	}
	for _, c := range cases {
		got := ScrubString(c.in)
		if got != c.want {
			t.Errorf("ScrubString(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestScrubEnvSecret_ShellRefPreserved(t *testing.T) {
	// The whole point of SCRUB-ENV-REF-001: shell env-var references are
	// POINTERS, not values. They must survive the scrubber unchanged so
	// diagnostic captures like `PGPASSWORD=$TSDB_PASSWORD` don't block
	// export-auto with a false PII alarm.
	cases := []struct {
		name, in string
	}{
		{"bash-plain", "PASSWORD=$TSDB_PASSWORD"},
		{"bash-braced", "PASSWORD=${TSDB_PASSWORD}"},
		{"bash-braced-default", "PASSWORD=${TSDB_PASSWORD:-fallback}"},
		{"bash-braced-error", "PASSWORD=${TSDB_PASSWORD:?required}"},
		{"windows-percent", "PASSWORD=%TSDB_PASSWORD%"},
		{"api-key-ref", "API_KEY=$OPENAI_KEY"},
		{"secret-braced", "SECRET=${MY_SECRET}"},
		{"token-plain", "TOKEN=$GITHUB_TOKEN"},
		{"private-key-ref", "PRIVATE_KEY=${SIGNING_KEY}"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ScrubString(c.in)
			if got != c.in {
				t.Errorf("shell env-var reference must be preserved.\n  in:  %q\n  got: %q", c.in, got)
			}
		})
	}
}

// The exact live production string that fired the false alarm — regression
// pin. If the scrubber ever stops preserving this class, export-auto will
// spuriously halt again with the same class of "PII" alert.
func TestScrubEnvSecret_LiveExportAutoRegressionPin(t *testing.T) {
	input := `bash error: source .env 2>/dev/null; PGPASSWORD=$TSDB_PASSWORD psql -h localhost -p ${TSDB_HOST_PORT:-5433} -U ${TSDB_USER:-mdemg} -d ${TSDB_DATABASE:-mdemg_metrics} <<'SQL' 2>&1`
	// The whole string contains PASSWORD, PORT, USER, DATABASE — all shell
	// env-var references. NONE should be redacted (no literal secret exposed).
	got := ScrubString(input)
	if got != input {
		t.Errorf("live export-auto false-alarm string must be preserved verbatim.\n  in:  %q\n  got: %q", input, got)
	}
}

func TestScrubEnvSecret_MixedRefAndLiteralOnSameLine(t *testing.T) {
	// If a single line has BOTH a reference AND a literal, only the literal
	// is redacted. Common in diagnostic captures ("used $ENV_A, hardcoded X").
	in := "used PASSWORD=$FOO and SECRET=hunter2 in prod"
	want := "used PASSWORD=$FOO and SECRET=[REDACTED] in prod"
	got := ScrubString(in)
	if got != want {
		t.Errorf("mixed-line handling:\n  in:   %q\n  got:  %q\n  want: %q", in, got, want)
	}
}

func TestIsShellEnvVarRef_Cases(t *testing.T) {
	cases := []struct {
		v    string
		want bool
	}{
		// Positive cases (refs — preserve).
		{"$FOO", true},
		{"$foo_bar", true},
		{"$X", true},
		{"${FOO}", true},
		{"${FOO:-default}", true},
		{"${FOO:?required}", true},
		{"%FOO%", true},
		{"%path%", true},
		// Negative cases (real values — redact).
		{"", false},
		{"$", false},               // too short
		{"$!bang", false},          // $ followed by non-identifier
		{"${}", false},             // empty braces
		{"$-flag", false},          // $ followed by hyphen
		{"abc$FOO", false},         // starts with literal char
		{"hunter2", false},         // no leading sentinel at all
		{"sk-test-key", false},     // real API key shape
		{"%malformed", false},      // half-Windows-style
		{"malformed%", false},      // half-Windows-style
		{"%", false},               // too short for Windows
	}
	for _, c := range cases {
		got := isShellEnvVarRef(c.v)
		if got != c.want {
			t.Errorf("isShellEnvVarRef(%q) = %v, want %v", c.v, got, c.want)
		}
	}
}
