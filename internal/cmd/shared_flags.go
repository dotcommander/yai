package cmd

import (
	"path/filepath"

	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/present"
	"github.com/dotcommander/yai/internal/storage"
	"github.com/spf13/cobra"
)

// registerSharedFlags registers flags common to both the root command and the
// chat subcommand.
func registerSharedFlags(cmd *cobra.Command, cfg *config.Config) {
	flags := cmd.Flags()
	s := present.StdoutStyles().FlagDesc

	flags.StringVarP(&cfg.Model, "model", "m", cfg.Model, s.Render(helpText["model"]))
	flags.StringVarP(&cfg.API, "api", "a", cfg.API, s.Render(helpText["api"]))
	flags.StringVarP(&cfg.HTTPProxy, "http-proxy", "x", cfg.HTTPProxy, s.Render(helpText["http-proxy"]))
	flags.BoolVarP(&cfg.Format, "format", "f", cfg.Format, s.Render(helpText["format"]))
	flags.StringVar(&cfg.FormatAs, "format-as", cfg.FormatAs, s.Render(helpText["format-as"]))
	flags.BoolVarP(&cfg.Raw, "raw", "r", cfg.Raw, s.Render(helpText["raw"]))
	flags.BoolVarP(&cfg.Quiet, "quiet", "q", cfg.Quiet, s.Render(helpText["quiet"]))
	flags.StringVarP(&cfg.Continue, "continue", "c", "", s.Render(helpText["continue"]))
	flags.BoolVarP(&cfg.ContinueLast, "continue-last", "C", false, s.Render(helpText["continue-last"]))
	flags.StringVarP(&cfg.Title, "title", "t", cfg.Title, s.Render(helpText["title"]))
	flags.StringVarP(&cfg.Role, "role", "R", cfg.Role, s.Render(helpText["role"]))
	flags.BoolVar(&cfg.NoCache, "no-cache", cfg.NoCache, s.Render(helpText["no-cache"]))
	flags.Int64Var(&cfg.MaxTokens, "max-tokens", cfg.MaxTokens, s.Render(helpText["max-tokens"]))
	flags.Int64Var(&cfg.MaxCompletionTokens, "max-completion-tokens", cfg.MaxCompletionTokens, s.Render(helpText["max-completion-tokens"]))
	flags.Float64Var(&cfg.Temperature, "temp", cfg.Temperature, s.Render(helpText["temp"]))
	flags.Float64Var(&cfg.TopP, "topp", cfg.TopP, s.Render(helpText["topp"]))
	flags.Int64Var(&cfg.TopK, "topk", cfg.TopK, s.Render(helpText["topk"]))
	flags.IntVar(&cfg.MaxRetries, "max-retries", cfg.MaxRetries, s.Render(helpText["max-retries"]))
	flags.Var(newDurationFlag(cfg.RequestTimeout, &cfg.RequestTimeout), "request-timeout", s.Render(helpText["request-timeout"]))
	flags.IntVar(&cfg.WordWrap, "word-wrap", cfg.WordWrap, s.Render(helpText["word-wrap"]))
	flags.BoolVar(&cfg.NoLimit, "no-limit", cfg.NoLimit, s.Render(helpText["no-limit"]))
	flags.StringArrayVar(&cfg.Stop, "stop", cfg.Stop, s.Render(helpText["stop"]))
	flags.UintVar(&cfg.Fanciness, "fanciness", cfg.Fanciness, s.Render(helpText["fanciness"]))
	flags.StringVar(&cfg.StatusText, "status-text", cfg.StatusText, s.Render(helpText["status-text"]))
	flags.StringVar(&cfg.Theme, "theme", cfg.Theme, s.Render(helpText["theme"]))
	flags.StringArrayVar(&cfg.MCPDisable, "mcp-disable", nil, s.Render(helpText["mcp-disable"]))
	flags.BoolVar(&cfg.MCPNoInheritEnv, "mcp-no-inherit-env", cfg.MCPNoInheritEnv, s.Render(helpText["mcp-no-inherit-env"]))

	registerConversationCompletion(cmd, cfg, "continue")
	_ = cmd.RegisterFlagCompletionFunc("role", func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return roleNames(cfg, toComplete), cobra.ShellCompDirectiveDefault
	})
}

// registerConversationCompletion registers shell-completion for flags that
// accept conversation IDs.
func registerConversationCompletion(cmd *cobra.Command, cfg *config.Config, flagNames ...string) {
	for _, name := range flagNames {
		_ = cmd.RegisterFlagCompletionFunc(name, func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if cfg.CachePath == "" {
				return nil, cobra.ShellCompDirectiveDefault
			}
			db, err := storage.Open(filepath.Join(cfg.CachePath, "conversations"))
			if err != nil {
				return nil, cobra.ShellCompDirectiveDefault
			}
			defer db.Close() //nolint:errcheck
			return db.Completions(toComplete), cobra.ShellCompDirectiveDefault
		})
	}
}
