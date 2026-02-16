package cmd

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/atotto/clipboard"
	timeago "github.com/caarlos0/timea.go"
	"github.com/charmbracelet/huh"
	"github.com/dotcommander/yai/internal/errs"
	"github.com/dotcommander/yai/internal/present"
	"github.com/dotcommander/yai/internal/storage"
	"github.com/muesli/termenv"

	"github.com/spf13/cobra"
)

func newHistoryCmd(rt *runtime) *cobra.Command {
	historyCmd := &cobra.Command{
		Use:   "history",
		Short: "Manage saved conversations",
	}

	historyCmd.AddCommand(newHistoryListCmd(rt))
	historyCmd.AddCommand(newHistoryShowCmd(rt))
	historyCmd.AddCommand(newHistoryDeleteCmd(rt))
	historyCmd.AddCommand(newHistoryPruneCmd(rt))

	return historyCmd
}

func newHistoryListCmd(rt *runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List saved conversations",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if rt.cfgErr != nil {
				return rt.cfgErr
			}
			return listConversations(&rt.cfg, rt.cfg.Raw)
		},
	}
}

func newHistoryShowCmd(rt *runtime) *cobra.Command {
	var last bool
	showCmd := &cobra.Command{
		Use:   "show [id-or-title]",
		Short: "Show a saved conversation",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if rt.cfgErr != nil {
				return rt.cfgErr
			}
			drainStdin()
			cfg := rt.cfg
			cfg.Show = ""
			cfg.ShowLast = last
			if len(args) == 1 {
				cfg.Show = args[0]
			}
			return showConversation(&cfg)
		},
	}
	showCmd.Flags().BoolVarP(&last, "last", "S", false, "Show the last saved conversation")
	return showCmd
}

func newHistoryDeleteCmd(rt *runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id-or-title> [more...]",
		Short: "Delete saved conversations",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if rt.cfgErr != nil {
				return rt.cfgErr
			}
			return deleteConversations(&rt.cfg, args)
		},
	}
}

func newHistoryPruneCmd(rt *runtime) *cobra.Command {
	var olderThan time.Duration
	pruneCmd := &cobra.Command{
		Use:   "prune",
		Short: "Delete conversations older than a duration",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if rt.cfgErr != nil {
				return rt.cfgErr
			}
			if olderThan == 0 {
				return errs.Wrap(errs.UserErrorf("missing --older-than"), "Could not delete old conversations.")
			}
			rt.cfg.DeleteOlderThan = olderThan
			return deleteConversationsOlderThan(&rt.cfg, olderThan.String())
		},
	}
	pruneCmd.Flags().Var(newDurationFlag(olderThan, &olderThan), "older-than", "Duration to prune; e.g. 24h, 7d")
	return pruneCmd
}

func makeOptions(conversations []storage.Conversation) []huh.Option[string] {
	opts := make([]huh.Option[string], 0, len(conversations))
	for _, c := range conversations {
		timea := present.StdoutStyles().Timeago.Render(timeago.Of(c.UpdatedAt))
		left := present.StdoutStyles().SHA1.Render(c.ID[:storage.SHA1Short])
		right := present.StdoutStyles().ConversationList.Render(c.Title, timea)
		if c.Model != nil {
			right += present.StdoutStyles().Comment.Render(*c.Model)
		}
		if c.API != nil {
			right += present.StdoutStyles().Comment.Render(" (" + *c.API + ")")
		}
		opts = append(opts, huh.NewOption(left+" "+right, c.ID))
	}
	return opts
}

func selectFromList(conversations []storage.Conversation) {
	var selected string
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Conversations").
				Value(&selected).
				Options(makeOptions(conversations)...),
		),
	).Run(); err != nil {
		if !errors.Is(err, huh.ErrUserAborted) {
			fmt.Fprintln(os.Stderr, err.Error())
		}
		return
	}

	_ = clipboard.WriteAll(selected)
	termenv.Copy(selected)
	present.PrintConfirmation("COPIED", selected)

	fmt.Println(present.StdoutStyles().Comment.Render("You can use this conversation ID with the following commands:"))
	suggestions := []string{
		"yai --show " + selected,
		"yai --continue " + selected,
		"yai --delete " + selected,
	}
	for _, s := range suggestions {
		fmt.Printf("  %s\n", present.StdoutStyles().InlineCode.Render(s))
	}
}

func printList(conversations []storage.Conversation) {
	for _, conversation := range conversations {
		_, _ = fmt.Fprintf(
			os.Stdout,
			"%s\t%s\t%s\n",
			present.StdoutStyles().SHA1.Render(conversation.ID[:storage.SHA1Short]),
			conversation.Title,
			present.StdoutStyles().Timeago.Render(timeago.Of(conversation.UpdatedAt)),
		)
	}
}
