# UDP Send (`udp-out`)

Sends `msg.payload` as a UDP datagram. Three modes share the same
write path; the differences are setsockopt-level — `SO_BROADCAST`
for broadcast and TTL / loopback / outbound interface for multicast.

| Inputs | Outputs |
|--------|---------|
| 1      | 0       |

## Modes

| Mode        | Behaviour                                                                                  |
|-------------|--------------------------------------------------------------------------------------------|
| `unicast`   | Send to a single host:port.                                                                |
| `broadcast` | Set `SO_BROADCAST` on the socket; send to a broadcast address (`255.255.255.255` or directed). |
| `multicast` | IPv4 multicast send with configurable TTL, loopback, and outbound interface.               |

## Configuration

| Field              | Default | Description                                                          |
|--------------------|---------|----------------------------------------------------------------------|
| `mode`             | `unicast` | `unicast`, `broadcast`, or `multicast`.                            |
| `host`             | —       | Destination host. `msg.host` overrides.                              |
| `port`             | —       | Destination port. `msg.port` overrides.                              |
| `bindHost`         | —       | Multicast only: name of the outbound interface (e.g. `eth0`).        |
| `multicastTTL`     | `1`     | Multicast only. `1` = LAN-local. Range 0–255.                        |
| `multicastLoopback`| `true`  | Multicast only. Loopback to the sending host.                        |
| `reuseSocket`      | `true`  | Keep one outbound socket across messages.                            |

## Incoming message

| Field    | Description                                                |
|----------|------------------------------------------------------------|
| `payload`| Body to send. `string`, `[]byte`, `[]int` (number array), or any other type (`fmt.Sprint`-rendered). Empty is allowed (zero-length datagram is legal in UDP). |
| `host`   | Per-message destination override.                          |
| `port`   | Per-message destination override.                          |

## Status

- Green `sent · 137B → host:port` after each successful send.
- Yellow `send: …` on transient errors (DNS, kernel buffer full).
- Red on configuration errors.

## Notes

- **Broadcast:** the node sets `SO_BROADCAST` on the socket so the
  kernel allows writes to broadcast addresses. The default unicast
  socket would silently fail.
- **Multicast TTL:** `1` keeps the packet on the local link. Bump
  it to traverse routed subnets — but coordinate with your network
  team because most routers drop multicast.
- **`reuseSocket=false`** opens a fresh socket for every send. Rare;
  useful only when source-port randomisation is required per packet.

## Examples

**Send a syslog event.**

```
[Inject every 1s] → [Function format syslog] → [udp-out 192.0.2.5:514]
```

**Multicast discovery beacon.**

```
[Inject every 30s] → [Change payload=…] → [udp-out 224.0.0.251:5353  mode=multicast TTL=1]
```

**Broadcast a heartbeat to the local subnet.**

```
[Inject every 5s] → [udp-out 255.255.255.255:9999  mode=broadcast]
```

## See also

- [UDP Receive](udp-in.md).
