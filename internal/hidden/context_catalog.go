// Package hidden — Phase 14.2 Note 05 sparse fingerprints / context catalog.
//
// A ContextCatalog represents a per-space, versioned bit-allocation table that
// maps observation features (paths, tags, role_type×layer tuples, symbols) to
// fixed positions in a 256-bit sparse fingerprint. Each MemoryNode observation
// records a `context_fingerprint_active []uint16` (sparse indices of set bits)
// + `context_fingerprint_version int` (which catalog version produced it).
//
// Two-phase fingerprint design (operator-approved 2026-05-05):
//   Phase A (observe-time): Service.Observe computes fingerprint from
//     observation-local features only — path, role_type, layer, top-N tags.
//     No co-activation needed. Cold-start works (every obs has *some* fingerprint).
//   Phase B (post-hoc refresh): CycleOrchestrator.RunCycle stage 6 walks
//     observations whose version < current_catalog.version, queries Neo4j
//     CO_ACTIVATED_WITH edges, and refines fingerprints with symbol bits.
//     Mature data gets HTM-faithful fingerprints after the first weekly refresh.
//
// Adaptive Builder (Phase 14.2 Epic 0 forensic):
//   Note 05 spec's static 64/64/64/64 split wastes 128 bits on every
//   production space surveyed (whk-wms, mdemg-dev, linear, ubts-load all have
//   0 distinct symbols + 0 distinct roles). This package's Builder measures
//   per-space density at refresh time and allocates bits proportionally:
//     - 32 bits → top-32 (role_type, layer) tuples  (always informative)
//     - up to 32 bits → top-N tags (or 0 if space has none)
//     - 192 bits (or more if tags=0) → distributed across (paths, symbols)
//       by log(1+distinct_count) with floor 16 each when both present
//
// Cycle-safety: this package does NOT import internal/conversation — fingerprint
// computation lives in internal/conversation/fingerprint.go and uses Catalog
// methods from here.
package hidden

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// BitKind enumerates the kinds of features a catalog bit can represent.
// Mirrors the `kind` field on `ContextCatalog.bits[]` in Neo4j (V0026).
type BitKind string

const (
	BitKindRoleTypeLayer BitKind = "role_type_layer" // tuple of (role_type, layer)
	BitKindTag           BitKind = "tag"             // tag string
	BitKindPath          BitKind = "path"            // file_path string
	BitKindSymbol        BitKind = "symbol"          // symbol node identifier
)

// BitEntry describes one position in the catalog's 256-bit allocation.
// Stored as part of `ContextCatalog.bits[]` in Neo4j and as JSON in
// TSDB V0020 `context_catalog_versions.allocation_json`.
type BitEntry struct {
	Position uint16  `json:"position"` // 0..255
	Kind     BitKind `json:"kind"`
	Ref      string  `json:"ref"`   // canonical identifier (path string, tag value, "(role_type,layer)" tuple, symbol node_id)
	Token    string  `json:"token"` // human-readable label for explainability
}

// Catalog is an immutable in-memory snapshot of a single ContextCatalog
// version for one space_id. Held by Service.Observe via LoadActive() and
// reused across many observations until a new version is published. The
// reverse-lookup maps below are computed once at Load and accessed under
// RLock during fingerprint computation.
type Catalog struct {
	SpaceID   string
	Version   int
	TotalBits uint16     // typically 256 (CONTEXT_FINGERPRINT_BIT_BUDGET default)
	Bits      []BitEntry // length == TotalBits

	// Reverse lookup tables, populated at Load time. Read-only after that.
	pathToBit      map[string]uint16
	tagToBit       map[string]uint16
	roleTypeLayerToBit map[string]uint16 // key = "<role_type>|<layer>"
	symbolToBit    map[string]uint16

	mu sync.RWMutex // protects no mutation post-Load; reserved for future hot-reload
}

// SymbolBit returns the bit position assigned to nodeID, if the catalog has
// allocated symbol bits and this nodeID is among the top-N most-frequent
// symbols at catalog build time. Returns (0, false) when no symbol bits are
// allocated (current state for all surveyed production spaces).
func (c *Catalog) SymbolBit(nodeID string) (uint16, bool) {
	if c == nil {
		return 0, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	bit, ok := c.symbolToBit[nodeID]
	return bit, ok
}

// PathBit returns the bit position assigned to path, if the catalog has a
// matching path entry.
func (c *Catalog) PathBit(path string) (uint16, bool) {
	if c == nil || path == "" {
		return 0, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	bit, ok := c.pathToBit[path]
	return bit, ok
}

// RoleBit is reserved for future use (no surveyed space has populated `role`).
// Currently always returns (0, false). Kept on the interface so a future
// space with role data doesn't require an API change.
func (c *Catalog) RoleBit(role string) (uint16, bool) {
	return 0, false
}

// RoleTypeLayerBit returns the bit position assigned to the (roleType, layer)
// tuple, if the catalog has a matching entry. The first 32 bits are always
// allocated to role_type×layer per the adaptive Builder algorithm.
func (c *Catalog) RoleTypeLayerBit(roleType string, layer int) (uint16, bool) {
	if c == nil || roleType == "" {
		return 0, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	key := fmt.Sprintf("%s|%d", roleType, layer)
	bit, ok := c.roleTypeLayerToBit[key]
	return bit, ok
}

// TagBit returns the bit position assigned to a tag string, if the catalog
// has allocated tag bits and this tag is among the top-N most-frequent.
func (c *Catalog) TagBit(tag string) (uint16, bool) {
	if c == nil || tag == "" {
		return 0, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	bit, ok := c.tagToBit[tag]
	return bit, ok
}

// LayerBit is a thin wrapper: layer-only bits are not allocated independently
// (they live within RoleTypeLayer tuples). Returns (0, false). Reserved on
// the API in case future Builder versions split layer out.
func (c *Catalog) LayerBit(layer int) (uint16, bool) {
	return 0, false
}

// NewCatalog constructs a Catalog from a sorted bits[] array. The input must
// have len == totalBits and unique Position values 0..totalBits-1. Builds
// the reverse-lookup maps eagerly — single-threaded, called once per Load.
func NewCatalog(spaceID string, version int, totalBits uint16, bits []BitEntry) (*Catalog, error) {
	if spaceID == "" {
		return nil, errors.New("hidden.NewCatalog: empty space_id")
	}
	if uint16(len(bits)) != totalBits {
		return nil, fmt.Errorf("hidden.NewCatalog: len(bits)=%d != totalBits=%d", len(bits), totalBits)
	}
	c := &Catalog{
		SpaceID:            spaceID,
		Version:            version,
		TotalBits:          totalBits,
		Bits:               bits,
		pathToBit:          make(map[string]uint16),
		tagToBit:           make(map[string]uint16),
		roleTypeLayerToBit: make(map[string]uint16),
		symbolToBit:        make(map[string]uint16),
	}
	for _, b := range bits {
		switch b.Kind {
		case BitKindPath:
			c.pathToBit[b.Ref] = b.Position
		case BitKindTag:
			c.tagToBit[b.Ref] = b.Position
		case BitKindRoleTypeLayer:
			// Ref format: "<role_type>|<layer>" (Builder writes it this way)
			c.roleTypeLayerToBit[b.Ref] = b.Position
		case BitKindSymbol:
			c.symbolToBit[b.Ref] = b.Position
		default:
			return nil, fmt.Errorf("hidden.NewCatalog: unknown BitKind %q at position %d", b.Kind, b.Position)
		}
	}
	return c, nil
}

// CatalogLoader abstracts the source of catalog data. The production loader
// reads from Neo4j (ContextCatalog node label, V0026 schema). Tests use a
// fixture-based loader. Caller of Service.Observe holds a CatalogLoader and
// invokes LoadActive once per (space_id, mdemg-restart) pair.
type CatalogLoader interface {
	// LoadActive returns the current is_active=true catalog for spaceID.
	// Returns (nil, nil) when no catalog exists yet (cold-start) — caller
	// must handle by setting empty fingerprint per Note 05 §2.4 fallback.
	LoadActive(ctx context.Context, spaceID string) (*Catalog, error)

	// LoadVersion returns a specific historical catalog version for spaceID.
	// Used for cross-version fingerprint comparison fallback.
	LoadVersion(ctx context.Context, spaceID string, version int) (*Catalog, error)
}

// Builder constructs a new Catalog version from per-space density data.
// Implementations live elsewhere (Epic 2): the production Builder queries
// Neo4j for distinct counts + top-N feature lists; tests use fixture data.
type Builder interface {
	// BuildForSpace measures density for spaceID, allocates 256 bits per
	// the adaptive algorithm, and returns the new Catalog. Implementation
	// must be deterministic (same input → same bit assignments) and
	// idempotent (re-running on stable data is a no-op at the bit level).
	//
	// The persistence step (writing the new ContextCatalog node + marking
	// the previous active version is_active=false) is the Builder's
	// responsibility to keep the (space_id, is_active=true) invariant
	// atomic.
	BuildForSpace(ctx context.Context, spaceID string) (*Catalog, error)
}

// SortBitsByPosition sorts a slice of BitEntry in place by Position ascending.
// Builder implementations call this before persistence to ensure stable
// on-disk representation.
func SortBitsByPosition(bits []BitEntry) {
	sort.SliceStable(bits, func(i, j int) bool {
		return bits[i].Position < bits[j].Position
	})
}
