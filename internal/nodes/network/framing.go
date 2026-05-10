// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package network

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Framing modes — keep in sync with the spec at
// specifications/issues/NODE_TCP_UDP.md § "Framing".
const (
	FramingStream       = "stream"
	FramingDelimiter    = "delimiter"
	FramingLengthPrefix = "length-prefix"
	FramingFixedLength  = "fixed-length"
)

// ErrFrameOversize is returned by Framer.Append when a single frame
// exceeds the configured maxFrameBytes. tcp-in and tcp-request map this
// to a catchable error and close the offending connection.
var ErrFrameOversize = errors.New("framing: frame exceeds maxFrameBytes")

// Framer splits an incoming byte stream into discrete frames according
// to a fixed strategy chosen at construction time. Implementations are
// **not** safe for concurrent use; one Framer per connection.
//
// Append is the single entry point: callers feed every Read() chunk
// into Append and consume the returned frames. Frames are returned in
// arrival order; a single Append call may produce zero, one, or many
// frames (or an error). The buffer parameter may be reused by the
// caller after Append returns — implementations either copy or take
// ownership of the relevant slice.
type Framer interface {
	Append(buf []byte) ([][]byte, error)
}

// NewStreamFramer returns a pass-through framer: every Append call
// emits exactly one frame containing whatever bytes were handed in (or
// no frames if the input is empty). maxFrameBytes is honoured per
// chunk so a single oversized read still trips the cap.
func NewStreamFramer(maxFrameBytes int) Framer {
	return &streamFramer{max: maxFrameBytes}
}

type streamFramer struct {
	max int
}

func (f *streamFramer) Append(buf []byte) ([][]byte, error) {
	if len(buf) == 0 {
		return nil, nil
	}
	if f.max > 0 && len(buf) > f.max {
		return nil, ErrFrameOversize
	}
	out := make([]byte, len(buf))
	copy(out, buf)
	return [][]byte{out}, nil
}

// NewDelimiterFramer returns a framer that splits the byte stream on
// the given delimiter; the delimiter is stripped from every emitted
// frame. Partial frames straddling Append calls are buffered. A frame
// that grows beyond maxFrameBytes (including the delimiter search) is
// reported via ErrFrameOversize and the buffer is reset.
func NewDelimiterFramer(delim []byte, maxFrameBytes int) (Framer, error) {
	if len(delim) == 0 {
		return nil, errors.New("framing: empty delimiter")
	}
	cp := make([]byte, len(delim))
	copy(cp, delim)
	return &delimiterFramer{delim: cp, max: maxFrameBytes}, nil
}

type delimiterFramer struct {
	delim []byte
	max   int
	buf   []byte
}

func (f *delimiterFramer) Append(buf []byte) ([][]byte, error) {
	if len(buf) > 0 {
		f.buf = append(f.buf, buf...)
	}
	var frames [][]byte
	for {
		idx := bytes.Index(f.buf, f.delim)
		if idx < 0 {
			break
		}
		frame := make([]byte, idx)
		copy(frame, f.buf[:idx])
		frames = append(frames, frame)
		f.buf = f.buf[idx+len(f.delim):]
	}
	if f.max > 0 && len(f.buf) > f.max {
		// The pending fragment is already larger than a complete
		// frame is allowed to be; no future delimiter could rescue
		// it. Drop the buffer and report.
		f.buf = nil
		return frames, ErrFrameOversize
	}
	return frames, nil
}

// NewLengthPrefixFramer returns a framer that reads a fixed-size
// big-/little-endian length header followed by exactly that many
// payload bytes. The header is stripped from emitted frames. When
// includesHeader is true the announced length covers the header
// itself (so payload size = announced - headerSize); otherwise the
// announced length is the payload size. headerSize must be 1, 2, 4,
// or 8.
func NewLengthPrefixFramer(headerSize int, endian binary.ByteOrder, includesHeader bool, maxFrameBytes int) (Framer, error) {
	switch headerSize {
	case 1, 2, 4, 8:
	default:
		return nil, fmt.Errorf("framing: invalid length-prefix header size %d (want 1/2/4/8)", headerSize)
	}
	if endian == nil {
		return nil, errors.New("framing: nil endian")
	}
	return &lengthPrefixFramer{
		headerSize:     headerSize,
		endian:         endian,
		includesHeader: includesHeader,
		max:            maxFrameBytes,
	}, nil
}

type lengthPrefixFramer struct {
	headerSize     int
	endian         binary.ByteOrder
	includesHeader bool
	max            int
	buf            []byte
}

func (f *lengthPrefixFramer) Append(buf []byte) ([][]byte, error) {
	if len(buf) > 0 {
		f.buf = append(f.buf, buf...)
	}
	var frames [][]byte
	for {
		if len(f.buf) < f.headerSize {
			break
		}
		announced := readUint(f.buf[:f.headerSize], f.endian, f.headerSize)
		var payloadSize int
		if f.includesHeader {
			if announced < uint64(f.headerSize) {
				return frames, fmt.Errorf("framing: announced length %d shorter than header (%d)", announced, f.headerSize)
			}
			payloadSize = int(announced) - f.headerSize
		} else {
			payloadSize = int(announced)
		}
		if payloadSize < 0 {
			return frames, fmt.Errorf("framing: negative payload size %d", payloadSize)
		}
		if f.max > 0 && payloadSize > f.max {
			return frames, ErrFrameOversize
		}
		total := f.headerSize + payloadSize
		if len(f.buf) < total {
			break
		}
		frame := make([]byte, payloadSize)
		copy(frame, f.buf[f.headerSize:total])
		frames = append(frames, frame)
		f.buf = f.buf[total:]
	}
	return frames, nil
}

// NewFixedLengthFramer returns a framer that emits one frame of
// exactly size bytes for every size bytes consumed.
func NewFixedLengthFramer(size int) (Framer, error) {
	if size <= 0 {
		return nil, fmt.Errorf("framing: fixed-length size must be > 0 (got %d)", size)
	}
	return &fixedLengthFramer{size: size}, nil
}

type fixedLengthFramer struct {
	size int
	buf  []byte
}

func (f *fixedLengthFramer) Append(buf []byte) ([][]byte, error) {
	if len(buf) > 0 {
		f.buf = append(f.buf, buf...)
	}
	var frames [][]byte
	for len(f.buf) >= f.size {
		frame := make([]byte, f.size)
		copy(frame, f.buf[:f.size])
		frames = append(frames, frame)
		f.buf = f.buf[f.size:]
	}
	return frames, nil
}

// readUint extracts a big-/little-endian unsigned integer of size 1, 2,
// 4, or 8 from b. Caller guarantees len(b) == size.
func readUint(b []byte, endian binary.ByteOrder, size int) uint64 {
	switch size {
	case 1:
		return uint64(b[0])
	case 2:
		return uint64(endian.Uint16(b))
	case 4:
		return uint64(endian.Uint32(b))
	case 8:
		return endian.Uint64(b)
	}
	return 0
}

// ParseDelimiter accepts the JS-style escape syntax used by the tcp-in
// and tcp-request configuration (e.g. "\\n", "\\r\\n", "\\0", "\\xFF")
// and returns the corresponding raw bytes. Strings without escapes
// pass through as their UTF-8 bytes.
//
// Supported escapes: \n \r \t \0 \\ \" \' \xHH (two hex digits).
// Unknown escapes return an error so a typo doesn't silently match a
// literal backslash sequence.
func ParseDelimiter(s string) ([]byte, error) {
	if s == "" {
		return nil, errors.New("framing: empty delimiter")
	}
	var out []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' {
			out = append(out, c)
			continue
		}
		if i+1 >= len(s) {
			return nil, errors.New("framing: trailing backslash in delimiter")
		}
		i++
		switch s[i] {
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case '0':
			out = append(out, 0)
		case '\\':
			out = append(out, '\\')
		case '"':
			out = append(out, '"')
		case '\'':
			out = append(out, '\'')
		case 'x':
			if i+2 >= len(s) {
				return nil, fmt.Errorf("framing: incomplete \\x escape at position %d", i-1)
			}
			hex := s[i+1 : i+3]
			v, err := strconv.ParseUint(hex, 16, 8)
			if err != nil {
				return nil, fmt.Errorf("framing: invalid \\x escape %q: %w", "\\x"+hex, err)
			}
			out = append(out, byte(v))
			i += 2
		default:
			return nil, fmt.Errorf("framing: unknown escape \\%s", string(s[i]))
		}
	}
	return out, nil
}

// ParseEndianness maps the configuration string to a binary.ByteOrder.
// Accepts "big" (default), "little", or any case-variation thereof.
func ParseEndianness(s string) (binary.ByteOrder, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "big", "be", "network":
		return binary.BigEndian, nil
	case "little", "le":
		return binary.LittleEndian, nil
	default:
		return nil, fmt.Errorf("framing: unknown endianness %q", s)
	}
}
