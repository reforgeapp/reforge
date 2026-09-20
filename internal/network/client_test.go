package network

import (
	"context"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func routeOptions(host string, cidrs ...string) Options {
	return Options{RunnerID: "runner", PrivateRoute: &PrivateRoute{OrgID: "org", ConnectionID: "connection", RunnerID: "runner", Host: host, CIDRs: cidrs}}
}

func TestEndpointAndPrivateRouteBoundaries(t *testing.T) {
	for _, endpoint := range []string{
		"http://example.com", "https://user:password@example.com", "https://example.com?token=secret", "https://example.com?", "https://example.com#fragment", "https://example.com#",
		"https://127.0.0.1", "https://[::1]", "https://[::ffff:127.0.0.1]", "https://[::ffff:10.0.0.1]", "https://10.0.0.1", "https://172.16.1.1", "https://192.168.1.1", "https://[fd12::1]",
		"https://169.254.169.254", "https://[fe80::1]", "https://[fe80::1%25eth0]", "https://[fd00:ec2::254]", "https://168.63.129.16", "https://100.100.100.200",
		"https://224.0.0.1", "https://[ff02::1]", "https://0.0.0.0", "https://[::]", "https://255.255.255.255", "https://0.1.2.3", "https://[64:ff9b::a00:1]", "https://[2002:a00:1::]",
		"https://example.com:0", "https://example.com:65536", "https://example.com:0443", "https://example.com:", "https://example.com./api", "https://example.com/a/../b", "https://example.com/a/%252e%252e/b", "https://example.com/a%5cb",
	} {
		t.Run(endpoint, func(t *testing.T) {
			if _, err := ValidateEndpoint(endpoint, Options{}); !errors.Is(err, ErrDestination) {
				t.Fatalf("unsafe endpoint accepted: %v", err)
			}
		})
	}
	for _, endpoint := range []string{"https://forge.example/api/v4", "https://8.8.8.8", "https://[2606:4700:4700::1111]"} {
		if _, err := ValidateEndpoint(endpoint, Options{}); err != nil {
			t.Fatalf("public endpoint rejected: %s %v", endpoint, err)
		}
	}
	good := routeOptions("10.1.2.3", "10.1.2.0/24")
	if _, err := ValidateEndpoint("https://10.1.2.3/api", good); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Options){
		func(o *Options) { o.RunnerID = "" }, func(o *Options) { o.RunnerID = "other" }, func(o *Options) { o.PrivateRoute.Host = "other.example" }, func(o *Options) { o.PrivateRoute.OrgID = "" }, func(o *Options) { o.PrivateRoute.ConnectionID = "" },
		func(o *Options) { o.PrivateRoute.CIDRs = nil }, func(o *Options) { o.PrivateRoute.CIDRs = []string{"10.2.0.0/16"} }, func(o *Options) { o.PrivateRoute.CIDRs = []string{"10.1.2.3/24"} }, func(o *Options) { o.PrivateRoute.CIDRs = []string{"::ffff:10.0.0.0/104"} },
	} {
		options := routeOptions("10.1.2.3", "10.1.2.0/24")
		mutate(&options)
		if _, err := NewClient("https://10.1.2.3", options); !errors.Is(err, ErrDestination) {
			t.Fatalf("bad route accepted: %+v %v", options, err)
		}
	}
	for _, host := range []string{"169.254.169.254", "168.63.129.16", "100.100.100.200", "fd00:ec2::254", "fe80::1", "224.0.0.1", "127.0.0.2", "::1"} {
		endpoint := "https://" + host
		cidr := "0.0.0.0/0"
		if strings.Contains(host, ":") {
			endpoint = "https://[" + host + "]"
			cidr = "::/0"
		}
		options := routeOptions(host, cidr)
		options.Development = true
		if _, err := NewClient(endpoint, options); !errors.Is(err, ErrDestination) {
			t.Fatalf("private route admitted special address: %s %v", host, err)
		}
	}
	options := routeOptions("127.0.0.1", "127.0.0.1/32")
	if _, err := NewClient("http://127.0.0.1", options); !errors.Is(err, ErrDestination) {
		t.Fatal("production loopback HTTP accepted")
	}
	options.Development = true
	if _, err := NewClient("http://127.0.0.1", options); err != nil {
		t.Fatal(err)
	}
	options = routeOptions("10.1.2.3", "10.1.2.0/24")
	options.Development = true
	if _, err := NewClient("http://10.1.2.3", options); err != nil {
		t.Fatal(err)
	}
	options = routeOptions("8.8.8.8", "8.8.8.8/32")
	options.Development = true
	if _, err := NewClient("http://8.8.8.8", options); !errors.Is(err, ErrDestination) {
		t.Fatal("unencrypted public route accepted")
	}
}

func TestDNSRebindingAndLiteralDial(t *testing.T) {
	p, err := newPolicy("https://forge.example/api", Options{})
	if err != nil {
		t.Fatal(err)
	}
	resolutions, dials := 0, 0
	lookup := func(context.Context, string) ([]netip.Addr, error) {
		resolutions++
		if resolutions == 1 {
			return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
		}
		return []netip.Addr{netip.MustParseAddr("10.0.0.1")}, nil
	}
	dial := func(_ context.Context, network, address string) (net.Conn, error) {
		dials++
		if network != "tcp" || address != "8.8.8.8:443" {
			t.Fatalf("hostname or wrong IP dial: %s %s", network, address)
		}
		a, b := net.Pipe()
		b.Close()
		return a, nil
	}
	guarded := p.dial(lookup, dial)
	connection, err := guarded(context.Background(), "tcp", "forge.example:443")
	if err != nil {
		t.Fatal(err)
	}
	connection.Close()
	if _, err = guarded(context.Background(), "tcp", "forge.example:443"); !errors.Is(err, ErrDestination) {
		t.Fatalf("rebind accepted: %v", err)
	}
	if dials != 1 || resolutions != 2 {
		t.Fatalf("rebind reached dial: dials=%d resolutions=%d", dials, resolutions)
	}
	if _, err = guarded(context.Background(), "tcp", "other.example:443"); !errors.Is(err, ErrDestination) {
		t.Fatal("dial authority changed")
	}
	for _, answers := range [][]string{{"8.8.8.8", "127.0.0.1"}, {"::ffff:169.254.169.254"}, {"::ffff:192.168.1.1"}, {"fd00:ec2::254"}, {}} {
		lookup := func(context.Context, string) ([]netip.Addr, error) {
			ips := []netip.Addr{}
			for _, a := range answers {
				ips = append(ips, netip.MustParseAddr(a))
			}
			return ips, nil
		}
		if _, err = p.dial(lookup, dial)(context.Background(), "tcp", "forge.example:443"); !errors.Is(err, ErrDestination) {
			t.Fatalf("unsafe DNS answers accepted: %+v %v", answers, err)
		}
	}
	if dials != 1 {
		t.Fatal("unsafe DNS answer reached network")
	}
}

func TestRequestOriginPathAndMethodGuard(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("ALL_PROXY", "http://127.0.0.1:1")
	t.Setenv("NO_PROXY", "")
	var dials atomic.Int32
	client, err := newClient("https://forge.example/api/v4", Options{}, func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}, func(context.Context, string, string) (net.Conn, error) {
		dials.Add(1)
		return nil, errors.New("fixture dial")
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, destination := range []string{
		"https://attacker.example/api/v4/projects", "http://forge.example/api/v4/projects", "https://forge.example:444/api/v4/projects", "https://forge.example/api/v40", "https://forge.example/api", "https://forge.example/api/v4/../admin", "https://forge.example/api/v4/%2e%2e/admin", "https://forge.example/api/v4/%252e%252e/admin", "https://forge.example/api/v4/..;/admin", "https://forge.example/api/v4/%5c..%5cadmin", "https://forge.example/api/v4//projects", "https://user:password@forge.example/api/v4", "https://forge.example/api/v4#fragment",
	} {
		request, err := http.NewRequest("GET", destination, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer connection-secret")
		if _, err = client.Do(request); !errors.Is(err, ErrRequest) {
			t.Fatalf("unsafe request accepted: %s %v", destination, err)
		}
	}
	for _, method := range []string{"CONNECT", "TRACE", "OPTIONS", "PROPFIND"} {
		request, _ := http.NewRequest(method, "https://forge.example/api/v4", nil)
		if _, err = client.Do(request); !errors.Is(err, ErrRequest) {
			t.Fatalf("method accepted: %s", method)
		}
	}
	for _, modify := range []func(*http.Request){func(r *http.Request) { r.Host = "attacker.example" }, func(r *http.Request) { r.Header.Set("Proxy-Authorization", "secret") }, func(r *http.Request) { r.Header.Set("Upgrade", "websocket") }, func(r *http.Request) { r.URL.Opaque = "//attacker.example" }} {
		request, _ := http.NewRequest("GET", "https://forge.example/api/v4", nil)
		modify(request)
		if _, err = client.Do(request); !errors.Is(err, ErrRequest) {
			t.Fatal("request authority override accepted")
		}
	}
	if dials.Load() != 0 {
		t.Fatal("rejected request reached dial")
	}
	request, _ := http.NewRequest("GET", "https://forge.example/api/v4/projects/group%2Frepo?per_page=50", nil)
	if _, err = client.Do(request); errors.Is(err, ErrRequest) || dials.Load() != 1 {
		t.Fatalf("valid encoded provider ID rejected: %v", err)
	}
}

func TestPrivateDNSCIDRAndRouteSnapshot(t *testing.T) {
	options := routeOptions("forge.internal", "10.2.0.0/16")
	p, err := newPolicy("https://forge.internal", options)
	if err != nil {
		t.Fatal(err)
	}
	options.PrivateRoute.Host = "attacker.example"
	options.PrivateRoute.CIDRs[0] = "0.0.0.0/0"
	count := 0
	dial := func(_ context.Context, _, address string) (net.Conn, error) {
		count++
		if address != "10.2.3.4:443" {
			t.Fatal("unexpected private address")
		}
		a, b := net.Pipe()
		b.Close()
		return a, nil
	}
	resolve := func(value string) lookupFunc {
		return func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr(value)}, nil
		}
	}
	conn, err := p.dial(resolve("10.2.3.4"), dial)(context.Background(), "tcp", "forge.internal:443")
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	for _, answer := range []string{"10.3.3.4", "169.254.169.254", "127.0.0.1", "8.8.8.8"} {
		if _, err = p.dial(resolve(answer), dial)(context.Background(), "tcp", "forge.internal:443"); !errors.Is(err, ErrDestination) {
			t.Fatalf("CIDR escape: %s %v", answer, err)
		}
	}
	if count != 1 {
		t.Fatal("private exception changed after construction")
	}
}

func TestLocalRedirectAndCustomCA(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/api/redirect" {
			w.Header().Set("Location", "https://attacker.example/steal")
			w.WriteHeader(302)
			return
		}
		if r.Header.Get("Authorization") != "Bearer fixture" {
			t.Error("fixture credential missing")
		}
		w.Write([]byte("verified"))
	}))
	defer server.Close()
	endpoint, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	options := routeOptions(endpoint.Hostname(), "127.0.0.1/32")
	options.Development = true
	client, err := NewClient(server.URL+"/api", options)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.Get(server.URL + "/api/status"); err == nil {
		t.Fatal("untrusted TLS certificate accepted")
	}
	client.CloseIdleConnections()
	options.CAPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	client, err = NewClient(server.URL+"/api", options)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("ALL_PROXY", "http://127.0.0.1:1")
	t.Setenv("NO_PROXY", "")
	request, _ := http.NewRequest("GET", server.URL+"/api/status", nil)
	request.Header.Set("Authorization", "Bearer fixture")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || string(data) != "verified" {
		t.Fatal("CA verification failed")
	}
	request, _ = http.NewRequest("GET", server.URL+"/api/redirect", nil)
	request.Header.Set("Authorization", "Bearer fixture")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 302 || calls.Load() != 2 {
		t.Fatal("redirect followed")
	}
	wrongHostClient, err := newClient("https://wrong.example/api", Options{CAPEM: options.CAPEM}, func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}, func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, endpoint.Host)
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = wrongHostClient.Get("https://wrong.example/api/status"); err == nil {
		t.Fatal("custom CA disabled hostname verification")
	}
	wrongHostClient.CloseIdleConnections()
	options.CAPEM = []byte("not a certificate")
	if _, err = NewClient(server.URL+"/api", options); err == nil {
		t.Fatal("invalid CA accepted")
	}
}

func TestTurnTimeoutRetainsDestinationBoundary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(20 * time.Millisecond); w.WriteHeader(204) }))
	defer server.Close()
	endpoint, _ := url.Parse(server.URL)
	options := routeOptions(endpoint.Hostname(), "127.0.0.1/32")
	options.Development = true
	original, err := NewClient(server.URL+"/api", options)
	if err != nil {
		t.Fatal(err)
	}
	defer original.CloseIdleConnections()
	original.Transport.(*transport).transport.ResponseHeaderTimeout = time.Millisecond
	scoped, err := WithTimeout(original, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer scoped.CloseIdleConnections()
	response, err := scoped.Get(server.URL + "/api/model")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if _, err = scoped.Get(server.URL + "/outside"); !errors.Is(err, ErrRequest) {
		t.Fatalf("timeout escaped path boundary: %v", err)
	}
	if original.Transport.(*transport).transport.ResponseHeaderTimeout != time.Millisecond {
		t.Fatal("original transport changed")
	}
}
