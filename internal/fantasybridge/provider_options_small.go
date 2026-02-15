//go:build yai_small

package fantasybridge

import (
	"charm.land/fantasy"
	fopenaicompat "charm.land/fantasy/providers/openaicompat"
	"github.com/dotcommander/yai/internal/proto"
)

func applyProviderOptions(call *fantasy.Call, api string, cfg Config, req proto.Request) {
	_ = api
	_ = cfg
	if req.User == "" {
		return
	}
	user := req.User
	call.ProviderOptions[fopenaicompat.Name] = &fopenaicompat.ProviderOptions{User: &user}
}
