# Contributing to LOOPZE

Thanks for your interest in contributing.

## Bug reports & feature requests

Open an issue on GitHub. For **security vulnerabilities**, see [SECURITY.md](SECURITY.md) — do **not** use public issues.

When filing a bug, please include:

- LOOPZE version (`./loopze --version` once available; otherwise commit hash)
- OS and architecture (e.g. `Linux x64`, `Windows ARM64`)
- Minimal reproducer (small flow JSON if relevant)
- Expected vs. actual behavior
- Relevant log output

## Pull requests

1. Fork the repo and create a topic branch.
2. Keep PRs focused — one logical change per PR.
3. Ensure `go build ./...`, `go test ./...`, and `go vet ./...` pass.
4. For frontend changes: `pnpm lint` and `pnpm type-check` clean.
5. Add or update tests for behavioral changes.
6. Follow existing code style (`gofmt` for Go, ESLint config for the frontend).
7. Open the PR against `main` with a clear description of *what* and *why*.

Drive-by refactors of unrelated code make review harder — please split them out.

## Licensing of contributions

LOOPZE is released under the [GNU Affero General Public License v3.0 or later (AGPL-3.0-or-later)](LICENSE).

**By submitting a contribution to this project, you agree that:**

1. **Your contribution is licensed under AGPL-3.0-or-later**, the same license as the rest of the project.
2. **You grant the project maintainer (Dennis Bleul) the right to also distribute your contribution under alternative licenses**, including commercial/proprietary terms, in addition to AGPL-3.0-or-later. This dual-licensing flexibility is necessary so the maintainer can offer LOOPZE to organizations whose use cases are incompatible with AGPL.
3. **You have the legal right to make this contribution** — the work is your own, or you have permission from the rights holder (e.g. your employer) to contribute it.
4. **Your contribution does not knowingly infringe** any third-party intellectual property rights.

If you cannot agree to point (2) — for example, because your employer's IP policy doesn't allow it — please open an issue first so we can discuss alternatives.

This is a lightweight inbound=outbound model and does not require a separately signed CLA.

## Adding a new node

Nodes are the primary extension point of LOOPZE. Each node type lives in two places:

- **Backend (Go):** `internal/nodes/` — implements the node logic
- **Frontend (Vue):** `frontend/src/components/config/` — provides the properties-panel editor

Step-by-step guides:

- [Backend: how to implement and register a Go node](internal/nodes/README.md)
- [Frontend: how to add a config editor component](frontend/src/components/config/README.md)

The short version of the checklist is:

1. Create `internal/nodes/<protocol>_<role>.go` implementing `flow.NodeInstance`
2. Add one line to `registerNodes()` in `internal/server/server.go`
3. Create `frontend/src/components/config/<NodeType>Config.vue`
4. Add one line to `nodeEditors.ts` (or `configEditors.ts` for config nodes)
5. Add a color category entry in `frontend/src/components/nodes/tokens.ts`
6. Write tests (`*_test.go` on the backend, `pnpm type-check` on the frontend)

For protocol nodes (Modbus, S7, OPC UA, …), also read the existing issue specs in
`specifications/issues/NODE_*.md` to understand the design conventions.

## Code of conduct

Be respectful. Disagree on technical merits, not personal attacks. Maintainers reserve the right to moderate or remove off-topic or hostile content from issues and PRs.
