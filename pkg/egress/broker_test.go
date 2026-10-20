package egress

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testBroker(t *testing.T, hosts []string) *Broker {
	t.Helper()
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := probe.Addr().String()
	_ = probe.Close()
	broker, err := ListenTCP(addr, hosts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = broker.Close() })
	return broker
}

func brokerRequest(t *testing.T, addr, host, token string) (net.Conn, *http.Response) {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	header := ""
	if token != "" {
		header = "Proxy-Authorization: Bearer " + token + "\r\n"
	}
	if _, err = fmt.Fprintf(conn, "CONNECT %s:443 HTTP/1.1\r\nHost: %s:443\r\n%s\r\n", host, host, header); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	return conn, response
}

func TestBrokerRequiresTokenAndRegistryPublicIP(t *testing.T) {
	broker := testBroker(t, Registries)
	var dials atomic.Int32
	broker.proxy.resolve = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}
	broker.proxy.dial = func(context.Context, string, string) (net.Conn, error) {
		dials.Add(1)
		return nil, fmt.Errorf("unexpected dial")
	}
	token, err := broker.NewSession(time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, host, token string
		want              int
	}{{"missing token", "registry.npmjs.org", "", http.StatusProxyAuthRequired}, {"bad token", "registry.npmjs.org", "not-a-token", http.StatusProxyAuthRequired}, {"unapproved host", "example.com", token, http.StatusForbidden}, {"private IP", "registry.npmjs.org", token, http.StatusForbidden}} {
		t.Run(tc.name, func(t *testing.T) {
			conn, response := brokerRequest(t, broker.listener.Addr().String(), tc.host, tc.token)
			defer conn.Close()
			if response.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", response.StatusCode, tc.want)
			}
		})
	}
	if got := dials.Load(); got != 0 {
		t.Fatalf("dial count = %d, want 0", got)
	}
}

func TestBrokerRevokeClosesActiveAndInFlightConnections(t *testing.T) {
	broker := testBroker(t, Registries)
	broker.proxy.resolve = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	upstreamServer, upstreamClient := net.Pipe()
	defer upstreamServer.Close()
	broker.proxy.dial = func(context.Context, string, string) (net.Conn, error) {
		return upstreamClient, nil
	}
	token, err := broker.NewSession(time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	client, response := brokerRequest(t, broker.listener.Addr().String(), "registry.npmjs.org", token)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	payload := bytes.Repeat([]byte("t"), maxBrokerHeaderBytes+1024)
	writeDone := make(chan error, 1)
	go func() { _, err := upstreamServer.Write(payload); writeDone <- err }()
	received := make([]byte, len(payload))
	if _, err := io.ReadFull(client, received); err != nil || !bytes.Equal(received, payload) {
		t.Fatalf("tunnel payload mismatch: received=%d error=%v", len(received), err)
	}
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}
	if !broker.Revoke(token) {
		t.Fatal("Revoke returned false")
	}
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := client.Read(make([]byte, 1)); err == nil {
		t.Fatal("client tunnel remained open after revoke")
	}
	_ = client.Close()
	_ = upstreamServer.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := upstreamServer.Read(make([]byte, 1)); err == nil {
		t.Fatal("upstream tunnel remained open after revoke")
	}
	if _, response := brokerRequest(t, broker.listener.Addr().String(), "registry.npmjs.org", token); response.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("revoked session status = %d, want %d", response.StatusCode, http.StatusProxyAuthRequired)
	}

	entered := make(chan struct{})
	broker.proxy.dial = func(ctx context.Context, _, _ string) (net.Conn, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	inflightToken, err := broker.NewSession(time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	inflight, err := net.Dial("tcp", broker.listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fmt.Fprintf(inflight, "CONNECT registry.npmjs.org:443 HTTP/1.1\r\nHost: registry.npmjs.org:443\r\nProxy-Authorization: Bearer %s\r\n\r\n", inflightToken)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("dial did not start")
	}
	if !broker.Revoke(inflightToken) {
		t.Fatal("in-flight session revoke returned false")
	}
	_ = inflight.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 64)
	if n, err := inflight.Read(buf); err == nil || strings.Contains(string(buf[:n]), "200 Connection Established") {
		t.Fatal("in-flight request was not stopped by revoke")
	}
	_ = inflight.Close()
}

func TestBrokerRejectsOversizedUnauthenticatedHeaders(t *testing.T) {
	broker := testBroker(t, Registries)
	var dials atomic.Int32
	broker.proxy.resolve = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	broker.proxy.dial = func(context.Context, string, string) (net.Conn, error) {
		dials.Add(1)
		return nil, fmt.Errorf("unexpected dial")
	}
	token, err := broker.NewSession(time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("tcp", broker.listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	_, err = fmt.Fprintf(conn, "CONNECT registry.npmjs.org:443 HTTP/1.1\r\nHost: registry.npmjs.org:443\r\nProxy-Authorization: Bearer %s\r\nX-Pad: %s\r\n\r\n", token, strings.Repeat("x", maxBrokerHeaderBytes))
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, err = conn.Read(make([]byte, 1)); err == nil {
		t.Fatal("oversized request remained open")
	}
	_ = conn.Close()
	if got := dials.Load(); got != 0 {
		t.Fatalf("oversized request reached dialer: %d", got)
	}
}

func TestBrokerSessionLifetimeBound(t *testing.T) {
	broker := testBroker(t, Registries)
	if _, err := broker.NewSession(maxSessionLifetime + time.Nanosecond); err != ErrSessionLifetime {
		t.Fatalf("error = %v, want %v", err, ErrSessionLifetime)
	}
}
