// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package nodes

import (
	"fmt"

	"github.com/dop251/goja"
	"github.com/niceclouds/flint/internal/buffer"
)

// registerBuffer adds the Buffer global to the Goja VM, providing a
// Node.js-compatible Buffer API for byte manipulation in Function Nodes.
//
// JS API:
//
//	Buffer.alloc(size)                  — create zero-filled buffer
//	Buffer.from(array)                  — from byte array [0x48, 0x65]
//	Buffer.from(string)                 — from UTF-8 string
//	Buffer.from(string, "hex")          — from hex string
//	Buffer.from(string, "base64")       — from base64 string
//	Buffer.concat([buf1, buf2])         — join buffers
//	buf.readUInt8(offset), buf.readInt16BE(offset), buf.readFloatBE(offset), ...
//	buf.writeUInt8(value, offset), buf.writeInt16BE(value, offset), ...
//	buf.swap16(), buf.swap32(), buf.swap64()
//	buf.toString(), buf.toString("hex"), buf.toString("base64")
//	buf.toJSON(), buf.slice(start, end), buf.copy(target, ...)
//	buf.length
func (n *FunctionNode) registerBuffer() {
	bufferObj := n.vm.NewObject()

	// ── Static methods ──────────────────────────────────────────────

	// Buffer.alloc(size)
	_ = bufferObj.Set("alloc", func(call goja.FunctionCall) goja.Value {
		size := int(call.Argument(0).ToInteger())
		if size < 0 {
			size = 0
		}
		return n.wrapBuffer(buffer.Alloc(size))
	})

	// Buffer.from(data, encoding?)
	_ = bufferObj.Set("from", func(call goja.FunctionCall) goja.Value {
		arg := call.Argument(0)
		encoding := ""
		if enc := call.Argument(1); !goja.IsUndefined(enc) && !goja.IsNull(enc) {
			encoding = enc.String()
		}

		exported := arg.Export()

		switch v := exported.(type) {
		case string:
			switch encoding {
			case "hex":
				buf, err := buffer.FromHex(v)
				if err != nil {
					panic(n.vm.NewGoError(err))
				}
				return n.wrapBuffer(buf)
			case "base64":
				buf, err := buffer.FromBase64(v)
				if err != nil {
					panic(n.vm.NewGoError(err))
				}
				return n.wrapBuffer(buf)
			default:
				return n.wrapBuffer(buffer.FromString(v))
			}

		case []interface{}:
			data := make([]byte, len(v))
			for i, item := range v {
				switch num := item.(type) {
				case int64:
					data[i] = byte(num)
				case float64:
					data[i] = byte(num)
				default:
					data[i] = 0
				}
			}
			return n.wrapBuffer(buffer.From(data))

		case []byte:
			return n.wrapBuffer(buffer.From(v))

		default:
			// Try to treat as array-like via the JS object.
			obj := arg.ToObject(n.vm)
			if obj == nil {
				panic(n.vm.NewGoError(fmt.Errorf("Buffer.from: unsupported argument type")))
			}
			lengthVal := obj.Get("length")
			if goja.IsUndefined(lengthVal) || goja.IsNull(lengthVal) {
				panic(n.vm.NewGoError(fmt.Errorf("Buffer.from: argument has no length")))
			}
			length := int(lengthVal.ToInteger())
			data := make([]byte, length)
			for i := 0; i < length; i++ {
				val := obj.Get(fmt.Sprintf("%d", i))
				if !goja.IsUndefined(val) && !goja.IsNull(val) {
					data[i] = byte(val.ToInteger())
				}
			}
			return n.wrapBuffer(buffer.From(data))
		}
	})

	// Buffer.concat(list)
	_ = bufferObj.Set("concat", func(call goja.FunctionCall) goja.Value {
		arg := call.Argument(0).Export()
		arr, ok := arg.([]interface{})
		if !ok {
			return n.wrapBuffer(buffer.Alloc(0))
		}
		bufs := make([]*buffer.Buffer, 0, len(arr))
		for _, item := range arr {
			if m, ok := item.(map[string]interface{}); ok {
				if raw, ok := m["__bufferData"].([]byte); ok {
					bufs = append(bufs, buffer.From(raw))
				}
			}
		}
		return n.wrapBuffer(buffer.Concat(bufs...))
	})

	_ = n.vm.Set("Buffer", bufferObj)
}

// wrapBuffer creates a Goja JS object that wraps a Go Buffer instance,
// exposing all read/write/swap/convert methods.
func (n *FunctionNode) wrapBuffer(buf *buffer.Buffer) goja.Value {
	obj := n.vm.NewObject()

	// Store raw data for Buffer.concat to access.
	_ = obj.Set("__bufferData", buf.Bytes())

	// length property
	_ = obj.Set("length", buf.Length())

	// ── Read — Integer ──────────────────────────────────────────────

	_ = obj.Set("readUInt8", func(call goja.FunctionCall) goja.Value {
		v, err := buf.ReadUInt8(optOffset(call, 0))
		if err != nil {
			panic(n.vm.NewGoError(err))
		}
		return n.vm.ToValue(int64(v))
	})

	_ = obj.Set("readInt8", func(call goja.FunctionCall) goja.Value {
		v, err := buf.ReadInt8(optOffset(call, 0))
		if err != nil {
			panic(n.vm.NewGoError(err))
		}
		return n.vm.ToValue(int64(v))
	})

	_ = obj.Set("readUInt16BE", func(call goja.FunctionCall) goja.Value {
		v, err := buf.ReadUInt16BE(optOffset(call, 0))
		if err != nil {
			panic(n.vm.NewGoError(err))
		}
		return n.vm.ToValue(int64(v))
	})

	_ = obj.Set("readUInt16LE", func(call goja.FunctionCall) goja.Value {
		v, err := buf.ReadUInt16LE(optOffset(call, 0))
		if err != nil {
			panic(n.vm.NewGoError(err))
		}
		return n.vm.ToValue(int64(v))
	})

	_ = obj.Set("readInt16BE", func(call goja.FunctionCall) goja.Value {
		v, err := buf.ReadInt16BE(optOffset(call, 0))
		if err != nil {
			panic(n.vm.NewGoError(err))
		}
		return n.vm.ToValue(int64(v))
	})

	_ = obj.Set("readInt16LE", func(call goja.FunctionCall) goja.Value {
		v, err := buf.ReadInt16LE(optOffset(call, 0))
		if err != nil {
			panic(n.vm.NewGoError(err))
		}
		return n.vm.ToValue(int64(v))
	})

	_ = obj.Set("readUInt32BE", func(call goja.FunctionCall) goja.Value {
		v, err := buf.ReadUInt32BE(optOffset(call, 0))
		if err != nil {
			panic(n.vm.NewGoError(err))
		}
		return n.vm.ToValue(int64(v))
	})

	_ = obj.Set("readUInt32LE", func(call goja.FunctionCall) goja.Value {
		v, err := buf.ReadUInt32LE(optOffset(call, 0))
		if err != nil {
			panic(n.vm.NewGoError(err))
		}
		return n.vm.ToValue(int64(v))
	})

	_ = obj.Set("readInt32BE", func(call goja.FunctionCall) goja.Value {
		v, err := buf.ReadInt32BE(optOffset(call, 0))
		if err != nil {
			panic(n.vm.NewGoError(err))
		}
		return n.vm.ToValue(int64(v))
	})

	_ = obj.Set("readInt32LE", func(call goja.FunctionCall) goja.Value {
		v, err := buf.ReadInt32LE(optOffset(call, 0))
		if err != nil {
			panic(n.vm.NewGoError(err))
		}
		return n.vm.ToValue(int64(v))
	})

	_ = obj.Set("readBigUInt64BE", func(call goja.FunctionCall) goja.Value {
		v, err := buf.ReadBigUInt64BE(optOffset(call, 0))
		if err != nil {
			panic(n.vm.NewGoError(err))
		}
		return n.vm.ToValue(float64(v))
	})

	_ = obj.Set("readBigUInt64LE", func(call goja.FunctionCall) goja.Value {
		v, err := buf.ReadBigUInt64LE(optOffset(call, 0))
		if err != nil {
			panic(n.vm.NewGoError(err))
		}
		return n.vm.ToValue(float64(v))
	})

	_ = obj.Set("readBigInt64BE", func(call goja.FunctionCall) goja.Value {
		v, err := buf.ReadBigInt64BE(optOffset(call, 0))
		if err != nil {
			panic(n.vm.NewGoError(err))
		}
		return n.vm.ToValue(float64(v))
	})

	_ = obj.Set("readBigInt64LE", func(call goja.FunctionCall) goja.Value {
		v, err := buf.ReadBigInt64LE(optOffset(call, 0))
		if err != nil {
			panic(n.vm.NewGoError(err))
		}
		return n.vm.ToValue(float64(v))
	})

	_ = obj.Set("readUIntBE", func(call goja.FunctionCall) goja.Value {
		offset := int(call.Argument(0).ToInteger())
		byteLen := int(call.Argument(1).ToInteger())
		v, err := buf.ReadUIntBE(offset, byteLen)
		if err != nil {
			panic(n.vm.NewGoError(err))
		}
		return n.vm.ToValue(float64(v))
	})

	_ = obj.Set("readUIntLE", func(call goja.FunctionCall) goja.Value {
		offset := int(call.Argument(0).ToInteger())
		byteLen := int(call.Argument(1).ToInteger())
		v, err := buf.ReadUIntLE(offset, byteLen)
		if err != nil {
			panic(n.vm.NewGoError(err))
		}
		return n.vm.ToValue(float64(v))
	})

	_ = obj.Set("readIntBE", func(call goja.FunctionCall) goja.Value {
		offset := int(call.Argument(0).ToInteger())
		byteLen := int(call.Argument(1).ToInteger())
		v, err := buf.ReadIntBE(offset, byteLen)
		if err != nil {
			panic(n.vm.NewGoError(err))
		}
		return n.vm.ToValue(float64(v))
	})

	_ = obj.Set("readIntLE", func(call goja.FunctionCall) goja.Value {
		offset := int(call.Argument(0).ToInteger())
		byteLen := int(call.Argument(1).ToInteger())
		v, err := buf.ReadIntLE(offset, byteLen)
		if err != nil {
			panic(n.vm.NewGoError(err))
		}
		return n.vm.ToValue(float64(v))
	})

	// ── Read — Float ────────────────────────────────────────────────

	_ = obj.Set("readFloatBE", func(call goja.FunctionCall) goja.Value {
		v, err := buf.ReadFloatBE(optOffset(call, 0))
		if err != nil {
			panic(n.vm.NewGoError(err))
		}
		return n.vm.ToValue(float64(v))
	})

	_ = obj.Set("readFloatLE", func(call goja.FunctionCall) goja.Value {
		v, err := buf.ReadFloatLE(optOffset(call, 0))
		if err != nil {
			panic(n.vm.NewGoError(err))
		}
		return n.vm.ToValue(float64(v))
	})

	_ = obj.Set("readDoubleBE", func(call goja.FunctionCall) goja.Value {
		v, err := buf.ReadDoubleBE(optOffset(call, 0))
		if err != nil {
			panic(n.vm.NewGoError(err))
		}
		return n.vm.ToValue(v)
	})

	_ = obj.Set("readDoubleLE", func(call goja.FunctionCall) goja.Value {
		v, err := buf.ReadDoubleLE(optOffset(call, 0))
		if err != nil {
			panic(n.vm.NewGoError(err))
		}
		return n.vm.ToValue(v)
	})

	// ── Write — Integer ─────────────────────────────────────────────

	_ = obj.Set("writeUInt8", n.writeMethod(buf, 1, func(v int64, o int) error { return buf.WriteUInt8(uint8(v), o) }))
	_ = obj.Set("writeInt8", n.writeMethod(buf, 1, func(v int64, o int) error { return buf.WriteInt8(int8(v), o) }))
	_ = obj.Set("writeUInt16BE", n.writeMethod(buf, 2, func(v int64, o int) error { return buf.WriteUInt16BE(uint16(v), o) }))
	_ = obj.Set("writeUInt16LE", n.writeMethod(buf, 2, func(v int64, o int) error { return buf.WriteUInt16LE(uint16(v), o) }))
	_ = obj.Set("writeInt16BE", n.writeMethod(buf, 2, func(v int64, o int) error { return buf.WriteInt16BE(int16(v), o) }))
	_ = obj.Set("writeInt16LE", n.writeMethod(buf, 2, func(v int64, o int) error { return buf.WriteInt16LE(int16(v), o) }))
	_ = obj.Set("writeUInt32BE", n.writeMethod(buf, 4, func(v int64, o int) error { return buf.WriteUInt32BE(uint32(v), o) }))
	_ = obj.Set("writeUInt32LE", n.writeMethod(buf, 4, func(v int64, o int) error { return buf.WriteUInt32LE(uint32(v), o) }))
	_ = obj.Set("writeInt32BE", n.writeMethod(buf, 4, func(v int64, o int) error { return buf.WriteInt32BE(int32(v), o) }))
	_ = obj.Set("writeInt32LE", n.writeMethod(buf, 4, func(v int64, o int) error { return buf.WriteInt32LE(int32(v), o) }))
	_ = obj.Set("writeBigUInt64BE", n.writeMethod(buf, 8, func(v int64, o int) error { return buf.WriteBigUInt64BE(uint64(v), o) }))
	_ = obj.Set("writeBigUInt64LE", n.writeMethod(buf, 8, func(v int64, o int) error { return buf.WriteBigUInt64LE(uint64(v), o) }))
	_ = obj.Set("writeBigInt64BE", n.writeMethod(buf, 8, func(v int64, o int) error { return buf.WriteBigInt64BE(v, o) }))
	_ = obj.Set("writeBigInt64LE", n.writeMethod(buf, 8, func(v int64, o int) error { return buf.WriteBigInt64LE(v, o) }))

	// Variable-length write: writeUIntBE(value, offset, byteLength)
	_ = obj.Set("writeUIntBE", func(call goja.FunctionCall) goja.Value {
		value := uint64(call.Argument(0).ToInteger())
		offset := int(call.Argument(1).ToInteger())
		byteLen := int(call.Argument(2).ToInteger())
		if err := buf.WriteUIntBE(value, offset, byteLen); err != nil {
			panic(n.vm.NewGoError(err))
		}
		return goja.Undefined()
	})

	_ = obj.Set("writeUIntLE", func(call goja.FunctionCall) goja.Value {
		value := uint64(call.Argument(0).ToInteger())
		offset := int(call.Argument(1).ToInteger())
		byteLen := int(call.Argument(2).ToInteger())
		if err := buf.WriteUIntLE(value, offset, byteLen); err != nil {
			panic(n.vm.NewGoError(err))
		}
		return goja.Undefined()
	})

	_ = obj.Set("writeIntBE", func(call goja.FunctionCall) goja.Value {
		value := call.Argument(0).ToInteger()
		offset := int(call.Argument(1).ToInteger())
		byteLen := int(call.Argument(2).ToInteger())
		if err := buf.WriteIntBE(value, offset, byteLen); err != nil {
			panic(n.vm.NewGoError(err))
		}
		return goja.Undefined()
	})

	_ = obj.Set("writeIntLE", func(call goja.FunctionCall) goja.Value {
		value := call.Argument(0).ToInteger()
		offset := int(call.Argument(1).ToInteger())
		byteLen := int(call.Argument(2).ToInteger())
		if err := buf.WriteIntLE(value, offset, byteLen); err != nil {
			panic(n.vm.NewGoError(err))
		}
		return goja.Undefined()
	})

	// ── Write — Float ───────────────────────────────────────────────

	_ = obj.Set("writeFloatBE", func(call goja.FunctionCall) goja.Value {
		if err := buf.WriteFloatBE(float32(call.Argument(0).ToFloat()), optOffset(call, 1)); err != nil {
			panic(n.vm.NewGoError(err))
		}
		return goja.Undefined()
	})

	_ = obj.Set("writeFloatLE", func(call goja.FunctionCall) goja.Value {
		if err := buf.WriteFloatLE(float32(call.Argument(0).ToFloat()), optOffset(call, 1)); err != nil {
			panic(n.vm.NewGoError(err))
		}
		return goja.Undefined()
	})

	_ = obj.Set("writeDoubleBE", func(call goja.FunctionCall) goja.Value {
		if err := buf.WriteDoubleBE(call.Argument(0).ToFloat(), optOffset(call, 1)); err != nil {
			panic(n.vm.NewGoError(err))
		}
		return goja.Undefined()
	})

	_ = obj.Set("writeDoubleLE", func(call goja.FunctionCall) goja.Value {
		if err := buf.WriteDoubleLE(call.Argument(0).ToFloat(), optOffset(call, 1)); err != nil {
			panic(n.vm.NewGoError(err))
		}
		return goja.Undefined()
	})

	// ── Swap ────────────────────────────────────────────────────────

	_ = obj.Set("swap16", func(call goja.FunctionCall) goja.Value {
		if err := buf.Swap16(); err != nil {
			panic(n.vm.NewGoError(err))
		}
		return obj
	})

	_ = obj.Set("swap32", func(call goja.FunctionCall) goja.Value {
		if err := buf.Swap32(); err != nil {
			panic(n.vm.NewGoError(err))
		}
		return obj
	})

	_ = obj.Set("swap64", func(call goja.FunctionCall) goja.Value {
		if err := buf.Swap64(); err != nil {
			panic(n.vm.NewGoError(err))
		}
		return obj
	})

	// ── Conversion ──────────────────────────────────────────────────

	_ = obj.Set("toString", func(call goja.FunctionCall) goja.Value {
		encoding := ""
		if enc := call.Argument(0); !goja.IsUndefined(enc) && !goja.IsNull(enc) {
			encoding = enc.String()
		}
		switch encoding {
		case "hex":
			return n.vm.ToValue(buf.ToHex())
		case "base64":
			return n.vm.ToValue(buf.ToBase64())
		default:
			return n.vm.ToValue(buf.ToString())
		}
	})

	_ = obj.Set("toJSON", func(call goja.FunctionCall) goja.Value {
		return n.vm.ToValue(buf.ToJSON())
	})

	_ = obj.Set("slice", func(call goja.FunctionCall) goja.Value {
		start := int(call.Argument(0).ToInteger())
		end := buf.Length()
		if arg1 := call.Argument(1); !goja.IsUndefined(arg1) && !goja.IsNull(arg1) {
			end = int(arg1.ToInteger())
		}
		return n.wrapBuffer(buf.Slice(start, end))
	})

	_ = obj.Set("copy", func(call goja.FunctionCall) goja.Value {
		targetObj := call.Argument(0).ToObject(n.vm)
		if targetObj == nil {
			panic(n.vm.NewGoError(fmt.Errorf("buffer.copy: target is not a buffer")))
		}
		targetData, ok := targetObj.Get("__bufferData").Export().([]byte)
		if !ok {
			panic(n.vm.NewGoError(fmt.Errorf("buffer.copy: target is not a buffer")))
		}
		targetBuf := buffer.From(targetData)
		targetStart := optOffset(call, 1)
		sourceStart := optOffset(call, 2)
		sourceEnd := buf.Length()
		if arg3 := call.Argument(3); !goja.IsUndefined(arg3) && !goja.IsNull(arg3) {
			sourceEnd = int(arg3.ToInteger())
		}
		copied := buf.Copy(targetBuf, targetStart, sourceStart, sourceEnd)
		// Write back to the target object's backing data.
		copy(targetData, targetBuf.Bytes())
		return n.vm.ToValue(copied)
	})

	return obj
}

// writeMethod creates a Goja function for write operations with signature: write(value, offset?).
func (n *FunctionNode) writeMethod(_ *buffer.Buffer, _ int, fn func(int64, int) error) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		value := call.Argument(0).ToInteger()
		offset := optOffset(call, 1)
		if err := fn(value, offset); err != nil {
			panic(n.vm.NewGoError(err))
		}
		return goja.Undefined()
	}
}

// optOffset extracts an optional integer offset from a Goja function call argument.
// Returns 0 if the argument is undefined or null.
func optOffset(call goja.FunctionCall, argIdx int) int {
	arg := call.Argument(argIdx)
	if goja.IsUndefined(arg) || goja.IsNull(arg) {
		return 0
	}
	return int(arg.ToInteger())
}
