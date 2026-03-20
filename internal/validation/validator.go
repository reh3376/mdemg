// Package validation provides request validation using go-playground/validator.
package validation

import (
	"reflect"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"
)

var (
	once     sync.Once
	validate *validator.Validate
)

// GetValidator returns the singleton validator instance with all custom validators registered.
// Thread-safe and initialized only once.
func GetValidator() *validator.Validate {
	once.Do(func() {
		validate = validator.New(validator.WithRequiredStructEnabled())

		// Register custom validators
		_ = validate.RegisterValidation("embedding_dims", validateEmbeddingDims)

		// Use JSON tag names in error messages
		validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
			name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
			if name == "-" {
				return ""
			}
			return name
		})
	})
	return validate
}

// Validate validates a struct and returns an error if validation fails.
// Use FormatValidationErrors from errors.go to format the error for clients.
func Validate(v any) error {
	return GetValidator().Struct(v)
}

// validateEmbeddingDims validates that an embedding slice has valid dimensions.
// Supported: 384 (all-minilm), 768 (nomic-embed-text), 1024 (mxbai-embed-large/snowflake/qwen3:0.6b),
// 1536 (OpenAI small), 2560 (qwen3-embedding:4b native), 3072 (OpenAI text-embedding-3-large / qwen3-embedding:8b truncated),
// 4096 (qwen3-embedding:8b native).
// Returns true for nil slices (let required/omitempty handle nil case).
func validateEmbeddingDims(fl validator.FieldLevel) bool {
	field := fl.Field()

	// Handle nil slice - let required/omitempty validators handle this case
	if field.IsNil() {
		return true
	}

	// Check slice length - must be a known embedding dimension
	length := field.Len()
	return length == 384 || length == 768 || length == 1024 || length == 1536 || length == 2560 || length == 3072 || length == 4096
}
