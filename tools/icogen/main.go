// icogen converts a PNG into a multi-resolution Windows .ico file.
//
// Usage:
//
//	go run ./tools/icogen -in design/logos/logo.png -out cmd/loopze/loopze.ico
//
// The output contains the standard set of sizes that Windows Explorer and the
// taskbar pick from: 16, 24, 32, 48, 64, 128, 256. The 256 entry is stored as
// PNG (per the ICO spec); smaller entries are stored as 32-bit BGRA bitmaps.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"sort"

	xdraw "golang.org/x/image/draw"
)

var sizes = []int{16, 24, 32, 48, 64, 128, 256}

func main() {
	in := flag.String("in", "", "input PNG (square, ideally 1024x1024 or larger)")
	out := flag.String("out", "", "output .ico path")
	flag.Parse()
	if *in == "" || *out == "" {
		log.Fatal("usage: icogen -in <png> -out <ico>")
	}

	src, err := loadPNG(*in)
	if err != nil {
		log.Fatalf("load %s: %v", *in, err)
	}

	entries := make([]iconEntry, 0, len(sizes))
	for _, s := range sizes {
		resized := resize(src, s)
		var data []byte
		if s >= 256 {
			data, err = encodePNG(resized)
		} else {
			data, err = encodeBMP(resized)
		}
		if err != nil {
			log.Fatalf("encode %dpx: %v", s, err)
		}
		entries = append(entries, iconEntry{size: s, data: data})
	}

	if err := writeICO(*out, entries); err != nil {
		log.Fatalf("write %s: %v", *out, err)
	}
	fmt.Printf("wrote %s (%d entries: %v)\n", *out, len(entries), sizes)
}

func loadPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func resize(src image.Image, size int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)
	return dst
}

func encodePNG(img *image.RGBA) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// encodeBMP writes a "DIB" (BITMAPINFOHEADER + BGRA pixels + AND mask) in the
// shape that the ICO format expects: no BMP file header, doubled height in the
// info header (image + mask), bottom-up rows.
func encodeBMP(img *image.RGBA) ([]byte, error) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	rowBGRA := w * 4
	maskRow := ((w + 31) / 32) * 4 // 1 bpp, padded to 4 bytes
	pixelBytes := rowBGRA * h
	maskBytes := maskRow * h

	var buf bytes.Buffer
	// BITMAPINFOHEADER (40 bytes), height doubled to include the AND mask.
	bi := struct {
		Size, Width                                          uint32
		Height                                               int32
		Planes, BitCount                                     uint16
		Compression, SizeImage                               uint32
		XPelsPerMeter, YPelsPerMeter                         int32
		ClrUsed, ClrImportant                                uint32
	}{
		Size:      40,
		Width:     uint32(w),
		Height:    int32(h * 2),
		Planes:    1,
		BitCount:  32,
		SizeImage: uint32(pixelBytes + maskBytes),
	}
	if err := binary.Write(&buf, binary.LittleEndian, bi); err != nil {
		return nil, err
	}

	// Pixel data, BGRA, bottom-up.
	for y := h - 1; y >= 0; y-- {
		for x := 0; x < w; x++ {
			c := img.RGBAAt(x, y)
			buf.WriteByte(c.B)
			buf.WriteByte(c.G)
			buf.WriteByte(c.R)
			buf.WriteByte(c.A)
		}
	}

	// AND mask: zero (32-bit BGRA already carries alpha; mask must still exist).
	buf.Write(make([]byte, maskBytes))
	return buf.Bytes(), nil
}

type iconEntry struct {
	size int
	data []byte
}

func writeICO(path string, entries []iconEntry) error {
	sort.Slice(entries, func(i, j int) bool { return entries[i].size < entries[j].size })

	var buf bytes.Buffer
	// ICONDIR: reserved=0, type=1 (icon), count=N
	binary.Write(&buf, binary.LittleEndian, uint16(0))
	binary.Write(&buf, binary.LittleEndian, uint16(1))
	binary.Write(&buf, binary.LittleEndian, uint16(len(entries)))

	// First image data starts after ICONDIR (6 bytes) + N * ICONDIRENTRY (16 bytes each).
	offset := uint32(6 + len(entries)*16)
	for _, e := range entries {
		w := byte(e.size)
		h := byte(e.size)
		if e.size >= 256 {
			w, h = 0, 0 // ICO encodes 256 as 0
		}
		// ICONDIRENTRY (16 bytes)
		buf.WriteByte(w)
		buf.WriteByte(h)
		buf.WriteByte(0) // color count
		buf.WriteByte(0) // reserved
		binary.Write(&buf, binary.LittleEndian, uint16(1))  // planes
		binary.Write(&buf, binary.LittleEndian, uint16(32)) // bit count
		binary.Write(&buf, binary.LittleEndian, uint32(len(e.data)))
		binary.Write(&buf, binary.LittleEndian, offset)
		offset += uint32(len(e.data))
	}
	for _, e := range entries {
		buf.Write(e.data)
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
