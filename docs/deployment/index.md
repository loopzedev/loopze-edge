# Deployment

LOOPZE is one statically linked binary. Deployment is mostly *getting it
to start on boot* and *keeping its data directory safe*.

!!! note "TODO"
    Per-platform guides (systemd, Windows service, Docker, edge appliance
    images) to be filled in.

## Data layout

```
/var/lib/loopze/                  # --data-dir
├── flows.json                    # the flow graph
├── credentials.json              # encrypted secrets
├── users.json                    # local users (Argon2id hashes)
├── loopze.session.key            # session-cookie HMAC key
└── *.key                         # credential encryption key — keep safe
```

Treat the data directory as the source of truth: backing it up backs up
the whole instance.

## Linux (systemd)

Sketch — full unit file to follow:

```ini
[Unit]
Description=LOOPZE
After=network-online.target

[Service]
ExecStart=/usr/local/bin/loopze --data-dir /var/lib/loopze --port 1880
Restart=on-failure
User=loopze
Group=loopze

[Install]
WantedBy=multi-user.target
```

## Windows

Run as a service via [NSSM](https://nssm.cc/) or
[winsw](https://github.com/winsw/winsw) until LOOPZE ships a native
service installer.

## Docker

A multi-stage `Dockerfile` and a `docker-compose.yml` live in the
repo's `demo/` directory.

```bash
cd demo
docker compose up -d
# → http://localhost:1880, state under ./data
```

See the dedicated [Docker guide](docker.md) for image layout, build
args, multi-arch builds, healthcheck and production notes.

## Reverse proxy / TLS

LOOPZE serves plain HTTP on its bound port. For production, terminate
TLS in front (Caddy, nginx, Traefik) and forward to the LOOPZE port.
