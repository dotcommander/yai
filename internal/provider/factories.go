package provider

import (
	"fmt"
	"net/http"
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

type providerFactory func(api, apiKey, baseURL string, httpClient *http.Client) (fantasy.Provider, error)

var factories = map[string]providerFactory{
	apiOpenAI:     newOpenAI,
	apiAnthropic:  newAnthropic,
	apiGoogle:     newGoogle,
	apiAzure:      newAzure,
	apiOpenRouter: newOpenRouter,
	apiVercel:     newVercel,
	apiBedrock:    newBedrock,
}

func newOpenAI(_, apiKey, baseURL string, httpClient *http.Client) (fantasy.Provider, error) {
	opts := []fopenai.Option{fopenai.WithAPIKey(apiKey)}
	if baseURL != "" {
		opts = append(opts, fopenai.WithBaseURL(baseURL))
	}
	if httpClient != nil {
		opts = append(opts, fopenai.WithHTTPClient(httpClient))
	}
	provider, err := fopenai.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("new fantasy openai provider: %w", err)
	}
	return provider, nil
}

func newAnthropic(_, apiKey, baseURL string, httpClient *http.Client) (fantasy.Provider, error) {
	opts := []anthropic.Option{anthropic.WithAPIKey(apiKey)}
	if baseURL != "" {
		opts = append(opts, anthropic.WithBaseURL(strings.TrimSuffix(baseURL, "/v1")))
	}
	if httpClient != nil {
		opts = append(opts, anthropic.WithHTTPClient(httpClient))
	}
	provider, err := anthropic.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("new fantasy anthropic provider: %w", err)
	}
	return provider, nil
}

func newGoogle(_, apiKey, baseURL string, httpClient *http.Client) (fantasy.Provider, error) {
	opts := []fgoogle.Option{fgoogle.WithGeminiAPIKey(apiKey)}
	if baseURL != "" {
		opts = append(opts, fgoogle.WithBaseURL(baseURL))
	}
	if httpClient != nil {
		opts = append(opts, fgoogle.WithHTTPClient(httpClient))
	}
	provider, err := fgoogle.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("new fantasy google provider: %w", err)
	}
	return provider, nil
}

func newAzure(_, apiKey, baseURL string, httpClient *http.Client) (fantasy.Provider, error) {
	opts := []azure.Option{azure.WithAPIKey(apiKey), azure.WithBaseURL(baseURL)}
	if httpClient != nil {
		opts = append(opts, azure.WithHTTPClient(httpClient))
	}
	provider, err := azure.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("new fantasy azure provider: %w", err)
	}
	return provider, nil
}

func newOpenRouter(_, apiKey, _ string, httpClient *http.Client) (fantasy.Provider, error) {
	opts := []openrouter.Option{openrouter.WithAPIKey(apiKey)}
	if httpClient != nil {
		opts = append(opts, openrouter.WithHTTPClient(httpClient))
	}
	provider, err := openrouter.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("new fantasy openrouter provider: %w", err)
	}
	return provider, nil
}

func newVercel(_, apiKey, baseURL string, httpClient *http.Client) (fantasy.Provider, error) {
	opts := []vercel.Option{vercel.WithAPIKey(apiKey)}
	if baseURL != "" {
		opts = append(opts, vercel.WithBaseURL(baseURL))
	}
	if httpClient != nil {
		opts = append(opts, vercel.WithHTTPClient(httpClient))
	}
	provider, err := vercel.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("new fantasy vercel provider: %w", err)
	}
	return provider, nil
}

func newBedrock(_, apiKey, _ string, httpClient *http.Client) (fantasy.Provider, error) {
	opts := []bedrock.Option{}
	if apiKey != "" {
		opts = append(opts, bedrock.WithAPIKey(apiKey))
	}
	if httpClient != nil {
		opts = append(opts, bedrock.WithHTTPClient(httpClient))
	}
	provider, err := bedrock.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("new fantasy bedrock provider: %w", err)
	}
	return provider, nil
}

func newOpenAICompat(api, apiKey, baseURL string, httpClient *http.Client) (fantasy.Provider, error) {
	opts := []fopenaicompat.Option{fopenaicompat.WithName(api)}
	if apiKey != "" {
		opts = append(opts, fopenaicompat.WithAPIKey(apiKey))
	}
	if baseURL != "" {
		opts = append(opts, fopenaicompat.WithBaseURL(baseURL))
	}
	if httpClient != nil {
		opts = append(opts, fopenaicompat.WithHTTPClient(httpClient))
	}
	provider, err := fopenaicompat.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("new fantasy openai-compatible provider: %w", err)
	}
	return provider, nil
}
