// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package network

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

func TestStreamFramerPassThrough(t *testing.T) {
	f := NewStreamFramer(0)
	frames, err := f.Append([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 || string(frames[0]) != "hello" {
		t.Errorf("got %q", frames)
	}
}

func TestStreamFramerEmptyInput(t *testing.T) {
	f := NewStreamFramer(0)
	frames, err := f.Append(nil)
	if err != nil || frames != nil {
		t.Errorf("got %v / %v", frames, err)
	}
}

func TestStreamFramerOversize(t *testing.T) {
	f := NewStreamFramer(4)
	_, err := f.Append([]byte("12345"))
	if !errors.Is(err, ErrFrameOversize) {
		t.Errorf("want ErrFrameOversize, got %v", err)
	}
}

func TestDelimiterFramerSingleFrame(t *testing.T) {
	f, err := NewDelimiterFramer([]byte("\n"), 0)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := f.Append([]byte("hello\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 || string(frames[0]) != "hello" {
		t.Errorf("got %q", frames)
	}
}

func TestDelimiterFramerMultipleFramesOneAppend(t *testing.T) {
	f, _ := NewDelimiterFramer([]byte("\n"), 0)
	frames, _ := f.Append([]byte("a\nb\nc\n"))
	if len(frames) != 3 {
		t.Fatalf("got %d frames", len(frames))
	}
	for i, want := range []string{"a", "b", "c"} {
		if string(frames[i]) != want {
			t.Errorf("frame %d = %q, want %q", i, frames[i], want)
		}
	}
}

func TestDelimiterFramerSplitAcrossAppends(t *testing.T) {
	f, _ := NewDelimiterFramer([]byte("\r\n"), 0)
	if frames, _ := f.Append([]byte("hel")); len(frames) != 0 {
		t.Errorf("partial: got %d frames", len(frames))
	}
	if frames, _ := f.Append([]byte("lo\r")); len(frames) != 0 {
		t.Errorf("partial-with-half-delim: got %d frames", len(frames))
	}
	frames, _ := f.Append([]byte("\nworld\r\n"))
	if len(frames) != 2 {
		t.Fatalf("got %d frames", len(frames))
	}
	if string(frames[0]) != "hello" || string(frames[1]) != "world" {
		t.Errorf("got %q / %q", frames[0], frames[1])
	}
}

func TestDelimiterFramerOversizePending(t *testing.T) {
	f, _ := NewDelimiterFramer([]byte("\n"), 4)
	_, err := f.Append([]byte("12345"))
	if !errors.Is(err, ErrFrameOversize) {
		t.Errorf("want ErrFrameOversize, got %v", err)
	}
}

func TestDelimiterFramerEmptyDelimiter(t *testing.T) {
	if _, err := NewDelimiterFramer(nil, 0); err == nil {
		t.Error("expected error for empty delimiter")
	}
}

func TestLengthPrefixFramerBigEndian(t *testing.T) {
	f, err := NewLengthPrefixFramer(4, binary.BigEndian, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.BigEndian, uint32(5))
	buf.WriteString("hello")
	_ = binary.Write(&buf, binary.BigEndian, uint32(3))
	buf.WriteString("hey")

	frames, err := f.Append(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 2 || string(frames[0]) != "hello" || string(frames[1]) != "hey" {
		t.Errorf("got %q", frames)
	}
}

func TestLengthPrefixFramerLittleEndian(t *testing.T) {
	f, _ := NewLengthPrefixFramer(2, binary.LittleEndian, false, 0)
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, uint16(4))
	buf.WriteString("test")
	frames, _ := f.Append(buf.Bytes())
	if len(frames) != 1 || string(frames[0]) != "test" {
		t.Errorf("got %q", frames)
	}
}

func TestLengthPrefixFramerIncludesHeader(t *testing.T) {
	// announced length = 8 (4 header + 4 payload)
	f, _ := NewLengthPrefixFramer(4, binary.BigEndian, true, 0)
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.BigEndian, uint32(8))
	buf.WriteString("body")
	frames, err := f.Append(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 || string(frames[0]) != "body" {
		t.Errorf("got %q", frames)
	}
}

func TestLengthPrefixFramerIncludesHeaderTooShort(t *testing.T) {
	f, _ := NewLengthPrefixFramer(4, binary.BigEndian, true, 0)
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.BigEndian, uint32(2)) // < header size
	if _, err := f.Append(buf.Bytes()); err == nil {
		t.Error("expected error for announced < header size")
	}
}

func TestLengthPrefixFramerSplitAcrossAppends(t *testing.T) {
	f, _ := NewLengthPrefixFramer(4, binary.BigEndian, false, 0)
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.BigEndian, uint32(5))
	buf.WriteString("hello")
	all := buf.Bytes()

	for i := 0; i < len(all); i++ {
		frames, err := f.Append(all[i : i+1])
		if err != nil {
			t.Fatal(err)
		}
		if i < len(all)-1 && len(frames) != 0 {
			t.Errorf("byte %d: got %d frames before complete", i, len(frames))
		}
		if i == len(all)-1 {
			if len(frames) != 1 || string(frames[0]) != "hello" {
				t.Errorf("final: got %q", frames)
			}
		}
	}
}

func TestLengthPrefixFramerOversize(t *testing.T) {
	f, _ := NewLengthPrefixFramer(4, binary.BigEndian, false, 16)
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.BigEndian, uint32(1024)) // exceeds max
	_, err := f.Append(buf.Bytes())
	if !errors.Is(err, ErrFrameOversize) {
		t.Errorf("want ErrFrameOversize, got %v", err)
	}
}

func TestLengthPrefixFramerInvalidHeaderSize(t *testing.T) {
	if _, err := NewLengthPrefixFramer(3, binary.BigEndian, false, 0); err == nil {
		t.Error("expected error for header size 3")
	}
}

func TestFixedLengthFramer(t *testing.T) {
	f, err := NewFixedLengthFramer(4)
	if err != nil {
		t.Fatal(err)
	}
	frames, _ := f.Append([]byte("aaaabbbbcc"))
	if len(frames) != 2 {
		t.Fatalf("got %d frames", len(frames))
	}
	if string(frames[0]) != "aaaa" || string(frames[1]) != "bbbb" {
		t.Errorf("got %q / %q", frames[0], frames[1])
	}
	// previous remainder "cc" + new "ccdddd" = "ccccdddd" → "cccc","dddd"
	frames2, _ := f.Append([]byte("ccdddd"))
	if len(frames2) != 2 {
		t.Fatalf("got %d frames", len(frames2))
	}
	if string(frames2[0]) != "cccc" || string(frames2[1]) != "dddd" {
		t.Errorf("got %q / %q", frames2[0], frames2[1])
	}
}

func TestFixedLengthFramerInvalidSize(t *testing.T) {
	if _, err := NewFixedLengthFramer(0); err == nil {
		t.Error("size 0: expected error")
	}
	if _, err := NewFixedLengthFramer(-1); err == nil {
		t.Error("size -1: expected error")
	}
}

func TestParseDelimiter(t *testing.T) {
	cases := []struct {
		in   string
		want []byte
	}{
		{`\n`, []byte{'\n'}},
		{`\r\n`, []byte{'\r', '\n'}},
		{`\0`, []byte{0}},
		{`\xFF`, []byte{0xFF}},
		{`\x00\xff`, []byte{0x00, 0xff}},
		{`abc\n`, []byte{'a', 'b', 'c', '\n'}},
		{`\\`, []byte{'\\'}},
		{`\t`, []byte{'\t'}},
		{`literal`, []byte("literal")},
	}
	for _, c := range cases {
		got, err := ParseDelimiter(c.in)
		if err != nil {
			t.Errorf("%q: %v", c.in, err)
			continue
		}
		if !bytes.Equal(got, c.want) {
			t.Errorf("%q -> %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseDelimiterErrors(t *testing.T) {
	bad := []string{``, `\`, `\xZZ`, `\x1`, `\q`}
	for _, s := range bad {
		if _, err := ParseDelimiter(s); err == nil {
			t.Errorf("%q: expected error", s)
		}
	}
}

func TestParseEndianness(t *testing.T) {
	be, _ := ParseEndianness("big")
	if be != binary.BigEndian {
		t.Error("big != BigEndian")
	}
	le, _ := ParseEndianness("LITTLE")
	if le != binary.LittleEndian {
		t.Error("LITTLE != LittleEndian")
	}
	def, _ := ParseEndianness("")
	if def != binary.BigEndian {
		t.Error("default != BigEndian")
	}
	if _, err := ParseEndianness("middle"); err == nil {
		t.Error("middle: expected error")
	}
}
