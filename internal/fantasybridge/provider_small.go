//go:build yai_small

package fantasybridge

import (
	"fmt"

	"charm.land/fantasy"
	fopenaicompat "charm.land/fantasy/providers/openaicompat"
)

func newProvider(cfg Config) (fantasy.Provider, error) {
	// In the small build, only the OpenAI-compatible provider is included.
	opts := []fopenaicompat.Option{fopenaicompat.WithName(cfg.API)}
	if cfg.APIKey != "" {
		opts = append(opts, fopenaicompat.WithAPIKey(cfg.APIKey))
	}
	if cfg.BaseURL != "" {
		opts = append(opts, fopenaicompat.WithBaseURL(cfg.BaseURL))
	}
	if cfg.HTTPClient != nil {
		opts = append(opts, fopenaicompat.WithHTTPClient(cfg.HTTPClient))
	}
	provider, err := fopenaicompat.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("new fantasy openai-compatible provider: %w", err)
	}
	return provider, nil
}
