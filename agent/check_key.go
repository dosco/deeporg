package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	ax "github.com/ax-llm/ax/packages/go"
)

// CheckAPIKey sends one small request to the provider with apiKey, so a key
// that does not work is found when it is entered and not at the first
// question. The key is used for this request only; it is not stored and does
// not replace the configured key source.
//
// factory, when not nil, builds the client in place of the provider profile.
// Tests use it to stand in for the provider.
func CheckAPIKey(ctx context.Context, cfg Config, apiKey string, factory ClientFactory) error {
	cfg = cfg.withDefaults()
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return fmt.Errorf("%w: no key was given", ErrMissingAPIKey)
	}
	var (
		client ax.AIClient
		err    error
	)
	if factory != nil {
		client, err = factory(cfg)
	} else {
		options := map[string]ax.Value{"apiKey": apiKey, "api_key": apiKey}
		if cfg.Model != "" {
			options["model"] = cfg.Model
		}
		if cfg.BaseURL != "" {
			options["baseUrl"] = cfg.BaseURL
			options["base_url"] = cfg.BaseURL
		}
		client, err = newProfileClient(cfg.Provider, options)
	}
	if err != nil {
		return err
	}
	if client == nil {
		return errors.New("the provider client could not be built")
	}
	_, err = client.Chat(ctx, map[string]ax.Value{
		"chat_prompt": ax.Array(ax.Object("role", "user", "content", "Reply with the single word: ok")),
	}, nil)
	return err
}
