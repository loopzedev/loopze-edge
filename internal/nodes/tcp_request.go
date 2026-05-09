// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cbroglie/mustache"

	"github.com/loopzedev/loopze-edge/internal/credentials"
	"github.com/loopzedev/loopze-edge/internal/flow"
)

// Terminator strategies — when does the response read finish?
const (
	tcpReqTermTime         = "time"
	tcpReqTermDelimiter    = "delimiter"
	tcpReqTermLength       = "length"
	tcpReqTermLengthPrefix = "length-prefix"
	tcpReqTermClose        = "close"
)

// Response payload encodings.
const (
	tcpEncBuffer = "buffer"
	tcpEncString = "string"
	tcpEncBase64 = "base64"
)

const (
	defaultTcpResponseTimeout = 5000 * time.Millisecond
	defaultTcpDialTimeout     = 10 * time.Second
)

// TCPRequestNode performs a TCP round-trip on each input message: dial
// (or reuse a kept-open connection) → send msg.payload → read response
// per the configured terminator → emit on output 0. With
// keepConnection=true the underlying conn is reused across calls,
// avoiding the dial overhead for chatty protocols. The engine drives
// HandleMessage serially per-node so a single persistent conn is
// race-free; on read/write error or destination change the conn is
// torn down and the next call dials fresh.
type TCPRequestNode struct {
	cfg flow.NodeConfig

	send    flow.SendFunc
	status  flow.StatusFunc
	debug   flow.DebugFunc
	errorFn flow.ErrorFunc

	// Parsed configuration.
	hostTemplate    string
	hostParsed      *mustache.Template
	portTemplate    string
	portParsed      *mustache.Template
	terminator      string
	delimiterBytes  []byte
	responseLength  int
	lengthPrefix    *lengthPrefixCfg
	appendDelimiter []byte
	responseEnc     string
	responseTimeout time.Duration
	dialTimeout     time.Duration
	keepConnection  bool
	tlsConfig       *tls.Config // nil when tls is disabled
	certStore       *credentials.CertStore

	// Persistent-connection state. Only used when keepConnection is
	// true. HandleMessage runs serially per node so connMu is mostly
	// belt-and-braces against Stop() racing with an in-flight call.
	connMu   sync.Mutex
	conn     net.Conn
	connHost string
	connPort int
}

type lengthPrefixCfg struct {
	headerSize     int
	endian         binary.ByteOrder
	includesHeader bool
}

// NewTCPRequestNode is the NodeFactory for the "tcp-request" node type.
func NewTCPRequestNode(cfg flow.NodeConfig) (flow.NodeInstance, error) {
	return &TCPRequestNode{cfg: cfg}, nil
}

// Init validates the configuration. Errors here cause the engine to
// refuse the deploy for this node.
func (n *TCPRequestNode) Init() error {
	props := n.cfg.Properties

	n.hostTemplate = strings.TrimSpace(stringVal(props, "host", ""))
	if n.hostTemplate != "" {
		t, err := mustache.ParseString(n.hostTemplate)
		if err != nil {
			return fmt.Errorf("tcp-request %s: host template parse: %w", n.cfg.ID, err)
		}
		n.hostParsed = t
	}

	n.portTemplate = strings.TrimSpace(stringVal(props, "port", ""))
	if n.portTemplate != "" {
		t, err := mustache.ParseString(n.portTemplate)
		if err != nil {
			return fmt.Errorf("tcp-request %s: port template parse: %w", n.cfg.ID, err)
		}
		n.portParsed = t
	}

	n.terminator = strings.ToLower(stringVal(props, "terminator", tcpReqTermTime))
	switch n.terminator {
	case tcpReqTermTime, tcpReqTermDelimiter, tcpReqTermLength, tcpReqTermLengthPrefix, tcpReqTermClose:
	default:
		return fmt.Errorf("tcp-request %s: invalid terminator %q", n.cfg.ID, n.terminator)
	}

	if n.terminator == tcpReqTermDelimiter {
		raw := stringVal(props, "delimiter", "\\n")
		bs, err := ParseDelimiter(raw)
		if err != nil {
			return fmt.Errorf("tcp-request %s: %w", n.cfg.ID, err)
		}
		n.delimiterBytes = bs
	}

	if n.terminator == tcpReqTermLength {
		n.responseLength = intVal(props, "responseLength", 0)
		if n.responseLength <= 0 {
			return fmt.Errorf("tcp-request %s: responseLength must be > 0 for terminator=length", n.cfg.ID)
		}
	}

	if n.terminator == tcpReqTermLengthPrefix {
		raw, _ := props["lengthPrefix"].(map[string]any)
		if raw == nil {
			return fmt.Errorf("tcp-request %s: lengthPrefix config required for terminator=length-prefix", n.cfg.ID)
		}
		bytesN := intVal(raw, "bytes", 4)
		endian, err := ParseEndianness(stringVal(raw, "endianness", "big"))
		if err != nil {
			return fmt.Errorf("tcp-request %s: %w", n.cfg.ID, err)
		}
		incl, _ := raw["includesHeader"].(bool)
		n.lengthPrefix = &lengthPrefixCfg{
			headerSize:     bytesN,
			endian:         endian,
			includesHeader: incl,
		}
	}

	if raw := stringVal(props, "appendDelimiter", ""); raw != "" {
		bs, err := ParseDelimiter(raw)
		if err != nil {
			return fmt.Errorf("tcp-request %s: appendDelimiter: %w", n.cfg.ID, err)
		}
		n.appendDelimiter = bs
	}

	n.responseEnc = strings.ToLower(stringVal(props, "responseEncoding", tcpEncBuffer))
	switch n.responseEnc {
	case tcpEncBuffer, tcpEncString, tcpEncBase64:
	default:
		return fmt.Errorf("tcp-request %s: invalid responseEncoding %q", n.cfg.ID, n.responseEnc)
	}

	timeoutMs := intVal(props, "responseTimeout", int(defaultTcpResponseTimeout/time.Millisecond))
	if timeoutMs <= 0 {
		timeoutMs = int(defaultTcpResponseTimeout / time.Millisecond)
	}
	n.responseTimeout = time.Duration(timeoutMs) * time.Millisecond

	dialSec := intVal(props, "dialTimeout", int(defaultTcpDialTimeout/time.Second))
	if dialSec <= 0 {
		dialSec = int(defaultTcpDialTimeout / time.Second)
	}
	n.dialTimeout = time.Duration(dialSec) * time.Second

	if v, ok := props["keepConnection"].(bool); ok {
		n.keepConnection = v
	}
	if n.keepConnection && n.terminator == tcpReqTermClose {
		return fmt.Errorf("tcp-request %s: terminator=close is incompatible with keepConnection (peer FIN ends the connection)", n.cfg.ID)
	}

	tlsCfg, err := ParseTLSBlock(props, n.cfg.ID, n.certStore)
	if err != nil {
		return fmt.Errorf("tcp-request %s: %w", n.cfg.ID, err)
	}
	n.tlsConfig = tlsCfg

	return nil
}

func (n *TCPRequestNode) SetSend(fn flow.SendFunc)     { n.send = fn }
func (n *TCPRequestNode) SetStatus(fn flow.StatusFunc) { n.status = fn }
func (n *TCPRequestNode) SetDebug(fn flow.DebugFunc)   { n.debug = fn }
func (n *TCPRequestNode) SetError(fn flow.ErrorFunc)   { n.errorFn = fn }

// SetCertStore implements flow.CertStoreProvider.
func (n *TCPRequestNode) SetCertStore(s *credentials.CertStore) { n.certStore = s }

func (n *TCPRequestNode) Start() error {
	slog.Info("tcp-request started",
		"node_id", n.cfg.ID,
		"host", n.hostTemplate, "port", n.portTemplate,
		"terminator", n.terminator,
		"response_timeout", n.responseTimeout,
	)
	return nil
}

func (n *TCPRequestNode) Stop() error {
	n.closePersistentConn()
	slog.Info("tcp-request stopped", "node_id", n.cfg.ID)
	return nil
}

// closePersistentConn tears down the kept-open connection if any. Safe
// to call multiple times.
func (n *TCPRequestNode) closePersistentConn() {
	n.connMu.Lock()
	defer n.connMu.Unlock()
	if n.conn != nil {
		_ = n.conn.Close()
		n.conn = nil
		n.connHost = ""
		n.connPort = 0
	}
}

// HandleMessage performs one TCP round-trip and emits the response.
func (n *TCPRequestNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil {
		msg = flow.NewMessage()
	}

	host, port, err := n.resolveHostPort(msg)
	if err != nil {
		n.setStatus("red", "address error")
		return nil, fmt.Errorf("tcp-request %s: %w", n.cfg.ID, err)
	}

	body, err := n.encodeRequestBody(msg)
	if err != nil {
		return nil, fmt.Errorf("tcp-request %s: encode body: %w", n.cfg.ID, err)
	}
	if len(n.appendDelimiter) > 0 {
		body = append(body, n.appendDelimiter...)
	}

	timeout := n.responseTimeout
	if v, ok := msg.Get("timeout").(int); ok && v > 0 {
		timeout = time.Duration(v) * time.Millisecond
	} else if v, ok := msg.Get("timeout").(float64); ok && v > 0 {
		timeout = time.Duration(v) * time.Millisecond
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	start := time.Now()

	conn, reused, err := n.acquireConn(host, port, addr)
	if err != nil {
		n.setStatus("yellow", truncateStatus(err.Error(), 40))
		return nil, fmt.Errorf("tcp-request %s: dial %s: %w", n.cfg.ID, addr, err)
	}

	resp, err := n.runRoundTrip(conn, body, timeout)
	if err != nil && reused {
		// The cached connection failed (peer closed it between
		// calls, NAT timeout, etc.). Tear it down and dial fresh
		// once so the user-facing call still succeeds. Only retry
		// when we reused — a fresh-dial failure is reported as-is.
		n.releaseConn(conn, true)
		conn, _, derr := n.acquireConn(host, port, addr)
		if derr != nil {
			n.setStatus("yellow", truncateStatus(derr.Error(), 40))
			return nil, fmt.Errorf("tcp-request %s: redial %s: %w", n.cfg.ID, addr, derr)
		}
		resp, err = n.runRoundTrip(conn, body, timeout)
		reused = false
	}
	if err != nil {
		n.releaseConn(conn, true)
		n.setStatus("yellow", truncateStatus(err.Error(), 40))
		return nil, fmt.Errorf("tcp-request %s: %w", n.cfg.ID, err)
	}
	n.releaseConn(conn, false)

	out := msg.COWClone()
	out.SetPayload(encodePayload(resp, n.responseEnc))
	out.Set("remoteAddr", conn.RemoteAddr().String())
	out.Set("bytesSent", len(body))
	out.Set("bytesReceived", len(resp))
	elapsed := time.Since(start)
	out.Set("durationMs", elapsed.Milliseconds())
	out.Set("terminator", n.terminator)
	if reused {
		out.Set("connectionReused", true)
	}

	n.setStatus("green", fmt.Sprintf("%dms · %dB", elapsed.Milliseconds(), len(resp)))
	return [][]*flow.Message{{out}}, nil
}

// acquireConn returns a connection to host:port. With keepConnection
// it reuses the persistent conn (dropping and redialing if the
// destination changed) and reports reused=true on a hit. Without it
// every call dials fresh. TLS handshake is performed inline when
// n.tlsConfig is non-nil.
func (n *TCPRequestNode) acquireConn(host string, port int, addr string) (net.Conn, bool, error) {
	if !n.keepConnection {
		c, err := n.dial(addr)
		return c, false, err
	}

	n.connMu.Lock()
	if n.conn != nil && (n.connHost != host || n.connPort != port) {
		_ = n.conn.Close()
		n.conn = nil
	}
	if n.conn != nil {
		c := n.conn
		n.connMu.Unlock()
		return c, true, nil
	}
	n.connMu.Unlock()

	c, err := n.dial(addr)
	if err != nil {
		return nil, false, err
	}
	n.connMu.Lock()
	n.conn = c
	n.connHost = host
	n.connPort = port
	n.connMu.Unlock()
	return c, false, nil
}

// dial opens a TCP connection — wrapped in TLS when configured. The
// dial timeout is honoured for the full handshake.
func (n *TCPRequestNode) dial(addr string) (net.Conn, error) {
	if n.tlsConfig == nil {
		return net.DialTimeout("tcp", addr, n.dialTimeout)
	}
	d := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: n.dialTimeout},
		Config:    n.tlsConfig,
	}
	return d.Dial("tcp", addr)
}

// releaseConn finalises a connection after a round-trip. With
// keepConnection=true the conn is left open for the next call (or
// torn down on faulty=true). Without keepConnection the conn is
// always closed.
func (n *TCPRequestNode) releaseConn(c net.Conn, faulty bool) {
	if c == nil {
		return
	}
	// Clear any deadline so the next caller starts with a fresh one.
	_ = c.SetDeadline(time.Time{})
	if !n.keepConnection || faulty {
		_ = c.Close()
		if n.keepConnection {
			n.connMu.Lock()
			if n.conn == c {
				n.conn = nil
				n.connHost = ""
				n.connPort = 0
			}
			n.connMu.Unlock()
		}
	}
}

// runRoundTrip sets the per-call deadline, writes the request body
// and reads the response per the configured terminator.
func (n *TCPRequestNode) runRoundTrip(conn net.Conn, body []byte, timeout time.Duration) ([]byte, error) {
	_ = conn.SetDeadline(time.Now().Add(timeout))
	if len(body) > 0 {
		if _, err := conn.Write(body); err != nil {
			return nil, fmt.Errorf("write: %w", err)
		}
	}
	resp, err := n.readResponse(conn, timeout)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	return resp, nil
}

// resolveHostPort renders host/port templates against the message,
// preferring msg.host / msg.port over rendered config. Returns a clean
// error if neither source yields a non-empty host/port.
func (n *TCPRequestNode) resolveHostPort(msg *flow.Message) (string, int, error) {
	host, _ := msg.Get("host").(string)
	if host == "" && n.hostParsed != nil {
		s, err := n.hostParsed.Render(msg.DataView())
		if err != nil {
			return "", 0, fmt.Errorf("host template render: %w", err)
		}
		host = strings.TrimSpace(s)
	}
	if host == "" {
		return "", 0, errors.New("empty host")
	}

	port := 0
	if v, ok := msg.Get("port").(int); ok && v > 0 {
		port = v
	} else if v, ok := msg.Get("port").(float64); ok && v > 0 {
		port = int(v)
	} else if v, ok := msg.Get("port").(string); ok && v != "" {
		p, err := strconv.Atoi(strings.TrimSpace(v))
		if err == nil && p > 0 {
			port = p
		}
	}
	if port == 0 && n.portParsed != nil {
		s, err := n.portParsed.Render(msg.DataView())
		if err != nil {
			return "", 0, fmt.Errorf("port template render: %w", err)
		}
		p, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || p <= 0 {
			return "", 0, fmt.Errorf("invalid rendered port %q", s)
		}
		port = p
	}
	if port == 0 {
		return "", 0, errors.New("empty port")
	}
	return host, port, nil
}

// encodeRequestBody coerces msg.payload into raw bytes. Same conventions
// as tcp-out and the MQTT publish nodes: string → UTF-8, []byte → raw,
// []int → raw bytes (mod 256), nil/missing → empty.
func (n *TCPRequestNode) encodeRequestBody(msg *flow.Message) ([]byte, error) {
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

// readResponse drains the connection per the configured terminator. The
// caller has already set a deadline via conn.SetDeadline; for terminator
// "time" the deadline IS the read budget. For all other terminators a
// successful read returns early.
func (n *TCPRequestNode) readResponse(conn net.Conn, timeout time.Duration) ([]byte, error) {
	switch n.terminator {
	case tcpReqTermTime:
		return readUntilDeadline(conn, timeout)
	case tcpReqTermClose:
		return io.ReadAll(conn)
	case tcpReqTermLength:
		buf := make([]byte, n.responseLength)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return nil, err
		}
		return buf, nil
	case tcpReqTermDelimiter:
		return readUntilDelimiter(conn, n.delimiterBytes)
	case tcpReqTermLengthPrefix:
		return readLengthPrefix(conn, n.lengthPrefix)
	}
	return nil, fmt.Errorf("unknown terminator %q", n.terminator)
}

// readUntilDeadline reads from conn until the configured timeout elapses
// or the peer closes; whatever has been received is returned.
func readUntilDeadline(conn net.Conn, _ time.Duration) ([]byte, error) {
	var out []byte
	chunk := make([]byte, 4096)
	for {
		n, err := conn.Read(chunk)
		if n > 0 {
			out = append(out, chunk[:n]...)
		}
		if err != nil {
			// Deadline / EOF / close are all normal terminations
			// for the time-based terminator — surface only "real"
			// errors.
			if errors.Is(err, io.EOF) {
				return out, nil
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				return out, nil
			}
			return out, err
		}
	}
}

func readUntilDelimiter(conn net.Conn, delim []byte) ([]byte, error) {
	var out []byte
	chunk := make([]byte, 1024)
	for {
		n, err := conn.Read(chunk)
		if n > 0 {
			out = append(out, chunk[:n]...)
			if i := indexBytes(out, delim); i >= 0 {
				return out[:i], nil
			}
		}
		if err != nil {
			return out, err
		}
	}
}

func readLengthPrefix(conn net.Conn, cfg *lengthPrefixCfg) ([]byte, error) {
	hdr := make([]byte, cfg.headerSize)
	if _, err := io.ReadFull(conn, hdr); err != nil {
		return nil, err
	}
	announced := readUint(hdr, cfg.endian, cfg.headerSize)
	var payloadSize int
	if cfg.includesHeader {
		if announced < uint64(cfg.headerSize) {
			return nil, fmt.Errorf("announced length %d shorter than header (%d)", announced, cfg.headerSize)
		}
		payloadSize = int(announced) - cfg.headerSize
	} else {
		payloadSize = int(announced)
	}
	if payloadSize < 0 {
		return nil, fmt.Errorf("negative payload size %d", payloadSize)
	}
	body := make([]byte, payloadSize)
	if _, err := io.ReadFull(conn, body); err != nil {
		return nil, err
	}
	return body, nil
}

// indexBytes is bytes.Index without taking the dependency.
func indexBytes(haystack, needle []byte) int {
	if len(needle) == 0 || len(haystack) < len(needle) {
		return -1
	}
outer:
	for i := 0; i <= len(haystack)-len(needle); i++ {
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				continue outer
			}
		}
		return i
	}
	return -1
}

// encodePayload renders received bytes per the configured response
// encoding. Mirrors the tcp-in / udp-in convention so a tcp-request
// response is shaped identically to a tcp-in frame.
func encodePayload(b []byte, enc string) any {
	switch enc {
	case tcpEncString:
		return string(b)
	case tcpEncBase64:
		return base64.StdEncoding.EncodeToString(b)
	}
	return bytesToNumberArray(b)
}

func (n *TCPRequestNode) setStatus(fill, text string) {
	if n.status != nil {
		n.status(fill, text)
	}
}

// TCPRequestTypeInfo returns the NodeTypeInfo for the tcp-request node.
func TCPRequestTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "tcp-request",
		Category:    "network",
		Label:       "TCP Request",
		Description: "Connect to a TCP server, send msg.payload, read the response, close",
		Icon:        "mdi-lan-pending",
		Defaults: map[string]any{
			"host":             "",
			"port":             "",
			"terminator":       tcpReqTermTime,
			"delimiter":        "\\n",
			"responseLength":   0,
			"appendDelimiter":  "",
			"responseEncoding": tcpEncBuffer,
			"responseTimeout":  int(defaultTcpResponseTimeout / time.Millisecond),
			"dialTimeout":      int(defaultTcpDialTimeout / time.Second),
			"keepConnection":   false,
		},
		Inputs:  1,
		Outputs: 1,
	}
}
