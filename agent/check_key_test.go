package agent

import (
	"context"
	"errors"
	"testing"

	ax "github.com/ax-llm/ax/packages/go"
)

type checkKeyClient struct {
	err   error
	calls int
}

func (c *checkKeyClient) Chat(context.Context, map[string]ax.Value, map[string]ax.Value) (ax.Value, error) {
	c.calls++
	return ax.Object("results", ax.Array()), c.err
}

func (c *checkKeyClient) Embed(context.Context, map[string]ax.Value, map[string]ax.Value) (ax.Value, error) {
	return nil, nil
}

func (c *checkKeyClient) Stream(context.Context, map[string]ax.Value, map[string]ax.Value) ([]ax.Value, error) {
	return nil, nil
}

func TestCheckAPIKey(t *testing.T) {
	ctx := context.Background()
	client := &checkKeyClient{}
	factory := func(Config) (ax.AIClient, error) { return client, nil }

	// No key: refused before anything is sent.
	if err := CheckAPIKey(ctx, Config{Provider: "openai"}, "  ", factory); !errors.Is(err, ErrMissingAPIKey) || client.calls != 0 {
		t.Fatalf("empty key: err=%v calls=%d", err, client.calls)
	}

	// A key the provider accepts: exactly one request is made.
	if err := CheckAPIKey(ctx, Config{Provider: "openai"}, "sk-test", factory); err != nil || client.calls != 1 {
		t.Fatalf("accepted key: err=%v calls=%d", err, client.calls)
	}

	// A key the provider refuses: its error comes back.
	refused := errors.New("401 incorrect api key")
	client.err = refused
	if err := CheckAPIKey(ctx, Config{Provider: "openai"}, "sk-test", factory); !errors.Is(err, refused) {
		t.Fatalf("refused key: err=%v", err)
	}

	// A provider that cannot be built is an error, not a panic.
	if err := CheckAPIKey(ctx, Config{Provider: "no-such-provider"}, "sk-test", nil); err == nil {
		t.Fatal("an unknown provider was accepted")
	}
	broken := errors.New("factory failed")
	if err := CheckAPIKey(ctx, Config{}, "sk-test", func(Config) (ax.AIClient, error) { return nil, broken }); !errors.Is(err, broken) {
		t.Fatalf("factory error: err=%v", err)
	}
}
