package provider

import "charm.land/fantasy"

func newProvider(cfg Config) (fantasy.Provider, error) {
	api := cfg.API
	if api == apiAzureAD {
		api = apiAzure
	}

	factory, ok := factories[api]
	if !ok {
		factory = newOpenAICompat
	}

	return factory(cfg.API, cfg.APIKey, cfg.BaseURL, cfg.HTTPClient)
}
