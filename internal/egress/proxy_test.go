package egress

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
)

func TestProxyAllowsOnlyPublicRegistryTunnels(t *testing.T) {
	p, err := Listen(filepath.Join(t.TempDir(), "egress.sock"), []string{"registry.npmjs.org", "evil.example"})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	dialed := ""
	p.resolve = func(_ context.Context, host string) ([]netip.Addr, error) {
		if host == "evil.example" {
			return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
		}
		return []netip.Addr{netip.MustParseAddr("104.16.0.35")}, nil
	}
	p.dial = func(_ context.Context, _ string, address string) (net.Conn, error) {
		dialed = address
		server, client := net.Pipe()
		go server.Close()
		return client, nil
	}
	status := func(request string) int {
		conn, err := net.Dial("unix", p.listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		_, _ = conn.Write([]byte(request))
		response, err := http.ReadResponse(bufio.NewReader(conn), nil)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode
	}
	if code := status("CONNECT registry.npmjs.org:443 HTTP/1.1\r\nHost: registry.npmjs.org:443\r\n\r\n"); code != 200 || !strings.HasPrefix(dialed, "104.16.0.35:443") {
		t.Fatalf("registry tunnel code=%d dialed=%s", code, dialed)
	}
	for _, request := range []string{
		"CONNECT evil.example:443 HTTP/1.1\r\nHost: evil.example:443\r\n\r\n",
		"CONNECT localhost:443 HTTP/1.1\r\nHost: localhost:443\r\n\r\n",
		"CONNECT registry.npmjs.org:22 HTTP/1.1\r\nHost: registry.npmjs.org:22\r\n\r\n",
		"GET http://registry.npmjs.org/ HTTP/1.1\r\nHost: registry.npmjs.org\r\n\r\n",
	} {
		if code := status(request); code != 403 {
			t.Fatalf("request %q allowed with %d", request, code)
		}
	}
}
