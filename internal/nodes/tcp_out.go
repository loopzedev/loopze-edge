// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// Modes for tcp-out.
const (
	tcpOutModeReply           = "reply"
	tcpOutModeServerBroadcast = "server-broadcast"
	tcpOutModeClient          = "client"
)

const (
	defaultTcpOutWriteTimeout      = 10 * time.Second
	defaultTcpOutDialTimeout       = 10 * time.Second
	defaultTcpOutQueueSize         = 64
)

// TCPOutNode writes msg.payload to a TCP peer. The selected mode
// determines how the destination is found:
//
//   - reply: msg.session is resolved against the engine's
//     SessionRegistry; the underlying net.Conn is written under its
//     mutex so concurrent fan-out from upstream branches is serialised.
//   - server-broadcast: every session whose owner equals targetTcpIn is
//     written to in turn. Per-target write errors are logged but do not
//     stop the broadcast (best-effort fan-out).
//   - client: dial host:port (msg.host / msg.port override) and write.
//     With keepConnection=true a single persistent connection is reused
//     across messages; backpressure is bounded via the outbound channel
//     and overflow surfaces as a catchable error.
type TCPOutNode struct {
	cfg flow.NodeConfig

	send    flow.SendFunc
	status  flow.StatusFunc
	debug   flow.DebugFunc
	errorFn flow.ErrorFunc

	// Injected by the engine.
	sessions *flow.SessionRegistry

	// Parsed configuration.
	mode             string
	host             string
	port             int
	targetTcpIn      string
	keepConnection   bool
	appendDelimiter  []byte
	closeAfterSend   bool
	dialTimeout      time.Duration
	writeTimeout     time.Duration
	outboundQueueSz  int
	tlsConfig        *tls.Config // client-mode only; nil when tls is disabled

	// Client-keep-connection state.
	clientMu     sync.Mutex
	clientConn   net.Conn
	clientStop   chan struct{}
	clientQueue  chan tcpOutboundMsg
	clientWg     sync.WaitGroup
	clientStarted bool
}

// tcpOutboundMsg is the unit queued for the client-keep worker. The
// origMsg pointer lets us route catchable errors back through the
// engine with the originating message attached.
type tcpOutboundMsg struct {
	body         []byte
	closeAfter   bool
	origMsg      *flow.Message
}

// NewTCPOutNode is the NodeFactory for the "tcp-out" node type.
func NewTCPOutNode(cfg flow.NodeConfig) (flow.NodeInstance, error) {
	return &TCPOutNode{cfg: cfg}, nil
}

func (n *TCPOutNode) Init() error {
	props := n.cfg.Properties

	n.mode = strings.ToLower(stringVal(props, "mode", tcpOutModeReply))
	switch n.mode {
	case tcpOutModeReply, tcpOutModeServerBroadcast, tcpOutModeClient:
	default:
		return fmt.Errorf("tcp-out %s: invalid mode %q", n.cfg.ID, n.mode)
	}

	if n.mode == tcpOutModeClient {
		n.host = strings.TrimSpace(stringVal(props, "host", ""))
		n.port = intVal(props, "port", 0)
		// Allow empty host/port at config time as long as msg.host /
		// msg.port will provide them at HandleMessage time. Validate
		// per-message.
	}

	if n.mode == tcpOutModeServerBroadcast {
		n.targetTcpIn = strings.TrimSpace(stringVal(props, "targetTcpIn", ""))
		if n.targetTcpIn == "" {
			return fmt.Errorf("tcp-out %s: server-broadcast requires targetTcpIn", n.cfg.ID)
		}
	}

	n.keepConnection = true
	if v, ok := props["keepConnection"].(bool); ok {
		n.keepConnection = v
	}

	if raw := stringVal(props, "appendDelimiter", ""); raw != "" {
		bs, err := ParseDelimiter(raw)
		if err != nil {
			return fmt.Errorf("tcp-out %s: appendDelimiter: %w", n.cfg.ID, err)
		}
		n.appendDelimiter = bs
	}

	if v, ok := props["closeAfterSend"].(bool); ok {
		n.closeAfterSend = v
	}

	dialSec := intVal(props, "dialTimeout", int(defaultTcpOutDialTimeout/time.Second))
	if dialSec <= 0 {
		dialSec = int(defaultTcpOutDialTimeout / time.Second)
	}
	n.dialTimeout = time.Duration(dialSec) * time.Second

	writeSec := intVal(props, "writeTimeout", int(defaultTcpOutWriteTimeout/time.Second))
	if writeSec <= 0 {
		writeSec = int(defaultTcpOutWriteTimeout / time.Second)
	}
	n.writeTimeout = time.Duration(writeSec) * time.Second

	n.outboundQueueSz = intVal(props, "outboundQueueSize", defaultTcpOutQueueSize)
	if n.outboundQueueSz <= 0 {
		n.outboundQueueSz = defaultTcpOutQueueSize
	}

	if n.mode == tcpOutModeClient {
		tlsCfg, err := ParseTLSBlock(props, n.cfg.ID)
		if err != nil {
			return fmt.Errorf("tcp-out %s: %w", n.cfg.ID, err)
		}
		n.tlsConfig = tlsCfg
	}

	return nil
}

// dial opens a TCP connection (client mode) — wrapped in TLS when
// configured.
func (n *TCPOutNode) dial(addr string) (net.Conn, error) {
	if n.tlsConfig == nil {
		return net.DialTimeout("tcp", addr, n.dialTimeout)
	}
	d := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: n.dialTimeout},
		Config:    n.tlsConfig,
	}
	return d.Dial("tcp", addr)
}

func (n *TCPOutNode) SetSend(fn flow.SendFunc)     { n.send = fn }
func (n *TCPOutNode) SetStatus(fn flow.StatusFunc) { n.status = fn }
func (n *TCPOutNode) SetDebug(fn flow.DebugFunc)   { n.debug = fn }
func (n *TCPOutNode) SetError(fn flow.ErrorFunc)   { n.errorFn = fn }

// SetSessionRegistry implements flow.SessionRegistryProvider.
func (n *TCPOutNode) SetSessionRegistry(r *flow.SessionRegistry) { n.sessions = r }

func (n *TCPOutNode) Start() error {
	if n.mode == tcpOutModeClient && n.keepConnection {
		n.clientStop = make(chan struct{})
		n.clientQueue = make(chan tcpOutboundMsg, n.outboundQueueSz)
		n.clientStarted = true
		n.clientWg.Add(1)
		go n.clientKeepWorker()
	}
	return nil
}

func (n *TCPOutNode) Stop() error {
	if n.clientStarted {
		close(n.clientStop)
		n.clientMu.Lock()
		if n.clientConn != nil {
			_ = n.clientConn.Close()
		}
		n.clientMu.Unlock()
		n.clientWg.Wait()
	}
	slog.Info("tcp-out stopped", "node_id", n.cfg.ID)
	return nil
}

// HandleMessage dispatches per the configured mode. Returns nil
// outputs (sink node) in the happy path; errors are catchable.
func (n *TCPOutNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil {
		return nil, nil
	}
	body, err := encodeOutboundBody(msg)
	if err != nil {
		return nil, fmt.Errorf("tcp-out %s: encode: %w", n.cfg.ID, err)
	}
	if len(n.appendDelimiter) > 0 {
		body = append(body, n.appendDelimiter...)
	}
	closeAfter := n.closeAfterSend
	if v, ok := msg.Get("closeAfterSend").(bool); ok {
		closeAfter = v
	}

	switch n.mode {
	case tcpOutModeReply:
		return nil, n.handleReply(msg, body, closeAfter)
	case tcpOutModeServerBroadcast:
		return nil, n.handleBroadcast(body)
	case tcpOutModeClient:
		return nil, n.handleClient(msg, body, closeAfter)
	}
	return nil, nil
}

// ─── Reply mode ─────────────────────────────────────────────────────────────

func (n *TCPOutNode) handleReply(msg *flow.Message, body []byte, closeAfter bool) error {
	handle, ok := msg.Get("session").(*flow.SessionHandle)
	if !ok || handle == nil {
		return fmt.Errorf("tcp-out %s: no session handle on msg.session", n.cfg.ID)
	}
	if n.sessions == nil {
		return fmt.Errorf("tcp-out %s: session registry not wired", n.cfg.ID)
	}
	slot, err := n.sessions.Resolve(handle.ID())
	if err != nil {
		return fmt.Errorf("tcp-out %s: %w", n.cfg.ID, err)
	}
	if err := writeSlot(slot, body, n.writeTimeout); err != nil {
		return fmt.Errorf("tcp-out %s: write: %w", n.cfg.ID, err)
	}
	if closeAfter {
		n.sessions.Close(handle.ID())
	}
	n.setStatus("green", fmt.Sprintf("sent · %dB → %s", len(body), slot.Conn.RemoteAddr().String()))
	return nil
}

// ─── Server-broadcast mode ──────────────────────────────────────────────────

func (n *TCPOutNode) handleBroadcast(body []byte) error {
	if n.sessions == nil {
		return fmt.Errorf("tcp-out %s: session registry not wired", n.cfg.ID)
	}
	slots := n.sessions.SessionsByOwner(n.targetTcpIn)
	if len(slots) == 0 {
		n.setStatus("yellow", "no targets")
		return nil
	}
	var failed int
	for _, slot := range slots {
		if err := writeSlot(slot, body, n.writeTimeout); err != nil {
			failed++
			slog.Warn("tcp-out: broadcast write",
				"node_id", n.cfg.ID, "target", slot.Conn.RemoteAddr().String(), "err", err)
		}
	}
	delivered := len(slots) - failed
	if failed > 0 {
		n.setStatus("yellow", fmt.Sprintf("broadcast %d/%d", delivered, len(slots)))
	} else {
		n.setStatus("green", fmt.Sprintf("broadcast %d", delivered))
	}
	return nil
}

// ─── Client mode ────────────────────────────────────────────────────────────

func (n *TCPOutNode) handleClient(msg *flow.Message, body []byte, closeAfter bool) error {
	host, port, err := n.resolveClientAddr(msg)
	if err != nil {
		return fmt.Errorf("tcp-out %s: %w", n.cfg.ID, err)
	}
	if !n.keepConnection {
		return n.dialAndSend(host, port, body, closeAfter)
	}
	// Persistent client: enqueue.
	select {
	case n.clientQueue <- tcpOutboundMsg{body: body, closeAfter: closeAfter, origMsg: msg}:
		return nil
	default:
		return fmt.Errorf("tcp-out %s: outbound queue full", n.cfg.ID)
	}
}

func (n *TCPOutNode) dialAndSend(host string, port int, body []byte, closeAfter bool) error {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	c, err := n.dial(addr)
	if err != nil {
		n.setStatus("yellow", "dial: "+truncateStatus(err.Error(), 32))
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	defer c.Close()
	if err := writeWithDeadline(c, body, n.writeTimeout); err != nil {
		n.setStatus("yellow", "write error")
		return fmt.Errorf("write: %w", err)
	}
	_ = closeAfter // dialAndSend already closes after deferred return
	n.setStatus("green", fmt.Sprintf("sent · %dB → %s", len(body), addr))
	return nil
}

func (n *TCPOutNode) resolveClientAddr(msg *flow.Message) (string, int, error) {
	host := n.host
	if v, _ := msg.Get("host").(string); v != "" {
		host = v
	}
	port := n.port
	if v, ok := msg.Get("port").(int); ok && v > 0 {
		port = v
	} else if v, ok := msg.Get("port").(float64); ok && v > 0 {
		port = int(v)
	} else if v, ok := msg.Get("port").(string); ok && v != "" {
		if p, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && p > 0 {
			port = p
		}
	}
	if host == "" {
		return "", 0, errors.New("empty host")
	}
	if port <= 0 {
		return "", 0, errors.New("empty port")
	}
	return host, port, nil
}

// clientKeepWorker drains the outbound queue, maintaining a single
// persistent connection. On disconnect it reconnects with exponential
// backoff; messages enqueued during the down phase wait in the
// channel buffer (overflow is rejected at Enqueue time, see
// handleClient).
func (n *TCPOutNode) clientKeepWorker() {
	defer n.clientWg.Done()

	delay := time.Second
	const maxDelay = 30 * time.Second

	for {
		select {
		case <-n.clientStop:
			return
		default:
		}

		host := n.host
		port := n.port
		if host == "" || port <= 0 {
			// Wait for first message to learn the address (rare —
			// users normally configure it). For simplicity we
			// pull from the queue blocking, then push back if we
			// can't dial yet.
			select {
			case m := <-n.clientQueue:
				if h, p, err := n.resolveClientAddr(m.origMsg); err == nil {
					host, port = h, p
				} else {
					if n.errorFn != nil {
						n.errorFn(fmt.Errorf("tcp-out %s: %w", n.cfg.ID, err), m.origMsg)
					}
					continue
				}
				// Dial then process this message + drain rest.
				addr := net.JoinHostPort(host, strconv.Itoa(port))
				c, err := n.dial(addr)
				if err != nil {
					if n.errorFn != nil {
						n.errorFn(fmt.Errorf("tcp-out %s: dial %s: %w", n.cfg.ID, addr, err), m.origMsg)
					}
					if !n.sleepOrStop(delay) {
						return
					}
					delay = backoff(delay, maxDelay)
					continue
				}
				n.setClientConn(c)
				delay = time.Second
				n.setStatus("green", "connected · "+addr)
				if err := writeWithDeadline(c, m.body, n.writeTimeout); err != nil {
					n.handleWriteFailure(err, m)
					continue
				}
				if m.closeAfter {
					n.closeClientConn()
				}
			case <-n.clientStop:
				return
			}
			continue
		}

		// Dial.
		addr := net.JoinHostPort(host, strconv.Itoa(port))
		c, err := n.dial(addr)
		if err != nil {
			n.setStatus("yellow", "dial: "+truncateStatus(err.Error(), 32))
			if !n.sleepOrStop(delay) {
				return
			}
			delay = backoff(delay, maxDelay)
			continue
		}
		n.setClientConn(c)
		delay = time.Second
		n.setStatus("green", "connected · "+addr)

		// Drain queue until the connection breaks.
		broken := false
		for !broken {
			select {
			case <-n.clientStop:
				return
			case m := <-n.clientQueue:
				if err := writeWithDeadline(c, m.body, n.writeTimeout); err != nil {
					n.handleWriteFailure(err, m)
					broken = true
					break
				}
				n.setStatus("green", fmt.Sprintf("sent · %dB → %s", len(m.body), addr))
				if m.closeAfter {
					n.closeClientConn()
					broken = true
				}
			}
		}
		// Backoff before redialing.
		if !n.sleepOrStop(delay) {
			return
		}
		delay = backoff(delay, maxDelay)
	}
}

func (n *TCPOutNode) handleWriteFailure(err error, m tcpOutboundMsg) {
	if n.errorFn != nil {
		n.errorFn(fmt.Errorf("tcp-out %s: write: %w", n.cfg.ID, err), m.origMsg)
	}
	n.closeClientConn()
}

func (n *TCPOutNode) setClientConn(c net.Conn) {
	n.clientMu.Lock()
	if n.clientConn != nil {
		_ = n.clientConn.Close()
	}
	n.clientConn = c
	n.clientMu.Unlock()
}

func (n *TCPOutNode) closeClientConn() {
	n.clientMu.Lock()
	if n.clientConn != nil {
		_ = n.clientConn.Close()
		n.clientConn = nil
	}
	n.clientMu.Unlock()
}

func (n *TCPOutNode) sleepOrStop(d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-n.clientStop:
		return false
	}
}

// ─── Helpers ────────────────────────────────────────────────────────────────

// writeSlot writes body to slot.Conn under slot.Mu, observing the
// configured timeout. Returns ErrSessionClosed if the slot is gone by
// the time we acquire the lock.
func writeSlot(slot *flow.SessionSlot, body []byte, timeout time.Duration) error {
	if slot.Closed.Load() {
		return flow.ErrSessionClosed
	}
	slot.Mu.Lock()
	defer slot.Mu.Unlock()
	if slot.Closed.Load() {
		return flow.ErrSessionClosed
	}
	return writeWithDeadline(slot.Conn, body, timeout)
}

func writeWithDeadline(c net.Conn, body []byte, timeout time.Duration) error {
	if timeout > 0 {
		_ = c.SetWriteDeadline(time.Now().Add(timeout))
		defer c.SetWriteDeadline(time.Time{})
	}
	if len(body) == 0 {
		return nil
	}
	_, err := c.Write(body)
	return err
}

func encodeOutboundBody(msg *flow.Message) ([]byte, error) {
	switch p := msg.Payload().(type) {
	case nil:
		return nil, nil
	case string:
		return []byte(p), nil
	case []byte:
		return p, nil
	case []int:
		out := make([]byte, len(p))
		for i, v := range p {
			out[i] = byte(v & 0xff)
		}
		return out, nil
	case []any:
		out := make([]byte, len(p))
		for i, v := range p {
			switch x := v.(type) {
			case int:
				out[i] = byte(x & 0xff)
			case float64:
				out[i] = byte(int(x) & 0xff)
			default:
				return nil, fmt.Errorf("payload[%d] not numeric: %T", i, v)
			}
		}
		return out, nil
	default:
		return []byte(fmt.Sprintf("%v", p)), nil
	}
}

func (n *TCPOutNode) setStatus(fill, text string) {
	if n.status != nil {
		n.status(fill, text)
	}
}

// TCPOutTypeInfo returns the NodeTypeInfo for the tcp-out node.
func TCPOutTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "tcp-out",
		Category:    "network",
		Label:       "TCP Send",
		Description: "Write msg.payload to a TCP peer (reply / broadcast / client)",
		Icon:        "mdi-lan-pending",
		Defaults: map[string]any{
			"mode":              tcpOutModeReply,
			"host":              "",
			"port":              0,
			"keepConnection":    true,
			"appendDelimiter":   "",
			"closeAfterSend":    false,
			"dialTimeout":       int(defaultTcpOutDialTimeout / time.Second),
			"writeTimeout":      int(defaultTcpOutWriteTimeout / time.Second),
			"outboundQueueSize": defaultTcpOutQueueSize,
		},
		Inputs:  1,
		Outputs: 0,
	}
}
