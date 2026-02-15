package fantasybridge

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/anthropic"
	"charm.land/fantasy/providers/azure"
	"charm.land/fantasy/providers/bedrock"
	fgoogle "charm.land/fantasy/providers/google"
	fopenai "charm.land/fantasy/providers/openai"
	fopenaicompat "charm.land/fantasy/providers/openaicompat"
	"charm.land/fantasy/providers/openrouter"
	"charm.land/fantasy/providers/vercel"
	"github.com/dotcommander/yai/internal/proto"
	"github.com/dotcommander/yai/internal/stream"
)

var _ stream.Client = &Client{}

const (
	apiAnthropic = "anthropic"
	apiGoogle    = "google"
	apiOpenAI    = "openai"
	apiAzure     = "azure"
	apiAzureAD   = "azure-ad"
)

// Config represents provider configuration used by the fantasy bridge.
type Config struct {
	API            string
	BaseURL        string
	APIKey         string
	HTTPClient     *http.Client
	ThinkingBudget int
}

// Client is a stream.Client backed by charm.land/fantasy.
type Client struct {
	provider fantasy.Provider
	config   Config
}

// New creates a new Fantasy-backed stream client.
func New(cfg Config) (*Client, error) {
	provider, err := newProvider(cfg)
	if err != nil {
		return nil, err
	}
	return &Client{provider: provider, config: cfg}, nil
}

func newProvider(cfg Config) (fantasy.Provider, error) {
	switch cfg.API {
	case apiOpenAI:
		opts := []fopenai.Option{fopenai.WithAPIKey(cfg.APIKey)}
		if cfg.BaseURL != "" {
			opts = append(opts, fopenai.WithBaseURL(cfg.BaseURL))
		}
		if cfg.HTTPClient != nil {
			opts = append(opts, fopenai.WithHTTPClient(cfg.HTTPClient))
		}
		provider, err := fopenai.New(opts...)
		if err != nil {
			return nil, fmt.Errorf("new fantasy openai provider: %w", err)
		}
		return provider, nil
	case apiAnthropic:
		opts := []anthropic.Option{anthropic.WithAPIKey(cfg.APIKey)}
		if cfg.BaseURL != "" {
			opts = append(opts, anthropic.WithBaseURL(strings.TrimSuffix(cfg.BaseURL, "/v1")))
		}
		if cfg.HTTPClient != nil {
			opts = append(opts, anthropic.WithHTTPClient(cfg.HTTPClient))
		}
		provider, err := anthropic.New(opts...)
		if err != nil {
			return nil, fmt.Errorf("new fantasy anthropic provider: %w", err)
		}
		return provider, nil
	case apiGoogle:
		opts := []fgoogle.Option{fgoogle.WithGeminiAPIKey(cfg.APIKey)}
		if cfg.BaseURL != "" {
			opts = append(opts, fgoogle.WithBaseURL(cfg.BaseURL))
		}
		if cfg.HTTPClient != nil {
			opts = append(opts, fgoogle.WithHTTPClient(cfg.HTTPClient))
		}
		provider, err := fgoogle.New(opts...)
		if err != nil {
			return nil, fmt.Errorf("new fantasy google provider: %w", err)
		}
		return provider, nil
	case apiAzure, apiAzureAD:
		opts := []azure.Option{azure.WithAPIKey(cfg.APIKey), azure.WithBaseURL(cfg.BaseURL)}
		if cfg.HTTPClient != nil {
			opts = append(opts, azure.WithHTTPClient(cfg.HTTPClient))
		}
		provider, err := azure.New(opts...)
		if err != nil {
			return nil, fmt.Errorf("new fantasy azure provider: %w", err)
		}
		return provider, nil
	case "openrouter":
		opts := []openrouter.Option{openrouter.WithAPIKey(cfg.APIKey)}
		if cfg.HTTPClient != nil {
			opts = append(opts, openrouter.WithHTTPClient(cfg.HTTPClient))
		}
		provider, err := openrouter.New(opts...)
		if err != nil {
			return nil, fmt.Errorf("new fantasy openrouter provider: %w", err)
		}
		return provider, nil
	case "vercel":
		opts := []vercel.Option{vercel.WithAPIKey(cfg.APIKey)}
		if cfg.BaseURL != "" {
			opts = append(opts, vercel.WithBaseURL(cfg.BaseURL))
		}
		if cfg.HTTPClient != nil {
			opts = append(opts, vercel.WithHTTPClient(cfg.HTTPClient))
		}
		provider, err := vercel.New(opts...)
		if err != nil {
			return nil, fmt.Errorf("new fantasy vercel provider: %w", err)
		}
		return provider, nil
	case "bedrock":
		opts := []bedrock.Option{}
		if cfg.APIKey != "" {
			opts = append(opts, bedrock.WithAPIKey(cfg.APIKey))
		}
		if cfg.HTTPClient != nil {
			opts = append(opts, bedrock.WithHTTPClient(cfg.HTTPClient))
		}
		provider, err := bedrock.New(opts...)
		if err != nil {
			return nil, fmt.Errorf("new fantasy bedrock provider: %w", err)
		}
		return provider, nil
	default:
		opts := []fopenaicompat.Option{fopenaicompat.WithName(cfg.API)}
		if cfg.APIKey != "" {
			opts = append(opts, fopenaicompat.WithAPIKey(cfg.APIKey))
		}
		if cfg.BaseURL != "" {
			opts = append(opts, fopenaicompat.WithBaseURL(cfg.BaseURL))
		}
		if cfg.HTTPClient != nil {
			opts = append(opts, fopenaicompat.WithHTTPClient(cfg.HTTPClient))
		}
		provider, err := fopenaicompat.New(opts...)
		if err != nil {
			return nil, fmt.Errorf("new fantasy openai-compatible provider: %w", err)
		}
		return provider, nil
	}
}

// Request implements stream.Client.
func (c *Client) Request(ctx context.Context, request proto.Request) stream.Stream {
	streamCtx, cancel := context.WithCancel(ctx)
	s := &Stream{
		ctx:         streamCtx,
		cancel:      cancel,
		provider:    c.provider,
		request:     request,
		messages:    request.Messages,
		api:         c.config.API,
		config:      c.config,
		warningSeen: map[string]struct{}{},
	}
	if err := s.startStep(); err != nil {
		s.err = err
	}
	return s
}

// Stream is a stream.Stream implementation backed by fantasy stream events.
type Stream struct {
	ctx      context.Context
	cancel   context.CancelFunc
	provider fantasy.Provider
	request  proto.Request
	api      string
	config   Config

	mu sync.Mutex

	messages []proto.Message

	partCh chan fantasy.StreamPart
	last   fantasy.StreamPart
	err    error

	stepText         strings.Builder
	stepToolCalls    []proto.ToolCall
	stepToolCallSeen map[string]struct{}
	stepDone         bool
	warningSeen      map[string]struct{}
	pendingWarnings  []string
}

// Next implements stream.Stream.
func (s *Stream) Next() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.err != nil {
		return false
	}

	if s.stepDone {
		if err := s.startStep(); err != nil {
			s.err = err
			return false
		}
	}

	part, ok := <-s.partCh
	if !ok {
		s.finalizeStep()
		return false
	}

	s.last = part
	s.consumePart(part)
	return true
}

// Current implements stream.Stream.
func (s *Stream) Current() (proto.Chunk, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch s.last.Type {
	case fantasy.StreamPartTypeTextDelta:
		return proto.Chunk{Content: s.last.Delta}, nil
	case fantasy.StreamPartTypeError:
		if s.last.Error != nil {
			s.err = s.last.Error
			return proto.Chunk{}, s.last.Error
		}
	case fantasy.StreamPartTypeWarnings,
		fantasy.StreamPartTypeTextStart,
		fantasy.StreamPartTypeTextEnd,
		fantasy.StreamPartTypeReasoningStart,
		fantasy.StreamPartTypeReasoningDelta,
		fantasy.StreamPartTypeReasoningEnd,
		fantasy.StreamPartTypeToolInputStart,
		fantasy.StreamPartTypeToolInputDelta,
		fantasy.StreamPartTypeToolInputEnd,
		fantasy.StreamPartTypeToolCall,
		fantasy.StreamPartTypeToolResult,
		fantasy.StreamPartTypeSource,
		fantasy.StreamPartTypeFinish:
		// no-op
	}

	return proto.Chunk{}, stream.ErrNoContent
}

// Close implements stream.Stream.
func (s *Stream) Close() error {
	s.cancel()
	return nil
}

// Err implements stream.Stream.
func (s *Stream) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Messages implements stream.Stream.
func (s *Stream) Messages() []proto.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.messages
}

// CallTools implements stream.Stream.
func (s *Stream) CallTools() []proto.ToolCallStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	statuses := make([]proto.ToolCallStatus, 0, len(s.stepToolCalls))
	for _, call := range s.stepToolCalls {
		msg, status := stream.CallTool(
			call.ID,
			call.Function.Name,
			call.Function.Arguments,
			s.request.ToolCaller,
		)
		s.messages = append(s.messages, msg)
		statuses = append(statuses, status)
	}

	s.stepToolCalls = nil
	s.stepToolCallSeen = map[string]struct{}{}

	return statuses
}

// DrainWarnings implements stream.Stream.
func (s *Stream) DrainWarnings() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	warnings := append([]string(nil), s.pendingWarnings...)
	s.pendingWarnings = nil
	return warnings
}

func (s *Stream) startStep() error {
	model, err := s.provider.LanguageModel(s.ctx, s.request.Model)
	if err != nil {
		return fmt.Errorf("fantasy language model: %w", err)
	}

	call := s.buildCall()

	seq, err := model.Stream(s.ctx, call)
	if err != nil {
		return fmt.Errorf("fantasy stream: %w", err)
	}

	s.partCh = make(chan fantasy.StreamPart, 64)
	s.stepDone = false
	s.stepText.Reset()
	s.stepToolCalls = nil
	s.stepToolCallSeen = map[string]struct{}{}

	go func() {
		defer close(s.partCh)
		for part := range seq {
			select {
			case <-s.ctx.Done():
				return
			case s.partCh <- part:
			}
		}
	}()

	return nil
}

func (s *Stream) buildCall() fantasy.Call {
	call := fantasy.Call{
		Prompt:          toFantasyPrompt(s.messages),
		MaxOutputTokens: s.request.MaxTokens,
		Temperature:     s.request.Temperature,
		TopP:            s.request.TopP,
		TopK:            s.request.TopK,
		Tools:           fromMCPTools(s.request.Tools),
		ToolChoice:      toolChoiceForRequest(s.request),
		ProviderOptions: fantasy.ProviderOptions{},
	}

	openAIOpts := &fopenai.ProviderOptions{}
	hasOpenAIOpts := false

	if s.request.User != "" {
		user := s.request.User
		switch s.api {
		case apiOpenAI, apiAzure, apiAzureAD:
			openAIOpts.User = &user
			hasOpenAIOpts = true
		case apiAnthropic, apiGoogle, "openrouter", "vercel", "bedrock":
			// no-op
		default:
			call.ProviderOptions[fopenaicompat.Name] = &fopenaicompat.ProviderOptions{User: &user}
		}
	}

	if s.request.MaxCompletionTokens != nil {
		switch s.api {
		case apiOpenAI, apiAzure, apiAzureAD:
			openAIOpts.MaxCompletionTokens = s.request.MaxCompletionTokens
			hasOpenAIOpts = true
		}
	}

	if hasOpenAIOpts {
		call.ProviderOptions[fopenai.Name] = openAIOpts
	}

	if s.api == apiGoogle && s.config.ThinkingBudget > 0 {
		call.ProviderOptions[fgoogle.Name] = &fgoogle.ProviderOptions{
			ThinkingConfig: &fgoogle.ThinkingConfig{
				ThinkingBudget: fantasy.Opt(int64(s.config.ThinkingBudget)),
			},
		}
	}

	return call
}

func (s *Stream) finalizeStep() {
	msg := proto.Message{
		Role:      proto.RoleAssistant,
		Content:   s.stepText.String(),
		ToolCalls: append([]proto.ToolCall(nil), s.stepToolCalls...),
	}
	if msg.Content != "" || len(msg.ToolCalls) > 0 {
		s.messages = append(s.messages, msg)
	}
	s.stepDone = true
}

func (s *Stream) consumePart(part fantasy.StreamPart) {
	switch part.Type {
	case fantasy.StreamPartTypeTextDelta:
		s.stepText.WriteString(part.Delta)
	case fantasy.StreamPartTypeToolCall:
		if part.ProviderExecuted {
			return
		}
		if _, exists := s.stepToolCallSeen[part.ID]; exists {
			return
		}
		s.stepToolCallSeen[part.ID] = struct{}{}
		s.stepToolCalls = append(s.stepToolCalls, proto.ToolCall{
			ID: part.ID,
			Function: proto.Function{
				Name:      part.ToolCallName,
				Arguments: []byte(part.ToolCallInput),
			},
		})
	case fantasy.StreamPartTypeError:
		s.err = part.Error
	case fantasy.StreamPartTypeWarnings:
		for _, warning := range part.Warnings {
			text := strings.TrimSpace(warning.Message)
			if text == "" {
				text = strings.TrimSpace(warning.Details)
			}
			if text == "" && warning.Setting != "" {
				text = fmt.Sprintf("unsupported setting: %s", warning.Setting)
			}
			if text == "" {
				text = "provider warning"
			}
			key := string(warning.Type) + ":" + text
			if _, exists := s.warningSeen[key]; exists {
				continue
			}
			s.warningSeen[key] = struct{}{}
			s.pendingWarnings = append(s.pendingWarnings, text)
		}
		return
	case fantasy.StreamPartTypeTextStart,
		fantasy.StreamPartTypeTextEnd,
		fantasy.StreamPartTypeReasoningStart,
		fantasy.StreamPartTypeReasoningDelta,
		fantasy.StreamPartTypeReasoningEnd,
		fantasy.StreamPartTypeToolInputStart,
		fantasy.StreamPartTypeToolInputDelta,
		fantasy.StreamPartTypeToolInputEnd,
		fantasy.StreamPartTypeToolResult,
		fantasy.StreamPartTypeSource,
		fantasy.StreamPartTypeFinish:
		return
	default:
		return
	}
}
