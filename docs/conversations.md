## Purpose

Use the local conversation history safely and predictably.

## What is saved

By default, yai saves:

- the message history (system/user/assistant/tool messages)
- a title (defaults to the first line of your last prompt)
- provider metadata (API/model)

Disable saving:

```bash
yai --no-cache "..."
```

## Storage locations

Conversation data lives under your config directory:

- Config: `~/.config/yai/yai.yml`
- History root: `~/.config/yai/history/`
- Conversation payloads: `~/.config/yai/history/conversations/`

## List, show, continue

```bash
yai --list
yai --show <title-or-id>
yai --title "my-title" "first prompt"
yai --continue <title-or-id> "follow up prompt"
yai --continue-last "follow up prompt"
```

Equivalent subcommands:

```bash
yai history list
yai history show <title-or-id>
yai --title "my-title" "first prompt"
yai --continue <title-or-id> "follow up prompt"
yai --continue-last "follow up prompt"
```

## Branching

You can branch a conversation by continuing from one title/ID but saving to a new title:

```bash
yai --title naturals "first 5 natural numbers"
yai --continue naturals --title naturals.json "format as json"
yai --continue naturals --title naturals.yaml "format as yaml"
```

## Delete

Delete is permanent.

```bash
yai --delete <title-or-id>
yai --delete-older-than 10d
```

Equivalent subcommands:

```bash
yai history delete <title-or-id>
yai history prune --older-than 10d
```

## Related docs

- Pipeline behavior and reproducibility: [`docs/integration.md`](integration.md)
