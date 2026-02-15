package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/errs"
	"github.com/dotcommander/yai/internal/present"
	"github.com/dotcommander/yai/internal/proto"
	"github.com/dotcommander/yai/internal/storage"
	"github.com/dotcommander/yai/internal/storage/cache"
	"github.com/dotcommander/yai/internal/tui"
)

func saveConversation(cfg *config.Config, db *storage.DB, convoCache *cache.Conversations, yai *tui.Yai) error {
	if cfg.NoCache {
		if !cfg.Quiet {
			fmt.Fprintf(
				os.Stderr,
				"\nConversation was not saved because %s or %s is set.\n",
				present.StderrStyles().InlineCode.Render("--no-cache"),
				present.StderrStyles().InlineCode.Render("NO_CACHE"),
			)
		}
		return nil
	}

	id := cfg.CacheWriteToID
	title := strings.TrimSpace(cfg.CacheWriteToTitle)

	msgs := yai.Messages()
	if storage.SHA1Regexp.MatchString(title) || title == "" {
		title = firstLine(lastPrompt(msgs))
	}

	errReason := fmt.Sprintf(
		"There was a problem writing %s to the cache. Use %s / %s to disable it.",
		cfg.CacheWriteToID,
		present.StderrStyles().InlineCode.Render("--no-cache"),
		present.StderrStyles().InlineCode.Render("NO_CACHE"),
	)
	if err := convoCache.Write(id, &msgs); err != nil {
		return errs.Error{Err: err, Reason: errReason}
	}
	if err := db.Save(id, title, cfg.API, cfg.Model); err != nil {
		_ = convoCache.Delete(id)
		return errs.Error{Err: err, Reason: errReason}
	}

	if !cfg.Quiet {
		fmt.Fprintln(
			os.Stderr,
			"\nConversation saved:",
			present.StderrStyles().InlineCode.Render(cfg.CacheWriteToID[:storage.SHA1Short]),
			present.StderrStyles().Comment.Render(title),
		)
	}
	return nil
}

func lastPrompt(messages []proto.Message) string {
	var result string
	for _, msg := range messages {
		if msg.Role != proto.RoleUser {
			continue
		}
		if msg.Content == "" {
			continue
		}
		result = msg.Content
	}
	return result
}

func firstLine(s string) string {
	first, _, _ := strings.Cut(s, "\n")
	return first
}
