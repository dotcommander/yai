# yai

yai is a CLI that sends a prompt (and optional stdin) to an LLM and streams the response.

Use it for:

- Summarizing command output
- Generating structured text (Markdown, JSON, YAML) in pipelines
- Iterating on a prompt while keeping a local conversation history

## Quick start

1) Install

```bash
brew install charmbracelet/tap/yai
# or
go install github.com/dotcommander/yai@latest
```

2) Configure

```bash
export OPENAI_API_KEY=... # or configure another provider
yai --settings            # creates/edits ~/.config/yai/yai.yml
```

3) First successful run

```bash
git status | yai "summarize what changed"
```

Docs: [`docs/README.md`](docs/README.md)
