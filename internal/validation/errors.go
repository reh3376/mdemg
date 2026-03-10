package validation

import (
	"errors"
	"fmt"

	"github.com/go-playground/validator/v10"
)

// ValidationError represents a single field validation error.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationErrorResponse is the structured error response returned to clients.
type ValidationErrorResponse struct {
	Error   string            `json:"error"`
	Details []ValidationError `json:"details"`
}

// FormatValidationErrors converts validator.ValidationErrors to a client-friendly response.
// If the error is not a ValidationErrors type, returns a generic response.
func FormatValidationErrors(err error) ValidationErrorResponse {
	var validationErrors validator.ValidationErrors
	if errors.As(err, &validationErrors) {
		details := make([]ValidationError, 0, len(validationErrors))
		for _, fe := range validationErrors {
			details = append(details, ValidationError{
				Field:   fe.Field(), // Uses JSON name due to RegisterTagNameFunc
				Message: formatMessage(fe),
			})
		}
		return ValidationErrorResponse{
			Error:   "validation_failed",
			Details: details,
		}
	}
	// Non-validation error (e.g., invalid type)
	return ValidationErrorResponse{
		Error:   "validation_failed",
		Details: nil,
	}
}

// formatMessage generates a human-readable error message for a validation error.
func formatMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "this field is required"
	case "required_without":
		return fmt.Sprintf("this field is required when %s is not provided", fe.Param())
	case "min":
		if fe.Kind().String() == "string" {
			return fmt.Sprintf("must be at least %s characters", fe.Param())
		}
		if fe.Kind().String() == "slice" {
			return fmt.Sprintf("must have at least %s items", fe.Param())
		}
		return fmt.Sprintf("must be at least %s", fe.Param())
	case "max":
		if fe.Kind().String() == "string" {
			return fmt.Sprintf("must be at most %s characters", fe.Param())
		}
		if fe.Kind().String() == "slice" {
			return fmt.Sprintf("must have at most %s items", fe.Param())
		}
		return fmt.Sprintf("must be at most %s", fe.Param())
	case "oneof":
		return fmt.Sprintf("must be one of: %s", fe.Param())
	case "embedding_dims":
		return "must have 384, 768, 1024, 1536, or 3072 dimensions"
	case "dive":
		return "contains invalid items"
	default:
		return fmt.Sprintf("failed on '%s' validation", fe.Tag())
	}
}
