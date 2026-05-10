#!/usr/bin/env python3
# Copyright (C) 2026 Dennis Bleul
# Licensed under the GNU Affero General Public License v3.0 or later.
# See LICENSE file for details.

"""
LOOPZE S7 demo PLC.

A small SIEMENS S7 server (python-snap7 / libsnap7) for testing the LOOPZE
s7-read / s7-write nodes (and any other S7 client). It pre-fills DB1 / DB10,
the Merker area, and inputs/outputs with well-known values and continuously
simulates a handful of "live" measurements so polling clients see motion.

Layout of pre-filled / animated values:

    DB1 (200 bytes) — general sensor data
      DBD0    REAL     Temperature (°C)    — drifts in [18.0, 24.0]
      DBD4    REAL     Pressure (bar)      — random walk around 1.0
      DBD8    DINT     Tick counter        — increments every 250 ms
      DBW12   INT      Setpoint            — RW; default 200
      DBW14   INT      Mode                — RW; default 1
      DBD16   REAL     Energy (kWh)        — monotonically increasing
      DB1.STRING50.20  S7 STRING           — "LOOPZE-S7-DEMO"

    DB10 (50 bytes) — live measurement
      DBD0    REAL     Sine wave (1 Hz, ±1.0)
      DBW4    INT      RPM                 — random walk in [1200, 1800]

    M (Merker, 16 bytes)
      M0.0    BOOL     Heartbeat           — toggles every second
      M0.1-7  BOOL     Writable scratch flags
      MB1     BYTE     Writable
      MD4     DWORD    Writable counter

    I (Inputs, 16 bytes)
      IB0     Running-light pattern        — one bit shifts per second
      IB1-15  Static zero

    Q (Outputs, 16 bytes)  All writable, default 0
"""

import argparse
import logging
import math
import random
import signal
import struct
import sys
import threading
import time
from datetime import date, datetime, timezone

from snap7 import util
from snap7.server import Server, SrvArea
from snap7.server import ServerISOConnection
from snap7.server import Server as _ServerForPatch  # alias for clarity in monkey-patch
from snap7.s7protocol import S7Function, S7PDUType
from snap7.datatypes import S7WordLen

DB1_SIZE = 200
DB2_SIZE = 600  # Deterministic byte-ramp for block-mode auto-split tests
DB3_SIZE = 256  # Datatype-showcase: every TIA-Portal type at a known offset
DB10_SIZE = 50
MK_SIZE = 16
PE_SIZE = 16
PA_SIZE = 16

DEFAULT_PORT = 1102

log = logging.getLogger("s7-demo")


def _patched_build_cotp_cc(self):
    """Snap7-compatible COTP CC.

    python-snap7 1.4's pure-Python S7 server emits a minimal 11-byte CC
    response (TPKT + 7-byte COTP). The Snap7 reference (and our gos7 client)
    require the CC to carry the standard COTP parameters — TPDU size, calling
    TSAP, called TSAP — for a total wire length of 22 bytes. The python-snap7
    *client* doesn't validate CC length so the upstream tests don't catch
    this; our gos7 integration does.

    This monkey-patch produces the 18-byte COTP body that wraps to a 22-byte
    TPKT frame. We echo back the canonical TSAP defaults — gos7 doesn't
    validate the values, only the frame length and PDU type byte. Filed for
    upstream contribution: https://github.com/gijzelaerr/python-snap7
    """
    cotp_cc = struct.pack(
        ">BBHHB",
        17,                              # COTP header length (excludes itself)
        ServerISOConnection.COTP_CC,    # PDU type 0xD0
        self.dst_ref,                    # destination ref (client's src ref)
        self.src_ref,                    # source ref
        0x00,                            # class / option
    )
    # Standard COTP parameters that Snap7-class tooling expects in a CC.
    params = (
        b"\xC0\x01\x0A"                 # TPDU size code + len + value (1024)
        b"\xC1\x02\x01\x00"             # Calling TSAP (rack/slot encoded)
        b"\xC2\x02\x01\x02"             # Called TSAP
    )
    return cotp_cc + params


ServerISOConnection._build_cotp_cc = _patched_build_cotp_cc


def _patched_handle_read_area(self, request, client_address):
    """Multi-item-aware READ_AREA handler.

    python-snap7 1.4's pure-Python S7 server only parses the FIRST address
    spec from the request and hardcodes ``item_count = 1`` in the response.
    gos7's ``AGReadMulti`` rejects the response with "invalid CPU answer"
    whenever ``itemsRead != itemsCount``, breaking any read of two or more
    variables in a single PDU — which is the standard pattern for
    industrial polling.

    This patch re-parses the raw request parameters (``raw_parameters`` is
    preserved on the request dict by ``_parse_request``) and emits a proper
    multi-item response: header + (function, count) + per-item data section,
    with each item padded to even length so the next item starts aligned.
    Filed for upstream contribution: https://github.com/gijzelaerr/python-snap7
    """
    try:
        raw_params = request.get("raw_parameters", b"")
        if len(raw_params) < 14 or raw_params[0] != S7Function.READ_AREA:
            return self._build_error_response(request, 0x8001)

        item_count = raw_params[1]
        if item_count < 1 or item_count > 20:
            return self._build_error_response(request, 0x8001)
        if len(raw_params) < 2 + item_count * 12:
            return self._build_error_response(request, 0x8001)

        per_item_sections = []
        for idx in range(item_count):
            spec = raw_params[2 + idx * 12 : 2 + (idx + 1) * 12]
            parsed = self._parse_address_specification(spec)
            if not parsed:
                # Per-item error: 0x05 = "invalid address". Empty data section
                # so the offset accounting in gos7 still advances past the
                # 4-byte item header.
                per_item_sections.append(struct.pack(">BBH", 0x05, 0x00, 0))
                continue

            area = parsed["area"]
            db_number = parsed["db_number"]
            start = parsed["start"]
            count = parsed["count"]
            word_len = parsed["word_len"]

            # Width per element — mirrors _parse_read_address (the original
            # single-item handler). WORD/COUNTER/TIMER are 2 bytes,
            # DWORD/DINT/REAL are 4 bytes, BIT is special (1 byte ships).
            if word_len in (S7WordLen.WORD, S7WordLen.COUNTER, S7WordLen.TIMER, S7WordLen.INT):
                byte_count = count * 2
            elif word_len in (S7WordLen.DWORD, S7WordLen.DINT, S7WordLen.REAL):
                byte_count = count * 4
            elif word_len == S7WordLen.BIT:
                byte_count = 1
            else:  # BYTE / CHAR / etc.
                byte_count = count

            data = self._read_from_memory_area(area, db_number, start, byte_count)
            if data is None:
                per_item_sections.append(struct.pack(">BBH", 0x0A, 0x00, 0))  # 0x0A = object not found
                continue

            # Per-item data section: success(0xFF) + transport(0x04 byte) +
            # length-in-bits(u16) + payload. gos7 divides size by 8 unless
            # the transport byte is octet/real/bit, so 0x04 + bits is the
            # canonical "byte stream" encoding.
            section = struct.pack(">BBH", 0xFF, 0x04, len(data) * 8) + bytes(data)
            if len(section) % 2 != 0:
                section += b"\x00"
            per_item_sections.append(section)

        data_section = b"".join(per_item_sections)

        header = struct.pack(
            ">BBHHHHBB",
            0x32,                   # Protocol ID
            S7PDUType.ACK_DATA,     # PDU type
            0x0000,                 # Reserved
            request["sequence"],    # Sequence (echo)
            0x0002,                 # Parameter length (function + count)
            len(data_section),      # Data length
            0x00,                   # Error class (success)
            0x00,                   # Error code (success)
        )
        parameters = struct.pack(">BB", S7Function.READ_AREA, item_count)
        return header + parameters + data_section
    except Exception as e:
        log.error("multi-read patch failed: %s", e, exc_info=True)
        return self._build_error_response(request, 0x8000)


_ServerForPatch._handle_read_area = _patched_handle_read_area


def make_buf(size: int) -> bytearray:
    # Must be `bytearray` (not ctypes): python-snap7's Server stores it by
    # reference, so live mutations from the animator thread are visible to
    # connected S7 clients. ctypes arrays would be silently copied.
    return bytearray(size)


def fill_static(db1) -> None:
    """Initial values that don't move (or move slowly enough that the first
    snapshot is meaningful before the animator thread starts mutating them)."""
    util.set_real(db1, 0, 21.5)               # Temperature
    util.set_real(db1, 4, 1.0)                # Pressure
    util.set_dint(db1, 8, 0)                  # Tick counter
    util.set_int(db1, 12, 200)                # Setpoint
    util.set_int(db1, 14, 1)                  # Mode
    util.set_real(db1, 16, 0.0)               # Energy
    util.set_string(db1, 50, "LOOPZE-S7-DEMO", 20)


def _bcd(n: int) -> int:
    """Pack a 0..99 decimal value into a single BCD byte (high nibble = tens,
    low nibble = units). Used by the DT (DATE_AND_TIME) encoding."""
    return ((n // 10) << 4) | (n % 10)


def fill_db3_datatype_showcase(db3) -> None:
    """Populate DB3 with one well-known value per Siemens TIA-Portal datatype,
    covering the full table from `specifications/issues/NODE_S7.md > Data Types`.

    Offsets are stable across restarts so integration tests can assert byte
    ranges directly. Each fill is documented with its wire encoding so a
    future contributor adding a new type can follow the pattern.

    All multi-byte integers are big-endian on the wire — the canonical S7
    byte order regardless of CPU family.
    """

    # ── Bitfields & small ints ───────────────────────────────────────────
    # BOOL bits at 0.0/0.1/0.7 — pattern 0b10000001 = 0x81
    util.set_bool(db3, 0, 0, True)
    util.set_bool(db3, 0, 1, False)
    util.set_bool(db3, 0, 7, True)

    db3[1] = 0x42                                          # BYTE = 66
    db3[2:3] = struct.pack(">b", -42)                      # SINT = -42
    db3[3] = 200                                           # USINT = 200
    db3[4:6] = struct.pack(">H", 0xCAFE)                   # WORD = 0xCAFE
    db3[6:8] = struct.pack(">h", -12345)                   # INT = -12345
    db3[8:10] = struct.pack(">H", 50000)                   # UINT = 50000

    # ── 32-bit ints & float ──────────────────────────────────────────────
    db3[10:14] = struct.pack(">I", 0xDEADBEEF)             # DWORD
    db3[14:18] = struct.pack(">i", -1_234_567_890)         # DINT
    db3[18:22] = struct.pack(">I", 4_000_000_000)          # UDINT
    db3[22:26] = struct.pack(">f", 3.14159)                # REAL ≈ π

    # ── 64-bit S7-1500 types ─────────────────────────────────────────────
    db3[26:34] = struct.pack(">Q", 0xFEEDFACECAFEBEEF)     # LWORD
    db3[34:42] = struct.pack(">q", -1_234_567_890_123_456) # LINT
    db3[42:50] = struct.pack(">Q", 18_000_000_000_000_000_000)  # ULINT
    db3[50:58] = struct.pack(">d", 2.718281828459045)      # LREAL ≈ e

    # ── Characters & strings ─────────────────────────────────────────────
    db3[58] = ord("A")                                     # CHAR = 'A'
    db3[59:61] = struct.pack(">H", ord("Ω"))               # WCHAR = U+03A9

    # STRING(20) at byte 61: header 2 bytes + 20 char-slots = 22 bytes total.
    # python-snap7's set_string writes [maxLen][actLen][chars…] with the
    # remaining tail zero-padded — exactly what we need.
    util.set_string(db3, 61, "Hello S7", 20)

    # WSTRING(20) at byte 83: header 4 bytes (maxLen u16 + actLen u16) +
    # 20 × 2-byte UCS-2 chars = 44 bytes total. We encode manually to avoid
    # python-snap7's set_wstring quirks (counts bytes vs. chars inconsistently).
    _write_wstring(db3, 83, "Hallo Welt", maxlen=20)

    # ── Time durations ───────────────────────────────────────────────────
    # S5TIME at 127..128: timebase code in upper nibble of byte 0 (00=10ms,
    # 01=100ms, 10=1s, 11=10s) + BCD value in remaining 12 bits (3 nibbles).
    # 5000 ms with 100ms timebase → BCD value 50 → byte0=0x10, byte1=0x50.
    db3[127] = (1 << 4) | 0  # 0x10 = 100ms timebase + hundreds digit (0)
    db3[128] = 0x50          # tens+units BCD (= 50)

    # TIME at 129..132: signed int32 milliseconds. T#1d2h3m4s567ms.
    time_ms = ((((1 * 24 + 2) * 60) + 3) * 60 + 4) * 1000 + 567  # = 93_784_567
    db3[129:133] = struct.pack(">i", time_ms)

    # LTIME at 133..140: signed int64 nanoseconds.
    # T#1d2h3m4s567ms890us123ns
    ltime_ns = time_ms * 1_000_000 + 890 * 1000 + 123
    db3[133:141] = struct.pack(">q", ltime_ns)

    # ── Date & time ──────────────────────────────────────────────────────
    # DATE at 141..142: int16 days since 1990-01-01.
    days = (date(2026, 5, 10) - date(1990, 1, 1)).days
    db3[141:143] = struct.pack(">H", days)

    # TOD at 143..146: uint32 milliseconds since midnight. 12:34:56.789.
    tod_ms = ((12 * 60 + 34) * 60 + 56) * 1000 + 789  # = 45_296_789
    db3[143:147] = struct.pack(">I", tod_ms)

    # LTOD at 147..154: uint64 nanoseconds since midnight. 12:34:56.123_456_789.
    ltod_ns = ((12 * 60 + 34) * 60 + 56) * 1_000_000_000 + 123_456_789
    db3[147:155] = struct.pack(">Q", ltod_ns)

    # DT at 155..162: 8 bytes BCD encoding per S7 spec
    #   byte 0: year-2000 (BCD; for years <90, otherwise year-1900)
    #   byte 1: month (BCD)
    #   byte 2: day (BCD)
    #   byte 3: hour (BCD)
    #   byte 4: minute (BCD)
    #   byte 5: second (BCD)
    #   byte 6: ms high 2 digits (BCD; ms / 10)
    #   byte 7: ms last digit (high nibble) | weekday (low nibble; 1=Sun..7=Sat)
    # Sample: 2026-05-10 12:34:56.789, weekday Sun → 1
    db3[155] = _bcd(2026 - 2000)  # year offset
    db3[156] = _bcd(5)             # month
    db3[157] = _bcd(10)            # day
    db3[158] = _bcd(12)            # hour
    db3[159] = _bcd(34)            # min
    db3[160] = _bcd(56)            # sec
    db3[161] = _bcd(789 // 10)     # high 2 ms digits
    db3[162] = (_bcd(789 % 10) << 4) | 1  # low ms digit + weekday (Sun)

    # LDT at 163..170: int64 nanoseconds since 1970-01-01 UTC.
    dt_obj = datetime(2026, 5, 10, 12, 34, 56, 789_000, tzinfo=timezone.utc)
    epoch_ns = int(dt_obj.timestamp()) * 1_000_000_000 + dt_obj.microsecond * 1000
    db3[163:171] = struct.pack(">q", epoch_ns)

    # DTL at 171..182: 12 bytes structured
    #   bytes 0-1: year (uint16 BE)
    #   byte  2: month
    #   byte  3: day
    #   byte  4: weekday (1=Sun..7=Sat — same as DT)
    #   byte  5: hour
    #   byte  6: min
    #   byte  7: sec
    #   bytes 8-11: nanoseconds (uint32 BE, 0..999_999_999)
    db3[171:173] = struct.pack(">H", 2026)
    db3[173] = 5
    db3[174] = 10
    db3[175] = 1   # Sun
    db3[176] = 12
    db3[177] = 34
    db3[178] = 56
    db3[179:183] = struct.pack(">I", 789_000_000)


def _write_wstring(buf, pos: int, value: str, maxlen: int) -> None:
    """Encode a Siemens WSTRING manually. Wire layout:
        [maxLen u16][actLen u16][char × maxLen × u16]
    All multi-byte fields are big-endian. Code points above U+FFFF are
    truncated to their low 16 bits (UCS-2 / BMP-only) — matching gos7's
    SetWStringAt behaviour and the LOOPZE codec's contract."""
    actlen = min(len(value), maxlen)
    buf[pos:pos + 2] = struct.pack(">H", maxlen)
    buf[pos + 2:pos + 4] = struct.pack(">H", actlen)
    for i, ch in enumerate(value[:actlen]):
        buf[pos + 4 + i * 2:pos + 6 + i * 2] = struct.pack(">H", ord(ch) & 0xFFFF)


def animate(db1, db10, mk, pe, stop_event: threading.Event) -> None:
    """Background thread mutating live values. Mutations on the ctypes
    buffers are visible to S7 clients immediately because libsnap7 reads from
    the same memory region we registered with `register_area`."""
    started = time.monotonic()
    rpm = 1500.0
    last_input_shift = started
    last_heartbeat = started
    heartbeat = False
    pe[0] = 0x01  # initial running-light bit position 0

    while not stop_event.is_set():
        now = time.monotonic()
        elapsed = now - started

        # DB1 temperature: slow drift in [18, 24]
        temp = 21.0 + 3.0 * math.sin(elapsed / 30.0) + random.uniform(-0.05, 0.05)
        util.set_real(db1, 0, max(18.0, min(24.0, temp)))

        # DB1 pressure: random walk in [0.5, 1.5]
        prev = util.get_real(db1, 4)
        new_p = max(0.5, min(1.5, prev + random.uniform(-0.02, 0.02)))
        util.set_real(db1, 4, new_p)

        # DB1 tick counter: +1 every 250 ms ⇒ +4/s
        util.set_dint(db1, 8, util.get_dint(db1, 8) + 1)

        # DB1 energy: 1 kWh / minute = 1/240 per 250 ms
        util.set_real(db1, 16, util.get_real(db1, 16) + 1.0 / 240.0)

        # DB10 sine wave (live sensor): 1 Hz, ±1.0
        util.set_real(db10, 0, math.sin(2.0 * math.pi * elapsed))

        # DB10 RPM: random walk in [1200, 1800]
        rpm = max(1200.0, min(1800.0, rpm + random.uniform(-15.0, 15.0)))
        util.set_int(db10, 4, int(rpm))

        # Heartbeat M0.0 — toggle every second
        if now - last_heartbeat >= 1.0:
            heartbeat = not heartbeat
            util.set_bool(mk, 0, 0, heartbeat)
            last_heartbeat = now

        # Inputs running light — shift one bit every second; wrap at MSB
        if now - last_input_shift >= 1.0:
            cur = pe[0]
            shifted = (cur << 1) & 0xFF
            pe[0] = shifted if shifted else 0x01
            last_input_shift = now

        time.sleep(0.25)


def setup_logging(verbose: bool) -> None:
    logging.basicConfig(
        level=logging.DEBUG if verbose else logging.INFO,
        format="%(asctime)s  %(levelname)-5s  %(message)s",
        datefmt="%H:%M:%S",
    )


def main() -> None:
    parser = argparse.ArgumentParser(description="LOOPZE S7 demo PLC")
    parser.add_argument(
        "-p", "--port", type=int, default=DEFAULT_PORT,
        help="TCP port to listen on (default: %(default)s; use 102 for the "
             "S7 standard port — requires root or CAP_NET_BIND_SERVICE)",
    )
    parser.add_argument(
        "-v", "--verbose", action="store_true",
        help="Log every S7 client request (drains the snap7 event queue)",
    )
    args = parser.parse_args()
    setup_logging(args.verbose)

    db1 = make_buf(DB1_SIZE)
    db2 = make_buf(DB2_SIZE)
    db3 = make_buf(DB3_SIZE)
    db10 = make_buf(DB10_SIZE)
    mk = make_buf(MK_SIZE)
    pe = make_buf(PE_SIZE)
    pa = make_buf(PA_SIZE)
    fill_static(db1)

    # DB2: deterministic ramp `buf[i] = i & 0xFF`. Used by the s7-read block-
    # mode auto-split tests — at 600 bytes it exceeds the typical 462-byte
    # PDU payload, so gos7's readArea has to issue ≥2 AGReadDB calls and
    # concatenate. The ramp lets the test verify byte-perfect concatenation.
    for i in range(DB2_SIZE):
        db2[i] = i & 0xFF

    # DB3: one well-known value per TIA-Portal datatype (BOOL through DTL).
    # Stable offsets so tests and manual UI checks can assert specific bytes.
    fill_db3_datatype_showcase(db3)

    server = Server(log=False)
    server.register_area(SrvArea.DB, 1, db1)
    server.register_area(SrvArea.DB, 2, db2)
    server.register_area(SrvArea.DB, 3, db3)
    server.register_area(SrvArea.DB, 10, db10)
    server.register_area(SrvArea.MK, 0, mk)
    server.register_area(SrvArea.PE, 0, pe)
    server.register_area(SrvArea.PA, 0, pa)

    log.info("LOOPZE S7 demo PLC starting on :%d (rack=0, slot=1)", args.port)
    log.info("Address map: see README.md in this directory")
    log.info("Press Ctrl+C to stop")

    stop_event = threading.Event()
    worker = threading.Thread(
        target=animate, args=(db1, db10, mk, pe, stop_event), daemon=True,
    )
    worker.start()

    def shutdown(_signum, _frame):
        log.info("Shutdown requested — stopping…")
        stop_event.set()
        try:
            server.stop()
            server.destroy()
        finally:
            sys.exit(0)

    signal.signal(signal.SIGINT, shutdown)
    signal.signal(signal.SIGTERM, shutdown)

    server.start(tcp_port=args.port)

    if args.verbose:
        while not stop_event.is_set():
            event = server.pick_event()
            if event:
                log.debug("S7 event: %s", server.event_text(event))
            else:
                time.sleep(0.1)
    else:
        while not stop_event.is_set():
            time.sleep(1.0)


if __name__ == "__main__":
    main()
