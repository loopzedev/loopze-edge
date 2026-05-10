// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package network

import (
	"github.com/loopzedev/loopze-edge/internal/nodes"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/ipv4"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

const (
	defaultUdpInMaxDatagram = 65507 // max UDPv4 payload (65535 − 8 UDP − 20 IP)
)

// UDPInNode binds a UDP port and emits one message per received
// datagram on output 0. Optionally joins one or more IPv4 multicast
// groups; CIDR allowlist drops senders that don't match.
type UDPInNode struct {
	cfg flow.NodeConfig

	nodes.BaseNode
	errorFn flow.ErrorFunc

	// Parsed configuration.
	host             string
	port             int
	multicastGroups  []multicastGroupCfg
	payloadEncoding  string
	maxDatagramBytes int
	allowedNets      []*net.IPNet

	// Runtime state.
	mu      sync.Mutex
	conn    *net.UDPConn
	pc4     *ipv4.PacketConn // wraps conn when multicast groups are configured
	stopCh  chan struct{}
	stopped bool
	wg      sync.WaitGroup
}

// multicastGroupCfg is a parsed entry from the multicastGroups config.
// iface is nil when the operator did not pin an interface (OS picks).
type multicastGroupCfg struct {
	addr  *net.UDPAddr
	iface *net.Interface
	raw   string // for diagnostics
}

// NewUDPInNode is the NodeFactory for the "udp-in" node type.
func NewUDPInNode(cfg flow.NodeConfig) (flow.NodeInstance, error) {
	return &UDPInNode{cfg: cfg, stopCh: make(chan struct{})}, nil
}

func (n *UDPInNode) Init() error {
	props := n.cfg.Properties

	n.host = strings.TrimSpace(nodes.StringVal(props, "host", "0.0.0.0"))
	n.port = nodes.IntVal(props, "port", 0)
	if n.port < 0 || n.port > 65535 {
		return fmt.Errorf("udp-in %s: invalid port %d", n.cfg.ID, n.port)
	}

	n.payloadEncoding = strings.ToLower(nodes.StringVal(props, "payloadEncoding", tcpEncBuffer))
	switch n.payloadEncoding {
	case tcpEncBuffer, tcpEncString, tcpEncBase64:
	default:
		return fmt.Errorf("udp-in %s: invalid payloadEncoding %q", n.cfg.ID, n.payloadEncoding)
	}

	n.maxDatagramBytes = nodes.IntVal(props, "maxDatagramBytes", defaultUdpInMaxDatagram)
	if n.maxDatagramBytes <= 0 {
		n.maxDatagramBytes = defaultUdpInMaxDatagram
	}

	for _, raw := range stringSlice(props, "multicastGroups") {
		grp, err := parseMulticastGroup(raw)
		if err != nil {
			return fmt.Errorf("udp-in %s: %w", n.cfg.ID, err)
		}
		n.multicastGroups = append(n.multicastGroups, grp)
	}

	for _, raw := range stringSlice(props, "allowedRemotes") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		_, ipnet, err := net.ParseCIDR(raw)
		if err != nil {
			return fmt.Errorf("udp-in %s: invalid CIDR %q: %w", n.cfg.ID, raw, err)
		}
		n.allowedNets = append(n.allowedNets, ipnet)
	}

	return nil
}

// parseMulticastGroup accepts "224.0.0.251" or "224.0.0.251%eth0"
// (interface-pinned). Returns the parsed *net.UDPAddr and resolved
// *net.Interface (or nil when no interface is pinned).
func parseMulticastGroup(raw string) (multicastGroupCfg, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return multicastGroupCfg{}, errors.New("empty multicast group entry")
	}
	addr := raw
	var iface *net.Interface
	if i := strings.Index(raw, "%"); i >= 0 {
		ifname := raw[i+1:]
		addr = raw[:i]
		nif, err := net.InterfaceByName(ifname)
		if err != nil {
			return multicastGroupCfg{}, fmt.Errorf("multicast group %q: interface %q: %w", raw, ifname, err)
		}
		iface = nif
	}
	ip := net.ParseIP(addr)
	if ip == nil || !ip.IsMulticast() {
		return multicastGroupCfg{}, fmt.Errorf("multicast group %q: not a valid multicast IP", raw)
	}
	return multicastGroupCfg{
		addr:  &net.UDPAddr{IP: ip},
		iface: iface,
		raw:   raw,
	}, nil
}

func (n *UDPInNode) SetError(fn flow.ErrorFunc)   { n.errorFn = fn }

func (n *UDPInNode) Start() error {
	addr := &net.UDPAddr{IP: net.ParseIP(n.host), Port: n.port}
	if addr.IP == nil {
		// Fallback: caller passed a name (rare for UDP listen) or
		// the wildcard form 0.0.0.0/::. Let net.ResolveUDPAddr take
		// a stab.
		resolved, err := net.ResolveUDPAddr("udp", net.JoinHostPort(n.host, strconv.Itoa(n.port)))
		if err != nil {
			n.setStatus("red", "resolve: "+truncateStatus(err.Error(), 32))
			return fmt.Errorf("udp-in %s: resolve %s: %w", n.cfg.ID, n.host, err)
		}
		addr = resolved
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		n.setStatus("red", "bind: "+truncateStatus(err.Error(), 32))
		return fmt.Errorf("udp-in %s: listen: %w", n.cfg.ID, err)
	}

	if len(n.multicastGroups) > 0 {
		pc := ipv4.NewPacketConn(conn)
		for _, grp := range n.multicastGroups {
			if err := pc.JoinGroup(grp.iface, grp.addr); err != nil {
				// A failed join is reported but the listener stays
				// up (per spec § "Multicast reception lifecycle").
				if n.errorFn != nil {
					n.errorFn(fmt.Errorf("udp-in %s: join %s: %w", n.cfg.ID, grp.raw, err), nil)
				}
				slog.Warn("udp-in: multicast join failed",
					"node_id", n.cfg.ID, "group", grp.raw, "err", err)
			}
		}
		n.pc4 = pc
	}

	n.mu.Lock()
	n.conn = conn
	n.mu.Unlock()

	n.publishStatus()
	n.wg.Add(1)
	go n.readLoop()
	slog.Info("udp-in listening", "node_id", n.cfg.ID, "addr", conn.LocalAddr().String(),
		"multicast_groups", len(n.multicastGroups))
	return nil
}

func (n *UDPInNode) Stop() error {
	n.mu.Lock()
	if n.stopped {
		n.mu.Unlock()
		return nil
	}
	n.stopped = true
	close(n.stopCh)
	pc := n.pc4
	conn := n.conn
	groups := n.multicastGroups
	n.mu.Unlock()

	if pc != nil {
		for _, grp := range groups {
			_ = pc.LeaveGroup(grp.iface, grp.addr)
		}
	}
	if conn != nil {
		_ = conn.Close() // unblocks ReadFromUDP
	}
	n.wg.Wait()
	slog.Info("udp-in stopped", "node_id", n.cfg.ID)
	return nil
}

// HandleMessage is unreachable for source nodes (udp-in has 0 inputs)
// but the interface requires the method.
func (n *UDPInNode) HandleMessage(_ *flow.Message) ([][]*flow.Message, error) {
	return nil, nil
}

func (n *UDPInNode) readLoop() {
	defer n.wg.Done()
	buf := make([]byte, n.maxDatagramBytes)
	localAddr := n.conn.LocalAddr().String()
	for {
		// Use a generous read deadline so Stop() can interrupt
		// without an external signal — Close() does that, but a
		// belt-and-braces deadline keeps the loop responsive on
		// platforms where Close on a UDPConn does not always
		// unblock ReadFromUDP immediately.
		_ = n.conn.SetReadDeadline(time.Now().Add(time.Second))

		nb, src, err := n.conn.ReadFromUDP(buf)
		if err != nil {
			if n.isStopped() {
				return
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue // periodic deadline tick
			}
			if isExpectedCloseErr(err) {
				return
			}
			if n.errorFn != nil {
				n.errorFn(fmt.Errorf("udp-in %s: read: %w", n.cfg.ID, err), nil)
			}
			return
		}
		if nb == 0 {
			continue
		}
		if !n.checkAllowed(src) {
			continue
		}

		datagram := make([]byte, nb)
		copy(datagram, buf[:nb])
		truncated := nb == n.maxDatagramBytes // best-effort indicator
		n.emit(datagram, src, localAddr, truncated)
	}
}

func (n *UDPInNode) checkAllowed(src *net.UDPAddr) bool {
	if len(n.allowedNets) == 0 {
		return true
	}
	for _, ipnet := range n.allowedNets {
		if ipnet.Contains(src.IP) {
			return true
		}
	}
	return false
}

func (n *UDPInNode) emit(datagram []byte, src *net.UDPAddr, localAddr string, truncated bool) {
	if n.Send == nil {
		return
	}
	msg := flow.NewMessage()
	msg.SetPayload(encodePayload(datagram, n.payloadEncoding))
	msg.Set("ip", src.IP.String())
	msg.Set("port", src.Port)
	msg.Set("remoteAddr", src.String())
	msg.Set("localAddr", localAddr)
	msg.Set("size", len(datagram))
	if truncated {
		msg.Set("truncated", true)
	}
	n.Send(0, msg)
}

func (n *UDPInNode) isStopped() bool {
	select {
	case <-n.stopCh:
		return true
	default:
		return false
	}
}

func (n *UDPInNode) publishStatus() {
	addr := net.JoinHostPort(n.host, strconv.Itoa(n.port))
	if len(n.multicastGroups) > 0 {
		n.setStatus("green", "listening · "+addr+" · mcast")
		return
	}
	n.setStatus("green", "listening · "+addr)
}

func (n *UDPInNode) setStatus(fill, text string) {
	if n.Status != nil {
		n.Status(fill, text)
	}
}

// UDPInTypeInfo returns the NodeTypeInfo for the udp-in node.
func UDPInTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "udp-in",
		Category:    "network",
		Label:       "UDP Receive",
		Description: "Bind a UDP port and emit one message per received datagram",
		Icon:        "mdi-broadcast",
		Defaults: map[string]any{
			"host":             "0.0.0.0",
			"port":             5000,
			"payloadEncoding":  tcpEncBuffer,
			"maxDatagramBytes": defaultUdpInMaxDatagram,
		},
		Inputs:  0,
		Outputs: 1,
	}
}
