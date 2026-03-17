// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

// Package ws provides a WebSocket hub for real-time communication between
// the Flint backend and connected editor clients. It manages client
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
}

// Hub maintains the set of active WebSocket clients and broadcasts messages
// to all of them. It is the central coordination point for real-time events
// flowing from the Flint runtime to the editor frontend.
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
					// Client send buffer is full; drop it.
					h.mu.RUnlock()
					h.mu.Lock()
					delete(h.clients, client)
					close(client.send)
					h.mu.Unlock()
					h.mu.RLock()
					slog.Warn("websocket client dropped (slow consumer)", "client", client.id)
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

// ServeWS handles a WebSocket upgrade request from an HTTP client. It upgrades
// the connection, registers the client with the hub, and starts the read/write
// pump goroutines. This should be mounted as an HTTP handler:
//
//	router.Get("/ws", hub.ServeWS)
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("websocket upgrade failed", "error", err, "remote", r.RemoteAddr)
		return
	}

	client := &Client{
		hub:  h,
		conn: conn,
		send: make(chan []byte, 256),
		id:   r.RemoteAddr,
	}

	h.register <- client

	// Start the write pump in a goroutine; the read pump runs in the
	// current goroutine (which is the HTTP handler goroutine).
	go client.writePump()
	go client.readPump()
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
