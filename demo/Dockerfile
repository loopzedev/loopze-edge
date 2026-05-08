# syntax=docker/dockerfile:1.7
#
# LOOPZE — Industrial Flow Automation
# Minimal multi-stage build: frontend (Node) → backend (Go) → runtime (Alpine).
#
# IMPORTANT: the build context must be the repo root, not the demo/ directory,
# because the Dockerfile copies go.mod, frontend/, etc. that live one level up.
#
#   # From the repo root:
#   make docker-build
#     # or
#   docker build -f demo/Dockerfile -t loopze-edge:local .
#
#   # From inside demo/ (note the trailing ".."):
#   docker build -f Dockerfile -t loopze-edge:local ..
#
#   # Easiest — compose handles the context for you:
#   cd demo && docker compose up -d --build
#

# ─── Stage 1: Build the Vue 3 frontend ───────────────────────────────────────
FROM node:20-alpine AS frontend
WORKDIR /src/frontend

COPY frontend/package.json frontend/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci

COPY frontend/ ./
# vite emits to ../web/dist (see frontend/vite.config.ts).
RUN npm run build

# ─── Stage 2: Build the Go binary with the embedded frontend ────────────────
FROM golang:1.25-alpine AS backend
RUN apk add --no-cache git
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
# Replace the empty/host web/dist with the freshly built frontend.
COPY --from=frontend /src/web/dist ./web/dist

ARG VERSION=docker
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux \
    go build -trimpath \
        -ldflags "-s -w \
            -X 'github.com/loopzedev/loopze-edge/internal/config.Version=${VERSION}' \
            -X 'github.com/loopzedev/loopze-edge/internal/config.Commit=${COMMIT}' \
            -X 'github.com/loopzedev/loopze-edge/internal/config.BuildTime=${BUILD_TIME}'" \
        -o /out/loopze ./cmd/loopze

# ─── Stage 3: Tiny runtime image ─────────────────────────────────────────────
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata wget \
 && addgroup -S loopze \
 && adduser -S -G loopze -h /data -u 1000 loopze \
 && mkdir -p /data \
 && chown loopze:loopze /data

COPY --from=backend /out/loopze /usr/local/bin/loopze

VOLUME ["/data"]

ENV LOOPZE_HOST=0.0.0.0 \
    LOOPZE_PORT=1880 \
    LOOPZE_DATA_DIR=/data

EXPOSE 1880

USER loopze
WORKDIR /data

HEALTHCHECK --interval=30s --timeout=5s --retries=3 --start-period=15s \
    CMD wget -q -O- http://127.0.0.1:1880/health || exit 1

ENTRYPOINT ["/usr/local/bin/loopze"]
