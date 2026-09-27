package egress

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxSessionLifetime = 10 * time.Minute
const maxBrokerSessions = 1024
const maxBrokerConnections = 2048
const maxBrokerHeaderBytes = 16 << 10

var ErrBrokerClosed = errors.New("egress broker closed")
var ErrSessionLifetime = errors.New("egress session lifetime must be positive and at most ten minutes")

type brokerSession struct {
	expires time.Time
	revoked bool
	active  map[net.Conn]struct{}
	cancel  map[net.Conn]context.CancelFunc
}

type Broker struct {
	listener net.Listener
	proxy    *Proxy
	mu       sync.Mutex
	sessions map[[32]byte]*brokerSession
	active   map[net.Conn]struct{}
	closed   bool
	wait     sync.WaitGroup
}

func ListenTCP(bind string, hosts []string) (*Broker, error) {
	if !fixedTCPBind(bind) {
		return nil, errors.New("egress broker bind must be fixed IP:port")
	}
	listener, err := net.Listen("tcp", bind)
	if err != nil {
		return nil, err
	}
	allow := make(map[string]bool, len(hosts))
	for _, host := range hosts {
		allow[host] = true
	}
	broker := &Broker{
		listener: listener,
		proxy: &Proxy{allow: allow, dial: (&net.Dialer{Timeout: 10 * time.Second}).DialContext, resolve: func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		}},
		sessions: map[[32]byte]*brokerSession{},
		active:   map[net.Conn]struct{}{},
	}
	broker.wait.Add(1)
	go broker.serve()
	return broker, nil
}

func fixedTCPBind(bind string) bool {
	host, port, err := net.SplitHostPort(bind)
	if err != nil {
		return false
	}
	address, err := netip.ParseAddr(host)
	if err != nil || address.Zone() != "" {
		return false
	}
	number, err := strconv.Atoi(port)
	return err == nil && number > 0 && number <= 65535
}

func (b *Broker) NewSession(lifetime time.Duration) (string, error) {
	if lifetime <= 0 || lifetime > maxSessionLifetime {
		return "", ErrSessionLifetime
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(secret[:])
	hash := sha256.Sum256([]byte(token))
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return "", ErrBrokerClosed
	}
	b.expireLocked(time.Now())
	if len(b.sessions) >= maxBrokerSessions {
		return "", errors.New("egress broker session capacity reached")
	}
	b.sessions[hash] = &brokerSession{expires: time.Now().Add(lifetime), active: map[net.Conn]struct{}{}, cancel: map[net.Conn]context.CancelFunc{}}
	return token, nil
}

func (b *Broker) Revoke(token string) bool {
	hash := sha256.Sum256([]byte(token))
	b.mu.Lock()
	defer b.mu.Unlock()
	session, ok := b.sessions[hash]
	if !ok {
		return false
	}
	delete(b.sessions, hash)
	b.revokeLocked(session)
	return true
}

func (b *Broker) expireLocked(now time.Time) {
	for hash, session := range b.sessions {
		if !now.Before(session.expires) {
			delete(b.sessions, hash)
			b.revokeLocked(session)
		}
	}
}

func (b *Broker) revokeLocked(session *brokerSession) {
	if session.revoked {
		return
	}
	session.revoked = true
	for _, cancel := range session.cancel {
		cancel()
	}
	for conn := range session.active {
		_ = conn.Close()
		delete(b.active, conn)
	}
	clear(session.cancel)
	clear(session.active)
}

func (b *Broker) Close() error {
	err := b.listener.Close()
	b.mu.Lock()
	if !b.closed {
		b.closed = true
		for _, session := range b.sessions {
			b.revokeLocked(session)
		}
		clear(b.sessions)
		for conn := range b.active {
			_ = conn.Close()
			delete(b.active, conn)
		}
	}
	b.mu.Unlock()
	b.wait.Wait()
	return err
}

func (b *Broker) serve() {
	defer b.wait.Done()
	for {
		client, err := b.listener.Accept()
		if err != nil {
			return
		}
		b.mu.Lock()
		if b.closed || len(b.active) >= maxBrokerConnections {
			b.mu.Unlock()
			_ = client.Close()
			continue
		}
		b.active[client] = struct{}{}
		b.wait.Add(1)
		b.mu.Unlock()
		go func() {
			defer b.wait.Done()
			b.handle(client)
		}()
	}
}

func (b *Broker) handle(client net.Conn) {
	defer b.untrack(client)
	_ = client.SetDeadline(time.Now().Add(15 * time.Second))
	limited := &io.LimitedReader{R: client, N: maxBrokerHeaderBytes + 1}
	reader := bufio.NewReader(limited)
	request, err := http.ReadRequest(reader)
	if err != nil || limited.N == 0 {
		return
	}
	limited.N = int64(^uint64(0) >> 1)
	token, ok := bearerToken(request.Header.Values("Proxy-Authorization"))
	if !ok {
		writeProxyResponse(client, http.StatusProxyAuthRequired)
		return
	}
	session, ctx, cancel, ok := b.attachClient(token, client)
	if !ok {
		writeProxyResponse(client, http.StatusProxyAuthRequired)
		return
	}
	defer cancel()
	upstream, err := b.proxy.connectContext(ctx, request)
	if err != nil {
		if ctx.Err() == nil {
			writeProxyResponse(client, http.StatusForbidden)
		}
		return
	}
	if !b.attachUpstream(session, upstream) {
		_ = upstream.Close()
		return
	}
	defer b.untrackSession(session, upstream)
	if err = client.SetDeadline(time.Time{}); err != nil {
		return
	}
	if _, err = io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	done := make(chan struct{}, 2)
	relay := func(dst net.Conn, src io.Reader) {
		_, _ = io.Copy(dst, src)
		done <- struct{}{}
	}
	go relay(upstream, reader)
	go relay(client, upstream)
	select {
	case <-ctx.Done():
	case <-done:
	}
	_ = client.Close()
	_ = upstream.Close()
}

func (b *Broker) attachClient(token string, client net.Conn) (*brokerSession, context.Context, context.CancelFunc, bool) {
	hash := sha256.Sum256([]byte(token))
	b.mu.Lock()
	defer b.mu.Unlock()
	session, ok := b.sessions[hash]
	if !ok || b.closed || session.revoked || !time.Now().Before(session.expires) {
		if ok {
			delete(b.sessions, hash)
			b.revokeLocked(session)
		}
		return nil, nil, nil, false
	}
	ctx, cancel := context.WithDeadline(context.Background(), session.expires)
	session.active[client] = struct{}{}
	session.cancel[client] = cancel
	return session, ctx, cancel, true
}

func (b *Broker) attachUpstream(session *brokerSession, upstream net.Conn) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || session.revoked || !time.Now().Before(session.expires) {
		return false
	}
	session.active[upstream] = struct{}{}
	b.active[upstream] = struct{}{}
	return true
}

func (b *Broker) untrack(conn net.Conn) {
	b.mu.Lock()
	delete(b.active, conn)
	for _, session := range b.sessions {
		delete(session.active, conn)
		delete(session.cancel, conn)
	}
	b.mu.Unlock()
	_ = conn.Close()
}

func (b *Broker) untrackSession(session *brokerSession, conn net.Conn) {
	b.mu.Lock()
	delete(b.active, conn)
	delete(session.active, conn)
	b.mu.Unlock()
	_ = conn.Close()
}

func bearerToken(values []string) (string, bool) {
	if len(values) != 1 {
		return "", false
	}
	parts := strings.Fields(values[0])
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || len(parts[1]) > 128 {
		return "", false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(decoded) != 32 {
		return "", false
	}
	return parts[1], true
}

func writeProxyResponse(conn net.Conn, status int) {
	statusText := http.StatusText(status)
	_, _ = io.WriteString(conn, "HTTP/1.1 "+strconv.Itoa(status)+" "+statusText+"\r\nContent-Length: 0\r\n\r\n")
}
