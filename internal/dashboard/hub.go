// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package dashboard

import (
	"crypto/sha256"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/loopzedev/loopze-edge/internal/flow"
	"github.com/loopzedev/loopze-edge/internal/ws"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 256 * 1024 // 256 KB — generous for input events
)

// Message types exchanged over the dashboard WebSocket.
const (
	// Server → client.
	msgTypeSnapshot = "snapshot"
	msgTypeWidget   = "widget"
	msgTypeDeploy   = "deploy" // PR 5
	msgTypeError    = "error"

	// Client → server.
	msgTypeHello = "hello"
	msgTypeEvent = "event"
)

// Hub is the dashboard's WebSocket hub. It mirrors internal/ws.Hub but
// carries a dashboard-specific message protocol, owns the per-widget
// last-value cache, and routes browser-side widget events to registered
// input widget callbacks.
//
// Hub satisfies flow.DashboardHub.
type Hub struct {
	upgrader websocket.Upgrader

	// clients and the broadcast / register / unregister channels follow
	// the same shape as internal/ws.Hub. The hub.Run goroutine owns the
	// clients map; outside callers reach it only through these channels.
	clients    map[*client]bool
	broadcast  chan []byte
	register   chan *client
	unregister chan *client
	done       chan struct{}
	mu         sync.RWMutex

	// cache stores the last value per widget. Replayed in the snapshot
	// handshake so freshly connected clients are never blank.
	cache *Cache

	// snapshot holds the latest dashboard layout. Written by
	// RebuildLayout (engine deploy callback) and read by the snapshot
	// handshake and Snapshot() accessor. atomic.Pointer keeps the
	// read path lock-free.
	snapshot atomic.Pointer[Snapshot]

	// lastLayoutHash is the SHA-256 of the most recently broadcast
	// layout snapshot. Used by RebuildLayout to set the
	// `layoutChanged` flag on the deploy frame. snapMu serialises
	// RebuildLayout calls so concurrent deploys can't interleave the
	// hash compare-and-swap with the broadcast.
	lastLayoutHash [32]byte
	snapMu         sync.Mutex

	// inputRegistry maps widget node IDs to the callback installed by
	// the corresponding input widget node in its Start method. Guarded
	// by inputMu rather than the broadcast lock so widget registration
	// does not contend with message fan-out.
	//
	// Each entry carries a monotonic sequence number so a late Stop
	// from an old node generation cannot clobber a freshly registered
	// callback after a re-deploy.
	inputMu       sync.RWMutex
	inputRegistry map[string]inputReg
	inputSeq      atomic.Uint64

	// socketSeq feeds WidgetClient.SocketID — a per-process monotonic
	// counter so input events can be correlated across the WS hop.
	socketSeq atomic.Uint64
}

// inputReg is the value stored in inputRegistry. seq distinguishes
// successive registrations of the same nodeID so unregister can be a
// no-op when the callback has already been replaced.
type inputReg struct {
	fn  func(flow.WidgetEvent)
	seq uint64
}

// client is a single dashboard browser connection. Compared to the
// editor hub's Client, it also tracks the SocketID used in
// WidgetClient.SocketID on input events.
type client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan []byte
	id       string // RemoteAddr
	userID   string // empty when ui-base.auth=none → anonUserID
	socketID string // per-process monotonic counter
}

// NewHub constructs a dashboard hub. allowedOrigins follows the same
// allowlist semantics as the editor hub (empty → same-origin only).
func NewHub(allowedOrigins []string) *Hub {
	return &Hub{
		upgrader:      ws.NewUpgrader(allowedOrigins),
		clients:       make(map[*client]bool),
		broadcast:     make(chan []byte, 256),
		register:      make(chan *client),
		unregister:    make(chan *client),
		done:          make(chan struct{}),
		cache:         NewCache(),
		inputRegistry: make(map[string]inputReg),
	}
}

// Run is the hub's main loop. Start it as a goroutine before any WS
// upgrade can land.
func (h *Hub) Run() {
	slog.Info("dashboard hub started")
	for {
		select {
		case c := <-h.register:
			h.mu.Lock()
			h.clients[c] = true
			count := len(h.clients)
			h.mu.Unlock()
			slog.Info("dashboard client connected",
				"client", c.id, "user", c.userID, "total", count)

		case c := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
			}
			count := len(h.clients)
			h.mu.Unlock()
			slog.Info("dashboard client disconnected",
				"client", c.id, "total", count)

		case msg := <-h.broadcast:
			h.mu.RLock()
			for c := range h.clients {
				select {
				case c.send <- msg:
				default:
					// Slow consumer: drop under the write lock to avoid
					// double-close races with concurrent disconnects.
					h.mu.RUnlock()
					h.mu.Lock()
					if _, ok := h.clients[c]; ok {
						delete(h.clients, c)
						close(c.send)
						slog.Warn("dashboard client dropped (slow consumer)", "client", c.id)
					}
					h.mu.Unlock()
					h.mu.RLock()
				}
			}
			h.mu.RUnlock()

		case <-h.done:
			slog.Info("dashboard hub stopping")
			h.mu.Lock()
			for c := range h.clients {
				close(c.send)
				delete(h.clients, c)
			}
			h.mu.Unlock()
			return
		}
	}
}

// Stop gracefully shuts down the hub.
func (h *Hub) Stop() {
	close(h.done)
}

// ClientCount returns the number of currently connected dashboard clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// ─── Snapshot / deploy hook ─────────────────────────────────────────────────

// RebuildLayout replaces the stored layout snapshot, prunes the cache
// of widgets that are no longer present, and broadcasts a deploy frame
// to every connected client. Called by the engine's DeployListener
// after every successful Deploy.
//
// Layout-change detection: a SHA-256 of the JSON-serialized snapshot
// is compared against the previous deploy's hash. The deploy frame is
// broadcast either way (so clients can confirm "deploy fired"), but
// the inline layout payload is only attached when the hash changed —
// see DASHBOARD_HOT_RELOAD.md for the locked rationale.
func (h *Hub) RebuildLayout(ws flow.Workspace) {
	snap := BuildLayout(ws)
	h.snapshot.Store(snap)
	h.cache.Retain(snap.WidgetIDs())

	h.snapMu.Lock()
	defer h.snapMu.Unlock()
	hash := snapshotHash(snap)
	changed := hash != h.lastLayoutHash
	h.lastLayoutHash = hash
	h.broadcastDeploy(snap, changed)
}

// snapshotHash returns a stable SHA-256 over the JSON-serialized
// snapshot. Determinism notes:
//   - encoding/json walks struct fields in declaration order.
//   - Map keys are emitted sorted (Go std library guarantee).
//   - Map[string]any values inside LayoutWidget.Config inherit the
//     same sort guarantee.
//
// A marshal error is impossible in practice (Snapshot contains no
// channels/funcs); on the off-chance one slips in, we return the zero
// hash, which compares equal to the previous zero hash and suppresses
// the layout payload — fail-safe behaviour.
func snapshotHash(s *Snapshot) [32]byte {
	buf, err := json.Marshal(s)
	if err != nil {
		slog.Error("dashboard: snapshot hash marshal failed", "error", err)
		return [32]byte{}
	}
	return sha256.Sum256(buf)
}

// broadcastDeploy sends a deploy frame to every connected client. The
// layout payload is attached only when changed=true (see L-1 / L-7 in
// DASHBOARD_HOT_RELOAD.md).
func (h *Hub) broadcastDeploy(snap *Snapshot, changed bool) {
	frame := deployFrame{
		Type:          msgTypeDeploy,
		LayoutChanged: changed,
	}
	if changed {
		frame.Layout = snap
	}
	h.broadcastJSON(frame)
}

// Snapshot returns the current layout snapshot. Always non-nil; before
// the first Deploy it returns an empty snapshot.
func (h *Hub) Snapshot() *Snapshot {
	if s := h.snapshot.Load(); s != nil {
		return s
	}
	return &Snapshot{
		Pages:   []LayoutPage{},
		Groups:  []LayoutGroup{},
		Widgets: []LayoutWidget{},
	}
}

// AuthMode reports the current ui-base.auth value. Returns "session"
// when no ui-base has been deployed yet (locked-down default).
func (h *Hub) AuthMode() string {
	snap := h.snapshot.Load()
	if snap == nil || snap.Base == nil {
		return "session"
	}
	if snap.Base.Auth == "" {
		return "session"
	}
	return snap.Base.Auth
}

// ─── flow.DashboardHub implementation ───────────────────────────────────────

// PushWidgetValue caches the latest value and broadcasts a widget
// update to every connected client.
func (h *Hub) PushWidgetValue(nodeID string, value any, ts time.Time) {
	if ts.IsZero() {
		ts = time.Now()
	}
	h.cache.Put(nodeID, value, ts)

	frame := serverFrame{
		Type:  msgTypeWidget,
		ID:    nodeID,
		Value: value,
		TS:    ts.UnixMilli(),
	}
	h.broadcastJSON(frame)
}

// RegisterInputWidget registers an input widget's emit callback and
// returns an unregister func the widget must call on Stop. The
// returned unregister is generation-aware: if a later registration has
// already replaced this entry (re-deploy of the same node), calling
// unregister is a safe no-op.
func (h *Hub) RegisterInputWidget(nodeID string, fn func(flow.WidgetEvent)) (unregister func()) {
	seq := h.inputSeq.Add(1)
	h.inputMu.Lock()
	h.inputRegistry[nodeID] = inputReg{fn: fn, seq: seq}
	h.inputMu.Unlock()
	return func() {
		h.inputMu.Lock()
		defer h.inputMu.Unlock()
		if cur, ok := h.inputRegistry[nodeID]; ok && cur.seq == seq {
			delete(h.inputRegistry, nodeID)
		}
	}
}

// dispatchEvent routes an incoming WS event to the registered input
// widget callback. Returns false if no widget with that ID is
// registered.
func (h *Hub) dispatchEvent(c *client, id string, value any) bool {
	h.inputMu.RLock()
	reg, ok := h.inputRegistry[id]
	h.inputMu.RUnlock()
	if !ok {
		return false
	}
	reg.fn(flow.WidgetEvent{
		Value: value,
		Client: flow.WidgetClient{
			UserID:    c.userID,
			SocketID:  c.socketID,
			SessionID: "", // populated in Phase 3 when ACLs land
		},
		TS: time.Now(),
	})
	return true
}

// ─── WS upgrade / read+write pumps ──────────────────────────────────────────

// ServeWSAuthed returns an http.HandlerFunc that authenticates the
// upgrade via authFn before accepting the connection. Mirror of
// ws.Hub.ServeWSAuthed for the dashboard hub's message protocol.
func (h *Hub) ServeWSAuthed(authFn ws.AuthFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := authFn(r)
		if err != nil {
			slog.Warn("dashboard ws auth error", "error", err, "remote", r.RemoteAddr)
		}
		if userID == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h.upgradeAndRun(w, r, userID)
	}
}

func (h *Hub) upgradeAndRun(w http.ResponseWriter, r *http.Request, userID string) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("dashboard ws upgrade failed", "error", err, "remote", r.RemoteAddr)
		return
	}
	c := &client{
		hub:      h,
		conn:     conn,
		send:     make(chan []byte, 256),
		id:       r.RemoteAddr,
		userID:   userID,
		socketID: nextSocketID(h),
	}
	h.register <- c

	// Send the initial snapshot before starting the pumps so the first
	// frame the client sees is always the layout + cached values.
	h.sendSnapshotTo(c)

	go c.writePump()
	go c.readPump()
}

func nextSocketID(h *Hub) string {
	n := h.socketSeq.Add(1)
	return "ws-" + jsonNumber(n)
}

func jsonNumber(n uint64) string {
	// Tiny inline alternative to strconv.FormatUint to avoid an import
	// just for a one-call use.
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = digits[n%10]
		n /= 10
	}
	return string(buf[i:])
}

// sendSnapshotTo unicasts the current layout + every cached widget
// value to a single client. Used on connect.
func (h *Hub) sendSnapshotTo(c *client) {
	snap := h.Snapshot()
	cache := h.cache.Snapshot()
	frame := snapshotFrame{
		Type:    msgTypeSnapshot,
		Layout:  snap,
		Widgets: cache,
		TS:      time.Now().UnixMilli(),
	}
	data, err := json.Marshal(frame)
	if err != nil {
		slog.Error("dashboard: failed to marshal snapshot frame", "error", err)
		return
	}
	select {
	case c.send <- data:
	default:
		slog.Warn("dashboard: snapshot dropped, client send buffer full", "client", c.id)
	}
}

// broadcastJSON marshals a frame and pushes it onto the broadcast
// channel. Drops the frame on overflow with a warning, same policy as
// the editor hub.
func (h *Hub) broadcastJSON(frame any) {
	data, err := json.Marshal(frame)
	if err != nil {
		slog.Error("dashboard: failed to marshal frame", "error", err)
		return
	}
	select {
	case h.broadcast <- data:
	default:
		slog.Warn("dashboard: broadcast channel full, dropping frame")
	}
}

// readPump consumes incoming frames from the client. Unrecognised
// frames are logged and discarded; malformed frames close the
// connection. The hub treats the read goroutine as the canonical
// "client alive" signal — any error here triggers the unregister path.
func (c *client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	if err := c.conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		slog.Error("dashboard: failed to set read deadline", "client", c.id, "error", err)
		return
	}
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Warn("dashboard ws read error", "client", c.id, "error", err)
			}
			return
		}
		var frame clientFrame
		if err := json.Unmarshal(data, &frame); err != nil {
			slog.Warn("dashboard: invalid client frame", "client", c.id, "error", err)
			continue
		}
		switch frame.Type {
		case msgTypeHello:
			// Snapshot was already sent on connect; hello is currently
			// informational only.
		case msgTypeEvent:
			if frame.ID == "" {
				slog.Warn("dashboard: event frame missing id", "client", c.id)
				continue
			}
			if !c.hub.dispatchEvent(c, frame.ID, frame.Value) {
				c.sendErrorFrame("unknown widget id: " + frame.ID)
			}
		default:
			slog.Debug("dashboard: ignoring unknown frame type",
				"client", c.id, "type", frame.Type)
		}
	}
}

// writePump streams frames from c.send onto the wire and emits periodic
// pings to keep NAT mappings alive. Mirror of ws.Client.writePump.
func (c *client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				slog.Error("dashboard: failed to set write deadline", "client", c.id, "error", err)
				return
			}
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				slog.Error("dashboard ws write error", "client", c.id, "error", err)
				return
			}
		case <-ticker.C:
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				return
			}
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// sendErrorFrame unicasts a small error frame to a single client.
// Used to surface "unknown widget id" and similar non-fatal protocol
// errors.
func (c *client) sendErrorFrame(message string) {
	frame := errorFrame{Type: msgTypeError, Message: message}
	data, err := json.Marshal(frame)
	if err != nil {
		return
	}
	select {
	case c.send <- data:
	default:
	}
}

// ─── Wire frames ────────────────────────────────────────────────────────────

type serverFrame struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Value any    `json:"value"`
	TS    int64  `json:"ts"`
}

type snapshotFrame struct {
	Type    string           `json:"type"`
	Layout  *Snapshot        `json:"layout"`
	Widgets map[string]Entry `json:"widgets"`
	TS      int64            `json:"ts"`
}

type errorFrame struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type deployFrame struct {
	Type          string    `json:"type"`
	LayoutChanged bool      `json:"layoutChanged"`
	Layout        *Snapshot `json:"layout,omitempty"`
}

type clientFrame struct {
	Type  string `json:"type"`
	ID    string `json:"id,omitempty"`
	Value any    `json:"value,omitempty"`
}
