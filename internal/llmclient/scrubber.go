package llmclient

import (
	"regexp"
	"strings"
)

var (
	// API keys: sk-*, ghp_*, AKIA*, Bearer tokens
	apiKeyPattern = regexp.MustCompile(`(?i)(sk-[a-zA-Z0-9]{20,}|ghp_[a-zA-Z0-9]{36}|AKIA[A-Z0-9]{16}|Bearer\s+[a-zA-Z0-9._\-]{20,})`)

	// Absolute paths: /Users/*, /home/*, C:\Users\*
	absPathPattern = regexp.MustCompile(`(?:/Users/[^\s"']+|/home/[^\s"']+|[A-Z]:\\\\Users\\\\[^\s"']+)`)

	// Environment secrets: VAR=value patterns for known sensitive vars.
	//
	// Value class excludes ) and ] on top of the original whitespace/quote
	// /comma/semicolon terminators. Rationale (SCRUB-IDEMPOTENT-001,
	// 2026-08-13): with the older `[^\s"',;]+` class, intake-scrubbing
	// `api_key = os.getenv("KEY")` produced `api_key=[REDACTED]KEY")`,
	// which a second scrub pass rewrote to `api_key=[REDACTED])`, and a
	// third to `api_key=[REDACTED]`. Intake runs ONE pass, so the export
	// scan-gate always saw a diff and blocked export-auto in a loop.
	// Excluding `)` and `]` (paired with the `isRedactedPlaceholder`
	// preserve branch in scrubEnvSecret) makes the regex reach a fixed
	// point after one pass. `{` `}` `<` `>` remain in the value class so
	// SCRUB-ENV-REF-001's `${VAR}` / `${VAR:-default}` shell-ref
	// preservation continues to work — narrowing further breaks those.
	// Real secrets rarely contain ) or ] so the truncation risk is low.
	envSecretPattern = regexp.MustCompile(`(?i)(PASSWORD|SECRET|TOKEN|API_KEY|PRIVATE_KEY)\s*[=:]\s*["']?([^\s"',;)\]]+)["']?`)

	// Email addresses
	emailPattern = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)

	// Neo4j credentials in connection strings
	neo4jCredPattern = regexp.MustCompile(`neo4j://[^:]+:[^@]+@`)
)

// patternEntry pairs a named privacy pattern with its replacement logic.
type patternEntry struct {
	name    string
	regex   *regexp.Regexp
	replace func(string) string
}

// patterns is the ordered registry of all privacy scrub patterns.
// Each entry captures its own replacement strategy (simple string vs custom func).
var patterns = []patternEntry{
	{"api_key", apiKeyPattern, func(s string) string { return apiKeyPattern.ReplaceAllString(s, "[REDACTED_KEY]") }},
	{"abs_path", absPathPattern, func(s string) string { return absPathPattern.ReplaceAllStringFunc(s, scrubAbsPath) }},
	{"env_secret", envSecretPattern, scrubEnvSecret},
	{"email", emailPattern, func(s string) string { return emailPattern.ReplaceAllString(s, "[EMAIL]") }},
	{"neo4j_cred", neo4jCredPattern, func(s string) string { return neo4jCredPattern.ReplaceAllString(s, "neo4j://[REDACTED]@") }},
}

// ScrubString removes sensitive data from a single string.
func ScrubString(s string) string {
	return ScrubStringExcluding(s, nil)
}

// ScrubStringExcluding removes sensitive data but skips named patterns in skip.
// Pass nil or empty skip to apply all patterns (equivalent to ScrubString).
func ScrubStringExcluding(s string, skip []string) string {
	if s == "" {
		return s
	}
	skipSet := make(map[string]bool, len(skip))
	for _, name := range skip {
		skipSet[name] = true
	}
	for _, p := range patterns {
		if skipSet[p.name] {
			continue
		}
		s = p.replace(s)
	}
	return s
}

// Scrub removes sensitive data from an InteractionRecord before storage.
func Scrub(rec *InteractionRecord) {
	rec.SystemPrompt = ScrubStringExcluding(rec.SystemPrompt, nil)
	rec.UserPrompt = ScrubStringExcluding(rec.UserPrompt, nil)
	rec.Response = ScrubStringExcluding(rec.Response, nil)
	rec.ThinkContent = ScrubStringExcluding(rec.ThinkContent, nil)
}

// scrubEnvSecret redacts env-secret matches, but PRESERVES matches whose
// value is a shell env-var REFERENCE (e.g. `$FOO`, `${FOO}`, `${FOO:-x}`,
// Windows `%FOO%`) rather than a literal secret. The reference doesn't
// carry the secret — it's a POINTER the shell will expand at runtime.
// Redacting these was the SCRUB-ENV-REF-001 false-positive class (2026-08-04):
// captured `bash error:` telemetry containing `PGPASSWORD=$TSDB_PASSWORD`
// blocked export-auto with a spurious "PII detected" halt.
//
// Kept as a top-level function (not a closure) so tests can call it directly.
func scrubEnvSecret(s string) string {
	return envSecretPattern.ReplaceAllStringFunc(s, func(match string) string {
		sub := envSecretPattern.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		name, value := sub[1], sub[2]
		if isShellEnvVarRef(value) {
			return match // preserve — the value is a reference, not a secret
		}
		if isRedactedPlaceholder(value) {
			return match // preserve — value is ALREADY a scrub placeholder (idempotency)
		}
		return name + "=[REDACTED]"
	})
}

// isRedactedPlaceholder reports whether the captured value is already one
// of the scrub replacement placeholders. Defense-in-depth against
// idempotency drift when the regex value-class change alone doesn't fully
// close the loop (SCRUB-IDEMPOTENT-001, 2026-08-13). Preserving these
// prevents re-scrubbing from consuming trailing characters that weren't
// part of the original secret.
func isRedactedPlaceholder(v string) bool {
	return strings.HasPrefix(v, "[REDACTED")
}

// isShellEnvVarRef reports whether a captured "value" is a shell env-var
// reference rather than a literal secret. The check is intentionally
// conservative: it recognises only well-formed references so a real secret
// that happens to start with `$` is still redacted (nothing legitimate
// starts with a bare `$` followed by non-identifier characters).
func isShellEnvVarRef(v string) bool {
	if len(v) < 2 {
		return false
	}
	// Windows %FOO% — begins AND ends with %.
	if v[0] == '%' && v[len(v)-1] == '%' && len(v) >= 3 {
		return true
	}
	// Bash ${FOO}, ${FOO:-default}, ${FOO:?err} — anything inside braces
	// that starts with an identifier char.
	if strings.HasPrefix(v, "${") && strings.HasSuffix(v, "}") && len(v) >= 4 {
		inner := v[2 : len(v)-1]
		return len(inner) > 0 && isIdentifierChar(inner[0])
	}
	// Bash $FOO — dollar followed by identifier chars until end (or a shell
	// separator, but the outer regex value-class already stops at spaces /
	// quotes / commas / semicolons, so we just need to verify the leading
	// chars form an identifier).
	if v[0] == '$' && isIdentifierChar(v[1]) {
		return true
	}
	return false
}

// isIdentifierChar matches [A-Za-z_] (identifier-lead) or a digit (allowed
// in identifier tails). Kept simple — the outer regex already bounds the
// value's overall shape.
func isIdentifierChar(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || b == '_' || (b >= '0' && b <= '9')
}

// scrubAbsPath replaces /Users/username/path with /[PATH]/last/two/components
func scrubAbsPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 3 {
		return "/[PATH]/" + strings.Join(parts[len(parts)-2:], "/")
	}
	return "[PATH]"
}
