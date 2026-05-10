// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package s7

import (
	"github.com/loopzedev/loopze-edge/internal/nodes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/loopzedev/loopze-edge/internal/flow"
	"github.com/robinson/gos7"
)

// S7 connection types — RFC1006 connect parameters. PG (programmer) is the
// default for normal client access on S7-300/400/1200/1500. Basic is what
// LOGO! and S7-200 Smart expect.
const (
	S7ConnectTypePG    = 1
	S7ConnectTypeOP    = 2
	S7ConnectTypeBasic = 3
)

const (
	s7DefaultPort = 102
	s7DefaultPDU  = 480

	// gos7's AGReadMulti / AGWriteMulti hard cap.
	s7MultiItemMaxCount = 20

	// Per-item descriptor in a multi-read request (s7Item in gos7's multi.go is
	// 12 bytes wide). Multi-read header is 19 bytes, response header is 21 bytes
	// plus 4 bytes per per-item result. The split planner uses these to decide
	// how many items fit in one PDU.
	s7MultiRequestHeader  = 19
	s7MultiPerItemReq     = 12
	s7MultiResponseHeader = 21
	s7MultiPerItemRespHdr = 4
)

// S7ReadResult carries the wire-level outcome of one item in a multi-read.
// The PLC may report per-item errors (e.g. one item points at a non-existent
// DB while the rest are fine); those land in Err while the overall function
// still returns nil. Whole-transaction failures (transport, connection lost)
// surface as the function's error return and leave Results empty.
type S7ReadResult struct {
	Item S7Item
	Data []byte // wire bytes; len == Item.ByteSize() on success
	Err  string // PLC-side error string; "" on success
}

// S7WriteItem bundles an addressed S7Item with the bytes to write.
// len(Data) must equal Item.ByteSize() — the manager does not pad or truncate.
type S7WriteItem struct {
	Item S7Item
	Data []byte
}

// S7WriteResult mirrors S7ReadResult on the write side.
type S7WriteResult struct {
	Item S7Item
	Err  string
}

// S7PLC is a config node that owns a single S7 connection. Multiple
// `s7-read` / `s7-write` nodes can reference the same PLC; all wire-level
// requests are serialised through one mutex because S7 is half-duplex per
// connection — only one transaction may be in flight at a time.
//
// Reconnect behaviour is lazy: a wire call detects a transport-level failure
// and marks the connection as dropped; the next call attempts a reconnect,
// throttled by `reconnectBackoff` to avoid hammering an unreachable PLC. A
// background-goroutine variant is tracked as a future optimisation (the
// trade-off is responsiveness vs. complexity; lazy reconnect matches the
// modbus-server pattern and works fine for current loads).
//
// Implements flow.ConfigInstance.
type S7PLC struct {
	id   string
	name string

	// Connection parameters.
	host             string
	port             int
	rack             int
	slot             int
	connectType      int
	pduRequested     int
	timeout          time.Duration
	idleTimeout      time.Duration
	reconnectBackoff time.Duration

	// gos7 transport.
	handler *gos7.TCPClientHandler
	client  gos7.Client

	mu                  sync.Mutex
	connected           bool
	pduActual           int // negotiated PDU; 0 before the first successful connect
	lastReconnectAttempt time.Time
	statusFuncs         []flow.StatusFunc
	currentFill         string
	currentText         string
}

// NewS7PLC parses a ConfigNode definition and constructs a manager instance.
// Connection itself is deferred to Start().
func NewS7PLC(cfg flow.ConfigNode) (flow.ConfigInstance, error) {
	props := cfg.Config

	host, _ := props["host"].(string)
	if host == "" {
		return nil, fmt.Errorf("s7-plc %s: host is required", cfg.ID)
	}
	port := nodes.IntVal(props, "port", s7DefaultPort)
	connection, _ := props["connection"].(string)
	if connection == "" {
		connection = "s7-1200-1500"
	}

	rack, slot, connectType, err := s7ConnectionDefaults(connection, props, cfg.ID)
	if err != nil {
		return nil, err
	}

	pduReq := nodes.IntVal(props, "pduSize", s7DefaultPDU)
	timeoutMs := nodes.IntVal(props, "timeout", 2000)
	timeout := time.Duration(timeoutMs) * time.Millisecond
	idleSec := nodes.IntVal(props, "idleTimeout", 60)
	idleTimeout := time.Duration(idleSec) * time.Second
	if idleSec == 0 {
		idleTimeout = 0
	}
	reconnectSec := nodes.IntVal(props, "reconnectBackoff", 5)
	reconnectBackoff := time.Duration(reconnectSec) * time.Second

	addr := fmt.Sprintf("%s:%d", host, port)
	handler := gos7.NewTCPClientHandlerWithConnectType(addr, rack, slot, connectType)
	handler.Timeout = timeout
	handler.IdleTimeout = idleTimeout

	s := &S7PLC{
		id:               cfg.ID,
		name:             cfg.Name,
		host:             host,
		port:             port,
		rack:             rack,
		slot:             slot,
		connectType:      connectType,
		pduRequested:     pduReq,
		timeout:          timeout,
		idleTimeout:      idleTimeout,
		reconnectBackoff: reconnectBackoff,
		handler:          handler,
		client:           gos7.NewClient(handler),
		currentFill:      "grey",
		currentText:      "ready",
	}
	return s, nil
}

// S7PLCConfigTypeInfo returns the config type metadata for the frontend.
func S7PLCConfigTypeInfo() flow.ConfigTypeInfo {
	return flow.ConfigTypeInfo{
		Type:        "s7-plc",
		Label:       "S7 PLC",
		Description: "SIEMENS S7 connection (RFC1006 / ISO-on-TCP)",
		Defaults: map[string]any{
			"host":             "localhost",
			"port":             s7DefaultPort,
			"connection":       "s7-1200-1500",
			"rack":             0,
			"slot":             1,
			"pduSize":          s7DefaultPDU,
			"timeout":          2000,
			"idleTimeout":      60,
			"reconnectBackoff": 5,
		},
	}
}

// s7ConnectionDefaults derives rack / slot / connectType from the user-facing
// connection enum. Custom mode honours user-supplied rack/slot values.
//
// Free-form local/remote TSAPs (the spec's `localTsap`/`remoteTsap` fields)
// are not exposed in v1 — gos7 only allows TSAPs through the rack/slot encoding
// (`(connectType<<8) | (rack*0x20) | slot`). When customers need raw TSAPs we
// either patch gos7 upstream or use a CGO Snap7 wrapper. Documented in the
// PR-3 plan as a tracked v1.x deferral.
func s7ConnectionDefaults(connection string, props map[string]any, configID string) (rack, slot, connectType int, err error) {
	switch connection {
	case "s7-1200-1500":
		return 0, 1, S7ConnectTypePG, nil
	case "s7-300-400":
		return 0, 2, S7ConnectTypePG, nil
	case "logo":
		// LOGO! / S7-200 Smart — Basic connection type. The gos7 TSAP encoding
		// turns rack=0/slot=2 into the 0x0302 remote TSAP that LOGO! expects.
		return 0, 2, S7ConnectTypeBasic, nil
	case "custom":
		rack = nodes.IntVal(props, "rack", 0)
		slot = nodes.IntVal(props, "slot", 1)
		ct := nodes.IntVal(props, "connectType", S7ConnectTypePG)
		if ct < S7ConnectTypePG || ct > S7ConnectTypeBasic {
			return 0, 0, 0, fmt.Errorf("s7-plc %s: invalid connectType %d (expected 1=PG, 2=OP, 3=Basic)", configID, ct)
		}
		return rack, slot, ct, nil
	default:
		return 0, 0, 0, fmt.Errorf("s7-plc %s: unknown connection %q (expected s7-1200-1500|s7-300-400|logo|custom)", configID, connection)
	}
}

// Start opens the connection eagerly so the user sees connectivity errors at
// deploy time. A failure does not block deploy — status reflects the situation
// and the next call will retry.
func (s *S7PLC) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.setStatusLocked("yellow", "connecting...")
	if err := s.connectLocked(); err != nil {
		s.setStatusLocked("red", err.Error())
		slog.Warn("s7-plc connect failed", "id", s.id, "name", s.name, "address", s.address(), "error", err)
		return nil
	}
	s.setStatusLocked("green", fmt.Sprintf("connected (PDU %d)", s.pduActual))
	slog.Info("s7-plc connected",
		"id", s.id, "name", s.name, "address", s.address(),
		"rack", s.rack, "slot", s.slot, "connectType", s.connectType,
		"pduSize", s.pduActual,
	)
	return nil
}

// Stop closes the connection.
func (s *S7PLC) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.handler != nil {
		if err := s.handler.Close(); err != nil {
			slog.Warn("s7-plc close failed", "id", s.id, "error", err)
		}
	}
	s.connected = false
	s.setStatusLocked("grey", "stopped")
	slog.Info("s7-plc stopped", "id", s.id)
	return nil
}

// Status returns the current connection state for UI display.
func (s *S7PLC) Status() (string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentFill, s.currentText
}

// RegisterStatusFunc allows referencing nodes to receive connection-state
// changes. The current state is delivered immediately so newly-attached UI
// reflects reality without waiting for the next change.
func (s *S7PLC) RegisterStatusFunc(fn flow.StatusFunc) {
	s.mu.Lock()
	s.statusFuncs = append(s.statusFuncs, fn)
	fill, text := s.currentFill, s.currentText
	s.mu.Unlock()
	fn(fill, text)
}

// Name returns the human-readable PLC name (used in default topic strings).
func (s *S7PLC) Name() string { return s.name }

// PDUSize returns the negotiated PDU size, or 0 before the first successful
// connect. Used by the read/write nodes for block-mode auto-split decisions.
func (s *S7PLC) PDUSize() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pduActual
}

// address returns "host:port" for log lines and error messages.
func (s *S7PLC) address() string {
	return fmt.Sprintf("%s:%d", s.host, s.port)
}

// connectLocked opens the wire connection. Caller must hold s.mu.
func (s *S7PLC) connectLocked() error {
	if err := s.handler.Connect(); err != nil {
		s.connected = false
		return err
	}
	s.connected = true
	s.pduActual = s.handler.PDULength
	s.lastReconnectAttempt = time.Time{}
	return nil
}

// connectIfNeededLocked opens the connection lazily on first use after a
// drop. Successive failed attempts are throttled by `reconnectBackoff` so we
// don't hammer an unreachable PLC at the user's request rate. Caller must
// hold s.mu.
func (s *S7PLC) connectIfNeededLocked() error {
	if s.connected {
		return nil
	}
	if !s.lastReconnectAttempt.IsZero() && time.Since(s.lastReconnectAttempt) < s.reconnectBackoff {
		return errors.New("PLC unavailable (waiting for next reconnect window)")
	}
	s.lastReconnectAttempt = time.Now()
	s.setStatusLocked("yellow", "reconnecting...")
	if err := s.connectLocked(); err != nil {
		s.setStatusLocked("red", err.Error())
		return err
	}
	s.setStatusLocked("green", fmt.Sprintf("connected (PDU %d)", s.pduActual))
	return nil
}

// markFailedLocked is called after a wire call fails. If the failure looks
// transport-level (network error, "Connection ... is null"), the connection
// is marked as dropped so the next call triggers a reconnect; per-item PLC
// errors (Item not available, etc.) are treated as application-level and
// leave the connection state intact. Caller must hold s.mu.
func (s *S7PLC) markFailedLocked(err error) {
	if err == nil {
		return
	}
	if isS7TransportError(err) {
		s.connected = false
		_ = s.handler.Close()
		s.setStatusLocked("red", fmt.Sprintf("disconnected: %s", err.Error()))
		return
	}
	// Application-level error — connection is still good.
	s.setStatusLocked("red", err.Error())
}

// markOkLocked is called after a wire call succeeds; resets the status to
// green if it was previously red/yellow. Caller must hold s.mu.
func (s *S7PLC) markOkLocked() {
	s.setStatusLocked("green", fmt.Sprintf("connected (PDU %d)", s.pduActual))
}

// ReadArea fetches one contiguous byte block from the PLC. The gos7 client
// chunks the wire request internally based on PDU size, so this function
// returns the full `length` bytes in a single buffer.
//
// `area` accepts the S7Area* constants from `s7_address.go`. DB reads require
// `db > 0`; M/I/Q ignore `db`.
func (s *S7PLC) ReadArea(area, db, start, length int) ([]byte, error) {
	if length <= 0 {
		return nil, fmt.Errorf("ReadArea: length must be > 0, got %d", length)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.connectIfNeededLocked(); err != nil {
		return nil, err
	}

	buf := make([]byte, length)
	var err error
	switch area {
	case S7AreaDB:
		if db <= 0 {
			return nil, fmt.Errorf("ReadArea: DB number must be > 0, got %d", db)
		}
		err = s.client.AGReadDB(db, start, length, buf)
	case S7AreaMK:
		err = s.client.AGReadMB(start, length, buf)
	case S7AreaPE:
		err = s.client.AGReadEB(start, length, buf)
	case S7AreaPA:
		err = s.client.AGReadAB(start, length, buf)
	default:
		return nil, fmt.Errorf("ReadArea: unsupported area 0x%02X (expected DB/M/I/Q)", area)
	}
	if err != nil {
		s.markFailedLocked(err)
		return nil, fmt.Errorf("AGRead area=0x%02X db=%d start=%d len=%d: %w", area, db, start, length, err)
	}
	s.markOkLocked()
	return buf, nil
}

// WriteArea writes one contiguous byte block to the PLC. Like ReadArea, the
// gos7 client splits the wire request by PDU internally.
//
// Writing to inputs (PE) is rejected here: it's not meaningful for the PE
// area on real CPUs (inputs reflect process state), even though the gos7
// AGWriteEB call exists. Most CPUs would refuse it anyway with a protocol
// error; rejecting client-side gives a clearer message.
func (s *S7PLC) WriteArea(area, db, start int, data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("WriteArea: data must not be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.connectIfNeededLocked(); err != nil {
		return err
	}

	var err error
	switch area {
	case S7AreaDB:
		if db <= 0 {
			return fmt.Errorf("WriteArea: DB number must be > 0, got %d", db)
		}
		err = s.client.AGWriteDB(db, start, len(data), data)
	case S7AreaMK:
		err = s.client.AGWriteMB(start, len(data), data)
	case S7AreaPA:
		err = s.client.AGWriteAB(start, len(data), data)
	case S7AreaPE:
		return fmt.Errorf("WriteArea: writing to inputs (I area) is not allowed")
	default:
		return fmt.Errorf("WriteArea: unsupported area 0x%02X (expected DB/M/Q)", area)
	}
	if err != nil {
		s.markFailedLocked(err)
		return fmt.Errorf("AGWrite area=0x%02X db=%d start=%d len=%d: %w", area, db, start, len(data), err)
	}
	s.markOkLocked()
	return nil
}

// ReadItems issues an S7 multi-read for a list of S7Item descriptors and
// returns one S7ReadResult per item, in the same order. Per-item PLC errors
// (e.g. one address points at a non-existent DB while others are fine) are
// surfaced via S7ReadResult.Err while the function still returns nil — only
// transport-level failures escalate to the function's error return.
//
// The split planner respects both the gos7 hard limit of 20 items per call
// and the negotiated PDU size, chunking automatically and concatenating the
// per-item results.
func (s *S7PLC) ReadItems(items []S7Item) ([]S7ReadResult, error) {
	if len(items) == 0 {
		return nil, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.connectIfNeededLocked(); err != nil {
		return nil, err
	}

	pdu := s.pduActual
	if pdu < 32 { // sanity floor; some test environments report 240+ but defaults exist
		pdu = s7DefaultPDU
	}
	chunks := splitItemsForMultiRead(items, pdu)

	results := make([]S7ReadResult, 0, len(items))
	for _, chunk := range chunks {
		gosItems := make([]gos7.S7DataItem, len(chunk))
		for i, it := range chunk {
			gosItems[i] = gos7.S7DataItem{
				Area:     it.Area,
				WordLen:  it.WordLen,
				DBNumber: it.DBNumber,
				Start:    it.Start,
				Bit:      it.Bit,
				Amount:   it.Amount,
				Data:     make([]byte, it.ByteSize()),
			}
		}
		if err := s.client.AGReadMulti(gosItems, len(gosItems)); err != nil {
			s.markFailedLocked(err)
			return nil, fmt.Errorf("AGReadMulti (%d items): %w", len(gosItems), err)
		}
		for i, it := range chunk {
			results = append(results, S7ReadResult{
				Item: it,
				Data: gosItems[i].Data,
				Err:  gosItems[i].Error,
			})
		}
	}
	s.markOkLocked()
	return results, nil
}

// WriteItems issues an S7 multi-write. Like ReadItems, per-item PLC errors
// surface via S7WriteResult.Err and only transport-level failures escalate
// to the function's error return. The caller must ensure
// `len(item.Data) == item.Item.ByteSize()` for each entry — the manager does
// not pad or truncate.
func (s *S7PLC) WriteItems(items []S7WriteItem) ([]S7WriteResult, error) {
	if len(items) == 0 {
		return nil, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.connectIfNeededLocked(); err != nil {
		return nil, err
	}

	pdu := s.pduActual
	if pdu < 32 {
		pdu = s7DefaultPDU
	}
	chunks := splitWriteItemsForMulti(items, pdu)

	results := make([]S7WriteResult, 0, len(items))
	for _, chunk := range chunks {
		gosItems := make([]gos7.S7DataItem, len(chunk))
		for i, w := range chunk {
			if len(w.Data) != w.Item.ByteSize() {
				return nil, fmt.Errorf("WriteItems: item %d: Data length %d != ByteSize %d for type %s",
					i, len(w.Data), w.Item.ByteSize(), w.Item.DataType)
			}
			gosItems[i] = gos7.S7DataItem{
				Area:     w.Item.Area,
				WordLen:  w.Item.WordLen,
				DBNumber: w.Item.DBNumber,
				Start:    w.Item.Start,
				Bit:      w.Item.Bit,
				Amount:   w.Item.Amount,
				Data:     w.Data,
			}
		}
		if err := s.client.AGWriteMulti(gosItems, len(gosItems)); err != nil {
			s.markFailedLocked(err)
			return nil, fmt.Errorf("AGWriteMulti (%d items): %w", len(gosItems), err)
		}
		for i, w := range chunk {
			results = append(results, S7WriteResult{
				Item: w.Item,
				Err:  gosItems[i].Error,
			})
		}
	}
	s.markOkLocked()
	return results, nil
}

// splitItemsForMultiRead chunks a list of items so each chunk fits both the
// gos7 hard limit (20 items / call) AND the negotiated PDU on both request
// (12 B per item descriptor + 19 B header) and response (4 B per result + the
// item's data + 21 B header) sides.
func splitItemsForMultiRead(items []S7Item, pdu int) [][]S7Item {
	requestCap := (pdu - s7MultiRequestHeader) / s7MultiPerItemReq
	if requestCap < 1 {
		requestCap = 1
	}
	if requestCap > s7MultiItemMaxCount {
		requestCap = s7MultiItemMaxCount
	}

	var chunks [][]S7Item
	current := make([]S7Item, 0, requestCap)
	currentRespSize := s7MultiResponseHeader
	for _, it := range items {
		itemRespSize := s7MultiPerItemRespHdr + it.ByteSize()
		flush := len(current) >= requestCap ||
			(len(current) > 0 && currentRespSize+itemRespSize > pdu)
		if flush {
			chunks = append(chunks, current)
			current = make([]S7Item, 0, requestCap)
			currentRespSize = s7MultiResponseHeader
		}
		current = append(current, it)
		currentRespSize += itemRespSize
	}
	if len(current) > 0 {
		chunks = append(chunks, current)
	}
	return chunks
}

// splitWriteItemsForMulti is the write-side analog. The wire layout for
// multi-writes is symmetric: each item's data (plus a 4-byte item header)
// goes into the request, and the response carries one ack byte per item.
func splitWriteItemsForMulti(items []S7WriteItem, pdu int) [][]S7WriteItem {
	requestCap := s7MultiItemMaxCount

	var chunks [][]S7WriteItem
	current := make([]S7WriteItem, 0, requestCap)
	currentReqSize := s7MultiRequestHeader
	for _, w := range items {
		itemReqSize := s7MultiPerItemReq + s7MultiPerItemRespHdr + w.Item.ByteSize()
		flush := len(current) >= requestCap ||
			(len(current) > 0 && currentReqSize+itemReqSize > pdu)
		if flush {
			chunks = append(chunks, current)
			current = make([]S7WriteItem, 0, requestCap)
			currentReqSize = s7MultiRequestHeader
		}
		current = append(current, w)
		currentReqSize += itemReqSize
	}
	if len(current) > 0 {
		chunks = append(chunks, current)
	}
	return chunks
}

// isS7TransportError returns true if `err` looks like a wire-level / network
// failure (as opposed to an application-level S7 protocol error like "Item
// not available"). The classification is best-effort because gos7 returns
// most errors as plain errors with formatted strings; we recognise the
// connection-nil sentinel and bubble up net.Error values.
func isS7TransportError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "Connection to address") && strings.Contains(msg, "is null"):
		return true
	case strings.Contains(msg, "EOF"):
		return true
	case strings.Contains(msg, "broken pipe"):
		return true
	case strings.Contains(msg, "connection reset"):
		return true
	case strings.Contains(msg, "connection refused"):
		return true
	}
	return false
}

// S7TestConnectionInfo is the result of a one-shot Test-Connection probe.
// Empty strings in the CPU/order fields indicate the server didn't return
// the value (typical for the python-snap7 demo server, whose placeholder
// SZL responses leave these blank). The HTTP handler renders empty strings
// as "unknown" for the user.
type S7TestConnectionInfo struct {
	Address           string `json:"address"`
	NegotiatedPDUSize int    `json:"negotiatedPduSize"`
	CPUType           string `json:"cpuType,omitempty"`
	OrderCode         string `json:"orderCode,omitempty"`
	ModuleName        string `json:"moduleName,omitempty"`
	SerialNumber      string `json:"serialNumber,omitempty"`
}

// S7TestConnect builds a transient S7PLC from the supplied config, opens a
// short-lived connection, captures the negotiated PDU size and any reported
// CPU info, and closes. Used by the `/api/v1/s7/test-connection` REST
// endpoint — the analog of `OpcuaTestConnect`.
//
// The function does NOT touch the engine's deployed config registry; it's
// safe to call concurrently with a deployed PLC of the same ID.
//
// Note on ctx: gos7 does not accept a `context.Context` on its connect /
// CPU-info paths, so the caller's ctx only governs the wrapping HTTP
// timeout. The transport timeout configured on the PLC config still applies
// to the underlying calls.
func S7TestConnect(ctx context.Context, cfg flow.ConfigNode) (*S7TestConnectionInfo, error) {
	inst, err := NewS7PLC(cfg)
	if err != nil {
		return nil, err
	}
	plc := inst.(*S7PLC)

	if err := plc.handler.Connect(); err != nil {
		return nil, fmt.Errorf("connect %s: %w", plc.address(), err)
	}
	defer func() {
		// Best-effort close — we already have the data, no point bubbling a
		// teardown error up to the user.
		_ = plc.handler.Close()
	}()

	// Honour ctx cancellation from the HTTP layer: if it's already done by
	// the time we wrap up, return an error. The actual gos7 calls below run
	// synchronously and ignore ctx.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	info := &S7TestConnectionInfo{
		Address:           plc.address(),
		NegotiatedPDUSize: plc.handler.PDULength,
	}
	// gos7's GetCPUInfo / GetOrderCode parse SZL responses with hard-coded
	// byte offsets and don't validate the response length. The python-snap7
	// demo server returns truncated SZL replies that trigger a slice-bounds
	// panic inside gos7. We can't fix the lib in PR-8, so defensively
	// recover and leave the relevant fields empty (the handler later
	// substitutes "unknown" so the UI never shows a blank cell).
	safeCallCPUInfo(plc.client, info)
	safeCallOrderCode(plc.client, info)
	return info, nil
}

// safeCallCPUInfo runs GetCPUInfo behind a recover() so a stripped-SZL panic
// from a non-conforming server (or a malformed response) doesn't crash the
// HTTP handler. Real Siemens CPUs return the full SZL block and don't
// trigger this.
func safeCallCPUInfo(c gos7.Client, info *S7TestConnectionInfo) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Debug("s7-plc: GetCPUInfo panicked (server returned truncated SZL)", "recover", rec)
		}
	}()
	cpu, err := c.GetCPUInfo()
	if err != nil {
		return
	}
	info.CPUType = strings.TrimSpace(cpu.ModuleTypeName)
	info.ModuleName = strings.TrimSpace(cpu.ModuleName)
	info.SerialNumber = strings.TrimSpace(cpu.SerialNumber)
}

func safeCallOrderCode(c gos7.Client, info *S7TestConnectionInfo) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Debug("s7-plc: GetOrderCode panicked (server returned truncated SZL)", "recover", rec)
		}
	}()
	oc, err := c.GetOrderCode()
	if err != nil {
		return
	}
	info.OrderCode = strings.TrimSpace(oc.Code)
}

// setStatusLocked updates the current status and notifies all registered
// listeners. Caller must hold s.mu.
func (s *S7PLC) setStatusLocked(fill, text string) {
	if s.currentFill == fill && s.currentText == text {
		return
	}
	s.currentFill = fill
	s.currentText = text
	for _, fn := range s.statusFuncs {
		fn(fill, text)
	}
}
