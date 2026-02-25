package guardrail

import (
	"strings"
	"testing"
)

// ---- parseDiff: Go function declarations ----

func TestParseDiff_GoFuncDeclarations(t *testing.T) {
	diff := `@@ -1,5 +1,7 @@
+func Foo(x int) error {
+func (r *Receiver) Bar(name string) bool {
 func existing() {}
`
	ctx := parseDiff(diff, nil)

	want := map[string]bool{"Foo": true, "Bar": true}
	for _, name := range ctx.FunctionNames {
		delete(want, name)
	}
	if len(want) != 0 {
		t.Errorf("missing Go function names: %v (got %v)", want, ctx.FunctionNames)
	}
}

func TestParseDiff_GoFuncDeclarations_Removed(t *testing.T) {
	// Removals (lines starting with -) should also be captured.
	diff := `-func OldHandler(w http.ResponseWriter, r *http.Request) {
`
	ctx := parseDiff(diff, nil)

	found := false
	for _, name := range ctx.FunctionNames {
		if name == "OldHandler" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected OldHandler in FunctionNames, got %v", ctx.FunctionNames)
	}
}

// ---- parseDiff: Python def extraction ----

func TestParseDiff_PythonDef(t *testing.T) {
	diff := `+def my_func(arg1, arg2):
+    pass
-def old_func():
`
	ctx := parseDiff(diff, nil)

	want := map[string]bool{"my_func": true, "old_func": true}
	for _, name := range ctx.FunctionNames {
		delete(want, name)
	}
	if len(want) != 0 {
		t.Errorf("missing Python function names: %v (got %v)", want, ctx.FunctionNames)
	}
}

// ---- parseDiff: JS/TS function/const extraction ----

func TestParseDiff_JSFunctions(t *testing.T) {
	diff := `+function foo(a, b) {
+const bar = (x) => x * 2;
+export function baz(name) {
`
	ctx := parseDiff(diff, nil)

	want := map[string]bool{"foo": true, "bar": true, "baz": true}
	for _, name := range ctx.FunctionNames {
		delete(want, name)
	}
	if len(want) != 0 {
		t.Errorf("missing JS/TS function names: %v (got %v)", want, ctx.FunctionNames)
	}
}

func TestParseDiff_JSConstLetVar(t *testing.T) {
	diff := `+const myConst = 42;
+let myLet = "hello";
+var myVar = true;
`
	ctx := parseDiff(diff, nil)

	want := map[string]bool{"myConst": true, "myLet": true, "myVar": true}
	for _, name := range ctx.FunctionNames {
		delete(want, name)
	}
	if len(want) != 0 {
		t.Errorf("missing JS identifier names: %v (got %v)", want, ctx.FunctionNames)
	}
}

// ---- parseDiff: Go import path extraction ----

func TestParseDiff_GoImportPaths(t *testing.T) {
	diff := `+import (
+	"path/to/pkg"
+	"fmt"
+)
`
	ctx := parseDiff(diff, nil)

	want := map[string]bool{"path/to/pkg": true, "fmt": true}
	for _, imp := range ctx.ImportPaths {
		delete(want, imp)
	}
	if len(want) != 0 {
		t.Errorf("missing Go import paths: %v (got %v)", want, ctx.ImportPaths)
	}
}

// ---- parseDiff: Python import extraction ----

func TestParseDiff_PythonImports(t *testing.T) {
	diff := `+import os
+from pathlib import Path
`
	ctx := parseDiff(diff, nil)

	want := map[string]bool{"os": true, "pathlib": true}
	for _, imp := range ctx.ImportPaths {
		delete(want, imp)
	}
	if len(want) != 0 {
		t.Errorf("missing Python imports: %v (got %v)", want, ctx.ImportPaths)
	}
}

// ---- parseDiff: JS import extraction ----

func TestParseDiff_JSImports(t *testing.T) {
	// jsImportRe matches: import ... from 'pkg'  AND  require('pkg') at line start.
	// Note: require() must appear at the beginning of the diff line (after +/-) — if
	// preceded by "const z = " it will not match the regex pattern.
	diff := `+import x from 'pkg'
+import y from "other-pkg"
+require('require-pkg')
`
	ctx := parseDiff(diff, nil)

	want := map[string]bool{"pkg": true, "other-pkg": true, "require-pkg": true}
	for _, imp := range ctx.ImportPaths {
		delete(want, imp)
	}
	if len(want) != 0 {
		t.Errorf("missing JS imports: %v (got %v)", want, ctx.ImportPaths)
	}
}

// ---- parseDiff: Added/removed line extraction ----

func TestParseDiff_AddedRemovedLines(t *testing.T) {
	diff := `--- a/file.go
+++ b/file.go
@@ -1,3 +1,4 @@
 unchanged line
+added line one
+added line two
-removed line
`
	ctx := parseDiff(diff, nil)

	// +++ and --- headers must NOT appear in added/removed lines.
	for _, line := range ctx.AddedLines {
		if strings.HasPrefix(line, "++") || strings.HasPrefix(line, "--") {
			t.Errorf("diff header leaked into lines: %q", line)
		}
	}
	for _, line := range ctx.RemovedLines {
		if strings.HasPrefix(line, "++") || strings.HasPrefix(line, "--") {
			t.Errorf("diff header leaked into lines: %q", line)
		}
	}

	wantAdded := 2
	if len(ctx.AddedLines) != wantAdded {
		t.Errorf("expected %d added lines, got %d: %v", wantAdded, len(ctx.AddedLines), ctx.AddedLines)
	}

	wantRemoved := 1
	if len(ctx.RemovedLines) != wantRemoved {
		t.Errorf("expected %d removed lines, got %d: %v", wantRemoved, len(ctx.RemovedLines), ctx.RemovedLines)
	}

	if ctx.AddedLines[0] != "added line one" {
		t.Errorf("unexpected added line: %q", ctx.AddedLines[0])
	}
	if ctx.RemovedLines[0] != "removed line" {
		t.Errorf("unexpected removed line: %q", ctx.RemovedLines[0])
	}
}

// ---- parseDiff: Hunk header context extraction ----

func TestParseDiff_HunkHeaderContext(t *testing.T) {
	diff := `@@ -1,5 +1,5 @@ func Login()
+    token := generateToken()
`
	ctx := parseDiff(diff, nil)

	found := false
	for _, kw := range ctx.Keywords {
		if kw == "func Login()" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected hunk context 'func Login()' in Keywords, got %v", ctx.Keywords)
	}
}

func TestParseDiff_HunkHeaderContextEmpty(t *testing.T) {
	// Hunk with no trailing context — should not add an empty keyword.
	diff := `@@ -1,3 +1,3 @@
+new line
`
	ctx := parseDiff(diff, nil)

	for _, kw := range ctx.Keywords {
		if kw == "" {
			t.Errorf("empty keyword added from hunk header: %v", ctx.Keywords)
		}
	}
}

// ---- parseDiff: Empty diff ----

func TestParseDiff_EmptyDiff(t *testing.T) {
	ctx := parseDiff("", nil)

	if len(ctx.AddedLines) != 0 {
		t.Errorf("expected empty AddedLines, got %v", ctx.AddedLines)
	}
	if len(ctx.RemovedLines) != 0 {
		t.Errorf("expected empty RemovedLines, got %v", ctx.RemovedLines)
	}
	if len(ctx.FunctionNames) != 0 {
		t.Errorf("expected empty FunctionNames, got %v", ctx.FunctionNames)
	}
	if len(ctx.ImportPaths) != 0 {
		t.Errorf("expected empty ImportPaths, got %v", ctx.ImportPaths)
	}
	if len(ctx.Keywords) != 0 {
		t.Errorf("expected empty Keywords, got %v", ctx.Keywords)
	}
	if ctx.Summary != "" {
		t.Errorf("expected empty Summary, got %q", ctx.Summary)
	}
}

func TestParseDiff_EmptyDiff_NonNilSlices(t *testing.T) {
	// Slices must be non-nil (JSON-serialisable as []).
	ctx := parseDiff("", nil)

	if ctx.AddedLines == nil {
		t.Error("AddedLines must not be nil")
	}
	if ctx.RemovedLines == nil {
		t.Error("RemovedLines must not be nil")
	}
	if ctx.FunctionNames == nil {
		t.Error("FunctionNames must not be nil")
	}
	if ctx.ImportPaths == nil {
		t.Error("ImportPaths must not be nil")
	}
	if ctx.Keywords == nil {
		t.Error("Keywords must not be nil")
	}
}

// ---- parseDiff: Summary truncation ----

func TestParseDiff_SummaryTruncation(t *testing.T) {
	// Build a diff with many added lines so the summary exceeds 2000 chars.
	// buildDiffSummary only uses the first 20 added lines, so each must be long enough
	// that 20 lines × (len + overhead) > 2000. Using 100-char lines:
	// 20 × (100 + 4 [prefix "  + "] + 1 [newline]) = 20 × 105 = 2100 chars,
	// plus the "Added:\n" header → well over 2000.
	var sb strings.Builder
	sb.WriteString("@@ -1,1 +1,100 @@\n")
	longLine := strings.Repeat("x", 100)
	for i := 0; i < 30; i++ {
		sb.WriteString("+" + longLine + "\n")
	}

	ctx := parseDiff(sb.String(), nil)

	if len(ctx.Summary) > 2000 {
		t.Errorf("Summary not truncated: len=%d (want <= 2000)", len(ctx.Summary))
	}
	if !strings.HasSuffix(ctx.Summary, "...") {
		t.Errorf("truncated Summary should end with '...', got suffix: %q (full len=%d)",
			ctx.Summary[max(0, len(ctx.Summary)-6):], len(ctx.Summary))
	}
}

// helper used by test — max of two ints (Go 1.21+ has builtin; supply locally for safety)
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ---- parseDiff: File paths added to keywords ----

func TestParseDiff_FilePathsInKeywords(t *testing.T) {
	// File-path basenames are appended to Keywords only when the diff is non-empty
	// (the early-return on empty diff skips that code path). Provide a minimal diff.
	files := []string{"internal/auth/handler.go", "pkg/util/helper.py"}
	ctx := parseDiff("+// change\n", files)

	want := map[string]bool{"handler.go": true, "helper.py": true}
	for _, kw := range ctx.Keywords {
		delete(want, kw)
	}
	if len(want) != 0 {
		t.Errorf("file basename keywords not found: %v (got %v)", want, ctx.Keywords)
	}
}

func TestParseDiff_FilePathsInFilePaths(t *testing.T) {
	files := []string{"cmd/main.go", "Makefile"}
	ctx := parseDiff("", files)

	if len(ctx.FilePaths) != 2 {
		t.Errorf("expected 2 FilePaths, got %d: %v", len(ctx.FilePaths), ctx.FilePaths)
	}
}

// ---- parseDiff: Deduplication of function names ----

func TestParseDiff_FunctionNameDeduplication(t *testing.T) {
	// The same function appears on both an added and a removed line.
	diff := `+func Duplicate(x int) {}
-func Duplicate(x string) {}
`
	ctx := parseDiff(diff, nil)

	count := 0
	for _, name := range ctx.FunctionNames {
		if name == "Duplicate" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected Duplicate to appear exactly once, got %d: %v", count, ctx.FunctionNames)
	}
}

// ---- parseDiff: Import path deduplication ----

func TestParseDiff_ImportPathDeduplication(t *testing.T) {
	diff := `+"fmt"
+"fmt"
`
	ctx := parseDiff(diff, nil)

	count := 0
	for _, imp := range ctx.ImportPaths {
		if imp == "fmt" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 'fmt' to appear exactly once in ImportPaths, got %d: %v", count, ctx.ImportPaths)
	}
}

// ---- parseDiff: Mixed language diff ----

func TestParseDiff_MixedLanguages(t *testing.T) {
	diff := `+func GoFunc(x int) {}
+def py_func():
+function jsFunc() {}
`
	ctx := parseDiff(diff, nil)

	want := map[string]bool{"GoFunc": true, "py_func": true, "jsFunc": true}
	for _, name := range ctx.FunctionNames {
		delete(want, name)
	}
	if len(want) != 0 {
		t.Errorf("missing function names: %v (got %v)", want, ctx.FunctionNames)
	}
}
