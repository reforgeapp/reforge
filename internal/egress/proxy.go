package egress

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"sync"
	"time"

	"reforge/internal/network"
)

var Registries = []string{"registry.npmjs.org", "proxy.golang.org", "sum.golang.org"}

type Proxy struct {
	listener net.Listener
	allow    map[string]bool
	resolve  func(context.Context, string) ([]netip.Addr, error)
	dial     func(context.Context, string, string) (net.Conn, error)
	wait     sync.WaitGroup
}

func Listen(path string, hosts []string) (*Proxy, error) {
	_ = os.Remove(path)
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err = os.Chmod(path, 0666); err != nil {
		listener.Close()
		return nil, err
	}
	p := &Proxy{listener: listener, allow: map[string]bool{}, dial: (&net.Dialer{Timeout: 10 * time.Second}).DialContext, resolve: func(ctx context.Context, host string) ([]netip.Addr, error) {
		return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	}}
	for _, host := range hosts {
		p.allow[host] = true
	}
	p.wait.Add(1)
	go p.serve()
	return p, nil
}

func (p *Proxy) Close() error {
	err := p.listener.Close()
	p.wait.Wait()
	return err
}

func (p *Proxy) serve() {
	defer p.wait.Done()
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			return
		}
		go p.handle(conn)
	}
}

func (p *Proxy) handle(client net.Conn) {
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(15 * time.Second))
	reader := bufio.NewReader(client)
	request, err := http.ReadRequest(reader)
	if err != nil {
		return
	}
	upstream, err := p.connect(request)
	if err != nil {
		_, _ = io.WriteString(client, "HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\n\r\n")
		return
	}
	defer upstream.Close()
	if _, err = io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	_ = client.SetDeadline(time.Time{})
	done := make(chan struct{}, 2)
	relay := func(dst net.Conn, src io.Reader) {
		_, _ = io.Copy(dst, src)
		done <- struct{}{}
	}
	go relay(upstream, reader)
	go relay(client, upstream)
	<-done
}

func (p *Proxy) connect(request *http.Request) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return p.connectContext(ctx, request)
}

func (p *Proxy) connectContext(ctx context.Context, request *http.Request) (net.Conn, error) {
	host, port, err := net.SplitHostPort(request.Host)
	if request.Method != http.MethodConnect || err != nil || port != "443" || !p.allow[host] {
		return nil, errors.New("destination not allowed")
	}
	addresses, err := p.resolve(ctx, host)
	if err != nil || len(addresses) == 0 {
		return nil, errors.New("destination did not resolve")
	}
	for _, address := range addresses {
		if !network.IsPublicIP(address.Unmap()) {
			return nil, errors.New("destination resolved to a non-public address")
		}
	}
	return p.dial(ctx, "tcp", netip.AddrPortFrom(addresses[0].Unmap(), 443).String())
}
