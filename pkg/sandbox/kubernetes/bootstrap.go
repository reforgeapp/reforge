package kubernetes

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"time"

	"github.com/reforgeapp/reforge/pkg/egress"
	"github.com/reforgeapp/reforge/pkg/sandbox"
	"github.com/reforgeapp/reforge/pkg/sandbox/guest"
)

var ErrEgressBootstrap = errors.New("registry egress bootstrap unavailable")

type registryBootstrapper struct {
	broker  *egress.Broker
	address string
}

type registryLease struct {
	mu     sync.Mutex
	broker *egress.Broker
	refs   BootstrapRefs
	config guest.TCPEgressRequest
	ended  bool
}

func NewRegistryBootstrapper(broker *egress.Broker, advertisedAddr string) (Bootstrapper, error) {
	address, err := netip.ParseAddrPort(advertisedAddr)
	if broker == nil || err != nil || !address.Addr().IsValid() || address.Addr().IsUnspecified() || address.Addr().Zone() != "" || address.Port() == 0 {
		return nil, ErrEgressBootstrap
	}
	return &registryBootstrapper{broker: broker, address: advertisedAddr}, nil
}

func (b *registryBootstrapper) Prepare(_ context.Context, request sandbox.WorkspaceRequest, _ string) (BootstrapLease, error) {
	if request.Dependencies != "" || request.Egress != "registry" || request.Timeout <= 0 {
		return nil, ErrEgressBootstrap
	}
	lifetime := request.Timeout
	if lifetime > 10*time.Minute {
		lifetime = 10 * time.Minute
	}
	token, err := b.broker.NewSession(lifetime)
	if err != nil {
		return nil, err
	}
	return &registryLease{
		broker: b.broker,
		refs:   BootstrapRefs{Egress: "registry"},
		config: guest.TCPEgressRequest{Address: b.address, Token: token},
	}, nil
}

func (l *registryLease) Refs() BootstrapRefs {
	return l.refs
}

func (l *registryLease) EgressConfig() guest.TCPEgressRequest {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.ended {
		return guest.TCPEgressRequest{}
	}
	return l.config
}

func (l *registryLease) EndEgress(context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.ended {
		return nil
	}
	l.broker.Revoke(l.config.Token)
	l.config.Token = ""
	l.ended = true
	return nil
}

func (l *registryLease) Close(ctx context.Context) error {
	return l.EndEgress(ctx)
}

func (b *registryBootstrapper) Close() error {
	return b.broker.Close()
}
