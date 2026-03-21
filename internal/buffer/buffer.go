// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

// Package buffer provides a byte buffer with typed read/write operations,
// compatible with the Node.js Buffer API. It is used both as a Go library
// by backend connector nodes (Modbus, S7, OPC-UA) and as the backing
// implementation for the Buffer JS global in Function Nodes.
package buffer

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
)

// Buffer wraps a byte slice with typed read/write operations and
// configurable endianness. All read/write methods perform bounds checking.
type Buffer struct {
	data []byte
}

// ── Constructors ────────────────────────────────────────────────────────────

// Alloc creates a zero-filled Buffer of the given size.
func Alloc(size int) *Buffer {
	return &Buffer{data: make([]byte, size)}
}

// From creates a Buffer backed by a copy of the given byte slice.
func From(data []byte) *Buffer {
	cp := make([]byte, len(data))
	copy(cp, data)
	return &Buffer{data: cp}
}

// FromString creates a Buffer from a UTF-8 string.
func FromString(s string) *Buffer {
	return &Buffer{data: []byte(s)}
}

// FromHex creates a Buffer from a hex-encoded string (e.g. "48656c6c6f").
func FromHex(h string) (*Buffer, error) {
	data, err := hex.DecodeString(h)
	if err != nil {
		return nil, fmt.Errorf("buffer: invalid hex string: %w", err)
	}
	return &Buffer{data: data}, nil
}

// FromBase64 creates a Buffer from a base64-encoded string.
func FromBase64(b64 string) (*Buffer, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("buffer: invalid base64 string: %w", err)
	}
	return &Buffer{data: data}, nil
}

// Concat creates a new Buffer by joining multiple buffers.
func Concat(buffers ...*Buffer) *Buffer {
	total := 0
	for _, b := range buffers {
		total += len(b.data)
	}
	result := make([]byte, 0, total)
	for _, b := range buffers {
		result = append(result, b.data...)
	}
	return &Buffer{data: result}
}

// ── Properties ──────────────────────────────────────────────────────────────

// Length returns the number of bytes in the buffer.
func (b *Buffer) Length() int { return len(b.data) }

// Bytes returns the underlying byte slice.
func (b *Buffer) Bytes() []byte { return b.data }

// ── Read — Integer ──────────────────────────────────────────────────────────

func (b *Buffer) ReadUInt8(offset int) (uint8, error) {
	if err := b.check(offset, 1); err != nil {
		return 0, err
	}
	return b.data[offset], nil
}

func (b *Buffer) ReadInt8(offset int) (int8, error) {
	if err := b.check(offset, 1); err != nil {
		return 0, err
	}
	return int8(b.data[offset]), nil
}

func (b *Buffer) ReadUInt16BE(offset int) (uint16, error) {
	if err := b.check(offset, 2); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(b.data[offset:]), nil
}

func (b *Buffer) ReadUInt16LE(offset int) (uint16, error) {
	if err := b.check(offset, 2); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(b.data[offset:]), nil
}

func (b *Buffer) ReadInt16BE(offset int) (int16, error) {
	v, err := b.ReadUInt16BE(offset)
	return int16(v), err
}

func (b *Buffer) ReadInt16LE(offset int) (int16, error) {
	v, err := b.ReadUInt16LE(offset)
	return int16(v), err
}

func (b *Buffer) ReadUInt32BE(offset int) (uint32, error) {
	if err := b.check(offset, 4); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(b.data[offset:]), nil
}

func (b *Buffer) ReadUInt32LE(offset int) (uint32, error) {
	if err := b.check(offset, 4); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(b.data[offset:]), nil
}

func (b *Buffer) ReadInt32BE(offset int) (int32, error) {
	v, err := b.ReadUInt32BE(offset)
	return int32(v), err
}

func (b *Buffer) ReadInt32LE(offset int) (int32, error) {
	v, err := b.ReadUInt32LE(offset)
	return int32(v), err
}

func (b *Buffer) ReadBigUInt64BE(offset int) (uint64, error) {
	if err := b.check(offset, 8); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(b.data[offset:]), nil
}

func (b *Buffer) ReadBigUInt64LE(offset int) (uint64, error) {
	if err := b.check(offset, 8); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(b.data[offset:]), nil
}

func (b *Buffer) ReadBigInt64BE(offset int) (int64, error) {
	v, err := b.ReadBigUInt64BE(offset)
	return int64(v), err
}

func (b *Buffer) ReadBigInt64LE(offset int) (int64, error) {
	v, err := b.ReadBigUInt64LE(offset)
	return int64(v), err
}

// ReadUIntBE reads an unsigned integer of 1-6 bytes in big-endian order.
func (b *Buffer) ReadUIntBE(offset, byteLength int) (uint64, error) {
	if byteLength < 1 || byteLength > 6 {
		return 0, fmt.Errorf("buffer: byteLength must be 1-6, got %d", byteLength)
	}
	if err := b.check(offset, byteLength); err != nil {
		return 0, err
	}
	var val uint64
	for i := 0; i < byteLength; i++ {
		val = (val << 8) | uint64(b.data[offset+i])
	}
	return val, nil
}

// ReadUIntLE reads an unsigned integer of 1-6 bytes in little-endian order.
func (b *Buffer) ReadUIntLE(offset, byteLength int) (uint64, error) {
	if byteLength < 1 || byteLength > 6 {
		return 0, fmt.Errorf("buffer: byteLength must be 1-6, got %d", byteLength)
	}
	if err := b.check(offset, byteLength); err != nil {
		return 0, err
	}
	var val uint64
	for i := byteLength - 1; i >= 0; i-- {
		val = (val << 8) | uint64(b.data[offset+i])
	}
	return val, nil
}

// ReadIntBE reads a signed integer of 1-6 bytes in big-endian order.
func (b *Buffer) ReadIntBE(offset, byteLength int) (int64, error) {
	v, err := b.ReadUIntBE(offset, byteLength)
	if err != nil {
		return 0, err
	}
	// Sign-extend: if the high bit is set, fill upper bits with 1s.
	if byteLength < 8 && v>>(uint(byteLength*8-1))&1 == 1 {
		v |= ^uint64(0) << uint(byteLength*8)
	}
	return int64(v), nil
}

// ReadIntLE reads a signed integer of 1-6 bytes in little-endian order.
func (b *Buffer) ReadIntLE(offset, byteLength int) (int64, error) {
	v, err := b.ReadUIntLE(offset, byteLength)
	if err != nil {
		return 0, err
	}
	if byteLength < 8 && v>>(uint(byteLength*8-1))&1 == 1 {
		v |= ^uint64(0) << uint(byteLength*8)
	}
	return int64(v), nil
}

// ── Read — Float ────────────────────────────────────────────────────────────

func (b *Buffer) ReadFloatBE(offset int) (float32, error) {
	v, err := b.ReadUInt32BE(offset)
	if err != nil {
		return 0, err
	}
	return math.Float32frombits(v), nil
}

func (b *Buffer) ReadFloatLE(offset int) (float32, error) {
	v, err := b.ReadUInt32LE(offset)
	if err != nil {
		return 0, err
	}
	return math.Float32frombits(v), nil
}

func (b *Buffer) ReadDoubleBE(offset int) (float64, error) {
	v, err := b.ReadBigUInt64BE(offset)
	if err != nil {
		return 0, err
	}
	return math.Float64frombits(v), nil
}

func (b *Buffer) ReadDoubleLE(offset int) (float64, error) {
	v, err := b.ReadBigUInt64LE(offset)
	if err != nil {
		return 0, err
	}
	return math.Float64frombits(v), nil
}

// ── Write — Integer ─────────────────────────────────────────────────────────

func (b *Buffer) WriteUInt8(value uint8, offset int) error {
	if err := b.check(offset, 1); err != nil {
		return err
	}
	b.data[offset] = value
	return nil
}

func (b *Buffer) WriteInt8(value int8, offset int) error {
	return b.WriteUInt8(uint8(value), offset)
}

func (b *Buffer) WriteUInt16BE(value uint16, offset int) error {
	if err := b.check(offset, 2); err != nil {
		return err
	}
	binary.BigEndian.PutUint16(b.data[offset:], value)
	return nil
}

func (b *Buffer) WriteUInt16LE(value uint16, offset int) error {
	if err := b.check(offset, 2); err != nil {
		return err
	}
	binary.LittleEndian.PutUint16(b.data[offset:], value)
	return nil
}

func (b *Buffer) WriteInt16BE(value int16, offset int) error {
	return b.WriteUInt16BE(uint16(value), offset)
}

func (b *Buffer) WriteInt16LE(value int16, offset int) error {
	return b.WriteUInt16LE(uint16(value), offset)
}

func (b *Buffer) WriteUInt32BE(value uint32, offset int) error {
	if err := b.check(offset, 4); err != nil {
		return err
	}
	binary.BigEndian.PutUint32(b.data[offset:], value)
	return nil
}

func (b *Buffer) WriteUInt32LE(value uint32, offset int) error {
	if err := b.check(offset, 4); err != nil {
		return err
	}
	binary.LittleEndian.PutUint32(b.data[offset:], value)
	return nil
}

func (b *Buffer) WriteInt32BE(value int32, offset int) error {
	return b.WriteUInt32BE(uint32(value), offset)
}

func (b *Buffer) WriteInt32LE(value int32, offset int) error {
	return b.WriteUInt32LE(uint32(value), offset)
}

func (b *Buffer) WriteBigUInt64BE(value uint64, offset int) error {
	if err := b.check(offset, 8); err != nil {
		return err
	}
	binary.BigEndian.PutUint64(b.data[offset:], value)
	return nil
}

func (b *Buffer) WriteBigUInt64LE(value uint64, offset int) error {
	if err := b.check(offset, 8); err != nil {
		return err
	}
	binary.LittleEndian.PutUint64(b.data[offset:], value)
	return nil
}

func (b *Buffer) WriteBigInt64BE(value int64, offset int) error {
	return b.WriteBigUInt64BE(uint64(value), offset)
}

func (b *Buffer) WriteBigInt64LE(value int64, offset int) error {
	return b.WriteBigUInt64LE(uint64(value), offset)
}

// WriteUIntBE writes an unsigned integer of 1-6 bytes in big-endian order.
func (b *Buffer) WriteUIntBE(value uint64, offset, byteLength int) error {
	if byteLength < 1 || byteLength > 6 {
		return fmt.Errorf("buffer: byteLength must be 1-6, got %d", byteLength)
	}
	if err := b.check(offset, byteLength); err != nil {
		return err
	}
	for i := byteLength - 1; i >= 0; i-- {
		b.data[offset+i] = byte(value & 0xFF)
		value >>= 8
	}
	return nil
}

// WriteUIntLE writes an unsigned integer of 1-6 bytes in little-endian order.
func (b *Buffer) WriteUIntLE(value uint64, offset, byteLength int) error {
	if byteLength < 1 || byteLength > 6 {
		return fmt.Errorf("buffer: byteLength must be 1-6, got %d", byteLength)
	}
	if err := b.check(offset, byteLength); err != nil {
		return err
	}
	for i := 0; i < byteLength; i++ {
		b.data[offset+i] = byte(value & 0xFF)
		value >>= 8
	}
	return nil
}

// WriteIntBE writes a signed integer of 1-6 bytes in big-endian order.
func (b *Buffer) WriteIntBE(value int64, offset, byteLength int) error {
	return b.WriteUIntBE(uint64(value), offset, byteLength)
}

// WriteIntLE writes a signed integer of 1-6 bytes in little-endian order.
func (b *Buffer) WriteIntLE(value int64, offset, byteLength int) error {
	return b.WriteUIntLE(uint64(value), offset, byteLength)
}

// ── Write — Float ───────────────────────────────────────────────────────────

func (b *Buffer) WriteFloatBE(value float32, offset int) error {
	return b.WriteUInt32BE(math.Float32bits(value), offset)
}

func (b *Buffer) WriteFloatLE(value float32, offset int) error {
	return b.WriteUInt32LE(math.Float32bits(value), offset)
}

func (b *Buffer) WriteDoubleBE(value float64, offset int) error {
	return b.WriteBigUInt64BE(math.Float64bits(value), offset)
}

func (b *Buffer) WriteDoubleLE(value float64, offset int) error {
	return b.WriteBigUInt64LE(math.Float64bits(value), offset)
}

// ── Swap ────────────────────────────────────────────────────────────────────

// Swap16 swaps byte order in 16-bit pairs. Buffer length must be even.
func (b *Buffer) Swap16() error {
	if len(b.data)%2 != 0 {
		return fmt.Errorf("buffer: length %d is not a multiple of 2", len(b.data))
	}
	for i := 0; i < len(b.data); i += 2 {
		b.data[i], b.data[i+1] = b.data[i+1], b.data[i]
	}
	return nil
}

// Swap32 swaps byte order in 32-bit groups. Buffer length must be divisible by 4.
func (b *Buffer) Swap32() error {
	if len(b.data)%4 != 0 {
		return fmt.Errorf("buffer: length %d is not a multiple of 4", len(b.data))
	}
	for i := 0; i < len(b.data); i += 4 {
		b.data[i], b.data[i+3] = b.data[i+3], b.data[i]
		b.data[i+1], b.data[i+2] = b.data[i+2], b.data[i+1]
	}
	return nil
}

// Swap64 swaps byte order in 64-bit groups. Buffer length must be divisible by 8.
func (b *Buffer) Swap64() error {
	if len(b.data)%8 != 0 {
		return fmt.Errorf("buffer: length %d is not a multiple of 8", len(b.data))
	}
	for i := 0; i < len(b.data); i += 8 {
		b.data[i], b.data[i+7] = b.data[i+7], b.data[i]
		b.data[i+1], b.data[i+6] = b.data[i+6], b.data[i+1]
		b.data[i+2], b.data[i+5] = b.data[i+5], b.data[i+2]
		b.data[i+3], b.data[i+4] = b.data[i+4], b.data[i+3]
	}
	return nil
}

// ── Conversion ──────────────────────────────────────────────────────────────

// ToString returns the buffer contents as a UTF-8 string.
func (b *Buffer) ToString() string { return string(b.data) }

// ToHex returns the buffer contents as a hex-encoded string.
func (b *Buffer) ToHex() string { return hex.EncodeToString(b.data) }

// ToBase64 returns the buffer contents as a base64-encoded string.
func (b *Buffer) ToBase64() string { return base64.StdEncoding.EncodeToString(b.data) }

// ToJSON returns the buffer as a byte-value slice (e.g. [72, 101, 108]).
func (b *Buffer) ToJSON() []int {
	result := make([]int, len(b.data))
	for i, v := range b.data {
		result[i] = int(v)
	}
	return result
}

// Slice returns a new Buffer containing a copy of bytes from start to end.
func (b *Buffer) Slice(start, end int) *Buffer {
	if start < 0 {
		start = 0
	}
	if end > len(b.data) {
		end = len(b.data)
	}
	if start >= end {
		return Alloc(0)
	}
	return From(b.data[start:end])
}

// Copy copies bytes from this buffer into target. Returns number of bytes copied.
func (b *Buffer) Copy(target *Buffer, targetStart, sourceStart, sourceEnd int) int {
	if sourceEnd > len(b.data) {
		sourceEnd = len(b.data)
	}
	if targetStart >= len(target.data) {
		return 0
	}
	return copy(target.data[targetStart:], b.data[sourceStart:sourceEnd])
}

// ── Internal ────────────────────────────────────────────────────────────────

func (b *Buffer) check(offset, size int) error {
	if offset < 0 || offset+size > len(b.data) {
		return fmt.Errorf("buffer: offset %d is outside the bounds [0, %d]", offset, len(b.data)-size)
	}
	return nil
}
