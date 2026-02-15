package cmd

import (
	"context"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/dotcommander/yai/internal/config"
	imcp "github.com/dotcommander/yai/internal/mcp"
	"github.com/dotcommander/yai/internal/present"
	mmcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/spf13/cobra"
)

func newMCPCmd(rt *runtime) *cobra.Command {
	mcpCmd := &cobra.Command{
		Use:   "mcp",
		Short: "MCP server integration",
	}

	mcpCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List configured MCP servers",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if rt.cfgErr != nil {
				return rt.cfgErr
			}
			mcpList(&rt.cfg)
			return nil
		},
	})

	mcpCmd.AddCommand(&cobra.Command{
		Use:   "tools",
		Short: "List tools from enabled MCP servers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if rt.cfgErr != nil {
				return rt.cfgErr
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), rt.cfg.MCPTimeout)
			defer cancel()
			return mcpListTools(ctx, &rt.cfg)
		},
	})

	return mcpCmd
}

func mcpList(cfg *config.Config) {
	svc := imcp.New(cfg)
	names := slices.Collect(maps.Keys(cfg.MCPServers))
	slices.Sort(names)
	for _, name := range names {
		s := name
		if svc.IsEnabled(name) {
			s += present.StdoutStyles().Timeago.Render(" (enabled)")
		}
		fmt.Println(s)
	}
}

func mcpListTools(ctx context.Context, cfg *config.Config) error {
	svc := imcp.New(cfg)
	servers, err := svc.Tools(ctx)
	if err != nil {
		return fmt.Errorf("mcp list tools: %w", err)
	}

	names := slices.Collect(maps.Keys(servers))
	slices.Sort(names)
	for _, sname := range names {
		tools := servers[sname]
		slices.SortFunc(tools, func(a, b mmcp.Tool) int { return strings.Compare(a.Name, b.Name) })
		for _, tool := range tools {
			_, _ = fmt.Fprint(os.Stdout, present.StdoutStyles().Timeago.Render(sname+" > "))
			_, _ = fmt.Fprintln(os.Stdout, tool.Name)
		}
	}
	return nil
}
