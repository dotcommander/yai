package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	stdstrings "strings"
	"text/template"
	"time"

	_ "embed"

	"github.com/caarlos0/env/v9"
	"gopkg.in/yaml.v3"

	"github.com/dotcommander/yai/internal/errs"
)

//go:embed config_template.yml
var configTemplate string

const (
	defaultMarkdownFormatText = "Format the response as markdown without enclosing backticks."
	defaultJSONFormatText     = "Format the response as json without enclosing backticks."
)

// Model represents the LLM model used in the API call.
type Model struct {
	Name           string
	API            string
	MaxChars       int64    `yaml:"max-input-chars"`
	Aliases        []string `yaml:"aliases"`
	Fallback       string   `yaml:"fallback"`
	ThinkingBudget int      `yaml:"thinking-budget,omitempty"`
}

// API represents an API endpoint and its models.
type API struct {
	Name      string
	APIKey    string           `yaml:"api-key"`
	APIKeyEnv string           `yaml:"api-key-env"`
	APIKeyCmd string           `yaml:"api-key-cmd"`
	Version   string           `yaml:"version"` // not used
	BaseURL   string           `yaml:"base-url"`
	Models    map[string]Model `yaml:"models"`
	User      string           `yaml:"user"`
}

// APIs is a type alias to allow custom YAML decoding.
type APIs []API

// UnmarshalYAML implements sorted API YAML decoding.
func (apis *APIs) UnmarshalYAML(node *yaml.Node) error {
	for i := 0; i < len(node.Content); i += 2 {
		var api API
		if err := node.Content[i+1].Decode(&api); err != nil {
			return fmt.Errorf("error decoding YAML file: %s", err)
		}
		api.Name = node.Content[i].Value
		*apis = append(*apis, api)
	}
	return nil
}

// FormatText is a map[format]formatting_text.
type FormatText map[string]string

// UnmarshalYAML conforms with yaml.Unmarshaler.
func (ft *FormatText) UnmarshalYAML(unmarshal func(any) error) error {
	var text string
	if err := unmarshal(&text); err != nil {
		var formats map[string]string
		if err := unmarshal(&formats); err != nil {
			return err
		}
		*ft = (FormatText)(formats)
		return nil
	}

	*ft = map[string]string{
		"markdown": text,
	}
	return nil
}

// Settings holds persisted configuration loaded from the YAML settings file
// and environment variables.
type Settings struct {
	API                 string              `yaml:"default-api" env:"API"`
	Model               string              `yaml:"default-model" env:"MODEL"`
	Format              bool                `yaml:"format" env:"FORMAT"`
	FormatText          FormatText          `yaml:"format-text"`
	FormatAs            string              `yaml:"format-as" env:"FORMAT_AS"`
	Raw                 bool                `yaml:"raw" env:"RAW"`
	Quiet               bool                `yaml:"quiet" env:"QUIET"`
	MaxTokens           int64               `yaml:"max-tokens" env:"MAX_TOKENS"`
	MaxCompletionTokens int64               `yaml:"max-completion-tokens" env:"MAX_COMPLETION_TOKENS"`
	MaxInputChars       int64               `yaml:"max-input-chars" env:"MAX_INPUT_CHARS"`
	Temperature         float64             `yaml:"temp" env:"TEMP"`
	Stop                []string            `yaml:"stop" env:"STOP"`
	TopP                float64             `yaml:"topp" env:"TOPP"`
	TopK                int64               `yaml:"topk" env:"TOPK"`
	NoLimit             bool                `yaml:"no-limit" env:"NO_LIMIT"`
	CachePath           string              `yaml:"cache-path" env:"CACHE_PATH"`
	NoCache             bool                `yaml:"no-cache" env:"NO_CACHE"`
	IncludePromptArgs   bool                `yaml:"include-prompt-args" env:"INCLUDE_PROMPT_ARGS"`
	IncludePrompt       int                 `yaml:"include-prompt" env:"INCLUDE_PROMPT"`
	MaxRetries          int                 `yaml:"max-retries" env:"MAX_RETRIES"`
	WordWrap            int                 `yaml:"word-wrap" env:"WORD_WRAP"`
	Fanciness           uint                `yaml:"fanciness" env:"FANCINESS"`
	StatusText          string              `yaml:"status-text" env:"STATUS_TEXT"`
	HTTPProxy           string              `yaml:"http-proxy" env:"HTTP_PROXY"`
	APIs                APIs                `yaml:"apis"`
	System              string              `yaml:"system"`
	Role                string              `yaml:"role" env:"ROLE"`
	Theme               string              `yaml:"theme" env:"THEME"`
	User                string              `yaml:"user" env:"YAI_USER"`
	Roles               map[string][]string `yaml:"roles"`

	MCPServers map[string]MCPServerConfig `yaml:"mcp-servers"`
	MCPDisable []string                   `yaml:"mcp-disable" env:"MCP_DISABLE"`
	MCPTimeout time.Duration              `yaml:"mcp-timeout" env:"MCP_TIMEOUT"`
}

// Runtime holds CLI/runtime-only options that should not be loaded from the
// settings file.
type Runtime struct {
	AskModel        bool
	ShowHelp        bool
	ResetSettings   bool
	Prefix          string
	Version         bool
	EditSettings    bool
	Dirs            bool
	SettingsPath    string
	ContinueLast    bool
	Continue        string
	Title           string
	ShowLast        bool
	Show            string
	List            bool
	ListRoles       bool
	Delete          []string
	DeleteOlderThan time.Duration
	MCPList         bool
	MCPListTools    bool
	OpenEditor      bool

	CacheReadFromID                   string
	CacheWriteToID, CacheWriteToTitle string
}

// Config is the application configuration (settings + runtime-only options).
//
// Settings fields are promoted for ergonomic access, but runtime fields are
// explicitly excluded from YAML/env parsing.
type Config struct {
	Settings `yaml:",inline"`
	Runtime  `yaml:"-" env:"-"`
}

// MCPServerConfig holds configuration for an MCP server.
type MCPServerConfig struct {
	Type    string   `yaml:"type"`
	Command string   `yaml:"command"`
	Env     []string `yaml:"env"`
	Args    []string `yaml:"args"`
	URL     string   `yaml:"url"`
}

// Ensure loads settings from disk and environment and applies defaults.
//
// It also creates the default settings file if it does not exist.
func Ensure() (Config, error) {
	var c Config
	home, err := os.UserHomeDir()
	if err != nil {
		return c, errs.Error{Err: err, Reason: "Could not determine home directory."}
	}

	sp := filepath.Join(home, ".config", "yai", "yai.yml")
	c.SettingsPath = sp

	dir := filepath.Dir(sp)
	if dirErr := os.MkdirAll(dir, 0o700); dirErr != nil {
		return c, errs.Error{Err: dirErr, Reason: "Could not create cache directory."}
	}

	if dirErr := WriteConfigFile(sp); dirErr != nil {
		return c, dirErr
	}
	content, err := os.ReadFile(sp)
	if err != nil {
		return c, errs.Error{Err: err, Reason: "Could not read settings file."}
	}
	if err := yaml.Unmarshal(content, &c); err != nil {
		return c, errs.Error{Err: err, Reason: "Could not parse settings file."}
	}

	if err := env.ParseWithOptions(&c, env.Options{Prefix: "YAI_"}); err != nil {
		return c, errs.Error{Err: err, Reason: "Could not parse environment into settings file."}
	}

	if err := MergeRolesFromDir(&c); err != nil {
		return c, errs.Error{Err: err, Reason: "Could not load roles from roles directory."}
	}

	if c.CachePath == "" {
		c.CachePath = filepath.Join(home, ".config", "yai", "history")
	}

	if err := os.MkdirAll(
		filepath.Join(c.CachePath, "conversations"),
		0o700,
	); err != nil {
		return c, errs.Error{Err: err, Reason: "Could not create cache directory."}
	}

	if c.WordWrap == 0 {
		c.WordWrap = 80
	}

	if c.FormatText == nil {
		c.FormatText = Default().FormatText
	}
	if c.FormatAs == "" {
		c.FormatAs = "markdown"
	}
	if c.MCPTimeout == 0 {
		c.MCPTimeout = Default().MCPTimeout
	}

	return c, nil
}

// MergeRolesFromDir merges role definitions from ~/.config/yai/roles into cfg.
func MergeRolesFromDir(cfg *Config) error {
	rolesDir := filepath.Join(filepath.Dir(cfg.SettingsPath), "roles")
	roles, err := readRolesFromDir(rolesDir)
	if err != nil {
		return err
	}
	if len(roles) == 0 {
		return nil
	}
	if cfg.Roles == nil {
		cfg.Roles = map[string][]string{}
	}
	for name, setup := range roles {
		if _, exists := cfg.Roles[name]; exists {
			continue
		}
		cfg.Roles[name] = setup
	}
	return nil
}

func readRolesFromDir(dir string) (map[string][]string, error) {
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("read roles directory %q: %w", dir, err)
	}

	roles := map[string][]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		ext := stdstrings.ToLower(filepath.Ext(path))
		if ext != ".md" {
			return nil
		}

		relPath, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return fmt.Errorf("resolve role path %q: %w", path, relErr)
		}

		roleName := stdstrings.TrimSuffix(filepath.ToSlash(relPath), filepath.Ext(relPath))
		if roleName == "" {
			return nil
		}

		setup, setupErr := roleSetupFromFile(path)
		if setupErr != nil {
			return fmt.Errorf("role file %q: %w", relPath, setupErr)
		}
		roles[roleName] = setup
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read roles directory %q: %w", dir, err)
	}

	return roles, nil
}

func roleSetupFromFile(path string) ([]string, error) {
	ext := stdstrings.ToLower(filepath.Ext(path))
	if ext != ".yml" && ext != ".yaml" {
		return []string{"file://" + path}, nil
	}

	bts, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read role file %q: %w", path, err)
	}

	var setup []string
	if err := yaml.Unmarshal(bts, &setup); err == nil {
		return setup, nil
	}

	var single string
	if err := yaml.Unmarshal(bts, &single); err == nil {
		return []string{single}, nil
	}

	return nil, fmt.Errorf("must be a YAML string or string list")
}

// WriteConfigFile creates the config file at path if it does not exist.
func WriteConfigFile(path string) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return createConfigFile(path)
	} else if err != nil {
		return errs.Error{Err: err, Reason: "Could not stat path."}
	}
	return nil
}

func createConfigFile(path string) error {
	tmpl := template.Must(template.New("config").Parse(configTemplate))

	f, err := os.Create(path)
	if err != nil {
		return errs.Error{Err: err, Reason: "Could not create configuration file."}
	}
	defer func() { _ = f.Close() }()

	m := struct{ Config Config }{Config: Default()}
	if err := tmpl.Execute(f, m); err != nil {
		return errs.Error{Err: err, Reason: "Could not render template."}
	}
	return nil
}

// Default returns the default configuration values.
func Default() Config {
	return Config{
		Settings: Settings{
			FormatAs: "markdown",
			FormatText: FormatText{
				"markdown": defaultMarkdownFormatText,
				"json":     defaultJSONFormatText,
			},
			MCPTimeout: 15 * time.Second,
		},
	}
}
