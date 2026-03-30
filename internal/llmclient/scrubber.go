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

// Scrub removes sensitive data from an InteractionRecord before storage.
func Scrub(rec *InteractionRecord) {
	rec.SystemPrompt = scrubText(rec.SystemPrompt)
	rec.UserPrompt = scrubText(rec.UserPrompt)
	rec.Response = scrubText(rec.Response)
	rec.ThinkContent = scrubText(rec.ThinkContent)
}

func scrubText(s string) string {
	if s == "" {
		return s
	}
	s = apiKeyPattern.ReplaceAllString(s, "[REDACTED_KEY]")
	s = absPathPattern.ReplaceAllStringFunc(s, scrubAbsPath)
	s = envSecretPattern.ReplaceAllString(s, "${1}=[REDACTED]")
	s = emailPattern.ReplaceAllString(s, "[EMAIL]")
	s = neo4jCredPattern.ReplaceAllString(s, "neo4j://[REDACTED]@")
	return s
}

// scrubAbsPath replaces /Users/username/path with /[PATH]/last/two/components
func scrubAbsPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 3 {
		return "/[PATH]/" + strings.Join(parts[len(parts)-2:], "/")
	}
	return "[PATH]"
}
