// Phase 14.2 Epic 2 — Adaptive Catalog Builder + Neo4j-backed Loader.
//
// The production Builder queries Neo4j for per-space feature density
// (paths, role_type×layer tuples, top-N tags), allocates 256 bits per the
// adaptive algorithm from Phase 14.2 Epic 0 forensic, then persists a new
// ContextCatalog node (atomically marking the previous active version
// is_active=false). Determinism + idempotence: same input → same bit
// assignments via stable sort + lexicographic tie-break.
//
// Symbols intentionally allocate 0 bits at observe time — Phase 14 + 14.2
// Epic 0 forensic confirmed no production space has populated `symbol`.
// Symbol bits will be added by the Phase 14.2 Epic 3 post-hoc refresh
// when CO_ACTIVATED_WITH edges are walked, bumping the catalog version.
//
// Cycle-safety: this file imports neo4j-go-driver/v5 (consistent with
// internal/hidden/constraint_conflicts.go) and internal/config. It does
// NOT import internal/conversation or internal/retrieval — those depend on
// us via the Catalog/Builder interfaces in context_catalog.go.
package hidden

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"mdemg/internal/config"
)

// BuilderOpts carries the per-call allocation knobs (mirrors Config fields
// to keep this package free of Config-shape coupling beyond the explicit
// constructor wiring).
type BuilderOpts struct {
	BitBudget            int // total bits per fingerprint
	TopNPaths            int // cap on path bits
	TopNTags             int // cap on tag bits
	FloorBitsPerKind     int // minimum bits for any kind with ≥10 distinct values
	RoleTypeLayerBits    int // reserved bits for top-N (role_type, layer) tuples
}

// BuilderOptsFromConfig translates Config into the package-local BuilderOpts.
func BuilderOptsFromConfig(cfg config.Config) BuilderOpts {
	return BuilderOpts{
		BitBudget:         cfg.ContextFingerprintBitBudget,
		TopNPaths:         cfg.ContextCatalogTopNPaths,
		TopNTags:          cfg.ContextCatalogTopNTags,
		FloorBitsPerKind:  cfg.ContextCatalogFloorBitsPerKind,
		RoleTypeLayerBits: cfg.ContextCatalogRoleTypeLayerBits,
	}
}

// neo4jBuilder is the production Builder backed by a Neo4j driver.
type neo4jBuilder struct {
	driver neo4j.DriverWithContext
	opts   BuilderOpts
}

// NewNeo4jBuilder constructs a production Builder bound to a Neo4j driver
// + the BuilderOpts derived from Config.
func NewNeo4jBuilder(driver neo4j.DriverWithContext, opts BuilderOpts) Builder {
	return &neo4jBuilder{driver: driver, opts: opts}
}

// densityResult holds the raw per-kind data the Builder needs to allocate bits.
// Populated by collectDensity; read by allocateBits.
type densityResult struct {
	// Top paths by frequency (stable order: count desc, then path asc).
	topPaths []countedRef
	// Top tags by frequency.
	topTags []countedRef
	// Top (role_type, layer) tuples by frequency.
	topRoleTypeLayer []countedRef
	// Distinct counts (used by the floor + log-density allocator).
	distinctPaths    int
	distinctTags     int
	distinctSymbols  int
}

type countedRef struct {
	Ref   string // canonical key (path string, tag, "<role_type>|<layer>")
	Token string // human-readable label
	Count int64
}

// BuildForSpace measures density for spaceID, allocates bits per the
// adaptive algorithm, persists the new ContextCatalog node atomically, and
// returns the new in-memory Catalog. Implements the Builder interface.
func (b *neo4jBuilder) BuildForSpace(ctx context.Context, spaceID string) (*Catalog, error) {
	if spaceID == "" {
		return nil, fmt.Errorf("hidden.BuildForSpace: empty space_id")
	}
	if b.opts.BitBudget < 32 {
		return nil, fmt.Errorf("hidden.BuildForSpace: bit_budget too small (%d)", b.opts.BitBudget)
	}

	density, err := b.collectDensity(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("collect density for %q: %w", spaceID, err)
	}

	bits := b.allocateBits(density)
	if uint16(len(bits)) != uint16(b.opts.BitBudget) {
		return nil, fmt.Errorf("allocator produced %d bits, want %d", len(bits), b.opts.BitBudget)
	}

	// Determine the new version: previous active version + 1, or 1 if cold-start.
	prevVersion, err := b.activeVersionFor(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("look up active version for %q: %w", spaceID, err)
	}
	newVersion := prevVersion + 1

	// Persist atomically: create new ContextCatalog with is_active=true,
	// mark all other versions for this space is_active=false.
	if err := b.persistCatalog(ctx, spaceID, newVersion, bits); err != nil {
		return nil, fmt.Errorf("persist catalog for %q v%d: %w", spaceID, newVersion, err)
	}

	cat, err := NewCatalog(spaceID, newVersion, uint16(b.opts.BitBudget), bits)
	if err != nil {
		return nil, fmt.Errorf("construct in-memory catalog: %w", err)
	}
	return cat, nil
}

// collectDensity runs four Cypher queries against Neo4j to gather feature
// density for the space. Each query is read-only, idempotent, and uses
// indexed lookups where available.
func (b *neo4jBuilder) collectDensity(ctx context.Context, spaceID string) (*densityResult, error) {
	sess := b.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	out := &densityResult{}

	// Query 1: top paths by frequency.
	queryTopPaths := `
MATCH (m:MemoryNode {space_id: $space_id})
WHERE m.path IS NOT NULL
WITH m.path AS path, count(*) AS freq
ORDER BY freq DESC, path ASC
LIMIT $limit
RETURN path, freq`
	if _, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, queryTopPaths, map[string]any{
			"space_id": spaceID,
			"limit":    int64(b.opts.TopNPaths),
		})
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			rec := res.Record()
			path, _ := rec.Get("path")
			freq, _ := rec.Get("freq")
			pathStr, _ := path.(string)
			freqInt, _ := freq.(int64)
			if pathStr == "" {
				continue
			}
			out.topPaths = append(out.topPaths, countedRef{Ref: pathStr, Token: pathStr, Count: freqInt})
		}
		return nil, res.Err()
	}); err != nil {
		return nil, fmt.Errorf("top paths: %w", err)
	}

	// Query 2: top "tags" — Phase 14.2.2 retune.
	//
	// Phase 14.2 collected from m.tags (LLM-summary buckets like `api`,
	// `architecture`, `caching`). Those buckets have low discrimination
	// against typical domain queries — vector + token derivation both
	// produced near-empty fingerprints in the 14.2 / 14.2.1 A/B runs.
	//
	// Phase 14.2.2 retune: collect top-N path segments (split paths on
	// '/'). Path segments carry domain vocabulary directly (e.g.
	// `barrelOwnership`, `auth`, `inventory-upload`) so query embeddings
	// land on the right bits. We filter out "everywhere" segments
	// (frequency > total_obs/2) since they contribute zero discrimination
	// and inflate the Jaccard noise floor.
	//
	// The bits keep BitKindTag (no schema migration) — semantically the
	// kind name now reads as "discriminative-token" rather than
	// "user-defined tag", and that's accurate for the vector-cosine
	// matching path which is the production use.
	queryTopTags := `
MATCH (m:MemoryNode {space_id: $space_id})
WHERE m.path IS NOT NULL
WITH count(DISTINCT m) AS total
MATCH (m:MemoryNode {space_id: $space_id})
WHERE m.path IS NOT NULL
UNWIND split(m.path, "/") AS seg
WITH seg, count(DISTINCT m) AS freq, total
WHERE seg <> "" AND size(seg) >= 2 AND freq <= total / 2
ORDER BY freq DESC, seg ASC
LIMIT $limit
RETURN seg AS tag, freq`
	if _, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, queryTopTags, map[string]any{
			"space_id": spaceID,
			"limit":    int64(b.opts.TopNTags),
		})
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			rec := res.Record()
			tag, _ := rec.Get("tag")
			freq, _ := rec.Get("freq")
			tagStr, _ := tag.(string)
			freqInt, _ := freq.(int64)
			if tagStr == "" {
				continue
			}
			out.topTags = append(out.topTags, countedRef{Ref: tagStr, Token: tagStr, Count: freqInt})
		}
		return nil, res.Err()
	}); err != nil {
		return nil, fmt.Errorf("top tags: %w", err)
	}

	// Query 3: top (role_type, layer) tuples.
	queryTopRTL := `
MATCH (m:MemoryNode {space_id: $space_id})
WHERE m.role_type IS NOT NULL AND m.layer IS NOT NULL
WITH m.role_type AS rt, m.layer AS l, count(*) AS freq
ORDER BY freq DESC, rt ASC, l ASC
LIMIT $limit
RETURN rt, l, freq`
	if _, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, queryTopRTL, map[string]any{
			"space_id": spaceID,
			"limit":    int64(b.opts.RoleTypeLayerBits),
		})
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			rec := res.Record()
			rtRaw, _ := rec.Get("rt")
			layerRaw, _ := rec.Get("l")
			freqRaw, _ := rec.Get("freq")
			rt, _ := rtRaw.(string)
			layer, _ := layerRaw.(int64)
			freq, _ := freqRaw.(int64)
			if rt == "" {
				continue
			}
			ref := fmt.Sprintf("%s|%d", rt, layer)
			token := fmt.Sprintf("(%s, L%d)", rt, layer)
			out.topRoleTypeLayer = append(out.topRoleTypeLayer, countedRef{Ref: ref, Token: token, Count: freq})
		}
		return nil, res.Err()
	}); err != nil {
		return nil, fmt.Errorf("top role_type/layer: %w", err)
	}

	// Query 4: distinct counts (for log-density allocator + Phase 14.2.1
	// drift analysis). Three small COUNT-DISTINCT queries.
	queryCounts := `
CALL {
  MATCH (m:MemoryNode {space_id: $space_id})
  WHERE m.path IS NOT NULL
  RETURN count(DISTINCT m.path) AS dp
}
CALL {
  MATCH (m:MemoryNode {space_id: $space_id})
  WHERE m.tags IS NOT NULL
  UNWIND m.tags AS tag
  RETURN count(DISTINCT tag) AS dt
}
CALL {
  MATCH (m:MemoryNode {space_id: $space_id})
  WHERE m.symbol IS NOT NULL
  RETURN count(DISTINCT m.symbol) AS ds
}
RETURN dp, dt, ds`
	if _, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, queryCounts, map[string]any{"space_id": spaceID})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			dp, _ := rec.Get("dp")
			dt, _ := rec.Get("dt")
			ds, _ := rec.Get("ds")
			if v, ok := dp.(int64); ok {
				out.distinctPaths = int(v)
			}
			if v, ok := dt.(int64); ok {
				out.distinctTags = int(v)
			}
			if v, ok := ds.(int64); ok {
				out.distinctSymbols = int(v)
			}
		}
		return nil, res.Err()
	}); err != nil {
		return nil, fmt.Errorf("distinct counts: %w", err)
	}

	return out, nil
}

// allocateBits is the pure deterministic bit-allocation algorithm. Given
// per-kind density data, returns a fully-populated bits[] slice of length
// b.opts.BitBudget. This is the function unit-tested without a Neo4j driver.
//
// Algorithm (per Phase 14.2 Epic 0 forensic):
//   1. Up to opts.RoleTypeLayerBits (default 32) → top-N role_type×layer tuples
//      (always informative; single-bit per tuple regardless of frequency)
//   2. Up to opts.TopNTags (default 32) → top-N tags (or 0 if empty)
//   3. Remaining bits → paths (or split with symbols when symbol density is
//      non-zero, with floor opts.FloorBitsPerKind each)
//   4. Pad any unused budget with empty BitEntry placeholders so callers
//      always see exactly bits[0..BitBudget-1] populated.
func (b *neo4jBuilder) allocateBits(d *densityResult) []BitEntry {
	bits := make([]BitEntry, 0, b.opts.BitBudget)
	pos := uint16(0)

	// Step 1: role_type×layer tuples
	rtlBudget := b.opts.RoleTypeLayerBits
	for i := 0; i < len(d.topRoleTypeLayer) && i < rtlBudget; i++ {
		entry := d.topRoleTypeLayer[i]
		bits = append(bits, BitEntry{
			Position: pos, Kind: BitKindRoleTypeLayer,
			Ref: entry.Ref, Token: entry.Token,
		})
		pos++
	}
	rtlUsed := int(pos)
	rtlSlack := rtlBudget - rtlUsed // unused bits we'll reclaim for paths

	// Step 2: tags
	tagBudget := b.opts.TopNTags
	tagStart := pos
	for i := 0; i < len(d.topTags) && i < tagBudget; i++ {
		entry := d.topTags[i]
		bits = append(bits, BitEntry{
			Position: pos, Kind: BitKindTag,
			Ref: entry.Ref, Token: entry.Token,
		})
		pos++
	}
	tagUsed := int(pos - tagStart)
	tagSlack := tagBudget - tagUsed

	// Step 3: paths (and optionally symbols if any space ever populates them).
	// For now, all surveyed production spaces have 0 symbols → all remaining
	// bits go to paths. Future code path for split-with-symbols below uses
	// log-density when both kinds have ≥1 distinct value.
	remainingBudget := max(b.opts.BitBudget-len(bits)+rtlSlack+tagSlack, 0)
	pathBudget := remainingBudget

	if d.distinctSymbols > 0 && d.distinctPaths > 0 {
		// Future: log-proportional split with floor. Today not exercised because
		// no production space has symbols. Pre-implemented for forward
		// compatibility: top-N most-common nodes by activation count would be
		// queried separately (queryTopSymbols TBD). For now, all goes to paths.
		// floor := b.opts.FloorBitsPerKind
		// logP := math.Log(float64(1 + d.distinctPaths))
		// logS := math.Log(float64(1 + d.distinctSymbols))
		// pathBudget = max(floor, int(float64(remainingBudget) * logP / (logP+logS)))
		_ = math.Log // mark as referenced; unused-import hygiene
	}
	pathStart := pos
	for i := 0; i < len(d.topPaths) && int(pos-pathStart) < pathBudget; i++ {
		entry := d.topPaths[i]
		bits = append(bits, BitEntry{
			Position: pos, Kind: BitKindPath,
			Ref: entry.Ref, Token: entry.Token,
		})
		pos++
	}

	// Step 4: pad to exact BitBudget. If we exhausted Cypher data before
	// filling the budget, remaining positions become empty BitKindPath
	// placeholders with empty refs (lookups will miss naturally).
	for int(pos) < b.opts.BitBudget {
		bits = append(bits, BitEntry{
			Position: pos, Kind: BitKindPath, Ref: "", Token: "",
		})
		pos++
	}

	SortBitsByPosition(bits)
	return bits
}

// activeVersionFor returns the current is_active=true catalog version for
// spaceID, or 0 if no catalog exists yet (cold-start).
func (b *neo4jBuilder) activeVersionFor(ctx context.Context, spaceID string) (int, error) {
	sess := b.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	q := `MATCH (c:ContextCatalog {space_id: $space_id, is_active: true}) RETURN c.version AS v LIMIT 1`
	v, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, q, map[string]any{"space_id": spaceID})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			raw, _ := rec.Get("v")
			if n, ok := raw.(int64); ok {
				return int(n), res.Err()
			}
		}
		return 0, res.Err()
	})
	if err != nil {
		return 0, err
	}
	if iv, ok := v.(int); ok {
		return iv, nil
	}
	return 0, nil
}

// persistCatalog atomically creates a new ContextCatalog node with
// is_active=true and marks all prior versions for this space is_active=false.
// Both writes happen in a single transaction so the (space_id, is_active=true)
// invariant is never violated.
func (b *neo4jBuilder) persistCatalog(ctx context.Context, spaceID string, version int, bits []BitEntry) error {
	sess := b.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Neo4j rejects arrays of maps as property values. Serialize the bits
	// array to a JSON string and store it on the ContextCatalog node;
	// loadOne unmarshals it back. Phase 14.2 V0026 originally specified an
	// array-of-map shape but the Cypher type system disallows it.
	bitsJSON, err := json.Marshal(bits)
	if err != nil {
		return fmt.Errorf("marshal bits: %w", err)
	}

	q := `
// Mark all prior versions inactive (idempotent if none exist).
MATCH (prev:ContextCatalog {space_id: $space_id, is_active: true})
SET prev.is_active = false, prev.deactivated_at = datetime()
WITH count(prev) AS _deactivated
// Create the new active version.
CREATE (c:ContextCatalog {
  space_id: $space_id,
  version: $version,
  total_bits: $total_bits,
  bits_json: $bits_json,
  is_active: true,
  created_at: datetime()
})
RETURN c.version AS v`

	_, err = sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, q, map[string]any{
			"space_id":   spaceID,
			"version":    int64(version),
			"total_bits": int64(b.opts.BitBudget),
			"bits_json":  string(bitsJSON),
		})
		if err != nil {
			return nil, err
		}
		// Drain to ensure the write completes
		for res.Next(ctx) {
		}
		return nil, res.Err()
	})
	return err
}

// neo4jLoader is the production CatalogLoader backed by Neo4j.
type neo4jLoader struct {
	driver neo4j.DriverWithContext
}

// NewNeo4jLoader constructs a CatalogLoader bound to a Neo4j driver.
func NewNeo4jLoader(driver neo4j.DriverWithContext) CatalogLoader {
	return &neo4jLoader{driver: driver}
}

// LoadActive reads the is_active=true catalog for spaceID from Neo4j.
// Returns (nil, nil) when no catalog exists yet — caller handles by setting
// empty fingerprint per Note 05 §2.4 fallback (cold-start).
func (l *neo4jLoader) LoadActive(ctx context.Context, spaceID string) (*Catalog, error) {
	return l.loadOne(ctx, spaceID, "MATCH (c:ContextCatalog {space_id: $space_id, is_active: true}) RETURN c LIMIT 1", map[string]any{
		"space_id": spaceID,
	})
}

// LoadVersion reads a specific historical catalog version for spaceID.
func (l *neo4jLoader) LoadVersion(ctx context.Context, spaceID string, version int) (*Catalog, error) {
	return l.loadOne(ctx, spaceID, "MATCH (c:ContextCatalog {space_id: $space_id, version: $version}) RETURN c LIMIT 1", map[string]any{
		"space_id": spaceID,
		"version":  int64(version),
	})
}

func (l *neo4jLoader) loadOne(ctx context.Context, spaceID, query string, params map[string]any) (*Catalog, error) {
	sess := l.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Use *neo4j.Node (typed pointer) so the cold-start path returns a
	// typed nil that survives the driver's generic castGeneric helper.
	// Returning untyped nil from the closure causes castGeneric's type
	// assertion to panic on `result.(any)` when the result is a true nil
	// interface (driver internals at transaction_helpers.go:70).
	nodePtr, err := neo4j.ExecuteRead(ctx, sess, func(tx neo4j.ManagedTransaction) (*neo4j.Node, error) {
		res, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}
		if !res.Next(ctx) {
			return nil, res.Err()
		}
		rec := res.Record()
		nodeRaw, ok := rec.Get("c")
		if !ok {
			return nil, fmt.Errorf("no 'c' field in result")
		}
		node, ok := nodeRaw.(neo4j.Node)
		if !ok {
			return nil, fmt.Errorf("'c' is not a Node (got %T)", nodeRaw)
		}
		return &node, res.Err()
	})
	if err != nil {
		return nil, err
	}
	if nodePtr == nil {
		return nil, nil // cold-start
	}

	node := *nodePtr
	props := node.Props
	versionVal, _ := props["version"].(int64)
	totalBitsVal, _ := props["total_bits"].(int64)

	// V0026 stores the bits array as a JSON-encoded string property
	// (Neo4j disallows arrays of maps). Older catalog rows that may have
	// been written before this fix use property "bits" with an array-of-map
	// shape — fall back to that when "bits_json" is absent.
	var bits []BitEntry
	if bitsJSON, ok := props["bits_json"].(string); ok && bitsJSON != "" {
		if err := json.Unmarshal([]byte(bitsJSON), &bits); err != nil {
			return nil, fmt.Errorf("unmarshal bits_json: %w", err)
		}
	} else if bitsRaw, ok := props["bits"].([]any); ok {
		bits = make([]BitEntry, 0, len(bitsRaw))
		for _, br := range bitsRaw {
			m, ok := br.(map[string]any)
			if !ok {
				continue
			}
			pos, _ := m["position"].(int64)
			kind, _ := m["kind"].(string)
			ref, _ := m["ref"].(string)
			token, _ := m["token"].(string)
			bits = append(bits, BitEntry{
				Position: uint16(pos),
				Kind:     BitKind(kind),
				Ref:      ref,
				Token:    token,
			})
		}
	}
	SortBitsByPosition(bits)
	return NewCatalog(spaceID, int(versionVal), uint16(totalBitsVal), bits)
}

// summarizeAllocation produces per-kind bit counts for diagnostics +
// the V0020 hypertable's denormalized columns. Pure helper.
func summarizeAllocation(bits []BitEntry) (rtlCount, tagCount, pathCount, symCount int) {
	for _, b := range bits {
		switch b.Kind {
		case BitKindRoleTypeLayer:
			rtlCount++
		case BitKindTag:
			tagCount++
		case BitKindPath:
			pathCount++
		case BitKindSymbol:
			symCount++
		}
	}
	return
}

// topRefs extracts the top-N Refs from a sorted bits slice for a given Kind.
// Used by the V0020 denormalized arrays.
func topRefs(bits []BitEntry, kind BitKind, limit int) []string {
	out := []string{}
	for _, b := range bits {
		if b.Kind == kind && b.Ref != "" {
			out = append(out, b.Ref)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

// stableSortCounted returns a sorted copy of refs by (count desc, ref asc)
// — the canonical determinism rule for the Builder. Used in tests.
func stableSortCounted(refs []countedRef) []countedRef {
	out := make([]countedRef, len(refs))
	copy(out, refs)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return strings.Compare(out[i].Ref, out[j].Ref) < 0
	})
	return out
}

// Freshness returns the time elapsed since the active catalog for spaceID
// was created. Used by the CycleOrchestrator stage 6 hook to decide whether
// to refresh (only after RefreshIntervalHours has elapsed).
//
// Returns (0, true) if no active catalog exists (cold-start: refresh now).
// Returns (age, false) when an active catalog exists; caller compares age
// against the configured interval.
func (l *neo4jLoader) Freshness(ctx context.Context, spaceID string) (time.Duration, bool, error) {
	sess := l.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	q := `MATCH (c:ContextCatalog {space_id: $space_id, is_active: true}) RETURN c.created_at AS ts LIMIT 1`
	// Use *time.Time (typed pointer) so the cold-start path returns a typed
	// nil that survives the driver's generic castGeneric helper.
	tsPtr, err := neo4j.ExecuteRead(ctx, sess, func(tx neo4j.ManagedTransaction) (*time.Time, error) {
		res, err := tx.Run(ctx, q, map[string]any{"space_id": spaceID})
		if err != nil {
			return nil, err
		}
		if !res.Next(ctx) {
			return nil, res.Err()
		}
		rec := res.Record()
		ts, _ := rec.Get("ts")
		if t, ok := ts.(time.Time); ok {
			return &t, res.Err()
		}
		return nil, res.Err()
	})
	if err != nil {
		return 0, false, err
	}
	if tsPtr == nil {
		return 0, true, nil
	}
	return time.Since(*tsPtr), false, nil
}
