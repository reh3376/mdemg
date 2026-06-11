package embeddings

import "testing"

func TestBase_WalksWrapperChain(t *testing.T) {
	stub := NewStub()

	if got := Base(stub); got != stub {
		t.Fatalf("bare embedder: Base must return it unchanged")
	}

	cached := NewCachedEmbedder(stub, 10)
	if got := Base(cached); got != stub {
		t.Fatalf("cache-wrapped: Base = %T, want stub", got)
	}

	// The DEFAULT production shape: rate-limit over cache over provider.
	rl := NewRateLimitedEmbedder(cached, 10, 5, true)
	if got := Base(rl); got != stub {
		t.Fatalf("ratelimit(cache(provider)): Base = %T, want stub", got)
	}
}

func TestFindCached_AnyDepth(t *testing.T) {
	stub := NewStub()

	if _, ok := FindCached(stub); ok {
		t.Fatalf("bare embedder must not report a cache layer")
	}

	cached := NewCachedEmbedder(stub, 10)
	rl := NewRateLimitedEmbedder(cached, 10, 5, true)
	got, ok := FindCached(rl)
	if !ok || got != cached {
		t.Fatalf("FindCached through outer wrapper failed: ok=%v got=%T", ok, got)
	}
}
