// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package filesystem

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/loopzedev/loopze-edge/internal/flow"
	"github.com/loopzedev/loopze-edge/internal/nodes"
)

// File-watch modes.
const (
	fileWatchModeRead      = "read"
	fileWatchModeWatch     = "watch"
	fileWatchModeReadWatch = "read+watch"
)

// sendAs values.
const (
	sendAsIndividual = "individual"
	sendAsArray      = "array"
)

// FileWatchNode watches a file or folder for filesystem changes and emits
// metadata-only events. It NEVER reads file content — to act on the
// content, wire a FileReadNode after it (typically reading msg.filename
// or msg.path from the watch event).
//
// In mode=read it returns a directory listing (also metadata only). In
// mode=read+watch it always behaves incrementally: each fsnotify event
// triggers a re-scan and only entries whose modTime advanced since the
// last scan are emitted.
type FileWatchNode struct {
	cfg flow.NodeConfig
	nodes.BaseNode

	mode        string
	path        string
	recursive   bool
	glob        string
	watchEvents []string
	sendAs      string
	debounceMs  int
	incremental bool
	fromStart   bool
	rootJail    string

	// runtime state
	watcher  *fsnotify.Watcher
	resolved string
	opMask   fsnotify.Op
	done     chan struct{}
	wg       sync.WaitGroup
	flowPers flow.ContextStore
}

// NewFileWatchNode is the NodeFactory for the file-watch node type.
func NewFileWatchNode(cfg flow.NodeConfig) (flow.NodeInstance, error) {
	return &FileWatchNode{cfg: cfg, done: make(chan struct{})}, nil
}

// SetContext implements flow.ContextProvider. Only flowPers is consumed
// (incremental modTime map). read+watch mode always uses it.
func (n *FileWatchNode) SetContext(_, _, _, flowPers flow.ContextStore) {
	n.flowPers = flowPers
}

// Init parses and validates configuration.
func (n *FileWatchNode) Init() error {
	p := n.cfg.Properties
	n.mode = nodes.StringVal(p, "mode", fileWatchModeWatch)
	n.path = nodes.StringVal(p, "path", "")
	n.recursive = nodes.BoolVal(p, "recursive", false)
	n.glob = nodes.StringVal(p, "glob", "*")
	n.watchEvents = stringSlice(p, "watchEvents", []string{"create", "write", "remove", "rename"})
	n.sendAs = nodes.StringVal(p, "sendAs", sendAsIndividual)
	n.debounceMs = nodes.IntVal(p, "debounceMs", 100)
	n.incremental = nodes.BoolVal(p, "incremental", false)
	n.fromStart = nodes.BoolVal(p, "fromStart", false)
	n.rootJail = nodes.StringVal(p, "rootJail", "")

	switch n.mode {
	case fileWatchModeRead, fileWatchModeWatch, fileWatchModeReadWatch:
	default:
		return fmt.Errorf("file-watch %s: invalid mode %q", n.cfg.ID, n.mode)
	}
	switch n.sendAs {
	case sendAsIndividual, sendAsArray:
	default:
		return fmt.Errorf("file-watch %s: invalid sendAs %q", n.cfg.ID, n.sendAs)
	}
	mask, err := buildOpMask(n.watchEvents)
	if err != nil {
		return fmt.Errorf("file-watch %s: %w", n.cfg.ID, err)
	}
	n.opMask = mask
	if n.glob == "" {
		n.glob = "*"
	}
	if _, err := filepath.Match(n.glob, "test"); err != nil {
		return fmt.Errorf("file-watch %s: invalid glob %q: %w", n.cfg.ID, n.glob, err)
	}
	return nil
}

// Start registers the watcher in watch / read+watch mode. read+watch is
// always incremental, so flowPers is required for that mode regardless
// of the incremental config flag.
func (n *FileWatchNode) Start() error {
	slog.Info("file-watch node started",
		"node_id", n.cfg.ID,
		"mode", n.mode,
		"path", n.path,
	)

	requiresPers := n.incremental || n.mode == fileWatchModeReadWatch
	if requiresPers && n.flowPers == nil {
		n.statusError("no persistent context")
		return fmt.Errorf("file-watch %s: incremental requires persistent flow context", n.cfg.ID)
	}
	if n.mode == fileWatchModeRead {
		return nil
	}

	resolved, err := resolvePath(n.path, nil, n.rootJail)
	if err != nil {
		n.statusError("path error")
		return fmt.Errorf("file-watch %s: %w", n.cfg.ID, err)
	}
	n.resolved = resolved

	w, err := fsnotify.NewWatcher()
	if err != nil {
		n.statusError("watch error")
		return fmt.Errorf("file-watch %s: new watcher: %w", n.cfg.ID, err)
	}
	if err := w.Add(resolved); err != nil {
		_ = w.Close()
		n.statusError("watch error")
		return fmt.Errorf("file-watch %s: add watch: %w", n.cfg.ID, err)
	}
	n.watcher = w

	if requiresPers {
		if err := n.initDirCursor(resolved); err != nil {
			_ = w.Close()
			n.statusError("cursor init error")
			return fmt.Errorf("file-watch %s: %w", n.cfg.ID, err)
		}
	}

	if n.Status != nil {
		n.Status("green", "watching · "+resolved)
	}
	n.wg.Add(1)
	go n.watchLoop()
	return nil
}

// Stop closes the watcher and waits for the goroutine to drain.
func (n *FileWatchNode) Stop() error {
	slog.Info("file-watch node stopped", "node_id", n.cfg.ID)
	if n.mode == fileWatchModeRead {
		return nil
	}
	close(n.done)
	if n.watcher != nil {
		_ = n.watcher.Close()
	}
	n.wg.Wait()
	return nil
}

// HandleMessage triggers a one-shot listing in mode=read. msg.path
// overrides the configured path. msg.resetCursor=true (in incremental
// mode) clears the persisted modTime map.
func (n *FileWatchNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil || n.mode != fileWatchModeRead {
		return nil, nil
	}

	folder, err := n.resolveFolderPath(msg)
	if err != nil {
		n.statusError("path error")
		return nil, fmt.Errorf("file-watch: %w", err)
	}

	if reset, _ := msg.Get("resetCursor").(bool); reset && n.incremental {
		key := dirCursorKey(n.cfg.ID, folder)
		if n.flowPers == nil {
			return nil, fmt.Errorf("file-watch: reset requires persistent context")
		}
		if err := n.flowPers.Delete(key); err != nil {
			n.statusError("cursor reset error")
			return nil, fmt.Errorf("file-watch: reset cursor: %w", err)
		}
		if n.Status != nil {
			n.Status("blue", "cursor reset")
		}
		return nil, nil
	}

	entries, err := scanFolder(folder, n.glob, n.recursive)
	if err != nil {
		n.statusError(folderReadErrorLabel(err))
		return nil, fmt.Errorf("file-watch: %w", err)
	}

	out, emitted, err := n.buildEmissions(entries)
	if err != nil {
		return nil, fmt.Errorf("file-watch: %w", err)
	}
	if n.Status != nil && emitted > 0 {
		n.Status("blue", fmt.Sprintf("read %d", emitted))
	}
	return out, nil
}

// resolveFolderPath honours the msg.path override and applies the jail.
func (n *FileWatchNode) resolveFolderPath(msg *flow.Message) (string, error) {
	if override, ok := msg.Get("path").(string); ok && override != "" {
		return validatePath(override, n.rootJail)
	}
	return validatePath(n.path, n.rootJail)
}

// initDirCursor pins the modTime map at deploy time so existing files are
// either captured (fromStart=false → skip-existing baseline) or emitted
// on first event (fromStart=true → empty baseline).
func (n *FileWatchNode) initDirCursor(absPath string) error {
	key := dirCursorKey(n.cfg.ID, absPath)
	existing, err := loadDirCursor(n.flowPers, key)
	if err != nil {
		return fmt.Errorf("read existing dir cursor: %w", err)
	}
	if existing != nil {
		return nil
	}
	if n.fromStart {
		return saveDirCursor(n.flowPers, key, dirModTimeMap{})
	}
	entries, err := scanFolder(absPath, n.glob, n.recursive)
	if err != nil {
		// A single-file path may not be scannable as a directory — fall
		// back to stat'ing the file itself.
		info, statErr := os.Stat(absPath)
		if statErr == nil && !info.IsDir() {
			return saveDirCursor(n.flowPers, key, dirModTimeMap{
				absPath: info.ModTime().UTC().Format(time.RFC3339Nano),
			})
		}
		return fmt.Errorf("scan for cursor init: %w", err)
	}
	m := make(dirModTimeMap, len(entries))
	for _, e := range entries {
		if !e.IsDir {
			m[e.Path] = e.ModTime
		}
	}
	return saveDirCursor(n.flowPers, key, m)
}

// watchLoop drains fsnotify events with the same inline-debounce pattern
// used by file-read's earlier watch incarnation.
func (n *FileWatchNode) watchLoop() {
	defer n.wg.Done()
	for {
		ev, ok := n.waitForEvent()
		if !ok {
			return
		}
		n.coalesce(ev)
		n.handleWatchTick()
	}
}

func (n *FileWatchNode) waitForEvent() (fsnotify.Event, bool) {
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
			slog.Warn("file-watch watcher error",
				"node_id", n.cfg.ID,
				"path", n.resolved,
				"error", err,
			)
		}
	}
}

// coalesce drains further matching events for debounceMs.
func (n *FileWatchNode) coalesce(_ fsnotify.Event) {
	if n.debounceMs <= 0 {
		return
	}
	deadline := time.NewTimer(time.Duration(n.debounceMs) * time.Millisecond)
	defer deadline.Stop()
	for {
		select {
		case <-deadline.C:
			return
		case <-n.done:
			return
		case ev, ok := <-n.watcher.Events:
			if !ok {
				return
			}
			_ = ev
		case err, ok := <-n.watcher.Errors:
			if !ok {
				return
			}
			slog.Warn("file-watch watcher error",
				"node_id", n.cfg.ID,
				"path", n.resolved,
				"error", err,
			)
		}
	}
}

// handleWatchTick re-scans the folder/file and emits per-mode messages.
func (n *FileWatchNode) handleWatchTick() {
	if n.Send == nil {
		return
	}
	entries, err := scanFolder(n.resolved, n.glob, n.recursive)
	if err != nil {
		// Single-file watch — fall back to stat'ing the path itself.
		info, statErr := os.Stat(n.resolved)
		if statErr != nil {
			slog.Warn("file-watch: scan/stat failed",
				"node_id", n.cfg.ID,
				"path", n.resolved,
				"error", err,
			)
			return
		}
		entries = []folderEntry{makeFolderEntry(n.resolved, info)}
	}

	switch n.mode {
	case fileWatchModeWatch:
		n.dispatchWatch(entries)
	case fileWatchModeReadWatch:
		// Always incremental per spec.
		n.dispatchWatchIncremental(entries)
	}
}

// dispatchWatch emits per-event messages for mode=watch. Without
// incremental it approximates "what just changed" by re-scanning and
// emitting entries whose modTime falls within the recent debounce window.
// Pure watch mode without incremental is inherently approximate.
func (n *FileWatchNode) dispatchWatch(entries []folderEntry) {
	if n.incremental {
		n.dispatchWatchIncremental(entries)
		return
	}
	since := time.Now().Add(-time.Duration(n.debounceMs+250) * time.Millisecond)
	for _, e := range entries {
		modTime, _ := time.Parse(time.RFC3339Nano, e.ModTime)
		if modTime.Before(since) {
			continue
		}
		msg := flow.NewMessage()
		msg.SetTopic(e.Path)
		msg.Set("event", "write")
		msg.Set("filename", e.Path)
		msg.SetPayload(entryToMap(e))
		n.Send(0, msg)
	}
}

// dispatchWatchIncremental compares against the persisted modTime map and
// emits one message per change. Deletions are emitted when "remove" is
// in the watchEvents whitelist.
func (n *FileWatchNode) dispatchWatchIncremental(entries []folderEntry) {
	key := dirCursorKey(n.cfg.ID, n.resolved)
	stored, err := loadDirCursor(n.flowPers, key)
	if err != nil {
		slog.Warn("file-watch: load dir cursor failed",
			"node_id", n.cfg.ID,
			"error", err,
		)
		return
	}
	if stored == nil {
		stored = dirModTimeMap{}
	}

	current := make(map[string]bool, len(entries))
	for _, e := range entries {
		if e.IsDir {
			continue
		}
		current[e.Path] = true
		prev, hadPrev := stored[e.Path]
		if !hadPrev {
			n.emitChangeMsg(e, "create", "")
			continue
		}
		if e.ModTime > prev {
			n.emitChangeMsg(e, "write", prev)
		}
	}

	if containsString(n.watchEvents, "remove") {
		for path, prev := range stored {
			if current[path] {
				continue
			}
			missing := folderEntry{
				Name: filepath.Base(path),
				Path: path,
			}
			n.emitChangeMsg(missing, "remove", prev)
		}
	}

	newMap := make(dirModTimeMap, len(entries))
	for _, e := range entries {
		if !e.IsDir {
			newMap[e.Path] = e.ModTime
		}
	}
	_ = saveDirCursor(n.flowPers, key, newMap)
}

// emitChangeMsg packages one entry change into a flow message and sends
// it on the output port. Both msg.filename and msg.path are set so the
// downstream file-read can pick up the changed path either way.
func (n *FileWatchNode) emitChangeMsg(e folderEntry, eventName, previousModTime string) {
	msg := flow.NewMessage()
	msg.SetTopic(e.Path)
	msg.Set("event", eventName)
	msg.Set("changed", true)
	msg.Set("filename", e.Path)
	if previousModTime != "" {
		msg.Set("previousModTime", previousModTime)
	}
	msg.SetPayload(entryToMap(e))
	n.Send(0, msg)
}

// buildEmissions packages a scanned entry list into the engine output
// envelope based on sendAs. For incremental mode the entries are
// pre-filtered against the stored map.
func (n *FileWatchNode) buildEmissions(entries []folderEntry) ([][]*flow.Message, int, error) {
	if n.incremental {
		filtered, isFirst, err := n.filterIncremental(entries)
		if err != nil {
			return nil, 0, err
		}
		if isFirst && !n.fromStart {
			return nil, 0, nil
		}
		entries = filtered
	}

	if len(entries) == 0 {
		return nil, 0, nil
	}

	if n.sendAs == sendAsArray {
		msg := flow.NewMessage()
		arr := make([]any, len(entries))
		for i, e := range entries {
			arr[i] = entryToMap(e)
		}
		msg.SetPayload(arr)
		return [][]*flow.Message{{msg}}, len(entries), nil
	}

	messages := make([]*flow.Message, len(entries))
	total := len(entries)
	for i, e := range entries {
		msg := flow.NewMessage()
		msg.SetTopic(e.Path)
		msg.SetPayload(entryToMap(e))
		msg.Set("filename", e.Path)
		msg.Set("isFirst", i == 0)
		msg.Set("isLast", i == total-1)
		msg.Set("index", i)
		msg.Set("total", total)
		messages[i] = msg
	}
	return [][]*flow.Message{messages}, total, nil
}

// filterIncremental returns only entries whose modTime advanced since the
// stored map, persists the new snapshot, and reports whether this was
// the first access.
func (n *FileWatchNode) filterIncremental(entries []folderEntry) ([]folderEntry, bool, error) {
	key := dirCursorKey(n.cfg.ID, n.resolved)
	if n.resolved == "" {
		key = dirCursorKey(n.cfg.ID, n.path)
	}
	stored, err := loadDirCursor(n.flowPers, key)
	if err != nil {
		return nil, false, err
	}
	isFirst := stored == nil
	if isFirst {
		stored = dirModTimeMap{}
	}

	var changed []folderEntry
	for _, e := range entries {
		if e.IsDir {
			continue
		}
		prev, hadPrev := stored[e.Path]
		if !hadPrev || e.ModTime > prev {
			ee := e
			ee.PreviousModTime = prev
			ee.Changed = true
			changed = append(changed, ee)
		}
	}

	newMap := make(dirModTimeMap, len(entries))
	for _, e := range entries {
		if !e.IsDir {
			newMap[e.Path] = e.ModTime
		}
	}
	if err := saveDirCursor(n.flowPers, key, newMap); err != nil {
		return nil, false, err
	}
	return changed, isFirst, nil
}

// folderEntry is the in-memory representation of one scanned entry.
// Content fields removed in the file-watch revision — this node never
// reads file contents.
type folderEntry struct {
	Name            string
	Path            string
	Size            int64
	ModTime         string // RFC3339Nano
	IsDir           bool
	Mode            string // octal "0644"
	Changed         bool
	PreviousModTime string
}

// entryToMap builds the JSON-friendly map shape used as msg.payload.
func entryToMap(e folderEntry) map[string]any {
	return map[string]any{
		"name":    e.Name,
		"path":    e.Path,
		"size":    e.Size,
		"modTime": e.ModTime,
		"isDir":   e.IsDir,
		"mode":    e.Mode,
	}
}

// scanFolder walks a folder honouring recursive + glob.
func scanFolder(absPath, glob string, recursive bool) ([]folderEntry, error) {
	if recursive {
		return scanRecursive(absPath, glob)
	}
	return scanFlat(absPath, glob)
}

func scanFlat(absPath, glob string) ([]folderEntry, error) {
	items, err := os.ReadDir(absPath)
	if err != nil {
		return nil, err
	}
	out := make([]folderEntry, 0, len(items))
	for _, item := range items {
		if !matchGlob(item, glob) {
			continue
		}
		info, err := item.Info()
		if err != nil {
			continue
		}
		out = append(out, makeFolderEntry(filepath.Join(absPath, item.Name()), info))
	}
	sortEntries(out)
	return out, nil
}

func scanRecursive(absPath, glob string) ([]folderEntry, error) {
	var out []folderEntry
	err := filepath.WalkDir(absPath, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if p == absPath {
			return nil
		}
		if !matchGlob(d, glob) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		out = append(out, makeFolderEntry(p, info))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortEntries(out)
	return out, nil
}

func matchGlob(d fs.DirEntry, glob string) bool {
	if d.IsDir() {
		return true
	}
	ok, _ := filepath.Match(glob, d.Name())
	return ok
}

func makeFolderEntry(path string, info os.FileInfo) folderEntry {
	return folderEntry{
		Name:    info.Name(),
		Path:    path,
		Size:    info.Size(),
		ModTime: info.ModTime().UTC().Format(time.RFC3339Nano),
		IsDir:   info.IsDir(),
		Mode:    fmt.Sprintf("%#o", info.Mode().Perm()),
	}
}

func sortEntries(entries []folderEntry) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
}

// folderReadErrorLabel maps a read/scan error to a stable status label.
func folderReadErrorLabel(err error) string {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "not found"
	case errors.Is(err, os.ErrPermission):
		return "permission denied"
	default:
		return "scan error"
	}
}

func (n *FileWatchNode) statusError(label string) {
	if n.Status != nil {
		n.Status("red", label)
	}
}

// containsString is a tiny helper used by the watchEvents whitelist check.
func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
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
	return mask, nil
}

// FileWatchTypeInfo returns the NodeTypeInfo for registering the file-watch node.
func FileWatchTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "file-watch",
		Category:    "filesystem",
		Label:       "File Watch",
		Description: "Watch a file or folder for changes (metadata only)",
		Icon:        "folder-in",
		Defaults: map[string]any{
			"mode":        "watch",
			"path":        "",
			"recursive":   false,
			"glob":        "*",
			"watchEvents": []string{"create", "write", "remove", "rename"},
			"sendAs":      "individual",
			"debounceMs":  100,
			"incremental": false,
			"fromStart":   false,
		},
		Inputs:  1,
		Outputs: 1,
	}
}
