// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

// Package ws provides a WebSocket hub for real-time communication between
// the LOOPZE backend and connected editor clients. It manages client
// connections, broadcasts events (debug messages, deploy status, node status,
// notifications), and handles graceful disconnection.
package ws

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// writeWait is the maximum time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// pongWait is the maximum time to wait for a pong response from the peer.
	pongWait = 60 * time.Second

	// pingPeriod sends pings at this interval. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// maxMessageSize is the maximum message size allowed from the peer.
	maxMessageSize = 512 * 1024 // 512 KB
)

// Event types sent over the WebSocket connection.
const (
	EventDebug        = "debug"
	EventStatus       = "status"
	EventDeploy       = "deploy"
	EventNotification = "notification"
	EventLog          = "log"
)

// upgrader configures the WebSocket upgrade from HTTP. CheckOrigin allows all
// origins in development; this should be tightened for production deployments.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// TODO: Restrict origins in production.
		return true
	},
}

// Event represents a message sent to or received from a WebSocket client.
type Event struct {
	// Type identifies the event category (e.g. "debug", "status", "deploy", "notification").
	Type string `json:"type"`

	// Payload carries the event-specific payload.
	Payload any `json:"payload"`

	// Timestamp records when the event was created.
	Timestamp time.Time `json:"timestamp"`
}

// Client represents a single WebSocket connection to the hub.
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
	id   string

	// userID identifies the authenticated user behind this connection.
	// Set during ServeWSAuthed; empty for anonymous clients (only
	// possible when auth is disabled).
	userID string
}

// Hub maintains the set of active WebSocket clients and broadcasts messages
// to all of them. It is the central coordination point for real-time events
// flowing from the LOOPZE runtime to the editor frontend.
type Hub struct {
	// clients holds all currently connected clients.
	clients map[*Client]bool

	// broadcast is the channel for messages that should be sent to all clients.
	broadcast chan []byte

	// register is the channel for new client connections.
	register chan *Client

	// unregister is the channel for client disconnections.
	unregister chan *Client

	// mu protects the clients map for concurrent reads outside the Run loop.
	mu sync.RWMutex

	// done signals the Run loop to stop.
	done chan struct{}
}

// NewHub creates a new WebSocket Hub. Call Run() in a goroutine to start
// processing client registrations and broadcasts.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		done:       make(chan struct{}),
	}
}

// Run is the main event loop for the Hub. It processes client registration,
// unregistration, and broadcast messages. It should be started as a goroutine:
//
//	go hub.Run()
func (h *Hub) Run() {
	slog.Info("websocket hub started")
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			count := len(h.clients)
			h.mu.Unlock()
			slog.Info("websocket client connected", "client", client.id, "total", count)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			count := len(h.clients)
			h.mu.Unlock()
			slog.Info("websocket client disconnected", "client", client.id, "total", count)

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// Client send buffer is full; drop it. Re-check
					// membership under the write lock so a concurrent
					// DisconnectUser cannot cause a double-close.
					h.mu.RUnlock()
					h.mu.Lock()
					if _, ok := h.clients[client]; ok {
						delete(h.clients, client)
						close(client.send)
						slog.Warn("websocket client dropped (slow consumer)", "client", client.id)
					}
					h.mu.Unlock()
					h.mu.RLock()
				}
			}
			h.mu.RUnlock()

		case <-h.done:
			slog.Info("websocket hub stopping")
			h.mu.Lock()
			for client := range h.clients {
				close(client.send)
				delete(h.clients, client)
			}
			h.mu.Unlock()
			return
		}
	}
}

// Stop gracefully shuts down the hub, closing all client connections.
func (h *Hub) Stop() {
	close(h.done)
}

// Broadcast sends an event to all connected WebSocket clients. The event is
// serialized to JSON before being dispatched. This is the primary method for
// the runtime to push real-time updates to the frontend.
//
// Supported event types: "debug", "status", "deploy", "notification".
func (h *Hub) Broadcast(eventType string, data any) {
	event := Event{
		Type:      eventType,
		Payload:   data,
		Timestamp: time.Now().UTC(),
	}

	msg, err := json.Marshal(event)
	if err != nil {
		slog.Error("failed to marshal websocket event", "type", eventType, "error", err)
		return
	}

	select {
	case h.broadcast <- msg:
	default:
		slog.Warn("websocket broadcast channel full, dropping event", "type", eventType)
	}
}

// ClientCount returns the number of currently connected WebSocket clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// AuthFunc is the signature of an authentication hook used by
// ServeWSAuthed. It runs *before* the WebSocket upgrade. Returning a
// non-empty userID accepts the request; an empty userID rejects it
// with HTTP 401 and the error (if non-nil) is logged.
type AuthFunc func(r *http.Request) (userID string, err error)

// ServeWS handles a WebSocket upgrade request without authentication. It
// is intended only for development setups (LOOPZE_AUTH_DISABLE) — in
// normal operation use ServeWSAuthed.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	h.upgradeAndRun(w, r, "")
}

// ServeWSAuthed returns an http.HandlerFunc that authenticates the
// upgrade request using authFn before performing the WebSocket upgrade.
// Failed auth produces a plain 401 (no upgrade), so misbehaving clients
// cannot keep an open connection without credentials.
func (h *Hub) ServeWSAuthed(authFn AuthFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := authFn(r)
		if err != nil {
			slog.Warn("websocket auth error", "error", err, "remote", r.RemoteAddr)
		}
		if userID == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h.upgradeAndRun(w, r, userID)
	}
}

// upgradeAndRun performs the WebSocket upgrade and starts the read/write
// pumps. userID is attached to the Client for later targeted teardown.
func (h *Hub) upgradeAndRun(w http.ResponseWriter, r *http.Request, userID string) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("websocket upgrade failed", "error", err, "remote", r.RemoteAddr)
		return
	}

	client := &Client{
		hub:    h,
		conn:   conn,
		send:   make(chan []byte, 256),
		id:     r.RemoteAddr,
		userID: userID,
	}

	h.register <- client

	// Start the write pump in a goroutine; the read pump runs in the
	// current goroutine (which is the HTTP handler goroutine).
	go client.writePump()
	go client.readPump()
}

// DisconnectUser closes every WebSocket connection that belongs to the
// given user. Used when a user logs out, is disabled, or has their
// password reset — the open WS sessions must terminate immediately so
// the user does not keep receiving live events on stale credentials.
//
// Closing the send channel makes the write pump send a CloseMessage and
// drop the underlying conn, which in turn causes the read pump to exit
// and unregister the client through the normal path.
func (h *Hub) DisconnectUser(userID string) {
	if userID == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	closed := 0
	for client := range h.clients {
		if client.userID != userID {
			continue
		}
		delete(h.clients, client)
		close(client.send)
		closed++
	}
	if closed > 0 {
		slog.Info("websocket clients disconnected for user", "user_id", userID, "count", closed)
	}
}

// readPump reads messages from the WebSocket connection. It runs in its own
// goroutine per client. When the connection is closed or an error occurs, the
// client is unregistered from the hub.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	if err := c.conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		slog.Error("failed to set read deadline", "client", c.id, "error", err)
		return
	}
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Warn("websocket read error", "client", c.id, "error", err)
			}
			break
		}

		// Handle incoming messages from the client.
		// For now, log them. In the future, this will dispatch commands
		// (e.g. subscribe to specific debug nodes, trigger inject, etc.).
		slog.Debug("websocket message received", "client", c.id, "size", len(message))
	}
}

// writePump pumps messages from the hub to the WebSocket connection. It runs
// in its own goroutine per client. It also sends periodic ping frames to keep
// the connection alive.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				slog.Error("failed to set write deadline", "client", c.id, "error", err)
				return
			}
			if !ok {
				// The hub closed the channel.
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				slog.Error("websocket write error", "client", c.id, "error", err)
				return
			}

		case <-ticker.C:
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				slog.Error("failed to set write deadline for ping", "client", c.id, "error", err)
				return
			}
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
