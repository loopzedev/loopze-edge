// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package filesystem

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// Delimiter values accepted by IncrementalOpts.Delimiter.
const (
	DelimLF   = "\n"
	DelimCRLF = "\r\n"
	DelimAuto = "auto"
	DelimNone = "none"
)

// cursorKey returns the flowPers key for the (nodeID, absPath) pair. The
// SHA-prefix keeps it short and free of characters that NATS KV rejects
// (':', '/'). Two file-in nodes pointing at the same file have distinct
// node IDs, so they get distinct keys and track independently.
func cursorKey(nodeID, absPath string) string {
	h := sha256.Sum256([]byte(nodeID + ":" + absPath))
	return "_filecursor." + hex.EncodeToString(h[:8])
}

// IncrementalOpts bundles the per-call settings for readIncremental.
type IncrementalOpts struct {
	NodeID       string
	AbsPath      string
	Delimiter    string // DelimLF / DelimCRLF / DelimAuto / DelimNone
	MaxLineBytes int64
	FromStart    bool
}

// IncrementalResult is the outcome of one incremental read.
type IncrementalResult struct {
	Data      []byte // bytes to emit; empty when only Reset is set
	Position  int64  // cursor after this call (persisted before return)
	BytesRead int    // len(Data)
	LineCount int    // complete delimited records in Data; 0 when Delimiter=DelimNone
	Reset     bool   // truncation/rotation detected since the previous read
	Skipped   bool   // true when there is nothing to emit (no new bytes, partial-only line)
}

// readIncremental seeks to the stored cursor, reads new bytes, applies the
// line-aware trim (when Delimiter is set), persists the new cursor, and
// returns what the caller should emit. Skipped=true means no emission.
//
// Truncation handling: if the stored cursor is beyond the current file size
// the cursor is reset to 0, Reset=true is set, and the read continues from
// the beginning of the (rotated/truncated) file.
func readIncremental(store flow.ContextStore, opts IncrementalOpts) (*IncrementalResult, error) {
	info, err := os.Stat(opts.AbsPath)
	if err != nil {
		return nil, err
	}
	fileSize := info.Size()

	key := cursorKey(opts.NodeID, opts.AbsPath)
	cursor, err := loadCursor(store, key, fileSize, opts.FromStart)
	if err != nil {
		return nil, fmt.Errorf("load cursor: %w", err)
	}

	res := &IncrementalResult{Position: cursor}

	// Truncation / rotation: stored cursor is past the new file size.
	if cursor > fileSize {
		res.Reset = true
		cursor = 0
		res.Position = 0
	}

	// No new bytes — emit nothing unless this call detected a reset.
	if cursor == fileSize {
		if res.Reset {
			// File was truncated to a smaller size and contains nothing new
			// to read; persist the reset cursor and let the caller emit a
			// reset-signal message.
			if err := saveCursor(store, key, 0); err != nil {
				return nil, fmt.Errorf("save cursor: %w", err)
			}
			return res, nil
		}
		res.Skipped = true
		return res, nil
	}

	// Read from cursor to EOF.
	f, err := os.Open(opts.AbsPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := f.Seek(cursor, io.SeekStart); err != nil {
		return nil, err
	}
	buf, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	if opts.Delimiter == "" || opts.Delimiter == DelimNone {
		res.Data = buf
		res.BytesRead = len(buf)
		res.Position = cursor + int64(len(buf))
		if err := saveCursor(store, key, res.Position); err != nil {
			return nil, fmt.Errorf("save cursor: %w", err)
		}
		return res, nil
	}

	trimmed, lineCount, err := trimToLastLine(buf, opts.Delimiter, opts.MaxLineBytes)
	if err != nil {
		return nil, err
	}
	if len(trimmed) == 0 {
		// Only a partial line is available — leave the cursor untouched so
		// the next read picks up the full line.
		if !res.Reset {
			res.Skipped = true
		}
		// On reset+partial-only, still surface the reset (Skipped stays false).
		return res, nil
	}

	res.Data = trimmed
	res.BytesRead = len(trimmed)
	res.LineCount = lineCount
	res.Position = cursor + int64(len(trimmed))
	if err := saveCursor(store, key, res.Position); err != nil {
		return nil, fmt.Errorf("save cursor: %w", err)
	}
	return res, nil
}

// loadCursor reads the persisted cursor or initialises one based on fromStart.
// On first access (no stored value) the cursor is 0 when fromStart=true,
// otherwise it equals the current file size (skip existing content).
func loadCursor(store flow.ContextStore, key string, fileSize int64, fromStart bool) (int64, error) {
	if store == nil {
		return 0, fmt.Errorf("no persistent context available")
	}
	val, err := store.Get(key)
	if err != nil {
		return 0, err
	}
	if val == nil {
		if fromStart {
			return 0, nil
		}
		return fileSize, nil
	}
	return toInt64(val), nil
}

// saveCursor persists the cursor under the given key. NATS KV stores values
// as JSON, so int64 round-trips as float64 — handled in toInt64 on the way back.
func saveCursor(store flow.ContextStore, key string, cursor int64) error {
	if store == nil {
		return fmt.Errorf("no persistent context available")
	}
	return store.Set(key, cursor)
}

// resetCursorKey deletes the persisted cursor.
func resetCursorKey(store flow.ContextStore, key string) error {
	if store == nil {
		return fmt.Errorf("no persistent context available")
	}
	return store.Delete(key)
}

// toInt64 coerces the JSON-decoded cursor value (typically float64) back to int64.
func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	}
	return 0
}

// trimToLastLine returns buf[0:end] where end is one past the last complete
// delimiter found in buf, plus the count of complete records before end.
// Returns (nil, 0, nil) when no complete line is present and buf is below
// the maxLineBytes guard. Returns an error when maxLineBytes is exceeded
// without finding a delimiter — that means the file is misconfigured for
// line-aware reads (binary content with delimiter set, or a single
// pathologically long line).
func trimToLastLine(buf []byte, delimiter string, maxLineBytes int64) ([]byte, int, error) {
	switch delimiter {
	case DelimCRLF:
		end, count := lastBoundaryCRLF(buf)
		return checkTrim(buf, end, count, maxLineBytes)
	case DelimLF, DelimAuto:
		// auto accepts either: a CRLF line still ends in LF so locating the
		// last LF is enough. CR alone is not treated as a line break.
		end, count := lastBoundaryLF(buf)
		return checkTrim(buf, end, count, maxLineBytes)
	default:
		return nil, 0, fmt.Errorf("unknown delimiter %q", delimiter)
	}
}

// checkTrim factors the maxLineBytes guard out of the per-delimiter helpers.
func checkTrim(buf []byte, end, count int, maxLineBytes int64) ([]byte, int, error) {
	if end <= 0 {
		if maxLineBytes > 0 && int64(len(buf)) >= maxLineBytes {
			return nil, 0, fmt.Errorf("line buffer exceeded maxLineBytes (%d)", maxLineBytes)
		}
		return nil, 0, nil
	}
	return buf[:end], count, nil
}

// lastBoundaryLF locates the last '\n' in buf and counts complete lines.
func lastBoundaryLF(buf []byte) (end, count int) {
	end = -1
	for i, b := range buf {
		if b == '\n' {
			end = i + 1
			count++
		}
	}
	if end < 0 {
		return 0, 0
	}
	return end, count
}

// lastBoundaryCRLF locates the last "\r\n" in buf and counts pairs. A bare
// '\n' or bare '\r' is not a boundary in strict CRLF mode.
func lastBoundaryCRLF(buf []byte) (end, count int) {
	end = -1
	for i := 0; i+1 < len(buf); i++ {
		if buf[i] == '\r' && buf[i+1] == '\n' {
			end = i + 2
			count++
			i++ // skip the '\n' we just consumed
		}
	}
	if end < 0 {
		return 0, 0
	}
	return end, count
}
