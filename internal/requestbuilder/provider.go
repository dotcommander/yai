package requestbuilder

import (
	"context"
	"strings"

	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/errs"
	"github.com/dotcommander/yai/internal/provider"
)

// providerDescriptor defines how to resolve auth and build provider config for
// a single API backend. Adding a new provider is a single map entry.
type providerDescriptor struct {
	envKey     string // required env var name (empty = optional/no key)
	docsURL    string // help URL shown when key is missing
	errLabel   string // human-readable provider name for error wrapping
	defaultURL string // fallback BaseURL when api.BaseURL is empty
	mapAPI     string // override the API field in provider.Config (e.g. azure-ad → azure)
	copyUser   bool   // when true, copy api.User → cfg.User
	thinking   bool   // when true, forward mod.ThinkingBudget to provider.Config
}

// providerRegistry maps API names to their config descriptors.
var providerRegistry = map[string]providerDescriptor{
	"openrouter": {envKey: "OPENROUTER_API_KEY", docsURL: "https://openrouter.ai/keys", errLabel: "OpenRouter"},
	"vercel":     {envKey: "VERCEL_API_KEY", docsURL: "https://vercel.com/dashboard/tokens", errLabel: "Vercel AI Gateway"},
	"bedrock":    {errLabel: "Bedrock"},
	"cohere":     {envKey: "COHERE_API_KEY", docsURL: "https://dashboard.cohere.com/api-keys", errLabel: "Cohere"},
	"ollama":     {defaultURL: "http://localhost:11434/v1"},
	"azure":      {envKey: "AZURE_OPENAI_KEY", docsURL: "https://aka.ms/oai/access", errLabel: "Azure", copyUser: true},
	"azure-ad":   {envKey: "AZURE_OPENAI_KEY", docsURL: "https://aka.ms/oai/access", errLabel: "Azure", mapAPI: "azure", copyUser: true},
	"anthropic":  {envKey: "ANTHROPIC_API_KEY", docsURL: "https://console.anthropic.com/settings/keys", errLabel: "Anthropic"},
	"google":     {envKey: "GOOGLE_API_KEY", docsURL: "https://aistudio.google.com/app/apikey", errLabel: "Google", thinking: true},
}

// defaultProvider is used for unrecognized API names (OpenAI-compatible).
var defaultProvider = providerDescriptor{
	envKey:   "OPENAI_API_KEY",
	docsURL:  "https://platform.openai.com/account/api-keys",
	errLabel: "OpenAI",
}

// PrepareProviderConfig builds the provider config for the selected model/API.
func PrepareProviderConfig(ctx context.Context, mod config.Model, api config.API, cfg *config.Config) (provider.Config, error) {
	desc, ok := providerRegistry[mod.API]
	if !ok {
		desc = defaultProvider
	}

	var key string
	var err error
	if desc.envKey != "" {
		key, err = ensureKey(ctx, api, desc.envKey, desc.docsURL)
	} else {
		key, err = optionalKey(ctx, api)
	}
	if err != nil {
		return provider.Config{}, errs.Wrap(err, desc.errLabel+" authentication failed")
	}

	baseURL := api.BaseURL
	if baseURL == "" && desc.defaultURL != "" {
		baseURL = desc.defaultURL
	}

	providerAPI := mod.API
	if desc.mapAPI != "" {
		providerAPI = desc.mapAPI
	}

	if desc.copyUser && api.User != "" {
		cfg.User = api.User
	}

	pcfg := provider.Config{API: providerAPI, APIKey: key, BaseURL: baseURL}
	if desc.thinking {
		pcfg.ThinkingBudget = mod.ThinkingBudget
	}

	return pcfg, nil
}

// ApplyHTTPConfig configures the provider HTTP client with hardened transport
// timeouts. When httpProxy is non-empty, the transport is additionally
// configured to route through the given HTTP proxy.
func ApplyHTTPConfig(httpProxy string, providerCfg *provider.Config) error {
	httpClient, err := config.NewHTTPClient(httpProxy)
	if err != nil {
		if strings.Contains(err.Error(), "parse proxy") {
			return errs.Wrap(err, "There was an error parsing your proxy URL.")
		}
		return errs.Wrap(err, "Could not configure HTTP transport.")
	}
	providerCfg.HTTPClient = httpClient
	return nil
}
