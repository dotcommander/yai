package cmd

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dotcommander/yai/internal/agent"
	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/errs"
	"github.com/dotcommander/yai/internal/present"
	"github.com/dotcommander/yai/internal/proto"
	"github.com/dotcommander/yai/internal/storage"
	"github.com/dotcommander/yai/internal/tui"
	"github.com/spf13/cobra"
)

func newChatCmd(rt *runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chat [initial prompt]",
		Short: "Start an interactive multi-turn chat session",
		Long:  "Start an interactive REPL for multi-turn conversations with an LLM. Type /exit or press Ctrl+C to quit.",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if rt.cfgErr != nil {
				return rt.cfgErr
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return rt.runChat(ctx, args)
		},
	}

	initChatFlags(cmd, &rt.cfg)
	return cmd
}

func initChatFlags(cmd *cobra.Command, cfg *config.Config) {
	flags := cmd.Flags()
	flags.StringVarP(&cfg.Model, "model", "m", cfg.Model, present.StdoutStyles().FlagDesc.Render(helpText["model"]))
	flags.StringVarP(&cfg.API, "api", "a", cfg.API, present.StdoutStyles().FlagDesc.Render(helpText["api"]))
	flags.StringVarP(&cfg.HTTPProxy, "http-proxy", "x", cfg.HTTPProxy, present.StdoutStyles().FlagDesc.Render(helpText["http-proxy"]))
	flags.BoolVarP(&cfg.Format, "format", "f", cfg.Format, present.StdoutStyles().FlagDesc.Render(helpText["format"]))
	flags.StringVar(&cfg.FormatAs, "format-as", cfg.FormatAs, present.StdoutStyles().FlagDesc.Render(helpText["format-as"]))
	flags.BoolVarP(&cfg.Raw, "raw", "r", cfg.Raw, present.StdoutStyles().FlagDesc.Render(helpText["raw"]))
	flags.BoolVarP(&cfg.Quiet, "quiet", "q", cfg.Quiet, present.StdoutStyles().FlagDesc.Render(helpText["quiet"]))
	flags.StringVarP(&cfg.Continue, "continue", "c", "", present.StdoutStyles().FlagDesc.Render(helpText["continue"]))
	flags.BoolVarP(&cfg.ContinueLast, "continue-last", "C", false, present.StdoutStyles().FlagDesc.Render(helpText["continue-last"]))
	flags.StringVarP(&cfg.Title, "title", "t", cfg.Title, present.StdoutStyles().FlagDesc.Render(helpText["title"]))
	flags.StringVarP(&cfg.Role, "role", "R", cfg.Role, present.StdoutStyles().FlagDesc.Render(helpText["role"]))
	flags.BoolVar(&cfg.NoCache, "no-cache", cfg.NoCache, present.StdoutStyles().FlagDesc.Render(helpText["no-cache"]))
	flags.Int64Var(&cfg.MaxTokens, "max-tokens", cfg.MaxTokens, present.StdoutStyles().FlagDesc.Render(helpText["max-tokens"]))
	flags.Int64Var(&cfg.MaxCompletionTokens, "max-completion-tokens", cfg.MaxCompletionTokens, present.StdoutStyles().FlagDesc.Render(helpText["max-completion-tokens"]))
	flags.Float64Var(&cfg.Temperature, "temp", cfg.Temperature, present.StdoutStyles().FlagDesc.Render(helpText["temp"]))
	flags.Float64Var(&cfg.TopP, "topp", cfg.TopP, present.StdoutStyles().FlagDesc.Render(helpText["topp"]))
	flags.Int64Var(&cfg.TopK, "topk", cfg.TopK, present.StdoutStyles().FlagDesc.Render(helpText["topk"]))
	flags.IntVar(&cfg.MaxRetries, "max-retries", cfg.MaxRetries, present.StdoutStyles().FlagDesc.Render(helpText["max-retries"]))
	flags.Var(newDurationFlag(cfg.RequestTimeout, &cfg.RequestTimeout), "request-timeout", present.StdoutStyles().FlagDesc.Render(helpText["request-timeout"]))
	flags.IntVar(&cfg.WordWrap, "word-wrap", cfg.WordWrap, present.StdoutStyles().FlagDesc.Render(helpText["word-wrap"]))
	flags.BoolVar(&cfg.NoLimit, "no-limit", cfg.NoLimit, present.StdoutStyles().FlagDesc.Render(helpText["no-limit"]))
	flags.StringArrayVar(&cfg.Stop, "stop", cfg.Stop, present.StdoutStyles().FlagDesc.Render(helpText["stop"]))
	flags.UintVar(&cfg.Fanciness, "fanciness", cfg.Fanciness, present.StdoutStyles().FlagDesc.Render(helpText["fanciness"]))
	flags.StringVar(&cfg.StatusText, "status-text", cfg.StatusText, present.StdoutStyles().FlagDesc.Render(helpText["status-text"]))
	flags.StringVar(&cfg.Theme, "theme", "charm", present.StdoutStyles().FlagDesc.Render(helpText["theme"]))
	flags.StringArrayVar(&cfg.MCPDisable, "mcp-disable", nil, present.StdoutStyles().FlagDesc.Render(helpText["mcp-disable"]))
	flags.BoolVar(&cfg.MCPNoInheritEnv, "mcp-no-inherit-env", cfg.MCPNoInheritEnv, present.StdoutStyles().FlagDesc.Render(helpText["mcp-no-inherit-env"]))
	flags.SortFlags = false

	// Shell completions.
	_ = cmd.RegisterFlagCompletionFunc("continue", func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
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
	_ = cmd.RegisterFlagCompletionFunc("role", func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return roleNames(cfg, toComplete), cobra.ShellCompDirectiveDefault
	})

	cmd.MarkFlagsMutuallyExclusive("continue", "continue-last")
}

func (rt *runtime) runChat(ctx context.Context, args []string) error {
	initialPrompt := strings.TrimSpace(strings.Join(args, " "))

	store, err := openConversationStore(rt.cfg.CachePath)
	if err != nil {
		return errs.Wrap(err, "Could not open conversation store.")
	}
	defer store.Close() //nolint:errcheck

	pl, err := planConversation(&rt.cfg, store.DB)
	if err != nil {
		return err
	}
	rt.cfg.CacheWriteToID = pl.WriteID
	rt.cfg.CacheWriteToTitle = pl.Title
	rt.cfg.CacheReadFromID = pl.ReadID
	rt.cfg.API = pl.API
	rt.cfg.Model = pl.Model

	// Load existing messages if continuing.
	var history []proto.Message
	if !rt.cfg.NoCache && pl.ReadID != "" {
		if err := store.Cache.Read(pl.ReadID, &history); err != nil {
			return errs.Wrap(err, "There was a problem reading the conversation from cache.")
		}
	}

	agentSvc := agent.New(&rt.cfg, store.Cache, nil)

	saveFn := func(msgs []proto.Message) error {
		return saveConversationWithFeedback(&rt.cfg, store, msgs, false)
	}

	chat := tui.NewChat(ctx, present.StderrRenderer(), &rt.cfg, agentSvc, history, saveFn, initialPrompt)

	p := tea.NewProgram(chat, tea.WithAltScreen(), tea.WithOutput(os.Stderr))
	m, err := p.Run()
	if err != nil {
		return errs.Wrap(err, "Couldn't start chat program.")
	}

	c := m.(*tui.Chat)
	if c.Error != nil {
		return *c.Error
	}

	if len(c.Messages()) > 0 {
		if err := saveConversationWithFeedback(&rt.cfg, store, c.Messages(), true); err != nil {
			return err
		}
	}

	return nil
}
