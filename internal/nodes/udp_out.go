// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/net/ipv4"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// Modes for udp-out.
const (
	udpOutModeUnicast   = "unicast"
	udpOutModeBroadcast = "broadcast"
	udpOutModeMulticast = "multicast"
)

const (
	defaultUdpMulticastTTL = 1
)

// UDPOutNode sends `msg.payload` as a UDP datagram. Three modes share
// most of the code; the differences are setsockopt-level (SO_BROADCAST
// for broadcast; multicast TTL/loopback/interface for multicast).
type UDPOutNode struct {
	cfg flow.NodeConfig

	send    flow.SendFunc
	status  flow.StatusFunc
	debug   flow.DebugFunc
	errorFn flow.ErrorFunc

	// Parsed configuration.
	host              string
	port              int
	bindHost          string
	mode              string
	multicastTTL      int
	multicastLoopback bool
	reuseSocket       bool
	multicastIface    *net.Interface

	// Runtime state. When reuseSocket=true a single UDPConn is held
	// open across messages; HandleMessage runs serially per node so
	// sockMu only matters at Stop().
	sockMu sync.Mutex
	conn   *net.UDPConn
	pc4    *ipv4.PacketConn // wraps conn when mode=multicast
}

// NewUDPOutNode is the NodeFactory for the "udp-out" node type.
func NewUDPOutNode(cfg flow.NodeConfig) (flow.NodeInstance, error) {
	return &UDPOutNode{cfg: cfg}, nil
}

func (n *UDPOutNode) Init() error {
	props := n.cfg.Properties

	n.host = strings.TrimSpace(stringVal(props, "host", ""))
	n.port = intVal(props, "port", 0)
	n.bindHost = strings.TrimSpace(stringVal(props, "bindHost", ""))
	n.mode = strings.ToLower(stringVal(props, "mode", udpOutModeUnicast))
	switch n.mode {
	case udpOutModeUnicast, udpOutModeBroadcast, udpOutModeMulticast:
	default:
		return fmt.Errorf("udp-out %s: invalid mode %q", n.cfg.ID, n.mode)
	}

	n.multicastTTL = intVal(props, "multicastTTL", defaultUdpMulticastTTL)
	if n.multicastTTL < 0 || n.multicastTTL > 255 {
		return fmt.Errorf("udp-out %s: multicastTTL %d out of range (0–255)", n.cfg.ID, n.multicastTTL)
	}
	n.multicastLoopback = true
	if v, ok := props["multicastLoopback"].(bool); ok {
		n.multicastLoopback = v
	}
	n.reuseSocket = true
	if v, ok := props["reuseSocket"].(bool); ok {
		n.reuseSocket = v
	}

	if n.mode == udpOutModeMulticast && n.bindHost != "" {
		// Resolve to an interface so we can set the outbound iface.
		ifname := n.bindHost
		// Allow either "eth0" (interface name) or an IP — IP form
		// pins by source address rather than interface, which most
		// users don't want, so accept names only.
		nif, err := net.InterfaceByName(ifname)
		if err != nil {
			return fmt.Errorf("udp-out %s: bind interface %q: %w", n.cfg.ID, ifname, err)
		}
		n.multicastIface = nif
	}

	return nil
}

func (n *UDPOutNode) SetSend(fn flow.SendFunc)     { n.send = fn }
func (n *UDPOutNode) SetStatus(fn flow.StatusFunc) { n.status = fn }
func (n *UDPOutNode) SetDebug(fn flow.DebugFunc)   { n.debug = fn }
func (n *UDPOutNode) SetError(fn flow.ErrorFunc)   { n.errorFn = fn }

func (n *UDPOutNode) Start() error { return nil }

func (n *UDPOutNode) Stop() error {
	n.closeSocket()
	slog.Info("udp-out stopped", "node_id", n.cfg.ID)
	return nil
}

func (n *UDPOutNode) closeSocket() {
	n.sockMu.Lock()
	defer n.sockMu.Unlock()
	if n.conn != nil {
		_ = n.conn.Close()
		n.conn = nil
		n.pc4 = nil
	}
}

// HandleMessage sends one datagram per inbound message. The node has
// no outputs (sink) so the return is always nil/(nil, error).
func (n *UDPOutNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil {
		return nil, nil
	}

	host, port, err := n.resolveDest(msg)
	if err != nil {
		return nil, fmt.Errorf("udp-out %s: %w", n.cfg.ID, err)
	}

	body, err := encodeOutboundBody(msg)
	if err != nil {
		return nil, fmt.Errorf("udp-out %s: encode: %w", n.cfg.ID, err)
	}

	dst, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return nil, fmt.Errorf("udp-out %s: resolve %s:%d: %w", n.cfg.ID, host, port, err)
	}

	if err := n.sendDatagram(body, dst); err != nil {
		n.setStatus("yellow", "send: "+truncateStatus(err.Error(), 32))
		return nil, fmt.Errorf("udp-out %s: send: %w", n.cfg.ID, err)
	}
	n.setStatus("green", fmt.Sprintf("sent · %dB → %s", len(body), dst.String()))
	return nil, nil
}

func (n *UDPOutNode) resolveDest(msg *flow.Message) (string, int, error) {
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
	if port <= 0 || port > 65535 {
		return "", 0, fmt.Errorf("invalid port %d", port)
	}
	return host, port, nil
}

// sendDatagram writes body to dst, opening (or reusing) the socket
// according to the node's mode and reuseSocket setting.
func (n *UDPOutNode) sendDatagram(body []byte, dst *net.UDPAddr) error {
	conn, _, err := n.acquireSocket()
	if err != nil {
		return err
	}
	if !n.reuseSocket {
		defer n.closeSocket()
	}
	_, err = conn.WriteToUDP(body, dst)
	return err
}

// acquireSocket returns the cached socket (creating it on first call
// when reuseSocket=true), or opens a fresh one for one-shot writes.
// The pc4 return is the multicast wrapper when applicable; not yet
// surfaced to callers but kept around in case future overrides are
// added per-message.
func (n *UDPOutNode) acquireSocket() (*net.UDPConn, *ipv4.PacketConn, error) {
	if n.reuseSocket {
		n.sockMu.Lock()
		if n.conn != nil {
			c, pc := n.conn, n.pc4
			n.sockMu.Unlock()
			return c, pc, nil
		}
		n.sockMu.Unlock()
	}

	conn, pc, err := n.openSocket()
	if err != nil {
		return nil, nil, err
	}
	if n.reuseSocket {
		n.sockMu.Lock()
		n.conn = conn
		n.pc4 = pc
		n.sockMu.Unlock()
	}
	return conn, pc, nil
}

func (n *UDPOutNode) openSocket() (*net.UDPConn, *ipv4.PacketConn, error) {
	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, nil, fmt.Errorf("listen: %w", err)
	}

	if n.mode == udpOutModeBroadcast {
		if err := setBroadcast(conn); err != nil {
			_ = conn.Close()
			return nil, nil, fmt.Errorf("set SO_BROADCAST: %w", err)
		}
	}

	var pc *ipv4.PacketConn
	if n.mode == udpOutModeMulticast {
		pc = ipv4.NewPacketConn(conn)
		if err := pc.SetMulticastTTL(n.multicastTTL); err != nil {
			_ = conn.Close()
			return nil, nil, fmt.Errorf("multicast TTL: %w", err)
		}
		if err := pc.SetMulticastLoopback(n.multicastLoopback); err != nil {
			_ = conn.Close()
			return nil, nil, fmt.Errorf("multicast loopback: %w", err)
		}
		if n.multicastIface != nil {
			if err := pc.SetMulticastInterface(n.multicastIface); err != nil {
				_ = conn.Close()
				return nil, nil, fmt.Errorf("multicast interface: %w", err)
			}
		}
	}
	return conn, pc, nil
}

// setBroadcast enables SO_BROADCAST on the given connection so the
// kernel will allow writes to broadcast addresses (255.255.255.255 and
// per-subnet directed broadcasts). Cross-platform: syscall.SO_BROADCAST
// is defined on linux/darwin/windows.
func setBroadcast(c *net.UDPConn) error {
	rc, err := c.SyscallConn()
	if err != nil {
		return err
	}
	var opErr error
	cerr := rc.Control(func(fd uintptr) {
		opErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
	})
	if cerr != nil {
		return cerr
	}
	return opErr
}

func (n *UDPOutNode) setStatus(fill, text string) {
	if n.status != nil {
		n.status(fill, text)
	}
}

// UDPOutTypeInfo returns the NodeTypeInfo for the udp-out node.
func UDPOutTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "udp-out",
		Category:    "network",
		Label:       "UDP Send",
		Description: "Send msg.payload as a UDP datagram (unicast / broadcast / multicast)",
		Icon:        "mdi-broadcast",
		Defaults: map[string]any{
			"host":              "",
			"port":              0,
			"bindHost":          "",
			"mode":              udpOutModeUnicast,
			"multicastTTL":      defaultUdpMulticastTTL,
			"multicastLoopback": true,
			"reuseSocket":       true,
		},
		Inputs:  1,
		Outputs: 0,
	}
}
