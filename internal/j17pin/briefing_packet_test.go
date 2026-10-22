// Package j17pin holds Go pin tests that enforce content contracts for J17
// protocol artifacts checked into the repo (docs/features/j17-*.md).
//
// Each pin test SHA-256-hashes a specific file + compares against an
// expected constant embedded in the test. A content edit MUST update the
// constant in the same commit; CI blocks drift. Same pattern as
// internal/grafanapin for Grafana dashboards.
//
// Why this exists: the J17 agent briefing packet is operator-facing
// contract delivered to peer frontier models at session-start. Content
// drift (someone edits the packet to say "you MUST follow Jiminy" and
// loses the v1.1 "data not commands" framing) silently reintroduces the
// authority-mismatch failure mode the peer-Opus-5.5 review surfaced.
// Hash-pinning the file at build time catches that drift at merge.
//
// UOTS note: the shipped UOTS framework (`docs/api/api-spec/uots/`)
// validates Grafana dashboards + JSON artifacts, not Markdown content.
// A Go pin test is the right-sized fit for static-doc content pinning —
// matches the grafanapin shape, zero new framework surface, zero runtime
// dependency.
package j17pin

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// packetFile returns the on-disk path to the J17 briefing packet,
// resolved from THIS test file's location. Keeps the pin test runnable
// regardless of the caller's cwd (matters for `go test ./...` from any
// subdirectory).
func packetFile(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed — cannot locate this test file")
	}
	// this file:  internal/j17pin/briefing_packet_test.go
	// target:     docs/features/j17-agent-briefing-packet.md
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Join(repoRoot, "docs", "features", "j17-agent-briefing-packet.md")
}

// Expected SHA-256 of docs/features/j17-agent-briefing-packet.md.
//
// Updating this constant REQUIRES updating the file + committing both in
// the same commit. The test's failure message prints the actual hash so
// a legitimate edit can copy-paste the new value verbatim.
//
// Current pin: v1.1 (2026-10-14) — reframed from v1.0's compliance-doc
// shape to evidence-with-provenance after peer-Opus-5.5 review.
const expectedBriefingPacketSHA256 = "5843d9d7acf61fb04a31fdc23f0860759bc82d74054b67650aee1a880ae70395"

// TestBriefingPacket_ContentHashPinned is the core drift guard. If this
// test fails because YOU edited the packet: copy the "actual" hash
// printed below into expectedBriefingPacketSHA256 and commit both
// files together. If the test fails and you did NOT edit the packet:
// something else did, and that's a real bug — investigate.
func TestBriefingPacket_ContentHashPinned(t *testing.T) {
	path := packetFile(t)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(b)
	actual := hex.EncodeToString(sum[:])
	if actual != expectedBriefingPacketSHA256 {
		t.Errorf("J17 briefing packet content changed — drift detected\n"+
			"  file:     %s\n"+
			"  expected: %s\n"+
			"  actual:   %s\n\n"+
			"If this edit was deliberate (bumping to v1.2+ etc.):\n"+
			"  1. open internal/j17pin/briefing_packet_test.go\n"+
			"  2. replace expectedBriefingPacketSHA256 with:\n"+
			"     %q\n"+
			"  3. commit both files together",
			path, expectedBriefingPacketSHA256, actual, actual)
	}
}

// TestBriefingPacket_StructuralInvariants protects the v1.1 framing
// that peer-Opus-5.5 review earned us. These phrases survive content
// edits that bump the hash — they encode the architectural posture, not
// the exact wording. Any edit that drops them is a framing regression
// and should fail CI even if the author updated the hash in good faith.
func TestBriefingPacket_StructuralInvariants(t *testing.T) {
	path := packetFile(t)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	content := string(b)

	// Required phrases — each encodes a v1.1 architectural invariant.
	// Comment names the invariant + the sprint/review that produced it.
	required := []struct {
		phrase string
		why    string
	}{
		{
			phrase: "data, not commands",
			why:    "data-not-commands opener (peer-Opus-5.5 reframe — hook-channel content is untrusted; MUST not be phrased as authority)",
		},
		{
			phrase: "Precedence",
			why:    "precedence section header (peer requirement #1 — explicit ranking of live-user vs CLAUDE.md vs packet vs guidance)",
		},
		{
			phrase: "Provenance",
			why:    "provenance contract header (peer requirement #2 — every src:NODE_ID resolvable)",
		},
		{
			phrase: "not_applicable",
			why:    "back-channel outcome type (peer requirement #4 — receiver can signal deliberate non-application; prevents follow-rate misread)",
		},
		{
			phrase: "absent,\nnot permissive",
			why:    "failure semantics one-liner (peer requirement #5 — absence of block ≠ approval)",
		},
	}

	for _, r := range required {
		if !containsAny(content, r.phrase) {
			t.Errorf("J17 briefing packet dropped required invariant\n"+
				"  phrase missing: %q\n"+
				"  why it matters: %s\n\n"+
				"If you intentionally removed this invariant, open a sprint plan "+
				"that justifies the v1.x bump + update this test's `required` list "+
				"in the same commit.",
				r.phrase, r.why)
		}
	}
}

// containsAny matches a phrase that may include a line break (writers
// often re-wrap paragraphs on edit). Checks both the literal phrase and
// a line-break-insensitive variant (collapses all \n in BOTH content
// and phrase to single spaces, then substring-matches).
func containsAny(content, phrase string) bool {
	if strings.Contains(content, phrase) {
		return true
	}
	flat := func(s string) string { return strings.ReplaceAll(s, "\n", " ") }
	return strings.Contains(flat(content), flat(phrase))
}
