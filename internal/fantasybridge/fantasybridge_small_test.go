//go:build yai_small

package fantasybridge

import (
	"testing"

	fopenaicompat "charm.land/fantasy/providers/openaicompat"
	"github.com/dotcommander/yai/internal/proto"
	"github.com/stretchr/testify/require"
)

func TestBuildCallUserProviderOptionsSmallBuild(t *testing.T) {
	s := &Stream{
		api: "deepseek",
		request: proto.Request{
			User: "alice",
		},
	}

	call := s.buildCall()
	v, ok := call.ProviderOptions[fopenaicompat.Name]
	require.True(t, ok)
	opts, ok := v.(*fopenaicompat.ProviderOptions)
	require.True(t, ok)
	require.NotNil(t, opts.User)
	require.Equal(t, "alice", *opts.User)
}
