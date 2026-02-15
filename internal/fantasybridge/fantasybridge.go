package fantasybridge

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"charm.land/fantasy"
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
	// Avoid blocking under the mutex.
	s.mu.Lock()
	if s.err != nil {
		s.mu.Unlock()
		return false
	}
	if s.stepDone {
		if err := s.startStep(); err != nil {
			s.err = err
			s.mu.Unlock()
			return false
		}
	}
	partCh := s.partCh
	s.mu.Unlock()

	select {
	case <-s.ctx.Done():
		s.mu.Lock()
		if s.err == nil {
			s.err = s.ctx.Err()
		}
		s.mu.Unlock()
		return false
	case part, ok := <-partCh:
		if !ok {
			s.mu.Lock()
			s.finalizeStep()
			s.mu.Unlock()
			return false
		}
		s.mu.Lock()
		s.last = part
		s.consumePart(part)
		s.mu.Unlock()
		return true
	}
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

	if s.request.ToolCaller == nil {
		statuses := make([]proto.ToolCallStatus, 0, len(s.stepToolCalls))
		for _, call := range s.stepToolCalls {
			msg := proto.Message{
				Role:    proto.RoleTool,
				Content: "tool execution is disabled",
				ToolCalls: []proto.ToolCall{{
					ID:      call.ID,
					IsError: true,
					Function: proto.Function{
						Name:      call.Function.Name,
						Arguments: call.Function.Arguments,
					},
				}},
			}
			s.messages = append(s.messages, msg)
			statuses = append(statuses, proto.ToolCallStatus{Name: call.Function.Name, Err: fmt.Errorf("tool execution is disabled")})
		}
		s.stepToolCalls = nil
		s.stepToolCallSeen = map[string]struct{}{}
		return statuses
	}

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

	applyProviderOptions(&call, s.api, s.config, s.request)

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
