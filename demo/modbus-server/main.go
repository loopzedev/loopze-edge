// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

// Modbus-demo is a small Modbus TCP slave for testing the LOOPZE modbus-read
// and modbus-write nodes (and any other Modbus client). It pre-fills its
// register space with a few well-known values and continuously simulates a
// handful of "live" measurements so polling clients see motion.
//
// Layout of pre-filled / animated values:
//
//	Holding Registers (FC3, RW)
//	  0..1   float32   Temperature (°C)        — drifts in [18.0, 24.0]
//	  2..3   uint32    Tick counter            — increments every 250 ms
//	  4..5   float32   Pressure (bar)          — random walk around 1.0
//	  6      int16     Setpoint                — RW; default 200
//	  7      uint16    Mode                    — RW; default 1
//	  10..14 string    "LOOPZE-DEMO"            — 5 registers, 10 ASCII chars
//	  20..21 float32   Energy (kWh)            — monotonically increasing
//
//	Input Registers (FC4, RO)
//	  0..1   float32   Live sensor (sinusoid)
//	  2      uint16    RPM                     — random walk
//
//	Coils (FC1, RW)              0..63 — all writable, default false
//	Discrete Inputs (FC2, RO)    0..15 — rotating "running light" pattern
//
// All values are encoded big-endian (Modbus default) with big-endian word
// order (ABCD) for multi-register types. Use Little-byte / Little-word in
// your client to verify your codec under all four order combinations.
package main

import (
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand/v2"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const (
	// Modbus exception codes returned in error responses.
	exIllegalFunction     = 0x01
	exIllegalDataAddress  = 0x02
	exIllegalDataValue    = 0x03
	exServerDeviceFailure = 0x04

	// Modbus TCP frame layout.
	mbapHeaderSize = 7
	maxADU         = 260

	// Address-space size. 65536 of each region as per spec; we allocate the
	// full space so any address the client requests is valid (cheap memory).
	regionSize = 65536
)

// dataStore holds all four Modbus register/coil regions. Access is guarded
// by mu; demo simulation and request handlers both acquire it.
type dataStore struct {
	mu              sync.Mutex
	holdingReg      [regionSize]uint16
	inputReg        [regionSize]uint16
	coils           [regionSize]bool
	discreteInputs  [regionSize]bool
	startedAt       time.Time
	rpm             float64
	pressure        float64
}

func newDataStore() *dataStore {
	d := &dataStore{
		startedAt: time.Now(),
		rpm:       1500,
		pressure:  1.0,
	}
	d.preload()
	return d
}

// preload writes the static demo string and seeds initial values.
func (d *dataStore) preload() {
	// "LOOPZE-DEMO" → 10 ASCII bytes, packed 2 chars/register, big-endian.
	demoStr := "LOOPZE-DEMO"
	for i := 0; i < len(demoStr); i += 2 {
		hi := byte(demoStr[i])
		var lo byte
		if i+1 < len(demoStr) {
			lo = byte(demoStr[i+1])
		}
		d.holdingReg[10+i/2] = uint16(hi)<<8 | uint16(lo)
	}
	d.writeInt16Locked(d.holdingReg[:], 6, 200) // setpoint
	d.holdingReg[7] = 1                         // mode
}

// simulate runs in the background and animates the live values.
func (d *dataStore) simulate() {
	tickFast := time.NewTicker(250 * time.Millisecond)
	tickSec := time.NewTicker(time.Second)
	defer tickFast.Stop()
	defer tickSec.Stop()

	for {
		select {
		case <-tickFast.C:
			d.mu.Lock()
			elapsed := time.Since(d.startedAt).Seconds()

			// Holding 0..1: temperature drift in [18, 24] via slow sinus + noise.
			temp := 21.0 + 3.0*math.Sin(elapsed/30.0) + (rand.Float64()-0.5)*0.2
			d.writeFloat32Locked(d.holdingReg[:], 0, float32(temp))

			// Holding 2..3: 250 ms tick counter (4× per second).
			counter := uint32(elapsed * 4)
			d.writeUint32Locked(d.holdingReg[:], 2, counter)

			// Holding 4..5: pressure random walk, clamped near 1.0 bar.
			d.pressure += (rand.Float64() - 0.5) * 0.02
			if d.pressure < 0.7 {
				d.pressure = 0.7
			}
			if d.pressure > 1.3 {
				d.pressure = 1.3
			}
			d.writeFloat32Locked(d.holdingReg[:], 4, float32(d.pressure))

			// Holding 20..21: energy accumulator in kWh (rises 1 kWh / minute).
			energy := elapsed / 60.0
			d.writeFloat32Locked(d.holdingReg[:], 20, float32(energy))

			// Input 0..1: live sensor — pure 0.5 Hz sinus, range [-1, +1].
			live := math.Sin(elapsed * math.Pi)
			d.writeFloat32Locked(d.inputReg[:], 0, float32(live))

			// Input 2: RPM random walk in [1200, 1800].
			d.rpm += (rand.Float64() - 0.5) * 5
			if d.rpm < 1200 {
				d.rpm = 1200
			}
			if d.rpm > 1800 {
				d.rpm = 1800
			}
			d.inputReg[2] = uint16(d.rpm)

			d.mu.Unlock()

		case <-tickSec.C:
			// Discrete inputs 0..15: running light, one bit shifts left every second.
			d.mu.Lock()
			pos := int(time.Since(d.startedAt).Seconds()) % 16
			for i := range d.discreteInputs[:16] {
				d.discreteInputs[i] = (i == pos)
			}
			d.mu.Unlock()
		}
	}
}

func (d *dataStore) writeFloat32Locked(regs []uint16, addr int, v float32) {
	bits := math.Float32bits(v)
	regs[addr] = uint16(bits >> 16)
	regs[addr+1] = uint16(bits & 0xFFFF)
}

func (d *dataStore) writeUint32Locked(regs []uint16, addr int, v uint32) {
	regs[addr] = uint16(v >> 16)
	regs[addr+1] = uint16(v & 0xFFFF)
}

func (d *dataStore) writeInt16Locked(regs []uint16, addr int, v int16) {
	regs[addr] = uint16(v)
}

// ── Request handling ─────────────────────────────────────────────────────────

// handleRequest dispatches an incoming PDU to the matching FC handler.
// Returns either the response PDU body (function code + data) or a Modbus
// exception (set isException=true with the exception code in data[0]).
func (d *dataStore) handleRequest(fc byte, data []byte) (respFC byte, respData []byte, exception bool) {
	switch fc {
	case 0x01: // Read Coils
		return d.readBits(fc, data, d.coilsAt)
	case 0x02: // Read Discrete Inputs
		return d.readBits(fc, data, d.discreteAt)
	case 0x03: // Read Holding Registers
		return d.readRegs(fc, data, d.holdingAt)
	case 0x04: // Read Input Registers
		return d.readRegs(fc, data, d.inputAt)
	case 0x05: // Write Single Coil
		return d.writeSingleCoil(data)
	case 0x06: // Write Single Register
		return d.writeSingleRegister(data)
	case 0x0F: // Write Multiple Coils
		return d.writeMultipleCoils(data)
	case 0x10: // Write Multiple Registers
		return d.writeMultipleRegisters(data)
	default:
		return fc | 0x80, []byte{exIllegalFunction}, true
	}
}

func (d *dataStore) coilsAt(i int) bool     { return d.coils[i] }
func (d *dataStore) discreteAt(i int) bool  { return d.discreteInputs[i] }
func (d *dataStore) holdingAt(i int) uint16 { return d.holdingReg[i] }
func (d *dataStore) inputAt(i int) uint16   { return d.inputReg[i] }

// readBits services FC1 (coils) and FC2 (discrete inputs).
func (d *dataStore) readBits(fc byte, data []byte, lookup func(int) bool) (byte, []byte, bool) {
	if len(data) != 4 {
		return fc | 0x80, []byte{exIllegalDataValue}, true
	}
	addr := int(binary.BigEndian.Uint16(data[0:2]))
	qty := int(binary.BigEndian.Uint16(data[2:4]))
	if qty < 1 || qty > 2000 || addr+qty > regionSize {
		return fc | 0x80, []byte{exIllegalDataAddress}, true
	}

	byteCount := (qty + 7) / 8
	resp := make([]byte, 1+byteCount)
	resp[0] = byte(byteCount)

	d.mu.Lock()
	defer d.mu.Unlock()
	for i := 0; i < qty; i++ {
		if lookup(addr + i) {
			resp[1+i/8] |= 1 << uint(i%8)
		}
	}
	return fc, resp, false
}

// readRegs services FC3 (holding) and FC4 (input registers).
func (d *dataStore) readRegs(fc byte, data []byte, lookup func(int) uint16) (byte, []byte, bool) {
	if len(data) != 4 {
		return fc | 0x80, []byte{exIllegalDataValue}, true
	}
	addr := int(binary.BigEndian.Uint16(data[0:2]))
	qty := int(binary.BigEndian.Uint16(data[2:4]))
	if qty < 1 || qty > 125 || addr+qty > regionSize {
		return fc | 0x80, []byte{exIllegalDataAddress}, true
	}

	byteCount := qty * 2
	resp := make([]byte, 1+byteCount)
	resp[0] = byte(byteCount)

	d.mu.Lock()
	defer d.mu.Unlock()
	for i := 0; i < qty; i++ {
		binary.BigEndian.PutUint16(resp[1+i*2:], lookup(addr+i))
	}
	return fc, resp, false
}

func (d *dataStore) writeSingleCoil(data []byte) (byte, []byte, bool) {
	if len(data) != 4 {
		return 0x05 | 0x80, []byte{exIllegalDataValue}, true
	}
	addr := int(binary.BigEndian.Uint16(data[0:2]))
	val := binary.BigEndian.Uint16(data[2:4])
	if val != 0x0000 && val != 0xFF00 {
		return 0x05 | 0x80, []byte{exIllegalDataValue}, true
	}
	if addr >= regionSize {
		return 0x05 | 0x80, []byte{exIllegalDataAddress}, true
	}
	d.mu.Lock()
	d.coils[addr] = (val == 0xFF00)
	d.mu.Unlock()
	return 0x05, data, false // echo back the request body
}

func (d *dataStore) writeSingleRegister(data []byte) (byte, []byte, bool) {
	if len(data) != 4 {
		return 0x06 | 0x80, []byte{exIllegalDataValue}, true
	}
	addr := int(binary.BigEndian.Uint16(data[0:2]))
	val := binary.BigEndian.Uint16(data[2:4])
	if addr >= regionSize {
		return 0x06 | 0x80, []byte{exIllegalDataAddress}, true
	}
	d.mu.Lock()
	d.holdingReg[addr] = val
	d.mu.Unlock()
	return 0x06, data, false // echo
}

func (d *dataStore) writeMultipleCoils(data []byte) (byte, []byte, bool) {
	if len(data) < 6 {
		return 0x0F | 0x80, []byte{exIllegalDataValue}, true
	}
	addr := int(binary.BigEndian.Uint16(data[0:2]))
	qty := int(binary.BigEndian.Uint16(data[2:4]))
	byteCount := int(data[4])
	if qty < 1 || qty > 1968 || byteCount != (qty+7)/8 || len(data) != 5+byteCount {
		return 0x0F | 0x80, []byte{exIllegalDataValue}, true
	}
	if addr+qty > regionSize {
		return 0x0F | 0x80, []byte{exIllegalDataAddress}, true
	}

	d.mu.Lock()
	for i := 0; i < qty; i++ {
		d.coils[addr+i] = (data[5+i/8]>>uint(i%8))&1 == 1
	}
	d.mu.Unlock()

	resp := make([]byte, 4)
	binary.BigEndian.PutUint16(resp[0:2], uint16(addr))
	binary.BigEndian.PutUint16(resp[2:4], uint16(qty))
	return 0x0F, resp, false
}

func (d *dataStore) writeMultipleRegisters(data []byte) (byte, []byte, bool) {
	if len(data) < 6 {
		return 0x10 | 0x80, []byte{exIllegalDataValue}, true
	}
	addr := int(binary.BigEndian.Uint16(data[0:2]))
	qty := int(binary.BigEndian.Uint16(data[2:4]))
	byteCount := int(data[4])
	if qty < 1 || qty > 123 || byteCount != qty*2 || len(data) != 5+byteCount {
		return 0x10 | 0x80, []byte{exIllegalDataValue}, true
	}
	if addr+qty > regionSize {
		return 0x10 | 0x80, []byte{exIllegalDataAddress}, true
	}

	d.mu.Lock()
	for i := 0; i < qty; i++ {
		d.holdingReg[addr+i] = binary.BigEndian.Uint16(data[5+i*2:])
	}
	d.mu.Unlock()

	resp := make([]byte, 4)
	binary.BigEndian.PutUint16(resp[0:2], uint16(addr))
	binary.BigEndian.PutUint16(resp[2:4], uint16(qty))
	return 0x10, resp, false
}

// ── TCP framing ──────────────────────────────────────────────────────────────

// serveConn reads MBAP-framed requests, dispatches them, and writes responses.
// It loops until the client closes the connection or a framing error occurs.
func serveConn(conn net.Conn, store *dataStore, verbose bool) {
	defer conn.Close()
	remote := conn.RemoteAddr().String()
	slog.Info("client connected", "remote", remote)
	defer slog.Info("client disconnected", "remote", remote)

	buf := make([]byte, maxADU)
	for {
		// Read MBAP header.
		if _, err := io.ReadFull(conn, buf[:mbapHeaderSize]); err != nil {
			if !errors.Is(err, io.EOF) {
				slog.Debug("read header failed", "remote", remote, "error", err)
			}
			return
		}
		txID := binary.BigEndian.Uint16(buf[0:2])
		protoID := binary.BigEndian.Uint16(buf[2:4])
		length := binary.BigEndian.Uint16(buf[4:6])
		unitID := buf[6]

		if protoID != 0 {
			slog.Warn("invalid protocol id", "remote", remote, "proto", protoID)
			return
		}
		if length < 2 || int(length)-1 > maxADU-mbapHeaderSize {
			slog.Warn("invalid length", "remote", remote, "length", length)
			return
		}

		// Read PDU (length covers unit + FC + data; unit was already read).
		pduLen := int(length) - 1
		if _, err := io.ReadFull(conn, buf[mbapHeaderSize:mbapHeaderSize+pduLen]); err != nil {
			slog.Debug("read pdu failed", "remote", remote, "error", err)
			return
		}
		fc := buf[mbapHeaderSize]
		data := buf[mbapHeaderSize+1 : mbapHeaderSize+pduLen]

		respFC, respData, isException := store.handleRequest(fc, data)

		if verbose {
			tag := "ok"
			if isException {
				tag = fmt.Sprintf("exception 0x%02x", respData[0])
			}
			slog.Info("request",
				"remote", remote, "unit", unitID, "tx", txID,
				"fc", fmt.Sprintf("0x%02x", fc), "result", tag)
		}

		// Build response: MBAP header + FC + data.
		resp := make([]byte, mbapHeaderSize+1+len(respData))
		binary.BigEndian.PutUint16(resp[0:2], txID)
		binary.BigEndian.PutUint16(resp[2:4], 0) // proto id
		binary.BigEndian.PutUint16(resp[4:6], uint16(2+len(respData)))
		resp[6] = unitID
		resp[mbapHeaderSize] = respFC
		copy(resp[mbapHeaderSize+1:], respData)

		if _, err := conn.Write(resp); err != nil {
			slog.Debug("write response failed", "remote", remote, "error", err)
			return
		}
	}
}

func main() {
	listen := flag.String("listen", ":5502", "Modbus TCP listen address (e.g. :502, 127.0.0.1:5502)")
	verbose := flag.Bool("v", false, "log every Modbus request")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	store := newDataStore()
	go store.simulate()

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		slog.Error("listen failed", "addr", *listen, "error", err)
		os.Exit(1)
	}
	slog.Info("modbus-demo listening",
		"addr", ln.Addr().String(),
		"verbose", *verbose,
		"hint", "FC3 holding 0..1=temperature, 2..3=counter, 4..5=pressure; FC4 input 0..1=sinus")

	// Graceful shutdown on Ctrl-C / SIGTERM.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		slog.Info("shutdown signal received")
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			slog.Warn("accept failed", "error", err)
			continue
		}
		go serveConn(conn, store, *verbose)
	}
}
