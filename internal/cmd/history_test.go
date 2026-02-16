package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/proto"
	"github.com/dotcommander/yai/internal/storage"
	"github.com/dotcommander/yai/internal/storage/cache"
	"github.com/stretchr/testify/require"
)

// newTestConversationStore creates a conversationStore backed by a temp directory.
// The DB and cache are cleaned up automatically when the test ends.
func newTestConversationStore(t *testing.T) (*conversationStore, string) {
	t.Helper()
	tmpDir := t.TempDir()
	convoDir := filepath.Join(tmpDir, "conversations")
	require.NoError(t, os.MkdirAll(convoDir, 0o700))

	db, err := storage.Open(convoDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	convoCache, err := cache.NewConversations(tmpDir)
	require.NoError(t, err)

	return &conversationStore{DB: db, Cache: convoCache}, tmpDir
}

func TestListConversations(t *testing.T) {
	t.Run("returns no error when no conversations exist", func(t *testing.T) {
		_, tmpDir := newTestConversationStore(t)
		cfg := &config.Config{
			Settings: config.Settings{CachePath: tmpDir},
		}

		err := listConversations(cfg, true)
		require.NoError(t, err)
	})

	t.Run("lists conversations when they exist", func(t *testing.T) {
		store, tmpDir := newTestConversationStore(t)
		require.NoError(t, store.DB.Save("abc123def456", "test conversation", "openai", "test-model"))

		cfg := &config.Config{
			Settings: config.Settings{CachePath: tmpDir},
		}

		err := listConversations(cfg, true)
		require.NoError(t, err)
	})
}

func TestDeleteConversations(t *testing.T) {
	t.Run("deletes single conversation", func(t *testing.T) {
		store, tmpDir := newTestConversationStore(t)
		require.NoError(t, store.DB.Save("abc123def456", "test conversation", "openai", "test-model"))
		messages := []proto.Message{}
		require.NoError(t, store.Cache.Write("abc123def456", &messages))
		// Close so deleteConversations can open its own store.
		require.NoError(t, store.Close())

		cfg := &config.Config{
			Settings: config.Settings{CachePath: tmpDir, Quiet: true},
		}

		err := deleteConversations(cfg, []string{"abc123def456"})
		require.NoError(t, err)

		// Re-open to verify deletion.
		db, err := storage.Open(filepath.Join(tmpDir, "conversations"))
		require.NoError(t, err)
		defer db.Close() //nolint:errcheck
		_, err = db.Find("abc123def456")
		require.Error(t, err)
	})

	t.Run("deletes multiple conversations", func(t *testing.T) {
		store, tmpDir := newTestConversationStore(t)
		require.NoError(t, store.DB.Save("abc123def456", "first", "openai", "test-model"))
		require.NoError(t, store.DB.Save("def456abc123", "second", "openai", "test-model"))
		messages := []proto.Message{}
		require.NoError(t, store.Cache.Write("abc123def456", &messages))
		require.NoError(t, store.Cache.Write("def456abc123", &messages))
		require.NoError(t, store.Close())

		cfg := &config.Config{
			Settings: config.Settings{CachePath: tmpDir, Quiet: true},
		}

		err := deleteConversations(cfg, []string{"abc123def456", "def456abc123"})
		require.NoError(t, err)

		db, err := storage.Open(filepath.Join(tmpDir, "conversations"))
		require.NoError(t, err)
		defer db.Close() //nolint:errcheck
		convos := db.List()
		require.Len(t, convos, 0)
	})
}

func TestDeleteConversationByID(t *testing.T) {
	t.Run("deletes conversation from both index and cache", func(t *testing.T) {
		store, _ := newTestConversationStore(t)
		require.NoError(t, store.DB.Save("test123abc456", "test", "openai", "test-model"))
		messages := []proto.Message{}
		require.NoError(t, store.Cache.Write("test123abc456", &messages))

		cfg := &config.Config{
			Settings: config.Settings{Quiet: true},
		}

		err := deleteConversationByID(cfg, store, "test123abc456")
		require.NoError(t, err)

		_, err = store.DB.Find("test123abc456")
		require.Error(t, err)
	})
}
