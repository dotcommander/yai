## Purpose

Understand what yai reads from config and environment.

## Settings file

- Default path: `~/.config/yai/yai.yml`
- Open/edit it: `yai --settings`

Equivalent subcommand:

```bash
yai config edit
```

Settings define:

- API endpoints (`apis`) and their models
- defaults (API, model, formatting, caching)
- roles (system prompt presets)
- MCP servers (tool discovery/execution)

## Environment overrides

yai supports `YAI_` environment overrides for config fields.

Example pattern:

```bash
export YAI_API=openai
export YAI_MODEL=gpt-4.1
```

## API keys

Keys can be provided in three ways (highest precedence first):

1. `api-key` in settings
2. `api-key-env` (read from an env var)
3. `api-key-cmd` (exec a local command and read stdout)

Some providers also fall back to well-known env vars (for example `OPENAI_API_KEY`).

Routing and provider behaviors: [`docs/providers.md`](providers.md)

## Roles

Roles prepend system messages before your user prompt.

In `yai.yml`:

```yaml
roles:
  shell:
    - you are a shell expert
    - you do not explain anything
    - you only output the command
```

Use a role:

```bash
yai --role shell "list files in the current directory"
```

Role files can also live under `~/.config/yai/roles/`:

- Markdown files (`.md`) and other non-YAML text files are loaded as file content
- YAML files (`.yml` or `.yaml`) parse as a string or list of strings
- Discovery is recursive
- The role name is the relative path without extension
- Markdown files may include YAML frontmatter; frontmatter is ignored

## Related docs

- MCP tool servers: [`docs/mcp.md`](mcp.md)
- Pipelines and stdout/stderr: [`docs/integration.md`](integration.md)
