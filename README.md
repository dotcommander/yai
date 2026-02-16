# yai

yai is a CLI that sends a prompt (and optional stdin) to an LLM and streams the response. Refactored from [charmbracelet/mods](https://github.com/charmbracelet/mods).

Use it for:

- Summarizing command output
- Generating structured text (Markdown, JSON, YAML) in pipelines
- Iterating on a prompt while keeping a local conversation history

## Quick start

1) Install

```bash
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

## Acknowledgments

yai is a refactored fork of [mods](https://github.com/charmbracelet/mods) by [Charmbracelet](https://charm.sh). The original project and its excellent Bubble Tea TUI framework made yai possible. Licensed under MIT — see [LICENSE](LICENSE).
