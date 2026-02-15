package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	timeago "github.com/caarlos0/timea.go"
	"github.com/charmbracelet/huh"
	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/errs"
	"github.com/dotcommander/yai/internal/present"
	"github.com/dotcommander/yai/internal/storage"
	"github.com/dotcommander/yai/internal/storage/cache"
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
			db, err := storage.Open(filepath.Join(rt.cfg.CachePath, "conversations"))
			if err != nil {
				return errs.Error{Err: err, Reason: "Could not open database."}
			}
			defer db.Close() //nolint:errcheck

			conversations := db.List()
			if len(conversations) == 0 {
				fmt.Fprintln(os.Stderr, "No conversations found.")
				return nil
			}

			if present.IsInputTTY() && present.IsOutputTTY() && !rt.cfg.Raw {
				selectFromList(conversations)
				return nil
			}
			printList(conversations)
			return nil
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
			convoCache, err := cache.NewConversations(rt.cfg.CachePath)
			if err != nil {
				return errs.Error{Err: err, Reason: "Couldn't delete conversation."}
			}
			db, err := storage.Open(filepath.Join(rt.cfg.CachePath, "conversations"))
			if err != nil {
				return errs.Error{Err: err, Reason: "Could not open database."}
			}
			defer db.Close() //nolint:errcheck

			for _, del := range args {
				convo, err := db.Find(del)
				if err != nil {
					return errs.Error{Err: err, Reason: "Couldn't find conversation to delete."}
				}
				if err := deleteConversationByID(&rt.cfg, db, convoCache, convo.ID); err != nil {
					return err
				}
			}
			return nil
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
				return errs.Error{Err: errs.UserErrorf("missing --older-than"), Reason: "Could not delete old conversations."}
			}

			db, err := storage.Open(filepath.Join(rt.cfg.CachePath, "conversations"))
			if err != nil {
				return errs.Error{Err: err, Reason: "Could not open database."}
			}
			defer db.Close() //nolint:errcheck

			conversations := db.ListOlderThan(olderThan)
			if len(conversations) == 0 {
				if !rt.cfg.Quiet {
					fmt.Fprintln(os.Stderr, "No conversations found.")
				}
				return nil
			}

			if !rt.cfg.Quiet {
				printList(conversations)

				if !present.IsOutputTTY() || !present.IsInputTTY() {
					fmt.Fprintln(os.Stderr)
					return errs.UserErrorf(
						"To delete the conversations above, run: %s",
						strings.Join(append(os.Args, "--quiet"), " "),
					)
				}
				var confirm bool
				if err := huh.Run(
					huh.NewConfirm().
						Title(fmt.Sprintf("Delete conversations older than %s?", olderThan)).
						Description(fmt.Sprintf("This will delete all the %d conversations listed above.", len(conversations))).
						Value(&confirm),
				); err != nil {
					return errs.Error{Err: err, Reason: "Couldn't delete old conversations."}
				}
				if !confirm {
					return errs.UserErrorf("Aborted by user")
				}
			}

			convoCache, err := cache.NewConversations(rt.cfg.CachePath)
			if err != nil {
				return errs.Error{Err: err, Reason: "Couldn't delete conversation."}
			}
			for _, c := range conversations {
				if err := deleteConversationByID(&rt.cfg, db, convoCache, c.ID); err != nil {
					return err
				}
			}
			return nil
		},
	}
	pruneCmd.Flags().Var(newDurationFlag(olderThan, &olderThan), "older-than", "Duration to prune; e.g. 24h, 7d")
	return pruneCmd
}

func deleteConversationByID(cfg *config.Config, db *storage.DB, convoCache *cache.Conversations, id string) error {
	if err := db.Delete(id); err != nil {
		return fmt.Errorf("delete conversation index: %w", err)
	}
	if err := convoCache.Delete(id); err != nil {
		return fmt.Errorf("delete conversation payload: %w", err)
	}
	if !cfg.Quiet {
		fmt.Fprintln(os.Stderr, "Conversation deleted:", id[:storage.SHA1MinLen])
	}
	return nil
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
