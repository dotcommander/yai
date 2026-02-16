package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/errs"
	"github.com/dotcommander/yai/internal/present"
	"github.com/dotcommander/yai/internal/storage"
)

func listConversations(cfg *config.Config, raw bool) error {
	store, err := openConversationStore(cfg.CachePath)
	if err != nil {
		return errs.Wrap(err, "Could not open conversation store.")
	}
	defer store.Close() //nolint:errcheck

	conversations := store.DB.List()
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
	store, err := openConversationStore(cfg.CachePath)
	if err != nil {
		return errs.Wrap(err, "Couldn't delete conversation.")
	}
	defer store.Close() //nolint:errcheck

	for _, del := range targets {
		convo, err := store.DB.Find(del)
		if err != nil {
			return errs.Wrap(err, "Couldn't find conversation to delete.")
		}
		if err := deleteConversationByID(cfg, store, convo.ID); err != nil {
			return err
		}
	}
	return nil
}

func deleteConversationByID(cfg *config.Config, store *conversationStore, id string) error {
	if err := store.DB.Delete(id); err != nil {
		return fmt.Errorf("delete conversation index: %w", err)
	}
	if err := store.Cache.Delete(id); err != nil {
		return fmt.Errorf("delete conversation payload: %w", err)
	}
	if !cfg.Quiet {
		fmt.Fprintln(os.Stderr, "Conversation deleted:", id[:storage.SHA1MinLen])
	}
	return nil
}

func deleteConversationsOlderThan(cfg *config.Config, olderThanDuration string) error {
	if cfg.DeleteOlderThan == 0 {
		return errs.Wrap(errs.UserErrorf("missing --delete-older-than"), "Could not delete old conversations.")
	}

	store, err := openConversationStore(cfg.CachePath)
	if err != nil {
		return errs.Wrap(err, "Could not open conversation store.")
	}
	defer store.Close() //nolint:errcheck

	conversations := store.DB.ListOlderThan(cfg.DeleteOlderThan)
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
			return errs.Wrap(err, "Couldn't delete old conversations.")
		}
		if !confirm {
			//nolint:wrapcheck // user-facing abort
			return errs.UserErrorf("Aborted by user")
		}
	}

	for _, c := range conversations {
		if err := deleteConversationByID(cfg, store, c.ID); err != nil {
			return err
		}
	}
	return nil
}
