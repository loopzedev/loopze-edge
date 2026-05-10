# Contributing

LOOPZE is open source ([AGPL-3.0-or-later](https://github.com/loopzedev/loopze-edge/blob/main/LICENSE))
and welcomes contributions — bug reports, feature requests, documentation
improvements, and code.

## Quick links

- [Adding a node](adding-a-node.md) — end-to-end walkthrough for shipping a
  new node group (backend + frontend) into the codebase.
- [CONTRIBUTING.md](https://github.com/loopzedev/loopze-edge/blob/main/CONTRIBUTING.md) — PR
  conventions, commit style, licensing of contributions.
- [Architecture reference](../reference/architecture.md) — how the runtime,
  engine and node groups fit together.

## Where to start

For a small fix (typo, regex, log message): just open a PR.

For a new feature: open an issue first to align on scope, especially when
it touches the engine or persistence. The maintainer will tag it
*good first issue* if it's a scoped, low-risk task.

For a new **node**: read [Adding a node](adding-a-node.md) — that document
covers the full surface from backend skeleton to frontend manifest.

## Development environment

```bash
# Backend (Go 1.23+)
make dev          # air-reloaded backend + Vite dev server

# Tests
make test         # go test ./... -race
make lint         # golangci-lint

# Cross-arch release build
make build-snapshot
```

The frontend lives in `frontend/` and is built into `web/dist/`, which is
embedded into the Go binary via `go:embed`. There is no separate frontend
deployment.
