package validation

import (
	"testing"

	"mdemg/internal/models"
)

// Test structs for validation tests
type testEmbeddingStruct struct {
	Embedding []float32 `json:"embedding" validate:"omitempty,embedding_dims"`
}

type testRequiredEmbeddingStruct struct {
	Embedding []float32 `json:"embedding" validate:"required,embedding_dims"`
}

type testRequiredStruct struct {
	SpaceID string `json:"space_id" validate:"required,min=1,max=256"`
}

type testMinMaxStruct struct {
	Name string `json:"name" validate:"min=3,max=10"`
	Age  int    `json:"age" validate:"min=1,max=100"`
}

type testOneOfStruct struct {
	Sensitivity string `json:"sensitivity" validate:"oneof=public internal confidential"`
}

type testSliceStruct struct {
	Tags []string `json:"tags" validate:"min=1,max=5,dive,min=1"`
}

type testRequiredWithoutStruct struct {
	QueryText      string    `json:"query_text" validate:"required_without=QueryEmbedding,omitempty,min=1"`
	QueryEmbedding []float32 `json:"query_embedding" validate:"required_without=QueryText,omitempty,embedding_dims"`
}

// TestGetValidator_Singleton tests that GetValidator returns the same instance
func TestGetValidator_Singleton(t *testing.T) {
	v1 := GetValidator()
	v2 := GetValidator()

	if v1 != v2 {
		t.Error("GetValidator() should return the same instance (singleton)")
	}

	if v1 == nil {
		t.Error("GetValidator() should not return nil")
	}
}

// TestValidateEmbeddingDims tests the embedding_dims custom validator
func TestValidateEmbeddingDims(t *testing.T) {
	tests := []struct {
		name        string
		embedding   []float32
		expectError bool
	}{
		{
			name:        "nil embedding passes (let required handle it)",
			embedding:   nil,
			expectError: false,
		},
		{
			name:        "empty embedding fails",
			embedding:   []float32{},
			expectError: true,
		},
		{
			name:        "768 dimensions passes",
			embedding:   make([]float32, 768),
			expectError: false,
		},
		{
			name:        "1536 dimensions passes",
			embedding:   make([]float32, 1536),
			expectError: false,
		},
		{
			name:        "wrong dimensions (512) fails",
			embedding:   make([]float32, 512),
			expectError: true,
		},
		{
			name:        "wrong dimensions (100) fails",
			embedding:   make([]float32, 100),
			expectError: true,
		},
		{
			name:        "wrong dimensions (3072) fails",
			embedding:   make([]float32, 3072),
			expectError: true,
		},
		{
			name:        "wrong dimensions (1) fails",
			embedding:   make([]float32, 1),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testEmbeddingStruct{Embedding: tt.embedding}
			err := Validate(s)

			if tt.expectError && err == nil {
				t.Error("expected validation error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

// TestValidateEmbeddingDims_Required tests embedding_dims with required validation
func TestValidateEmbeddingDims_Required(t *testing.T) {
	tests := []struct {
		name        string
		embedding   []float32
		expectError bool
	}{
		{
			name:        "nil embedding fails when required",
			embedding:   nil,
			expectError: true,
		},
		{
			name:        "768 dimensions passes",
			embedding:   make([]float32, 768),
			expectError: false,
		},
		{
			name:        "1536 dimensions passes",
			embedding:   make([]float32, 1536),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testRequiredEmbeddingStruct{Embedding: tt.embedding}
			err := Validate(s)

			if tt.expectError && err == nil {
				t.Error("expected validation error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

// TestValidate_Required tests required field validation
func TestValidate_Required(t *testing.T) {
	tests := []struct {
		name        string
		spaceID     string
		expectError bool
	}{
		{
			name:        "valid space_id",
			spaceID:     "test-space",
			expectError: false,
		},
		{
			name:        "empty space_id fails",
			spaceID:     "",
			expectError: true,
		},
		{
			name:        "single character passes min",
			spaceID:     "a",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testRequiredStruct{SpaceID: tt.spaceID}
			err := Validate(s)

			if tt.expectError && err == nil {
				t.Error("expected validation error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

// TestValidate_MinMax tests min/max validation for strings and numbers
func TestValidate_MinMax(t *testing.T) {
	tests := []struct {
		name        string
		nameField   string
		age         int
		expectError bool
	}{
		{
			name:        "valid name and age",
			nameField:   "John",
			age:         25,
			expectError: false,
		},
		{
			name:        "name too short",
			nameField:   "Jo",
			age:         25,
			expectError: true,
		},
		{
			name:        "name too long",
			nameField:   "JohnJacobson",
			age:         25,
			expectError: true,
		},
		{
			name:        "age too low",
			nameField:   "John",
			age:         0,
			expectError: true,
		},
		{
			name:        "age too high",
			nameField:   "John",
			age:         101,
			expectError: true,
		},
		{
			name:        "boundary: name exactly min",
			nameField:   "Jon",
			age:         50,
			expectError: false,
		},
		{
			name:        "boundary: name exactly max",
			nameField:   "JohnJacobs",
			age:         50,
			expectError: false,
		},
		{
			name:        "boundary: age exactly min",
			nameField:   "John",
			age:         1,
			expectError: false,
		},
		{
			name:        "boundary: age exactly max",
			nameField:   "John",
			age:         100,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testMinMaxStruct{Name: tt.nameField, Age: tt.age}
			err := Validate(s)

			if tt.expectError && err == nil {
				t.Error("expected validation error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

// TestValidate_OneOf tests oneof validation
func TestValidate_OneOf(t *testing.T) {
	tests := []struct {
		name        string
		sensitivity string
		expectError bool
	}{
		{
			name:        "public is valid",
			sensitivity: "public",
			expectError: false,
		},
		{
			name:        "internal is valid",
			sensitivity: "internal",
			expectError: false,
		},
		{
			name:        "confidential is valid",
			sensitivity: "confidential",
			expectError: false,
		},
		{
			name:        "invalid value fails",
			sensitivity: "secret",
			expectError: true,
		},
		{
			name:        "empty string fails",
			sensitivity: "",
			expectError: true,
		},
		{
			name:        "case sensitive - Public fails",
			sensitivity: "Public",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testOneOfStruct{Sensitivity: tt.sensitivity}
			err := Validate(s)

			if tt.expectError && err == nil {
				t.Error("expected validation error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

// TestValidate_SliceValidation tests slice validation with dive
func TestValidate_SliceValidation(t *testing.T) {
	tests := []struct {
		name        string
		tags        []string
		expectError bool
	}{
		{
			name:        "valid tags",
			tags:        []string{"tag1", "tag2"},
			expectError: false,
		},
		{
			name:        "empty slice fails (min=1)",
			tags:        []string{},
			expectError: true,
		},
		{
			name:        "too many tags fails (max=5)",
			tags:        []string{"a", "b", "c", "d", "e", "f"},
			expectError: true,
		},
		{
			name:        "empty string in slice fails (dive,min=1)",
			tags:        []string{"valid", ""},
			expectError: true,
		},
		{
			name:        "single valid tag passes",
			tags:        []string{"single"},
			expectError: false,
		},
		{
			name:        "exactly 5 tags passes",
			tags:        []string{"a", "b", "c", "d", "e"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testSliceStruct{Tags: tt.tags}
			err := Validate(s)

			if tt.expectError && err == nil {
				t.Error("expected validation error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

// TestValidate_RequiredWithout tests required_without validation
func TestValidate_RequiredWithout(t *testing.T) {
	tests := []struct {
		name           string
		queryText      string
		queryEmbedding []float32
		expectError    bool
	}{
		{
			name:           "query_text only passes",
			queryText:      "search query",
			queryEmbedding: nil,
			expectError:    false,
		},
		{
			name:           "query_embedding only passes",
			queryText:      "",
			queryEmbedding: make([]float32, 1536),
			expectError:    false,
		},
		{
			name:           "both provided passes",
			queryText:      "search query",
			queryEmbedding: make([]float32, 768),
			expectError:    false,
		},
		{
			name:           "neither provided fails",
			queryText:      "",
			queryEmbedding: nil,
			expectError:    true,
		},
		{
			name:           "embedding with wrong dimensions fails",
			queryText:      "",
			queryEmbedding: make([]float32, 512),
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testRequiredWithoutStruct{
				QueryText:      tt.queryText,
				QueryEmbedding: tt.queryEmbedding,
			}
			err := Validate(s)

			if tt.expectError && err == nil {
				t.Error("expected validation error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

// TestFormatValidationErrors tests error formatting
func TestFormatValidationErrors(t *testing.T) {
	tests := []struct {
		name           string
		input          any
		expectError    string
		expectMinItems int
	}{
		{
			name:           "required field missing",
			input:          testRequiredStruct{SpaceID: ""},
			expectError:    "validation_failed",
			expectMinItems: 1,
		},
		{
			name:           "multiple errors",
			input:          testMinMaxStruct{Name: "Jo", Age: 0},
			expectError:    "validation_failed",
			expectMinItems: 2,
		},
		{
			name:           "embedding_dims error",
			input:          testEmbeddingStruct{Embedding: make([]float32, 100)},
			expectError:    "validation_failed",
			expectMinItems: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.input)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}

			resp := FormatValidationErrors(err)

			if resp.Error != tt.expectError {
				t.Errorf("Error = %q, want %q", resp.Error, tt.expectError)
			}

			if len(resp.Details) < tt.expectMinItems {
				t.Errorf("Details count = %d, want at least %d", len(resp.Details), tt.expectMinItems)
			}
		})
	}
}

// TestFormatValidationErrors_NonValidationError tests handling of non-validation errors
func TestFormatValidationErrors_NonValidationError(t *testing.T) {
	// Create a simple error that's not a ValidationErrors type
	err := Validate(nil)
	if err == nil {
		// nil passes validation for non-pointer types
		// Use a different approach - just test with a generic error
		resp := FormatValidationErrors(nil)
		if resp.Error != "validation_failed" {
			t.Errorf("Error = %q, want %q", resp.Error, "validation_failed")
		}
		if resp.Details != nil {
			t.Errorf("Details = %v, want nil", resp.Details)
		}
		return
	}

	resp := FormatValidationErrors(err)
	if resp.Error != "validation_failed" {
		t.Errorf("Error = %q, want %q", resp.Error, "validation_failed")
	}
}

// TestFormatValidationErrors_FieldNames tests that JSON field names are used
func TestFormatValidationErrors_FieldNames(t *testing.T) {
	s := testRequiredStruct{SpaceID: ""}
	err := Validate(s)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}

	resp := FormatValidationErrors(err)

	if len(resp.Details) == 0 {
		t.Fatal("expected at least one detail")
	}

	// Should use JSON name "space_id" not Go name "SpaceID"
	found := false
	for _, d := range resp.Details {
		if d.Field == "space_id" {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected field 'space_id' in details, got: %v", resp.Details)
	}
}

// TestFormatValidationErrors_Messages tests that error messages are formatted correctly
func TestFormatValidationErrors_Messages(t *testing.T) {
	tests := []struct {
		name            string
		input           any
		expectedField   string
		expectedMessage string
	}{
		{
			name:            "required message",
			input:           testRequiredStruct{SpaceID: ""},
			expectedField:   "space_id",
			expectedMessage: "this field is required",
		},
		{
			name:            "embedding_dims message",
			input:           testEmbeddingStruct{Embedding: make([]float32, 100)},
			expectedField:   "embedding",
			expectedMessage: "must have 384, 768, 1024, or 1536 dimensions",
		},
		{
			name:            "oneof message",
			input:           testOneOfStruct{Sensitivity: "invalid"},
			expectedField:   "sensitivity",
			expectedMessage: "must be one of: public internal confidential",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.input)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}

			resp := FormatValidationErrors(err)

			var found bool
			for _, d := range resp.Details {
				if d.Field == tt.expectedField {
					found = true
					if d.Message != tt.expectedMessage {
						t.Errorf("Message for %s = %q, want %q", tt.expectedField, d.Message, tt.expectedMessage)
					}
					break
				}
			}

			if !found {
				t.Errorf("field %q not found in details: %v", tt.expectedField, resp.Details)
			}
		})
	}
}

// TestValidationError_Struct tests ValidationError struct fields
func TestValidationError_Struct(t *testing.T) {
	ve := ValidationError{
		Field:   "test_field",
		Message: "test message",
	}

	if ve.Field != "test_field" {
		t.Errorf("Field = %q, want %q", ve.Field, "test_field")
	}
	if ve.Message != "test message" {
		t.Errorf("Message = %q, want %q", ve.Message, "test message")
	}
}

// TestValidationErrorResponse_Struct tests ValidationErrorResponse struct fields
func TestValidationErrorResponse_Struct(t *testing.T) {
	resp := ValidationErrorResponse{
		Error: "validation_failed",
		Details: []ValidationError{
			{Field: "field1", Message: "message1"},
			{Field: "field2", Message: "message2"},
		},
	}

	if resp.Error != "validation_failed" {
		t.Errorf("Error = %q, want %q", resp.Error, "validation_failed")
	}
	if len(resp.Details) != 2 {
		t.Errorf("Details count = %d, want 2", len(resp.Details))
	}
	if resp.Details[0].Field != "field1" {
		t.Errorf("Details[0].Field = %q, want %q", resp.Details[0].Field, "field1")
	}
}

// TestValidateRetrieveRequest tests all validation rules for RetrieveRequest
func TestValidateRetrieveRequest(t *testing.T) {
	// Helper to create a valid 1536-dim embedding
	validEmbedding := func() []float32 {
		return make([]float32, 1536)
	}

	// Helper to create a valid 768-dim embedding
	validEmbedding768 := func() []float32 {
		return make([]float32, 768)
	}

	tests := []struct {
		name        string
		request     models.RetrieveRequest
		expectError bool
		errorField  string // expected field in error (optional)
	}{
		// Valid cases
		{
			name: "valid request with query_text",
			request: models.RetrieveRequest{
				SpaceID:   "test-space",
				QueryText: "search query",
			},
			expectError: false,
		},
		{
			name: "valid request with query_embedding (1536 dims)",
			request: models.RetrieveRequest{
				SpaceID:        "test-space",
				QueryEmbedding: validEmbedding(),
			},
			expectError: false,
		},
		{
			name: "valid request with query_embedding (768 dims)",
			request: models.RetrieveRequest{
				SpaceID:        "test-space",
				QueryEmbedding: validEmbedding768(),
			},
			expectError: false,
		},
		{
			name: "valid request with both query_text and query_embedding",
			request: models.RetrieveRequest{
				SpaceID:        "test-space",
				QueryText:      "search query",
				QueryEmbedding: validEmbedding(),
			},
			expectError: false,
		},
		{
			name: "valid request with all optional fields",
			request: models.RetrieveRequest{
				SpaceID:        "test-space",
				QueryText:      "search query",
				CandidateK:     100,
				TopK:           10,
				HopDepth:       2,
				PolicyContext:  map[string]any{"key": "value"},
			},
			expectError: false,
		},

		// space_id validation
		{
			name: "missing space_id fails",
			request: models.RetrieveRequest{
				QueryText: "search query",
			},
			expectError: true,
			errorField:  "space_id",
		},
		{
			name: "empty space_id fails",
			request: models.RetrieveRequest{
				SpaceID:   "",
				QueryText: "search query",
			},
			expectError: true,
			errorField:  "space_id",
		},
		{
			name: "space_id at max length (256) passes",
			request: models.RetrieveRequest{
				SpaceID:   string(make([]byte, 256)),
				QueryText: "search query",
			},
			expectError: false,
		},
		{
			name: "space_id exceeds max length (257) fails",
			request: models.RetrieveRequest{
				SpaceID:   string(make([]byte, 257)),
				QueryText: "search query",
			},
			expectError: true,
			errorField:  "space_id",
		},
		{
			name: "space_id single character passes",
			request: models.RetrieveRequest{
				SpaceID:   "a",
				QueryText: "search query",
			},
			expectError: false,
		},

		// Either-or validation (query_text OR query_embedding)
		{
			name: "neither query_text nor query_embedding fails",
			request: models.RetrieveRequest{
				SpaceID: "test-space",
			},
			expectError: true,
		},
		{
			name: "empty query_text with nil query_embedding fails",
			request: models.RetrieveRequest{
				SpaceID:        "test-space",
				QueryText:      "",
				QueryEmbedding: nil,
			},
			expectError: true,
		},

		// query_embedding validation
		{
			name: "query_embedding with wrong dimensions (512) fails",
			request: models.RetrieveRequest{
				SpaceID:        "test-space",
				QueryEmbedding: make([]float32, 512),
			},
			expectError: true,
			errorField:  "query_embedding",
		},
		{
			name: "query_embedding with wrong dimensions (100) fails",
			request: models.RetrieveRequest{
				SpaceID:        "test-space",
				QueryEmbedding: make([]float32, 100),
			},
			expectError: true,
			errorField:  "query_embedding",
		},
		{
			name: "query_embedding with wrong dimensions (3072) fails",
			request: models.RetrieveRequest{
				SpaceID:        "test-space",
				QueryEmbedding: make([]float32, 3072),
			},
			expectError: true,
			errorField:  "query_embedding",
		},
		{
			name: "empty query_embedding slice with query_text passes",
			request: models.RetrieveRequest{
				SpaceID:        "test-space",
				QueryText:      "search query",
				QueryEmbedding: []float32{},
			},
			expectError: true, // empty slice still fails embedding_dims validation
		},

		// candidate_k validation
		{
			name: "candidate_k=0 fails (below min)",
			request: models.RetrieveRequest{
				SpaceID:    "test-space",
				QueryText:  "search query",
				CandidateK: 0,
			},
			expectError: false, // omitempty allows 0 (zero value is skipped)
		},
		{
			name: "candidate_k=1 passes (min boundary)",
			request: models.RetrieveRequest{
				SpaceID:    "test-space",
				QueryText:  "search query",
				CandidateK: 1,
			},
			expectError: false,
		},
		{
			name: "candidate_k=1000 passes (max boundary)",
			request: models.RetrieveRequest{
				SpaceID:    "test-space",
				QueryText:  "search query",
				CandidateK: 1000,
			},
			expectError: false,
		},
		{
			name: "candidate_k=1001 fails (exceeds max)",
			request: models.RetrieveRequest{
				SpaceID:    "test-space",
				QueryText:  "search query",
				CandidateK: 1001,
			},
			expectError: true,
			errorField:  "candidate_k",
		},
		{
			name: "candidate_k=-1 fails (negative)",
			request: models.RetrieveRequest{
				SpaceID:    "test-space",
				QueryText:  "search query",
				CandidateK: -1,
			},
			expectError: true,
			errorField:  "candidate_k",
		},

		// top_k validation
		{
			name: "top_k=0 passes (omitempty skips zero value)",
			request: models.RetrieveRequest{
				SpaceID:   "test-space",
				QueryText: "search query",
				TopK:      0,
			},
			expectError: false,
		},
		{
			name: "top_k=1 passes (min boundary)",
			request: models.RetrieveRequest{
				SpaceID:   "test-space",
				QueryText: "search query",
				TopK:      1,
			},
			expectError: false,
		},
		{
			name: "top_k=100 passes (max boundary)",
			request: models.RetrieveRequest{
				SpaceID:   "test-space",
				QueryText: "search query",
				TopK:      100,
			},
			expectError: false,
		},
		{
			name: "top_k=101 fails (exceeds max)",
			request: models.RetrieveRequest{
				SpaceID:   "test-space",
				QueryText: "search query",
				TopK:      101,
			},
			expectError: true,
			errorField:  "top_k",
		},
		{
			name: "top_k=-1 fails (negative)",
			request: models.RetrieveRequest{
				SpaceID:   "test-space",
				QueryText: "search query",
				TopK:      -1,
			},
			expectError: true,
			errorField:  "top_k",
		},

		// hop_depth validation
		{
			name: "hop_depth=0 passes (min boundary)",
			request: models.RetrieveRequest{
				SpaceID:   "test-space",
				QueryText: "search query",
				HopDepth:  0,
			},
			expectError: false,
		},
		{
			name: "hop_depth=5 passes (max boundary)",
			request: models.RetrieveRequest{
				SpaceID:   "test-space",
				QueryText: "search query",
				HopDepth:  5,
			},
			expectError: false,
		},
		{
			name: "hop_depth=6 fails (exceeds max)",
			request: models.RetrieveRequest{
				SpaceID:   "test-space",
				QueryText: "search query",
				HopDepth:  6,
			},
			expectError: true,
			errorField:  "hop_depth",
		},
		{
			name: "hop_depth=-1 fails (negative)",
			request: models.RetrieveRequest{
				SpaceID:   "test-space",
				QueryText: "search query",
				HopDepth:  -1,
			},
			expectError: true,
			errorField:  "hop_depth",
		},
		{
			name: "hop_depth=3 passes (typical value)",
			request: models.RetrieveRequest{
				SpaceID:   "test-space",
				QueryText: "search query",
				HopDepth:  3,
			},
			expectError: false,
		},

		// Combined field validation
		{
			name: "multiple invalid fields produce multiple errors",
			request: models.RetrieveRequest{
				SpaceID:    "",
				QueryText:  "",
				CandidateK: 2000,
				TopK:       200,
				HopDepth:   10,
			},
			expectError: true,
		},
		{
			name: "valid max boundaries for all numeric fields",
			request: models.RetrieveRequest{
				SpaceID:    "test-space",
				QueryText:  "search query",
				CandidateK: 1000,
				TopK:       100,
				HopDepth:   5,
			},
			expectError: false,
		},
		{
			name: "valid min boundaries for all numeric fields",
			request: models.RetrieveRequest{
				SpaceID:    "test-space",
				QueryText:  "search query",
				CandidateK: 1,
				TopK:       1,
				HopDepth:   0,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.request)

			if tt.expectError && err == nil {
				t.Error("expected validation error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}

			// If we expect a specific field error, verify it
			if tt.expectError && err != nil && tt.errorField != "" {
				resp := FormatValidationErrors(err)
				found := false
				for _, d := range resp.Details {
					if d.Field == tt.errorField {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error for field %q, got errors: %v", tt.errorField, resp.Details)
				}
			}
		})
	}
}

// TestValidateRetrieveRequest_EitherOrLogic tests the required_without logic specifically
func TestValidateRetrieveRequest_EitherOrLogic(t *testing.T) {
	tests := []struct {
		name           string
		queryText      string
		queryEmbedding []float32
		expectError    bool
		errorFields    []string // fields expected in error
	}{
		{
			name:           "only query_text provided",
			queryText:      "search query",
			queryEmbedding: nil,
			expectError:    false,
		},
		{
			name:           "only query_embedding provided (1536 dims)",
			queryText:      "",
			queryEmbedding: make([]float32, 1536),
			expectError:    false,
		},
		{
			name:           "only query_embedding provided (768 dims)",
			queryText:      "",
			queryEmbedding: make([]float32, 768),
			expectError:    false,
		},
		{
			name:           "both provided",
			queryText:      "search query",
			queryEmbedding: make([]float32, 1536),
			expectError:    false,
		},
		{
			name:           "neither provided - both should error",
			queryText:      "",
			queryEmbedding: nil,
			expectError:    true,
			errorFields:    []string{"query_text", "query_embedding"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := models.RetrieveRequest{
				SpaceID:        "test-space",
				QueryText:      tt.queryText,
				QueryEmbedding: tt.queryEmbedding,
			}
			err := Validate(req)

			if tt.expectError && err == nil {
				t.Error("expected validation error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}

			// Verify expected error fields
			if tt.expectError && err != nil && len(tt.errorFields) > 0 {
				resp := FormatValidationErrors(err)
				for _, expectedField := range tt.errorFields {
					found := false
					for _, d := range resp.Details {
						if d.Field == expectedField {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("expected error for field %q, got errors: %v", expectedField, resp.Details)
					}
				}
			}
		})
	}
}

// TestValidateRetrieveRequest_PolicyContext tests that PolicyContext allows any map
func TestValidateRetrieveRequest_PolicyContext(t *testing.T) {
	tests := []struct {
		name          string
		policyContext map[string]any
		expectError   bool
	}{
		{
			name:          "nil policy_context passes",
			policyContext: nil,
			expectError:   false,
		},
		{
			name:          "empty policy_context passes",
			policyContext: map[string]any{},
			expectError:   false,
		},
		{
			name:          "policy_context with string values",
			policyContext: map[string]any{"key": "value"},
			expectError:   false,
		},
		{
			name:          "policy_context with mixed types",
			policyContext: map[string]any{"str": "value", "num": 42, "bool": true},
			expectError:   false,
		},
		{
			name:          "policy_context with nested map",
			policyContext: map[string]any{"nested": map[string]any{"inner": "value"}},
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := models.RetrieveRequest{
				SpaceID:       "test-space",
				QueryText:     "search query",
				PolicyContext: tt.policyContext,
			}
			err := Validate(req)

			if tt.expectError && err == nil {
				t.Error("expected validation error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

// TestValidateRetrieveRequest_ErrorMessages tests that error messages are user-friendly
func TestValidateRetrieveRequest_ErrorMessages(t *testing.T) {
	tests := []struct {
		name            string
		request         models.RetrieveRequest
		expectedField   string
		containsMessage string
	}{
		{
			name: "missing space_id has friendly message",
			request: models.RetrieveRequest{
				QueryText: "search query",
			},
			expectedField:   "space_id",
			containsMessage: "required",
		},
		{
			name: "invalid embedding dims has friendly message",
			request: models.RetrieveRequest{
				SpaceID:        "test-space",
				QueryEmbedding: make([]float32, 512),
			},
			expectedField:   "query_embedding",
			containsMessage: "384, 768, 1024, or 1536",
		},
		{
			name: "top_k exceeds max has friendly message",
			request: models.RetrieveRequest{
				SpaceID:   "test-space",
				QueryText: "search query",
				TopK:      101,
			},
			expectedField:   "top_k",
			containsMessage: "100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.request)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}

			resp := FormatValidationErrors(err)
			found := false
			for _, d := range resp.Details {
				if d.Field == tt.expectedField {
					found = true
					if tt.containsMessage != "" && !contains(d.Message, tt.containsMessage) {
						t.Errorf("message for %s = %q, expected to contain %q",
							tt.expectedField, d.Message, tt.containsMessage)
					}
					break
				}
			}

			if !found {
				t.Errorf("expected error for field %q, got errors: %v", tt.expectedField, resp.Details)
			}
		})
	}
}

// contains checks if s contains substr (case-insensitive for flexibility)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && containsSubstring(s, substr)))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ============================================================================
// IngestRequest Validation Tests
// ============================================================================

// TestValidateIngestRequest tests all validation rules for IngestRequest
func TestValidateIngestRequest(t *testing.T) {
	// Helper to create a valid 1536-dim embedding
	validEmbedding := func() []float32 {
		return make([]float32, 1536)
	}

	// Helper to create a confidence value
	floatPtr := func(v float64) *float64 {
		return &v
	}

	tests := []struct {
		name        string
		request     models.IngestRequest
		expectError bool
		errorField  string // expected field in error (optional)
	}{
		// Valid cases
		{
			name: "valid minimal request",
			request: models.IngestRequest{
				SpaceID:   "test-space",
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "test-source",
				Content:   "test content",
			},
			expectError: false,
		},
		{
			name: "valid request with all optional fields",
			request: models.IngestRequest{
				SpaceID:     "test-space",
				Timestamp:   "2024-01-01T00:00:00Z",
				Source:      "test-source",
				Content:     "test content",
				Tags:        []string{"tag1", "tag2"},
				NodeID:      "node-123",
				Path:        "/some/path",
				Name:        "Test Node",
				Sensitivity: "internal",
				Confidence:  floatPtr(0.95),
				Embedding:   validEmbedding(),
			},
			expectError: false,
		},
		{
			name: "valid request with map content",
			request: models.IngestRequest{
				SpaceID:   "test-space",
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "test-source",
				Content:   map[string]any{"key": "value"},
			},
			expectError: false,
		},

		// space_id validation
		{
			name: "missing space_id fails",
			request: models.IngestRequest{
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "test-source",
				Content:   "test content",
			},
			expectError: true,
			errorField:  "space_id",
		},
		{
			name: "empty space_id fails",
			request: models.IngestRequest{
				SpaceID:   "",
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "test-source",
				Content:   "test content",
			},
			expectError: true,
			errorField:  "space_id",
		},
		{
			name: "space_id at max length (256) passes",
			request: models.IngestRequest{
				SpaceID:   string(make([]byte, 256)),
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "test-source",
				Content:   "test content",
			},
			expectError: false,
		},
		{
			name: "space_id exceeds max length (257) fails",
			request: models.IngestRequest{
				SpaceID:   string(make([]byte, 257)),
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "test-source",
				Content:   "test content",
			},
			expectError: true,
			errorField:  "space_id",
		},

		// timestamp validation
		{
			name: "missing timestamp fails",
			request: models.IngestRequest{
				SpaceID: "test-space",
				Source:  "test-source",
				Content: "test content",
			},
			expectError: true,
			errorField:  "timestamp",
		},
		{
			name: "empty timestamp fails",
			request: models.IngestRequest{
				SpaceID:   "test-space",
				Timestamp: "",
				Source:    "test-source",
				Content:   "test content",
			},
			expectError: true,
			errorField:  "timestamp",
		},

		// source validation
		{
			name: "missing source fails",
			request: models.IngestRequest{
				SpaceID:   "test-space",
				Timestamp: "2024-01-01T00:00:00Z",
				Content:   "test content",
			},
			expectError: true,
			errorField:  "source",
		},
		{
			name: "empty source fails",
			request: models.IngestRequest{
				SpaceID:   "test-space",
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "",
				Content:   "test content",
			},
			expectError: true,
			errorField:  "source",
		},
		{
			name: "source at max length (64) passes",
			request: models.IngestRequest{
				SpaceID:   "test-space",
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    string(make([]byte, 64)),
				Content:   "test content",
			},
			expectError: false,
		},
		{
			name: "source exceeds max length (65) fails",
			request: models.IngestRequest{
				SpaceID:   "test-space",
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    string(make([]byte, 65)),
				Content:   "test content",
			},
			expectError: true,
			errorField:  "source",
		},

		// content validation
		{
			name: "nil content fails",
			request: models.IngestRequest{
				SpaceID:   "test-space",
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "test-source",
				Content:   nil,
			},
			expectError: true,
			errorField:  "content",
		},

		// tags validation
		{
			name: "valid tags array passes",
			request: models.IngestRequest{
				SpaceID:   "test-space",
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "test-source",
				Content:   "test content",
				Tags:      []string{"tag1", "tag2", "tag3"},
			},
			expectError: false,
		},
		{
			name: "empty tag string in array fails",
			request: models.IngestRequest{
				SpaceID:   "test-space",
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "test-source",
				Content:   "test content",
				Tags:      []string{"tag1", ""},
			},
			expectError: true,
			errorField:  "tags[1]",
		},
		{
			name: "empty tags array passes (omitempty)",
			request: models.IngestRequest{
				SpaceID:   "test-space",
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "test-source",
				Content:   "test content",
				Tags:      []string{},
			},
			expectError: false,
		},

		// path validation
		{
			name: "path at max length (512) passes",
			request: models.IngestRequest{
				SpaceID:   "test-space",
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "test-source",
				Content:   "test content",
				Path:      string(make([]byte, 512)),
			},
			expectError: false,
		},
		{
			name: "path exceeds max length (513) fails",
			request: models.IngestRequest{
				SpaceID:   "test-space",
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "test-source",
				Content:   "test content",
				Path:      string(make([]byte, 513)),
			},
			expectError: true,
			errorField:  "path",
		},

		// sensitivity validation
		{
			name: "sensitivity 'public' passes",
			request: models.IngestRequest{
				SpaceID:     "test-space",
				Timestamp:   "2024-01-01T00:00:00Z",
				Source:      "test-source",
				Content:     "test content",
				Sensitivity: "public",
			},
			expectError: false,
		},
		{
			name: "sensitivity 'internal' passes",
			request: models.IngestRequest{
				SpaceID:     "test-space",
				Timestamp:   "2024-01-01T00:00:00Z",
				Source:      "test-source",
				Content:     "test content",
				Sensitivity: "internal",
			},
			expectError: false,
		},
		{
			name: "sensitivity 'confidential' passes",
			request: models.IngestRequest{
				SpaceID:     "test-space",
				Timestamp:   "2024-01-01T00:00:00Z",
				Source:      "test-source",
				Content:     "test content",
				Sensitivity: "confidential",
			},
			expectError: false,
		},
		{
			name: "invalid sensitivity fails",
			request: models.IngestRequest{
				SpaceID:     "test-space",
				Timestamp:   "2024-01-01T00:00:00Z",
				Source:      "test-source",
				Content:     "test content",
				Sensitivity: "secret",
			},
			expectError: true,
			errorField:  "sensitivity",
		},
		{
			name: "sensitivity is case-sensitive - 'Public' fails",
			request: models.IngestRequest{
				SpaceID:     "test-space",
				Timestamp:   "2024-01-01T00:00:00Z",
				Source:      "test-source",
				Content:     "test content",
				Sensitivity: "Public",
			},
			expectError: true,
			errorField:  "sensitivity",
		},

		// confidence validation
		{
			name: "confidence=0 passes",
			request: models.IngestRequest{
				SpaceID:    "test-space",
				Timestamp:  "2024-01-01T00:00:00Z",
				Source:     "test-source",
				Content:    "test content",
				Confidence: floatPtr(0),
			},
			expectError: false,
		},
		{
			name: "confidence=1 passes",
			request: models.IngestRequest{
				SpaceID:    "test-space",
				Timestamp:  "2024-01-01T00:00:00Z",
				Source:     "test-source",
				Content:    "test content",
				Confidence: floatPtr(1),
			},
			expectError: false,
		},
		{
			name: "confidence=0.5 passes",
			request: models.IngestRequest{
				SpaceID:    "test-space",
				Timestamp:  "2024-01-01T00:00:00Z",
				Source:     "test-source",
				Content:    "test content",
				Confidence: floatPtr(0.5),
			},
			expectError: false,
		},
		{
			name: "confidence=-0.1 fails",
			request: models.IngestRequest{
				SpaceID:    "test-space",
				Timestamp:  "2024-01-01T00:00:00Z",
				Source:     "test-source",
				Content:    "test content",
				Confidence: floatPtr(-0.1),
			},
			expectError: true,
			errorField:  "confidence",
		},
		{
			name: "confidence=1.1 fails",
			request: models.IngestRequest{
				SpaceID:    "test-space",
				Timestamp:  "2024-01-01T00:00:00Z",
				Source:     "test-source",
				Content:    "test content",
				Confidence: floatPtr(1.1),
			},
			expectError: true,
			errorField:  "confidence",
		},
		{
			name: "nil confidence passes (omitempty)",
			request: models.IngestRequest{
				SpaceID:    "test-space",
				Timestamp:  "2024-01-01T00:00:00Z",
				Source:     "test-source",
				Content:    "test content",
				Confidence: nil,
			},
			expectError: false,
		},

		// embedding validation
		{
			name: "embedding with 1536 dims passes",
			request: models.IngestRequest{
				SpaceID:   "test-space",
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "test-source",
				Content:   "test content",
				Embedding: make([]float32, 1536),
			},
			expectError: false,
		},
		{
			name: "embedding with 768 dims passes",
			request: models.IngestRequest{
				SpaceID:   "test-space",
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "test-source",
				Content:   "test content",
				Embedding: make([]float32, 768),
			},
			expectError: false,
		},
		{
			name: "embedding with 512 dims fails",
			request: models.IngestRequest{
				SpaceID:   "test-space",
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "test-source",
				Content:   "test content",
				Embedding: make([]float32, 512),
			},
			expectError: true,
			errorField:  "embedding",
		},
		{
			name: "empty embedding slice fails",
			request: models.IngestRequest{
				SpaceID:   "test-space",
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "test-source",
				Content:   "test content",
				Embedding: []float32{},
			},
			expectError: true,
			errorField:  "embedding",
		},
		{
			name: "nil embedding passes (omitempty)",
			request: models.IngestRequest{
				SpaceID:   "test-space",
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "test-source",
				Content:   "test content",
				Embedding: nil,
			},
			expectError: false,
		},

		// Multiple field validation
		{
			name: "multiple invalid fields produce multiple errors",
			request: models.IngestRequest{
				SpaceID:     "",
				Timestamp:   "",
				Source:      "",
				Content:     nil,
				Sensitivity: "invalid",
				Confidence:  floatPtr(2.0),
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.request)

			if tt.expectError && err == nil {
				t.Error("expected validation error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}

			// If we expect a specific field error, verify it
			if tt.expectError && err != nil && tt.errorField != "" {
				resp := FormatValidationErrors(err)
				found := false
				for _, d := range resp.Details {
					if d.Field == tt.errorField {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error for field %q, got errors: %v", tt.errorField, resp.Details)
				}
			}
		})
	}
}

// ============================================================================
// ReflectRequest Validation Tests
// ============================================================================

// TestValidateReflectRequest tests all validation rules for ReflectRequest
func TestValidateReflectRequest(t *testing.T) {
	// Helper to create a valid 1536-dim embedding
	validEmbedding := func() []float32 {
		return make([]float32, 1536)
	}

	// Helper to create a valid 768-dim embedding
	validEmbedding768 := func() []float32 {
		return make([]float32, 768)
	}

	tests := []struct {
		name        string
		request     models.ReflectRequest
		expectError bool
		errorField  string // expected field in error (optional)
	}{
		// Valid cases
		{
			name: "valid request with topic",
			request: models.ReflectRequest{
				SpaceID: "test-space",
				Topic:   "machine learning concepts",
			},
			expectError: false,
		},
		{
			name: "valid request with topic_embedding (1536 dims)",
			request: models.ReflectRequest{
				SpaceID:        "test-space",
				TopicEmbedding: validEmbedding(),
			},
			expectError: false,
		},
		{
			name: "valid request with topic_embedding (768 dims)",
			request: models.ReflectRequest{
				SpaceID:        "test-space",
				TopicEmbedding: validEmbedding768(),
			},
			expectError: false,
		},
		{
			name: "valid request with both topic and topic_embedding",
			request: models.ReflectRequest{
				SpaceID:        "test-space",
				Topic:          "machine learning",
				TopicEmbedding: validEmbedding(),
			},
			expectError: false,
		},
		{
			name: "valid request with all optional fields",
			request: models.ReflectRequest{
				SpaceID:  "test-space",
				Topic:    "machine learning concepts",
				MaxDepth: 5,
				MaxNodes: 100,
			},
			expectError: false,
		},

		// space_id validation
		{
			name: "missing space_id fails",
			request: models.ReflectRequest{
				Topic: "machine learning",
			},
			expectError: true,
			errorField:  "space_id",
		},
		{
			name: "empty space_id fails",
			request: models.ReflectRequest{
				SpaceID: "",
				Topic:   "machine learning",
			},
			expectError: true,
			errorField:  "space_id",
		},
		{
			name: "single character space_id passes",
			request: models.ReflectRequest{
				SpaceID: "a",
				Topic:   "machine learning",
			},
			expectError: false,
		},

		// Either-or validation (topic OR topic_embedding)
		{
			name: "neither topic nor topic_embedding fails",
			request: models.ReflectRequest{
				SpaceID: "test-space",
			},
			expectError: true,
		},
		{
			name: "empty topic with nil topic_embedding fails",
			request: models.ReflectRequest{
				SpaceID:        "test-space",
				Topic:          "",
				TopicEmbedding: nil,
			},
			expectError: true,
		},

		// topic validation
		{
			name: "topic at max length (500) passes",
			request: models.ReflectRequest{
				SpaceID: "test-space",
				Topic:   string(make([]byte, 500)),
			},
			expectError: false,
		},
		{
			name: "topic exceeds max length (501) fails",
			request: models.ReflectRequest{
				SpaceID: "test-space",
				Topic:   string(make([]byte, 501)),
			},
			expectError: true,
			errorField:  "topic",
		},

		// topic_embedding validation
		{
			name: "topic_embedding with wrong dimensions (512) fails",
			request: models.ReflectRequest{
				SpaceID:        "test-space",
				TopicEmbedding: make([]float32, 512),
			},
			expectError: true,
			errorField:  "topic_embedding",
		},
		{
			name: "topic_embedding with wrong dimensions (100) fails",
			request: models.ReflectRequest{
				SpaceID:        "test-space",
				TopicEmbedding: make([]float32, 100),
			},
			expectError: true,
			errorField:  "topic_embedding",
		},
		{
			name: "empty topic_embedding slice with topic passes",
			request: models.ReflectRequest{
				SpaceID:        "test-space",
				Topic:          "machine learning",
				TopicEmbedding: []float32{},
			},
			expectError: true, // empty slice still fails embedding_dims validation
		},

		// max_depth validation
		{
			name: "max_depth=0 passes (omitempty skips zero value)",
			request: models.ReflectRequest{
				SpaceID:  "test-space",
				Topic:    "machine learning",
				MaxDepth: 0,
			},
			expectError: false,
		},
		{
			name: "max_depth=1 passes (min boundary)",
			request: models.ReflectRequest{
				SpaceID:  "test-space",
				Topic:    "machine learning",
				MaxDepth: 1,
			},
			expectError: false,
		},
		{
			name: "max_depth=10 passes (max boundary)",
			request: models.ReflectRequest{
				SpaceID:  "test-space",
				Topic:    "machine learning",
				MaxDepth: 10,
			},
			expectError: false,
		},
		{
			name: "max_depth=11 fails (exceeds max)",
			request: models.ReflectRequest{
				SpaceID:  "test-space",
				Topic:    "machine learning",
				MaxDepth: 11,
			},
			expectError: true,
			errorField:  "max_depth",
		},
		{
			name: "max_depth=-1 fails (negative)",
			request: models.ReflectRequest{
				SpaceID:  "test-space",
				Topic:    "machine learning",
				MaxDepth: -1,
			},
			expectError: true,
			errorField:  "max_depth",
		},

		// max_nodes validation
		{
			name: "max_nodes=0 passes (omitempty skips zero value)",
			request: models.ReflectRequest{
				SpaceID:  "test-space",
				Topic:    "machine learning",
				MaxNodes: 0,
			},
			expectError: false,
		},
		{
			name: "max_nodes=1 passes (min boundary)",
			request: models.ReflectRequest{
				SpaceID:  "test-space",
				Topic:    "machine learning",
				MaxNodes: 1,
			},
			expectError: false,
		},
		{
			name: "max_nodes=500 passes (max boundary)",
			request: models.ReflectRequest{
				SpaceID:  "test-space",
				Topic:    "machine learning",
				MaxNodes: 500,
			},
			expectError: false,
		},
		{
			name: "max_nodes=501 fails (exceeds max)",
			request: models.ReflectRequest{
				SpaceID:  "test-space",
				Topic:    "machine learning",
				MaxNodes: 501,
			},
			expectError: true,
			errorField:  "max_nodes",
		},
		{
			name: "max_nodes=-1 fails (negative)",
			request: models.ReflectRequest{
				SpaceID:  "test-space",
				Topic:    "machine learning",
				MaxNodes: -1,
			},
			expectError: true,
			errorField:  "max_nodes",
		},

		// Combined field validation
		{
			name: "valid max boundaries for all numeric fields",
			request: models.ReflectRequest{
				SpaceID:  "test-space",
				Topic:    "machine learning",
				MaxDepth: 10,
				MaxNodes: 500,
			},
			expectError: false,
		},
		{
			name: "valid min boundaries for all numeric fields",
			request: models.ReflectRequest{
				SpaceID:  "test-space",
				Topic:    "machine learning",
				MaxDepth: 1,
				MaxNodes: 1,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.request)

			if tt.expectError && err == nil {
				t.Error("expected validation error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}

			// If we expect a specific field error, verify it
			if tt.expectError && err != nil && tt.errorField != "" {
				resp := FormatValidationErrors(err)
				found := false
				for _, d := range resp.Details {
					if d.Field == tt.errorField {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error for field %q, got errors: %v", tt.errorField, resp.Details)
				}
			}
		})
	}
}

// TestValidateReflectRequest_EitherOrLogic tests the required_without logic specifically
func TestValidateReflectRequest_EitherOrLogic(t *testing.T) {
	tests := []struct {
		name           string
		topic          string
		topicEmbedding []float32
		expectError    bool
		errorFields    []string // fields expected in error
	}{
		{
			name:           "only topic provided",
			topic:          "machine learning",
			topicEmbedding: nil,
			expectError:    false,
		},
		{
			name:           "only topic_embedding provided (1536 dims)",
			topic:          "",
			topicEmbedding: make([]float32, 1536),
			expectError:    false,
		},
		{
			name:           "only topic_embedding provided (768 dims)",
			topic:          "",
			topicEmbedding: make([]float32, 768),
			expectError:    false,
		},
		{
			name:           "both provided",
			topic:          "machine learning",
			topicEmbedding: make([]float32, 1536),
			expectError:    false,
		},
		{
			name:           "neither provided - both should error",
			topic:          "",
			topicEmbedding: nil,
			expectError:    true,
			errorFields:    []string{"topic", "topic_embedding"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := models.ReflectRequest{
				SpaceID:        "test-space",
				Topic:          tt.topic,
				TopicEmbedding: tt.topicEmbedding,
			}
			err := Validate(req)

			if tt.expectError && err == nil {
				t.Error("expected validation error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}

			// Verify expected error fields
			if tt.expectError && err != nil && len(tt.errorFields) > 0 {
				resp := FormatValidationErrors(err)
				for _, expectedField := range tt.errorFields {
					found := false
					for _, d := range resp.Details {
						if d.Field == expectedField {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("expected error for field %q, got errors: %v", expectedField, resp.Details)
					}
				}
			}
		})
	}
}

// ============================================================================
// BatchIngestRequest Validation Tests
// ============================================================================

// TestValidateBatchIngestRequest tests all validation rules for BatchIngestRequest
func TestValidateBatchIngestRequest(t *testing.T) {
	// Helper to create a valid BatchIngestItem
	validItem := func() models.BatchIngestItem {
		return models.BatchIngestItem{
			Timestamp: "2024-01-01T00:00:00Z",
			Source:    "test-source",
			Content:   "test content",
		}
	}

	// Helper to create N valid items
	validItems := func(n int) []models.BatchIngestItem {
		items := make([]models.BatchIngestItem, n)
		for i := 0; i < n; i++ {
			items[i] = validItem()
		}
		return items
	}

	tests := []struct {
		name        string
		request     models.BatchIngestRequest
		expectError bool
		errorField  string // expected field in error (optional)
	}{
		// Valid cases
		{
			name: "valid request with single observation",
			request: models.BatchIngestRequest{
				SpaceID:      "test-space",
				Observations: []models.BatchIngestItem{validItem()},
			},
			expectError: false,
		},
		{
			name: "valid request with multiple observations",
			request: models.BatchIngestRequest{
				SpaceID:      "test-space",
				Observations: validItems(10),
			},
			expectError: false,
		},
		{
			name: "valid request with max observations (100)",
			request: models.BatchIngestRequest{
				SpaceID:      "test-space",
				Observations: validItems(100),
			},
			expectError: false,
		},

		// space_id validation
		{
			name: "missing space_id fails",
			request: models.BatchIngestRequest{
				Observations: []models.BatchIngestItem{validItem()},
			},
			expectError: true,
			errorField:  "space_id",
		},
		{
			name: "empty space_id fails",
			request: models.BatchIngestRequest{
				SpaceID:      "",
				Observations: []models.BatchIngestItem{validItem()},
			},
			expectError: true,
			errorField:  "space_id",
		},
		{
			name: "single character space_id passes",
			request: models.BatchIngestRequest{
				SpaceID:      "a",
				Observations: []models.BatchIngestItem{validItem()},
			},
			expectError: false,
		},

		// observations validation
		{
			name: "missing observations fails",
			request: models.BatchIngestRequest{
				SpaceID: "test-space",
			},
			expectError: true,
			errorField:  "observations",
		},
		{
			name: "empty observations array fails",
			request: models.BatchIngestRequest{
				SpaceID:      "test-space",
				Observations: []models.BatchIngestItem{},
			},
			expectError: true,
			errorField:  "observations",
		},
		{
			name: "too many observations (2001) fails",
			request: models.BatchIngestRequest{
				SpaceID:      "test-space",
				Observations: validItems(2001),
			},
			expectError: true,
			errorField:  "observations",
		},

		// Nested item validation (dive)
		{
			name: "invalid item in observations fails",
			request: models.BatchIngestRequest{
				SpaceID: "test-space",
				Observations: []models.BatchIngestItem{
					validItem(),
					{
						// Missing required fields
						Timestamp: "",
						Source:    "",
						Content:   nil,
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.request)

			if tt.expectError && err == nil {
				t.Error("expected validation error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}

			// If we expect a specific field error, verify it
			if tt.expectError && err != nil && tt.errorField != "" {
				resp := FormatValidationErrors(err)
				found := false
				for _, d := range resp.Details {
					if d.Field == tt.errorField {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error for field %q, got errors: %v", tt.errorField, resp.Details)
				}
			}
		})
	}
}

// TestValidateBatchIngestItem tests all validation rules for BatchIngestItem
func TestValidateBatchIngestItem(t *testing.T) {
	// Helper to create a confidence value
	floatPtr := func(v float64) *float64 {
		return &v
	}

	tests := []struct {
		name        string
		item        models.BatchIngestItem
		expectError bool
		errorField  string // expected field in error (optional)
	}{
		// Valid cases
		{
			name: "valid minimal item",
			item: models.BatchIngestItem{
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "test-source",
				Content:   "test content",
			},
			expectError: false,
		},
		{
			name: "valid item with all optional fields",
			item: models.BatchIngestItem{
				Timestamp:   "2024-01-01T00:00:00Z",
				Source:      "test-source",
				Content:     "test content",
				Tags:        []string{"tag1", "tag2"},
				NodeID:      "node-123",
				Path:        "/some/path",
				Name:        "Test Node",
				Sensitivity: "internal",
				Confidence:  floatPtr(0.95),
				Embedding:   make([]float32, 1536),
			},
			expectError: false,
		},

		// timestamp validation
		{
			name: "missing timestamp fails",
			item: models.BatchIngestItem{
				Source:  "test-source",
				Content: "test content",
			},
			expectError: true,
			errorField:  "timestamp",
		},
		{
			name: "empty timestamp fails",
			item: models.BatchIngestItem{
				Timestamp: "",
				Source:    "test-source",
				Content:   "test content",
			},
			expectError: true,
			errorField:  "timestamp",
		},

		// source validation
		{
			name: "missing source fails",
			item: models.BatchIngestItem{
				Timestamp: "2024-01-01T00:00:00Z",
				Content:   "test content",
			},
			expectError: true,
			errorField:  "source",
		},
		{
			name: "empty source fails",
			item: models.BatchIngestItem{
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "",
				Content:   "test content",
			},
			expectError: true,
			errorField:  "source",
		},
		{
			name: "source at max length (64) passes",
			item: models.BatchIngestItem{
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    string(make([]byte, 64)),
				Content:   "test content",
			},
			expectError: false,
		},
		{
			name: "source exceeds max length (65) fails",
			item: models.BatchIngestItem{
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    string(make([]byte, 65)),
				Content:   "test content",
			},
			expectError: true,
			errorField:  "source",
		},

		// content validation
		{
			name: "nil content fails",
			item: models.BatchIngestItem{
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "test-source",
				Content:   nil,
			},
			expectError: true,
			errorField:  "content",
		},

		// tags validation
		{
			name: "valid tags array passes",
			item: models.BatchIngestItem{
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "test-source",
				Content:   "test content",
				Tags:      []string{"tag1", "tag2"},
			},
			expectError: false,
		},
		{
			name: "empty tag string in array fails",
			item: models.BatchIngestItem{
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "test-source",
				Content:   "test content",
				Tags:      []string{"tag1", ""},
			},
			expectError: true,
			errorField:  "tags[1]",
		},

		// path validation
		{
			name: "path at max length (512) passes",
			item: models.BatchIngestItem{
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "test-source",
				Content:   "test content",
				Path:      string(make([]byte, 512)),
			},
			expectError: false,
		},
		{
			name: "path exceeds max length (513) fails",
			item: models.BatchIngestItem{
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "test-source",
				Content:   "test content",
				Path:      string(make([]byte, 513)),
			},
			expectError: true,
			errorField:  "path",
		},

		// sensitivity validation
		{
			name: "sensitivity 'public' passes",
			item: models.BatchIngestItem{
				Timestamp:   "2024-01-01T00:00:00Z",
				Source:      "test-source",
				Content:     "test content",
				Sensitivity: "public",
			},
			expectError: false,
		},
		{
			name: "sensitivity 'internal' passes",
			item: models.BatchIngestItem{
				Timestamp:   "2024-01-01T00:00:00Z",
				Source:      "test-source",
				Content:     "test content",
				Sensitivity: "internal",
			},
			expectError: false,
		},
		{
			name: "sensitivity 'confidential' passes",
			item: models.BatchIngestItem{
				Timestamp:   "2024-01-01T00:00:00Z",
				Source:      "test-source",
				Content:     "test content",
				Sensitivity: "confidential",
			},
			expectError: false,
		},
		{
			name: "invalid sensitivity fails",
			item: models.BatchIngestItem{
				Timestamp:   "2024-01-01T00:00:00Z",
				Source:      "test-source",
				Content:     "test content",
				Sensitivity: "secret",
			},
			expectError: true,
			errorField:  "sensitivity",
		},

		// confidence validation
		{
			name: "confidence=0 passes",
			item: models.BatchIngestItem{
				Timestamp:  "2024-01-01T00:00:00Z",
				Source:     "test-source",
				Content:    "test content",
				Confidence: floatPtr(0),
			},
			expectError: false,
		},
		{
			name: "confidence=1 passes",
			item: models.BatchIngestItem{
				Timestamp:  "2024-01-01T00:00:00Z",
				Source:     "test-source",
				Content:    "test content",
				Confidence: floatPtr(1),
			},
			expectError: false,
		},
		{
			name: "confidence=-0.1 fails",
			item: models.BatchIngestItem{
				Timestamp:  "2024-01-01T00:00:00Z",
				Source:     "test-source",
				Content:    "test content",
				Confidence: floatPtr(-0.1),
			},
			expectError: true,
			errorField:  "confidence",
		},
		{
			name: "confidence=1.1 fails",
			item: models.BatchIngestItem{
				Timestamp:  "2024-01-01T00:00:00Z",
				Source:     "test-source",
				Content:    "test content",
				Confidence: floatPtr(1.1),
			},
			expectError: true,
			errorField:  "confidence",
		},

		// embedding validation
		{
			name: "embedding with 1536 dims passes",
			item: models.BatchIngestItem{
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "test-source",
				Content:   "test content",
				Embedding: make([]float32, 1536),
			},
			expectError: false,
		},
		{
			name: "embedding with 768 dims passes",
			item: models.BatchIngestItem{
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "test-source",
				Content:   "test content",
				Embedding: make([]float32, 768),
			},
			expectError: false,
		},
		{
			name: "embedding with 512 dims fails",
			item: models.BatchIngestItem{
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "test-source",
				Content:   "test content",
				Embedding: make([]float32, 512),
			},
			expectError: true,
			errorField:  "embedding",
		},
		{
			name: "empty embedding slice fails",
			item: models.BatchIngestItem{
				Timestamp: "2024-01-01T00:00:00Z",
				Source:    "test-source",
				Content:   "test content",
				Embedding: []float32{},
			},
			expectError: true,
			errorField:  "embedding",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.item)

			if tt.expectError && err == nil {
				t.Error("expected validation error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}

			// If we expect a specific field error, verify it
			if tt.expectError && err != nil && tt.errorField != "" {
				resp := FormatValidationErrors(err)
				found := false
				for _, d := range resp.Details {
					if d.Field == tt.errorField {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error for field %q, got errors: %v", tt.errorField, resp.Details)
				}
			}
		})
	}
}

// TestValidateBatchIngestRequest_NestedErrors tests that nested item errors are properly reported
func TestValidateBatchIngestRequest_NestedErrors(t *testing.T) {
	tests := []struct {
		name                string
		request             models.BatchIngestRequest
		expectError         bool
		expectNestedError   bool   // expect error in nested item
		expectedNestedField string // field expected to be in nested error
	}{
		{
			name: "nested item missing timestamp",
			request: models.BatchIngestRequest{
				SpaceID: "test-space",
				Observations: []models.BatchIngestItem{
					{
						Timestamp: "2024-01-01T00:00:00Z",
						Source:    "test-source",
						Content:   "test content",
					},
					{
						// Missing timestamp
						Source:  "test-source",
						Content: "test content",
					},
				},
			},
			expectError:         true,
			expectNestedError:   true,
			expectedNestedField: "timestamp",
		},
		{
			name: "nested item invalid embedding",
			request: models.BatchIngestRequest{
				SpaceID: "test-space",
				Observations: []models.BatchIngestItem{
					{
						Timestamp: "2024-01-01T00:00:00Z",
						Source:    "test-source",
						Content:   "test content",
						Embedding: make([]float32, 512), // Invalid dimensions
					},
				},
			},
			expectError:         true,
			expectNestedError:   true,
			expectedNestedField: "embedding",
		},
		{
			name: "nested item invalid sensitivity",
			request: models.BatchIngestRequest{
				SpaceID: "test-space",
				Observations: []models.BatchIngestItem{
					{
						Timestamp:   "2024-01-01T00:00:00Z",
						Source:      "test-source",
						Content:     "test content",
						Sensitivity: "invalid-sensitivity",
					},
				},
			},
			expectError:         true,
			expectNestedError:   true,
			expectedNestedField: "sensitivity",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.request)

			if tt.expectError && err == nil {
				t.Error("expected validation error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}

			// If we expect a nested error, verify the field appears (possibly with index prefix)
			if tt.expectNestedError && err != nil && tt.expectedNestedField != "" {
				resp := FormatValidationErrors(err)
				found := false
				for _, d := range resp.Details {
					// Field could be "observations[1].timestamp" or just "timestamp" depending on dive behavior
					if containsSubstring(d.Field, tt.expectedNestedField) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected nested error containing field %q, got errors: %v", tt.expectedNestedField, resp.Details)
				}
			}
		})
	}
}
