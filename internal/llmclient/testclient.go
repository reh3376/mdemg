package llmclient

import "context"

// TestClient is a Completer test double. CompleteFn is invoked by Complete;
// tests set it to control behaviour (return canned output, assert on inputs,
// simulate errors, etc.). Introduced for Sprint FT-LORA-B's guardrail
// migration, but usable by any consumer that depends on the Completer
// interface instead of *Client.
type TestClient struct {
	CompleteFn func(ctx context.Context, messages []Message, opts CompleteOpts) (string, error)
}

// Complete delegates to CompleteFn. Returns an empty string and nil error
// when CompleteFn is unset (useful for tests that only need a stub).
func (t *TestClient) Complete(ctx context.Context, messages []Message, opts CompleteOpts) (string, error) {
	if t.CompleteFn == nil {
		return "", nil
	}
	return t.CompleteFn(ctx, messages, opts)
}
