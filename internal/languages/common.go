package languages

import (
	"os"
	"regexp"
	"strings"
)

// Cross-cutting concern patterns for detection
var ConcernPatterns = map[string][]string{
	"authentication": {
		"auth", "login", "logout", "signin", "signout", "session",
		"token", "jwt", "oauth", "msal", "azure-ad", "passport",
	},
	"authorization": {
		"acl", "rbac", "permission", "role", "guard", "policy",
		"access-control", "authorize", "can-activate",
	},
	"error-handling": {
		"error", "exception", "filter", "catch", "handler",
		"fault", "failure", "recovery",
	},
	"validation": {
		"validat", "schema", "dto", "constraint", "sanitize",
		"class-validator", "joi", "zod",
	},
	"logging": {
		"logger", "logging", "log", "audit", "trace", "monitor",
	},
	"caching": {
		"cache", "redis", "memcache", "ttl", "invalidat",
	},
	"temporal": {
		"validfrom", "validto", "valid_from", "valid_to",
		"effectivedate", "effective_date", "expirationdate", "expiration_date",
		"startdate", "start_date", "enddate", "end_date",
		"createdat", "created_at", "updatedat", "updated_at", "deletedat", "deleted_at",
		"softdelete", "soft_delete", "paranoid",
		"temporal", "bitemporal", "versioned",
		"daterange", "date_range", "timerange", "time_range",
		"historiz", "audit_trail", "snapshot",
	},
}

// DetectConcerns analyzes file path and content to detect cross-cutting concerns
func DetectConcerns(filePath, content string) []string {
	var concerns []string
	seen := make(map[string]bool)

	pathLower := strings.ToLower(filePath)
	contentLower := strings.ToLower(content)

	for concern, patterns := range ConcernPatterns {
		for _, pattern := range patterns {
			if strings.Contains(pathLower, pattern) || strings.Contains(contentLower, pattern) {
				if !seen[concern] {
					concerns = append(concerns, concern)
					seen[concern] = true
				}
				break
			}
		}
	}

	return concerns
}

// IsConfigFile checks if a file is a configuration file
func IsConfigFile(path string) bool {
	pathLower := strings.ToLower(path)
	configPatterns := []string{
		"config", "settings", "options", "constants", "types",
		".env", "conf", "yaml", "yml", "json", "toml",
	}
	for _, pattern := range configPatterns {
		if strings.Contains(pathLower, pattern) {
			return true
		}
	}
	return false
}

// ReadFileContent reads file content and returns it as a string
func ReadFileContent(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// TruncateContent truncates content to maxLen, adding a truncation marker
func TruncateContent(content string, maxLen int) string {
	truncated, _ := TruncateContentWithInfo(content, maxLen)
	return truncated
}

// TruncateContentWithInfo truncates content to maxLen and reports whether truncation occurred.
func TruncateContentWithInfo(content string, maxLen int) (string, bool) {
	if len(content) > maxLen {
		return content[:maxLen] + "\n... [truncated]", true
	}
	return content, false
}

// NewDiagnostic creates a new Diagnostic with the given fields.
func NewDiagnostic(severity, code, message, parser string) Diagnostic {
	return Diagnostic{
		Severity: severity,
		Code:     code,
		Message:  message,
		Parser:   parser,
	}
}

// NewDiagnosticWithContext creates a Diagnostic with additional key-value context.
func NewDiagnosticWithContext(severity, code, message, parser string, ctx map[string]string) Diagnostic {
	return Diagnostic{
		Severity: severity,
		Code:     code,
		Message:  message,
		Parser:   parser,
		Context:  ctx,
	}
}

// CleanValue cleans up a value string (removes quotes, trims whitespace)
func CleanValue(value string) string {
	value = strings.TrimSpace(value)
	// Remove trailing comments
	if idx := strings.Index(value, "//"); idx != -1 {
		value = strings.TrimSpace(value[:idx])
	}
	// Remove quotes from string values but preserve the unquoted value
	// Must be at least 2 chars to have matching quotes
	if len(value) >= 2 {
		if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
			(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) ||
			(strings.HasPrefix(value, "`") && strings.HasSuffix(value, "`")) {
			value = value[1 : len(value)-1]
		}
	}
	return value
}

// FindAllMatches finds all matches for a pattern in content and returns unique results
func FindAllMatches(content string, pattern string) []string {
	re := regexp.MustCompile(pattern)
	matches := re.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool)
	var results []string
	for _, match := range matches {
		if len(match) > 1 && !seen[match[1]] {
			results = append(results, match[1])
			seen[match[1]] = true
		}
	}
	return results
}

// HasExtension checks if a file has one of the given extensions
func HasExtension(path string, extensions []string) bool {
	pathLower := strings.ToLower(path)
	for _, ext := range extensions {
		if strings.HasSuffix(pathLower, ext) {
			return true
		}
	}
	return false
}

// UniqueStrings returns a slice with duplicate strings removed, preserving order.
func UniqueStrings(input []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range input {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// uniqueStrings is an alias for UniqueStrings (for package-internal use).
func uniqueStrings(input []string) []string {
	return UniqueStrings(input)
}

// DeriveElementKind maps a Kind (code construct) to an ElementKind (ingestion unit type).
// Kind represents what the code IS (function, class, struct, etc.)
// ElementKind represents how it's INGESTED (symbol, unit, section, etc.)
//
// Mapping:
//   - function, class, struct, interface, enum, trait, method, macro, typedef → "symbol"
//   - module → "unit"
//   - config → "keypath_fact"
//   - doc, markdown → "section"
//   - kernel, device_function → "kernel"
//   - migration → "migration"
//   - file → "file"
//   - everything else → "other"
func DeriveElementKind(kind string) string {
	switch strings.ToLower(kind) {
	case "function", "class", "struct", "interface", "enum", "trait", "method", "macro", "typedef", "constant", "variable":
		return "symbol"
	case "module", "package", "namespace":
		return "unit"
	case "config", "env", "settings":
		return "keypath_fact"
	case "doc", "markdown", "readme", "documentation":
		return "section"
	case "kernel", "device_function":
		return "kernel"
	case "migration", "schema":
		return "migration"
	case "file":
		return "file"
	case "snippet":
		return "snippet"
	default:
		return "other"
	}
}
