package languages

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&PHPParser{})
}

// PHPParser implements LanguageParser for PHP source files.
type PHPParser struct{}

func (p *PHPParser) Name() string {
	return "php"
}

func (p *PHPParser) Extensions() []string {
	return []string{".php"}
}

func (p *PHPParser) CanParse(path string) bool {
	return HasExtension(path, p.Extensions())
}

func (p *PHPParser) IsTestFile(path string) bool {
	name := filepath.Base(path)
	inTestDir := strings.Contains(path, "/tests/") || strings.Contains(path, "/test/")
	hasTestSuffix := strings.HasSuffix(name, "Test.php") || strings.HasSuffix(name, "_test.php")
	// Require test directory for suffix-based detection to avoid false positives
	// on production files like app/Models/Test.php (the /app/ check uses slashes
	// on both sides so it won't match paths like /app-testing/)
	return inTestDir || (hasTestSuffix && !strings.Contains(path, "/app/"))
}

func (p *PHPParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	var elements []CodeElement

	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)
	moduleName := strings.TrimSuffix(fileName, ".php")

	// Extract top-level constructs for the summary
	classes := FindAllMatches(content, `(?m)^\s*(?:abstract\s+|final\s+|readonly\s+)*class\s+(\w+)`)
	interfaces := FindAllMatches(content, `(?m)^\s*interface\s+(\w+)`)
	traits := FindAllMatches(content, `(?m)^\s*trait\s+(\w+)`)
	enums := FindAllMatches(content, `(?m)^\s*enum\s+(\w+)`)
	functions := FindAllMatches(content, `(?m)^\s*function\s+(\w+)\s*\(`)
	namespaces := FindAllMatches(content, `(?m)^\s*namespace\s+([\w\\]+)`)

	// Build content for embedding
	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("PHP module: %s\n", moduleName))
	contentBuilder.WriteString(fmt.Sprintf("File: %s\n", relPath))

	if len(namespaces) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Namespace: %s\n", strings.Join(uniqueStrings(namespaces), ", ")))
	}
	if len(classes) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Classes: %s\n", strings.Join(uniqueStrings(classes), ", ")))
	}
	if len(interfaces) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Interfaces: %s\n", strings.Join(uniqueStrings(interfaces), ", ")))
	}
	if len(traits) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Traits: %s\n", strings.Join(uniqueStrings(traits), ", ")))
	}
	if len(enums) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Enums: %s\n", strings.Join(uniqueStrings(enums), ", ")))
	}
	if len(functions) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Functions: %s\n", strings.Join(uniqueStrings(functions), ", ")))
	}

	contentBuilder.WriteString("\n--- Code ---\n")
	truncated, wasTruncated := TruncateContentWithInfo(content, 4000)
	contentBuilder.WriteString(truncated)

	// Detect concerns and build tags
	concerns := DetectConcerns(relPath, content)
	tags := []string{"php", "module"}
	tags = append(tags, concerns...)
	tags = append(tags, detectPHPFrameworkTags(relPath, content)...)

	// Extract symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	// Build diagnostics
	var diagnostics []Diagnostic
	if wasTruncated {
		diagnostics = append(diagnostics, NewDiagnosticWithContext("info", "TRUNCATED", "File content exceeded 4000 character limit", "php", nil))
	}

	elements = append(elements, CodeElement{
		Name:        moduleName,
		Kind:        "module",
		Path:        "/" + relPath,
		Content:     contentBuilder.String(),
		Package:     moduleName,
		FilePath:    relPath,
		Tags:        tags,
		Concerns:    concerns,
		Symbols:     symbols,
		Diagnostics: diagnostics,
	})

	// Add classes, interfaces, traits, and enums as separate elements
	allTypes := append(uniqueStrings(classes), uniqueStrings(interfaces)...)
	allTypes = append(allTypes, uniqueStrings(traits)...)
	allTypes = append(allTypes, uniqueStrings(enums)...)

	for _, typeName := range allTypes {
		kind := detectPHPTypeKind(content, typeName)
		typeContent := extractPHPTypeContent(content, typeName, moduleName)
		elements = append(elements, CodeElement{
			Name:     typeName,
			Kind:     kind,
			Path:     fmt.Sprintf("/%s#%s", relPath, typeName),
			Content:  typeContent,
			Package:  moduleName,
			FilePath: relPath,
			Tags:     append([]string{"php", kind}, concerns...),
			Concerns: concerns,
		})
	}

	return elements, nil
}

func (p *PHPParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	// Regex patterns
	namespacePattern := regexp.MustCompile(`^\s*namespace\s+([\w\\]+)`)
	classPattern := regexp.MustCompile(`^\s*(?:abstract\s+|final\s+|readonly\s+)*class\s+(\w+)`)
	interfacePattern := regexp.MustCompile(`^\s*interface\s+(\w+)`)
	traitPattern := regexp.MustCompile(`^\s*trait\s+(\w+)`)
	enumPattern := regexp.MustCompile(`^\s*enum\s+(\w+)`)
	enumCasePattern := regexp.MustCompile(`^\s*case\s+(\w+)`)
	// Method with visibility modifier (handles abstract/static before or after visibility)
	methodPattern := regexp.MustCompile(`^\s*(?:abstract\s+)?(?:static\s+)?(public|protected|private)\s+(?:static\s+)?(?:abstract\s+)?function\s+&?(\w+)\s*\(`)
	// Bare method without visibility (WordPress/legacy PHP style — defaults to public)
	bareMethodPattern := regexp.MustCompile(`^\s*(?:abstract\s+)?(?:static\s+)?function\s+&?(\w+)\s*\(`)
	// Top-level function — allow indentation for guarded functions: if (! function_exists(...)) { function foo() }
	topFuncPattern := regexp.MustCompile(`^\s*function\s+&?(\w+)\s*\(`)
	// Class constant: optional visibility + const (value captured loosely, handles trailing comments)
	classConstPattern := regexp.MustCompile(`^\s*(public|protected|private)?\s*const\s+(\w+)\s*=\s*(.+?)\s*;`)
	// Multi-line class constant (opens with = [ and no ;)
	classConstMultilinePattern := regexp.MustCompile(`^\s*(public|protected|private)?\s*const\s+(\w+)\s*=\s*\[`)
	// Top-level constant (handles trailing comments)
	topConstPattern := regexp.MustCompile(`^\s*const\s+(\w+)\s*=\s*(.+?)\s*;`)
	// define() constant
	definePattern := regexp.MustCompile(`^\s*define\s*\(\s*['"](\w+)['"]\s*,\s*(.+?)\)\s*;`)
	// Property/field with visibility (handles static before or after visibility)
	fieldPattern := regexp.MustCompile(`^\s*(?:static\s+)?(public|protected|private)\s+(?:static\s+)?(?:readonly\s+)?(?:[\w\\?|]+\s+)?\$(\w+)`)
	// Constructor promoted property — scans parameter content between parens
	promotedPropInParens := regexp.MustCompile(`(?:^|,)\s*(public|protected|private)\s+(?:readonly\s+)?(?:[\w\\?|]+\s+)?\$(\w+)`)
	// Heredoc/nowdoc opener
	heredocPattern := regexp.MustCompile(`<<<\s*'?(\w+)'?\s*$`)

	var currentType string     // current class/interface/trait/enum name
	var currentTypeKind string // "class", "interface", "trait", "enum"
	var braceDepth int         // track brace nesting
	var typeStartDepth int     // brace depth when we entered the type
	var inHeredoc string       // heredoc/nowdoc closing label (empty = not in heredoc)
	var inMultilineConst bool  // inside a multi-line const = [...];
	var inBlockComment bool    // inside a /* ... */ block comment
	var skipUntilLine int      // skip lines consumed by multi-line constructor scan

	for i, line := range lines {
		lineNum := i + 1

		// Skip lines already consumed by multi-line constructor param scan
		// Braces were already counted during the lookahead — do NOT re-count.
		if lineNum <= skipUntilLine {
			continue
		}

		// Track block comments
		if inBlockComment {
			if strings.Contains(line, "*/") {
				inBlockComment = false
			}
			continue
		}
		if strings.Contains(strings.TrimSpace(line), "/*") && !strings.Contains(line, "*/") {
			inBlockComment = true
			// Still count braces on this line (before the comment marker)
			braceDepth += countBracesOutsideStrings(line)
			updateTypeScope(&currentType, &currentTypeKind, &braceDepth, typeStartDepth)
			continue
		}

		// Skip heredoc/nowdoc content
		if inHeredoc != "" {
			if strings.TrimSpace(line) == inHeredoc || strings.TrimSpace(line) == inHeredoc+";" {
				inHeredoc = ""
			}
			continue
		}

		// Detect heredoc/nowdoc opener
		if matches := heredocPattern.FindStringSubmatch(line); matches != nil {
			inHeredoc = matches[1]
			// Still process this line for brace counting below
		}

		// Skip multi-line const array body
		if inMultilineConst {
			if strings.Contains(line, "];") {
				inMultilineConst = false
			}
			continue
		}

		// Count braces using string-aware counting (skips braces inside quotes)
		braceDelta := countBracesOutsideStrings(line)
		braceDepth += braceDelta
		if braceDepth < 0 {
			braceDepth = 0
		}

		// Check if we've exited the current type
		updateTypeScope(&currentType, &currentTypeKind, &braceDepth, typeStartDepth)

		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "#[") {
			continue
		}

		// Constructor handling: emit __construct as method AND extract promoted properties
		if currentType != "" && strings.Contains(trimmed, "function __construct(") {
			// Emit __construct as a method symbol
			constructorVisibility := "public"
			if strings.Contains(trimmed, "private function") || strings.Contains(trimmed, "private static function") {
				constructorVisibility = "private"
			} else if strings.Contains(trimmed, "protected function") || strings.Contains(trimmed, "protected static function") {
				constructorVisibility = "protected"
			}
			symbols = append(symbols, Symbol{
				Name:     "__construct",
				Type:     "method",
				Line:     lineNum,
				Exported: constructorVisibility == "public",
				Parent:   currentType,
				Language: "php",
			})

			// Extract promoted properties from constructor params
			// Collect all parameter text (may span multiple lines)
			paramText := ""
			if idx := strings.Index(line, "__construct("); idx >= 0 {
				paramText = line[idx+len("__construct("):]
			}
			if strings.Contains(paramText, ")") {
				// Single-line constructor — extract params before )
				if closeIdx := strings.Index(paramText, ")"); closeIdx >= 0 {
					paramText = paramText[:closeIdx]
				}
				for _, m := range promotedPropInParens.FindAllStringSubmatch(paramText, -1) {
					symbols = append(symbols, Symbol{
						Name:     strings.TrimPrefix(m[2], "$"),
						Type:     "field",
						Line:     lineNum,
						Exported: m[1] == "public",
						Parent:   currentType,
						Language: "php",
					})
				}
			} else {
				// Multi-line constructor — scan subsequent lines until )
				for _, m := range promotedPropInParens.FindAllStringSubmatch(paramText, -1) {
					symbols = append(symbols, Symbol{
						Name:     strings.TrimPrefix(m[2], "$"),
						Type:     "field",
						Line:     lineNum,
						Exported: m[1] == "public",
						Parent:   currentType,
						Language: "php",
					})
				}
				for j := i + 1; j < len(lines); j++ {
					paramLine := lines[j]
					paramLineNum := j + 1
					// Count braces on param lines (these lines will be skipped by skipUntilLine)
					braceDepth += countBracesOutsideStrings(paramLine)
					if braceDepth < 0 {
						braceDepth = 0
					}

					for _, m := range promotedPropInParens.FindAllStringSubmatch(paramLine, -1) {
						symbols = append(symbols, Symbol{
							Name:     strings.TrimPrefix(m[2], "$"),
							Type:     "field",
							Line:     paramLineNum,
							Exported: m[1] == "public",
							Parent:   currentType,
							Language: "php",
						})
					}
					if strings.Contains(paramLine, ")") {
						skipUntilLine = paramLineNum
						break
					}
				}
			}
			continue
		}

		// Namespace
		if matches := namespacePattern.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:     matches[1],
				Type:     "namespace",
				Line:     lineNum,
				Exported: true,
				Language: "php",
			})
			continue
		}

		// Track whether we're inside a function/method body
		inFunctionBody := (currentType == "" && braceDepth > 0) || (currentType != "" && braceDepth > typeStartDepth+1)

		// Top-level define() constant (skip if inside a function/method body)
		if !inFunctionBody {
			if matches := definePattern.FindStringSubmatch(line); matches != nil {
				symbols = append(symbols, Symbol{
					Name:     matches[1],
					Type:     "constant",
					Value:    CleanValue(matches[2]),
					RawValue: matches[2],
					Line:     lineNum,
					Exported: true,
					Language: "php",
				})
				continue
			}
		}

		// Top-level const
		if currentType == "" {
			if matches := topConstPattern.FindStringSubmatch(line); matches != nil {
				symbols = append(symbols, Symbol{
					Name:     matches[1],
					Type:     "constant",
					Value:    CleanValue(matches[2]),
					RawValue: matches[2],
					Line:     lineNum,
					Exported: true,
					Language: "php",
				})
				continue
			}
		}

		// Class definition
		if matches := classPattern.FindStringSubmatch(line); matches != nil {
			currentType = matches[1]
			currentTypeKind = "class"
			typeStartDepth = braceDepth - braceDelta
			symbols = append(symbols, Symbol{
				Name:     matches[1],
				Type:     "class",
				Line:     lineNum,
				Exported: true,
				Language: "php",
			})
			continue
		}

		// Interface definition
		if matches := interfacePattern.FindStringSubmatch(line); matches != nil {
			currentType = matches[1]
			currentTypeKind = "interface"
			typeStartDepth = braceDepth - braceDelta
			symbols = append(symbols, Symbol{
				Name:     matches[1],
				Type:     "interface",
				Line:     lineNum,
				Exported: true,
				Language: "php",
			})
			continue
		}

		// Trait definition
		if matches := traitPattern.FindStringSubmatch(line); matches != nil {
			currentType = matches[1]
			currentTypeKind = "trait"
			typeStartDepth = braceDepth - braceDelta
			symbols = append(symbols, Symbol{
				Name:     matches[1],
				Type:     "trait",
				Line:     lineNum,
				Exported: true,
				Language: "php",
			})
			continue
		}

		// Enum definition
		if matches := enumPattern.FindStringSubmatch(line); matches != nil {
			currentType = matches[1]
			currentTypeKind = "enum"
			typeStartDepth = braceDepth - braceDelta
			symbols = append(symbols, Symbol{
				Name:     matches[1],
				Type:     "enum",
				Line:     lineNum,
				Exported: true,
				Language: "php",
			})
			continue
		}

		// Enum case (inside enum)
		if currentType != "" && currentTypeKind == "enum" {
			if matches := enumCasePattern.FindStringSubmatch(trimmed); matches != nil {
				symbols = append(symbols, Symbol{
					Name:     matches[1],
					Type:     "enum_value",
					Line:     lineNum,
					Exported: true,
					Parent:   currentType,
					Language: "php",
				})
				continue
			}
		}

		// Class/trait constant (multi-line array form: const NAME = [...])
		if currentType != "" {
			if matches := classConstMultilinePattern.FindStringSubmatch(trimmed); matches != nil {
				if !strings.Contains(trimmed, "];") {
					// Value spans multiple lines — record the constant, skip the array body
					visibility := matches[1]
					if visibility == "" {
						visibility = "public"
					}
					symbols = append(symbols, Symbol{
						Name:     matches[2],
						Type:     "constant",
						Line:     lineNum,
						Exported: visibility == "public",
						Parent:   currentType,
						Language: "php",
					})
					inMultilineConst = true
					continue
				}
			}
		}

		// Class/trait constant (single-line)
		if currentType != "" {
			if matches := classConstPattern.FindStringSubmatch(trimmed); matches != nil {
				visibility := matches[1]
				if visibility == "" {
					visibility = "public"
				}
				symbols = append(symbols, Symbol{
					Name:     matches[2],
					Type:     "constant",
					Value:    CleanValue(matches[3]),
					RawValue: matches[3],
					Line:     lineNum,
					Exported: visibility == "public",
					Parent:   currentType,
					Language: "php",
				})
				continue
			}
		}

		// Field/property (inside class/trait, not in method)
		if currentType != "" && (currentTypeKind == "class" || currentTypeKind == "trait") {
			if matches := fieldPattern.FindStringSubmatch(trimmed); matches != nil {
				// Skip if this looks like a method parameter or local var
				if !strings.Contains(trimmed, "function ") {
					symbols = append(symbols, Symbol{
						Name:     matches[2],
						Type:     "field",
						Line:     lineNum,
						Exported: matches[1] == "public",
						Parent:   currentType,
						Language: "php",
					})
					continue
				}
			}
		}

		// Method with visibility (inside a type)
		if currentType != "" {
			if matches := methodPattern.FindStringSubmatch(trimmed); matches != nil {
				symbols = append(symbols, Symbol{
					Name:     matches[2],
					Type:     "method",
					Line:     lineNum,
					Exported: matches[1] == "public",
					Parent:   currentType,
					Language: "php",
				})
				continue
			}
			// Bare method without visibility (WordPress/legacy style — defaults to public)
			if matches := bareMethodPattern.FindStringSubmatch(trimmed); matches != nil {
				symbols = append(symbols, Symbol{
					Name:     matches[1],
					Type:     "method",
					Line:     lineNum,
					Exported: true, // PHP bare methods default to public
					Parent:   currentType,
					Language: "php",
				})
				continue
			}
		}

		// Top-level function (allow indentation for guarded functions)
		if currentType == "" {
			if matches := topFuncPattern.FindStringSubmatch(line); matches != nil {
				symbols = append(symbols, Symbol{
					Name:     matches[1],
					Type:     "function",
					Line:     lineNum,
					Exported: true,
					Language: "php",
				})
				continue
			}
		}
	}

	return symbols
}

// detectPHPFrameworkTags returns framework-specific tags based on path and content.
func detectPHPFrameworkTags(relPath, content string) []string {
	var tags []string

	// Laravel detection
	if strings.Contains(relPath, "app/Models") ||
		strings.Contains(relPath, "app/Http") ||
		strings.Contains(relPath, "app/Providers") ||
		strings.Contains(relPath, "routes/") ||
		strings.Contains(content, "use Illuminate\\") ||
		strings.Contains(content, "extends Model") ||
		strings.Contains(content, "extends Controller") {
		tags = append(tags, "laravel")
	}

	// WordPress detection
	if strings.Contains(content, "add_action(") ||
		strings.Contains(content, "add_filter(") ||
		strings.Contains(content, "WP_") ||
		strings.Contains(content, "wp_") ||
		strings.Contains(relPath, "wp-content/") {
		tags = append(tags, "wordpress")
	}

	return tags
}

// detectPHPTypeKind determines whether a name is a class, interface, trait, or enum.
// Uses line-based string matching to avoid regex compilation in loops.
func detectPHPTypeKind(content, name string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if matchesTypeKeyword(trimmed, "interface", name) {
			return "interface"
		}
		if matchesTypeKeyword(trimmed, "trait", name) {
			return "trait"
		}
		if matchesTypeKeyword(trimmed, "enum", name) {
			return "enum"
		}
	}
	return "class"
}

// matchesTypeKeyword checks if a trimmed line declares a type with the given keyword and name.
// Handles optional modifiers (abstract, final, readonly) before the keyword.
func matchesTypeKeyword(trimmed, keyword, name string) bool {
	// Strip optional leading modifiers
	for _, mod := range []string{"abstract ", "final ", "readonly "} {
		if strings.HasPrefix(trimmed, mod) {
			trimmed = strings.TrimSpace(trimmed[len(mod):])
		}
	}
	prefix := keyword + " "
	if !strings.HasPrefix(trimmed, prefix) {
		return false
	}
	rest := trimmed[len(prefix):]
	// The name should be the next word
	return strings.HasPrefix(rest, name) &&
		(len(rest) == len(name) || !isWordChar(rest[len(name)]))
}

// isWordChar returns true if c is a letter, digit, or underscore.
func isWordChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

// extractPHPTypeContent extracts the definition block for a class/interface/trait/enum.
func extractPHPTypeContent(content, typeName, moduleName string) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("PHP %s: %s (in %s)\n", detectPHPTypeKind(content, typeName), typeName, moduleName))

	lines := strings.Split(content, "\n")

	inType := false
	braceCount := 0
	for _, line := range lines {
		if !inType {
			trimmed := strings.TrimSpace(line)
			if matchesTypeKeyword(trimmed, "class", typeName) ||
				matchesTypeKeyword(trimmed, "interface", typeName) ||
				matchesTypeKeyword(trimmed, "trait", typeName) ||
				matchesTypeKeyword(trimmed, "enum", typeName) {
				inType = true
				braceCount = 0
			} else {
				continue
			}
		}

		builder.WriteString(line + "\n")
		braceCount += countBracesOutsideStrings(line)
		if inType && braceCount <= 0 && strings.Contains(line, "}") {
			break
		}
	}

	return TruncateContent(builder.String(), 4000)
}

// countBracesOutsideStrings counts { and } braces on a line while skipping
// braces inside single-quoted, double-quoted strings, and inline comments.
// Returns (opens - closes) as a signed delta.
func countBracesOutsideStrings(line string) int {
	delta := 0
	inSingle := false
	inDouble := false
	escaped := false

	for j := 0; j < len(line); j++ {
		ch := line[j]

		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}

		// Skip rest of line on single-line comment (// or #)
		if !inSingle && !inDouble {
			if ch == '/' && j+1 < len(line) && line[j+1] == '/' {
				break
			}
			if ch == '#' && (j+1 >= len(line) || line[j+1] != '[') {
				// PHP # comment (but not #[ attribute)
				break
			}
		}

		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if inSingle || inDouble {
			continue
		}

		switch ch {
		case '{':
			delta++
		case '}':
			delta--
		}
	}
	return delta
}

// updateTypeScope resets currentType/currentTypeKind when braceDepth falls
// to or below the type's starting depth.
func updateTypeScope(currentType, currentTypeKind *string, braceDepth *int, typeStartDepth int) {
	if *currentType != "" && *braceDepth <= typeStartDepth {
		*currentType = ""
		*currentTypeKind = ""
	}
}
