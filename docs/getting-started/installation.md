# Installation

LOOPZE ships as a single, statically linked binary. No runtime, no package
manager, no service dependencies.

## Download a release

Pre-built binaries for every supported platform are available on the
[GitHub releases page](https://github.com/loopzedev/loopze-edge/releases).

Supported platforms:

- Linux (x64, ARM64, ARM v7, ARM v6)
- Windows (x64)
- macOS (x64, ARM64)

## Build from source

### Prerequisites

- Go 1.24 or later
- Node.js 20+ and pnpm — only required when modifying the embedded frontend

### Build

```bash
git clone https://github.com/loopzedev/loopze-edge.git
cd loopze-edge

# Build everything (frontend + backend)
make build-all

# Or just the backend (uses embedded frontend placeholder)
make build
```

The resulting binary lands in `./bin/loopze`.

## First run

```bash
./bin/loopze --port 1880 --data-dir ./data
```

Open `http://localhost:1880` in a browser. On first launch you will be
prompted to create the initial admin account — the editor stays locked
until that is done.

!!! tip "Next step"
    Continue with the [Quickstart](quickstart.md) to build your first flow.
