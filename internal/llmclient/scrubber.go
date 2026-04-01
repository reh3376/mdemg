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

	// Environment secrets: VAR=value patterns for known sensitive vars
	envSecretPattern = regexp.MustCompile(`(?i)(PASSWORD|SECRET|TOKEN|API_KEY|PRIVATE_KEY)\s*[=:]\s*["']?([^\s"',;]+)["']?`)

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
	{"env_secret", envSecretPattern, func(s string) string { return envSecretPattern.ReplaceAllString(s, "${1}=[REDACTED]") }},
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

// scrubAbsPath replaces /Users/username/path with /[PATH]/last/two/components
func scrubAbsPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 3 {
		return "/[PATH]/" + strings.Join(parts[len(parts)-2:], "/")
	}
	return "[PATH]"
}
