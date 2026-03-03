package requestbuilder

import (
	"context"
	"testing"

	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/proto"
	"github.com/stretchr/testify/require"
)

func TestResolveModel(t *testing.T) {
	cfg := &config.Config{Settings: config.Settings{
		APIs: config.APIs{
			{
				Name: "openai",
				Models: map[string]config.Model{
					"gpt-4.1": {
						Aliases: []string{"gpt-four"},
					},
					"gpt-5": {},
				},
			},
		},
		API:   "openai",
		Model: "gpt-four",
	}}

	api, mod, err := ResolveModel(cfg)
	require.NoError(t, err)
	require.Equal(t, "openai", api.Name)
	require.Equal(t, "gpt-4.1", mod.Name)
	require.Equal(t, "gpt-4.1", cfg.Model)
}

func TestResolveModelMissingModelRequiresAPI(t *testing.T) {
	cfg := &config.Config{Settings: config.Settings{
		APIs: config.APIs{
			{
				Name: "openai",
				Models: map[string]config.Model{
					"gpt-4.1": {},
				},
			},
		},
		API:   "openai",
		Model: "missing",
	}}

	_, _, err := ResolveModel(cfg)
	require.Error(t, err)
}

func TestBuildRequestFromHistoryAddsSystemMessagesAndSkipsHistorySystem(t *testing.T) {
	cfg := &config.Config{Settings: config.Settings{
		Format: true,
		FormatText: config.FormatText{
			"markdown": "format this",
		},
		FormatAs: "markdown",
		Role:     "assistant",
		Roles: map[string][]string{
			"assistant": {
				"you are concise",
			},
		},
	}}

	mod := config.Model{Name: "gpt-4.1", MaxChars: 100000}
	history := []proto.Message{
		{Role: proto.RoleSystem, Content: "legacy system"},
		{Role: proto.RoleUser, Content: "first"},
		{Role: proto.RoleAssistant, Content: "reply"},
	}

	req, err := BuildRequestFromHistory(cfg, mod, history, "new prompt")
	require.NoError(t, err)
	require.Len(t, req.Messages, 5)
	require.Equal(t, proto.RoleSystem, req.Messages[0].Role)
	require.Equal(t, proto.RoleSystem, req.Messages[1].Role)
	require.Equal(t, proto.RoleUser, req.Messages[2].Role)
	require.Equal(t, proto.RoleAssistant, req.Messages[3].Role)
	require.Equal(t, proto.RoleUser, req.Messages[4].Role)
	require.Equal(t, "new prompt", req.Messages[4].Content)
}

func TestBuildRequestFromHistoryTruncatesPromptWhenLimited(t *testing.T) {
	cfg := &config.Config{}
	mod := config.Model{Name: "gpt-4.1", MaxChars: 5}
	req, err := BuildRequestFromHistory(cfg, mod, nil, "abcdefghijkl")
	require.NoError(t, err)
	require.Equal(t, "abcde", req.Messages[0].Content)

	cfg.NoLimit = true
	req, err = BuildRequestFromHistory(cfg, mod, nil, "abcdefghijkl")
	require.NoError(t, err)
	require.Equal(t, "abcdefghijkl", req.Messages[0].Content)
}

func TestIsReasoningModel(t *testing.T) {
	require.True(t, IsReasoningModel("gpt-5-claude"))
	require.True(t, IsReasoningModel("o1-mini"))
	require.True(t, IsReasoningModel("  O4-mini  "))
	require.False(t, IsReasoningModel("gpt-4o"))
}

func TestBuildRequestDropsSamplingForReasoningModel(t *testing.T) {
	cfg := &config.Config{Settings: config.Settings{Temperature: 1, TopP: 0.9, TopK: 40}}
	mod := config.Model{Name: "gpt-5"}
	req := BuildRequest(cfg, mod, nil)
	require.Nil(t, req.Temperature)
	require.Nil(t, req.TopP)
	require.Nil(t, req.TopK)
}

func TestBuildPreparedFromPrompt(t *testing.T) {
	cfg := &config.Config{
		Settings: config.Settings{
			APIs: config.APIs{
				{
					Name:   "openai",
					APIKey: "test-key",
					Models: map[string]config.Model{
						"gpt-4.1": {},
					},
				},
			},
			Model: "gpt-4.1",
			API:   "openai",
		},
	}

	prepared, err := BuildPreparedFromPrompt(context.Background(), cfg, nil, "hello")
	require.NoError(t, err)
	require.Equal(t, "openai", prepared.Provider.API)
	require.Equal(t, "test-key", prepared.Provider.APIKey)
	require.Equal(t, "gpt-4.1", prepared.Request.Model)
	require.Len(t, prepared.Request.Messages, 1)
	require.Equal(t, proto.RoleUser, prepared.Request.Messages[0].Role)
	require.Equal(t, "hello", prepared.Request.Messages[0].Content)
}

func TestBuildPreparedFromHistory(t *testing.T) {
	cfg := &config.Config{
		Settings: config.Settings{
			APIs: config.APIs{
				{
					Name:   "openai",
					APIKey: "test-key",
					Models: map[string]config.Model{
						"gpt-4.1": {},
					},
				},
			},
			Model: "gpt-4.1",
			API:   "openai",
		},
	}

	prepared, err := BuildPreparedFromHistory(context.Background(), cfg, []proto.Message{{
		Role:    proto.RoleSystem,
		Content: "system should be excluded",
	}, {
		Role:    proto.RoleUser,
		Content: "first prompt",
	}}, "follow up")
	require.NoError(t, err)
	require.Equal(t, "gpt-4.1", prepared.Request.Model)
	require.Len(t, prepared.Request.Messages, 2)
	require.Equal(t, proto.RoleUser, prepared.Request.Messages[0].Role)
	require.Equal(t, proto.RoleUser, prepared.Request.Messages[1].Role)
	require.Equal(t, "follow up", prepared.Request.Messages[1].Content)
}
