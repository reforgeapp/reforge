package kubernetes

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/internal/egress"
	"github.com/reforgeapp/reforge/internal/sandbox"
)

func TestRegistryBootstrapLeaseKeepsTokenOutOfRefsAndRevokes(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := probe.Addr().String()
	_ = probe.Close()
	broker, err := egress.ListenTCP(address, egress.Registries)
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	bootstrapper, err := NewRegistryBootstrapper(broker, address)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := bootstrapper.Prepare(context.Background(), sandbox.WorkspaceRequest{Egress: "registry", Timeout: 20 * time.Minute}, "rf-ws-test")
	if err != nil {
		t.Fatal(err)
	}
	if got := lease.Refs(); got != (BootstrapRefs{Egress: "registry"}) {
		t.Fatalf("refs=%+v", got)
	}
	config := lease.(*registryLease).EgressConfig()
	if config.Address != address || config.Token == "" {
		t.Fatalf("config missing broker credentials: address=%q token present=%t", config.Address, config.Token != "")
	}
	if err = lease.EndEgress(context.Background()); err != nil {
		t.Fatal(err)
	}
	if lease.(*registryLease).EgressConfig().Token != "" {
		t.Fatal("ended lease still exposes egress token")
	}
	if broker.Revoke(config.Token) {
		t.Fatal("broker session remained active after lease revoke")
	}
	if err = lease.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryBootstrapRejectsHostPathsAndDNSAddress(t *testing.T) {
	if _, err := NewRegistryBootstrapper(nil, "runner-proxy:8443"); err == nil {
		t.Fatal("DNS broker address accepted")
	}
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := probe.Addr().String()
	_ = probe.Close()
	broker, err := egress.ListenTCP(address, egress.Registries)
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	bootstrapper, err := NewRegistryBootstrapper(broker, address)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = bootstrapper.Prepare(context.Background(), sandbox.WorkspaceRequest{Dependencies: "/host/deps", Egress: "registry", Timeout: time.Minute}, "rf-ws-test"); err == nil {
		t.Fatal("host dependency path accepted")
	}
}
