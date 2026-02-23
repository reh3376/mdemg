// Package languages provides a modular framework for parsing different programming languages.
// Each language is implemented as a separate parser that implements the LanguageParser interface.
package languages

// CodeElement represents a parsed code element (function, class, struct, etc.)
type CodeElement struct {
	// Existing fields (v1 - preserved for backward compatibility)
	Name     string
	Kind     string   // Code construct: "function", "class", "struct", "interface", "enum", "trait", "module", "kernel"
	Path     string   // Relative path to file
	Content  string   // Source code content
	Summary  string   // Brief summary from docstrings/comments
	Package  string   // Package/module name
	FilePath string   // Full file path
	Tags     []string // Language, file type, etc.
	Concerns []string // Cross-cutting concerns detected
	Symbols  []Symbol // Extracted code symbols

	// New fields (v2 - evidence and stability)
	ElementKind string // Ingestion unit type: "file", "symbol", "section", "keypath_fact", "unit", "snippet", "migration", "kernel", "other"
	StartLine   int    // First line of element in source file (1-indexed, 0 = not set)
	EndLine     int    // Last line of element in source file (1-indexed, 0 = not set)
	StableID    string // Deterministic ID: hash(space_id + path + element_kind + qualname + start_line + end_line)
	Signature   string // Human-readable signature (e.g., "func ParseFile(root, path string) ([]CodeElement, error)")

	// Diagnostics (v2.1 - structured parser diagnostics)
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"` // Structured diagnostics
}

// Diagnostic represents a structured parser diagnostic.
//
// Diagnostic codes:
//   - TRUNCATED: content was truncated by TruncateContent (severity: info)
//   - LARGE_FILE: file exceeds size threshold (severity: warning)
//   - PARTIAL_PARSE: parser skipped constructs it can't handle (severity: warning)
//   - BINARY_DETECTED: non-text content detected (severity: info)
type Diagnostic struct {
	Severity string            `json:"severity"`           // "info", "warning", "error"
	Code     string            `json:"code"`               // stable machine-readable code
	Message  string            `json:"message"`            // human-readable description
	Parser   string            `json:"parser,omitempty"`   // parser that emitted this
	Context  map[string]string `json:"context,omitempty"`  // additional key-value context
}

// DiagnosticSummary collects diagnostic counts by severity and code.
type DiagnosticSummary struct {
	Total    int
	BySev    map[string]int // severity → count
	ByCode   map[string]int // code → count
}

// NewDiagnosticSummary creates an empty DiagnosticSummary.
func NewDiagnosticSummary() *DiagnosticSummary {
	return &DiagnosticSummary{
		BySev:  make(map[string]int),
		ByCode: make(map[string]int),
	}
}

// Add records a diagnostic into the summary.
func (ds *DiagnosticSummary) Add(d Diagnostic) {
	ds.Total++
	ds.BySev[d.Severity]++
	ds.ByCode[d.Code]++
}

// AddAll records all diagnostics from a slice of CodeElements.
func (ds *DiagnosticSummary) AddAll(elements []CodeElement) {
	for _, elem := range elements {
		for _, d := range elem.Diagnostics {
			ds.Add(d)
		}
	}
}

// Symbol represents an extracted code symbol (constant, function signature, etc.)
// Field names follow UPTS (Universal Parser Test Specification) v1.0.0
type Symbol struct {
	Name           string `json:"name"`
	Type           string `json:"type"` // "constant", "function", "class", "interface", "variable", "struct", "enum", "method", "macro", "kernel", "trait", "field"
	Line           int    `json:"line"`                      // 1-indexed line number (UPTS standard)
	LineEnd        int    `json:"line_end,omitempty"`        // End line for multi-line symbols
	Exported       bool   `json:"exported"`                  // Public visibility
	Parent         string `json:"parent,omitempty"`          // Parent class/struct/module for methods
	Signature      string `json:"signature,omitempty"`       // Full signature
	Value          string `json:"value,omitempty"`           // Constant value
	RawValue       string `json:"raw_value,omitempty"`       // Original source text
	DocComment     string `json:"doc_comment,omitempty"`     // Documentation/decorators
	TypeAnnotation string `json:"type_annotation,omitempty"` // Type annotation
	Language       string `json:"language,omitempty"`        // Source language
}

// LanguageParser defines the interface that all language parsers must implement.
type LanguageParser interface {
	// Name returns the language name (e.g., "go", "rust", "python")
	Name() string

	// Extensions returns file extensions this parser handles (e.g., [".go"], [".rs"])
	Extensions() []string

	// CanParse returns true if this parser can handle the given file path
	CanParse(path string) bool

	// ParseFile parses a source file and returns code elements
	ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error)

	// IsTestFile returns true if the file is a test file
	IsTestFile(path string) bool
}

// ParserConfig holds configuration options for language parsers
type ParserConfig struct {
	ExtractSymbols bool     // Whether to extract detailed symbols
	IncludeTests   bool     // Whether to include test files
	ExcludeDirs    []string // Directories to exclude
}

// BuildContext holds cross-file build metadata gathered during ingestion.
// This is populated by BuildContextParser implementations and used by
// ContextAwareParser implementations to enhance parsing with build info.
type BuildContext struct {
	CompilerFlags map[string][]string // path pattern → flags (e.g., "*.cu" → ["-arch=sm_80"])
	IncludePaths  []string            // Global include paths from build system
	Defines       map[string]string   // Preprocessor defines (e.g., "CUDA_VERSION" → "11.0")
	BuildSystem   string              // Detected build system: "cmake", "make", "bazel", "cargo", etc.
	SourceRoot    string              // Root path for resolving includes
}

// BuildContextParser extracts build context from build configuration files.
// Examples: CMakeLists.txt, Makefile, Cargo.toml, pyproject.toml
type BuildContextParser interface {
	// CanParseBuildFile returns true if this parser can extract build context from the file
	CanParseBuildFile(path string) bool

	// ParseBuildFile extracts build context from a build configuration file
	ParseBuildFile(root, path string) (*BuildContext, error)
}

// ContextAwareParser extends LanguageParser with build context support.
// Parsers that benefit from build metadata (like CUDA, C/C++) can implement this.
type ContextAwareParser interface {
	LanguageParser

	// ParseFileWithContext parses a source file with additional build context
	ParseFileWithContext(root, path string, extractSymbols bool, ctx *BuildContext) ([]CodeElement, error)
}

// registry holds all registered language parsers
var registry = make(map[string]LanguageParser)

// Register adds a language parser to the registry
func Register(parser LanguageParser) {
	registry[parser.Name()] = parser
}

// GetParser returns a parser by language name
func GetParser(name string) (LanguageParser, bool) {
	p, ok := registry[name]
	return p, ok
}

// GetParserForFile returns the appropriate parser for a file based on extension
func GetParserForFile(path string) (LanguageParser, bool) {
	for _, parser := range registry {
		if parser.CanParse(path) {
			return parser, true
		}
	}
	return nil, false
}

// AllParsers returns all registered parsers
func AllParsers() []LanguageParser {
	parsers := make([]LanguageParser, 0, len(registry))
	for _, p := range registry {
		parsers = append(parsers, p)
	}
	return parsers
}

// SupportedExtensions returns all file extensions supported by registered parsers
func SupportedExtensions() []string {
	var extensions []string
	for _, parser := range registry {
		extensions = append(extensions, parser.Extensions()...)
	}
	return extensions
}
