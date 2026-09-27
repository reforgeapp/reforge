package network

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrDestination = errors.New("connection destination is not permitted")
var ErrRequest = errors.New("request is outside the connection boundary")

type Options struct {
	RunnerID     string
	PrivateRoute *PrivateRoute
	CAPEM        []byte
	Development  bool
}

type PrivateRoute struct {
	OrgID        string   `json:"org_id"`
	ConnectionID string   `json:"connection_id"`
	RunnerID     string   `json:"runner_id"`
	Host         string   `json:"host"`
	CIDRs        []string `json:"cidrs"`
}

type policy struct {
	endpoint    *url.URL
	host        string
	port        string
	path        string
	route       *PrivateRoute
	cidrs       []netip.Prefix
	development bool
}

type lookupFunc func(context.Context, string) ([]netip.Addr, error)
type dialFunc func(context.Context, string, string) (net.Conn, error)

type transport struct {
	policy    *policy
	transport *http.Transport
}

func ValidateEndpoint(endpoint string, options Options) (*url.URL, error) {
	p, err := newPolicy(endpoint, options)
	if err != nil {
		return nil, err
	}
	return p.endpoint, nil
}

func NewClient(endpoint string, options Options) (*http.Client, error) {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return newClient(endpoint, options, func(ctx context.Context, host string) ([]netip.Addr, error) {
		return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	}, dialer.DialContext)
}

func newClient(endpoint string, options Options, lookup lookupFunc, dial dialFunc) (*http.Client, error) {
	p, err := newPolicy(endpoint, options)
	if err != nil {
		return nil, err
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: p.host}
	if len(options.CAPEM) > 0 {
		roots, err := x509.SystemCertPool()
		if err != nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(options.CAPEM) {
			return nil, errors.New("invalid connection CA")
		}
		tlsConfig.RootCAs = roots
	}
	base := &http.Transport{
		Proxy:                  nil,
		DialContext:            p.dial(lookup, dial),
		ForceAttemptHTTP2:      true,
		TLSClientConfig:        tlsConfig,
		TLSHandshakeTimeout:    10 * time.Second,
		ResponseHeaderTimeout:  20 * time.Second,
		ExpectContinueTimeout:  time.Second,
		IdleConnTimeout:        30 * time.Second,
		MaxIdleConns:           10,
		MaxIdleConnsPerHost:    5,
		MaxConnsPerHost:        10,
		MaxResponseHeaderBytes: 1 << 20,
	}
	return &http.Client{Transport: &transport{policy: p, transport: base}, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, nil
}

func newPolicy(endpoint string, options Options) (*policy, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || u.Opaque != "" || strings.Contains(endpoint, "#") || u.Scheme != "https" && u.Scheme != "http" {
		return nil, ErrDestination
	}
	host := u.Hostname()
	if !validHost(host) || strings.HasSuffix(u.Host, ":") {
		return nil, ErrDestination
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port {
		return nil, ErrDestination
	}
	base, err := safePath(u.EscapedPath())
	if err != nil {
		return nil, ErrDestination
	}
	p := &policy{endpoint: u, host: strings.ToLower(host), port: port, path: strings.TrimSuffix(base, "/"), development: options.Development}
	if options.PrivateRoute != nil {
		r := *options.PrivateRoute
		r.CIDRs = append([]string(nil), r.CIDRs...)
		if options.RunnerID == "" || r.RunnerID != options.RunnerID || r.OrgID == "" || r.ConnectionID == "" || r.Host != host || len(r.CIDRs) == 0 || len(r.CIDRs) > 64 {
			return nil, ErrDestination
		}
		for _, raw := range r.CIDRs {
			prefix, err := netip.ParsePrefix(raw)
			if err != nil || prefix.Addr().Is4In6() || prefix != prefix.Masked() {
				return nil, ErrDestination
			}
			p.cidrs = append(p.cidrs, prefix)
		}
		p.route = &r
	}
	if u.Scheme == "http" && (!options.Development || p.route == nil) {
		return nil, ErrDestination
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if !p.permits(ip) {
			return nil, ErrDestination
		}
	}
	return p, nil
}

func validHost(host string) bool {
	if host == "" || len(host) > 253 || strings.ContainsAny(host, "%\\") || strings.HasSuffix(host, ".") {
		return false
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return true
	}
	if strings.Contains(host, ":") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

func safePath(raw string) (string, error) {
	if raw == "" {
		return "/", nil
	}
	for i := 0; i < 8; i++ {
		for _, c := range raw {
			if c < ' ' || c == 127 || c == '\\' || c == ';' {
				return "", ErrRequest
			}
		}
		if !strings.HasPrefix(raw, "/") || strings.Contains(raw, "//") {
			return "", ErrRequest
		}
		for _, segment := range strings.Split(raw, "/") {
			if segment == "." || segment == ".." {
				return "", ErrRequest
			}
		}
		if !strings.Contains(raw, "%") {
			return raw, nil
		}
		decoded, err := url.PathUnescape(raw)
		if err != nil {
			return "", ErrRequest
		}
		raw = decoded
	}
	return "", ErrRequest
}

var forbiddenPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("fec0::/10"),
}

func IsPublicIP(ip netip.Addr) bool {
	return (&policy{}).permits(ip)
}

func (p *policy) permits(ip netip.Addr) bool {
	if !ip.IsValid() || ip.Zone() != "" {
		return false
	}
	ip = ip.Unmap()
	if ip == netip.MustParseAddr("168.63.129.16") || ip == netip.MustParseAddr("fd00:ec2::254") {
		return false
	}
	if ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	for _, prefix := range forbiddenPrefixes {
		if prefix.Contains(ip) {
			return false
		}
	}
	if ip.IsLoopback() {
		return p.development && p.route != nil && p.host == "127.0.0.1" && ip == netip.MustParseAddr("127.0.0.1") && p.inCIDRs(ip)
	}
	if !ip.IsGlobalUnicast() {
		return false
	}
	if p.route != nil {
		if p.endpoint.Scheme == "http" && !ip.IsPrivate() {
			return false
		}
		return p.inCIDRs(ip)
	}
	return !ip.IsPrivate()
}

func (p *policy) inCIDRs(ip netip.Addr) bool {
	for _, prefix := range p.cidrs {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}

func (p *policy) dial(lookup lookupFunc, dial dialFunc) dialFunc {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || strings.ToLower(host) != p.host || port != p.port || network != "tcp" {
			return nil, ErrDestination
		}
		var ips []netip.Addr
		if ip, err := netip.ParseAddr(host); err == nil {
			ips = []netip.Addr{ip}
		} else {
			ips, err = lookup(ctx, host)
			if err != nil {
				return nil, ErrDestination
			}
		}
		if len(ips) == 0 {
			return nil, ErrDestination
		}
		for _, ip := range ips {
			if !p.permits(ip) {
				return nil, ErrDestination
			}
		}
		return dialValidated(ctx, ips, port, dial)
	}
}

type dialResult struct {
	connection net.Conn
	err        error
}

func dialValidated(ctx context.Context, ips []netip.Addr, port string, dial dialFunc) (net.Conn, error) {
	dialCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan dialResult)
	for index, ip := range interleaveFamilies(ips) {
		go func(delay time.Duration, address string) {
			if delay > 0 {
				timer := time.NewTimer(delay)
				defer timer.Stop()
				select {
				case <-timer.C:
				case <-dialCtx.Done():
					return
				}
			}
			connection, err := dial(dialCtx, "tcp", net.JoinHostPort(address, port))
			select {
			case results <- dialResult{connection: connection, err: err}:
			case <-dialCtx.Done():
				if connection != nil {
					connection.Close()
				}
			}
		}(time.Duration(index)*250*time.Millisecond, ip.Unmap().String())
	}
	for range ips {
		select {
		case result := <-results:
			if result.err == nil {
				if err := ctx.Err(); err != nil {
					result.connection.Close()
					return nil, err
				}
				cancel()
				return result.connection, nil
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, errors.New("connection destination unavailable")
}

func interleaveFamilies(ips []netip.Addr) []netip.Addr {
	firstIs4 := ips[0].Unmap().Is4()
	first, second := make([]netip.Addr, 0, len(ips)), make([]netip.Addr, 0, len(ips))
	for _, ip := range ips {
		if ip.Unmap().Is4() == firstIs4 {
			first = append(first, ip)
		} else {
			second = append(second, ip)
		}
	}
	ordered := make([]netip.Addr, 0, len(ips))
	for len(first) > 0 || len(second) > 0 {
		if len(first) > 0 {
			ordered = append(ordered, first[0])
			first = first[1:]
		}
		if len(second) > 0 {
			ordered = append(ordered, second[0])
			second = second[1:]
		}
	}
	return ordered
}

func (t *transport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil {
		return nil, ErrRequest
	}
	u := request.URL
	if u.Scheme != t.policy.endpoint.Scheme || !strings.EqualFold(u.Host, t.policy.endpoint.Host) || u.User != nil || u.Fragment != "" || u.RawFragment != "" || u.Opaque != "" || request.Host != "" && !strings.EqualFold(request.Host, t.policy.endpoint.Host) || request.RequestURI != "" {
		return nil, ErrRequest
	}
	path, err := safePath(u.EscapedPath())
	if err != nil {
		return nil, err
	}
	if t.policy.path != "" && path != t.policy.path && !strings.HasPrefix(path, t.policy.path+"/") {
		return nil, ErrRequest
	}
	switch request.Method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD":
	default:
		return nil, ErrRequest
	}
	if request.Header.Get("Proxy-Authorization") != "" || request.Header.Get("Proxy-Connection") != "" || request.Header.Get("Upgrade") != "" {
		return nil, ErrRequest
	}
	return t.transport.RoundTrip(request)
}

func (t *transport) CloseIdleConnections() { t.transport.CloseIdleConnections() }

func WithTimeout(client *http.Client, timeout time.Duration) (*http.Client, error) {
	if client == nil || timeout <= 0 || timeout > 5*time.Minute {
		return nil, ErrRequest
	}
	guarded, ok := client.Transport.(*transport)
	if !ok {
		return nil, ErrRequest
	}
	copyTransport := *guarded
	copyTransport.transport = guarded.transport.Clone()
	copyTransport.transport.ResponseHeaderTimeout = timeout
	copyClient := *client
	copyClient.Transport = &copyTransport
	copyClient.Timeout = timeout
	return &copyClient, nil
}
