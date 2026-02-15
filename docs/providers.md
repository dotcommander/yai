## Purpose

Understand which provider APIs yai can talk to, and how requests are routed.

## Routing model

yai routes LLM requests through `charm.land/fantasy`.

This matters because provider features are expressed via Fantasy provider options, and some settings are provider-specific.

## Provider routing matrix

| Provider API | Fantasy path | Notes |
|---|---|---|
| `openai` | Yes | Native Fantasy OpenAI provider |
| `anthropic` | Yes | Native Fantasy Anthropic provider |
| `google` | Yes | Native Fantasy Google provider; maps `thinking-budget` |
| `azure` | Yes | Native Fantasy Azure provider |
| `azure-ad` | Yes | Aliased through the Fantasy Azure provider |
| `openrouter` | Yes | Native Fantasy OpenRouter provider |
| `vercel` | Yes | Native Fantasy Vercel provider |
| `bedrock` | Yes | Native Fantasy Bedrock provider |
| `cohere` | Yes | Routed via Fantasy OpenAI-compatible provider |
| `ollama` | Yes | Routed via Fantasy OpenAI-compatible provider |
| OpenAI-compatible custom APIs (for example `groq`, `deepseek`) | Yes | Routed via Fantasy OpenAI-compatible provider |

## Known behavior notes

- Stop sequences (`--stop`) are accepted by yai, but are currently not forwarded by the Fantasy Call API. yai prints a one-time warning (unless `--quiet`).
- For OpenAI and OpenAI-compatible providers, yai forwards the configured `user` field via Fantasy provider options when supported.

## Configure credentials

yai reads keys from either the selected API entry in `~/.config/yai/yai.yml` or provider-specific environment variables.

Common env vars:

- `OPENAI_API_KEY`
- `ANTHROPIC_API_KEY`
- `GOOGLE_API_KEY`
- `AZURE_OPENAI_KEY`
- `OPENROUTER_API_KEY`
- `VERCEL_API_KEY`
- `COHERE_API_KEY`

## Related docs

- Setup and first run: [`docs/setup.md`](setup.md)
- Settings details: [`docs/configuration.md`](configuration.md)
