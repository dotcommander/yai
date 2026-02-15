//go:build !yai_small

package fantasybridge

import (
	"fmt"
	"strings"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/anthropic"
	"charm.land/fantasy/providers/azure"
	"charm.land/fantasy/providers/bedrock"
	fgoogle "charm.land/fantasy/providers/google"
	fopenai "charm.land/fantasy/providers/openai"
	fopenaicompat "charm.land/fantasy/providers/openaicompat"
	"charm.land/fantasy/providers/openrouter"
	"charm.land/fantasy/providers/vercel"
)

func newProvider(cfg Config) (fantasy.Provider, error) {
	switch cfg.API {
	case apiOpenAI:
		opts := []fopenai.Option{fopenai.WithAPIKey(cfg.APIKey)}
		if cfg.BaseURL != "" {
			opts = append(opts, fopenai.WithBaseURL(cfg.BaseURL))
		}
		if cfg.HTTPClient != nil {
			opts = append(opts, fopenai.WithHTTPClient(cfg.HTTPClient))
		}
		provider, err := fopenai.New(opts...)
		if err != nil {
			return nil, fmt.Errorf("new fantasy openai provider: %w", err)
		}
		return provider, nil
	case apiAnthropic:
		opts := []anthropic.Option{anthropic.WithAPIKey(cfg.APIKey)}
		if cfg.BaseURL != "" {
			opts = append(opts, anthropic.WithBaseURL(strings.TrimSuffix(cfg.BaseURL, "/v1")))
		}
		if cfg.HTTPClient != nil {
			opts = append(opts, anthropic.WithHTTPClient(cfg.HTTPClient))
		}
		provider, err := anthropic.New(opts...)
		if err != nil {
			return nil, fmt.Errorf("new fantasy anthropic provider: %w", err)
		}
		return provider, nil
	case apiGoogle:
		opts := []fgoogle.Option{fgoogle.WithGeminiAPIKey(cfg.APIKey)}
		if cfg.BaseURL != "" {
			opts = append(opts, fgoogle.WithBaseURL(cfg.BaseURL))
		}
		if cfg.HTTPClient != nil {
			opts = append(opts, fgoogle.WithHTTPClient(cfg.HTTPClient))
		}
		provider, err := fgoogle.New(opts...)
		if err != nil {
			return nil, fmt.Errorf("new fantasy google provider: %w", err)
		}
		return provider, nil
	case apiAzure, apiAzureAD:
		opts := []azure.Option{azure.WithAPIKey(cfg.APIKey), azure.WithBaseURL(cfg.BaseURL)}
		if cfg.HTTPClient != nil {
			opts = append(opts, azure.WithHTTPClient(cfg.HTTPClient))
		}
		provider, err := azure.New(opts...)
		if err != nil {
			return nil, fmt.Errorf("new fantasy azure provider: %w", err)
		}
		return provider, nil
	case "openrouter":
		opts := []openrouter.Option{openrouter.WithAPIKey(cfg.APIKey)}
		if cfg.HTTPClient != nil {
			opts = append(opts, openrouter.WithHTTPClient(cfg.HTTPClient))
		}
		provider, err := openrouter.New(opts...)
		if err != nil {
			return nil, fmt.Errorf("new fantasy openrouter provider: %w", err)
		}
		return provider, nil
	case "vercel":
		opts := []vercel.Option{vercel.WithAPIKey(cfg.APIKey)}
		if cfg.BaseURL != "" {
			opts = append(opts, vercel.WithBaseURL(cfg.BaseURL))
		}
		if cfg.HTTPClient != nil {
			opts = append(opts, vercel.WithHTTPClient(cfg.HTTPClient))
		}
		provider, err := vercel.New(opts...)
		if err != nil {
			return nil, fmt.Errorf("new fantasy vercel provider: %w", err)
		}
		return provider, nil
	case "bedrock":
		opts := []bedrock.Option{}
		if cfg.APIKey != "" {
			opts = append(opts, bedrock.WithAPIKey(cfg.APIKey))
		}
		if cfg.HTTPClient != nil {
			opts = append(opts, bedrock.WithHTTPClient(cfg.HTTPClient))
		}
		provider, err := bedrock.New(opts...)
		if err != nil {
			return nil, fmt.Errorf("new fantasy bedrock provider: %w", err)
		}
		return provider, nil
	default:
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
}
