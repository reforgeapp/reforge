package privateconnector

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/runner"
)

type routeTable struct {
	mu   sync.Mutex
	rows map[string]map[string]bool
	pods []*Connector
}

type podRoutes struct {
	table *routeTable
	self  string
}

func (r podRoutes) Claim(_ context.Context, _, route string, _ time.Time) error {
	r.table.mu.Lock()
	if r.table.rows[route] == nil {
		r.table.rows[route] = map[string]bool{}
	}
	r.table.rows[route][r.self] = true
	pods := append([]*Connector(nil), r.table.pods...)
	r.table.mu.Unlock()
	for _, pod := range pods {
		pod.Notify(route)
	}
	return nil
}

func (r podRoutes) Release(_, route string) {
	r.table.mu.Lock()
	defer r.table.mu.Unlock()
	delete(r.table.rows[route], r.self)
}

func (r podRoutes) Elsewhere(_ context.Context, _, route string) (string, error) {
	r.table.mu.Lock()
	defer r.table.mu.Unlock()
	for pod := range r.table.rows[route] {
		if pod != r.self {
			return pod, nil
		}
	}
	return "", nil
}

func TestRoutesMoveRunnerToDispatchingInstance(t *testing.T) {
	f := fixture(t)
	table := &routeTable{rows: map[string]map[string]bool{}}
	pod := func(self string) *Connector {
		c, err := New(Config{TTL: 2 * time.Second, Development: true, Routes: podRoutes{table: table, self: self}, Authenticate: func(_ context.Context, raw string) (runner.Runner, error) {
			if raw != f.credential {
				return runner.Runner{}, auth.ErrUnauthenticated
			}
			return f.identity, nil
		}})
		if err != nil {
			t.Fatal(err)
		}
		table.mu.Lock()
		table.pods = append(table.pods, c)
		table.mu.Unlock()
		t.Cleanup(c.Close)
		return c
	}
	a, b := pod("a:8080"), pod("b:8080")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	polled := make(chan error, 1)
	go func() {
		_, err := b.Poll(ctx, f.credential)
		polled <- err
	}()
	time.Sleep(30 * time.Millisecond)
	op := Operation{ID: domain.NewID(), Kind: GiteaProbe}
	finished := make(chan error, 1)
	go func() {
		_, err := a.Dispatch(ctx, f.target, op, func(_ context.Context, _ Ready, deliver Deliver) error {
			_, err := deliver(f.spec(op.ID))
			return err
		})
		finished <- err
	}()
	var elsewhere Elsewhere
	if err := <-polled; !errors.As(err, &elsewhere) || elsewhere.Address != "a:8080" {
		t.Fatalf("poll on idle instance was not redirected: %v", err)
	}
	grant, err := a.Poll(WithForwarded(ctx), f.credential)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Holds(grant.ID) || b.Holds(grant.ID) {
		t.Fatal("grant held by the wrong instance")
	}
	if owner, _ := b.GrantOwner(ctx, f.credential, grant.ID); owner != "a:8080" {
		t.Fatalf("grant owner = %q", owner)
	}
	if err = a.Complete(ctx, f.credential, grant.ID, grant.ResultCapability, Result{OperationID: op.ID, Capabilities: &forge.Capabilities{Provider: "gitea"}}); err != nil {
		t.Fatal(err)
	}
	if err = <-finished; err != nil {
		t.Fatal(err)
	}
	if owner, _ := b.GrantOwner(ctx, f.credential, grant.ID); owner != "" {
		t.Fatal("grant route not released")
	}
	if owner, _ := b.routes.Elsewhere(ctx, f.target.OrgID, "wait:"+f.target.RunnerID); owner != "" {
		t.Fatal("wait route not released")
	}
}
