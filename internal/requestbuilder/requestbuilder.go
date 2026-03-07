// Package requestbuilder assembles LLM requests from configuration and prompt context.
package requestbuilder

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

// ResolveModel finds the requested API and model in settings.
func ResolveModel(cfg *config.Config) (config.API, config.Model, error) {
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
			return config.API{}, config.Model{}, errs.Wrap(
				errs.UserErrorf("Available models are: %s", strings.Join(available, ", ")),
				fmt.Sprintf("The API endpoint %s does not contain the model %s", cfg.API, cfg.Model),
			)
		}
	}

	return config.API{}, config.Model{}, errs.Wrap(
		errs.UserErrorf("Please specify an API endpoint with --api or configure the model in the settings: yai --settings"),
		fmt.Sprintf("Model %s is not in the settings file.", cfg.Model),
	)
}

// BuildPreparedFromPrompt resolves provider/model and builds a request from the
// current prompt context.
func BuildPreparedFromPrompt(
	ctx context.Context,
	cfg *config.Config,
	cacheStore *cache.Conversations,
	prompt string,
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

	req, err := BuildRequestFromPrompt(cfg, mod, cacheStore, prompt)
	if err != nil {
		return PreparedStream{}, err
	}

	return PreparedStream{
		Model:    mod,
		Provider: providerCfg,
		Request:  req,
	}, nil
}

// BuildPreparedFromHistory resolves provider/model and builds a request using
// existing conversation history.
func BuildPreparedFromHistory(
	ctx context.Context,
	cfg *config.Config,
	history []proto.Message,
	prompt string,
) (PreparedStream, error) {
	api, resolvedMod, err := ResolveModel(cfg)
	if err != nil {
		return PreparedStream{}, err
	}

	providerCfg, err := PrepareProviderConfig(ctx, resolvedMod, api, cfg)
	if err != nil {
		return PreparedStream{}, err
	}
	if err := ApplyHTTPConfig(cfg.HTTPProxy, &providerCfg); err != nil {
		return PreparedStream{}, err
	}

	req, err := BuildRequestFromHistory(cfg, resolvedMod, history, prompt)
	if err != nil {
		return PreparedStream{}, err
	}

	return PreparedStream{
		Model:    resolvedMod,
		Provider: providerCfg,
		Request:  req,
	}, nil
}

// PrepareProviderConfig builds the provider config for the selected model/API.
func PrepareProviderConfig(ctx context.Context, mod config.Model, api config.API, cfg *config.Config) (provider.Config, error) {
	switch mod.API {
	case "openrouter":
		key, err := ensureKey(ctx, api, "OPENROUTER_API_KEY", "https://openrouter.ai/keys")
		if err != nil {
			return provider.Config{}, errs.Wrap(err, "OpenRouter authentication failed")
		}
		return provider.Config{API: mod.API, APIKey: key, BaseURL: api.BaseURL}, nil
	case "vercel":
		key, err := ensureKey(ctx, api, "VERCEL_API_KEY", "https://vercel.com/dashboard/tokens")
		if err != nil {
			return provider.Config{}, errs.Wrap(err, "Vercel AI Gateway authentication failed")
		}
		return provider.Config{API: mod.API, APIKey: key, BaseURL: api.BaseURL}, nil
	case "bedrock":
		key, err := optionalKey(ctx, api)
		if err != nil {
			return provider.Config{}, errs.Wrap(err, "Bedrock authentication failed")
		}
		return provider.Config{API: mod.API, APIKey: key, BaseURL: api.BaseURL}, nil
	case "cohere":
		key, err := ensureKey(ctx, api, "COHERE_API_KEY", "https://dashboard.cohere.com/api-keys")
		if err != nil {
			return provider.Config{}, errs.Wrap(err, "Cohere authentication failed")
		}
		return provider.Config{API: mod.API, APIKey: key, BaseURL: api.BaseURL}, nil
	case "ollama":
		baseURL := api.BaseURL
		if baseURL == "" {
			baseURL = "http://localhost:11434/v1"
		}
		return provider.Config{API: mod.API, BaseURL: baseURL}, nil
	case "azure", "azure-ad":
		key, err := ensureKey(ctx, api, "AZURE_OPENAI_KEY", "https://aka.ms/oai/access")
		if err != nil {
			return provider.Config{}, errs.Wrap(err, "Azure authentication failed")
		}
		providerAPI := mod.API
		if mod.API == "azure-ad" {
			providerAPI = "azure"
		}
		if api.User != "" {
			cfg.User = api.User
		}
		return provider.Config{API: providerAPI, APIKey: key, BaseURL: api.BaseURL}, nil
	case "anthropic":
		key, err := ensureKey(ctx, api, "ANTHROPIC_API_KEY", "https://console.anthropic.com/settings/keys")
		if err != nil {
			return provider.Config{}, errs.Wrap(err, "Anthropic authentication failed")
		}
		return provider.Config{API: mod.API, APIKey: key, BaseURL: api.BaseURL}, nil
	case "google":
		key, err := ensureKey(ctx, api, "GOOGLE_API_KEY", "https://aistudio.google.com/app/apikey")
		if err != nil {
			return provider.Config{}, errs.Wrap(err, "Google authentication failed")
		}
		return provider.Config{API: mod.API, APIKey: key, BaseURL: api.BaseURL, ThinkingBudget: mod.ThinkingBudget}, nil
	default:
		key, err := ensureKey(ctx, api, "OPENAI_API_KEY", "https://platform.openai.com/account/api-keys")
		if err != nil {
			return provider.Config{}, errs.Wrap(err, "OpenAI authentication failed")
		}
		return provider.Config{API: mod.API, APIKey: key, BaseURL: api.BaseURL}, nil
	}
}

// ApplyHTTPConfig configures the provider HTTP client with hardened transport
// timeouts. When httpProxy is non-empty, the transport is additionally
// configured to route through the given HTTP proxy.
func ApplyHTTPConfig(httpProxy string, providerCfg *provider.Config) error {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return errs.Wrap(fmt.Errorf("default transport is not *http.Transport"), "Could not configure HTTP transport.")
	}
	tr := base.Clone()
	tr.DialContext = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	tr.TLSHandshakeTimeout = 10 * time.Second
	tr.ResponseHeaderTimeout = 30 * time.Second
	tr.IdleConnTimeout = 90 * time.Second
	tr.ExpectContinueTimeout = 1 * time.Second

	if httpProxy != "" {
		proxyURL, err := url.Parse(httpProxy)
		if err != nil {
			return errs.Wrap(err, "There was an error parsing your proxy URL.")
		}
		tr.Proxy = http.ProxyURL(proxyURL)
	}

	providerCfg.HTTPClient = &http.Client{Transport: tr}
	return nil
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

	if mod.MaxChars == 0 {
		mod.MaxChars = cfg.MaxInputChars
	}

	if !cfg.NoCache && cfg.CacheReadFromID != "" {
		if cacheStore == nil {
			return proto.Request{}, errs.Error{Reason: "Cache is not available"}
		}
		if err := cacheStore.Read(cfg.CacheReadFromID, &messages); err != nil {
			return proto.Request{}, errs.Wrap(err, "There was a problem reading the cache. Use --no-cache / NO_CACHE to disable it.")
		}
	}

	if !cfg.NoLimit && mod.MaxChars > 0 && int64(len(prompt)) > mod.MaxChars {
		prompt = prompt[:mod.MaxChars]
	}

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

	if mod.MaxChars == 0 {
		mod.MaxChars = cfg.MaxInputChars
	}

	if !cfg.NoLimit && mod.MaxChars > 0 && int64(len(prompt)) > mod.MaxChars {
		prompt = prompt[:mod.MaxChars]
	}

	messages = append(messages, proto.Message{Role: proto.RoleUser, Content: prompt})
	return BuildRequest(cfg, mod, messages), nil
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

// IsReasoningModel reports whether the given model name is a reasoning model
// (e.g. o1, o3, o4, gpt-5 series) that does not support temperature/top-p/top-k.
func IsReasoningModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return false
	}
	if slash := strings.LastIndex(m, "/"); slash >= 0 && slash < len(m)-1 {
		m = m[slash+1:]
	}

	return strings.HasPrefix(m, "gpt-5") ||
		strings.HasPrefix(m, "o1") ||
		strings.HasPrefix(m, "o3") ||
		strings.HasPrefix(m, "o4")
}

func ensureKey(ctx context.Context, api config.API, defaultEnv, docsURL string) (string, error) {
	key := api.APIKey
	if key == "" && api.APIKeyEnv != "" && api.APIKeyCmd == "" {
		key = os.Getenv(api.APIKeyEnv)
	}
	if key == "" && api.APIKeyCmd != "" {
		args, err := shellwords.Parse(api.APIKeyCmd)
		if err != nil {
			return "", errs.Wrap(err, "Failed to parse api-key-cmd")
		}
		out, err := exec.CommandContext(ctx, args[0], args[1:]...).CombinedOutput() //nolint:gosec // G204: api-key-cmd is user-configured in yai.yml, intentional shell command
		if err != nil {
			return "", errs.Wrap(err, "Cannot exec api-key-cmd")
		}
		key = strings.TrimSpace(string(out))
	}
	if key == "" {
		key = os.Getenv(defaultEnv)
	}
	if key != "" {
		return key, nil
	}
	return "", errs.Wrap(
		errs.UserErrorf("You can grab one at %s", docsURL),
		fmt.Sprintf("%s required; set %s or update yai.yml through yai --settings.", defaultEnv, defaultEnv),
	)
}

func optionalKey(ctx context.Context, api config.API) (string, error) {
	key := api.APIKey
	if key == "" && api.APIKeyEnv != "" && api.APIKeyCmd == "" {
		key = os.Getenv(api.APIKeyEnv)
	}
	if key == "" && api.APIKeyCmd != "" {
		args, err := shellwords.Parse(api.APIKeyCmd)
		if err != nil {
			return "", errs.Wrap(err, "Failed to parse api-key-cmd")
		}
		out, err := exec.CommandContext(ctx, args[0], args[1:]...).CombinedOutput() //nolint:gosec // G204: api-key-cmd is user-configured in yai.yml, intentional shell command
		if err != nil {
			return "", errs.Wrap(err, "Cannot exec api-key-cmd")
		}
		key = strings.TrimSpace(string(out))
	}
	return key, nil
}
