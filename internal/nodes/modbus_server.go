// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/goburrow/modbus"
	"github.com/loopzedev/loopze-edge/internal/flow"
)

// ModbusServer is a config node that manages a single Modbus connection
// (TCP or RTU). Multiple modbus-read / modbus-write nodes can reference the
// same server; all wire-level requests are serialised through a single mutex
// because Modbus is a half-duplex request/response protocol — only one
// transaction may be in flight per connection.
//
// Implements flow.ConfigInstance.
type ModbusServer struct {
	id   string
	name string

	transport        string // "tcp" or "rtu"
	tcpHandler       *modbus.TCPClientHandler
	rtuHandler       *modbus.RTUClientHandler
	client           modbus.Client
	defaultUnitID    byte
	timeout          time.Duration
	reconnectBackoff time.Duration

	mu           sync.Mutex
	statusFuncs  []flow.StatusFunc
	currentFill  string
	currentText  string
	lastFailedAt time.Time
}

// NewModbusServer parses a ConfigNode definition and constructs a server
// instance. Connection itself is deferred to Start().
func NewModbusServer(cfg flow.ConfigNode) (flow.ConfigInstance, error) {
	props := cfg.Config

	transport, _ := props["transport"].(string)
	if transport == "" {
		transport = "tcp"
	}
	if transport != "tcp" && transport != "rtu" {
		return nil, fmt.Errorf("modbus-server %s: invalid transport %q (expected tcp|rtu)", cfg.ID, transport)
	}

	timeoutMs := readIntProp(props, "timeout", 1000)
	timeout := time.Duration(timeoutMs) * time.Millisecond

	idleTimeoutSec := readIntProp(props, "idleTimeout", 60)
	idleTimeout := time.Duration(idleTimeoutSec) * time.Second
	if idleTimeoutSec == 0 {
		idleTimeout = 0
	}

	defaultUnit := byte(readIntProp(props, "defaultUnitId", 1))
	reconnectSec := readIntProp(props, "reconnectBackoff", 5)
	reconnectBackoff := time.Duration(reconnectSec) * time.Second

	s := &ModbusServer{
		id:               cfg.ID,
		name:             cfg.Name,
		transport:        transport,
		defaultUnitID:    defaultUnit,
		timeout:          timeout,
		reconnectBackoff: reconnectBackoff,
		currentFill:      "grey",
		currentText:      "ready",
	}

	switch transport {
	case "tcp":
		host, _ := props["host"].(string)
		if host == "" {
			return nil, fmt.Errorf("modbus-server %s: host is required for TCP", cfg.ID)
		}
		port := readIntProp(props, "port", 502)
		h := modbus.NewTCPClientHandler(fmt.Sprintf("%s:%d", host, port))
		h.Timeout = timeout
		h.IdleTimeout = idleTimeout
		s.tcpHandler = h
		s.client = modbus.NewClient(h)

	case "rtu":
		serialPort, _ := props["serialPort"].(string)
		if serialPort == "" {
			return nil, fmt.Errorf("modbus-server %s: serialPort is required for RTU", cfg.ID)
		}
		baud := readIntProp(props, "baudRate", 9600)
		dataBits := readIntProp(props, "dataBits", 8)
		stopBits := readIntProp(props, "stopBits", 1)
		parity, _ := props["parity"].(string)
		if parity == "" {
			parity = "none"
		}
		h := modbus.NewRTUClientHandler(serialPort)
		h.BaudRate = baud
		h.DataBits = dataBits
		h.StopBits = stopBits
		h.Parity = parityCode(parity)
		h.Timeout = timeout
		h.IdleTimeout = idleTimeout
		s.rtuHandler = h
		s.client = modbus.NewClient(h)
	}

	return s, nil
}

// ModbusServerConfigTypeInfo returns the config type metadata for the frontend.
func ModbusServerConfigTypeInfo() flow.ConfigTypeInfo {
	return flow.ConfigTypeInfo{
		Type:        "modbus-server",
		Label:       "Modbus Server",
		Description: "Modbus TCP or RTU connection",
		Defaults: map[string]any{
			"transport":        "tcp",
			"host":             "localhost",
			"port":             502,
			"serialPort":       "",
			"baudRate":         9600,
			"dataBits":         8,
			"stopBits":         1,
			"parity":           "none",
			"timeout":          1000,
			"idleTimeout":      60,
			"defaultUnitId":    1,
			"reconnectBackoff": 5,
		},
	}
}

// Start opens the underlying connection eagerly so that connectivity errors
// surface in the UI immediately. A failure does not block deploy — the next
// request will retry.
func (s *ModbusServer) Start() error {
	s.setStatus("yellow", "connecting...")
	if err := s.connect(); err != nil {
		s.setStatus("red", err.Error())
		slog.Warn("modbus-server connect failed", "id", s.id, "error", err)
		// Don't return error: we accept that the device may be temporarily
		// unreachable at deploy time. Status reflects the situation.
		return nil
	}
	s.setStatus("green", "connected")
	slog.Info("modbus-server connected", "id", s.id, "name", s.name, "transport", s.transport)
	return nil
}

// Stop closes the underlying connection.
func (s *ModbusServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var err error
	if s.tcpHandler != nil {
		err = s.tcpHandler.Close()
	} else if s.rtuHandler != nil {
		err = s.rtuHandler.Close()
	}
	s.setStatusLocked("grey", "stopped")
	slog.Info("modbus-server stopped", "id", s.id)
	if err != nil {
		slog.Warn("modbus-server close failed", "id", s.id, "error", err)
	}
	return nil
}

// Status returns the current connection state.
func (s *ModbusServer) Status() (string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentFill, s.currentText
}

// RegisterStatusFunc allows referencing nodes to receive connection state
// changes. The current state is delivered immediately.
func (s *ModbusServer) RegisterStatusFunc(fn flow.StatusFunc) {
	s.mu.Lock()
	s.statusFuncs = append(s.statusFuncs, fn)
	fill, text := s.currentFill, s.currentText
	s.mu.Unlock()
	fn(fill, text)
}

// DefaultUnitID returns the unit ID configured at the server level — used as
// fallback when a node doesn't specify its own.
func (s *ModbusServer) DefaultUnitID() byte { return s.defaultUnitID }

// Name returns the human-readable server name (used in default topic strings).
func (s *ModbusServer) Name() string { return s.name }

// connect opens the underlying connection. Caller must NOT hold s.mu.
func (s *ModbusServer) connect() error {
	if s.tcpHandler != nil {
		return s.tcpHandler.Connect()
	}
	if s.rtuHandler != nil {
		return s.rtuHandler.Connect()
	}
	return errors.New("no handler configured")
}

// setSlaveIDLocked updates the slave ID on the active handler. Caller must
// hold s.mu so SlaveId-set + wire-call are atomic.
func (s *ModbusServer) setSlaveIDLocked(unitID byte) {
	if s.tcpHandler != nil {
		s.tcpHandler.SlaveId = unitID
	} else if s.rtuHandler != nil {
		s.rtuHandler.SlaveId = unitID
	}
}

// Read executes a read function code (1=Coils, 2=DiscreteInputs,
// 3=HoldingRegisters, 4=InputRegisters) and returns the raw response bytes.
// Concurrent calls across nodes referencing the same server are serialised.
func (s *ModbusServer) Read(unitID byte, fc int, address, quantity uint16) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.setSlaveIDLocked(unitID)
	var (
		out []byte
		err error
	)
	switch fc {
	case 1:
		out, err = s.client.ReadCoils(address, quantity)
	case 2:
		out, err = s.client.ReadDiscreteInputs(address, quantity)
	case 3:
		out, err = s.client.ReadHoldingRegisters(address, quantity)
	case 4:
		out, err = s.client.ReadInputRegisters(address, quantity)
	default:
		return nil, fmt.Errorf("read: unsupported function code %d (expected 1|2|3|4)", fc)
	}
	s.recordResultLocked(err)
	return out, wrapModbusError(err)
}

// WriteSingleCoil executes FC5.
func (s *ModbusServer) WriteSingleCoil(unitID byte, address uint16, value bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.setSlaveIDLocked(unitID)
	v := uint16(0)
	if value {
		v = 0xFF00 // Modbus convention: 0xFF00 = ON, 0x0000 = OFF
	}
	_, err := s.client.WriteSingleCoil(address, v)
	s.recordResultLocked(err)
	return wrapModbusError(err)
}

// WriteSingleRegister executes FC6.
func (s *ModbusServer) WriteSingleRegister(unitID byte, address, value uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.setSlaveIDLocked(unitID)
	_, err := s.client.WriteSingleRegister(address, value)
	s.recordResultLocked(err)
	return wrapModbusError(err)
}

// WriteMultipleCoils executes FC15. The packed bytes carry coils LSB-first
// per byte (use packCoilBits to produce them).
func (s *ModbusServer) WriteMultipleCoils(unitID byte, address uint16, packed []byte, quantity uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.setSlaveIDLocked(unitID)
	_, err := s.client.WriteMultipleCoils(address, quantity, packed)
	s.recordResultLocked(err)
	return wrapModbusError(err)
}

// WriteMultipleRegisters executes FC16.
func (s *ModbusServer) WriteMultipleRegisters(unitID byte, address uint16, regs []uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.setSlaveIDLocked(unitID)
	data := RegistersToBytes(regs)
	_, err := s.client.WriteMultipleRegisters(address, uint16(len(regs)), data)
	s.recordResultLocked(err)
	return wrapModbusError(err)
}

// recordResultLocked updates connection status based on the result of the
// last wire call. Caller must hold s.mu.
func (s *ModbusServer) recordResultLocked(err error) {
	if err == nil {
		s.lastFailedAt = time.Time{}
		s.setStatusLocked("green", "connected")
		return
	}
	// A ModbusError is a protocol-level exception (slave responded). The
	// connection is fine, only the request was rejected — keep status green
	// so other nodes don't see a false alarm; the per-node error path
	// surfaces the exception to its catch handler.
	var mbErr *modbus.ModbusError
	if errors.As(err, &mbErr) {
		return
	}
	s.lastFailedAt = time.Now()
	s.setStatusLocked("red", err.Error())
}

// wrapModbusError annotates a modbus.ModbusError with its exception code
// label so users see e.g. "Illegal Data Address (0x02)" instead of just
// "modbus: exception '2' (illegal data address), function '131'".
func wrapModbusError(err error) error {
	if err == nil {
		return nil
	}
	var mbErr *modbus.ModbusError
	if errors.As(err, &mbErr) {
		return fmt.Errorf("%s (0x%02x)", exceptionLabel(mbErr.ExceptionCode), mbErr.ExceptionCode)
	}
	return err
}

// exceptionLabel returns the canonical Modbus exception name for code.
func exceptionLabel(code byte) string {
	switch code {
	case modbus.ExceptionCodeIllegalFunction:
		return "Illegal Function"
	case modbus.ExceptionCodeIllegalDataAddress:
		return "Illegal Data Address"
	case modbus.ExceptionCodeIllegalDataValue:
		return "Illegal Data Value"
	case modbus.ExceptionCodeServerDeviceFailure:
		return "Server Device Failure"
	case modbus.ExceptionCodeAcknowledge:
		return "Acknowledge"
	case modbus.ExceptionCodeServerDeviceBusy:
		return "Server Device Busy"
	case modbus.ExceptionCodeMemoryParityError:
		return "Memory Parity Error"
	case modbus.ExceptionCodeGatewayPathUnavailable:
		return "Gateway Path Unavailable"
	case modbus.ExceptionCodeGatewayTargetDeviceFailedToRespond:
		return "Gateway Target Device Failed to Respond"
	default:
		return fmt.Sprintf("Modbus Exception 0x%02x", code)
	}
}

// setStatus updates the status and notifies all registered listeners.
func (s *ModbusServer) setStatus(fill, text string) {
	s.mu.Lock()
	s.setStatusLocked(fill, text)
	s.mu.Unlock()
}

// setStatusLocked must be called with s.mu held.
func (s *ModbusServer) setStatusLocked(fill, text string) {
	if s.currentFill == fill && s.currentText == text {
		return
	}
	s.currentFill = fill
	s.currentText = text
	for _, fn := range s.statusFuncs {
		fn(fill, text)
	}
}

// readIntProp extracts an integer property from a config map, accepting the
// usual JSON-decoded types and falling back to a default when missing/invalid.
func readIntProp(props map[string]any, key string, fallback int) int {
	switch x := props[key].(type) {
	case nil:
		return fallback
	case float64:
		return int(x)
	case int:
		return x
	case int64:
		return int(x)
	default:
		return fallback
	}
}

// parityCode normalises the human-friendly "none|even|odd" config value into
// the single-char code expected by goburrow/serial.
func parityCode(s string) string {
	switch s {
	case "even", "E":
		return "E"
	case "odd", "O":
		return "O"
	default:
		return "N"
	}
}
