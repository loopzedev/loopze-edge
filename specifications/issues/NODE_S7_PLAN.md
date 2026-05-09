# Plan: SIEMENS S7 Nodes

Implementation plan for [`NODE_S7.md`](./NODE_S7.md) and [`PARSER_S7_NODE.md`](./PARSER_S7_NODE.md). Three new node types: `s7-plc` (config), `s7-read`, `s7-write`, plus `s7-parser` from the companion spec. Modelled on the Modbus stack — `s7-plc` is the analog of `modbus-server`, `s7-read/write` mirror `modbus-read/write`, `s7-parser` mirrors `modbus-parser`.

## Locked Decisions

These resolve the open architecture questions implied by the two specs. Anything below this line is the contract for the implementation:

1. **gos7 area / WordLen constants are unexported.** We declare our own copy in `s7_address.go`:
   ```go
   const (
       s7AreaPE = 0x81; s7AreaPA = 0x82; s7AreaMK = 0x83; s7AreaDB = 0x84
       s7AreaCT = 0x1C; s7AreaTM = 0x1D
       s7WLBit = 0x01; s7WLByte = 0x02; s7WLChar = 0x03; s7WLWord = 0x04
       s7WLInt = 0x05; s7WLDWord = 0x06; s7WLDInt = 0x07; s7WLReal = 0x08
       s7WLCounter = 0x1C; s7WLTimer = 0x1D
   )
   ```
   These mirror gos7's internal values 1:1 and are stable on the wire (S7 protocol, never changes).

2. **LOGO! mode in v1: rack/slot via ConnectType only**, no free-form local/remote TSAP. gos7 only exposes TSAP indirectly via `NewTCPClientHandlerWithConnectType(addr, rack, slot, connectType)`. The spec's `localTsap` / `remoteTsap` fields are deferred; the LOGO! UI offers a `connectType` enum (PG=1, OP=2, S7Basic=3) plus rack/slot. Free-form TSAP needs an upstream gos7 patch and is tracked as a v1.x follow-up.

3. **Negotiated PDU size**: read post-connect from `(*TCPClientHandler).PDULength`. On reconnect we re-read it. Block-mode auto-split uses `max(len) = pdu - 22` for `AGReadArea`/`AGWriteArea`.

4. **Per-PLC serialization**: single mutex on the `S7PLC` instance wrapping every wire call (`AGReadDB/MB/EB/AB`, `AGReadMulti`, `AGWriteMulti`). S7 is half-duplex per connection; this matches the modbus-server pattern.

5. **Variables-mode bundling**: a single `s7-read` static-mode tick converts its variable list to `[]S7DataItem`, splits into PDU-sized chunks (item header ≈ 12 B + value bytes), calls `AGReadMulti` per chunk, then concatenates. The auto-coalesce-into-block optimization is explicitly Phase 3 (per spec) — out of scope.

6. **Block-mode dispatch**: `AGReadDB` for area=DB, `AGReadMB/EB/AB` for M/I/Q. Split happens in the manager when `length > pdu - 22`, multiple sequential calls, results concatenated.

7. **Status text** matches modbus phrasing — `connected · 1s` / `connecting...` / `reconnecting...` / `<error>`.

8. **Error semantics**: gos7 returns ints from `CPUError`/`ErrorText`. We wrap with our own `s7ErrorText(uint32) string` that maps the table from `NODE_S7.md` ("Item not available", "Address out of range", …). Per-item errors in `AGReadMulti` come back via `S7DataItem.Error string`; the read node folds them into the per-variable result.

9. **`s7-parser` codec reuse**: parser depends on `s7_codec.go` from PR-2 (`DecodeS7Scalar`, `EncodeS7Scalar`, `DecodeS7String`, `EncodeS7String`). The parser PR cannot land before that codec exists.

10. **`ApplyScale` reuse**: existing `internal/nodes/modbus_codec.go` `ApplyScale` / `UnapplyScale` are package-level and generic — reuse as-is (they live in `package nodes`, same package as S7 nodes). No new helper needed.

11. **Frontend palette**: S7 reuses the existing `rust` palette (modbus already uses it). If a Siemens-distinct color is wanted later, add an `s7` palette entry — not blocking.

12. **`TlsConfigSection` reuse**: not used. S7 over RFC1006 is unencrypted in v1 (per spec). The cert-store integration plug-in remains a v2 concern; nothing to do.

13. **Test PLC env var**: `LOOPZE_S7_TEST_HOST` (default skip) + `LOOPZE_S7_TEST_PORT` (default `1102`). Mirrors the OPC UA pattern; tests `t.Skip()` when unset.

14. **Demo PLC gap — DB2 ≥ 600 B**: needed for block-mode auto-split testing. The current demo has DB1 (200 B), DB10 (50 B). We extend the demo with a `DB2` (600 B) populated with a deterministic byte ramp (`buf[i] = i & 0xFF`) so split tests can verify byte-perfect concatenation. This extension lands as part of PR-6 (or its own pre-PR).

## Critical-Path Modules

Three modules whose design choices ripple everywhere — they need extra care up front so later PRs don't churn:

### `internal/nodes/s7_address.go` — the address parser
- Pure function `ParseS7Address(s, dataType string) (S7Item, error)` returning a populated item struct convertible to `gos7.S7DataItem`.
- Drives the read, write, and any future browse/discovery surface. Wrong choice here means re-doing every test in PR-3..PR-5.
- **Design lock**: parser is **strict by form-type contract** — `dataType` and address syntax must agree (e.g. `M0.0` requires `bool`, `MD0` rejects `bool`). The frontend's `S7AddressInput.vue` validates the same regex set so users see errors at typing time.
- **Test fixture**: 25-row table-driven test covering every spec form, plus negative tests (symbolic `DB1.MyVar`, out-of-range bits, missing DB number, …).

### `internal/nodes/s7_codec.go` — encode/decode primitives
- One function per S7 type, value-based: `DecodeS7Scalar(typ string, signed bool, b []byte) (any, error)`, `EncodeS7Scalar(typ string, signed bool, v any) ([]byte, error)`, `DecodeS7String(b []byte) (string, error)`, `EncodeS7String(maxLen int, s string) ([]byte, error)`.
- **Shared between `s7-read`, `s7-write`, AND `s7-parser`** — this is the chokepoint. Sloppy signatures here force every consumer to recompute.
- Builds on the `gos7.Helper` getters/setters (`GetRealAt`, `SetStringAt`, etc.) but wraps them so the parser can operate on slices without preallocating a `Helper{}`.
- BCD decode (counters) and S5 time decode (timers) come from `gos7.Helper.GetCounter` / `GetS5TimeAt`.
- **Test fixture**: round-trip property tests per type; explicit S7 STRING `[maxLen][actLen][chars…]` round-trip.

### `internal/nodes/s7_plc.go` — the per-PLC manager
- Owns the gos7 `Client` + `TCPClientHandler` + reconnect goroutine + serialization mutex.
- Public surface intentionally small and stable:
  ```go
  Read(ctx context.Context, items []gos7.S7DataItem) error              // multi-read with PDU split
  ReadArea(area, db, start, length int) ([]byte, error)                  // block read with PDU split
  Write(ctx context.Context, items []gos7.S7DataItem) error              // multi-write with PDU split
  WriteArea(area, db, start int, data []byte) error                      // block write with PDU split
  PDUSize() int                                                          // post-connect, 0 before
  Name() string
  RegisterStatusFunc(flow.StatusFunc)
  ```
- All four wire methods take the mutex so concurrent callers from multiple read/write nodes serialise. Mirrors `ModbusServer` — keep the names different (`Read`/`ReadArea` vs modbus's `Read`) so a future "industrial server" interface can union them cleanly without conflicts.
- **Test fixture**: TestS7PlcSerialization with two parallel goroutines hammering the same PLC and asserting no interleaving via a sentinel.

These three are the "do them carefully or pay later" set. PR-1 and PR-2 lock their signatures.

## Order

Backend first; smallest verifiable slice first. Each PR compiles, tests pass, and `make demo-s7` exercises the surface end-to-end (where applicable).

```
[PR-1] s7_address.go + tests           (no runtime deps)
   ↓
[PR-2] s7_codec.go + tests             (no runtime deps; codec primitives only)
   ↓                                    └─→ unblocks PR-7 (parser) in parallel
[PR-3] s7_plc.go (manager + lifecycle) + tests against demo PLC
   ↓
[PR-4] s7_read.go (static + dynamic) + tests
       ── parallel ──
[PR-5] s7_write.go (static + dynamic) + tests
   ↓
[PR-6] s7-read/write block mode + DB2 demo extension + tests
   ↓
[PR-7] s7-parser node (depends on PR-2 codec) + tests          ← can start after PR-2 lands
       ── parallel with PR-4/5/6 once PR-2 is in main ──
   ↓
[PR-8] Backend wiring + Test-Connection REST endpoint + handler tests
   ↓
[PR-9] Frontend: S7PlcConfig.vue + S7NodeConfig.vue + tokens/dispatch + types
   ↓
[PR-10] Frontend: S7ParserConfig.vue + S7ParserLayoutEditor.vue
```

**Parallelism summary**:
- PR-1 and PR-2 are independent — can run in parallel.
- PR-4 and PR-5 are independent once PR-3 ships (read node and write node touch separate files, share only the manager).
- **PR-7 (parser) blocks on PR-2 only.** It does NOT need PR-3..PR-6. The parser sits between two `s7-read/write` block-mode nodes but only operates on `[]byte` — no manager dependency. Implementer can start PR-7 the moment PR-2 merges; it lands in parallel with the read/write track.
- PR-9 and PR-10 are independent on the frontend side once PR-8 has registered the type IDs.

**Critical path** (slowest to "shippable end-to-end"): PR-1 → PR-2 → PR-3 → PR-4 → PR-6 → PR-8 → PR-9. ~7 sequential steps. Parser (PR-7, PR-10) is a side-track joining at the end.

---

## PR-1 — Address Parser

**Scope**: pure-function S7 address parser, no I/O, no engine wiring.

**New files**:
- `internal/nodes/s7_address.go`
- `internal/nodes/s7_address_test.go`

**Public surface**:
```go
type S7Item struct {  // mirrors gos7.S7DataItem but ours, stable
    Area, WordLen, DBNumber, Start, Bit, Amount int
}
func ParseS7Address(addr, dataType string) (S7Item, error)
func (i S7Item) ToGos7() gos7.S7DataItem
```

**Test strategy**: table-driven, exhaustive over the spec's address form table. ≥25 positive cases, ≥10 negatives (symbolic, out-of-range, type mismatch).

**Unblocks**: PR-3 (manager needs to wire variables→items), PR-9 (frontend regex validator should mirror these exactly — the regex set is documented as comments in `s7_address.go`).

**Done-criteria**: `go test ./internal/nodes -run TestS7Address` green. Shippable but not user-visible.

---

## PR-2 — Codec Primitives

**Scope**: type encode/decode, scale/offset (reuse), S7 STRING handling.

**New files**:
- `internal/nodes/s7_codec.go`
- `internal/nodes/s7_codec_test.go`

**Public surface**:
```go
func DecodeS7Scalar(typ string, signed bool, b []byte) (any, error)
func EncodeS7Scalar(typ string, signed bool, v any) ([]byte, error)
func DecodeS7String(b []byte) (string, error)
func EncodeS7String(maxLen int, s string) ([]byte, error)
func S7TypeWordLen(typ string) int  // returns s7WL* for use by ParseS7Address
func S7TypeByteSize(typ string, length int) int  // for parser block sizing
```

Reuses `ApplyScale` / `UnapplyScale` from `modbus_codec.go` — no duplication.

**Go dependency added**: `github.com/robinson/gos7` (use `gos7.Helper{}` for the BCD/REAL/STRING primitives so we don't reinvent IEEE-754 swapping).

**Test strategy**: per-type round-trip (`Encode→Decode == identity`); explicit byte patterns for REAL (sample IEEE-754), STRING (`[20][14]LOOPZE-S7-DEMO\0…`); scale/offset edge cases (zero scale rejected, negative offset).

**Unblocks**: PR-3 (read decoding), PR-5 (write encoding), **PR-7 (parser, in parallel with the rest)**.

**Done-criteria**: `go test ./internal/nodes -run TestS7Codec` green. Shippable but not user-visible.

---

## PR-3 — `s7-plc` Config Node

**Scope**: PLC manager with connect/reconnect, mutex-serialized wire calls, lifecycle, status broadcast.

**New files**:
- `internal/nodes/s7_plc.go`
- `internal/nodes/s7_plc_test.go`

**Backend changes**:
- `internal/server/server.go` — register `s7-plc` config factory in `registerNodes()`. (One line: `registry.RegisterConfig("s7-plc", nodes.NewS7PLC, nodes.S7PLCConfigTypeInfo())`.)

**Behaviour**:
- Implements `flow.ConfigInstance` exactly like `ModbusServer` — same `Start`/`Stop`/`Status`/`RegisterStatusFunc`/`Name` surface, same `setStatus`/`setStatusLocked` pattern.
- `Start()` opens the connection eagerly, captures `handler.PDULength`. Failure logs WARN and sets red status — does NOT block deploy (matches modbus). This means the engine's existing config-node lifecycle handles errors gracefully — no engine change needed.
- Background reconnect goroutine wakes when a wire call returns a transport error; sleeps `reconnectBackoff` between attempts; re-runs COMM SETUP and re-reads `PDULength` each time.
- `connection` enum: `s7-1200-1500` → ConnectType=1 (PG); `s7-300-400` → ConnectType=1; `logo` → ConnectType=3 (S7Basic) — see decision #2.

**Test strategy**:
- Unit: PDU split math (`splitItemsForPDU(items, pdu) [][]S7DataItem`), `s7ErrorText` mapping table.
- Integration (`-tags=s7integration` or via env-var skip): `TestS7PlcConnect_S71500`, `TestS7PlcReconnect`, `TestS7PlcMultiRead`, `TestS7PlcSerialization`. All gated on `LOOPZE_S7_TEST_HOST` (default skip).
- CI runs the demo PLC via `make demo-s7` in a pre-test step (or via Docker compose `demo/docker-compose.yml`).

**Unblocks**: PR-4, PR-5, PR-8.

**Done-criteria**: `go test ./internal/nodes -run TestS7Plc` green with `LOOPZE_S7_TEST_HOST=127.0.0.1 LOOPZE_S7_TEST_PORT=1102` against `make demo-s7`. Config node is registered but no canvas nodes use it yet — not user-visible.

---

## PR-4 — `s7-read` Node (variables modes)

**Scope**: static + dynamic modes only. Block mode is PR-6.

**New files**:
- `internal/nodes/s7_read.go`
- `internal/nodes/s7_read_test.go`

**Backend changes**:
- `internal/server/server.go` — register `s7-read` in `registerNodes()`.

**Behaviour**:
- `mode=static`: ticker poll loop, identical shape to `ModbusReadNode.pollLoop()`. Re-uses `resolveConfigInstance[S7PLC]` from `config_lookup.go`.
- `mode=dynamic`: `HandleMessage` triggers; `msg.variables[]` overrides the configured list; convenience `msg.address`/`msg.dataType` for single reads.
- `outputShape`: scalar/array/object. Defaults to `object` when >1 variable.
- `topicTemplate` substitution.
- Status mirrors modbus (green `connected · <interval>`, etc.).
- Per-variable errors in multi-read responses surface as `s7.variables[i].error` (string).

**Test strategy**: TestS7ReadStatic, TestS7ReadDynamic, TestS7ReadEmitOnChange, TestS7ReadOptimizedDB (uses a non-existent DB number to force `Item not available`), TestS7ReadOutputShapes. All require `LOOPZE_S7_TEST_HOST`.

**Unblocks**: nothing strict; the read node is user-visible without write.

**Done-criteria**: `go test ./internal/nodes -run TestS7Read` green; manual: drag s7-read onto canvas in dev server, point at `localhost:1102`, see DB10.DBD0 sine wave in Debug — shippable end-to-end on the read side once PR-9 (frontend) lands.

---

## PR-5 — `s7-write` Node (variables modes)

**Scope**: static + dynamic. Block mode is PR-6. Independent of PR-4.

**New files**:
- `internal/nodes/s7_write.go`
- `internal/nodes/s7_write_test.go`

**Backend changes**: register `s7-write` in `registerNodes()`.

**Behaviour**:
- `valueSource: static|msg`, `valuePath` resolution via `flow.Message.Get(path)`.
- Type coercion table from spec — uses `EncodeS7Scalar` from PR-2; out-of-range produces `BadOutOfRange`; type mismatch produces `BadTypeMismatch` with field path. **No `flow.Message` payload mutation** — output is either silent (default), ACK-only (`emitAck`), or pass-through (`passthrough`).
- Inverse scaling via `UnapplyScale` (already exists).

**Test strategy**: TestS7WriteStatic, TestS7WriteDynamic, TestS7WriteString (round-trip via demo PLC), TestS7WriteTypeMismatch, TestS7WritePassthrough.

**Done-criteria**: shippable as a vertical slice with PR-3+PR-4+PR-9.

---

## PR-6 — Block Mode (read + write) + Demo PLC Extension

**Scope**: `mode=block` for both read and write nodes; auto-split when block exceeds PDU; new DB2 in the demo for split testing.

**Modified files**:
- `internal/nodes/s7_read.go` — add block-mode branch in `Init`/poll/HandleMessage.
- `internal/nodes/s7_write.go` — add block-mode branch.
- `internal/nodes/s7_plc.go` — `ReadArea(area, db, start, length int) ([]byte, error)` + `WriteArea(area, db, start int, data []byte) error` with auto-split loop.
- `demo/s7-server/main.py` — register **DB2 (600 bytes)** with a deterministic ramp pattern (`buf[i] = i & 0xFF`). Update README address map. This is the **only demo gap** — current largest area (DB1) at 200 B is below the typical 460 B PDU payload limit, so auto-split cannot be exercised against it.

**New tests**:
- TestS7ReadBlockStatic — DB1, length=200, single PDU, byte-perfect.
- TestS7ReadBlockOversized — DB2, length=600, forces ≥2 `AGReadArea` calls; assert concatenation matches the ramp pattern.
- TestS7ReadBlockTriggerOnInput, TestS7ReadBlockDynamicOverride — `msg.s7.{area,db,start,length}` overrides.
- TestS7WriteBlockStatic, TestS7WriteBlockLengthMismatch, TestS7WriteBlockInputProperty.

**Unblocks**: PR-7 has a meaningful end-to-end demo (s7-read block → s7-parser → debug).

**Done-criteria**: `go test ./internal/nodes -run TestS7.*Block` green; manual flow `s7-read block (DB2, 600B)` → `debug` shows a 600-byte ramp.

---

## PR-7 — `s7-parser` Node (parallel with PR-3..PR-6 once PR-2 lands)

**Scope**: declarative byte-block parser/encoder per `PARSER_S7_NODE.md`. Standalone — no PLC dependency, only the codec from PR-2.

**New files**:
- `internal/nodes/s7_parser.go`
- `internal/nodes/s7_parser_test.go`

**Backend changes**: register `s7-parser` in `registerNodes()`.

**Architecture borrowed from `modbus_parser.go`** (recently shipped — read it as a template):
- `s7ParserField` struct mirroring `modbusField`, but with `Byte int` + `Bit int` (BOOL dotted offsets) instead of register/bit.
- `Init()` parses + validates the layout (duplicate names, bit ∈ [0,7], string length ∈ [1,254], blockLength sanity).
- `HandleMessage()` dispatches on `action=auto|parse|encode` exactly like the modbus parser; same `[]byte` / `[]any` / `map[string]any` type switch.
- `parse(buf []byte) (map[string]any, error)` — slice + `DecodeS7Scalar`/`DecodeS7String`/raw, then `ApplyScale`.
- `encode(in map[string]any) ([]byte, error)` — sparse-zero default (matches modbus parser decision), BOOL bits OR-aggregated per byte (analogous to modbus parser's bit pre-pass), `setStartAddress` writes `msg.s7.start`.

**Test strategy**: TestS7Parser_LayoutValidate_*, TestS7Parser_Parse_*, TestS7Parser_Encode_*, TestS7Parser_RoundTrip, TestS7Parser_BoolBitsOR (multiple bools at byte 12 → byte 12 = 0x83 when bits 0/1/7 set), TestS7Parser_StringRoundTrip (S7 STRING `[20][14]LOOPZE-S7-DEMO`).

**Done-criteria**: `go test ./internal/nodes -run TestS7Parser` green. End-to-end demo with PR-6: `[s7-read DB1 block 200] → [s7-parser layout] → [debug]` shows `{temperature, pressure, counter, alarm, running, mode, energy, tag}` decoded values matching the demo's animated state.

---

## PR-8 — Test-Connection REST Endpoint + API Wiring

**Scope**: `POST /api/v1/s7/test-connection`, no persistence.

**New files**:
- `internal/api/s7_handlers.go`
- `internal/api/s7_handlers_test.go`

**Modified files**:
- `internal/api/routes.go` — add `r.Post("/s7/test-connection", deps.handleS7TestConnection)` under the editor-role group (mirror `/opcua/test-connection`).

**Behaviour**: build a transient `S7PLC` from the request payload, `Start()`, call `client.GetCPUInfo()` + `client.GetOrderCode()`, capture `handler.PDULength`, `Stop()`. Returns CPU type, order code, negotiated PDU. **Tolerates the snap7-server placeholder strings** for CPUInfo (per spec — the demo server returns blanks; the handler reports "unknown" for empty strings rather than failing).

**Test strategy**: TestS7TestConnectionHandler against demo PLC (skipped without env var); also a unit test against bad rack/slot returning the expected error string.

**Done-criteria**: `curl` against the endpoint with the demo PLC returns `{ok: true, info: {cpuType, negotiatedPduSize, orderCode}}`; bad rack returns `{ok: false, error: "..."}`.

---

## PR-9 — Frontend: PLC Config + Read/Write Node Properties

**Scope**: dialog for `s7-plc` config; shared properties panel for read/write; address validator; type dropdown; tokens registration.

**New files**:
- `frontend/src/components/config/S7PlcConfig.vue`
- `frontend/src/components/config/S7NodeConfig.vue`
- `frontend/src/components/config/shared/S7AddressInput.vue`
- `frontend/src/components/config/shared/S7DataTypeSelect.vue`

**Modified files**:
- `frontend/src/components/PropertyPanel.vue` — import + dispatch for `s7-read` and `s7-write`.
- `frontend/src/components/config/configEditors.ts` — add `'s7-plc'` line.
- `frontend/src/components/nodes/tokens.ts` — add `'s7-read': 'rust'`, `'s7-write': 'rust'`, `'s7-parser': 'rust'`. (Reuse rust palette per decision #11. If a Siemens-petrol palette is wanted later, swap one line.)
- `frontend/src/types/flow.ts` — TypeScript types for `S7PlcConfig`, `S7ReadConfig`, `S7WriteConfig`, `S7Variable`.
- `frontend/src/views/FlowEditor.vue` — `<template #node-s7-read>`, `#node-s7-write` if needed.
- `frontend/src/components/nodes/BaseNode.vue` — `typeLabel` map entries.

**Address validator**: regex set in `S7AddressInput.vue` mirrors the table in `s7_address.go` (kept in sync via comment cross-reference). Symbolic addresses (`DB1.MyVar`) match a separate regex and trigger the OPC UA hint inline.

**Test-Connection wiring**: button POSTs to `/api/v1/s7/test-connection` (PR-8), shows CPU type / order code / PDU on success; error string on failure.

**Test strategy**: render tests + validation tests for `S7AddressInput`. Validate that the connection-type switch auto-fills rack/slot.

**Done-criteria**: full vertical slice — user can create an S7 PLC config, drag s7-read onto canvas, configure variables list, deploy, see decoded values in Debug panel.

---

## PR-10 — Frontend: Parser Properties Panel

**Scope**: layout-table editor for `s7-parser`. Mirrors `ModbusParserConfig.vue` + `ModbusParserLayoutEditor.vue` 1:1, swapping the type dropdown to S7 types.

**New files**:
- `frontend/src/components/config/S7ParserConfig.vue`
- `frontend/src/components/config/S7ParserLayoutEditor.vue`

**Modified files**: PropertyPanel dispatch, tokens already done in PR-9, BaseNode typeLabel, FlowEditor template. Types in `flow.ts`.

**Done-criteria**: end-to-end flow `[s7-read block (DB1, 200)] → [s7-parser DB1 layout] → [debug]` shows the decoded object live.

---

## Risks & Unknowns

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| **gos7 area / WordLen constants are unexported** | Verified | High | Decision #1: copy values into `s7_address.go`. They're S7 protocol constants — they don't change. |
| **gos7 has no public API for free-form local/remote TSAP** (only ConnectType + rack/slot via `setConnectionParameters`, unexported) | Verified | Medium | Decision #2: drop the spec's `localTsap` / `remoteTsap` fields from v1; expose `connectType` enum (PG=1, OP=2, S7Basic=3) instead. Document in spec follow-up. Free-form TSAP needs an upstream gos7 patch — track as v1.x. |
| **`PDULength` accessor on TCPClientHandler** | Verified — exported as `(*TCPClientHandler).PDULength int` | Low | Cache after `Connect()`, re-read after each reconnect. |
| **gos7's `AGReadMulti` returns per-item errors via `S7DataItem.Error string`** (verified — field is exported) | Low | Read node folds these into `s7.variables[i].error`, doesn't fail the whole transaction. |
| **snap7 demo server returns blanks for `GetCPUInfo` / `GetOrderCode`** | Verified by spec | Low | PR-8 handler tolerates empty strings (renders "unknown"); real-CPU test parity lives outside CI per spec. |
| **Engine config-node lifecycle on Start() error** | Verified — engine logs an error and continues; the missing `configInstances[id]` later surfaces via `resolveConfigInstance` returning a clear "not found" error to dependent nodes. **Deploy is NOT blocked.** | Low | Matches modbus; same status-pill behaviour. |
| **`ApplyScale` / `UnapplyScale` reuse** | Verified — already package-level in `modbus_codec.go`, same `package nodes` | None | Direct reuse. No new helper needed. |
| **`TlsConfigSection` reuse** | Not relevant — S7 is unencrypted in v1 | None | Skip TLS UI block entirely for `S7PlcConfig.vue`. |
| **Demo DB1 too small to test block-mode auto-split** (200 B < 462 B PDU payload) | Real | Medium | PR-6 extends demo with DB2 (600 B) populated with a deterministic ramp. **Single demo PLC change**, name it now so the implementer knows. |
| **gos7 maintenance** — last commit Dec 2024, sole maintainer | Real | Low | Vendor it if abandonment worries grow. CGO Snap7 wrapper is the documented fallback (NODE_S7.md "Library Choice"). |
| **`tcpTransporter` reconnect semantics** — gos7 doesn't expose a clean "reconnect" call; we call `Close()` + `Connect()` on the handler | Code-read | Medium | Test explicitly: TestS7PlcReconnect closes the demo PLC's TCP listener mid-session, asserts the next read recovers. |
| **Frontend address regex drift** — backend `s7_address.go` and frontend `S7AddressInput.vue` could diverge | Real | Medium | Cross-reference comment in both files; add a TS unit test that exercises the regex set against the same fixture table the Go test uses. |

---

## Demo PLC Gaps

The current demo (`demo/s7-server/main.py`) covers most fixtures except one. Add these in PR-6:

1. **DB2, size 600 bytes**: deterministic byte ramp (`buf[i] = i & 0xFF`). Required for `TestS7ReadBlockOversized` and `TestS7WriteBlockOversized` (split + concat round-trip). Only addition; no animation needed.

Everything else the spec calls for is already in the demo:
- DB1 (200 B): Temperature/Pressure/Counter/Setpoint/Mode/Energy/STRING — covers parser fixtures.
- DB10 (50 B): Sine wave + RPM — covers the live-polling smoke test.
- M (16 B): Heartbeat + writable scratch — covers M-area read/write.
- I (16 B): Running-light pattern — covers I-area read.
- Q (16 B): All writable — covers Q-area write.

No other demo work needed.

---

## Done-Criteria per PR (shippable vs. follow-up-needed)

| PR | "Shippable end-to-end" criterion |
|---|---|
| PR-1 | Tests green. Not user-visible alone — depends on PR-3+. |
| PR-2 | Tests green. Not user-visible alone — depends on PR-3+ or PR-7. |
| PR-3 | Config node deployable; `Test-Connection` works once PR-8 lands. **Not user-visible without PR-4/5 or frontend.** |
| PR-4 | Vertical slice with PR-3+PR-9 gives a working `s7-read`. **Shippable as the first user-visible S7 milestone.** |
| PR-5 | Vertical slice with PR-3+PR-9 gives a working `s7-write`. Shippable. |
| PR-6 | Block mode shippable end-to-end alone (raw bytes to Debug). Becomes a serious story with PR-7+PR-10. |
| PR-7 | Parser node shippable in isolation (Inject `[]byte` → s7-parser → Debug). Becomes the production story with PR-6 upstream. |
| PR-8 | Test-Connection button works. Pure UX gain, not a hard dependency. |
| PR-9 | Read+write user-visible. **First "complete" ship: PR-1..5+8+9.** |
| PR-10 | Parser user-visible. Final ship: PR-1..10. |

**Two natural release boundaries**:
- **Milestone A** = PR-1..5, PR-8, PR-9 → "S7 read/write variables mode, end-to-end, with Test-Connection". Skips block + parser. This is the minimum that's worth a release note.
- **Milestone B** = + PR-6, PR-7, PR-10 → "S7 block mode + declarative parser". This is the production-grade story (the one operators actually use for tightly-packed Siemens DBs).

If the user wants to ship in halves, A is the natural cut.

---

## Critical Files for Implementation

The 5 files whose design choices ripple farthest:

- `internal/nodes/s7_address.go` — address parser, frontend regex source of truth
- `internal/nodes/s7_codec.go` — codec primitives, shared by read/write/parser
- `internal/nodes/s7_plc.go` — connection manager, PDU bundling, serialization, reconnect
- `internal/nodes/s7_read.go` — drives variables AND block mode dispatch (largest surface)
- `demo/s7-server/main.py` — DB2 extension is the only demo change needed across the entire feature
