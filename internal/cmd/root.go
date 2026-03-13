package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	glamour "github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/x/editor"
	"github.com/dotcommander/yai/internal/agent"
	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/errs"
	"github.com/dotcommander/yai/internal/present"
	"github.com/dotcommander/yai/internal/proto"
	"github.com/dotcommander/yai/internal/tui"
	"github.com/spf13/cobra"
)

type runtime struct {
	build  BuildInfo
	cfg    config.Config
	cfgErr error
}

// NewRootCmd constructs the Cobra root command.
func NewRootCmd(build BuildInfo, cfg config.Config, cfgErr error) *cobra.Command {
	// XXX: unset error styles in Glamour dark and light styles.
	glamour.DarkStyleConfig.CodeBlock.Chroma.Error.BackgroundColor = new(string)
	glamour.LightStyleConfig.CodeBlock.Chroma.Error.BackgroundColor = new(string)

	rt := &runtime{build: normalizeBuildInfo(build), cfg: cfg, cfgErr: cfgErr}

	rootCmd := &cobra.Command{
		Use:                "yai",
		Short:              "GPT on the command line. Built for pipelines.",
		SilenceUsage:       true,
		SilenceErrors:      true,
		Example:            randomExample(),
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: false,
		TraverseChildren:   true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if rt.cfgErr != nil {
				return rt.cfgErr
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			cmd.SetContext(ctx)
			return rt.runGenerate(cmd, args)
		},
	}

	rootCmd.SetUsageFunc(usageFunc)
	rootCmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return newFlagParseError(err)
	})

	rootCmd.CompletionOptions.HiddenDefaultCmd = true
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})

	rootCmd.Version = rt.build.Version
	rootCmd.SetVersionTemplate(versionTemplate(rt.build))

	initRootFlags(rootCmd, &rt.cfg)

	// Commands.
	rootCmd.AddCommand(newHistoryCmd(rt))
	rootCmd.AddCommand(newConfigCmd(rt))
	rootCmd.AddCommand(newMCPCmd(rt))
	rootCmd.AddCommand(newManCmd(rootCmd))
	rootCmd.AddCommand(newUpgradeCmd(rt))
	rootCmd.AddCommand(newChatCmd(rt))

	// Enable completion now that we have subcommands.
	rootCmd.InitDefaultCompletionCmd()

	return rootCmd
}

func (rt *runtime) runGenerate(cmd *cobra.Command, args []string) error {
	rt.cfg.Prefix = present.RemoveWhitespace(strings.Join(args, " "))

	if err := rt.applyPatchMode(cmd); err != nil {
		return err
	}
	if err := rt.maybeLoadPromptFromEditor(); err != nil {
		return err
	}
	if handled, err := rt.runHeadlessMode(cmd, args); handled || err != nil {
		return err
	}
	if err := rt.maybeAskForPromptInfo(); err != nil {
		return err
	}

	store, err := rt.openAndPlanStore()
	if err != nil {
		return err
	}
	defer store.Close() //nolint:errcheck

	yai, err := rt.runGenerateProgram(cmd.Context(), rt.programOptions(), store)
	if err != nil {
		return err
	}
	if err := rt.ensurePromptInput(yai); err != nil {
		return err
	}
	rt.printGenerateOutput(yai)
	return saveConversation(&rt.cfg, store, yai.Messages())
}

func (rt *runtime) applyPatchMode(cmd *cobra.Command) error {
	if !rt.cfg.Patch {
		return nil
	}
	if cmd.Flags().Changed("format") {
		return fmt.Errorf("%w", errs.UserErrorf("--patch and --format cannot be used together"))
	}
	if cmd.Flags().Changed("role") {
		return fmt.Errorf("%w", errs.UserErrorf("--patch and --role cannot be used together"))
	}
	rt.cfg.Raw = true
	rt.cfg.Role = "diff"
	rt.cfg.Format = false
	return nil
}

func (rt *runtime) programOptions() []tea.ProgramOption {
	if os.Getenv("VIMRUNTIME") != "" {
		rt.cfg.Quiet = true
	}

	opts := []tea.ProgramOption{}
	if !present.IsInputTTY() || rt.cfg.Raw {
		opts = append(opts, tea.WithInput(nil))
	}
	if present.IsOutputTTY() && !rt.cfg.Raw {
		return append(opts, tea.WithOutput(os.Stderr))
	}
	return append(opts, tea.WithoutRenderer())
}

func (rt *runtime) maybeLoadPromptFromEditor() error {
	if !isNoArgs(&rt.cfg) || !present.IsInputTTY() || !rt.cfg.OpenEditor {
		return nil
	}
	prompt, err := prefixFromEditor(rt.cfg.SettingsPath)
	if err != nil {
		return err
	}
	rt.cfg.Prefix = prompt
	return nil
}

func (rt *runtime) runHeadlessMode(cmd *cobra.Command, args []string) (bool, error) {
	// Headless modes (no TUI) still drain stdin to keep pipes predictable.
	switch {
	case rt.cfg.ShowHelp:
		drainStdin()
		if err := cmd.Usage(); err != nil {
			return true, fmt.Errorf("usage: %w", err)
		}
		return true, nil
	case rt.cfg.Show != "" || rt.cfg.ShowLast:
		drainStdin()
		return true, showConversation(&rt.cfg)
	case rt.cfg.Dirs:
		drainStdin()
		printDirs(&rt.cfg, args)
		return true, nil
	case rt.cfg.EditSettings:
		drainStdin()
		return true, editSettings(&rt.cfg)
	case rt.cfg.ResetSettings:
		drainStdin()
		return true, resetSettings(&rt.cfg)
	case rt.cfg.ListRoles:
		drainStdin()
		listRoles(&rt.cfg)
		return true, nil
	case rt.cfg.MCPList:
		drainStdin()
		mcpList(&rt.cfg)
		return true, nil
	case rt.cfg.MCPListTools:
		drainStdin()
		ctx, cancel := context.WithTimeout(cmd.Context(), rt.cfg.MCPTimeout)
		defer cancel()
		return true, mcpListTools(ctx, &rt.cfg)
	case rt.cfg.List:
		drainStdin()
		return true, listConversations(&rt.cfg, rt.cfg.Raw)
	case len(rt.cfg.Delete) > 0:
		drainStdin()
		return true, deleteConversations(&rt.cfg, rt.cfg.Delete)
	case rt.cfg.DeleteOlderThan != 0:
		drainStdin()
		return true, deleteConversationsOlderThan(&rt.cfg, rt.cfg.DeleteOlderThan.String())
	default:
		return false, nil
	}
}

func (rt *runtime) maybeAskForPromptInfo() error {
	if !(isNoArgs(&rt.cfg) || rt.cfg.AskModel) || !present.IsInputTTY() {
		return nil
	}
	if err := askInfo(&rt.cfg); err != nil && err == huh.ErrUserAborted {
		return errs.Wrap(err, "User canceled.")
	} else if err != nil {
		return errs.Wrap(err, "Prompt failed.")
	}
	return nil
}

func (rt *runtime) openAndPlanStore() (*conversationStore, error) {
	store, err := openConversationStore(rt.cfg.CachePath)
	if err != nil {
		return nil, errs.Wrap(err, "Could not open conversation store.")
	}

	pl, err := planConversation(&rt.cfg, store.DB)
	if err != nil {
		store.Close() //nolint:errcheck
		return nil, err
	}
	rt.cfg.CacheWriteToID = pl.WriteID
	rt.cfg.CacheWriteToTitle = pl.Title
	rt.cfg.CacheReadFromID = pl.ReadID
	rt.cfg.API = pl.API
	rt.cfg.Model = pl.Model
	return store, nil
}

func (rt *runtime) runGenerateProgram(
	ctx context.Context,
	opts []tea.ProgramOption,
	store *conversationStore,
) (*tui.Yai, error) {
	agentSvc := agent.New(&rt.cfg, store.Cache, nil)
	startStreamFn := agentSvc.Stream
	yai := tui.NewYai(ctx, present.StderrRenderer(), &rt.cfg, agentSvc, startStreamFn)
	p := tea.NewProgram(yai, opts...)
	m, err := p.Run()
	if err != nil {
		return nil, errs.Wrap(err, "Couldn't start Bubble Tea program.")
	}

	yai = m.(*tui.Yai)
	if yai.Error != nil {
		return nil, *yai.Error
	}
	return yai, nil
}

func (rt *runtime) ensurePromptInput(yai *tui.Yai) error {
	if yai.Input != "" || !isNoArgs(&rt.cfg) {
		return nil
	}
	return errs.Wrap(
		errs.UserErrorf(
			"You can give your prompt as arguments and/or pipe it from STDIN.\nExample: %s",
			present.StdoutStyles().InlineCode.Render("yai [prompt]"),
		),
		"You haven't provided any prompt input.",
	)
}

func (rt *runtime) printGenerateOutput(yai *tui.Yai) {
	if !present.IsOutputTTY() || rt.cfg.Raw {
		return
	}
	switch {
	case yai.GlamourOutput() != "":
		fmt.Print(yai.GlamourOutput())
	case yai.Output != "":
		fmt.Print(yai.Output)
	}
}

func showConversation(cfg *config.Config) error {
	store, err := openConversationStore(cfg.CachePath)
	if err != nil {
		return errs.Wrap(err, "Could not open conversation store.")
	}
	defer store.Close() //nolint:errcheck

	in := cfg.Show
	if cfg.ShowLast {
		in = ""
	}
	found, err := findReadConversation(cfg, store.DB, in)
	if err != nil {
		return errs.Wrap(err, "There was an error loading the conversation.")
	}

	var messages []proto.Message
	if err := store.Cache.Read(found.ID, &messages); err != nil {
		return errs.Wrap(err, "There was an error loading the conversation.")
	}

	out := proto.Conversation(messages).String()
	if present.IsOutputTTY() && !cfg.Raw {
		formatted, err := present.RenderMarkdownForTTY(out, cfg.WordWrap)
		if err == nil {
			out = formatted
		}
	}
	fmt.Print(out)
	return nil
}

func prefixFromEditor(appName string) (string, error) {
	f, err := os.CreateTemp("", "prompt")
	if err != nil {
		return "", fmt.Errorf("could not create temporary file: %w", err)
	}
	_ = f.Close()
	defer func() { _ = os.Remove(f.Name()) }() //nolint:gosec // G703: path from os.CreateTemp, not user input

	c, err := editor.Cmd(
		appName,
		f.Name(),
	)
	if err != nil {
		return "", fmt.Errorf("could not open editor: %w", err)
	}
	c.Stdin = os.Stdin
	c.Stderr = os.Stderr
	c.Stdout = os.Stdout
	if err := c.Run(); err != nil {
		return "", fmt.Errorf("could not open editor: %w", err)
	}
	prompt, err := os.ReadFile(f.Name()) //nolint:gosec // G703: path from os.CreateTemp, not user input
	if err != nil {
		return "", fmt.Errorf("could not read file: %w", err)
	}
	return string(prompt), nil
}

// askInfo is the interactive prompt that can pick API/model and optionally the prompt.
func askInfo(cfg *config.Config) error {
	if err := promptForAPIAndModel(cfg); err != nil {
		return fmt.Errorf("prompt: %w", err)
	}
	return nil
}

func promptForAPIAndModel(cfg *config.Config) error {
	apis := make([]huh.Option[string], 0, len(cfg.APIs))
	opts := map[string][]huh.Option[string]{}
	for _, api := range cfg.APIs {
		apis = append(apis, huh.NewOption(api.Name, api.Name))
		for name, model := range api.Models {
			opts[api.Name] = append(opts[api.Name], huh.NewOption(name, name))

			if !cfg.AskModel &&
				(cfg.API == "" || cfg.API == api.Name) &&
				(cfg.Model == name || slices.Contains(model.Aliases, cfg.Model)) {
				cfg.API = api.Name
				cfg.Model = name
			}
		}
	}

	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Choose the API:").
				Options(apis...).
				Value(&cfg.API),
			huh.NewSelect[string]().
				TitleFunc(func() string {
					return fmt.Sprintf("Choose the model for '%s':", cfg.API)
				}, &cfg.API).
				OptionsFunc(func() []huh.Option[string] {
					return opts[cfg.API]
				}, &cfg.API).
				Value(&cfg.Model),
		),
		huh.NewGroup(
			huh.NewText().
				TitleFunc(func() string {
					return fmt.Sprintf("Enter a prompt for %s/%s:", cfg.API, cfg.Model)
				}, &cfg.Model).
				Value(&cfg.Prefix),
		).WithHideFunc(func() bool {
			return cfg.Prefix != ""
		}),
	).
		WithTheme(themeFrom(cfg.Theme)).
		Run(); err != nil {
		return fmt.Errorf("prompt form: %w", err)
	}
	return nil
}

func themeFrom(theme string) *huh.Theme {
	switch theme {
	case "dracula":
		return huh.ThemeDracula()
	case "catppuccin":
		return huh.ThemeCatppuccin()
	case "base16":
		return huh.ThemeBase16()
	default:
		return huh.ThemeCharm()
	}
}
