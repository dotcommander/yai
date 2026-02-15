package present

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderMarkdownForTTY(t *testing.T) {
	out, err := RenderMarkdownForTTY("hello\tworld\n", 80)
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(out, "\n"))
	require.False(t, strings.Contains(out, "\t"))
}
