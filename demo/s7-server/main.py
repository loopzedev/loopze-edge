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

from snap7 import util
from snap7.server import Server, SrvArea
from snap7.server import ServerISOConnection

DB1_SIZE = 200
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
    db10 = make_buf(DB10_SIZE)
    mk = make_buf(MK_SIZE)
    pe = make_buf(PE_SIZE)
    pa = make_buf(PA_SIZE)
    fill_static(db1)

    server = Server(log=False)
    server.register_area(SrvArea.DB, 1, db1)
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
