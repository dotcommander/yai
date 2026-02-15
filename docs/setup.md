#+#+#+#+--------------------------------------------------------------------
## Purpose

Install yai, configure a provider, and run one successful prompt.

## Install

Choose one:

```bash
# Homebrew (macOS/Linux)
brew install charmbracelet/tap/yai

# Go toolchain
go install github.com/dotcommander/yai@latest
```

Verify the binary is on your PATH:

```bash
yai -v
```

## Configure

Open (or create) the settings file:

```bash
yai --settings
```

Equivalent subcommand:

```bash
yai config edit
```

Then set an API key. Example for OpenAI:

```bash
export OPENAI_API_KEY=...
```

Provider-specific notes and routing: [`docs/providers.md`](providers.md)

## First successful run

Run yai with a prompt:

```bash
yai "say hello in one short sentence"
```

Pipe command output into yai:

```bash
git status | yai "summarize what changed"
```

## Verification

- `yai -v` prints a version
- `yai "..."` prints a response
- `yai --settings` opens `~/.config/yai/yai.yml`

Equivalent subcommand:

```bash
yai config edit
```

## Related docs

- Pipelines and scripting contract: [`docs/integration.md`](integration.md)
- Settings file, roles, and env overrides: [`docs/configuration.md`](configuration.md)
- Example workflows to copy/paste: [`docs/common-tasks.md`](common-tasks.md)
