package scraper

import "testing"

// --- getStr tests ---

func TestGetStr_Present(t *testing.T) {
	props := map[string]any{"name": "hello"}
	if got := getStr(props, "name"); got != "hello" {
		t.Errorf("getStr = %q, want %q", got, "hello")
	}
}

func TestGetStr_Missing(t *testing.T) {
	props := map[string]any{}
	if got := getStr(props, "name"); got != "" {
		t.Errorf("getStr = %q, want empty", got)
	}
}

func TestGetStr_NonString(t *testing.T) {
	props := map[string]any{"count": 42}
	if got := getStr(props, "count"); got != "" {
		t.Errorf("getStr(int) = %q, want empty", got)
	}
}

func TestGetStr_NilValue(t *testing.T) {
	props := map[string]any{"key": nil}
	if got := getStr(props, "key"); got != "" {
		t.Errorf("getStr(nil) = %q, want empty", got)
	}
}

// --- getInt tests ---

func TestGetInt_Int64(t *testing.T) {
	props := map[string]any{"count": int64(42)}
	if got := getInt(props, "count"); got != 42 {
		t.Errorf("getInt(int64) = %d, want 42", got)
	}
}

func TestGetInt_Int(t *testing.T) {
	props := map[string]any{"count": 7}
	if got := getInt(props, "count"); got != 7 {
		t.Errorf("getInt(int) = %d, want 7", got)
	}
}

func TestGetInt_Float64(t *testing.T) {
	props := map[string]any{"count": float64(3.9)}
	if got := getInt(props, "count"); got != 3 {
		t.Errorf("getInt(float64) = %d, want 3", got)
	}
}

func TestGetInt_Missing(t *testing.T) {
	props := map[string]any{}
	if got := getInt(props, "count"); got != 0 {
		t.Errorf("getInt(missing) = %d, want 0", got)
	}
}

func TestGetInt_StringType(t *testing.T) {
	props := map[string]any{"count": "not a number"}
	if got := getInt(props, "count"); got != 0 {
		t.Errorf("getInt(string) = %d, want 0", got)
	}
}

// --- getFloat tests ---

func TestGetFloat_Float64(t *testing.T) {
	props := map[string]any{"score": float64(0.95)}
	if got := getFloat(props, "score"); got != 0.95 {
		t.Errorf("getFloat(float64) = %f, want 0.95", got)
	}
}

func TestGetFloat_Int64(t *testing.T) {
	props := map[string]any{"score": int64(5)}
	if got := getFloat(props, "score"); got != 5.0 {
		t.Errorf("getFloat(int64) = %f, want 5.0", got)
	}
}

func TestGetFloat_Missing(t *testing.T) {
	props := map[string]any{}
	if got := getFloat(props, "score"); got != 0 {
		t.Errorf("getFloat(missing) = %f, want 0.0", got)
	}
}

func TestGetFloat_StringType(t *testing.T) {
	props := map[string]any{"score": "high"}
	if got := getFloat(props, "score"); got != 0 {
		t.Errorf("getFloat(string) = %f, want 0.0", got)
	}
}

func TestGetFloat_NilValue(t *testing.T) {
	props := map[string]any{"score": nil}
	if got := getFloat(props, "score"); got != 0 {
		t.Errorf("getFloat(nil) = %f, want 0.0", got)
	}
}
