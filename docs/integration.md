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

## Workflows

Copy/paste examples showing what yai is good at.

### Summarize and explain

```bash
git status | yai "summarize what changed and what to do next"
git log -n 20 --oneline | yai "turn this into a changelog"
```

### Refactor or review code

```bash
yai -f "review this code and propose a safer version" < main.go
yai -f --format-as markdown "write a PR description" < README.md
```

### Turn text into JSON

```bash
cat notes.txt | yai -f --format-as json "extract tasks as JSON array"
```

Then validate with a tool you control:

```bash
cat notes.txt | yai -f --format-as json "extract tasks as JSON array" | jq .
```

### Summarize API responses

```bash
curl -s "https://api.open-meteo.com/v1/forecast?latitude=29.00&longitude=-90.00&current_weather=true" \
  | yai "summarize this for a human"
```

## Related docs

- Settings file, roles, and env overrides: [`docs/configuration.md`](configuration.md)
- Storage layout and delete operations: [`docs/conversations.md`](conversations.md)
- CLI reference surface: run `yai -h`
