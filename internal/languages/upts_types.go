package languages

import "encoding/json"

// UPTSSpec represents a loaded UPTS (Universal Parser Test Specification) spec file.
type UPTSSpec struct {
	Version  string       `json:"upts_version"`
	Language string       `json:"language"`
	Variants []string     `json:"variants"`
	Metadata UPTSMetadata `json:"metadata"`
	Config   UPTSConfig   `json:"config"`
	Fixture  UPTSFixture  `json:"fixture"`
	Expected UPTSExpected `json:"expected"`
	Patterns []string     `json:"patterns_covered"`
}

// UPTSMetadata holds spec metadata (author, dates, description).
type UPTSMetadata struct {
	Author       string   `json:"author"`
	Created      string   `json:"created"`
	Updated      string   `json:"updated,omitempty"`
	Description  string   `json:"description"`
	ParserStatus string   `json:"parser_status,omitempty"`
	Tags         []string `json:"tags,omitempty"`
}

// UPTSConfig holds test configuration options.
type UPTSConfig struct {
	LineTolerance     int  `json:"line_tolerance"`
	RequireAllSymbols bool `json:"require_all_symbols"`
	AllowExtraSymbols bool `json:"allow_extra_symbols"`
	ValidateSignature bool `json:"validate_signature"`
	ValidateValue     bool `json:"validate_value"`
	ValidateParent    bool `json:"validate_parent"`
	ValidateEvidence  bool `json:"validate_evidence,omitempty"`
}

// UPTSFixture describes the test fixture file.
type UPTSFixture struct {
	Type     string `json:"type"`
	Path     string `json:"path"`
	Content  string `json:"content,omitempty"`
	Filename string `json:"filename,omitempty"`
	SHA256   string `json:"sha256"`
}

// UPTSExpected holds expected parse output.
type UPTSExpected struct {
	SymbolCount json.RawMessage `json:"symbol_count"`
	Symbols     []UPTSSymbol    `json:"symbols"`
	Excluded    []UPTSExcluded  `json:"excluded,omitempty"`
}

// SymbolCountRange returns the min/max expected symbol count.
func (e *UPTSExpected) SymbolCountRange() (min, max int) {
	// Try {min,max} object format
	var rangeObj struct {
		Min int `json:"min"`
		Max int `json:"max"`
	}
	if err := json.Unmarshal(e.SymbolCount, &rangeObj); err == nil && rangeObj.Max > 0 {
		return rangeObj.Min, rangeObj.Max
	}
	// Try plain integer
	var count int
	if err := json.Unmarshal(e.SymbolCount, &count); err == nil {
		return count, count
	}
	return 0, 9999
}

// UPTSSymbol represents an expected symbol in the test spec.
type UPTSSymbol struct {
	Name              string   `json:"name"`
	Type              string   `json:"type"`
	Line              int      `json:"line"`
	LineEnd           int      `json:"line_end,omitempty"`
	Exported          *bool    `json:"exported,omitempty"`
	Parent            string   `json:"parent,omitempty"`
	Signature         string   `json:"signature,omitempty"`
	SignatureContains []string `json:"signature_contains,omitempty"`
	Value             string   `json:"value,omitempty"`
	DocComment        string   `json:"doc_comment,omitempty"`
	Decorators        []string `json:"decorators,omitempty"`
	Pattern           string   `json:"pattern,omitempty"`
	Optional          bool     `json:"optional,omitempty"`
	Tags              []string `json:"tags,omitempty"`
}

// UPTSExcluded represents a symbol that should NOT appear in parser output.
type UPTSExcluded struct {
	Name        string `json:"name,omitempty"`
	NamePattern string `json:"name_pattern,omitempty"`
	Reason      string `json:"reason,omitempty"`
}
