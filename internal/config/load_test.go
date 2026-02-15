package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadMsg(t *testing.T) {
	const content = "just text"

	t.Run("normal msg", func(t *testing.T) {
		msg, err := LoadMsg(content)
		require.NoError(t, err)
		require.Equal(t, content, msg)
	})

	t.Run("file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "foo.txt")
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

		msg, err := LoadMsg("file://" + path)
		require.NoError(t, err)
		require.Equal(t, content, msg)
	})

	t.Run("markdown file strips yaml frontmatter", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "role.md")
		md := "---\nname: helper\nstyle: calm\n---\nYou are concise and direct.\n"
		require.NoError(t, os.WriteFile(path, []byte(md), 0o644))

		msg, err := LoadMsg("file://" + path)
		require.NoError(t, err)
		require.Equal(t, "You are concise and direct.\n", msg)
	})

	t.Run("markdown file with invalid frontmatter errors", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "role.md")
		md := "---\nname: [broken\n---\ncontent"
		require.NoError(t, os.WriteFile(path, []byte(md), 0o644))

		_, err := LoadMsg("file://" + path)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid markdown frontmatter")
	})
}
