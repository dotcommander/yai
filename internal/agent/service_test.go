package agent

import (
	"context"
	"testing"

	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/fantasybridge"
	"github.com/dotcommander/yai/internal/proto"
	"github.com/dotcommander/yai/internal/stream"
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

func TestNewWithClientFactory(t *testing.T) {
	t.Run("New() without factory uses default", func(t *testing.T) {
		cfg := &config.Config{}
		svc := New(cfg, nil, nil)
		require.NotNil(t, svc)
		require.NotNil(t, svc.clientFactory)
	})

	t.Run("New() with custom factory uses that factory", func(t *testing.T) {
		cfg := &config.Config{}
		customFactory := func(fantasybridge.Config) (stream.Client, error) {
			return &stubClient{}, nil
		}
		svc := New(cfg, nil, nil, customFactory)
		require.NotNil(t, svc)
		require.NotNil(t, svc.clientFactory)
	})

	t.Run("Stream() calls the injected factory", func(t *testing.T) {
		factoryCalled := false
		customFactory := func(fantasybridge.Config) (stream.Client, error) {
			factoryCalled = true
			return &stubClient{}, nil
		}

		cfg := &config.Config{
			Settings: config.Settings{
				APIs: config.APIs{
					{
						Name:   "anthropic",
						APIKey: "test-key",
						Models: map[string]config.Model{
							"claude-3-sonnet-20240229": {MaxChars: 100000},
						},
					},
				},
				Model: "claude-3-sonnet-20240229",
				API:   "anthropic",
			},
		}

		svc := New(cfg, nil, nil, customFactory)
		_, err := svc.Stream(context.Background(), "test prompt")
		require.NoError(t, err)
		require.True(t, factoryCalled, "custom factory should have been called")
	})
}

// stubClient is a test double for stream.Client.
type stubClient struct{}

func (s *stubClient) Request(ctx context.Context, req proto.Request) stream.Stream {
	return &stubStream{}
}

// stubStream is a test double for stream.Stream.
type stubStream struct{}

func (s *stubStream) Next() bool                      { return false }
func (s *stubStream) Current() (proto.Chunk, error)   { return proto.Chunk{}, nil }
func (s *stubStream) Err() error                      { return nil }
func (s *stubStream) Close() error                    { return nil }
func (s *stubStream) Messages() []proto.Message       { return nil }
func (s *stubStream) CallTools() []proto.ToolCallStatus { return nil }
func (s *stubStream) DrainWarnings() []string         { return nil }
