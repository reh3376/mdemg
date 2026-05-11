package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"mdemg/internal/config"
)

// OllamaFetcher is the Ollama Library backend for mdemg model pull. It is the
// only file in Sprint MODEL-DIST-001 that knows about Ollama-specific concepts:
//   - ~/.ollama/models filesystem layout (OLLAMA_MODELS env)
//   - registry.ollama.ai manifest path (OLLAMA_HOST env)
//   - `ollama pull` / `ollama rm` CLI invocations
//   - manifest layer mediaType filtering (application/vnd.ollama.image.model)
//
// The CLI layer (internal/cli/model.go) never references any of these — it
// goes through the Fetcher interface.
type OllamaFetcher struct {
	cfg config.Config
}

func NewOllamaFetcher(cfg config.Config) *OllamaFetcher {
	return &OllamaFetcher{cfg: cfg}
}

func (f *OllamaFetcher) Name() string { return "ollama" }

// buildTag composes the Ollama tag from request fields. Adapter form uses
// the "<name>-adapter:latest" pattern documented in the sprint plan.
func (f *OllamaFetcher) buildTag(req FetchRequest) string {
	if req.Adapter {
		return fmt.Sprintf("%s/%s-adapter:latest", req.Namespace, req.Name)
	}
	return fmt.Sprintf("%s/%s:%s", req.Namespace, req.Name, req.Quant)
}

// manifestPath returns the path to Ollama's manifest file for this tag.
// Mirrors Ollama's on-disk layout: <OLLAMA_MODELS>/manifests/<OLLAMA_HOST>/<ns>/<name>/<tag>.
func (f *OllamaFetcher) manifestPath(req FetchRequest) string {
	tag := req.Quant
	name := req.Name
	if req.Adapter {
		name = req.Name + "-adapter"
		tag = "latest"
	}
	return filepath.Join(f.cfg.OllamaModelsRoot, "manifests", f.cfg.OllamaRegistryHost, req.Namespace, name, tag)
}

// blobPath resolves a sha256-prefixed digest to the absolute blob path.
func (f *OllamaFetcher) blobPath(digest string) string {
	// Ollama stores blobs as "sha256-<hex>" filenames (note hyphen, not colon).
	digest = strings.TrimPrefix(digest, "sha256:")
	return filepath.Join(f.cfg.OllamaModelsRoot, "blobs", "sha256-"+digest)
}

// ollamaManifest is the relevant subset of the JSON Ollama writes to
// manifests/<host>/<ns>/<name>/<tag>. We only need digests + mediaTypes.
type ollamaManifest struct {
	SchemaVersion int                  `json:"schemaVersion"`
	MediaType     string               `json:"mediaType"`
	Config        ollamaManifestConfig `json:"config"`
	Layers        []ollamaManifestLayer `json:"layers"`
}

type ollamaManifestConfig struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

type ollamaManifestLayer struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

// readModelBlobDigest locates the GGUF blob digest within a manifest by
// filtering on the canonical mediaType. Returns digest + declared size.
func (f *OllamaFetcher) readModelBlobDigest(req FetchRequest) (digest string, size int64, err error) {
	mfPath := f.manifestPath(req)
	raw, err := os.ReadFile(mfPath)
	if err != nil {
		return "", 0, fmt.Errorf("read ollama manifest %s: %w", mfPath, err)
	}
	var mf ollamaManifest
	if err := json.Unmarshal(raw, &mf); err != nil {
		return "", 0, fmt.Errorf("parse ollama manifest %s: %w", mfPath, err)
	}
	const modelMT = "application/vnd.ollama.image.model"
	for _, layer := range mf.Layers {
		if layer.MediaType == modelMT {
			return layer.Digest, layer.Size, nil
		}
	}
	return "", 0, fmt.Errorf("ollama manifest %s has no layer with mediaType %s", mfPath, modelMT)
}

// preflight ensures the ollama CLI is on PATH. The actionable error message
// covers both install (brew install ollama) and Configurability Contract
// override (MDEMG_MODEL_BACKEND=<other>) paths.
func (f *OllamaFetcher) preflight() error {
	if _, err := exec.LookPath("ollama"); err != nil {
		return fmt.Errorf(`ollama CLI not found on PATH.
  Resolve via one of:
    1. Install: brew install ollama  (macOS)  or  curl -fsSL https://ollama.com/install.sh | sh  (linux)
    2. Switch backend: MDEMG_MODEL_BACKEND=<future-backend>   (future: hf, s3, github-release, file)
    3. Manual install: obtain a GGUF, set MDEMG_MODEL_PATH directly, skip 'mdemg model pull'`)
	}
	return nil
}

// Fetch executes the full pull → blob discovery → symlink → SHA verify path.
func (f *OllamaFetcher) Fetch(ctx context.Context, req FetchRequest) (*FetchResult, error) {
	start := time.Now()
	if req.Adapter {
		return nil, ErrAdapterDeferred
	}
	if err := f.preflight(); err != nil {
		return nil, err
	}
	tag := f.buildTag(req)

	if req.DryRun {
		// Don't invoke ollama; just compute what we would do.
		mfPath := f.manifestPath(req)
		destPath := filepath.Join(req.DestDir, fmt.Sprintf("%s.%s.gguf", req.Name, req.Quant))
		return &FetchResult{
			LocalPath:   destPath,
			BlobPath:    fmt.Sprintf("(would resolve from manifest at %s)", mfPath),
			SHA256:      "",
			BackendName: f.Name(),
			SizeBytes:   0,
			LatencyMS:   time.Since(start).Milliseconds(),
			Adapter:     req.Adapter,
		}, nil
	}

	// Run `ollama pull <tag>` with passthrough so the operator sees progress.
	cmd := exec.CommandContext(ctx, "ollama", "pull", tag)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ollama pull %s: %w", tag, err)
	}

	// Discover the GGUF blob via Ollama's manifest.
	digest, declaredSize, err := f.readModelBlobDigest(req)
	if err != nil {
		return nil, err
	}
	blob := f.blobPath(digest)
	if _, err := os.Stat(blob); err != nil {
		return nil, fmt.Errorf("ollama blob %s not found on disk (manifest digest %s): %w", blob, digest, err)
	}

	// Create destination directory + symlink.
	if err := os.MkdirAll(req.DestDir, 0o755); err != nil {
		return nil, fmt.Errorf("create dest dir %s: %w", req.DestDir, err)
	}
	destPath := filepath.Join(req.DestDir, fmt.Sprintf("%s.%s.gguf", req.Name, req.Quant))
	// Remove existing symlink/file to make the operation idempotent.
	_ = os.Remove(destPath)
	if err := os.Symlink(blob, destPath); err != nil {
		return nil, fmt.Errorf("symlink %s -> %s: %w", destPath, blob, err)
	}

	// Compute SHA256 of the resolved file (verify against manifest happens
	// in the CLI layer to keep this Fetcher backend-pure).
	sha, err := sha256OfFile(blob)
	if err != nil {
		return nil, fmt.Errorf("sha256 %s: %w", blob, err)
	}

	return &FetchResult{
		LocalPath:   destPath,
		BlobPath:    blob,
		SHA256:      sha,
		BackendName: f.Name(),
		SizeBytes:   declaredSize,
		LatencyMS:   time.Since(start).Milliseconds(),
		Adapter:     req.Adapter,
	}, nil
}

// Verify re-checks the symlink target SHA. The CLI layer cross-checks against
// the QuantManifest separately; this method only confirms the on-disk file
// hasn't been corrupted vs the digest in Ollama's manifest.
func (f *OllamaFetcher) Verify(ctx context.Context, req FetchRequest) error {
	digest, _, err := f.readModelBlobDigest(req)
	if err != nil {
		return err
	}
	blob := f.blobPath(digest)
	if _, err := os.Stat(blob); err != nil {
		return fmt.Errorf("ollama blob %s missing: %w", blob, err)
	}
	got, err := sha256OfFile(blob)
	if err != nil {
		return fmt.Errorf("sha256 %s: %w", blob, err)
	}
	want := strings.TrimPrefix(digest, "sha256:")
	if got != want {
		return fmt.Errorf("blob %s SHA mismatch: have %s, ollama manifest says %s", blob, got, want)
	}
	return nil
}

// Remove unlinks the local symlink AND invokes `ollama rm <tag>`. Callers
// should confirm before invoking; this method is purposely destructive.
func (f *OllamaFetcher) Remove(ctx context.Context, req FetchRequest) error {
	destPath := filepath.Join(req.DestDir, fmt.Sprintf("%s.%s.gguf", req.Name, req.Quant))
	if err := os.Remove(destPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove symlink %s: %w", destPath, err)
	}
	tag := f.buildTag(req)
	cmd := exec.CommandContext(ctx, "ollama", "rm", tag)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// Non-fatal: ollama rm of a non-existent tag fails; the symlink is
		// already gone; report but don't error if it's a "not found" path.
		return fmt.Errorf("ollama rm %s: %w (symlink already removed at %s)", tag, err, destPath)
	}
	return nil
}

// sha256OfFile computes SHA256 in a streaming fashion (no full-file read into RAM).
func sha256OfFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
