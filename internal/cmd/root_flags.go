package cmd

import (
	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/present"
	"github.com/spf13/cobra"
)

func initRootFlags(cmd *cobra.Command, cfg *config.Config) {
	registerSharedFlags(cmd, cfg)

	flags := cmd.Flags()
	s := present.StdoutStyles().FlagDesc

	// Root-only flags.
	flags.BoolVarP(&cfg.AskModel, "ask-model", "M", cfg.AskModel, s.Render(helpText["ask-model"]))
	flags.IntVarP(&cfg.IncludePrompt, "prompt", "P", cfg.IncludePrompt, s.Render(helpText["prompt"]))
	flags.BoolVarP(&cfg.IncludePromptArgs, "prompt-args", "p", cfg.IncludePromptArgs, s.Render(helpText["prompt-args"]))
	flags.BoolVarP(&cfg.List, "list", "l", cfg.List, s.Render(helpText["list"]))
	flags.StringArrayVarP(&cfg.Delete, "delete", "d", cfg.Delete, s.Render(helpText["delete"]))
	flags.Var(newDurationFlag(cfg.DeleteOlderThan, &cfg.DeleteOlderThan), "delete-older-than", s.Render(helpText["delete-older-than"]))
	flags.StringVarP(&cfg.Show, "show", "s", cfg.Show, s.Render(helpText["show"]))
	flags.BoolVarP(&cfg.ShowLast, "show-last", "S", false, s.Render(helpText["show-last"]))
	flags.BoolVarP(&cfg.ShowHelp, "help", "h", false, s.Render(helpText["help"]))
	flags.BoolVarP(&cfg.Version, "version", "v", false, s.Render(helpText["version"]))
	flags.BoolVar(&cfg.ResetSettings, "reset-settings", cfg.ResetSettings, s.Render(helpText["reset-settings"]))
	flags.BoolVar(&cfg.EditSettings, "settings", false, s.Render(helpText["settings"]))
	flags.BoolVar(&cfg.Dirs, "dirs", false, s.Render(helpText["dirs"]))
	flags.BoolVar(&cfg.ListRoles, "list-roles", cfg.ListRoles, s.Render(helpText["list-roles"]))
	flags.BoolVar(&cfg.Patch, "patch", false, s.Render(helpText["patch"]))
	flags.BoolVarP(&cfg.OpenEditor, "editor", "e", false, s.Render(helpText["editor"]))
	flags.BoolVar(&cfg.MCPList, "mcp-list", false, s.Render(helpText["mcp-list"]))
	flags.BoolVar(&cfg.MCPListTools, "mcp-list-tools", false, s.Render(helpText["mcp-list-tools"]))
	flags.BoolVar(&cfg.MCPAllowNonTTY, "mcp-allow-non-tty", cfg.MCPAllowNonTTY, s.Render(helpText["mcp-allow-non-tty"]))
	flags.Lookup("prompt").NoOptDefVal = "-1"
	flags.SortFlags = false

	flags.BoolVar(&memprofile, "memprofile", false, "Write memory profiles to CWD")
	_ = flags.MarkHidden("memprofile")

	// Shell completions for show/delete IDs (continue + role already registered by registerSharedFlags).
	registerConversationCompletion(cmd, cfg, "show", "delete")

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
