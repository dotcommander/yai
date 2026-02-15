package tui

import (
	"io"
	"os"
	"sync"
	"testing"

	"github.com/dotcommander/yai/internal/config"
	"github.com/stretchr/testify/require"
)

func TestRemoveWhitespace(t *testing.T) {
	t.Run("only whitespaces", func(t *testing.T) {
		require.Equal(t, "", removeWhitespace(" \n"))
	})

	t.Run("regular text", func(t *testing.T) {
		require.Equal(t, " regular\n ", removeWhitespace(" regular\n "))
	})
}

func TestUpdateFlushesBufferedContentForRawOutput(t *testing.T) {
	m := &Yai{Config: &config.Config{Settings: config.Settings{Raw: true}}, contentMutex: &sync.Mutex{}}
	m.content = []string{"hello from cache"}

	output := captureStdout(t, func() {
		_, _ = m.Update(completionOutput{})
	})

	require.Equal(t, "hello from cache", output)
	require.Empty(t, m.content)
	require.Equal(t, doneState, m.state)
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	fn()

	require.NoError(t, w.Close())
	os.Stdout = orig

	out, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, r.Close())
	return string(out)
}
