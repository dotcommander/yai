// Package fantasybridge converts yai protocol messages to/from Fantasy types.
package fantasybridge

import (
	"errors"
	"fmt"

	"charm.land/fantasy"
	"github.com/dotcommander/yai/internal/proto"
	"github.com/mark3labs/mcp-go/mcp"
)

func toFantasyPrompt(input []proto.Message) fantasy.Prompt {
	messages := make([]fantasy.Message, 0, len(input))

	for _, msg := range input {
		switch msg.Role {
		case proto.RoleSystem:
			messages = append(messages, fantasy.Message{
				Role: fantasy.MessageRoleSystem,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: msg.Content},
				},
			})
		case proto.RoleUser:
			messages = append(messages, fantasy.Message{
				Role: fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: msg.Content},
				},
			})
		case proto.RoleAssistant:
			parts := make([]fantasy.MessagePart, 0, 1+len(msg.ToolCalls))
			if msg.Content != "" {
				parts = append(parts, fantasy.TextPart{Text: msg.Content})
			}
			for _, call := range msg.ToolCalls {
				parts = append(parts, fantasy.ToolCallPart{
					ToolCallID:       call.ID,
					ToolName:         call.Function.Name,
					Input:            string(call.Function.Arguments),
					ProviderExecuted: false,
				})
			}
			if len(parts) > 0 {
				messages = append(messages, fantasy.Message{
					Role:    fantasy.MessageRoleAssistant,
					Content: parts,
				})
			}
		case proto.RoleTool:
			parts := make([]fantasy.MessagePart, 0, len(msg.ToolCalls))
			for _, call := range msg.ToolCalls {
				var output fantasy.ToolResultOutputContent
				if call.IsError {
					output = fantasy.ToolResultOutputContentError{Error: errors.New(msg.Content)}
				} else {
					output = fantasy.ToolResultOutputContentText{Text: msg.Content}
				}
				parts = append(parts, fantasy.ToolResultPart{
					ToolCallID: call.ID,
					Output:     output,
				})
			}
			if len(parts) > 0 {
				messages = append(messages, fantasy.Message{
					Role:    fantasy.MessageRoleTool,
					Content: parts,
				})
			}
		}
	}

	return messages
}

func fromMCPTools(mcps map[string][]mcp.Tool) []fantasy.Tool {
	tools := make([]fantasy.Tool, 0)
	for serverName, serverTools := range mcps {
		for _, tool := range serverTools {
			inputSchema := map[string]any{
				"type":       "object",
				"properties": tool.InputSchema.Properties,
			}
			if len(tool.InputSchema.Required) > 0 {
				inputSchema["required"] = tool.InputSchema.Required
			}

			tools = append(tools, fantasy.FunctionTool{
				Name:        fmt.Sprintf("%s_%s", serverName, tool.Name),
				Description: tool.Description,
				InputSchema: inputSchema,
			})
		}
	}
	return tools
}

func toolChoiceForRequest(request proto.Request) *fantasy.ToolChoice {
	if len(request.Tools) == 0 {
		return nil
	}
	choice := fantasy.ToolChoiceAuto
	return &choice
}
