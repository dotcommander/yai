package agent

import (
	"testing"

	"github.com/dotcommander/yai/internal/fantasybridge"
	"github.com/stretchr/testify/require"
)

func TestNewFantasyClientRouting(t *testing.T) {
	t.Run("azure-ad returns fantasy client", func(t *testing.T) {
		client, err := NewFantasyClient(
			fantasybridge.Config{API: "azure", APIKey: "token", BaseURL: "https://example.openai.azure.com"},
		)
		require.NoError(t, err)
		require.NotNil(t, client)
	})

	t.Run("supported core provider returns fantasy client", func(t *testing.T) {
		client, err := NewFantasyClient(fantasybridge.Config{API: "openai"})
		require.NoError(t, err)
		require.NotNil(t, client)
	})

	t.Run("openai-compatible custom api returns fantasy client", func(t *testing.T) {
		client, err := NewFantasyClient(
			fantasybridge.Config{API: "deepseek", BaseURL: "https://api.deepseek.com"},
		)
		require.NoError(t, err)
		require.NotNil(t, client)
	})

	t.Run("cohere returns fantasy client", func(t *testing.T) {
		client, err := NewFantasyClient(
			fantasybridge.Config{API: "cohere", APIKey: "token", BaseURL: "https://api.cohere.com/v1"},
		)
		require.NoError(t, err)
		require.NotNil(t, client)
	})

	t.Run("openrouter returns fantasy client", func(t *testing.T) {
		client, err := NewFantasyClient(
			fantasybridge.Config{API: "openrouter", APIKey: "token"},
		)
		require.NoError(t, err)
		require.NotNil(t, client)
	})

	t.Run("vercel returns fantasy client", func(t *testing.T) {
		client, err := NewFantasyClient(
			fantasybridge.Config{API: "vercel", APIKey: "token"},
		)
		require.NoError(t, err)
		require.NotNil(t, client)
	})

	t.Run("ollama returns fantasy client without api key", func(t *testing.T) {
		client, err := NewFantasyClient(
			fantasybridge.Config{API: "ollama", BaseURL: "http://localhost:11434/v1"},
		)
		require.NoError(t, err)
		require.NotNil(t, client)
	})

	t.Run("missing provider config returns error", func(t *testing.T) {
		client, err := NewFantasyClient(fantasybridge.Config{})
		require.Error(t, err)
		require.Nil(t, client)
	})
}

func TestApplyProxyConfigIncludesFantasyClient(t *testing.T) {
	providerCfg := fantasybridge.Config{}
	err := ApplyProxyConfig("http://127.0.0.1:8080", &providerCfg)
	require.NoError(t, err)
	require.NotNil(t, providerCfg.HTTPClient)
}
