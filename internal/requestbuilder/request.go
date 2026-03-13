// Package requestbuilder assembles LLM requests from configuration and prompt context.
package requestbuilder

import (
	"context"
	"fmt"
	"strings"

	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/errs"
	"github.com/dotcommander/yai/internal/proto"
	"github.com/dotcommander/yai/internal/provider"
	"github.com/dotcommander/yai/internal/storage/cache"
)

// PreparedStream contains prebuilt stream input for an LLM request.
//
// This keeps request/model/provider assembly distinct from stream execution.
type PreparedStream struct {
	Model    config.Model
	Provider provider.Config
	Request  proto.Request
}

// BuildPreparedFromPrompt resolves provider/model and builds a request from the
// current prompt context.
func BuildPreparedFromPrompt(
	ctx context.Context,
	cfg *config.Config,
	cacheStore *cache.Conversations,
	prompt string,
) (PreparedStream, error) {
	return buildPreparedStream(ctx, cfg, func(mod config.Model) (proto.Request, error) {
		return BuildRequestFromPrompt(cfg, mod, cacheStore, prompt)
	})
}

// BuildPreparedFromHistory resolves provider/model and builds a request using
// existing conversation history.
func BuildPreparedFromHistory(
	ctx context.Context,
	cfg *config.Config,
	history []proto.Message,
	prompt string,
) (PreparedStream, error) {
	return buildPreparedStream(ctx, cfg, func(mod config.Model) (proto.Request, error) {
		return BuildRequestFromHistory(cfg, mod, history, prompt)
	})
}

func buildPreparedStream(
	ctx context.Context,
	cfg *config.Config,
	buildRequest func(config.Model) (proto.Request, error),
) (PreparedStream, error) {
	api, mod, err := ResolveModel(cfg)
	if err != nil {
		return PreparedStream{}, err
	}

	providerCfg, err := PrepareProviderConfig(ctx, mod, api, cfg)
	if err != nil {
		return PreparedStream{}, err
	}
	if err := ApplyHTTPConfig(cfg.HTTPProxy, &providerCfg); err != nil {
		return PreparedStream{}, err
	}

	req, err := buildRequest(mod)
	if err != nil {
		return PreparedStream{}, err
	}

	return PreparedStream{Model: mod, Provider: providerCfg, Request: req}, nil
}

// BuildRequestFromPrompt creates a prompt-only request, optionally loading a
// cached conversation when cache reading is configured.
func BuildRequestFromPrompt(cfg *config.Config, mod config.Model, cacheStore *cache.Conversations, prompt string) (proto.Request, error) {
	messages, err := buildSystemMessages(cfg)
	if err != nil {
		return proto.Request{}, err
	}

	if cfg.Prefix != "" {
		prompt = strings.TrimSpace(cfg.Prefix + "\n\n" + prompt)
	}

	if !cfg.NoCache && cfg.CacheReadFromID != "" {
		if cacheStore == nil {
			return proto.Request{}, errs.Error{Reason: "Cache is not available"}
		}
		if err := cacheStore.Read(cfg.CacheReadFromID, &messages); err != nil {
			return proto.Request{}, errs.Wrap(err, "There was a problem reading the cache. Use --no-cache / NO_CACHE to disable it.")
		}
	}

	prompt = applyInputLimit(cfg, mod, prompt)

	messages = append(messages, proto.Message{Role: proto.RoleUser, Content: prompt})

	return BuildRequest(cfg, mod, messages), nil
}

// BuildRequestFromHistory creates a request using existing conversation messages.
func BuildRequestFromHistory(cfg *config.Config, mod config.Model, history []proto.Message, prompt string) (proto.Request, error) {
	messages, err := buildSystemMessages(cfg)
	if err != nil {
		return proto.Request{}, err
	}

	// 75% of the character budget goes to history; the remaining 25% is
	// reserved for the new prompt and system messages.
	historyBudget := int64(0)
	if mod.MaxChars > 0 {
		historyBudget = mod.MaxChars * 3 / 4
	}
	for _, msg := range windowHistory(history, historyBudget) {
		if msg.Role != proto.RoleSystem {
			messages = append(messages, msg)
		}
	}

	prompt = applyInputLimit(cfg, mod, prompt)

	messages = append(messages, proto.Message{Role: proto.RoleUser, Content: prompt})
	return BuildRequest(cfg, mod, messages), nil
}

// applyInputLimit defaults MaxChars from config and truncates the prompt when
// input limiting is enabled.
func applyInputLimit(cfg *config.Config, mod config.Model, prompt string) string {
	maxChars := mod.MaxChars
	if maxChars == 0 {
		maxChars = cfg.MaxInputChars
	}
	if !cfg.NoLimit && maxChars > 0 && int64(len(prompt)) > maxChars {
		return prompt[:maxChars]
	}
	return prompt
}

func windowHistory(history []proto.Message, budgetChars int64) []proto.Message {
	if budgetChars <= 0 || len(history) == 0 {
		return history
	}
	var total int64
	start := len(history)
	for i := len(history) - 1; i >= 0; i-- {
		total += int64(len(history[i].Content))
		if total > budgetChars {
			break
		}
		start = i
	}
	if start >= len(history) {
		start = len(history) - 1
	}
	return history[start:]
}

func buildSystemMessages(cfg *config.Config) ([]proto.Message, error) {
	messages := make([]proto.Message, 0, 8)

	if txt := cfg.FormatText[cfg.FormatAs]; cfg.Format && txt != "" {
		messages = append(messages, proto.Message{Role: proto.RoleSystem, Content: txt})
	}

	if cfg.Role != "" {
		roleSetup, ok := cfg.Roles[cfg.Role]
		if !ok {
			return nil, errs.Wrap(fmt.Errorf("role %q does not exist", cfg.Role), "Could not use role")
		}
		for _, msg := range roleSetup {
			content, err := config.LoadMsg(msg, cfg.HTTPProxy)
			if err != nil {
				return nil, errs.Wrap(err, "Could not use role")
			}
			messages = append(messages, proto.Message{Role: proto.RoleSystem, Content: content})
		}
	}

	return messages, nil
}

// BuildRequest populates a protocol request from prompt context.
func BuildRequest(cfg *config.Config, mod config.Model, messages []proto.Message) proto.Request {
	temperature := (*float64)(nil)
	if cfg.Temperature >= 0 {
		v := cfg.Temperature
		temperature = &v
	}
	topP := (*float64)(nil)
	if cfg.TopP >= 0 {
		v := cfg.TopP
		topP = &v
	}
	topK := (*int64)(nil)
	if cfg.TopK >= 0 {
		v := cfg.TopK
		topK = &v
	}

	if IsReasoningModel(mod.Name) {
		temperature = nil
		topP = nil
		topK = nil
	}

	request := proto.Request{
		Messages:    messages,
		API:         mod.API,
		Model:       mod.Name,
		User:        cfg.User,
		Temperature: temperature,
		TopP:        topP,
		TopK:        topK,
		Stop:        cfg.Stop,
	}

	if cfg.MaxTokens > 0 && !IsReasoningModel(mod.Name) {
		request.MaxTokens = &cfg.MaxTokens
	}
	if cfg.MaxCompletionTokens > 0 {
		request.MaxCompletionTokens = &cfg.MaxCompletionTokens
	}

	return request
}
