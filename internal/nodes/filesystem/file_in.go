// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package filesystem

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/loopzedev/loopze-edge/internal/flow"
	"github.com/loopzedev/loopze-edge/internal/nodes"
)

// File-in modes.
const (
	fileInModeRead      = "read"
	fileInModeWatch     = "watch"
	fileInModeReadWatch = "read+watch"
)

// FileInNode reads a file on demand (mode=read), watches it for changes
// (mode=watch), or both (mode=read+watch). Optional incremental mode reads
// only bytes appended since the last read using a persistent cursor.
//
// Phase 4: read + watch + read+watch. Incremental cursor lands in phase 5.
type FileInNode struct {
	cfg flow.NodeConfig
	nodes.BaseNode

	mode         string
	path         string
	encoding     string
	rootJail     string
	watchEvents  []string
	debounceMs   int
	incremental  bool
	fromStart    bool
	delimiter    string
	maxLineBytes int64

	// watch-mode runtime state
	watcher  *fsnotify.Watcher
	resolved string // path after Start-time validation; what the watcher uses
	opMask   fsnotify.Op
	done     chan struct{}
	wg       sync.WaitGroup

	// Persistent flow context for the incremental cursor. Optional —
	// nodes that do not need it work just fine without flowPers.
	flowPers flow.ContextStore
}

// SetContext implements flow.ContextProvider. Only flowPers is consumed
// (incremental cursor); the other stores are accepted but ignored.
func (n *FileInNode) SetContext(_, _, _, flowPers flow.ContextStore) {
	n.flowPers = flowPers
}

// resolveDelimiter applies the spec default: when incremental is on and the
// user left delimiter at the placeholder default, use "\n" for utf-8 / "auto"
// encoding and "none" for binary. Explicit values pass through.
func (n *FileInNode) resolveDelimiter(absPath string) string {
	switch n.delimiter {
	case DelimLF, DelimCRLF, DelimAuto, DelimNone:
		// Encoding-aware default: when the resolved encoding is binary the
		// LF default makes no sense — switch to none unless the user
		// explicitly asked for a delimiter.
		if n.delimiter == DelimLF && resolveEncoding(absPath, n.encoding) == "binary" {
			return DelimNone
		}
		return n.delimiter
	default:
		return DelimLF
	}
}

// NewFileInNode is the NodeFactory for the file-in node type.
func NewFileInNode(cfg flow.NodeConfig) (flow.NodeInstance, error) {
	return &FileInNode{cfg: cfg, done: make(chan struct{})}, nil
}

// Init parses every config field, validates the mode, encoding, and the
// watchEvents whitelist.
func (n *FileInNode) Init() error {
	p := n.cfg.Properties
	n.mode = nodes.StringVal(p, "mode", fileInModeRead)
	n.path = nodes.StringVal(p, "path", "")
	n.encoding = nodes.StringVal(p, "encoding", "auto")
	n.rootJail = nodes.StringVal(p, "rootJail", "")
	n.watchEvents = stringSlice(p, "watchEvents", []string{"write", "create"})
	n.debounceMs = nodes.IntVal(p, "debounceMs", 50)
	n.incremental = nodes.BoolVal(p, "incremental", false)
	n.fromStart = nodes.BoolVal(p, "fromStart", false)
	n.delimiter = nodes.StringVal(p, "delimiter", "\n")
	n.maxLineBytes = int64(nodes.IntVal(p, "maxLineBytes", 1048576))

	switch n.mode {
	case fileInModeRead, fileInModeWatch, fileInModeReadWatch:
	default:
		return fmt.Errorf("file-in %s: invalid mode %q", n.cfg.ID, n.mode)
	}
	switch n.encoding {
	case "auto", "utf-8", "binary":
	default:
		return fmt.Errorf("file-in %s: invalid encoding %q", n.cfg.ID, n.encoding)
	}

	mask, err := buildOpMask(n.watchEvents)
	if err != nil {
		return fmt.Errorf("file-in %s: %w", n.cfg.ID, err)
	}
	n.opMask = mask

	switch n.delimiter {
	case "", DelimLF, DelimCRLF, DelimAuto, DelimNone:
	default:
		return fmt.Errorf("file-in %s: invalid delimiter %q", n.cfg.ID, n.delimiter)
	}
	return nil
}

// Start registers the fsnotify watcher in watch / read+watch mode and
// launches the event-handling goroutine. Mode=read does no setup.
func (n *FileInNode) Start() error {
	slog.Info("file-in node started",
		"node_id", n.cfg.ID,
		"mode", n.mode,
		"path", n.path,
		"incremental", n.incremental,
	)
	if n.incremental && n.flowPers == nil {
		n.statusError("no persistent context")
		return fmt.Errorf("file-in %s: incremental mode requires persistent flow context", n.cfg.ID)
	}
	if n.mode == fileInModeRead {
		return nil
	}

	// Watch / read+watch — resolve and validate the path eagerly.
	resolved, err := resolvePath(n.path, nil, n.rootJail)
	if err != nil {
		n.statusError("path error")
		return fmt.Errorf("file-in %s: %w", n.cfg.ID, err)
	}
	n.resolved = resolved

	w, err := fsnotify.NewWatcher()
	if err != nil {
		n.statusError("watch error")
		return fmt.Errorf("file-in %s: new watcher: %w", n.cfg.ID, err)
	}
	if err := w.Add(resolved); err != nil {
		_ = w.Close()
		n.statusError("watch error")
		return fmt.Errorf("file-in %s: add watch: %w", n.cfg.ID, err)
	}
	n.watcher = w

	// Pin the incremental cursor to current EOF at Start time so writes
	// that arrive between deploy and the first event are skipped (when
	// fromStart=false). Without this, the first event-driven read sees
	// whatever EOF is at that later moment and would lose those writes.
	if n.incremental {
		if err := n.initIncrementalCursor(resolved); err != nil {
			_ = w.Close()
			n.statusError("cursor init error")
			return fmt.Errorf("file-in %s: %w", n.cfg.ID, err)
		}
	}

	if n.Status != nil {
		n.Status("green", "watching · "+resolved)
	}

	n.wg.Add(1)
	go n.watchLoop()
	return nil
}

// initIncrementalCursor pins the cursor at deploy time when no value is
// stored yet. fromStart=true sets it to 0; fromStart=false to current EOF.
// A pre-existing cursor (from a previous deploy) is left untouched —
// persistence is intentional.
func (n *FileInNode) initIncrementalCursor(absPath string) error {
	key := cursorKey(n.cfg.ID, absPath)
	existing, err := n.flowPers.Get(key)
	if err != nil {
		return fmt.Errorf("read existing cursor: %w", err)
	}
	if existing != nil {
		return nil
	}
	var cursor int64 = 0
	if !n.fromStart {
		info, err := os.Stat(absPath)
		if err != nil {
			return fmt.Errorf("stat for cursor init: %w", err)
		}
		cursor = info.Size()
	}
	return n.flowPers.Set(key, cursor)
}

// Stop closes the fsnotify watcher (in watch modes) and waits for the
// event goroutine to drain.
func (n *FileInNode) Stop() error {
	slog.Info("file-in node stopped", "node_id", n.cfg.ID)
	if n.mode == fileInModeRead {
		return nil
	}
	close(n.done)
	if n.watcher != nil {
		_ = n.watcher.Close()
	}
	n.wg.Wait()
	return nil
}

// HandleMessage performs a one-shot file read in mode=read. In watch /
// read+watch mode the node has no input wired by the engine, so
// HandleMessage is normally not invoked; if it is, the message is dropped.
//
// When msg.resetCursor is true the persisted cursor is cleared and no
// content is emitted. Useful for forcing a re-read after a known log rotation.
func (n *FileInNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil || n.mode != fileInModeRead {
		return nil, nil
	}

	resolved, err := resolvePath(n.path, msg, n.rootJail)
	if err != nil {
		n.statusError("path error")
		return nil, fmt.Errorf("file-in: %w", err)
	}

	if reset, _ := msg.Get("resetCursor").(bool); reset && n.incremental {
		key := cursorKey(n.cfg.ID, resolved)
		if err := resetCursorKey(n.flowPers, key); err != nil {
			n.statusError("cursor reset error")
			return nil, fmt.Errorf("file-in: reset cursor: %w", err)
		}
		if n.Status != nil {
			n.Status("blue", "cursor reset")
		}
		return nil, nil
	}

	if n.incremental {
		return n.readIncrementalAndEmit(resolved, "", msg)
	}

	out, err := n.buildReadMessage(resolved, "", msg)
	if err != nil {
		return nil, fmt.Errorf("file-in: %w", err)
	}
	if n.Status != nil {
		n.Status("blue", "read "+humanSize(int(out.Get("size").(int64))))
	}
	return [][]*flow.Message{{out}}, nil
}

// watchLoop drains fsnotify events with inline debounce: after the first
// matching event arrives, the loop keeps consuming further events for
// debounceMs to collapse bursts (write → flush → close → chmod), then
// dispatches a single handleWatchEvent for the latest event.
func (n *FileInNode) watchLoop() {
	defer n.wg.Done()
	for {
		ev, ok := n.waitForEvent()
		if !ok {
			return
		}
		latest := n.coalesce(ev)
		n.handleWatchEvent(latest)
	}
}

// waitForEvent blocks until the next matching event, an error, or shutdown.
// Returns (event, true) on a real event; (zero, false) when the loop should exit.
func (n *FileInNode) waitForEvent() (fsnotify.Event, bool) {
	for {
		select {
		case <-n.done:
			return fsnotify.Event{}, false
		case ev, ok := <-n.watcher.Events:
			if !ok {
				return fsnotify.Event{}, false
			}
			if ev.Op&n.opMask != 0 {
				return ev, true
			}
		case err, ok := <-n.watcher.Errors:
			if !ok {
				return fsnotify.Event{}, false
			}
			slog.Warn("file-in watcher error",
				"node_id", n.cfg.ID,
				"path", n.resolved,
				"error", err,
			)
		}
	}
}

// coalesce keeps consuming further matching events for debounceMs before
// returning, so bursts collapse to a single dispatch.
func (n *FileInNode) coalesce(first fsnotify.Event) fsnotify.Event {
	if n.debounceMs <= 0 {
		return first
	}
	latest := first
	deadline := time.NewTimer(time.Duration(n.debounceMs) * time.Millisecond)
	defer deadline.Stop()
	for {
		select {
		case <-deadline.C:
			return latest
		case <-n.done:
			return latest
		case ev, ok := <-n.watcher.Events:
			if !ok {
				return latest
			}
			if ev.Op&n.opMask != 0 {
				latest = ev
			}
		case err, ok := <-n.watcher.Errors:
			if !ok {
				return latest
			}
			slog.Warn("file-in watcher error",
				"node_id", n.cfg.ID,
				"path", n.resolved,
				"error", err,
			)
		}
	}
}

// handleWatchEvent dispatches one event into the flow. In watch mode it
// emits only event metadata; in read+watch mode it reads the file and
// emits the content alongside the event field.
func (n *FileInNode) handleWatchEvent(ev fsnotify.Event) {
	if n.Send == nil {
		return
	}
	eventName := primaryEventName(ev.Op)

	if n.mode == fileInModeWatch {
		msg := flow.NewMessage()
		msg.Set("filename", ev.Name)
		msg.Set("event", eventName)
		n.Send(0, msg)
		return
	}

	// read+watch: read content and emit
	if n.incremental {
		out, err := n.readIncrementalAndEmit(ev.Name, eventName, flow.NewMessage())
		if err != nil {
			slog.Warn("file-in read+watch incremental: read failed",
				"node_id", n.cfg.ID,
				"path", ev.Name,
				"error", err,
			)
			return
		}
		// readIncrementalAndEmit already wrote to n.Send when it had content.
		_ = out
		return
	}
	out, err := n.buildReadMessage(ev.Name, eventName, flow.NewMessage())
	if err != nil {
		slog.Warn("file-in read+watch: read failed",
			"node_id", n.cfg.ID,
			"path", ev.Name,
			"error", err,
		)
		return
	}
	n.Send(0, out)
}

// readIncrementalAndEmit performs an incremental read, applies the
// line-aware trim, and pushes a message via n.Send when there is content
// to emit (or a reset to signal). For HandleMessage callers it also
// returns the [][]*flow.Message envelope so the engine can route the
// message; for watch dispatch the return value is unused.
//
// When the incremental result is Skipped (no new bytes / partial-only
// line / unchanged file), nothing is emitted and the engine envelope is
// nil — matching the spec rule that incremental stays silent in those
// cases.
func (n *FileInNode) readIncrementalAndEmit(absPath, eventName string, msg *flow.Message) ([][]*flow.Message, error) {
	delim := n.resolveDelimiter(absPath)

	res, err := readIncremental(n.flowPers, IncrementalOpts{
		NodeID:       n.cfg.ID,
		AbsPath:      absPath,
		Delimiter:    delim,
		MaxLineBytes: n.maxLineBytes,
		FromStart:    n.fromStart,
	})
	if err != nil {
		n.statusError(incrementalErrorLabel(err))
		return nil, fmt.Errorf("file-in: %w", err)
	}
	if res.Skipped {
		return nil, nil
	}

	encoding := resolveEncoding(absPath, n.encoding)
	msg.SetPayload(decodePayload(res.Data, encoding))
	msg.Set("filename", absPath)
	msg.Set("encoding", encoding)
	msg.Set("position", res.Position)
	msg.Set("bytesRead", res.BytesRead)
	if delim != DelimNone {
		msg.Set("lineCount", res.LineCount)
	}
	if eventName != "" {
		msg.Set("event", eventName)
	}
	if res.Reset {
		msg.Set("reset", true)
	}

	if n.Status != nil {
		n.Status("blue", "read "+humanSize(res.BytesRead))
	}

	// In watch mode dispatch via n.Send (no input port). In read mode
	// return through the engine envelope so msg pass-through preserves
	// any existing fields the upstream node set.
	if n.mode == fileInModeRead {
		return [][]*flow.Message{{msg}}, nil
	}
	if n.Send != nil {
		n.Send(0, msg)
	}
	return nil, nil
}

// incrementalErrorLabel maps cursor / line-buffer errors to a stable label.
func incrementalErrorLabel(err error) string {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "not found"
	case errors.Is(err, os.ErrPermission):
		return "permission denied"
	}
	if strings.Contains(err.Error(), "exceeded maxLineBytes") {
		return "line buffer exceeded"
	}
	return "incremental error"
}

// buildReadMessage reads the file at path, encodes the payload, and writes
// the standard output fields onto msg. The eventName, when non-empty, is
// added as msg.event (used by watch / read+watch). Returns the same msg
// for chained use.
func (n *FileInNode) buildReadMessage(path, eventName string, msg *flow.Message) (*flow.Message, error) {
	data, info, err := readWholeFile(path)
	if err != nil {
		n.statusError(fileReadErrorLabel(err))
		return nil, err
	}
	encoding := resolveEncoding(path, n.encoding)
	msg.SetPayload(decodePayload(data, encoding))
	msg.Set("filename", path)
	msg.Set("encoding", encoding)
	msg.Set("size", info.Size())
	if eventName != "" {
		msg.Set("event", eventName)
	}
	return msg, nil
}

// readWholeFile reads the entire file into memory and returns its FileInfo
// alongside the bytes.
func readWholeFile(path string) ([]byte, os.FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	return data, info, nil
}

// fileReadErrorLabel maps a stat / read error to a stable status label.
func fileReadErrorLabel(err error) string {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "not found"
	case errors.Is(err, os.ErrPermission):
		return "permission denied"
	default:
		return "read error"
	}
}

func (n *FileInNode) statusError(label string) {
	if n.Status != nil {
		n.Status("red", label)
	}
}

// buildOpMask validates the watchEvents whitelist and folds it into a single
// fsnotify.Op bitmask used to filter incoming events.
func buildOpMask(events []string) (fsnotify.Op, error) {
	var mask fsnotify.Op
	for _, ev := range events {
		switch ev {
		case "create":
			mask |= fsnotify.Create
		case "write":
			mask |= fsnotify.Write
		case "remove":
			mask |= fsnotify.Remove
		case "rename":
			mask |= fsnotify.Rename
		default:
			return 0, fmt.Errorf("invalid watch event %q", ev)
		}
	}
	if mask == 0 {
		// Empty list is valid config but means "never emit"; allow it.
		return 0, nil
	}
	return mask, nil
}

// primaryEventName picks the most specific name when fsnotify reports
// multiple bits in the same Op (e.g. CREATE|WRITE on initial file write).
// Order: write > create > rename > remove (write is the most useful signal
// for "content changed").
func primaryEventName(op fsnotify.Op) string {
	switch {
	case op.Has(fsnotify.Write):
		return "write"
	case op.Has(fsnotify.Create):
		return "create"
	case op.Has(fsnotify.Rename):
		return "rename"
	case op.Has(fsnotify.Remove):
		return "remove"
	default:
		return op.String()
	}
}

// stringSlice extracts a []string from the property bag. JSON-decoded values
// arrive as []any with string elements; native callers may pass []string
// directly. Falls back to the supplied default for missing or wrong-typed
// entries.
func stringSlice(m map[string]any, key string, fallback []string) []string {
	switch v := m[key].(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		if len(out) == 0 {
			return fallback
		}
		return out
	}
	return fallback
}

// FileInTypeInfo returns the NodeTypeInfo for registering the file-in node.
func FileInTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "file-in",
		Category:    "filesystem",
		Label:       "File In",
		Description: "Read file content and/or watch for changes",
		Icon:        "file-in",
		Defaults: map[string]any{
			"mode":         "read",
			"path":         "",
			"encoding":     "auto",
			"watchEvents":  []string{"write", "create"},
			"debounceMs":   50,
			"incremental":  false,
			"fromStart":    false,
			"delimiter":    "\n",
			"maxLineBytes": 1048576,
		},
		Inputs:  1,
		Outputs: 1,
	}
}
