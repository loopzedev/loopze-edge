// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/loopzedev/loopze-edge/internal/credentials"
	"github.com/loopzedev/loopze-edge/internal/flow"
)

// Modes for tcp-in.
const (
	tcpInModeServer = "server"
	tcpInModeClient = "client"
)

const (
	defaultTcpInMaxFrame             = 1 << 20 // 1 MiB
	defaultTcpInKeepAliveInterval    = 30 * time.Second
	defaultTcpInDialTimeout          = 10 * time.Second
	defaultTcpInReconnectInitialDelay = time.Second
	defaultTcpInReconnectMaxDelay    = 30 * time.Second
)

// tcpFramingCfg captures the parsed framing strategy. lengthPrefix is
// non-nil only when framing == "length-prefix".
type tcpFramingCfg struct {
	mode           string
	delimiterBytes []byte
	lengthPrefix   *lengthPrefixCfg
	fixedLength    int
}

func (cfg *tcpFramingCfg) build(maxFrameBytes int) (Framer, error) {
	switch cfg.mode {
	case FramingStream:
		return NewStreamFramer(maxFrameBytes), nil
	case FramingDelimiter:
		return NewDelimiterFramer(cfg.delimiterBytes, maxFrameBytes)
	case FramingLengthPrefix:
		return NewLengthPrefixFramer(cfg.lengthPrefix.headerSize, cfg.lengthPrefix.endian, cfg.lengthPrefix.includesHeader, maxFrameBytes)
	case FramingFixedLength:
		return NewFixedLengthFramer(cfg.fixedLength)
	}
	return nil, fmt.Errorf("unknown framing %q", cfg.mode)
}

// TCPInNode listens on a TCP port (server mode) or dials a remote
// (client mode) and emits one message per received frame on output 0.
// Server mode supports many concurrent peers; each accepted connection
// becomes a session that downstream tcp-out nodes can resolve via
// msg.session.
type TCPInNode struct {
	cfg flow.NodeConfig

	send    flow.SendFunc
	status  flow.StatusFunc
	debug   flow.DebugFunc
	errorFn flow.ErrorFunc

	// Injected by the engine.
	sessions *flow.SessionRegistry

	// Parsed configuration.
	mode               string
	host               string
	port               int
	framing            tcpFramingCfg
	payloadEncoding    string
	maxFrameBytes      int
	keepAlive          bool
	keepAliveInterval  time.Duration
	noDelay            bool
	emitCloseEvent     bool // emit a `_event=close` marker on disconnect
	allowedNets        []*net.IPNet // server-only
	maxConnections     int          // server-only; 0 = unbounded
	reconnect          bool         // client-only
	reconnectInitial   time.Duration
	reconnectMax       time.Duration
	dialTimeout        time.Duration
	tlsConfig          *tls.Config // client-only; nil when tls is disabled
	certStore          *credentials.CertStore

	// Runtime state.
	mu        sync.Mutex
	listener  net.Listener
	stopCh    chan struct{}
	stopped   bool
	wg        sync.WaitGroup
	connCount int // server: currently-accepted conns
}

// NewTCPInNode is the NodeFactory for the "tcp-in" node type.
func NewTCPInNode(cfg flow.NodeConfig) (flow.NodeInstance, error) {
	return &TCPInNode{cfg: cfg, stopCh: make(chan struct{})}, nil
}

func (n *TCPInNode) Init() error {
	props := n.cfg.Properties

	n.mode = strings.ToLower(stringVal(props, "mode", tcpInModeServer))
	if n.mode != tcpInModeServer && n.mode != tcpInModeClient {
		return fmt.Errorf("tcp-in %s: invalid mode %q", n.cfg.ID, n.mode)
	}

	n.host = strings.TrimSpace(stringVal(props, "host", ""))
	if n.mode == tcpInModeServer && n.host == "" {
		n.host = "0.0.0.0"
	}
	if n.mode == tcpInModeClient && n.host == "" {
		return fmt.Errorf("tcp-in %s: client mode requires host", n.cfg.ID)
	}

	n.port = intVal(props, "port", 0)
	if n.port <= 0 || n.port > 65535 {
		return fmt.Errorf("tcp-in %s: invalid port %d", n.cfg.ID, n.port)
	}

	if err := n.parseFraming(props); err != nil {
		return err
	}

	n.payloadEncoding = strings.ToLower(stringVal(props, "payloadEncoding", tcpEncBuffer))
	switch n.payloadEncoding {
	case tcpEncBuffer, tcpEncString, tcpEncBase64:
	default:
		return fmt.Errorf("tcp-in %s: invalid payloadEncoding %q", n.cfg.ID, n.payloadEncoding)
	}

	n.maxFrameBytes = intVal(props, "maxFrameBytes", defaultTcpInMaxFrame)
	if n.maxFrameBytes < 0 {
		n.maxFrameBytes = 0
	}

	n.keepAlive = true
	if v, ok := props["keepAlive"].(bool); ok {
		n.keepAlive = v
	}
	kaSec := intVal(props, "keepAliveInterval", int(defaultTcpInKeepAliveInterval/time.Second))
	if kaSec <= 0 {
		kaSec = int(defaultTcpInKeepAliveInterval / time.Second)
	}
	n.keepAliveInterval = time.Duration(kaSec) * time.Second
	if v, ok := props["nodelay"].(bool); ok {
		n.noDelay = v
	}
	if v, ok := props["emitCloseEvent"].(bool); ok {
		n.emitCloseEvent = v
	}

	if n.mode == tcpInModeServer {
		raw := stringSlice(props, "allowedRemotes")
		for _, s := range raw {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			_, ipnet, err := net.ParseCIDR(s)
			if err != nil {
				return fmt.Errorf("tcp-in %s: invalid CIDR %q: %w", n.cfg.ID, s, err)
			}
			n.allowedNets = append(n.allowedNets, ipnet)
		}
		n.maxConnections = intVal(props, "maxConnections", 0)
		if n.maxConnections < 0 {
			n.maxConnections = 0
		}
	}

	if n.mode == tcpInModeClient {
		n.reconnect = true
		if v, ok := props["reconnect"].(bool); ok {
			n.reconnect = v
		}
		ms := intVal(props, "reconnectInitialDelay", int(defaultTcpInReconnectInitialDelay/time.Millisecond))
		if ms <= 0 {
			ms = int(defaultTcpInReconnectInitialDelay / time.Millisecond)
		}
		n.reconnectInitial = time.Duration(ms) * time.Millisecond
		ms = intVal(props, "reconnectMaxDelay", int(defaultTcpInReconnectMaxDelay/time.Millisecond))
		if ms <= 0 {
			ms = int(defaultTcpInReconnectMaxDelay / time.Millisecond)
		}
		n.reconnectMax = time.Duration(ms) * time.Millisecond
		ds := intVal(props, "dialTimeout", int(defaultTcpInDialTimeout/time.Second))
		if ds <= 0 {
			ds = int(defaultTcpInDialTimeout / time.Second)
		}
		n.dialTimeout = time.Duration(ds) * time.Second

		tlsCfg, err := ParseTLSBlock(props, n.cfg.ID, n.certStore)
		if err != nil {
			return fmt.Errorf("tcp-in %s: %w", n.cfg.ID, err)
		}
		n.tlsConfig = tlsCfg
	}

	return nil
}

func (n *TCPInNode) parseFraming(props map[string]any) error {
	mode := strings.ToLower(stringVal(props, "framing", FramingStream))
	switch mode {
	case FramingStream, FramingDelimiter, FramingLengthPrefix, FramingFixedLength:
	default:
		return fmt.Errorf("tcp-in %s: invalid framing %q", n.cfg.ID, mode)
	}
	n.framing.mode = mode

	if mode == FramingDelimiter {
		bs, err := ParseDelimiter(stringVal(props, "delimiter", `\n`))
		if err != nil {
			return fmt.Errorf("tcp-in %s: %w", n.cfg.ID, err)
		}
		n.framing.delimiterBytes = bs
	}
	if mode == FramingLengthPrefix {
		raw, _ := props["lengthPrefix"].(map[string]any)
		if raw == nil {
			raw = map[string]any{}
		}
		bytesN := intVal(raw, "bytes", 4)
		endian, err := ParseEndianness(stringVal(raw, "endianness", "big"))
		if err != nil {
			return fmt.Errorf("tcp-in %s: %w", n.cfg.ID, err)
		}
		incl, _ := raw["includesHeader"].(bool)
		n.framing.lengthPrefix = &lengthPrefixCfg{
			headerSize:     bytesN,
			endian:         endian,
			includesHeader: incl,
		}
	}
	if mode == FramingFixedLength {
		size := intVal(props, "fixedLength", 0)
		if size <= 0 {
			return fmt.Errorf("tcp-in %s: fixedLength must be > 0", n.cfg.ID)
		}
		n.framing.fixedLength = size
	}
	return nil
}

func (n *TCPInNode) SetSend(fn flow.SendFunc)     { n.send = fn }
func (n *TCPInNode) SetStatus(fn flow.StatusFunc) { n.status = fn }
func (n *TCPInNode) SetDebug(fn flow.DebugFunc)   { n.debug = fn }
func (n *TCPInNode) SetError(fn flow.ErrorFunc)   { n.errorFn = fn }

// SetSessionRegistry implements flow.SessionRegistryProvider.
func (n *TCPInNode) SetSessionRegistry(r *flow.SessionRegistry) { n.sessions = r }

// SetCertStore implements flow.CertStoreProvider.
func (n *TCPInNode) SetCertStore(s *credentials.CertStore) { n.certStore = s }

func (n *TCPInNode) Start() error {
	switch n.mode {
	case tcpInModeServer:
		return n.startServer()
	case tcpInModeClient:
		return n.startClient()
	}
	return fmt.Errorf("tcp-in %s: invalid mode %q", n.cfg.ID, n.mode)
}

func (n *TCPInNode) Stop() error {
	n.mu.Lock()
	if n.stopped {
		n.mu.Unlock()
		return nil
	}
	n.stopped = true
	close(n.stopCh)
	if n.listener != nil {
		_ = n.listener.Close()
	}
	n.mu.Unlock()

	// Tear down every accepted session belonging to this node so
	// reader goroutines unblock from their Read calls.
	if n.sessions != nil {
		n.sessions.CloseByOwner(n.cfg.ID)
	}

	n.wg.Wait()
	slog.Info("tcp-in stopped", "node_id", n.cfg.ID)
	return nil
}

// HandleMessage is unreachable for source nodes (tcp-in has 0 inputs)
// but the interface requires the method.
func (n *TCPInNode) HandleMessage(_ *flow.Message) ([][]*flow.Message, error) {
	return nil, nil
}

// ─── Server mode ────────────────────────────────────────────────────────────

func (n *TCPInNode) startServer() error {
	addr := net.JoinHostPort(n.host, strconv.Itoa(n.port))
	l, err := net.Listen("tcp", addr)
	if err != nil {
		n.setStatus("red", "bind: "+truncateStatus(err.Error(), 32))
		return fmt.Errorf("tcp-in %s: listen %s: %w", n.cfg.ID, addr, err)
	}
	n.mu.Lock()
	n.listener = l
	n.mu.Unlock()

	n.publishServerStatus()
	n.wg.Add(1)
	go n.acceptLoop(l)
	slog.Info("tcp-in listening", "node_id", n.cfg.ID, "addr", l.Addr().String())
	return nil
}

func (n *TCPInNode) acceptLoop(l net.Listener) {
	defer n.wg.Done()
	for {
		c, err := l.Accept()
		if err != nil {
			if n.isStopped() {
				return
			}
			// Transient accept errors: log and retry after a tiny
			// backoff so a flaky listener doesn't spin.
			slog.Warn("tcp-in accept", "node_id", n.cfg.ID, "err", err)
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if !n.checkAcceptLimits(c) {
			_ = c.Close()
			continue
		}
		n.applySocketOptions(c)
		n.wg.Add(1)
		go n.serveConn(c)
	}
}

func (n *TCPInNode) checkAcceptLimits(c net.Conn) bool {
	if n.maxConnections > 0 {
		n.mu.Lock()
		over := n.connCount >= n.maxConnections
		n.mu.Unlock()
		if over {
			slog.Warn("tcp-in: max connections reached, dropping",
				"node_id", n.cfg.ID, "max", n.maxConnections)
			return false
		}
	}
	if len(n.allowedNets) == 0 {
		return true
	}
	host, _, _ := net.SplitHostPort(c.RemoteAddr().String())
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, ipnet := range n.allowedNets {
		if ipnet.Contains(ip) {
			return true
		}
	}
	slog.Info("tcp-in: rejecting remote outside allowlist",
		"node_id", n.cfg.ID, "remote", c.RemoteAddr().String())
	return false
}

func (n *TCPInNode) applySocketOptions(c net.Conn) {
	tc, ok := c.(*net.TCPConn)
	if !ok {
		return
	}
	if n.keepAlive {
		_ = tc.SetKeepAlive(true)
		_ = tc.SetKeepAlivePeriod(n.keepAliveInterval)
	}
	_ = tc.SetNoDelay(n.noDelay)
}

func (n *TCPInNode) serveConn(c net.Conn) {
	defer n.wg.Done()

	if n.sessions == nil {
		// Defensive: registry should always be wired. Without it a
		// reply node could never write back, so refuse the conn.
		_ = c.Close()
		return
	}

	handle, _ := n.sessions.Register(c, n.cfg.ID)
	n.bumpConnCount(+1)
	n.publishServerStatus()
	defer func() {
		n.sessions.Close(handle.ID())
		n.bumpConnCount(-1)
		n.publishServerStatus()
	}()

	n.readLoop(c, handle, c.LocalAddr().String(), c.RemoteAddr().String())
}

func (n *TCPInNode) bumpConnCount(delta int) {
	n.mu.Lock()
	n.connCount += delta
	if n.connCount < 0 {
		n.connCount = 0
	}
	n.mu.Unlock()
}

// ─── Client mode ────────────────────────────────────────────────────────────

func (n *TCPInNode) startClient() error {
	n.wg.Add(1)
	go n.clientLoop()
	return nil
}

func (n *TCPInNode) clientLoop() {
	defer n.wg.Done()

	addr := net.JoinHostPort(n.host, strconv.Itoa(n.port))
	delay := n.reconnectInitial

	for {
		if n.isStopped() {
			return
		}

		n.setStatus("yellow", "connecting · "+addr)
		c, err := n.dial(addr)
		if err != nil {
			n.setStatus("yellow", "retry: "+truncateStatus(err.Error(), 32))
			if !n.reconnect {
				if n.errorFn != nil {
					n.errorFn(fmt.Errorf("tcp-in %s: dial %s: %w", n.cfg.ID, addr, err), nil)
				}
				return
			}
			if !n.sleepOrStop(delay) {
				return
			}
			delay = backoff(delay, n.reconnectMax)
			continue
		}

		n.applySocketOptions(c)
		delay = n.reconnectInitial // reset on success
		n.setStatus("green", "connected · "+addr)

		var handle *flow.SessionHandle
		if n.sessions != nil {
			handle, _ = n.sessions.Register(c, n.cfg.ID)
		}
		n.readLoop(c, handle, c.LocalAddr().String(), c.RemoteAddr().String())
		if handle != nil && n.sessions != nil {
			n.sessions.Close(handle.ID())
		}

		if n.isStopped() || !n.reconnect {
			return
		}
		n.setStatus("yellow", "disconnected · retry in "+delay.String())
		if !n.sleepOrStop(delay) {
			return
		}
		delay = backoff(delay, n.reconnectMax)
	}
}

// dial opens a TCP connection — wrapped in TLS when configured.
func (n *TCPInNode) dial(addr string) (net.Conn, error) {
	if n.tlsConfig == nil {
		return net.DialTimeout("tcp", addr, n.dialTimeout)
	}
	d := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: n.dialTimeout},
		Config:    n.tlsConfig,
	}
	return d.Dial("tcp", addr)
}

// sleepOrStop sleeps for d (with ±20% jitter) but returns false if a
// Stop arrives in the meantime.
func (n *TCPInNode) sleepOrStop(d time.Duration) bool {
	jitter := jitterPct(d, 0.2)
	timer := time.NewTimer(jitter)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-n.stopCh:
		return false
	}
}

// backoff doubles the current delay capped at max.
func backoff(d, max time.Duration) time.Duration {
	next := d * 2
	if next > max {
		next = max
	}
	if next <= 0 {
		next = max
	}
	return next
}

// jitterPct returns d ± pct (e.g. pct=0.2 yields a value in [0.8d, 1.2d]).
func jitterPct(d time.Duration, pct float64) time.Duration {
	if d <= 0 {
		return d
	}
	span := float64(d) * pct
	delta := (rand.Float64()*2 - 1) * span
	return d + time.Duration(delta)
}

// ─── Shared read loop ───────────────────────────────────────────────────────

func (n *TCPInNode) readLoop(c net.Conn, handle *flow.SessionHandle, localAddr, remoteAddr string) {
	framer, err := n.framing.build(n.maxFrameBytes)
	if err != nil {
		if n.errorFn != nil {
			n.errorFn(fmt.Errorf("tcp-in %s: build framer: %w", n.cfg.ID, err), nil)
		}
		return
	}

	chunk := make([]byte, 4096)
	host, portStr, _ := net.SplitHostPort(remoteAddr)
	port, _ := strconv.Atoi(portStr)
	for {
		nb, err := c.Read(chunk)
		if nb > 0 {
			frames, ferr := framer.Append(chunk[:nb])
			for _, f := range frames {
				n.emitFrame(f, handle, localAddr, remoteAddr, host, port)
			}
			if ferr != nil {
				if n.errorFn != nil {
					n.errorFn(fmt.Errorf("tcp-in %s: framing: %w", n.cfg.ID, ferr), nil)
				}
				_ = c.Close()
				break
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !n.isStopped() {
				// A closed pipe / use-of-closed-connection on
				// teardown is normal; only surface real read
				// errors.
				if !isExpectedCloseErr(err) && n.errorFn != nil {
					n.errorFn(fmt.Errorf("tcp-in %s: read: %w", n.cfg.ID, err), nil)
				}
			}
			break
		}
	}
	n.emitClose(handle, localAddr, remoteAddr, host, port)
}

func (n *TCPInNode) emitFrame(frame []byte, handle *flow.SessionHandle, localAddr, remoteAddr, host string, port int) {
	if n.send == nil {
		return
	}
	msg := flow.NewMessage()
	msg.SetPayload(encodePayload(frame, n.payloadEncoding))
	if handle != nil {
		msg.Set("session", handle)
	}
	msg.Set("remoteAddr", remoteAddr)
	msg.Set("localAddr", localAddr)
	msg.Set("ip", host)
	msg.Set("port", port)
	msg.Set("frameSize", len(frame))
	n.send(0, msg)
}

func (n *TCPInNode) emitClose(handle *flow.SessionHandle, localAddr, remoteAddr, host string, port int) {
	if n.send == nil || !n.emitCloseEvent {
		return
	}
	msg := flow.NewMessage()
	msg.SetPayload(nil)
	if handle != nil {
		msg.Set("session", handle)
	}
	msg.Set("remoteAddr", remoteAddr)
	msg.Set("localAddr", localAddr)
	msg.Set("ip", host)
	msg.Set("port", port)
	msg.Set("_event", "close")
	n.send(0, msg)
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func (n *TCPInNode) isStopped() bool {
	select {
	case <-n.stopCh:
		return true
	default:
		return false
	}
}

func (n *TCPInNode) publishServerStatus() {
	if n.mode != tcpInModeServer {
		return
	}
	n.mu.Lock()
	count := n.connCount
	n.mu.Unlock()
	addr := net.JoinHostPort(n.host, strconv.Itoa(n.port))
	if count == 0 {
		n.setStatus("green", "listening · "+addr)
		return
	}
	n.setStatus("green", fmt.Sprintf("listening · %s · %d conn", addr, count))
}

func (n *TCPInNode) setStatus(fill, text string) {
	if n.status != nil {
		n.status(fill, text)
	}
}

// isExpectedCloseErr reports whether err is one of the "the conn was
// closed under us" errors we don't want to surface as catchable errors.
func isExpectedCloseErr(err error) bool {
	if err == nil {
		return true
	}
	s := err.Error()
	return strings.Contains(s, "use of closed network connection") ||
		strings.Contains(s, "connection reset by peer")
}

// TCPInTypeInfo returns the NodeTypeInfo for the tcp-in node.
func TCPInTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "tcp-in",
		Category:    "network",
		Label:       "TCP Receive",
		Description: "Listen on a TCP port (server) or dial a remote (client) and emit one message per frame",
		Icon:        "mdi-lan-connect",
		Defaults: map[string]any{
			"mode":              tcpInModeServer,
			"host":              "0.0.0.0",
			"port":              7000,
			"framing":           FramingStream,
			"delimiter":         `\n`,
			"payloadEncoding":   tcpEncBuffer,
			"maxFrameBytes":     defaultTcpInMaxFrame,
			"keepAlive":         true,
			"keepAliveInterval": int(defaultTcpInKeepAliveInterval / time.Second),
			"nodelay":           false,
			"emitCloseEvent":    false,
			"reconnect":         true,
		},
		Inputs:  0,
		Outputs: 1,
	}
}
