package cli

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"mdemg/internal/config"
)

// FetchRequest captures everything a backend needs to fulfill a model pull.
// Every field is sourced from config / env / flags — no constants in call sites.
type FetchRequest struct {
	Namespace string
	Name      string
	Quant     string // resolved (never "auto" by the time a Fetcher sees it)
	Adapter   bool   // adapter tag form — Sprint MODEL-DIST-001 ships fused only; --adapter errors with deferral message
	DestDir   string
	DryRun    bool
}

// FetchResult is what the backend returns. The CLI layer prints, records,
// and verifies — it doesn't know whether bytes came from Ollama, HF, S3, etc.
type FetchResult struct {
	LocalPath   string // resolved symlink path under DestDir
	BlobPath    string // backend-native source location (for diagnostics)
	SHA256      string // computed over the resolved file
	BackendName string // "ollama" / "hf" / "s3" / "file"
	SizeBytes   int64
	LatencyMS   int64
	Adapter     bool // echoes request.Adapter; consumers of FetchResult may key behavior on it
}

// Fetcher is the interface every distribution backend implements.
type Fetcher interface {
	Name() string
	Fetch(ctx context.Context, req FetchRequest) (*FetchResult, error)
	Verify(ctx context.Context, req FetchRequest) error
	Remove(ctx context.Context, req FetchRequest) error
}

// ErrAdapterDeferred is returned by any backend when Adapter=true in a sprint
// that hasn't shipped the adapter path yet. Sprint MODEL-DIST-001 ships fused
// only; Sprint MODEL-DIST-002 lifts this restriction.
var ErrAdapterDeferred = fmt.Errorf("adapter (--adapter) distribution lands in Sprint MODEL-DIST-002 (see docs/development/model-dist-001/epic_2_forensic.md); use a fused quant for now")

// NewFetcher dispatches on cfg.ModelBackend. Adding a new backend means
// adding one file + one case branch — the CLI surface stays unchanged.
func NewFetcher(cfg config.Config) (Fetcher, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.ModelBackend)) {
	case "", "ollama":
		return NewOllamaFetcher(cfg), nil
	// Future: case "hf": return NewHFFetcher(cfg), nil
	// Future: case "s3": return NewS3Fetcher(cfg), nil
	// Future: case "github-release": return NewGitHubReleaseFetcher(cfg), nil
	// Future: case "file": return NewFileFetcher(cfg), nil
	default:
		return nil, fmt.Errorf("unknown MDEMG_MODEL_BACKEND %q (supported: ollama)", cfg.ModelBackend)
	}
}

// QuantManifest is the runtime contract for verifying pulled models against
// the SHAs captured at sprint build time. Source-of-truth file is committed
// in this package as quant_manifest.json (also mirrored in docs).
type QuantManifest struct {
	Version          string                 `json:"version"`
	ModelName        string                 `json:"model_name"`
	NamespaceDefault string                 `json:"namespace_default"`
	BuildDate        string                 `json:"build_date"`
	Quants           map[string]QuantRecord `json:"quants"`
}

// QuantRecord is the per-quant runtime record. Fields marked json:"-" are
// optional/docs-only and not enforced at verify time.
type QuantRecord struct {
	SizeBytes            int64   `json:"size_bytes"`
	SizeHuman            string  `json:"size_human"`
	BPW                  float64 `json:"bpw"`
	MinRAMGB             int     `json:"min_ram_gb"`
	RecommendedRAMGB     int     `json:"recommended_ram_gb"`
	SHA256               string  `json:"sha256"`
	OllamaLocalID        string  `json:"ollama_local_id"`
	OllamaManifestDigest string  `json:"ollama_manifest_digest"`
}

//go:embed quant_manifest.json
var embeddedQuantManifest []byte

// LoadQuantManifest reads the runtime manifest. Order:
//  1. cfg.ModelManifestPath (operator override; air-gapped deployments)
//  2. embedded quant_manifest.json (canonical, sprint-built)
func LoadQuantManifest(cfg config.Config) (*QuantManifest, error) {
	var data []byte
	if p := strings.TrimSpace(cfg.ModelManifestPath); p != "" {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("read manifest %s: %w", p, err)
		}
		data = b
	} else {
		data = embeddedQuantManifest
	}
	var m QuantManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse quant manifest: %w", err)
	}
	return &m, nil
}

// ResolveQuants parses cfg.ModelQuants (comma-separated allowlist) into a
// trimmed slice. Empty entries dropped. Order preserved.
func ResolveQuants(cfg config.Config) []string {
	raw := strings.TrimSpace(cfg.ModelQuants)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ramTier represents a single rule from MDEMG_MODEL_RAM_TIERS. MaxGB == 0
// signals the default fallback. Tiers parse from the JSON syntax described
// in the Configurability Contract: keys are either "<N" (less-than threshold
// in GB) or "default"; values are quant names.
type ramTier struct {
	MaxGB int    // 0 → fallback ("default")
	Quant string
}

// parseRamTiers decodes MDEMG_MODEL_RAM_TIERS JSON. Tiers with explicit
// thresholds are sorted ascending; the "default" tier becomes MaxGB=0 and
// is consulted last.
func parseRamTiers(raw string) ([]ramTier, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, fmt.Errorf("parse MDEMG_MODEL_RAM_TIERS: %w", err)
	}
	out := make([]ramTier, 0, len(m))
	for k, v := range m {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		switch {
		case k == "default":
			out = append(out, ramTier{MaxGB: 0, Quant: v})
		case strings.HasPrefix(k, "<"):
			n, err := strconv.Atoi(strings.TrimPrefix(k, "<"))
			if err != nil {
				return nil, fmt.Errorf("MDEMG_MODEL_RAM_TIERS key %q: expected '<N' or 'default': %w", k, err)
			}
			out = append(out, ramTier{MaxGB: n, Quant: v})
		default:
			return nil, fmt.Errorf("MDEMG_MODEL_RAM_TIERS key %q: expected '<N' or 'default'", k)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		// default last (MaxGB=0). Otherwise ascending.
		if out[i].MaxGB == 0 {
			return false
		}
		if out[j].MaxGB == 0 {
			return true
		}
		return out[i].MaxGB < out[j].MaxGB
	})
	return out, nil
}

// PickQuantForRAM walks the parsed tiers and returns the matching quant for
// a host with hostRAMGB GB of RAM. The first tier where hostRAMGB < MaxGB
// wins; if none matches, the default (MaxGB=0) tier is returned. If no
// default exists and no tier matches, returns an error.
func PickQuantForRAM(tiers []ramTier, hostRAMGB int) (string, error) {
	var defaultQuant string
	for _, t := range tiers {
		if t.MaxGB == 0 {
			defaultQuant = t.Quant
			continue
		}
		if hostRAMGB < t.MaxGB {
			return t.Quant, nil
		}
	}
	if defaultQuant == "" {
		return "", fmt.Errorf("no quant matches host RAM %d GB and no default rule found in MDEMG_MODEL_RAM_TIERS", hostRAMGB)
	}
	return defaultQuant, nil
}

// ResolveQuant returns the concrete quant given a user-specified value
// (or "auto" for RAM-tier dispatch), the allowlist from cfg.ModelQuants,
// and the host's RAM in GB. Errors if the resolved quant isn't in the
// allowlist — this guards against typos and ensures Modelfile <-> CLI
// agreement.
func ResolveQuant(cfg config.Config, requested string, hostRAMGB int) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		requested = strings.TrimSpace(cfg.ModelQuant)
	}
	if requested == "" {
		requested = "auto"
	}

	allowed := ResolveQuants(cfg)
	if len(allowed) == 0 {
		return "", fmt.Errorf("MDEMG_MODEL_QUANTS is empty; cannot resolve any quant")
	}

	if strings.EqualFold(requested, "auto") {
		tiers, err := parseRamTiers(cfg.ModelRamTiers)
		if err != nil {
			return "", err
		}
		picked, err := PickQuantForRAM(tiers, hostRAMGB)
		if err != nil {
			return "", err
		}
		requested = picked
	}

	for _, q := range allowed {
		if q == requested {
			return requested, nil
		}
	}
	return "", fmt.Errorf("quant %q not in MDEMG_MODEL_QUANTS allowlist %v", requested, allowed)
}
