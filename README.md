# LOOPZE

**Industrial Flow Automation**

LOOPZE is a lightweight, high-performance flow automation platform built for industrial environments. Think Node-RED, but written in Go — compiled to a single binary with zero dependencies.

Designed for automation technicians, PLC programmers, and engineers who think in signal flows and function blocks. LOOPZE brings visual flow-based programming to the edge — with first-class support for industrial protocols like MQTT, Modbus, and OPC-UA.

## Features

- **Single Binary** — No runtime dependencies, no package managers. Just download and run.
- **Visual Flow Editor** — Drag-and-drop node editor built with Vue 3 and Vue Flow.
- **High Performance** — Go-powered runtime with goroutine-per-node concurrency.
- **Industrial First** — MQTT, Modbus TCP/RTU, OPC UA (Read / Subscribe / Write with structure-aware ExtensionObject support and address-space browser), and Serial support out of the box.
- **Dual Scripting** — JavaScript (via Goja) for complex logic, expr-lang for fast expressions.
- **Embedded NATS** — Built-in message broker for context storage, debug streams, and fleet communication.
- **Cross-Platform** — Linux (x64/ARM64/ARM32), Windows (x64), macOS (x64/ARM64).
- **Encrypted Credentials** — Secrets stored separately with AES-256-GCM encryption.
- **Local User Accounts** — First-run admin setup, Argon2id password hashing, three roles (admin/editor/viewer).

## Quick Start

### Prerequisites

- Go 1.24 or later
- Node.js 20+ and pnpm (for frontend development only)

### Build & Run

```
# Clone the repository
git clone https://github.com/loopzedev/loopze-edge.git
cd loopze

# Build everything (frontend + backend)
make build-all

# Or just the backend (uses embedded frontend placeholder)
make build

# Run LOOPZE
./bin/loopze --port 1880 --data-dir ./data

# Open your browser
# http://localhost:1880
# On first launch you will be prompted to create the initial admin
# account — the editor stays locked until that is done.
```

### Command-Line Options

| Flag                       | Default            | Description                                                  |
|----------------------------|--------------------|--------------------------------------------------------------|
| `--host`                   | `0.0.0.0`          | Host address to bind to                                      |
| `--port`                   | `1880`             | HTTP port for the web interface                              |
| `--data-dir`               | `./data`           | Directory for flows and data                                 |
| `--users-file`             | `users.json`       | User records file (in `--data-dir`)                          |
| `--session-key-file`       | `loopze.session.key`| HMAC signing key for session cookies (auto-generated)        |
| `--session-ttl`            | `12h`              | Sliding-window session lifetime                              |
| `--auth-insecure-cookies`  | `false`            | Drop `Secure` flag on cookies — only use over plain HTTP/dev |
| `--auth-disable`           | `false`            | Skip authentication entirely (development only)              |

All flags are mirrored as `LOOPZE_*` environment variables (e.g. `LOOPZE_AUTH_DISABLE=1`).

### Development

```
# Run in development mode
make dev

# Run tests
make test

# Run linter
make lint

# Cross-compile for all platforms
make cross-compile
```

## Architecture

LOOPZE follows a clean, modular architecture:

```
loopze/
├── cmd/loopze/          # Application entry point
├── internal/
│   ├── api/            # REST API handlers and routes
│   ├── auth/           # User identity, sessions, role-based middleware
│   ├── config/         # Configuration management
│   ├── credentials/    # Encrypted credential storage
│   ├── flow/           # Flow runtime engine, types, and node registry
│   ├── server/         # HTTP server setup and middleware
│   ├── storage/        # Flow, credential, and user file persistence
│   └── ws/             # WebSocket hub for real-time communication
├── web/                # Embedded Vue 3 frontend (go:embed)
├── go.mod
├── Makefile
└── DECISIONS.md        # Project decisions and architecture log
```

## Project Decisions

All architectural decisions, technology choices, and design rationale are documented in [DECISIONS.md](DECISIONS.md).

## License

LOOPZE is licensed under the [GNU Affero General Public License v3.0 or later (AGPL-3.0-or-later)](LICENSE).

- ✅ Free to use, study, modify, and redistribute
- ✅ Free for commercial and private internal use
- ⚠️ Modifications and derivative works **must** be released under AGPL-3.0-or-later
- ⚠️ If you run a modified version on a server and let users interact with it over a network,
     you **must** offer those users the corresponding source code (AGPL § 13)

A copy of the license is included in [LICENSE](LICENSE); the copyright and
source-availability notice is in [NOTICE](NOTICE).

For use cases that are incompatible with AGPL (e.g. embedding LOOPZE into a
proprietary product), a separate commercial license can be negotiated with the
copyright holder.

**Copyright © 2026 Dennis Bleul**

---

*LOOPZE — Small, hard, reliable. Like the stone that sparks the fire.*