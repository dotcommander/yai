package fantasybridge

import (
	"errors"
	"testing"

	"charm.land/fantasy"
	"github.com/dotcommander/yai/internal/proto"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func TestToFantasyPrompt(t *testing.T) {
	messages := []proto.Message{
		{Role: proto.RoleSystem, Content: "sys"},
		{Role: proto.RoleUser, Content: "hello"},
		{Role: proto.RoleAssistant, Content: "calling tool", ToolCalls: []proto.ToolCall{{
			ID: "call_1",
			Function: proto.Function{
				Name:      "srv_tool",
				Arguments: []byte(`{"x":1}`),
			},
		}}},
		{Role: proto.RoleTool, Content: "ok", ToolCalls: []proto.ToolCall{{ID: "call_1"}}},
		{Role: proto.RoleTool, Content: "boom", ToolCalls: []proto.ToolCall{{ID: "call_2", IsError: true}}},
	}

	prompt := toFantasyPrompt(messages)
	require.Len(t, prompt, 5)

	require.Equal(t, fantasy.MessageRoleSystem, prompt[0].Role)
	require.Equal(t, fantasy.MessageRoleUser, prompt[1].Role)
	require.Equal(t, fantasy.MessageRoleAssistant, prompt[2].Role)
	require.Equal(t, fantasy.MessageRoleTool, prompt[3].Role)
	require.Equal(t, fantasy.MessageRoleTool, prompt[4].Role)

	resultPart, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](prompt[3].Content[0])
	require.True(t, ok)
	_, textOK := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](resultPart.Output)
	require.True(t, textOK)

	errPart, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](prompt[4].Content[0])
	require.True(t, ok)
	errOutput, errOK := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentError](errPart.Output)
	require.True(t, errOK)
	require.Equal(t, errors.New("boom").Error(), errOutput.Error.Error())
}

func TestFromMCPTools(t *testing.T) {
	tools := fromMCPTools(map[string][]mcp.Tool{
		"server": []mcp.Tool{
			{
				Name:        "search",
				Description: "search docs",
				InputSchema: mcp.ToolInputSchema{
					Properties: map[string]any{
						"query": map[string]any{"type": "string"},
					},
					Required: []string{"query"},
				},
			},
		},
	})

	require.Len(t, tools, 1)
	fn, ok := tools[0].(fantasy.FunctionTool)
	require.True(t, ok)
	require.Equal(t, "server_search", fn.Name)
	require.Equal(t, "search docs", fn.Description)
	require.Equal(t, "object", fn.InputSchema["type"])
	require.Equal(t, []string{"query"}, fn.InputSchema["required"])
}
