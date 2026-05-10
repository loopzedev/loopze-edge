// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"time"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// ModbusReadNode reads from a Modbus device. Two modes:
//
//   - "static" (default): polls the configured address at pollInterval; 0 inputs.
//   - "dynamic": triggered by an incoming message; 1 input. Every per-read
//     parameter (fc, address, quantity, dataType, byteOrder, wordOrder, unitId)
//     can be overridden via msg.<field>.
//
// Implements flow.ConfigProvider and flow.ErrorProvider.
type ModbusReadNode struct {
	config       flow.NodeConfig
	BaseNode
	configLookup flow.ConfigLookupFunc
	errFn        flow.ErrorFunc

	mode      string
	serverID  string
	server    *ModbusServer
	unitID    byte // 0 = use server default
	fc        int
	address   uint16
	quantity  int
	dataType  string
	byteOrder ByteOrder
	wordOrder WordOrder
	scale     float64
	offset    float64

	pollInterval time.Duration
	emitOnChange bool
	emitOnError  bool

	mu          sync.Mutex
	lastPayload any
	done        chan struct{}
	wg          sync.WaitGroup
}

// NewModbusReadNode is the NodeFactory for the modbus-read node type.
func NewModbusReadNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &ModbusReadNode{
		config: config,
		done:   make(chan struct{}),
	}, nil
}

func (n *ModbusReadNode) Init() error {
	props := n.config.Properties

	n.serverID, _ = props["server"].(string)
	if n.serverID == "" {
		return fmt.Errorf("modbus-read %s: no server configured", n.config.ID)
	}

	mode, _ := props["mode"].(string)
	if mode == "" {
		mode = "static"
	}
	if mode != "static" && mode != "dynamic" {
		return fmt.Errorf("modbus-read %s: invalid mode %q (expected static|dynamic)", n.config.ID, mode)
	}
	n.mode = mode

	n.unitID = byte(IntVal(props, "unitId", 0))

	n.fc = IntVal(props, "fc", 3)
	if n.fc < 1 || n.fc > 4 {
		return fmt.Errorf("modbus-read %s: invalid function code %d (expected 1|2|3|4)", n.config.ID, n.fc)
	}

	n.address = uint16(IntVal(props, "address", 0))
	n.quantity = IntVal(props, "quantity", 1)
	if n.quantity < 1 {
		n.quantity = 1
	}

	n.dataType, _ = props["dataType"].(string)
	if n.dataType == "" {
		n.dataType = "raw"
	}

	if n.dataType == "bool" && (n.fc != 1 && n.fc != 2) {
		return fmt.Errorf("modbus-read %s: dataType=bool requires FC1 or FC2", n.config.ID)
	}

	bo, _ := props["byteOrder"].(string)
	n.byteOrder = parseByteOrder(bo)
	wo, _ := props["wordOrder"].(string)
	n.wordOrder = parseWordOrder(wo)

	n.scale = FloatVal(props, "scale", 1)
	n.offset = FloatVal(props, "offset", 0)
	if n.scale == 0 {
		n.scale = 1
	}

	if v, ok := props["pollInterval"].(float64); ok && v > 0 {
		n.pollInterval = time.Duration(v) * time.Millisecond
	} else {
		n.pollInterval = time.Second
	}

	if v, ok := props["emitOnChange"].(bool); ok {
		n.emitOnChange = v
	}
	if v, ok := props["emitOnError"].(bool); ok {
		n.emitOnError = v
	}

	return nil
}

func (n *ModbusReadNode) SetConfigLookup(fn flow.ConfigLookupFunc) { n.configLookup = fn }
func (n *ModbusReadNode) SetError(fn flow.ErrorFunc)               { n.errFn = fn }

func (n *ModbusReadNode) Start() error {
	server, err := ResolveConfigInstance[ModbusServer](n.configLookup, n.serverID, n.Status, ResolveConfigParams{
		NodeKind:   "modbus-read",
		NodeID:     n.config.ID,
		ConfigKind: "server",
		TypeLabel:  "a Modbus server",
	})
	if err != nil {
		return err
	}
	n.server = server
	n.server.RegisterStatusFunc(n.handleServerStatus)

	if n.mode == "static" {
		n.wg.Add(1)
		go n.pollLoop()
		slog.Info("modbus-read started (static)",
			"node_id", n.config.ID, "fc", n.fc, "address", n.address,
			"quantity", n.quantity, "dataType", n.dataType, "interval", n.pollInterval)
	} else {
		slog.Info("modbus-read started (dynamic, idle)", "node_id", n.config.ID)
	}
	return nil
}

func (n *ModbusReadNode) Stop() error {
	select {
	case <-n.done:
		// already closed
	default:
		close(n.done)
	}
	n.wg.Wait()
	slog.Info("modbus-read stopped", "node_id", n.config.ID)
	return nil
}

// HandleMessage in dynamic mode triggers a read using effective parameters
// (config defaults overridden by msg.<field>). The incoming message itself is
// not forwarded — the output carries only the read result.
func (n *ModbusReadNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if n.mode != "dynamic" {
		return nil, nil
	}

	params := n.effectiveParams(msg)
	out, err := n.doRead(params)
	if err != nil {
		return nil, err // engine forwards via errFn for catch-node integration
	}
	if out == nil {
		// emitOnChange suppressed an unchanged read — nothing to emit.
		return nil, nil
	}
	return [][]*flow.Message{{out}}, nil
}

// readParams holds the resolved parameters for a single read operation,
// after applying msg overrides on top of the configured defaults.
type readParams struct {
	unitID    byte
	fc        int
	address   uint16
	quantity  int
	dataType  string
	byteOrder ByteOrder
	wordOrder WordOrder
}

// effectiveParams overlays per-message overrides onto the configured defaults.
func (n *ModbusReadNode) effectiveParams(msg *flow.Message) readParams {
	p := readParams{
		unitID:    n.unitID,
		fc:        n.fc,
		address:   n.address,
		quantity:  n.quantity,
		dataType:  n.dataType,
		byteOrder: n.byteOrder,
		wordOrder: n.wordOrder,
	}
	if msg == nil {
		return p
	}
	if v, ok := ReadPositiveInt(msg.Get("unitId")); ok && v <= 0xFF {
		p.unitID = byte(v)
	}
	if v, ok := ReadPositiveInt(msg.Get("fc")); ok && v >= 1 && v <= 4 {
		p.fc = v
	}
	if v, ok := ReadPositiveInt(msg.Get("address")); ok && v <= 0xFFFF {
		p.address = uint16(v)
	} else if v, ok := msg.Get("address").(float64); ok && v >= 0 && v <= 0xFFFF {
		// Accept zero-valued address explicitly.
		p.address = uint16(v)
	}
	if v, ok := ReadPositiveInt(msg.Get("quantity")); ok && v >= 1 {
		p.quantity = v
	}
	if dt, ok := msg.Get("dataType").(string); ok && dt != "" {
		p.dataType = dt
	}
	if bo, ok := msg.Get("byteOrder").(string); ok && bo != "" {
		p.byteOrder = parseByteOrder(bo)
	}
	if wo, ok := msg.Get("wordOrder").(string); ok && wo != "" {
		p.wordOrder = parseWordOrder(wo)
	}
	return p
}

// pollLoop is the static-mode polling goroutine. Runs until Stop().
func (n *ModbusReadNode) pollLoop() {
	defer n.wg.Done()
	ticker := time.NewTicker(n.pollInterval)
	defer ticker.Stop()

	// First read fires immediately so the user sees data without waiting a tick.
	n.tick()

	for {
		select {
		case <-n.done:
			return
		case <-ticker.C:
			n.tick()
		}
	}
}

// tick performs one read and dispatches the result.
func (n *ModbusReadNode) tick() {
	params := readParams{
		unitID:    n.unitID,
		fc:        n.fc,
		address:   n.address,
		quantity:  n.quantity,
		dataType:  n.dataType,
		byteOrder: n.byteOrder,
		wordOrder: n.wordOrder,
	}
	msg, err := n.doRead(params)
	if err != nil {
		// Forward to catch-node listeners.
		if n.errFn != nil {
			n.errFn(err, nil)
		}
		if n.emitOnError {
			out := flow.NewMessage()
			out.Set("error", err.Error())
			n.Send(0, out)
		}
		return
	}
	if msg != nil {
		n.Send(0, msg)
	}
}

// doRead executes a single read with the given parameters and returns the
// outgoing message (or nil when emitOnChange suppresses the emit). On error
// returns the wrapped exception/transport error.
func (n *ModbusReadNode) doRead(p readParams) (*flow.Message, error) {
	unitID := p.unitID
	if unitID == 0 {
		unitID = n.server.DefaultUnitID()
	}

	// For coil-related types, the wire-level quantity is the number of coils.
	// For register-types, derive register count from dataType (with raw/string
	// using the user-provided quantity).
	wireQty := p.quantity
	switch p.dataType {
	case "bool":
		wireQty = 1
	case "raw", "string":
		// keep p.quantity
	default:
		if p.fc == 3 || p.fc == 4 {
			wireQty = RegistersForType(p.dataType, p.quantity)
		}
	}
	if wireQty < 1 {
		wireQty = 1
	}

	raw, err := n.server.Read(unitID, p.fc, p.address, uint16(wireQty))
	if err != nil {
		n.Status("red", err.Error())
		return nil, fmt.Errorf("modbus-read %s: %w", n.config.ID, err)
	}

	var (
		decoded any
		regs    []uint16
	)
	if p.fc == 1 || p.fc == 2 {
		decoded, err = DecodeCoils(raw, wireQty, p.dataType)
	} else {
		regs = BytesToRegisters(raw)
		decoded, err = DecodeRegisters(p.dataType, p.byteOrder, p.wordOrder, regs)
	}
	if err != nil {
		return nil, fmt.Errorf("modbus-read %s: decode failed: %w", n.config.ID, err)
	}

	scaled := ApplyScale(decoded, n.scale, n.offset)

	if n.emitOnChange {
		n.mu.Lock()
		if reflect.DeepEqual(scaled, n.lastPayload) {
			n.mu.Unlock()
			return nil, nil
		}
		n.lastPayload = scaled
		n.mu.Unlock()
	}

	msg := flow.NewMessage()
	msg.SetPayload(scaled)
	msg.SetTopic(fmt.Sprintf("modbus/%s/%d", serverTopicSegment(n.server), p.address))

	meta := map[string]any{
		"fc":       p.fc,
		"address":  int(p.address),
		"quantity": wireQty,
		"dataType": p.dataType,
		"unitId":   int(unitID),
	}
	if regs != nil {
		// For register reads (FC3/FC4) we expose both representations next to
		// the typed payload:
		//
		//   msg.modbus.raw  → []int of uint16 register values (debugging)
		//   msg.bytes       → []int of uint8 wire bytes (Buffer.from-friendly)
		//
		// This way users can pick either representation in a Function Node
		// without writing register-to-byte conversion code by hand.
		rawArr := make([]int, len(regs))
		for i, r := range regs {
			rawArr[i] = int(r)
		}
		meta["raw"] = rawArr

		bytesArr := make([]int, len(raw))
		for i, b := range raw {
			bytesArr[i] = int(b)
		}
		msg.Set("bytes", bytesArr)
	}
	msg.Set("modbus", meta)

	return msg, nil
}

// handleServerStatus is invoked by the server config node whenever its
// connection state changes. We pass it through unchanged for the read node.
func (n *ModbusReadNode) handleServerStatus(fill, text string) {
	if n.Status == nil {
		return
	}
	if fill == "green" {
		n.Status("green", n.connectedStatus())
		return
	}
	n.Status(fill, text)
}

// connectedStatus is the green-state status text — describes the polling
// configuration for static mode and "idle" for dynamic.
func (n *ModbusReadNode) connectedStatus() string {
	if n.mode == "static" {
		return fmt.Sprintf("connected · %s", n.pollInterval)
	}
	return "connected · idle"
}

// serverTopicSegment returns a topic-safe segment for the server name (or its
// ID when the user hasn't named it).
func serverTopicSegment(s *ModbusServer) string {
	if s == nil {
		return "unknown"
	}
	if name := s.Name(); name != "" {
		return name
	}
	return s.id
}

// ModbusReadTypeInfo returns the node type metadata for the palette.
func ModbusReadTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "modbus-read",
		Category:    "industrial",
		Label:       "Modbus Read",
		Description: "Reads coils, discrete inputs, holding or input registers from a Modbus device",
		Icon:        "memory",
		Defaults: map[string]any{
			"server":       "",
			"mode":         "static",
			"unitId":       0,
			"fc":           3,
			"address":      0,
			"quantity":     1,
			"dataType":     "raw",
			"byteOrder":    "bigEndian",
			"wordOrder":    "bigEndian",
			"scale":        1,
			"offset":       0,
			"pollInterval": 1000,
			"emitOnChange": false,
			"emitOnError":  false,
		},
		Inputs:  0,
		Outputs: 1,
	}
}
