package cmd

import (
	"testing"

	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/errs"
	"github.com/dotcommander/yai/internal/storage"
	"github.com/stretchr/testify/require"
)

func testDB(tb testing.TB) *storage.DB {
	db, err := storage.Open(":memory:")
	require.NoError(tb, err)
	tb.Cleanup(func() {
		require.NoError(tb, db.Close())
	})
	return db
}

func TestPlanConversation(t *testing.T) {
	newCfg := func() *config.Config {
		return &config.Config{}
	}

	t.Run("all empty", func(t *testing.T) {
		db := testDB(t)
		cfg := newCfg()
		pl, err := planConversation(cfg, db)
		require.NoError(t, err)
		require.Empty(t, pl.ReadID)
		require.NotEmpty(t, pl.WriteID)
		require.Empty(t, pl.Title)
	})

	t.Run("show id", func(t *testing.T) {
		db := testDB(t)
		cfg := newCfg()
		id := storage.NewConversationID()
		require.NoError(t, db.Save(id, "message", "openai", "gpt-4"))
		cfg.Show = id[:8]

		pl, err := planConversation(cfg, db)
		require.NoError(t, err)
		require.Equal(t, id, pl.ReadID)
	})

	t.Run("show title", func(t *testing.T) {
		db := testDB(t)
		cfg := newCfg()
		id := storage.NewConversationID()
		require.NoError(t, db.Save(id, "message 1", "openai", "gpt-4"))
		cfg.Show = "message 1"

		pl, err := planConversation(cfg, db)
		require.NoError(t, err)
		require.Equal(t, id, pl.ReadID)
	})

	t.Run("continue id", func(t *testing.T) {
		db := testDB(t)
		cfg := newCfg()
		id := storage.NewConversationID()
		require.NoError(t, db.Save(id, "message", "openai", "gpt-4"))
		cfg.Continue = id[:5]
		cfg.Prefix = "prompt"

		pl, err := planConversation(cfg, db)
		require.NoError(t, err)
		require.Equal(t, id, pl.ReadID)
		require.Equal(t, id, pl.WriteID)
	})

	t.Run("continue with no prompt", func(t *testing.T) {
		db := testDB(t)
		cfg := newCfg()
		id := storage.NewConversationID()
		require.NoError(t, db.Save(id, "message 1", "openai", "gpt-4"))
		cfg.ContinueLast = true

		pl, err := planConversation(cfg, db)
		require.NoError(t, err)
		require.Equal(t, id, pl.ReadID)
		require.Equal(t, id, pl.WriteID)
		require.Empty(t, pl.Title)
	})

	t.Run("continue title", func(t *testing.T) {
		db := testDB(t)
		cfg := newCfg()
		id := storage.NewConversationID()
		require.NoError(t, db.Save(id, "message 1", "openai", "gpt-4"))
		cfg.Continue = "message 1"
		cfg.Prefix = "prompt"

		pl, err := planConversation(cfg, db)
		require.NoError(t, err)
		require.Equal(t, id, pl.ReadID)
		require.Equal(t, id, pl.WriteID)
	})

	t.Run("continue last", func(t *testing.T) {
		db := testDB(t)
		cfg := newCfg()
		id := storage.NewConversationID()
		require.NoError(t, db.Save(id, "message 1", "openai", "gpt-4"))
		cfg.ContinueLast = true
		cfg.Prefix = "prompt"

		pl, err := planConversation(cfg, db)
		require.NoError(t, err)
		require.Equal(t, id, pl.ReadID)
		require.Equal(t, id, pl.WriteID)
		require.Empty(t, pl.Title)
	})

	t.Run("continue last with name", func(t *testing.T) {
		db := testDB(t)
		cfg := newCfg()
		id := storage.NewConversationID()
		require.NoError(t, db.Save(id, "message 1", "openai", "gpt-4"))
		cfg.Continue = "message 2"
		cfg.Prefix = "prompt"

		pl, err := planConversation(cfg, db)
		require.NoError(t, err)
		require.Equal(t, id, pl.ReadID)
		require.Equal(t, "message 2", pl.Title)
		require.NotEmpty(t, pl.WriteID)
		require.Equal(t, id, pl.WriteID)
	})

	t.Run("write", func(t *testing.T) {
		db := testDB(t)
		cfg := newCfg()
		cfg.Title = "some title"

		pl, err := planConversation(cfg, db)
		require.NoError(t, err)
		require.Empty(t, pl.ReadID)
		require.NotEmpty(t, pl.WriteID)
		require.NotEqual(t, "some title", pl.WriteID)
		require.Equal(t, "some title", pl.Title)
	})

	t.Run("continue id and write with title", func(t *testing.T) {
		db := testDB(t)
		cfg := newCfg()
		id := storage.NewConversationID()
		require.NoError(t, db.Save(id, "message 1", "openai", "gpt-4"))
		cfg.Title = "some title"
		cfg.Continue = id[:10]

		pl, err := planConversation(cfg, db)
		require.NoError(t, err)
		require.Equal(t, id, pl.ReadID)
		require.NotEmpty(t, pl.WriteID)
		require.NotEqual(t, id, pl.WriteID)
		require.NotEqual(t, "some title", pl.WriteID)
		require.Equal(t, "some title", pl.Title)
	})

	t.Run("continue title and write with title", func(t *testing.T) {
		db := testDB(t)
		cfg := newCfg()
		id := storage.NewConversationID()
		require.NoError(t, db.Save(id, "message 1", "openai", "gpt-4"))
		cfg.Title = "some title"
		cfg.Continue = "message 1"

		pl, err := planConversation(cfg, db)
		require.NoError(t, err)
		require.Equal(t, id, pl.ReadID)
		require.NotEmpty(t, pl.WriteID)
		require.NotEqual(t, id, pl.WriteID)
		require.NotEqual(t, "some title", pl.WriteID)
		require.Equal(t, "some title", pl.Title)
	})

	t.Run("show invalid", func(t *testing.T) {
		db := testDB(t)
		cfg := newCfg()
		cfg.Show = "aaa"

		_, err := planConversation(cfg, db)
		require.Error(t, err)

		e := errs.Error{}
		require.ErrorAs(t, err, &e)
		require.Equal(t, "Could not find the conversation.", e.Reason)
		require.ErrorContains(t, e, "no conversations found: aaa")
	})

	t.Run("uses config model and api not global config", func(t *testing.T) {
		db := testDB(t)
		cfg := newCfg()
		cfg.Model = "claude-3.7-sonnet"
		cfg.API = "anthropic"

		pl, err := planConversation(cfg, db)
		require.NoError(t, err)
		require.Equal(t, "claude-3.7-sonnet", pl.Model)
		require.Equal(t, "anthropic", pl.API)
		require.Empty(t, pl.ReadID)
		require.NotEmpty(t, pl.WriteID)
	})
}
