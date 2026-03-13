package cmd

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dotcommander/yai/internal/agent"
	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/errs"
	"github.com/dotcommander/yai/internal/present"
	"github.com/dotcommander/yai/internal/proto"
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
	registerSharedFlags(cmd, cfg)
	cmd.Flags().SortFlags = false

	cmd.MarkFlagsMutuallyExclusive("continue", "continue-last")
}

func (rt *runtime) runChat(ctx context.Context, args []string) error {
	initialPrompt := strings.TrimSpace(strings.Join(args, " "))

	store, err := rt.openAndPlanStore()
	if err != nil {
		return err
	}
	defer store.Close() //nolint:errcheck

	// Load existing messages if continuing.
	var history []proto.Message
	if !rt.cfg.NoCache && rt.cfg.CacheReadFromID != "" {
		if err := store.Cache.Read(rt.cfg.CacheReadFromID, &history); err != nil {
			return errs.Wrap(err, "There was a problem reading the conversation from cache.")
		}
	}

	agentSvc := agent.New(&rt.cfg, store.Cache, nil)
	startStreamFn := agentSvc.StreamContinue

	saveFn := func(msgs []proto.Message) error {
		return saveConversationWithFeedback(&rt.cfg, store, msgs, false)
	}

	chat := tui.NewChat(tui.ChatOptions{
		Context:       ctx,
		Renderer:      present.StderrRenderer(),
		Config:        &rt.cfg,
		Agent:         agentSvc,
		StartStream:   startStreamFn,
		History:       history,
		Save:          saveFn,
		InitialPrompt: initialPrompt,
	})

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
