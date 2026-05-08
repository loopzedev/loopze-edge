# Docker

LOOPZE ships with a multi-stage `Dockerfile` and a `docker-compose.yml`
under `demo/`. The Dockerfile builds the Vue 3 frontend, embeds it into
the Go binary, and produces a minimal Alpine-based runtime image.

## Quickstart

From the repo root:

```bash
make docker-up                      # via Makefile, simplest
# — or equivalently:
cd demo && docker compose up -d     # compose handles the build context
```

The first build takes ~1–2 minutes (frontend + Go binary). Subsequent
builds reuse Docker's BuildKit cache and complete in seconds.

!!! warning "Build context must be the repo root"
    The `Dockerfile` lives in `demo/` but copies files that live *one
    level up* (`go.mod`, `frontend/`, `internal/`, …). A bare
    `docker build .` from inside `demo/` will fail with `go.sum: not
    found`. Use `make docker-build`, or pass the parent directory
    explicitly: `docker build -f Dockerfile -t loopze-edge:local ..`.

Open [http://localhost:1880](http://localhost:1880) — on first launch
you'll be prompted to create the initial admin account. State persists
in `demo/data/` on the host.

```bash
docker compose logs -f loopze   # follow logs
docker compose down             # stop (state in ./data is kept)
docker compose down -v          # stop AND wipe state
```

## Image layout

The `Dockerfile` uses three stages:

| Stage      | Base image           | Purpose                                                                       |
|------------|----------------------|-------------------------------------------------------------------------------|
| `frontend` | `node:20-alpine`     | Build the Vue 3 editor (`vite build`).                                        |
| `backend`  | `golang:1.25-alpine` | Compile the Go binary, embedding the frontend via `go:embed`.                 |
| *runtime*  | `alpine:3.20`        | Minimal runtime — `ca-certificates`, `tzdata`, `wget` for the healthcheck.    |

The final image is around **90 MB**. It's not smaller because the
frontend ships with the full Monaco editor (~4 MB pre-gzip) which is
embedded into the binary.

The container runs as the unprivileged user `loopze` (UID/GID 1000).

## Configuration

Every command-line flag is mirrored as a `LOOPZE_*` environment
variable — set them in `docker-compose.yml`:

```yaml
services:
  loopze:
    environment:
      LOOPZE_PORT: 1880
      LOOPZE_LOG_LEVEL: info
      LOOPZE_AUTH_DISABLE: "false"
      # LOOPZE_BASE_PATH: /loopze
      # LOOPZE_TRUSTED_PROXIES: 10.0.0.0/8,172.16.0.0/12
```

See the [CLI reference](../reference/cli.md) for the full list.

## Persistence

The image presets `LOOPZE_DATA_DIR=/data` and declares `/data` as a
volume. The compose file bind-mounts `./data` on the host:

```yaml
volumes:
  - ./data:/data
```

Inside the volume:

```
data/
├── flows.json            # the flow graph
├── credentials.json      # AES-256-GCM-encrypted secrets
├── users.json            # local user accounts (Argon2id hashes)
├── loopze.session.key    # session-cookie HMAC key
└── *.key                 # credential encryption key
```

!!! danger "Backup the whole data directory"
    Without `*.key` you cannot decrypt `credentials.json`. Back up the
    directory as a unit — partial backups can leave you with encrypted
    secrets you can't read.

If you bind-mount from the host, ensure ownership permits the container
UID/GID (`chown -R 1000:1000 ./data` works in most cases).

## Healthcheck

The image runs a periodic check against `/health`:

```dockerfile
HEALTHCHECK --interval=30s --timeout=5s --retries=3 --start-period=15s \
    CMD wget -q -O- http://127.0.0.1:1880/health || exit 1
```

`docker ps` shows `(healthy)` in the `STATUS` column once the check
passes.

## Building tagged images

The build accepts `VERSION`, `COMMIT` and `BUILD_TIME` as build-args.
For a tagged release:

```bash
docker build \
  --build-arg VERSION=v0.5.1 \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  --build-arg BUILD_TIME=$(date -u '+%Y-%m-%dT%H:%M:%SZ') \
  -f demo/Dockerfile -t loopze-edge:0.5.1 .
```

The values appear in `loopze --version` output and in the startup banner.

## Multi-arch builds

For Raspberry Pi and other ARM targets:

```bash
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -f demo/Dockerfile \
  -t myregistry/loopze:0.5.1 \
  --push .
```

## Production notes

- **TLS** — terminate TLS in front (Caddy / nginx / Traefik). LOOPZE
  serves plain HTTP and is not intended to face the public internet
  directly.
- **Reverse proxy under a subpath** — set `LOOPZE_BASE_PATH=/loopze`
  and point your proxy at `http://loopze:1880/`. The frontend's
  `<base>` tag is rewritten at request time, so no rebuild is needed.
- **Trusted proxies** — set `LOOPZE_TRUSTED_PROXIES` (CIDRs) so the
  real client IP is honored from `X-Forwarded-For`.
- **Pin image versions** — for production, pull from your registry by
  tag rather than rebuilding from source on every deploy.
