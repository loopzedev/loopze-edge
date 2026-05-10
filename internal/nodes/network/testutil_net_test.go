// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package network

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"testing"
	"time"
)

// listenLoopback opens a TCP listener on 127.0.0.1:0 (OS-assigned port)
// and returns the listener plus the chosen port. The listener is
// registered with t.Cleanup so tests don't have to remember to close
// it.
func listenLoopback(t *testing.T) (*net.TCPListener, int) {
	t.Helper()
	l, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listenLoopback: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l, l.Addr().(*net.TCPAddr).Port
}

// dialLoopback opens a client connection to 127.0.0.1:port. The conn is
// registered with t.Cleanup.
func dialLoopback(t *testing.T, port int) net.Conn {
	t.Helper()
	c, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", itoa(port)), 2*time.Second)
	if err != nil {
		t.Fatalf("dialLoopback: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// echoServer accepts a single connection on l, echoes everything it
// reads, and closes when the peer half-closes. Useful as a remote end
// for tcp-request tests.
func echoServer(t *testing.T, l *net.TCPListener) {
	t.Helper()
	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_, _ = io.Copy(c, c)
	}()
}

// generateSelfSignedCert builds a fresh self-signed TLS certificate
// valid for "127.0.0.1" / "localhost". Returns the PEM-encoded cert
// (suitable for caBundle config), the parsed key pair (suitable for
// tls.Listener), and the *x509.CertPool that trusts it.
func generateSelfSignedCert(t *testing.T) ([]byte, tls.Certificate, *x509.CertPool) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "loopze-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair: %v", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(certPEM) {
		t.Fatal("AppendCertsFromPEM failed")
	}
	return certPEM, pair, pool
}

// listenLoopbackTLS opens a TLS listener on 127.0.0.1:0 with the given
// certificate. Returns the listener and the chosen port.
func listenLoopbackTLS(t *testing.T, cert tls.Certificate) (net.Listener, int) {
	t.Helper()
	cfg := &tls.Config{Certificates: []tls.Certificate{cert}}
	l, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatalf("tls.Listen: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l, l.Addr().(*net.TCPAddr).Port
}

// itoa is a tiny dependency-free integer-to-string helper so tests
// don't pull strconv just for this.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	n := len(buf)
	for i > 0 {
		n--
		buf[n] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		n--
		buf[n] = '-'
	}
	return string(buf[n:])
}
