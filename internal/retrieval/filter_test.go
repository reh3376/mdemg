package retrieval

import (
	"testing"

	"mdemg/internal/models"
)

func TestFileFilter_BuildCypherFilter(t *testing.T) {
	tests := []struct {
		name         string
		filter       FileFilter
		wantEmpty    bool
		wantContains []string
	}{
		{
			name:      "empty filter",
			filter:    FileFilter{},
			wantEmpty: true,
		},
		{
			name: "include only",
			filter: FileFilter{
				IncludeExtensions: []string{"java", "go"},
			},
			wantContains: []string{"includeExtensions", "ENDS WITH"},
		},
		{
			name: "exclude only",
			filter: FileFilter{
				ExcludeExtensions: []string{"md", "txt"},
			},
			wantContains: []string{"excludeExtensions", "NOT ANY"},
		},
		{
			name: "both include and exclude",
			filter: FileFilter{
				IncludeExtensions: []string{"java"},
				ExcludeExtensions: []string{"md"},
			},
			wantContains: []string{"includeExtensions", "excludeExtensions", "AND"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.filter.BuildCypherFilter()

			if tt.wantEmpty && result != "" {
				t.Errorf("expected empty filter, got: %s", result)
			}

			for _, want := range tt.wantContains {
				if !contains(result, want) {
					t.Errorf("filter %q should contain %q", result, want)
				}
			}
		})
	}
}

func TestFileFilter_IsEmpty(t *testing.T) {
	tests := []struct {
		name   string
		filter FileFilter
		want   bool
	}{
		{
			name:   "empty",
			filter: FileFilter{},
			want:   true,
		},
		{
			name:   "include set",
			filter: FileFilter{IncludeExtensions: []string{"java"}},
			want:   false,
		},
		{
			name:   "exclude set",
			filter: FileFilter{ExcludeExtensions: []string{"md"}},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filter.IsEmpty(); got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewFileFilterFromRequest(t *testing.T) {
	// Test basic filter creation
	req := models.RetrieveRequest{
		IncludeExtensions: []string{"java", "go"},
		ExcludeExtensions: []string{"md"},
	}
	filter := NewFileFilterFromRequest(req)

	if len(filter.IncludeExtensions) != 2 {
		t.Errorf("expected 2 include extensions, got %d", len(filter.IncludeExtensions))
	}
	if len(filter.ExcludeExtensions) != 1 {
		t.Errorf("expected 1 exclude extension, got %d", len(filter.ExcludeExtensions))
	}

	// Test CodeOnly flag
	reqCodeOnly := models.RetrieveRequest{
		CodeOnly: true,
	}
	filterCodeOnly := NewFileFilterFromRequest(reqCodeOnly)

	if len(filterCodeOnly.ExcludeExtensions) < 5 {
		t.Errorf("CodeOnly should add multiple exclusions, got %d", len(filterCodeOnly.ExcludeExtensions))
	}
}

func TestCodeOnlyExclusions(t *testing.T) {
	// Verify common non-code extensions are excluded
	expectedExclusions := []string{"md", "txt", "json", "yaml"}

	for _, ext := range expectedExclusions {
		found := false
		for _, excluded := range CodeOnlyExclusions {
			if excluded == ext {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("CodeOnlyExclusions should contain %q", ext)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
