package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/sync/errgroup"

	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/errs"
)

// Service provides access to MCP server discovery and tool execution.
type Service struct {
	cfg     *config.Config
	mu      sync.Mutex
	clients map[string]*client.Client
}

// New creates a new MCP service.
func New(cfg *config.Config) *Service {
	return &Service{cfg: cfg, clients: map[string]*client.Client{}}
}

// getClient returns a cached client for the named server, creating one if needed.
func (s *Service) getClient(ctx context.Context, name string, server config.MCPServerConfig) (*client.Client, error) {
	s.mu.Lock()
	if cli, ok := s.clients[name]; ok {
		s.mu.Unlock()
		return cli, nil
	}
	s.mu.Unlock()

	cli, err := initClient(ctx, s.cfg, server)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	if existing, ok := s.clients[name]; ok {
		s.mu.Unlock()
		cli.Close() //nolint:errcheck
		return existing, nil
	}
	s.clients[name] = cli
	s.mu.Unlock()
	return cli, nil
}

// Close shuts down all cached MCP clients.
func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for name, cli := range s.clients {
		cli.Close() //nolint:errcheck
		delete(s.clients, name)
	}
}

// IsEnabled reports whether the named MCP server is enabled.
func (s *Service) IsEnabled(name string) bool {
	return !slices.Contains(s.cfg.MCPDisable, "*") &&
		!slices.Contains(s.cfg.MCPDisable, name)
}

// EnabledServers iterates enabled MCP servers in stable order.
func (s *Service) EnabledServers() iter.Seq2[string, config.MCPServerConfig] {
	return func(yield func(string, config.MCPServerConfig) bool) {
		names := slices.Collect(maps.Keys(s.cfg.MCPServers))
		slices.Sort(names)
		for _, name := range names {
			if !s.IsEnabled(name) {
				continue
			}
			if !yield(name, s.cfg.MCPServers[name]) {
				return
			}
		}
	}
}

// Tools returns tools grouped by server name.
func (s *Service) Tools(ctx context.Context) (map[string][]mcp.Tool, error) {
	var mu sync.Mutex
	var wg errgroup.Group
	result := map[string][]mcp.Tool{}
	for sname, server := range s.EnabledServers() {
		wg.Go(func() error {
			serverTools, err := s.toolsFor(ctx, sname, server)
			if errors.Is(err, context.DeadlineExceeded) {
				return errs.Wrap(
					fmt.Errorf("timeout while listing tools for %q - make sure the configuration is correct. If your server requires a docker container, make sure it's running", sname),
					"Could not list tools",
				)
			}
			if err != nil {
				return errs.Wrap(err, "Could not list tools")
			}
			mu.Lock()
			result[sname] = append(result[sname], serverTools...)
			mu.Unlock()
			return nil
		})
	}
	if err := wg.Wait(); err != nil {
		return nil, fmt.Errorf("mcp tools: %w", err)
	}
	return result, nil
}

// CallTool executes a tool call against the configured server.
// fullName must be of the form: <server>_<tool>.
func (s *Service) CallTool(ctx context.Context, fullName string, data []byte) (string, error) {
	sname, tool, ok := strings.Cut(fullName, "_")
	if !ok {
		return "", fmt.Errorf("mcp: invalid tool name: %q", fullName)
	}
	server, ok := s.cfg.MCPServers[sname]
	if !ok {
		return "", fmt.Errorf("mcp: invalid server name: %q", sname)
	}
	if !s.IsEnabled(sname) {
		return "", fmt.Errorf("mcp: server is disabled: %q", sname)
	}
	cli, err := s.getClient(ctx, sname, server)
	if err != nil {
		return "", fmt.Errorf("mcp: %w", err)
	}

	var args map[string]any
	if len(data) > 0 {
		if err := json.Unmarshal(data, &args); err != nil {
			return "", fmt.Errorf("mcp: %w: %s", err, string(data))
		}
	}

	request := mcp.CallToolRequest{}
	request.Params.Name = tool
	request.Params.Arguments = args
	result, err := cli.CallTool(ctx, request)
	if err != nil {
		return "", fmt.Errorf("mcp: %w", err)
	}

	const maxToolResultBytes = 128 * 1024
	const truncMsg = "\n\n[output truncated at 128 KiB]"

	var sb strings.Builder
	for _, content := range result.Content {
		if sb.Len() >= maxToolResultBytes {
			break
		}
		switch content := content.(type) {
		case mcp.TextContent:
			remaining := maxToolResultBytes - sb.Len()
			if len(content.Text) > remaining {
				// Don't split mid-rune: back up to last valid rune boundary.
				truncated := content.Text[:remaining]
				for len(truncated) > 0 {
					r, _ := utf8.DecodeLastRuneInString(truncated)
					if r != utf8.RuneError {
						break
					}
					truncated = truncated[:len(truncated)-1]
				}
				sb.WriteString(truncated)
				sb.WriteString(truncMsg)
			} else {
				sb.WriteString(content.Text)
			}
		default:
			sb.WriteString("[Non-text content]")
		}
	}

	if result.IsError {
		return "", errors.New(sb.String())
	}
	return sb.String(), nil
}

// initClient creates and initializes an MCP client for the given server config.
// For stdio servers, the parent process environment is inherited by default
// (merged with any explicit server.Env entries) unless MCPNoInheritEnv is set.
func initClient(ctx context.Context, cfg *config.Config, server config.MCPServerConfig) (*client.Client, error) {
	var cli *client.Client
	var err error

	switch server.Type {
	case "", "stdio":
		env := server.Env
		if cfg != nil && !cfg.MCPNoInheritEnv {
			env = append(os.Environ(), server.Env...)
		}
		cli, err = client.NewStdioMCPClient(
			server.Command,
			env,
			server.Args...,
		)
	case "sse":
		var sseOpts []transport.ClientOption
		if len(server.Headers) > 0 {
			sseOpts = append(sseOpts, transport.WithHeaders(server.Headers))
		}
		cli, err = client.NewSSEMCPClient(server.URL, sseOpts...)
	case "http":
		var httpOpts []transport.StreamableHTTPCOption
		if len(server.Headers) > 0 {
			httpOpts = append(httpOpts, transport.WithHTTPHeaders(server.Headers))
		}
		cli, err = client.NewStreamableHttpClient(server.URL, httpOpts...)
	default:
		return nil, fmt.Errorf("unsupported MCP server type: %q, supported types are: stdio, sse, http", server.Type)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create MCP client: %w", err)
	}

	if err := cli.Start(ctx); err != nil {
		cli.Close() //nolint:errcheck,gosec
		return nil, fmt.Errorf("failed to start MCP client: %w", err)
	}

	if _, err := cli.Initialize(ctx, mcp.InitializeRequest{}); err != nil {
		cli.Close() //nolint:errcheck,gosec
		return nil, fmt.Errorf("failed to initialize MCP client: %w", err)
	}

	return cli, nil
}

func (s *Service) toolsFor(ctx context.Context, name string, server config.MCPServerConfig) ([]mcp.Tool, error) {
	cli, err := s.getClient(ctx, name, server)
	if err != nil {
		return nil, fmt.Errorf("could not setup %s: %w", name, err)
	}

	tools, err := cli.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("could not setup %s: %w", name, err)
	}
	return tools.Tools, nil
}
