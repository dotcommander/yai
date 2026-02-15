## Purpose

Make a change to yai and verify it locally.

## Build and test

```bash
go build ./...
go test ./...
```

Lint (if you have it installed):

```bash
golangci-lint run
```

## Where to look

- CLI entry and flags: `main.go`
- Bubble Tea model/state machine: `yai.go`
- Provider bridge (Fantasy routing): `internal/fantasybridge/fantasybridge.go`
- Shared request/response types: `internal/proto/proto.go`

## Related docs

- Provider routing: [`docs/providers.md`](providers.md)
