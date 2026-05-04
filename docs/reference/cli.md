# Command-line reference

```
loopze [flags]
```

## Flags

| Flag                  | Default               | Description                                                  |
|-----------------------|-----------------------|--------------------------------------------------------------|
| `--host`              | `0.0.0.0`             | Host address to bind to.                                     |
| `--port`              | `1880`                | HTTP port for the web interface.                             |
| `--data-dir`          | `./data`              | Directory for flows, credentials and user records.           |
| `--users-file`        | `users.json`          | User records file (relative to `--data-dir`).                |
| `--session-key-file`  | `loopze.session.key`  | HMAC signing key for session cookies (auto-generated).       |
| `--session-ttl`       | `12h`                 | Sliding-window session lifetime.                             |
| `--auth-disable`      | `false`               | Skip authentication entirely. **Development only.**          |
| `--version`           | —                     | Print version info and exit.                                 |

All flags are mirrored as `LOOPZE_*` environment variables — for example
`LOOPZE_AUTH_DISABLE=1` is equivalent to `--auth-disable`.

## Examples

Run on a non-default port with a dedicated data directory:

```bash
loopze --port 8080 --data-dir /var/lib/loopze
```

Run via environment variables (handy for systemd / Docker):

```bash
LOOPZE_PORT=8080 LOOPZE_DATA_DIR=/var/lib/loopze loopze
```

Disable auth for local development:

```bash
loopze --auth-disable
```

!!! warning
    `--auth-disable` exposes the editor and API to anyone who can reach
    the bound port. Never enable this in production.
