// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package modbus

import (
	"github.com/loopzedev/loopze-edge/internal/nodes"
	"fmt"
	"log/slog"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// ModbusWriteNode publishes incoming flow messages onto a Modbus device.
// 1 input; 0 outputs by default, 1 output when emitAck=true.
//
// Every per-write parameter (fc, address, dataType, byteOrder, wordOrder,
// unitId) can be overridden via msg.<field>. The value to write is taken from
// msg.payload.
//
// Implements flow.ConfigProvider and flow.ErrorProvider.
type ModbusWriteNode struct {
	config       flow.NodeConfig
	nodes.BaseNode
	configLookup flow.ConfigLookupFunc
	errFn        flow.ErrorFunc

	serverID  string
	server    *ModbusServer
	unitID    byte
	fc        int
	address   uint16
	dataType  string
	byteOrder ByteOrder
	wordOrder WordOrder
	scale     float64
	offset    float64
	emitAck   bool
}

// NewModbusWriteNode is the NodeFactory for the modbus-write node type.
func NewModbusWriteNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &ModbusWriteNode{config: config}, nil
}

func (n *ModbusWriteNode) Init() error {
	props := n.config.Properties

	n.serverID, _ = props["server"].(string)
	if n.serverID == "" {
		return fmt.Errorf("modbus-write %s: no server configured", n.config.ID)
	}

	n.unitID = byte(nodes.IntVal(props, "unitId", 0))
	n.fc = nodes.IntVal(props, "fc", 16)
	if !isWriteFC(n.fc) {
		return fmt.Errorf("modbus-write %s: invalid function code %d (expected 5|6|15|16)", n.config.ID, n.fc)
	}

	n.address = uint16(nodes.IntVal(props, "address", 0))
	n.dataType, _ = props["dataType"].(string)
	if n.dataType == "" {
		n.dataType = "raw"
	}

	bo, _ := props["byteOrder"].(string)
	n.byteOrder = parseByteOrder(bo)
	wo, _ := props["wordOrder"].(string)
	n.wordOrder = parseWordOrder(wo)

	n.scale = nodes.FloatVal(props, "scale", 1)
	n.offset = nodes.FloatVal(props, "offset", 0)
	if n.scale == 0 {
		n.scale = 1
	}

	if v, ok := props["emitAck"].(bool); ok {
		n.emitAck = v
	}
	return nil
}

func (n *ModbusWriteNode) SetConfigLookup(fn flow.ConfigLookupFunc) { n.configLookup = fn }
func (n *ModbusWriteNode) SetError(fn flow.ErrorFunc)               { n.errFn = fn }

func (n *ModbusWriteNode) Start() error {
	server, err := nodes.ResolveConfigInstance[ModbusServer](n.configLookup, n.serverID, n.Status, nodes.ResolveConfigParams{
		NodeKind:   "modbus-write",
		NodeID:     n.config.ID,
		ConfigKind: "server",
		TypeLabel:  "a Modbus server",
	})
	if err != nil {
		return err
	}
	n.server = server
	n.server.RegisterStatusFunc(n.Status)

	slog.Info("modbus-write started",
		"node_id", n.config.ID, "fc", n.fc, "address", n.address, "dataType", n.dataType)
	return nil
}

func (n *ModbusWriteNode) Stop() error {
	slog.Info("modbus-write stopped", "node_id", n.config.ID)
	return nil
}

func (n *ModbusWriteNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	p := n.effectiveParams(msg)

	unitID := p.unitID
	if unitID == 0 {
		unitID = n.server.DefaultUnitID()
	}

	payload := msg.Payload()

	written, err := n.dispatchWrite(unitID, p, payload)
	if err != nil {
		return nil, fmt.Errorf("modbus-write %s: %w", n.config.ID, err)
	}

	if !n.emitAck {
		return nil, nil
	}

	out := flow.NewMessage()
	out.SetPayload(true)
	out.Set("modbus", map[string]any{
		"fc":       p.fc,
		"address":  int(p.address),
		"quantity": writtenLen(written),
		"dataType": p.dataType,
		"unitId":   int(unitID),
		"written":  written,
	})
	return [][]*flow.Message{{out}}, nil
}

// writeParams holds resolved per-message parameters. Same shape as readParams
// but with the write-side FC range.
type writeParams struct {
	unitID    byte
	fc        int
	address   uint16
	dataType  string
	byteOrder ByteOrder
	wordOrder WordOrder
}

func (n *ModbusWriteNode) effectiveParams(msg *flow.Message) writeParams {
	p := writeParams{
		unitID:    n.unitID,
		fc:        n.fc,
		address:   n.address,
		dataType:  n.dataType,
		byteOrder: n.byteOrder,
		wordOrder: n.wordOrder,
	}
	if msg == nil {
		return p
	}
	if v, ok := nodes.ReadPositiveInt(msg.Get("unitId")); ok && v <= 0xFF {
		p.unitID = byte(v)
	}
	if v, ok := nodes.ReadPositiveInt(msg.Get("fc")); ok && isWriteFC(v) {
		p.fc = v
	}
	if v, ok := nodes.ReadPositiveInt(msg.Get("address")); ok && v <= 0xFFFF {
		p.address = uint16(v)
	} else if v, ok := msg.Get("address").(float64); ok && v >= 0 && v <= 0xFFFF {
		p.address = uint16(v)
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

// dispatchWrite encodes the payload, calls the appropriate server method, and
// returns the wire-level data that was written (for emitAck metadata).
func (n *ModbusWriteNode) dispatchWrite(unitID byte, p writeParams, payload any) (any, error) {
	switch p.fc {
	case 5: // Write Single Coil
		bools, err := nodes.ToBoolSlice(payload)
		if err != nil {
			return nil, fmt.Errorf("FC5 expects bool: %w", err)
		}
		if len(bools) != 1 {
			return nil, fmt.Errorf("FC5 expects single bool, got %d values", len(bools))
		}
		if err := n.server.WriteSingleCoil(unitID, p.address, bools[0]); err != nil {
			n.Status("red", err.Error())
			return nil, err
		}
		return bools[0], nil

	case 6: // Write Single Register
		// Apply inverse scaling if configured, then encode as a single uint16.
		v, err := nodes.UnapplyScale(payload, n.scale, n.offset)
		if err != nil {
			// If scaling is identity, fall through with raw payload.
			if n.scale != 1 || n.offset != 0 {
				return nil, fmt.Errorf("FC6 scaling: %w", err)
			}
		} else if n.scale != 1 || n.offset != 0 {
			payload = v
		}
		regs, err := EncodeRegisters("uint16", p.byteOrder, WordOrderBig, payload)
		if err != nil {
			return nil, fmt.Errorf("FC6 encode: %w", err)
		}
		if err := n.server.WriteSingleRegister(unitID, p.address, regs[0]); err != nil {
			n.Status("red", err.Error())
			return nil, err
		}
		return []int{int(regs[0])}, nil

	case 15: // Write Multiple Coils
		packed, qty, err := EncodeCoils(payload)
		if err != nil {
			return nil, fmt.Errorf("FC15 encode: %w", err)
		}
		if err := n.server.WriteMultipleCoils(unitID, p.address, packed, uint16(qty)); err != nil {
			n.Status("red", err.Error())
			return nil, err
		}
		bools, _ := nodes.ToBoolSlice(payload)
		return bools, nil

	case 16: // Write Multiple Registers
		// Apply inverse scaling on scalar numeric types only.
		if n.scale != 1 || n.offset != 0 {
			if v, err := nodes.UnapplyScale(payload, n.scale, n.offset); err == nil {
				payload = v
			}
		}
		regs, err := EncodeRegisters(p.dataType, p.byteOrder, p.wordOrder, payload)
		if err != nil {
			return nil, fmt.Errorf("FC16 encode: %w", err)
		}
		if err := n.server.WriteMultipleRegisters(unitID, p.address, regs); err != nil {
			n.Status("red", err.Error())
			return nil, err
		}
		out := make([]int, len(regs))
		for i, r := range regs {
			out[i] = int(r)
		}
		return out, nil

	default:
		return nil, fmt.Errorf("unsupported function code %d", p.fc)
	}
}

func isWriteFC(fc int) bool {
	return fc == 5 || fc == 6 || fc == 15 || fc == 16
}

// writtenLen returns the count of items in the "written" field of an ACK
// message — used purely for human-readable metadata.
func writtenLen(v any) int {
	switch x := v.(type) {
	case []int:
		return len(x)
	case []bool:
		return len(x)
	case bool:
		return 1
	default:
		return 0
	}
}

// ModbusWriteTypeInfo returns the node type metadata for the palette.
func ModbusWriteTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "modbus-write",
		Category:    "industrial",
		Label:       "Modbus Write",
		Description: "Writes coils or holding registers to a Modbus device",
		Icon:        "memory",
		Defaults: map[string]any{
			"server":    "",
			"unitId":    0,
			"fc":        16,
			"address":   0,
			"dataType":  "raw",
			"byteOrder": "bigEndian",
			"wordOrder": "bigEndian",
			"scale":     1,
			"offset":    0,
			"emitAck":   false,
		},
		Inputs:  1,
		Outputs: 0,
	}
}
