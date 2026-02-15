package cmd

import (
	"path/filepath"

	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/present"
	"github.com/dotcommander/yai/internal/storage"
	"github.com/spf13/cobra"
)

func initRootFlags(cmd *cobra.Command, cfg *config.Config) {
	flags := cmd.Flags()
	flags.StringVarP(&cfg.Model, "model", "m", cfg.Model, present.StdoutStyles().FlagDesc.Render(helpText["model"]))
	flags.BoolVarP(&cfg.AskModel, "ask-model", "M", cfg.AskModel, present.StdoutStyles().FlagDesc.Render(helpText["ask-model"]))
	flags.StringVarP(&cfg.API, "api", "a", cfg.API, present.StdoutStyles().FlagDesc.Render(helpText["api"]))
	flags.StringVarP(&cfg.HTTPProxy, "http-proxy", "x", cfg.HTTPProxy, present.StdoutStyles().FlagDesc.Render(helpText["http-proxy"]))
	flags.BoolVarP(&cfg.Format, "format", "f", cfg.Format, present.StdoutStyles().FlagDesc.Render(helpText["format"]))
	flags.StringVar(&cfg.FormatAs, "format-as", cfg.FormatAs, present.StdoutStyles().FlagDesc.Render(helpText["format-as"]))
	flags.BoolVarP(&cfg.Raw, "raw", "r", cfg.Raw, present.StdoutStyles().FlagDesc.Render(helpText["raw"]))
	flags.IntVarP(&cfg.IncludePrompt, "prompt", "P", cfg.IncludePrompt, present.StdoutStyles().FlagDesc.Render(helpText["prompt"]))
	flags.BoolVarP(&cfg.IncludePromptArgs, "prompt-args", "p", cfg.IncludePromptArgs, present.StdoutStyles().FlagDesc.Render(helpText["prompt-args"]))
	flags.StringVarP(&cfg.Continue, "continue", "c", "", present.StdoutStyles().FlagDesc.Render(helpText["continue"]))
	flags.BoolVarP(&cfg.ContinueLast, "continue-last", "C", false, present.StdoutStyles().FlagDesc.Render(helpText["continue-last"]))
	flags.BoolVarP(&cfg.List, "list", "l", cfg.List, present.StdoutStyles().FlagDesc.Render(helpText["list"]))
	flags.StringVarP(&cfg.Title, "title", "t", cfg.Title, present.StdoutStyles().FlagDesc.Render(helpText["title"]))
	flags.StringArrayVarP(&cfg.Delete, "delete", "d", cfg.Delete, present.StdoutStyles().FlagDesc.Render(helpText["delete"]))
	flags.Var(newDurationFlag(cfg.DeleteOlderThan, &cfg.DeleteOlderThan), "delete-older-than", present.StdoutStyles().FlagDesc.Render(helpText["delete-older-than"]))
	flags.StringVarP(&cfg.Show, "show", "s", cfg.Show, present.StdoutStyles().FlagDesc.Render(helpText["show"]))
	flags.BoolVarP(&cfg.ShowLast, "show-last", "S", false, present.StdoutStyles().FlagDesc.Render(helpText["show-last"]))
	flags.BoolVarP(&cfg.Quiet, "quiet", "q", cfg.Quiet, present.StdoutStyles().FlagDesc.Render(helpText["quiet"]))
	flags.BoolVarP(&cfg.ShowHelp, "help", "h", false, present.StdoutStyles().FlagDesc.Render(helpText["help"]))
	flags.BoolVarP(&cfg.Version, "version", "v", false, present.StdoutStyles().FlagDesc.Render(helpText["version"]))
	flags.IntVar(&cfg.MaxRetries, "max-retries", cfg.MaxRetries, present.StdoutStyles().FlagDesc.Render(helpText["max-retries"]))
	flags.BoolVar(&cfg.NoLimit, "no-limit", cfg.NoLimit, present.StdoutStyles().FlagDesc.Render(helpText["no-limit"]))
	flags.Int64Var(&cfg.MaxTokens, "max-tokens", cfg.MaxTokens, present.StdoutStyles().FlagDesc.Render(helpText["max-tokens"]))
	flags.IntVar(&cfg.WordWrap, "word-wrap", cfg.WordWrap, present.StdoutStyles().FlagDesc.Render(helpText["word-wrap"]))
	flags.Float64Var(&cfg.Temperature, "temp", cfg.Temperature, present.StdoutStyles().FlagDesc.Render(helpText["temp"]))
	flags.StringArrayVar(&cfg.Stop, "stop", cfg.Stop, present.StdoutStyles().FlagDesc.Render(helpText["stop"]))
	flags.Float64Var(&cfg.TopP, "topp", cfg.TopP, present.StdoutStyles().FlagDesc.Render(helpText["topp"]))
	flags.Int64Var(&cfg.TopK, "topk", cfg.TopK, present.StdoutStyles().FlagDesc.Render(helpText["topk"]))
	flags.UintVar(&cfg.Fanciness, "fanciness", cfg.Fanciness, present.StdoutStyles().FlagDesc.Render(helpText["fanciness"]))
	flags.StringVar(&cfg.StatusText, "status-text", cfg.StatusText, present.StdoutStyles().FlagDesc.Render(helpText["status-text"]))
	flags.BoolVar(&cfg.NoCache, "no-cache", cfg.NoCache, present.StdoutStyles().FlagDesc.Render(helpText["no-cache"]))
	flags.BoolVar(&cfg.ResetSettings, "reset-settings", cfg.ResetSettings, present.StdoutStyles().FlagDesc.Render(helpText["reset-settings"]))
	flags.BoolVar(&cfg.EditSettings, "settings", false, present.StdoutStyles().FlagDesc.Render(helpText["settings"]))
	flags.BoolVar(&cfg.Dirs, "dirs", false, present.StdoutStyles().FlagDesc.Render(helpText["dirs"]))
	flags.StringVarP(&cfg.Role, "role", "R", cfg.Role, present.StdoutStyles().FlagDesc.Render(helpText["role"]))
	flags.BoolVar(&cfg.ListRoles, "list-roles", cfg.ListRoles, present.StdoutStyles().FlagDesc.Render(helpText["list-roles"]))
	flags.StringVar(&cfg.Theme, "theme", "charm", present.StdoutStyles().FlagDesc.Render(helpText["theme"]))
	flags.BoolVarP(&cfg.OpenEditor, "editor", "e", false, present.StdoutStyles().FlagDesc.Render(helpText["editor"]))
	flags.BoolVar(&cfg.MCPList, "mcp-list", false, present.StdoutStyles().FlagDesc.Render(helpText["mcp-list"]))
	flags.BoolVar(&cfg.MCPListTools, "mcp-list-tools", false, present.StdoutStyles().FlagDesc.Render(helpText["mcp-list-tools"]))
	flags.StringArrayVar(&cfg.MCPDisable, "mcp-disable", nil, present.StdoutStyles().FlagDesc.Render(helpText["mcp-disable"]))
	flags.Lookup("prompt").NoOptDefVal = "-1"
	flags.SortFlags = false

	flags.BoolVar(&memprofile, "memprofile", false, "Write memory profiles to CWD")
	_ = flags.MarkHidden("memprofile")

	// Shell completions for continue/show/delete IDs. Open DB lazily.
	for _, name := range []string{"show", "delete", "continue"} {
		_ = cmd.RegisterFlagCompletionFunc(name, func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if cfg.CachePath == "" {
				return nil, cobra.ShellCompDirectiveDefault
			}
			db, err := storage.Open(filepath.Join(cfg.CachePath, "conversations"))
			if err != nil {
				return nil, cobra.ShellCompDirectiveDefault
			}
			defer db.Close() //nolint:errcheck
			results := db.Completions(toComplete)
			return results, cobra.ShellCompDirectiveDefault
		})
	}
	_ = cmd.RegisterFlagCompletionFunc("role", func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return roleNames(cfg, toComplete), cobra.ShellCompDirectiveDefault
	})

	cmd.MarkFlagsMutuallyExclusive(
		"settings",
		"show",
		"show-last",
		"delete",
		"delete-older-than",
		"list",
		"continue",
		"continue-last",
		"reset-settings",
		"mcp-list",
		"mcp-list-tools",
	)
}
