package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/errs"
	"github.com/dotcommander/yai/internal/present"
	"github.com/dotcommander/yai/internal/proto"
	"github.com/dotcommander/yai/internal/storage"
	"github.com/dotcommander/yai/internal/storage/cache"
)

// conversationStore bundles the DB index and payload cache that together
// form a conversation store. Most cmd functions need both; this avoids
// repeating the open-and-check boilerplate at every call site.
type conversationStore struct {
	DB    *storage.DB
	Cache *cache.Conversations
}

// openConversationStore opens both the metadata DB and the payload cache.
func openConversationStore(cachePath string) (*conversationStore, error) {
	convoCache, err := cache.NewConversations(cachePath)
	if err != nil {
		return nil, fmt.Errorf("open conversation cache: %w", err)
	}
	db, err := storage.Open(filepath.Join(cachePath, "conversations"))
	if err != nil {
		return nil, fmt.Errorf("open conversation database: %w", err)
	}
	return &conversationStore{DB: db, Cache: convoCache}, nil
}

// Close releases the underlying DB resources.
func (s *conversationStore) Close() error {
	return s.DB.Close()
}

func saveConversation(cfg *config.Config, store *conversationStore, msgs []proto.Message) error {
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

	if storage.SHA1Regexp.MatchString(title) || title == "" {
		title = firstLine(lastPrompt(msgs))
	}

	errReason := fmt.Sprintf(
		"There was a problem writing %s to the cache. Use %s / %s to disable it.",
		cfg.CacheWriteToID,
		present.StderrStyles().InlineCode.Render("--no-cache"),
		present.StderrStyles().InlineCode.Render("NO_CACHE"),
	)
	if err := store.Cache.Write(id, &msgs); err != nil {
		return errs.Wrap(err, errReason)
	}
	if err := store.DB.Save(id, title, cfg.API, cfg.Model); err != nil {
		_ = store.Cache.Delete(id)
		return errs.Wrap(err, errReason)
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
