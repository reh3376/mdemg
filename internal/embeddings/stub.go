package embeddings

import (
	"context"
	"hash/fnv"
	"math"
)

const stubDimensions = 3072

// Stub implements the Embedder interface with deterministic fixed-dimension vectors.
// It requires no external services, making it suitable for CI and testing environments.
// Vectors are derived from a hash of the input text so identical inputs produce identical embeddings.
type Stub struct{}

// NewStub creates a new stub embedder.
func NewStub() *Stub {
	return &Stub{}
}

func (s *Stub) Embed(_ context.Context, text string) ([]float32, error) {
	return stubEmbed(text), nil
}

func (s *Stub) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, t := range texts {
		results[i] = stubEmbed(t)
	}
	return results, nil
}

func (s *Stub) Dimensions() int { return stubDimensions }
func (s *Stub) Name() string    { return "stub" }

// stubEmbed generates a deterministic unit vector from text using FNV hashing.
// The vector is normalized so cosine similarity works correctly.
func stubEmbed(text string) []float32 {
	h := fnv.New64a()
	h.Write([]byte(text))
	seed := h.Sum64()

	vec := make([]float32, stubDimensions)
	var sumSq float64
	for i := range vec {
		// Simple deterministic pseudo-random from seed
		seed = seed*6364136223846793005 + 1442695040888963407
		val := float64(int64(seed>>33)) / float64(1<<31)
		vec[i] = float32(val)
		sumSq += val * val
	}

	// Normalize to unit vector
	norm := float32(math.Sqrt(sumSq))
	if norm > 0 {
		for i := range vec {
			vec[i] /= norm
		}
	}

	return vec
}
