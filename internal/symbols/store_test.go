package symbols

import (
	"testing"
)

func TestGenerateSymbolID(t *testing.T) {
	tests := []struct {
		name       string
		spaceID    string
		filePath   string
		symbolName string
		lineNumber int
	}{
		{
			name:       "basic constant",
			spaceID:    "test-space",
			filePath:   "src/storage.ts",
			symbolName: "DEFAULT_FLUSH_INTERVAL",
			lineNumber: 42,
		},
		{
			name:       "function symbol",
			spaceID:    "vscode-scale",
			filePath:   "src/vs/editor/cursor.ts",
			symbolName: "moveCursor",
			lineNumber: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id1 := GenerateSymbolID(tt.spaceID, tt.filePath, tt.symbolName, tt.lineNumber)
			id2 := GenerateSymbolID(tt.spaceID, tt.filePath, tt.symbolName, tt.lineNumber)

			// IDs should be deterministic
			if id1 != id2 {
				t.Errorf("GenerateSymbolID not deterministic: %s != %s", id1, id2)
			}

			// ID should be 32 characters (hex of 16 bytes)
			if len(id1) != 32 {
				t.Errorf("GenerateSymbolID length = %d, want 32", len(id1))
			}

			// Line number is excluded from hash — same symbol at different lines
			// produces the same ID (prevents duplicates on code reformatting)
			id3 := GenerateSymbolID(tt.spaceID, tt.filePath, tt.symbolName, tt.lineNumber+1)
			if id1 != id3 {
				t.Error("Different line numbers should produce the same ID")
			}

			// Different names should still produce different IDs
			id4 := GenerateSymbolID(tt.spaceID, tt.filePath, tt.symbolName+"_other", tt.lineNumber)
			if id1 == id4 {
				t.Error("Different symbol names should produce different IDs")
			}
		})
	}
}

func TestToRecord(t *testing.T) {
	sym := Symbol{
		Name:           "MAX_CURSOR_COUNT",
		Type:           SymbolTypeConst,
		Value:          "10000",
		RawValue:       "10000",
		FilePath:       "src/vs/editor/cursor.ts",
		Line:           25,
		LineEnd:        25,
		Column:         0,
		Exported:       true,
		DocComment:     "Maximum number of cursors",
		Language:       LangTypeScript,
		TypeAnnotation: "number",
	}

	record := ToRecord("test-space", sym)

	if record.SpaceID != "test-space" {
		t.Errorf("SpaceID = %s, want test-space", record.SpaceID)
	}
	if record.Name != sym.Name {
		t.Errorf("Name = %s, want %s", record.Name, sym.Name)
	}
	if record.SymbolType != string(sym.Type) {
		t.Errorf("SymbolType = %s, want %s", record.SymbolType, string(sym.Type))
	}
	if record.Value != sym.Value {
		t.Errorf("Value = %s, want %s", record.Value, sym.Value)
	}
	if record.FilePath != sym.FilePath {
		t.Errorf("FilePath = %s, want %s", record.FilePath, sym.FilePath)
	}
	if record.Line != sym.Line {
		t.Errorf("Line = %d, want %d", record.Line, sym.Line)
	}
	if record.Exported != sym.Exported {
		t.Errorf("Exported = %v, want %v", record.Exported, sym.Exported)
	}
	if record.Language != string(sym.Language) {
		t.Errorf("Language = %s, want %s", record.Language, string(sym.Language))
	}

	// SymbolID should be generated
	if record.SymbolID == "" {
		t.Error("SymbolID should not be empty")
	}
}

func TestNodePropertyHelpers(t *testing.T) {
	props := map[string]any{
		"name":        "testSymbol",
		"line_number": int64(42),
		"exported":    true,
	}

	if got := getString(props, "name"); got != "testSymbol" {
		t.Errorf("getString = %s, want testSymbol", got)
	}
	if got := getString(props, "missing"); got != "" {
		t.Errorf("getString for missing key = %s, want empty", got)
	}

	if got := getInt(props, "line_number"); got != 42 {
		t.Errorf("getInt = %d, want 42", got)
	}
	if got := getInt(props, "missing"); got != 0 {
		t.Errorf("getInt for missing key = %d, want 0", got)
	}

	if got := getBool(props, "exported"); !got {
		t.Error("getBool = false, want true")
	}
	if got := getBool(props, "missing"); got {
		t.Error("getBool for missing key = true, want false")
	}
}

func TestGetFloat32Slice(t *testing.T) {
	tests := []struct {
		name     string
		props    map[string]any
		key      string
		wantLen  int
		wantNil  bool
	}{
		{
			name: "float64 slice",
			props: map[string]any{
				"embedding": []float64{0.1, 0.2, 0.3},
			},
			key:     "embedding",
			wantLen: 3,
		},
		{
			name: "any slice",
			props: map[string]any{
				"embedding": []any{0.1, 0.2, 0.3, 0.4},
			},
			key:     "embedding",
			wantLen: 4,
		},
		{
			name:    "missing key",
			props:   map[string]any{},
			key:     "embedding",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getFloat32Slice(tt.props, tt.key)
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}
			if len(got) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}
