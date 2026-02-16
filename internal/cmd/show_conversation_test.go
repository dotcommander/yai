package cmd

import (
	"io"
	"os"
	"testing"

	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/proto"
	"github.com/dotcommander/yai/internal/storage"
	"github.com/stretchr/testify/require"
)

func captureStdout(tb testing.TB, fn func()) string {
	tb.Helper()

	orig := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(tb, err)
	os.Stdout = w

	fn()

	require.NoError(tb, w.Close())
	os.Stdout = orig

	out, err := io.ReadAll(r)
	require.NoError(tb, err)
	require.NoError(tb, r.Close())
	return string(out)
}

func TestShowConversation_Headless(t *testing.T) {
	store, tmpDir := newTestConversationStore(t)

	cfg := config.Config{}
	cfg.CachePath = tmpDir

	msgs1 := []proto.Message{
		{Role: proto.RoleUser, Content: "first"},
		{Role: proto.RoleAssistant, Content: "one"},
	}
	msgs2 := []proto.Message{
		{Role: proto.RoleUser, Content: "second"},
		{Role: proto.RoleAssistant, Content: "two"},
	}

	id1 := storage.NewConversationID()
	require.NoError(t, store.Cache.Write(id1, &msgs1))
	require.NoError(t, store.DB.Save(id1, "title-1", "openai", "gpt-4"))

	id2 := storage.NewConversationID()
	require.NoError(t, store.Cache.Write(id2, &msgs2))
	require.NoError(t, store.DB.Save(id2, "title-2", "openai", "gpt-4"))

	t.Run("show by id prefix", func(t *testing.T) {
		c := cfg
		c.Show = id1[:8]
		out := captureStdout(t, func() {
			require.NoError(t, showConversation(&c))
		})
		require.Equal(t, proto.Conversation(msgs1).String(), out)
	})

	t.Run("show by title", func(t *testing.T) {
		c := cfg
		c.Show = "title-1"
		out := captureStdout(t, func() {
			require.NoError(t, showConversation(&c))
		})
		require.Equal(t, proto.Conversation(msgs1).String(), out)
	})

	t.Run("show last", func(t *testing.T) {
		c := cfg
		c.ShowLast = true
		out := captureStdout(t, func() {
			require.NoError(t, showConversation(&c))
		})
		require.Equal(t, proto.Conversation(msgs2).String(), out)
	})
}
