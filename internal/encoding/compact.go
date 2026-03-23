// Package encoding provides generalized compact encoding utilities for AI-to-AI communication.
// Used by J17 protocol and other MDEMG subsystems to reduce token consumption.
package encoding

import (
	"encoding/json"
	"strings"
)

// CompactEncoder defines the interface for compact encoding implementations.
type CompactEncoder interface {
	// Encode transforms text into a compact representation.
	Encode(text string) string
	// Name returns the encoder name.
	Name() string
}

// TelegraphicEncoder removes articles, pronouns, and filler words for compact NL.
type TelegraphicEncoder struct{}

// NewTelegraphicEncoder creates a new telegraphic encoder.
func NewTelegraphicEncoder() *TelegraphicEncoder {
	return &TelegraphicEncoder{}
}

// Name returns the encoder name.
func (e *TelegraphicEncoder) Name() string {
	return "telegraphic"
}

// Encode compresses text by removing articles, pronouns, and filler words.
func (e *TelegraphicEncoder) Encode(text string) string {
	return TelegraphicCompress(text)
}

// stopWords are removed during telegraphic compression.
var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "are": true,
	"was": true, "were": true, "be": true, "been": true, "being": true,
	"have": true, "has": true, "had": true, "do": true, "does": true,
	"did": true, "will": true, "would": true, "could": true, "should": true,
	"may": true, "might": true, "shall": true, "can": true,
	"it": true, "its": true, "this": true, "that": true, "these": true,
	"those": true, "i": true, "you": true, "he": true, "she": true,
	"we": true, "they": true, "me": true, "him": true, "her": true,
	"us": true, "them": true, "my": true, "your": true, "his": true,
	"our": true, "their": true, "myself": true, "yourself": true,
	"of": true, "in": true, "to": true, "for": true, "with": true,
	"on": true, "at": true, "by": true, "from": true, "as": true,
	"into": true, "about": true, "between": true, "through": true,
	"and": true, "but": true, "or": true, "so": true, "yet": true,
	"just": true, "also": true, "very": true, "really": true,
	"quite": true, "rather": true, "somewhat": true,
	"there": true, "here": true, "then": true, "now": true,
	"not": false, // keep "not" — it's semantically critical
}

// TelegraphicCompress removes stop words and compresses whitespace.
func TelegraphicCompress(text string) string {
	words := strings.Fields(text)
	var result []string
	for _, word := range words {
		lower := strings.ToLower(strings.Trim(word, ".,;:!?\"'"))
		if !stopWords[lower] {
			result = append(result, word)
		}
	}
	return strings.Join(result, " ")
}

// AbbreviateNodeID truncates a node ID to the first n characters.
func AbbreviateNodeID(nodeID string, maxLen int) string {
	if len(nodeID) <= maxLen {
		return nodeID
	}
	return nodeID[:maxLen]
}

// CodedEncoder maps full strings to short codes for maximum compression.
type CodedEncoder struct {
	codes map[string]string // full → code
}

// NewCodedEncoder creates a new coded encoder with the given code map.
func NewCodedEncoder(codes map[string]string) *CodedEncoder {
	return &CodedEncoder{codes: codes}
}

// Name returns the encoder name.
func (e *CodedEncoder) Name() string {
	return "coded"
}

// Encode looks up a code for the given text. Returns text unchanged if no code exists.
func (e *CodedEncoder) Encode(text string) string {
	if code, ok := e.codes[text]; ok {
		return code
	}
	return text
}

// CompactJSON marshals v as compact single-line JSON (no indentation).
func CompactJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// TruncateAtWord truncates s to at most maxLen characters on a word boundary,
// appending "..." if truncated.
func TruncateAtWord(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 4 {
		return s[:maxLen]
	}
	// Find the last space before the limit
	cut := maxLen - 3
	idx := strings.LastIndex(s[:cut], " ")
	if idx > 0 {
		cut = idx
	}
	return s[:cut] + "..."
}

// CompressSection applies telegraphic compression then truncates at maxLen.
func CompressSection(s string, maxLen int) string {
	compressed := TelegraphicCompress(s)
	return TruncateAtWord(compressed, maxLen)
}
