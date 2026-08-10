// PLUGIN-PATH-INJECTION-FIX (2026-08-10): pin tests for the two guard
// functions added in plugin_handlers.go — validatePluginID and
// verifyPluginBinaryInside. Closes CodeQL alert #22 class
// (go/command-injection / path-injection) at the HTTP handler layer.
//
// Contract asserted:
//   1. validatePluginID rejects: empty, path separators, dots (incl. "."
//      and ".."), leading/embedded traversal segments, over-length, and
//      shell metacharacters.
//   2. validatePluginID accepts: alnum + underscore + hyphen up to 64.
//   3. verifyPluginBinaryInside rejects an absolute path outside the
//      configured plugins directory (the actual class the CodeQL alert
//      flagged — a manifest that names an out-of-tree binary).
//   4. verifyPluginBinaryInside accepts a path inside the plugins dir.

package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidatePluginID_RejectsDangerous(t *testing.T) {
	cases := []struct {
		name, in string
	}{
		{"empty", ""},
		{"dot", "."},
		{"double_dot", ".."},
		{"traversal_prefix", "../etc/passwd"},
		{"traversal_embedded", "safe/../etc/passwd"},
		{"absolute_unix", "/etc/passwd"},
		{"backslash", "safe\\..\\etc"},
		{"url_encoded_dot", "..%2Fetc"},
		{"contains_dot_in_name", "my.plugin"},
		{"space_char", "my plugin"},
		{"shell_meta_semi", "safe;rm"},
		{"shell_meta_amp", "safe&&rm"},
		{"shell_meta_pipe", "safe|rm"},
		{"shell_meta_dollar", "safe$IFS"},
		{"shell_meta_backtick", "safe`id`"},
		{"newline", "safe\n"},
		{"null_byte", "safe\x00"},
		{"unicode_lookalike_slash", "safe⁄etc"},
		{"over_length", strings.Repeat("a", 65)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validatePluginID(tc.in); err == nil {
				t.Fatalf("validatePluginID(%q) unexpectedly PASSED — pluginID guard would let this reach filepath.Join(PluginsDir, id) and enable path traversal", tc.in)
			}
		})
	}
}

func TestValidatePluginID_AcceptsLegit(t *testing.T) {
	cases := []string{
		"myplugin",
		"my_plugin",
		"my-plugin",
		"MyPlugin",
		"plugin1",
		"a",
		strings.Repeat("a", 64),
		"ingestion-worker-v2",
		"UPPER_snake_and-hyphens_42",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			if err := validatePluginID(in); err != nil {
				t.Fatalf("validatePluginID(%q) unexpectedly REJECTED legitimate id: %v", in, err)
			}
		})
	}
}

func TestVerifyPluginBinaryInside(t *testing.T) {
	// Build a mock plugins dir + a mock binary inside it.
	pluginsDir := t.TempDir()
	inside := filepath.Join(pluginsDir, "myplugin", "bin", "plugin-binary")
	if err := os.MkdirAll(filepath.Dir(inside), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(inside, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}

	// A sibling directory OUTSIDE the plugins dir — simulates a manifest
	// naming e.g. /etc/passwd or /tmp/attacker/binary.
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "attacker-binary")
	if err := os.WriteFile(outside, []byte("x"), 0o755); err != nil {
		t.Fatalf("write outside: %v", err)
	}

	t.Run("accepts_inside_binary", func(t *testing.T) {
		if err := verifyPluginBinaryInside(pluginsDir, inside); err != nil {
			t.Fatalf("verifyPluginBinaryInside rejected LEGIT inside binary: %v", err)
		}
	})

	t.Run("rejects_outside_binary", func(t *testing.T) {
		if err := verifyPluginBinaryInside(pluginsDir, outside); err == nil {
			t.Fatalf("verifyPluginBinaryInside PASSED an outside path — would allow validator.go to exec an arbitrary binary")
		}
	})

	t.Run("rejects_traversal_to_outside", func(t *testing.T) {
		// A path that lexically starts with pluginsDir but resolves via
		// .. to outside — the reason we need filepath.Abs, not just
		// strings.HasPrefix on the raw input.
		trav := filepath.Join(pluginsDir, "..", filepath.Base(outsideDir), "attacker-binary")
		if err := verifyPluginBinaryInside(pluginsDir, trav); err == nil {
			t.Fatalf("verifyPluginBinaryInside PASSED a ..-traversal path resolving outside: %s", trav)
		}
	})
}
