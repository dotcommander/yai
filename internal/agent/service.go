package agent

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/caarlos0/go-shellwords"
	mmcp "github.com/mark3labs/mcp-go/mcp"

	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/errs"
	"github.com/dotcommander/yai/internal/fantasybridge"
	"github.com/dotcommander/yai/internal/mcp"
	"github.com/dotcommander/yai/internal/present"
	"github.com/dotcommander/yai/internal/proto"
	"github.com/dotcommander/yai/internal/storage/cache"
	"github.com/dotcommander/yai/internal/stream"
)

// Service is the core orchestration layer for starting LLM streams.
//
// It is intentionally UI-agnostic and can be used by both the TUI and headless
// commands.
type Service struct {
	cfg   *config.Config
	cache *cache.Conversations
	mcp   *mcp.Service
}

// New creates an agent service.
func New(cfg *config.Config, cache *cache.Conversations, mcpSvc *mcp.Service) *Service {
	if mcpSvc == nil {
		mcpSvc = mcp.New(cfg)
	}
	return &Service{cfg: cfg, cache: cache, mcp: mcpSvc}
}

// StreamStart contains the stream plus metadata about the resolved request.
type StreamStart struct {
	Stream   stream.Stream
	Model    config.Model
	Messages []proto.Message
}

// Stream starts a streaming completion for the given prompt.
func (s *Service) Stream(ctx context.Context, prompt string) (StreamStart, error) {
	cfg := s.cfg

	api, mod, err := resolveModel(cfg)
	if err != nil {
		return StreamStart{}, err
	}
	// Keep runtime cfg in sync with resolved model.
	cfg.API = mod.API
	cfg.Model = mod.Name

	providerCfg, err := prepareProviderConfig(ctx, mod, api, cfg)
	if err != nil {
		return StreamStart{}, err
	}
	if err := ApplyProxyConfig(cfg.HTTPProxy, &providerCfg); err != nil {
		return StreamStart{}, err
	}

	if mod.MaxChars == 0 {
		mod.MaxChars = cfg.MaxInputChars
	}

	toolsEnabled := true
	if !cfg.MCPAllowNonTTY && !present.IsInputTTY() {
		toolsEnabled = false
	}

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

	messages, err := s.buildMessages(prompt, mod)
	if err != nil {
		return StreamStart{}, err
	}

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

	request := proto.Request{
		Messages:    messages,
		API:         mod.API,
		Model:       mod.Name,
		User:        cfg.User,
		Temperature: temperature,
		TopP:        topP,
		TopK:        topK,
		Stop:        cfg.Stop,
		Tools:       tools,
	}
	if toolsEnabled {
		request.ToolCaller = func(name string, data []byte) (string, error) {
			callCtx, cancel := context.WithTimeout(ctx, cfg.MCPTimeout)
			defer cancel()
			return s.mcp.CallTool(callCtx, name, data)
		}
	}

	// o1 models do not accept max_tokens.
	if cfg.MaxTokens > 0 && !strings.HasPrefix(mod.Name, "o1") {
		request.MaxTokens = &cfg.MaxTokens
	}
	if cfg.MaxCompletionTokens > 0 {
		request.MaxCompletionTokens = &cfg.MaxCompletionTokens
	}

	client, err := NewFantasyClient(providerCfg)
	if err != nil {
		return StreamStart{}, err
	}

	st := client.Request(ctx, request)
	return StreamStart{Stream: st, Model: mod, Messages: messages}, nil
}

func (s *Service) buildMessages(prompt string, mod config.Model) ([]proto.Message, error) {
	cfg := s.cfg
	messages := make([]proto.Message, 0, 8)

	if txt := cfg.FormatText[cfg.FormatAs]; cfg.Format && txt != "" {
		messages = append(messages, proto.Message{Role: proto.RoleSystem, Content: txt})
	}

	if cfg.Role != "" {
		roleSetup, ok := cfg.Roles[cfg.Role]
		if !ok {
			return nil, errs.Error{Err: fmt.Errorf("role %q does not exist", cfg.Role), Reason: "Could not use role"}
		}
		for _, msg := range roleSetup {
			content, err := config.LoadMsg(msg)
			if err != nil {
				return nil, errs.Error{Err: err, Reason: "Could not use role"}
			}
			messages = append(messages, proto.Message{Role: proto.RoleSystem, Content: content})
		}
	}

	if prefix := cfg.Prefix; prefix != "" {
		prompt = strings.TrimSpace(prefix + "\n\n" + prompt)
	}

	if !cfg.NoLimit && mod.MaxChars > 0 && int64(len(prompt)) > mod.MaxChars {
		prompt = prompt[:mod.MaxChars]
	}

	if !cfg.NoCache && cfg.CacheReadFromID != "" {
		if s.cache == nil {
			return nil, errs.Error{Reason: "Cache is not available"}
		}
		if err := s.cache.Read(cfg.CacheReadFromID, &messages); err != nil {
			return nil, errs.Error{
				Err:    err,
				Reason: "There was a problem reading the cache. Use --no-cache / NO_CACHE to disable it.",
			}
		}
	}

	messages = append(messages, proto.Message{Role: proto.RoleUser, Content: prompt})
	return messages, nil
}

func resolveModel(cfg *config.Config) (config.API, config.Model, error) {
	for _, api := range cfg.APIs {
		if api.Name != cfg.API && cfg.API != "" {
			continue
		}
		for name, mod := range api.Models {
			if name == cfg.Model || slices.Contains(mod.Aliases, cfg.Model) {
				cfg.Model = name
				break
			}
		}
		mod, ok := api.Models[cfg.Model]
		if ok {
			mod.Name = cfg.Model
			mod.API = api.Name
			return api, mod, nil
		}
		if cfg.API != "" {
			available := make([]string, 0, len(api.Models))
			for name := range api.Models {
				available = append(available, name)
			}
			slices.Sort(available)
			return config.API{}, config.Model{}, errs.Error{
				Err:    errs.UserErrorf("Available models are: %s", strings.Join(available, ", ")),
				Reason: fmt.Sprintf("The API endpoint %s does not contain the model %s", cfg.API, cfg.Model),
			}
		}
	}

	return config.API{}, config.Model{}, errs.Error{
		Reason: fmt.Sprintf("Model %s is not in the settings file.", cfg.Model),
		Err:    errs.UserErrorf("Please specify an API endpoint with --api or configure the model in the settings: yai --settings"),
	}
}

func prepareProviderConfig(ctx context.Context, mod config.Model, api config.API, cfg *config.Config) (fantasybridge.Config, error) {
	switch mod.API {
	case "openrouter":
		key, err := ensureKey(ctx, api, "OPENROUTER_API_KEY", "https://openrouter.ai/keys")
		if err != nil {
			return fantasybridge.Config{}, errs.Error{Err: err, Reason: "OpenRouter authentication failed"}
		}
		return fantasybridge.Config{API: mod.API, APIKey: key, BaseURL: api.BaseURL}, nil
	case "vercel":
		key, err := ensureKey(ctx, api, "VERCEL_API_KEY", "https://vercel.com/dashboard/tokens")
		if err != nil {
			return fantasybridge.Config{}, errs.Error{Err: err, Reason: "Vercel AI Gateway authentication failed"}
		}
		return fantasybridge.Config{API: mod.API, APIKey: key, BaseURL: api.BaseURL}, nil
	case "bedrock":
		key, err := optionalKey(ctx, api)
		if err != nil {
			return fantasybridge.Config{}, errs.Error{Err: err, Reason: "Bedrock authentication failed"}
		}
		return fantasybridge.Config{API: mod.API, APIKey: key, BaseURL: api.BaseURL}, nil
	case "cohere":
		key, err := ensureKey(ctx, api, "COHERE_API_KEY", "https://dashboard.cohere.com/api-keys")
		if err != nil {
			return fantasybridge.Config{}, errs.Error{Err: err, Reason: "Cohere authentication failed"}
		}
		return fantasybridge.Config{API: mod.API, APIKey: key, BaseURL: api.BaseURL}, nil
	case "ollama":
		baseURL := api.BaseURL
		if baseURL == "" {
			baseURL = "http://localhost:11434/v1"
		}
		return fantasybridge.Config{API: mod.API, BaseURL: baseURL}, nil
	case "azure", "azure-ad":
		key, err := ensureKey(ctx, api, "AZURE_OPENAI_KEY", "https://aka.ms/oai/access")
		if err != nil {
			return fantasybridge.Config{}, errs.Error{Err: err, Reason: "Azure authentication failed"}
		}
		providerAPI := mod.API
		if mod.API == "azure-ad" {
			providerAPI = "azure"
		}
		if api.User != "" {
			cfg.User = api.User
		}
		return fantasybridge.Config{API: providerAPI, APIKey: key, BaseURL: api.BaseURL}, nil
	case "anthropic":
		key, err := ensureKey(ctx, api, "ANTHROPIC_API_KEY", "https://console.anthropic.com/settings/keys")
		if err != nil {
			return fantasybridge.Config{}, errs.Error{Err: err, Reason: "Anthropic authentication failed"}
		}
		return fantasybridge.Config{API: mod.API, APIKey: key, BaseURL: api.BaseURL}, nil
	case "google":
		key, err := ensureKey(ctx, api, "GOOGLE_API_KEY", "https://aistudio.google.com/app/apikey")
		if err != nil {
			return fantasybridge.Config{}, errs.Error{Err: err, Reason: "Google authentication failed"}
		}
		return fantasybridge.Config{API: mod.API, APIKey: key, BaseURL: api.BaseURL, ThinkingBudget: mod.ThinkingBudget}, nil
	default:
		key, err := ensureKey(ctx, api, "OPENAI_API_KEY", "https://platform.openai.com/account/api-keys")
		if err != nil {
			return fantasybridge.Config{}, errs.Error{Err: err, Reason: "OpenAI authentication failed"}
		}
		return fantasybridge.Config{API: mod.API, APIKey: key, BaseURL: api.BaseURL}, nil
	}
}

// ApplyProxyConfig configures the provider HTTP client to use an HTTP proxy.
func ApplyProxyConfig(httpProxy string, providerCfg *fantasybridge.Config) error {
	if httpProxy == "" {
		return nil
	}
	proxyURL, err := url.Parse(httpProxy)
	if err != nil {
		return errs.Error{Err: err, Reason: "There was an error parsing your proxy URL."}
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return errs.Error{Err: fmt.Errorf("default transport is not *http.Transport"), Reason: "Could not configure proxy."}
	}
	tr := base.Clone()
	tr.Proxy = http.ProxyURL(proxyURL)
	// Ensure we have sensible transport timeouts even when upstream SDKs don't.
	tr.DialContext = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	tr.TLSHandshakeTimeout = 10 * time.Second
	tr.ResponseHeaderTimeout = 30 * time.Second
	tr.IdleConnTimeout = 90 * time.Second
	tr.ExpectContinueTimeout = 1 * time.Second
	providerCfg.HTTPClient = &http.Client{Transport: tr}
	return nil
}

// NewFantasyClient creates the fantasy bridge client.
func NewFantasyClient(cfg fantasybridge.Config) (stream.Client, error) {
	if cfg.API == "" {
		return nil, errs.Error{Reason: "missing fantasy provider configuration"}
	}
	client, err := fantasybridge.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("new fantasy bridge client: %w", err)
	}
	return client, nil
}

func ensureKey(ctx context.Context, api config.API, defaultEnv, docsURL string) (string, error) {
	key := api.APIKey
	if key == "" && api.APIKeyEnv != "" && api.APIKeyCmd == "" {
		key = os.Getenv(api.APIKeyEnv)
	}
	if key == "" && api.APIKeyCmd != "" {
		args, err := shellwords.Parse(api.APIKeyCmd)
		if err != nil {
			return "", errs.Error{Err: err, Reason: "Failed to parse api-key-cmd"}
		}
		// #nosec G204 -- api-key-cmd is explicitly configured by the local user.
		out, err := exec.CommandContext(ctx, args[0], args[1:]...).CombinedOutput()
		if err != nil {
			return "", errs.Error{Err: err, Reason: "Cannot exec api-key-cmd"}
		}
		key = strings.TrimSpace(string(out))
	}
	if key == "" {
		key = os.Getenv(defaultEnv)
	}
	if key != "" {
		return key, nil
	}
	return "", errs.Error{
		Reason: fmt.Sprintf("%s required; set %s or update yai.yml through yai --settings.", defaultEnv, defaultEnv),
		Err:    errs.UserErrorf("You can grab one at %s", docsURL),
	}
}

func optionalKey(ctx context.Context, api config.API) (string, error) {
	key := api.APIKey
	if key == "" && api.APIKeyEnv != "" && api.APIKeyCmd == "" {
		key = os.Getenv(api.APIKeyEnv)
	}
	if key == "" && api.APIKeyCmd != "" {
		args, err := shellwords.Parse(api.APIKeyCmd)
		if err != nil {
			return "", errs.Error{Err: err, Reason: "Failed to parse api-key-cmd"}
		}
		// #nosec G204 -- api-key-cmd is explicitly configured by the local user.
		out, err := exec.CommandContext(ctx, args[0], args[1:]...).CombinedOutput()
		if err != nil {
			return "", errs.Error{Err: err, Reason: "Cannot exec api-key-cmd"}
		}
		key = strings.TrimSpace(string(out))
	}
	return key, nil
}
