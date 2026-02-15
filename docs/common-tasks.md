## Purpose

Copy/paste workflows that show what yai is good at.

## Summarize and explain

```bash
git status | yai "summarize what changed and what to do next"
git log -n 20 --oneline | yai "turn this into a changelog"
```

## Refactor or review code

```bash
yai -f "review this code and propose a safer version" < main.go
yai -f --format-as markdown "write a PR description" < README.md
```

## Turn text into JSON

```bash
cat notes.txt | yai -f --format-as json "extract tasks as JSON array"
```

Then validate with a tool you control:

```bash
cat notes.txt | yai -f --format-as json "extract tasks as JSON array" | jq .
```

## Summarize API responses

```bash
curl -s "https://api.open-meteo.com/v1/forecast?latitude=29.00&longitude=-90.00&current_weather=true" \
  | yai "summarize this for a human"
```

## Related docs

- Pipeline semantics: [`docs/integration.md`](integration.md)
- Formatting knobs: [`docs/configuration.md`](configuration.md)
