## Purpose

Use MCP (Model Context Protocol) tool servers with yai.

yai discovers MCP tools at request time and exposes them to the model. When the model calls a tool, yai executes it via the configured MCP server.

## Commands

List configured MCP servers:

```bash
yai --mcp-list
```

Equivalent subcommand:

```bash
yai mcp list
```

List available tools (with a timeout):

```bash
yai --mcp-list-tools
```

Equivalent subcommand:

```bash
yai mcp tools
```

Disable one or more servers for a run:

```bash
yai --mcp-disable server-name --mcp-disable other-server "..."
```

## Related docs

- Settings schema and locations: [`docs/configuration.md`](configuration.md)
