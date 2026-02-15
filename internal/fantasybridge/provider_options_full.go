//go:build !yai_small

package fantasybridge

import (
	"charm.land/fantasy"
	fgoogle "charm.land/fantasy/providers/google"
	fopenai "charm.land/fantasy/providers/openai"
	fopenaicompat "charm.land/fantasy/providers/openaicompat"
	"github.com/dotcommander/yai/internal/proto"
)

func applyProviderOptions(call *fantasy.Call, api string, cfg Config, req proto.Request) {
	openAIOpts := &fopenai.ProviderOptions{}
	hasOpenAIOpts := false

	if req.User != "" {
		user := req.User
		switch api {
		case apiOpenAI, apiAzure, apiAzureAD:
			openAIOpts.User = &user
			hasOpenAIOpts = true
		case apiAnthropic, apiGoogle, "openrouter", "vercel", "bedrock":
			// no-op
		default:
			call.ProviderOptions[fopenaicompat.Name] = &fopenaicompat.ProviderOptions{User: &user}
		}
	}

	if req.MaxCompletionTokens != nil {
		switch api {
		case apiOpenAI, apiAzure, apiAzureAD:
			openAIOpts.MaxCompletionTokens = req.MaxCompletionTokens
			hasOpenAIOpts = true
		}
	}

	if hasOpenAIOpts {
		call.ProviderOptions[fopenai.Name] = openAIOpts
	}

	if api == apiGoogle && cfg.ThinkingBudget > 0 {
		call.ProviderOptions[fgoogle.Name] = &fgoogle.ProviderOptions{
			ThinkingConfig: &fgoogle.ThinkingConfig{
				ThinkingBudget: fantasy.Opt(int64(cfg.ThinkingBudget)),
			},
		}
	}
}
