package agent

import (
	"context"
	"fmt"

	mmcp "github.com/mark3labs/mcp-go/mcp"

	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/errs"
	"github.com/dotcommander/yai/internal/mcp"
	"github.com/dotcommander/yai/internal/present"
	"github.com/dotcommander/yai/internal/proto"
	"github.com/dotcommander/yai/internal/provider"
	"github.com/dotcommander/yai/internal/requestbuilder"
	"github.com/dotcommander/yai/internal/storage/cache"
	"github.com/dotcommander/yai/internal/stream"
)

// ClientFactory creates a stream.Client from a provider configuration.
// It allows tests to replace the real Fantasy bridge with a stub.
type ClientFactory func(provider.Config) (stream.Client, error)

// Service is the core orchestration layer for starting LLM streams.
//
// It is intentionally UI-agnostic and can be used by both the TUI and headless
// commands.
type Service struct {
	cfg           *config.Config
	cache         *cache.Conversations
	mcp           *mcp.Service
	clientFactory ClientFactory
}

// New creates an agent service. An optional ClientFactory can be provided for
// testing; when nil, the default Fantasy bridge client is used.
func New(cfg *config.Config, cache *cache.Conversations, mcpSvc *mcp.Service, opts ...ClientFactory) *Service {
	if mcpSvc == nil {
		mcpSvc = mcp.New(cfg)
	}
	factory := ClientFactory(NewFantasyClient)
	if len(opts) > 0 && opts[0] != nil {
		factory = opts[0]
	}
	return &Service{cfg: cfg, cache: cache, mcp: mcpSvc, clientFactory: factory}
}

// StreamStart contains the stream plus metadata about the resolved request.
type StreamStart struct {
	Stream   stream.Stream
	Model    config.Model
	Messages []proto.Message
}

// PreparedStream contains pre-resolved stream input prepared by higher layers.
type PreparedStream = requestbuilder.PreparedStream

// Stream starts a streaming completion for the given prompt.
func (s *Service) Stream(ctx context.Context, prompt string) (StreamStart, error) {
	prepared, err := requestbuilder.BuildPreparedFromPrompt(ctx, s.cfg, s.cache, prompt)
	if err != nil {
		return StreamStart{}, fmt.Errorf("build request: %w", err)
	}

	return s.StreamFromPrepared(ctx, prepared)
}

// StreamContinue starts a streaming completion using pre-built conversation
// history. It prepends system messages (format + role) to the provided history
// and appends the new user message. This avoids per-turn disk I/O and prevents
// system message duplication across turns.
func (s *Service) StreamContinue(ctx context.Context, history []proto.Message, prompt string) (StreamStart, error) {
	prepared, err := requestbuilder.BuildPreparedFromHistory(ctx, s.cfg, history, prompt)
	if err != nil {
		return StreamStart{}, fmt.Errorf("build request: %w", err)
	}

	return s.StreamFromPrepared(ctx, prepared)
}

// StreamFromPrepared starts a stream from pre-built request data.
func (s *Service) StreamFromPrepared(ctx context.Context, prepared PreparedStream) (StreamStart, error) {
	return s.startStream(ctx, prepared.Request, prepared.Model, prepared.Provider)
}

func (s *Service) startStream(ctx context.Context, req proto.Request, mod config.Model, providerCfg provider.Config) (StreamStart, error) {
	cfg := s.cfg

	toolsEnabled := cfg.MCPAllowNonTTY || present.IsInputTTY()

	var tools map[string][]mmcp.Tool
	if toolsEnabled {
		toolsCtx, cancel := context.WithTimeout(ctx, cfg.MCPTimeout)
		var err error
		tools, err = s.mcp.Tools(toolsCtx)
		cancel()
		if err != nil {
			return StreamStart{}, fmt.Errorf("mcp tools: %w", err)
		}
	}

	if toolsEnabled {
		req.Tools = tools
		req.ToolCaller = func(name string, data []byte) (string, error) {
			callCtx, cancel := context.WithTimeout(ctx, cfg.MCPTimeout)
			defer cancel()
			return s.mcp.CallTool(callCtx, name, data)
		}
	}

	client, err := s.clientFactory(providerCfg)
	if err != nil {
		return StreamStart{}, err
	}

	st := client.Request(ctx, req)
	return StreamStart{Stream: st, Model: mod, Messages: req.Messages}, nil
}

// ApplyHTTPConfig configures the provider HTTP client with hardened transport
// timeouts and an optional HTTP proxy.
func ApplyHTTPConfig(httpProxy string, providerCfg *provider.Config) error {
	if err := requestbuilder.ApplyHTTPConfig(httpProxy, providerCfg); err != nil {
		return fmt.Errorf("apply http config: %w", err)
	}
	return nil
}

// NewFantasyClient creates the fantasy bridge client.
func NewFantasyClient(cfg provider.Config) (stream.Client, error) {
	if cfg.API == "" {
		return nil, errs.Error{Reason: "missing fantasy provider configuration"}
	}
	client, err := provider.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("new fantasy bridge client: %w", err)
	}
	return client, nil
}
