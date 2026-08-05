// Package symbols provides AST-based symbol extraction and Neo4j storage.
package symbols

import (
	"context"
	"fmt"
	"go/types"
	"log/slog"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

// GoTypesAnalyzer performs deep type analysis using go/types to discover
// Go's IMPLICIT interface satisfaction — which cannot be detected by tree-
// sitter query files because Go has no `implements` keyword; a struct
// implements an interface just by having the right method set. Only the Go
// type checker can compute the match.
//
// GO-IMPLEMENTS-001: this replaced a nil-returning stub. The Q3 audit
// verified zero Go IMPLEMENTS edges existed in a Go-dominant codebase; the
// structural RRF column + IMPLEMENTS edge-attention weight (0.70) were
// consumers waiting on data.
type GoTypesAnalyzer struct{}

// NewGoTypesAnalyzer creates a new go/types analyzer.
func NewGoTypesAnalyzer() *GoTypesAnalyzer {
	return &GoTypesAnalyzer{}
}

// AnalyzeImplements discovers interface implementations in a Go project via
// go/types.
//
// Args:
//   - spaceID: Neo4j space the returned records will land under
//   - projectRoot: filesystem path where `packages.Load` runs (typically the
//     git-repo root or module root)
//
// Returns one RelationshipRecord per (concrete → interface) pair, with
// symbol_ids computed via GenerateSymbolID(spaceID, filePath, name, 0) —
// matches the deterministic-hash scheme SymbolNode ingest uses, so
// SaveRelationships' MATCH clauses find the existing nodes.
//
// Filters:
//   - stdlib pairs (either side under a bare-name package like "fmt",
//     "io", "context") are skipped to prevent edge-count blow-up
//   - vendored deps under vendor/ or the module cache are skipped
//   - self-implements pairs (T → T interface) are skipped
//
// ResolutionMethod: "go_types" (Confidence 1.0 — go/types is authoritative,
// not a heuristic). Tier: 2 (matches sibling non-Go IMPLEMENTS tier).
func (a *GoTypesAnalyzer) AnalyzeImplements(ctx context.Context, spaceID, projectRoot string) ([]RelationshipRecord, error) {
	if projectRoot == "" {
		return nil, fmt.Errorf("projectRoot required")
	}

	cfg := &packages.Config{
		Context: ctx,
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedCompiledGoFiles |
			packages.NeedImports |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedSyntax |
			packages.NeedModule,
		Dir:   projectRoot,
		Tests: false, // don't chase _test packages — they'd double the load
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, fmt.Errorf("packages.Load: %w", err)
	}

	// go/packages reports per-package errors as diagnostics (e.g. missing
	// imports in a broken package). Log at Warn but continue — we still get
	// IMPLEMENTS data for the packages that DID load cleanly.
	loadErrors := 0
	for _, p := range pkgs {
		for _, e := range p.Errors {
			slog.Warn("go/packages diagnostic (continuing)",
				"pkg", p.PkgPath, "error", e.Error())
			loadErrors++
		}
	}
	slog.Info("go/packages loaded",
		"projectRoot", projectRoot,
		"packages", len(pkgs),
		"diagnostics", loadErrors)

	// Collect every interface and every concrete named type across all
	// packages, remembering which package they came from (for stdlib skip
	// and file-path resolution).
	type namedInfo struct {
		named    *types.Named
		pkg      *packages.Package
		filePath string
		name     string
	}
	var interfaces []namedInfo
	var concretes []namedInfo

	for _, p := range pkgs {
		if p.Types == nil {
			continue
		}
		// Skip stdlib packages (canonical stdlib import paths never
		// contain a "/").
		if isStdlib(p.PkgPath) {
			continue
		}
		for _, name := range p.Types.Scope().Names() {
			obj := p.Types.Scope().Lookup(name)
			if obj == nil {
				continue
			}
			tn, ok := obj.(*types.TypeName)
			if !ok {
				continue
			}
			named, ok := tn.Type().(*types.Named)
			if !ok {
				continue
			}
			filePath := ""
			if pos := p.Fset.Position(obj.Pos()); pos.IsValid() {
				filePath = pos.Filename
			}
			// Skip vendored / module-cache files.
			if isVendored(filePath) {
				continue
			}
			// GO-IMPLEMENTS-002 (2026-08-05): skip generated protobuf code.
			// The .pb.go / _grpc.pb.go files carry gRPC-wire interfaces
			// (*Server, Unsafe*Server) that describe RPC transport, not
			// domain semantics. Tree-sitter ingest already excludes them
			// from SymbolNode; the analyzer emitting IMPLEMENTS pairs whose
			// targets don't exist in the graph was the dominant 79-pair
			// class in the GO-IMPLEMENTS-002 gap audit.
			if isGeneratedProtobuf(filePath) {
				continue
			}
			info := namedInfo{named: named, pkg: p, filePath: filePath, name: name}
			if _, isIface := named.Underlying().(*types.Interface); isIface {
				interfaces = append(interfaces, info)
			} else {
				concretes = append(concretes, info)
			}
		}
	}

	slog.Info("go/types collected named types",
		"interfaces", len(interfaces),
		"concretes", len(concretes))

	// Compute IMPLEMENTS pairs.
	// - concrete T implements interface I when types.Implements(T, I) or
	//   types.Implements(*T, I) (pointer-receiver methods count).
	// - interface J embeds/refines I when types.Implements(J, I) and J != I.
	var out []RelationshipRecord
	seen := make(map[string]struct{}) // dedup by (source_id, target_id)

	emit := func(src namedInfo, dst namedInfo) {
		if src.filePath == "" || dst.filePath == "" {
			return
		}
		srcID := GenerateSymbolID(spaceID, relativizePath(src.filePath, projectRoot), src.name, 0)
		dstID := GenerateSymbolID(spaceID, relativizePath(dst.filePath, projectRoot), dst.name, 0)
		if srcID == dstID {
			return // self-implements
		}
		key := srcID + "->" + dstID
		if _, dup := seen[key]; dup {
			return
		}
		seen[key] = struct{}{}
		out = append(out, RelationshipRecord{
			SourceSymbolID:   srcID,
			TargetSymbolID:   dstID,
			Relation:         "IMPLEMENTS",
			SpaceID:          spaceID,
			Tier:             2,
			Confidence:       1.0,
			ResolutionMethod: "go_types",
			// GO-IMPLEMENTS-002 diagnostic breadcrumbs (not persisted).
			SourceName: src.name,
			SourcePath: relativizePath(src.filePath, projectRoot),
			TargetName: dst.name,
			TargetPath: relativizePath(dst.filePath, projectRoot),
		})
	}

	for _, iface := range interfaces {
		ifaceType := iface.named.Underlying().(*types.Interface)
		if ifaceType.NumMethods() == 0 {
			// Every type satisfies the empty interface — skipping keeps
			// edge count sane (would otherwise emit N*M edges for the
			// bare `any` case).
			continue
		}
		for _, c := range concretes {
			if types.Implements(c.named, ifaceType) ||
				types.Implements(types.NewPointer(c.named), ifaceType) {
				emit(c, iface)
			}
		}
		// Interface-embeds-interface: J implements I when J is narrower.
		for _, j := range interfaces {
			if j.named == iface.named {
				continue
			}
			jIface := j.named.Underlying().(*types.Interface)
			if jIface.NumMethods() < ifaceType.NumMethods() {
				continue // J can't satisfy I with fewer methods
			}
			if types.Implements(j.named, ifaceType) {
				emit(j, iface)
			}
		}
	}

	slog.Info("go/types IMPLEMENTS analysis complete",
		"pairs_emitted", len(out))
	return out, nil
}

// isStdlib returns true when pkgPath is a Go standard library package
// (canonical stdlib import paths never contain a "/" or "." — e.g. "fmt",
// "io", "encoding/json"; the "/" test alone misses second-level stdlib like
// "encoding/json", so we also check for a leading domain component).
func isStdlib(pkgPath string) bool {
	if pkgPath == "" {
		return false
	}
	// Third-party packages always have a domain-shaped first segment
	// ("github.com/…", "golang.org/…", "mdemg/…" when in-module).
	first := pkgPath
	if idx := strings.Index(pkgPath, "/"); idx >= 0 {
		first = pkgPath[:idx]
	}
	// Domain-shaped = contains a "." (github.com, golang.org, gopkg.in, …).
	// Standard library first segments never contain a ".".
	return !strings.Contains(first, ".") && !strings.Contains(first, "mdemg")
}

// isVendored returns true when filePath is under vendor/ or the Go module
// cache — noise-reduction; those pairs aren't part of the operator's
// project.
func isVendored(filePath string) bool {
	return strings.Contains(filePath, "/vendor/") ||
		strings.Contains(filePath, "/pkg/mod/") ||
		strings.Contains(filePath, "/go/pkg/mod/")
}

// isGeneratedProtobuf returns true for protobuf-generated Go files
// (`*.pb.go`, `*_grpc.pb.go`). Those files carry wire-format interfaces
// (like `*Server` / `Unsafe*Server`) that describe RPC transport, not
// domain semantics — the tree-sitter symbol ingest already excludes them
// from SymbolNode, so emitting IMPLEMENTS pairs against them would
// silently drop at SaveRelationships' MATCH. GO-IMPLEMENTS-002 (2026-08-05).
func isGeneratedProtobuf(filePath string) bool {
	return strings.HasSuffix(filePath, ".pb.go") ||
		strings.HasSuffix(filePath, "_grpc.pb.go")
}

// relativizePath returns filePath relative to projectRoot when possible; the
// tree-sitter ingest path stores relative paths in SymbolNode.file_path, so
// this MUST match to hit the same symbol_id hash.
func relativizePath(filePath, projectRoot string) string {
	if projectRoot == "" || filePath == "" {
		return filePath
	}
	abs, err := filepath.Abs(projectRoot)
	if err != nil {
		return filePath
	}
	rel, err := filepath.Rel(abs, filePath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filePath
	}
	// The tree-sitter ingest path stores paths with a leading "/" (e.g.
	// "/internal/symbols/go_types.go"). Match that shape.
	if !strings.HasPrefix(rel, "/") {
		rel = "/" + rel
	}
	return rel
}
