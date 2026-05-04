---
title: LOOPZE Documentation
hide:
  - navigation
---

# LOOPZE

**Industrial Flow Automation**

LOOPZE is a lightweight, high-performance flow automation platform built for
industrial environments. Think Node-RED, but written in Go — compiled to a
single binary with zero dependencies.

Designed for automation technicians, PLC programmers and engineers who think
in signal flows and function blocks. LOOPZE brings visual flow-based
programming to the edge — with first-class support for industrial protocols
like MQTT, Modbus and OPC UA.

---

## Where to start

<div class="grid cards" markdown>

-   :material-rocket-launch:{ .lg .middle } **Getting Started**

    ---

    Install LOOPZE, build your first flow and learn the core concepts.

    [:octicons-arrow-right-24: Installation](getting-started/installation.md)

-   :material-shape-outline:{ .lg .middle } **Nodes**

    ---

    Reference documentation for every built-in node — Inject, Function,
    MQTT, Modbus, OPC UA and more.

    [:octicons-arrow-right-24: Node reference](nodes/index.md)

-   :material-language-javascript:{ .lg .middle } **Scripting**

    ---

    JavaScript via Goja for complex logic, expr-lang for fast expressions.

    [:octicons-arrow-right-24: Scripting guide](scripting/index.md)

-   :material-server:{ .lg .middle } **Deployment**

    ---

    Run LOOPZE on Linux, Windows, macOS, ARM. Systemd, Docker, edge boxes.

    [:octicons-arrow-right-24: Deployment](deployment/index.md)

</div>

---

## Highlights

- **Single binary** — no runtime dependencies, no package managers.
- **Visual flow editor** — drag-and-drop, built with Vue 3 and Vue Flow.
- **Industrial first** — MQTT, Modbus TCP/RTU, OPC UA (read / subscribe /
  write with structure-aware ExtensionObject support and address-space
  browser), serial.
- **Embedded NATS** — built-in message broker for context, debug streams
  and fleet communication.
- **Encrypted credentials** — secrets stored separately, AES-256-GCM.
- **Local user accounts** — Argon2id password hashing, three roles
  (admin / editor / viewer).

---

*LOOPZE — Small, hard, reliable. Like the stone that sparks the fire.*
