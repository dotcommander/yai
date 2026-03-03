package cmd

import (
	"context"
	"fmt"

	"github.com/dotcommander/yai/internal/agent"
	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/proto"
	"github.com/dotcommander/yai/internal/requestbuilder"
	"github.com/dotcommander/yai/internal/storage/cache"
)

func makePromptStreamStarter(
	cfg *config.Config,
	cacheStore *cache.Conversations,
	agentSvc *agent.Service,
) func(context.Context, string) (agent.StreamStart, error) {
	return func(ctx context.Context, content string) (agent.StreamStart, error) {
		prepared, err := requestbuilder.BuildPreparedFromPrompt(ctx, cfg, cacheStore, content)
		if err != nil {
			return agent.StreamStart{}, fmt.Errorf("build request: %w", err)
		}

		return agentSvc.StreamFromPrepared(ctx, prepared)
	}
}

func makeHistoryStreamStarter(
	cfg *config.Config,
	agentSvc *agent.Service,
) func(context.Context, []proto.Message, string) (agent.StreamStart, error) {
	return func(ctx context.Context, history []proto.Message, prompt string) (agent.StreamStart, error) {
		prepared, err := requestbuilder.BuildPreparedFromHistory(ctx, cfg, history, prompt)
		if err != nil {
			return agent.StreamStart{}, fmt.Errorf("build request: %w", err)
		}

		return agentSvc.StreamFromPrepared(ctx, prepared)
	}
}
