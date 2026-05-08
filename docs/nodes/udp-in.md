# UDP Receive (`udp-in`)

Binds a UDP port and emits one message per received datagram on its
single output. Optionally joins one or more IPv4 multicast groups.
A CIDR allowlist drops senders that don't match.

| Inputs | Outputs |
|--------|---------|
| 0      | 1       |

## Configuration

| Field              | Default     | Description                                                          |
|--------------------|-------------|----------------------------------------------------------------------|
| `host`             | `0.0.0.0`   | Bind address. `0.0.0.0` for all IPv4, `::` for all IPv6.             |
| `port`             | `5000`      | UDP port to bind. `0` lets the OS pick (rare).                       |
| `payloadEncoding`  | `buffer`    | `buffer` (number array), `string`, or `base64`.                      |
| `maxDatagramBytes` | `65507`     | Read buffer. Larger datagrams are truncated and reported.            |
| `multicastGroups`  | `[]`        | List of IPv4 multicast addresses. See [Multicast](#multicast) below. |
| `allowedRemotes`   | `[]`        | CIDR allowlist; mismatched senders are dropped silently.             |

## Multicast

Each entry in `multicastGroups` is either:

- A bare multicast address — `224.0.0.251` — letting the OS pick the
  receiving interface.
- An interface-pinned address — `224.0.0.251%eth0` — joining the
  group only on the named interface.

A failed group join is reported as a catchable error, but the unicast
listener stays up (so a misconfigured multicast address does not
disable the whole node).

IPv4 only in v1. IPv6 multicast is a future addition.

## Outgoing message

```json
{
  "payload": "<bytes — encoded per payloadEncoding>",
  "ip":   "192.0.2.10",
  "port": 49152,
  "remoteAddr": "192.0.2.10:49152",
  "localAddr":  "0.0.0.0:514",
  "size": 137,
  "truncated": false
}
```

`truncated` is `true` when the datagram filled the read buffer
exactly — best-effort indicator (Go's `ReadFromUDP` silently truncates
without telling the caller, so an exact-size datagram produces a
false positive).

## Status

- Green `listening · :514` — bound, no multicast.
- Green `listening · :514 · mcast` — bound and at least one multicast group joined.
- Red `bind: …` — the port is in use or the address is invalid.

## Examples

**Syslog collector.**

```
[udp-in :514] → [Function: parse syslog] → [Switch facility] → [Debug]
```

**mDNS listener.**

```
[udp-in :5353  multicastGroups=[224.0.0.251]] → [Function: parse mDNS] → [Debug]
```

## See also

- [UDP Send](udp-out.md).
- For TLS, use [TCP Receive](tcp-in.md) — DTLS is out of scope.
