package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/errs"
	"github.com/dotcommander/yai/internal/present"
	"github.com/dotcommander/yai/internal/storage"
	"github.com/dotcommander/yai/internal/storage/cache"
)

func listConversations(cfg *config.Config, raw bool) error {
	db, err := storage.Open(filepath.Join(cfg.CachePath, "conversations"))
	if err != nil {
		return errs.Error{Err: err, Reason: "Could not open database."}
	}
	defer db.Close() //nolint:errcheck

	conversations := db.List()
	if len(conversations) == 0 {
		fmt.Fprintln(os.Stderr, "No conversations found.")
		return nil
	}

	if present.IsInputTTY() && present.IsOutputTTY() && !raw {
		selectFromList(conversations)
		return nil
	}
	printList(conversations)
	return nil
}

func deleteConversations(cfg *config.Config, targets []string) error {
	convoCache, err := cache.NewConversations(cfg.CachePath)
	if err != nil {
		return errs.Error{Err: err, Reason: "Couldn't delete conversation."}
	}
	db, err := storage.Open(filepath.Join(cfg.CachePath, "conversations"))
	if err != nil {
		return errs.Error{Err: err, Reason: "Could not open database."}
	}
	defer db.Close() //nolint:errcheck

	for _, del := range targets {
		convo, err := db.Find(del)
		if err != nil {
			return errs.Error{Err: err, Reason: "Couldn't find conversation to delete."}
		}
		if err := deleteConversationByID(cfg, db, convoCache, convo.ID); err != nil {
			return err
		}
	}
	return nil
}

func deleteConversationsOlderThan(cfg *config.Config, olderThanDuration string) error {
	if cfg.DeleteOlderThan == 0 {
		return errs.Error{Err: errs.UserErrorf("missing --delete-older-than"), Reason: "Could not delete old conversations."}
	}

	db, err := storage.Open(filepath.Join(cfg.CachePath, "conversations"))
	if err != nil {
		return errs.Error{Err: err, Reason: "Could not open database."}
	}
	defer db.Close() //nolint:errcheck

	conversations := db.ListOlderThan(cfg.DeleteOlderThan)
	if len(conversations) == 0 {
		if !cfg.Quiet {
			fmt.Fprintln(os.Stderr, "No conversations found.")
		}
		return nil
	}

	if !cfg.Quiet {
		printList(conversations)

		if !present.IsOutputTTY() || !present.IsInputTTY() {
			fmt.Fprintln(os.Stderr)
			//nolint:wrapcheck // user-facing guidance error
			return errs.UserErrorf(
				"To delete the conversations above, run: %s",
				strings.Join(append(os.Args, "--quiet"), " "),
			)
		}
		var confirm bool
		if err := huh.Run(
			huh.NewConfirm().
				Title(fmt.Sprintf("Delete conversations older than %s?", olderThanDuration)).
				Description(fmt.Sprintf("This will delete all the %d conversations listed above.", len(conversations))).
				Value(&confirm),
		); err != nil {
			return errs.Error{Err: err, Reason: "Couldn't delete old conversations."}
		}
		if !confirm {
			//nolint:wrapcheck // user-facing abort
			return errs.UserErrorf("Aborted by user")
		}
	}

	convoCache, err := cache.NewConversations(cfg.CachePath)
	if err != nil {
		return errs.Error{Err: err, Reason: "Couldn't delete conversation."}
	}
	for _, c := range conversations {
		if err := deleteConversationByID(cfg, db, convoCache, c.ID); err != nil {
			return err
		}
	}
	return nil
}
