package embeddings

import (
	"context"
	"errors"
)

// ErrNoEmbedder is returned when embedding is attempted without a configured embedder.
var ErrNoEmbedder = errors.New("no embedder configured")

// NilSafe wraps an Embedder and returns ErrNoEmbedder instead of panicking when nil.
// Use this at construction time: embedder = embeddings.NilSafe(maybeNilEmbedder)
func NilSafe(e Embedder) Embedder {
	if e == nil {
		return &nilEmbedder{}
	}
	return e
}

type nilEmbedder struct{}

func (n *nilEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, ErrNoEmbedder
}

func (n *nilEmbedder) EmbedBatch(_ context.Context, _ []string) ([][]float32, error) {
	return nil, ErrNoEmbedder
}

func (n *nilEmbedder) Dimensions() int { return 0 }
func (n *nilEmbedder) Name() string    { return "none" }
