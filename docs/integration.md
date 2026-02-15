## Purpose

Use yai in pipelines and scripts without guessing where output goes.

## I/O contract

- Prompt comes from CLI arguments (for example `yai "summarize this"`).
- Optional stdin is appended to the prompt when stdin is not a TTY.
- Response streams to stdout.
- Interactive UI (spinner/viewport) renders to stderr only when stdout is a TTY.
- Use `--quiet` to suppress non-error UI/warnings.

## Format control

Ask the model to format its output:

```bash
# Ask for JSON
echo "name: alice\nage: 29" | yai -f --format-as json "extract a JSON object"

# Ask for Markdown
git diff | yai -f --format-as markdown "write release notes"
```

If you need plain text for machine parsing, use `--raw`.

## Prompt shaping

Common flags that change what is sent:

- `--prompt-args` includes the CLI prompt in the streamed output
- `--prompt` includes N lines of stdin in the streamed output (`-P -1` means all)
- `--role <name>` prepends one or more system messages (roles) before the user prompt

Details and role file loading: [`docs/configuration.md`](configuration.md)

## Caching and reproducibility

yai saves conversations locally by default.

- Disable local writes: `--no-cache`
- List history: `yai --list` (or `yai history list`)
- Continue an existing conversation: `yai --continue <title-or-id> "..."`

Storage layout and delete operations: [`docs/conversations.md`](conversations.md)

## Related docs

- Copy/paste workflows: [`docs/common-tasks.md`](common-tasks.md)
- CLI reference surface: run `yai -h`
