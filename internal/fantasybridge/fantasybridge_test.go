package fantasybridge

import (
	"testing"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/google"
	fopenai "charm.land/fantasy/providers/openai"
	fopenaicompat "charm.land/fantasy/providers/openaicompat"
	"github.com/dotcommander/yai/internal/proto"
	"github.com/stretchr/testify/require"
)

func TestBuildCallGoogleThinkingBudget(t *testing.T) {
	s := &Stream{
		api: "google",
		config: Config{
			ThinkingBudget: 256,
		},
		request: proto.Request{},
	}

	call := s.buildCall()

	v, ok := call.ProviderOptions[google.Name]
	require.True(t, ok)
	opts, ok := v.(*google.ProviderOptions)
	require.True(t, ok)
	require.NotNil(t, opts.ThinkingConfig)
	require.NotNil(t, opts.ThinkingConfig.ThinkingBudget)
	require.EqualValues(t, 256, *opts.ThinkingConfig.ThinkingBudget)
}

func TestBuildCallNonGoogleNoThinkingBudgetOption(t *testing.T) {
	s := &Stream{
		api: "openai",
		config: Config{
			ThinkingBudget: 512,
		},
		request: proto.Request{},
	}

	call := s.buildCall()
	require.Empty(t, call.ProviderOptions)
}

func TestNewAzureADProviderAlias(t *testing.T) {
	client, err := New(Config{
		API:     "azure-ad",
		APIKey:  "token",
		BaseURL: "https://example.openai.azure.com",
	})
	require.NoError(t, err)
	require.NotNil(t, client)
}

func TestBuildCallUserProviderOptions(t *testing.T) {
	t.Run("openai user propagates to openai provider options", func(t *testing.T) {
		s := &Stream{
			api: "openai",
			request: proto.Request{
				User: "alice",
			},
		}

		call := s.buildCall()
		v, ok := call.ProviderOptions[fopenai.Name]
		require.True(t, ok)
		opts, ok := v.(*fopenai.ProviderOptions)
		require.True(t, ok)
		require.NotNil(t, opts.User)
		require.Equal(t, "alice", *opts.User)
	})

	t.Run("azure user propagates to openai provider options", func(t *testing.T) {
		s := &Stream{
			api: "azure",
			request: proto.Request{
				User: "dana",
			},
		}

		call := s.buildCall()
		v, ok := call.ProviderOptions[fopenai.Name]
		require.True(t, ok)
		opts, ok := v.(*fopenai.ProviderOptions)
		require.True(t, ok)
		require.NotNil(t, opts.User)
		require.Equal(t, "dana", *opts.User)
	})

	t.Run("custom openai-compatible user propagates to compat options", func(t *testing.T) {
		s := &Stream{
			api: "deepseek",
			request: proto.Request{
				User: "bob",
			},
		}

		call := s.buildCall()
		v, ok := call.ProviderOptions[fopenaicompat.Name]
		require.True(t, ok)
		opts, ok := v.(*fopenaicompat.ProviderOptions)
		require.True(t, ok)
		require.NotNil(t, opts.User)
		require.Equal(t, "bob", *opts.User)
	})

	t.Run("google does not attach user provider option", func(t *testing.T) {
		s := &Stream{
			api: "google",
			request: proto.Request{
				User: "carol",
			},
		}

		call := s.buildCall()
		_, hasOpenAI := call.ProviderOptions[fopenai.Name]
		_, hasCompat := call.ProviderOptions[fopenaicompat.Name]
		require.False(t, hasOpenAI)
		require.False(t, hasCompat)
	})
}

func TestBuildCallMaxCompletionTokensProviderOptions(t *testing.T) {
	tokens := int64(321)

	t.Run("openai uses provider max completion tokens", func(t *testing.T) {
		s := &Stream{
			api: "openai",
			request: proto.Request{
				MaxCompletionTokens: &tokens,
			},
		}

		call := s.buildCall()
		v, ok := call.ProviderOptions[fopenai.Name]
		require.True(t, ok)
		opts, ok := v.(*fopenai.ProviderOptions)
		require.True(t, ok)
		require.NotNil(t, opts.MaxCompletionTokens)
		require.EqualValues(t, 321, *opts.MaxCompletionTokens)
	})

	t.Run("openai-compatible does not set max completion tokens", func(t *testing.T) {
		s := &Stream{
			api: "deepseek",
			request: proto.Request{
				MaxCompletionTokens: &tokens,
			},
		}

		call := s.buildCall()
		_, hasCompat := call.ProviderOptions[fopenaicompat.Name]
		require.False(t, hasCompat)
	})
}

func TestConsumePartSkipsProviderExecutedToolCalls(t *testing.T) {
	s := &Stream{stepToolCallSeen: map[string]struct{}{}}

	s.consumePart(fantasy.StreamPart{
		Type:             fantasy.StreamPartTypeToolCall,
		ID:               "tc_1",
		ToolCallName:     "tool",
		ToolCallInput:    "{}",
		ProviderExecuted: true,
	})

	require.Empty(t, s.stepToolCalls)
}

func TestDrainWarningsDeduplicates(t *testing.T) {
	s := &Stream{warningSeen: map[string]struct{}{}}

	s.consumePart(fantasy.StreamPart{
		Type: fantasy.StreamPartTypeWarnings,
		Warnings: []fantasy.CallWarning{
			{Type: fantasy.CallWarningTypeUnsupportedSetting, Setting: "top_k", Message: "unsupported setting: top_k"},
			{Type: fantasy.CallWarningTypeUnsupportedSetting, Setting: "top_k", Message: "unsupported setting: top_k"},
		},
	})

	warnings := s.DrainWarnings()
	require.Equal(t, []string{"unsupported setting: top_k"}, warnings)
	require.Empty(t, s.DrainWarnings())
}
