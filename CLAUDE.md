# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

YAI is [dotcommander/yai](https://github.com/dotcommander/yai) — an AI CLI tool for piping command output through LLMs. It reads stdin, prepends a prompt, sends to an LLM API, and prints the response (optionally formatted as Markdown). Supports OpenAI, Anthropic, Google, Cohere, Ollama, and Azure OpenAI.

The binary is named `yai` and the Go module path is `github.com/dotcommander/yai`.

## Build & Test

```bash
go build ./...                     # build
go test ./...                      # all tests
go test -run TestFoo ./...         # single test
go test -v -cover -timeout=30s ./... # CI-style
golangci-lint run                  # lint (config in .golangci.yml)
```

The built binary is `./yai` (listed in .gitignore as `yai`).

## Architecture

### Bubble Tea TUI

The app is a [Bubble Tea](https://github.com/charmbracelet/bubbletea) program. The main model is `Yai` in `internal/tui/yai.go`. State machine flow:

```
startState → configLoadedState → requestState → responseState → doneState
```

- `Init()` resolves cache/conversation details
- `readStdinCmd` reads piped input
- `startCompletionCmd` calls the agent to resolve model/config and start streaming
- `receiveCompletionStreamCmd` iterates the stream, appending chunks to output
- `appendToOutput` renders Markdown via Glamour into a viewport for TTY output

### Provider Abstraction (`internal/`)

Each LLM provider implements `stream.Client` and `stream.Stream` (defined in `internal/stream/stream.go`):

| Package | Provider | SDK |
|---------|----------|-----|
| `internal/fantasybridge` | OpenAI-compatible + Anthropic + Google + Azure | `charm.land/fantasy` |

The `internal/proto` package defines the shared `Message`, `Request`, `Chunk`, and `ToolCall` types used across all providers.

### Conversation Storage

- **Metadata index**: JSONL append-only log at `~/.config/yai/history/conversations/index.jsonl` (managed by `internal/storage.DB`). Events are `upsert` or `delete`. Auto-compacts when ops exceed thresholds.
- **Payload cache**: Sharded JSON files under `~/.config/yai/history/conversations/<2-char-prefix>/<id>.json` (managed by `internal/storage/cache`). Legacy flat files are still readable.
- Conversations are identified by SHA-1 IDs (see `internal/storage/id.go`).

### Configuration

- `internal/config/config.go` defines `Config`, `API`, `Model` structs
- Settings file: `~/.config/yai/yai.yml` (templated from embedded `config_template.yml`)
- Environment override: `YAI_` prefix (parsed via `caarlos0/env`)
- CLI flags + routing: defined in `internal/cmd/` using cobra/pflag
- Roles/system prompts: loadable from strings, URLs, or `file://` paths (`internal/config/load.go`)

### MCP (Model Context Protocol)

`internal/mcp/service.go` supports MCP tool servers (stdio, SSE, HTTP) configured in the settings YAML under `mcp-servers`. Tools are discovered at request time and passed to the LLM. Tool calls are dispatched back through MCP clients.

### Key Files

| File | Purpose |
|------|---------|
| `main.go` | CLI entry (thin wrapper over `internal/cmd`) |
| `internal/cmd/root.go` | Cobra root + flag wiring + routing |
| `internal/tui/yai.go` | Bubble Tea model, streaming/render orchestration |
| `internal/agent/service.go` | Model resolution, auth, request assembly, stream start |
| `internal/agent/errors.go` | Provider error normalization + retry/fallback decisions |
| `internal/config/config.go` | Config structs + YAML/env parsing + defaults |
| `internal/config/load.go` | Role/system prompt loading (string/url/file://) |
| `internal/storage/db.go` | JSONL conversation metadata store |
| `internal/storage/cache/` | Conversation payload cache |
| `internal/mcp/service.go` | MCP server integration |
| `internal/present/styles.go` | Lipgloss styling helpers |
| `internal/tui/anim.go` | Loading animation |

## Operational Context

- Settings file path is `~/.config/yai/yai.yml`.
- Keep exactly one `main` package entrypoint (currently `main.go`). Having both `main.go` and `cmd/yai/main.go` creates duplicate `main` packages and can cause `go install ./...` to produce conflicting binaries.
- Roles can be loaded from `~/.config/yai/roles/`: filename (minus extension) is the role name; non-YAML files are loaded as file content, and `.yml/.yaml` files parse as a string or list of strings.
- Conversation cache base path defaults to `~/.config/yai/history`, with conversations under `~/.config/yai/history/conversations`.
- Conversation metadata index is stored as JSONL at `~/.config/yai/history/conversations/index.jsonl`.
- Conversation payload files are sharded by 2-char ID prefix under `~/.config/yai/history/conversations/<prefix>/<id>.json`.
- Legacy flat payload files at `~/.config/yai/history/conversations/<id>.json` are still readable/deletable for migration compatibility.
- models.dev provider/model sync is configured locally with:
  - script: `~/.config/yai/bin/models-dev-refresh.sh`
  - launch agent: `~/Library/LaunchAgents/dev.yai.modelsdev-refresh.plist`
  - interval: every 12 hours (`StartInterval=43200`)
  - outputs: `~/.config/yai/cache/models.dev.api.json` and `~/.config/yai/cache/models.dev.providers-models.json`
- Fantasy routing covers OpenAI-compatible APIs plus native providers for `anthropic`, `google`, `azure`, `azure-ad`, `openrouter`, `vercel`, and `bedrock`; `cohere` and `ollama` use `openaicompat` routing.
- Fantasy bridge maps Google `thinking-budget` via provider options.
- Fantasy bridge forwards `request.User` via provider options for Fantasy-routed OpenAI (`openai`/`azure`) and OpenAI-compatible APIs.
- Fantasy bridge forwards `max-completion-tokens` via OpenAI provider options for `openai`/`azure`/`azure-ad`.
- `stop` is still present in yai config/request, but the current Fantasy `Call` API (v0.8.1) has no direct stop-sequences field, so stop sequences are not currently forwarded by the bridge.
- When `stop` is configured and `quiet` is false, yai prints a runtime warning to stderr once per run to make the no-op behavior explicit.
- Fantasy stream `warnings` events are now surfaced to users (stderr, non-quiet) once per unique message via bridge warning dedupe + `DrainWarnings()`.
- Fantasy `ProviderExecuted` tool calls are skipped by yai MCP execution to avoid duplicate local tool invocation.
- Provider retry fallback now uses Fantasy-native `ProviderError.IsRetryable()` and `ErrorTitleForStatusCode(...)` instead of custom 500/default branching.
- HTTP 429 handling now also goes through the same Fantasy retryability path (no special-case branch), with reason text derived from `ErrorTitleForStatusCode(...)`.
- Non-retryable provider errors now also prefer Fantasy `ErrorTitleForStatusCode(...)` for user-facing reason text.
- Unauthorized (401) provider errors now also flow through Fantasy status-title mapping (no custom invalid-key branch).
- Retry wait timing now uses Fantasy `RetryWithExponentialBackoffRespectingRetryHeaders` (single-step) for provider errors so `retry-after` headers are honored.
- Current `charm.land/fantasy` version is v0.8.1; provider set is `anthropic`, `azure`, `bedrock`, `google`, `openai`, `openaicompat`, `openrouter`, `vercel`; `cohere` and `ollama` use `openaicompat` routing.
