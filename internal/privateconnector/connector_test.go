package privateconnector

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
	"github.com/reforgeapp/reforge/internal/network"
	"github.com/reforgeapp/reforge/internal/runner"
)

type testFixture struct {
	connector  *Connector
	target     Target
	identity   runner.Runner
	secret     string
	credential string
	revoked    atomic.Bool
}

func fixture(t *testing.T) *testFixture {
	t.Helper()
	f := &testFixture{target: Target{OrgID: domain.NewID(), RunnerID: domain.NewID()}, secret: "fixture-provider-secret-0123456789", credential: "fixture-supervisor-secret-0123456789"}
	f.identity = runner.Runner{ID: f.target.RunnerID, OrgID: f.target.OrgID, PoolID: domain.NewID(), State: "active", Version: 1, CredentialExpiresAt: time.Now().Add(time.Hour)}
	var e error
	f.connector, e = New(Config{TTL: time.Second, Development: true, Authenticate: func(ctx context.Context, raw string) (runner.Runner, error) {
		if raw != f.credential || f.revoked.Load() {
			return runner.Runner{}, auth.ErrUnauthenticated
		}
		return f.identity, nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(f.connector.Close)
	return f
}
func (f *testFixture) spec(id string) GrantSpec {
	connectionID := domain.NewID()
	h := sha256.Sum256([]byte(f.credential))
	return GrantSpec{RunnerVersion: f.identity.Version, CredentialHash: hex.EncodeToString(h[:]), OperationID: id, AuthorityID: domain.NewID(), Connection: Connection{ID: connectionID, OrgID: f.target.OrgID, Provider: "gitea", Endpoint: "http://127.0.0.1:53000", Version: 1, CredentialVersion: 1, Secret: f.secret, Route: network.PrivateRoute{OrgID: f.target.OrgID, ConnectionID: connectionID, RunnerID: f.target.RunnerID, Host: "127.0.0.1", CIDRs: []string{"127.0.0.1/32"}}}}
}
func TestReadyBeforeAuthorizationAndOneUseResult(t *testing.T) {
	f := fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	op := Operation{ID: domain.NewID(), Kind: GiteaProbe}
	authorized := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		_, e := f.connector.Dispatch(ctx, f.target, op, func(ctx context.Context, r Ready, deliver Deliver) error {
			if r.ID != f.identity.ID {
				return auth.ErrForbidden
			}
			close(authorized)
			_, e := deliver(f.spec(op.ID))
			return e
		})
		finished <- e
	}()
	select {
	case <-authorized:
		t.Fatal("authorization began before readiness")
	case <-time.After(30 * time.Millisecond):
	}
	grant, e := f.connector.Poll(ctx, f.credential)
	if e != nil {
		t.Fatal(e)
	}
	select {
	case <-authorized:
	default:
		t.Fatal("grant preceded authorization")
	}
	response := Result{OperationID: op.ID, Capabilities: &forge.Capabilities{Provider: "gitea"}}
	if e = f.connector.Complete(ctx, "wrong-supervisor", grant.ID, grant.ResultCapability, response); !errors.Is(e, auth.ErrUnauthenticated) {
		t.Fatal("wrong supervisor accepted")
	}
	if e = f.connector.Complete(ctx, f.credential, grant.ID, "wrong-capability", response); !errors.Is(e, auth.ErrUnauthenticated) {
		t.Fatal("wrong capability accepted")
	}
	if e = f.connector.Complete(ctx, f.credential, grant.ID, grant.ResultCapability, response); e != nil {
		t.Fatal(e)
	}
	if e = f.connector.Complete(ctx, f.credential, grant.ID, grant.ResultCapability, response); e == nil {
		t.Fatal("result replay accepted")
	}
	if e = <-finished; e != nil {
		t.Fatal(e)
	}
	if _, e = f.connector.Dispatch(ctx, f.target, op, func(context.Context, Ready, Deliver) error { return nil }); !errors.Is(e, ErrConflict) {
		t.Fatal("operation replay accepted")
	}
}

func TestKnownReadFailurePreservesBackoffButCommitLossIsUncertain(t *testing.T) {
	for _, commitLost := range []bool{false, true} {
		t.Run(fmt.Sprint(commitLost), func(t *testing.T) {
			f := fixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			op := Operation{ID: domain.NewID(), Kind: ForgeProbe}
			finished := make(chan error, 1)
			go func() {
				_, err := f.connector.Dispatch(ctx, f.target, op, func(_ context.Context, _ Ready, deliver Deliver) error {
					_, err := deliver(f.spec(op.ID))
					if commitLost {
						return errors.New("transaction outcome lost")
					}
					return err
				})
				finished <- err
			}()
			grant, err := f.connector.Poll(ctx, f.credential)
			if err != nil {
				t.Fatal(err)
			}
			if err = f.connector.Complete(ctx, f.credential, grant.ID, grant.ResultCapability, Result{OperationID: op.ID, Failure: &Failure{Code: "rate_limit", RetryAfterMS: 17000}}); err != nil {
				t.Fatal(err)
			}
			err = <-finished
			var provider *domain.ProviderError
			if commitLost {
				if !errors.Is(err, ErrUncertain) {
					t.Fatalf("commit loss: %v", err)
				}
			} else if !errors.As(err, &provider) || provider.Kind != "rate_limit" || provider.RetryAfter != 17*time.Second {
				t.Fatalf("provider failure/backoff lost: %v", err)
			}
		})
	}
}
func TestRevocationAfterReadinessPreventsCredentialDelivery(t *testing.T) {
	f := fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	finished := make(chan error, 1)
	op := Operation{ID: domain.NewID(), Kind: GiteaProbe}
	originalAuth := f.connector.auth
	var calls atomic.Int64
	f.connector.auth = func(ctx context.Context, raw string) (runner.Runner, error) {
		if calls.Add(1) == 2 {
			f.revoked.Store(true)
		}
		return originalAuth(ctx, raw)
	}
	go func() {
		_, e := f.connector.Dispatch(ctx, f.target, op, func(ctx context.Context, r Ready, deliver Deliver) error {
			t.Error("authorization started with revoked readiness")
			return nil
		})
		finished <- e
	}()
	grant, e := f.connector.Poll(ctx, f.credential)
	if e == nil || grant.Connection.Secret != "" {
		t.Fatal("revoked runner received credentials")
	}
	if e = <-finished; !errors.Is(e, auth.ErrUnauthenticated) {
		t.Fatalf("revocation not enforced: %v", e)
	}
}
func TestTimeoutAndServerLossAreUncertainWithoutReplay(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		t.Run(fmt.Sprint(shutdown), func(t *testing.T) {
			f := fixture(t)
			ctx := context.Background()
			op := Operation{ID: domain.NewID(), Kind: GiteaProbe}
			finished := make(chan error, 1)
			go func() {
				_, e := f.connector.Dispatch(ctx, f.target, op, func(ctx context.Context, r Ready, deliver Deliver) error {
					_, e := deliver(f.spec(op.ID))
					return e
				})
				finished <- e
			}()
			grant, e := f.connector.Poll(ctx, f.credential)
			if e != nil {
				t.Fatal(e)
			}
			if shutdown {
				f.connector.Close()
			}
			if e = <-finished; !errors.Is(e, ErrUncertain) {
				t.Fatalf("lost result not uncertain: %v", e)
			}
			if e = f.connector.Complete(ctx, f.credential, grant.ID, grant.ResultCapability, Result{OperationID: op.ID, Capabilities: &forge.Capabilities{}}); e == nil {
				t.Fatal("expired/lost grant completed")
			}
			if !shutdown {
				if _, e = f.connector.Dispatch(ctx, f.target, op, func(context.Context, Ready, Deliver) error { return nil }); !errors.Is(e, ErrConflict) {
					t.Fatal("timed-out operation replayed")
				}
			}
		})
	}
}
func TestOperationAndCredentialSerializationBoundaries(t *testing.T) {
	f := fixture(t)
	for _, kind := range []Kind{"http.request", "gitea.merge", "model.infer"} {
		if _, e := f.connector.Dispatch(context.Background(), f.target, Operation{ID: domain.NewID(), Kind: kind}, func(context.Context, Ready, Deliver) error {
			t.Fatal("unsupported operation authorized")
			return nil
		}); !errors.Is(e, ErrUnsupported) {
			t.Fatalf("unknown kind accepted: %v", e)
		}
	}
	spec := f.spec(domain.NewID())
	grant := Grant{ID: domain.NewID(), Connection: spec.Connection, ResultCapability: "capability-secret"}
	cfg := ClientConfig{Credential: f.credential}
	for _, v := range []any{spec, spec.Connection, grant, cfg} {
		for _, format := range []string{"%v", "%+v", "%#v"} {
			text := fmt.Sprintf(format, v)
			if strings.Contains(text, f.secret) || strings.Contains(text, f.credential) || strings.Contains(text, "capability-secret") {
				t.Fatal("format leaked credentials")
			}
		}
		raw, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		if strings.Contains(string(raw), f.secret) || strings.Contains(string(raw), f.credential) || strings.Contains(string(raw), "capability-secret") {
			t.Fatal("ordinary JSON leaked credentials")
		}
	}
	wire, e := grant.MarshalWire()
	if e != nil || !strings.Contains(string(wire), f.secret) {
		t.Fatal("explicit authenticated wire omitted credential")
	}
	if _, e = DecodeGrant([]byte(`{"id":"x","method":"GET","url":"http://169.254.169.254"}`)); !errors.Is(e, ErrInvalid) {
		t.Fatal("arbitrary proxy fields accepted")
	}
	if _, e = NewClient(ClientConfig{Endpoint: "http://10.0.0.1", Credential: f.credential, Target: f.target, Development: true}); e == nil {
		t.Fatal("development allowed non-loopback plaintext controller")
	}
}

func TestCrossTenantConnectionAndExtraArgumentsDenied(t *testing.T) {
	f := fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	op := Operation{ID: domain.NewID(), Kind: GiteaProbe}
	finished := make(chan error, 1)
	go func() {
		_, e := f.connector.Dispatch(ctx, f.target, op, func(ctx context.Context, r Ready, deliver Deliver) error {
			spec := f.spec(op.ID)
			spec.Connection.OrgID = domain.NewID()
			_, e := deliver(spec)
			return e
		})
		finished <- e
	}()
	if grant, e := f.connector.Poll(ctx, f.credential); e == nil || grant.Connection.Secret != "" {
		t.Fatal("cross-tenant connection delivered")
	}
	if e := <-finished; !errors.Is(e, ErrInvalid) {
		t.Fatalf("cross-tenant binding accepted: %v", e)
	}
	if e := (Operation{ID: domain.NewID(), Kind: GiteaProbe, Inventory: &InventoryArgs{Limit: 100}}).validate(); !errors.Is(e, ErrInvalid) {
		t.Fatal("extra provider arguments accepted")
	}
}

func TestAuthorizationOrderingAndRunnerProof(t *testing.T) {
	for _, stale := range []bool{false, true} {
		t.Run(fmt.Sprint(stale), func(t *testing.T) {
			f := fixture(t)
			var calls atomic.Int64
			var authorizing atomic.Bool
			original := f.connector.auth
			f.connector.auth = func(ctx context.Context, raw string) (runner.Runner, error) {
				if authorizing.Load() {
					t.Error("authentication acquired resources inside authorization")
				}
				calls.Add(1)
				return original(ctx, raw)
			}
			op := Operation{ID: domain.NewID(), Kind: GiteaProbe}
			done := make(chan error, 1)
			go func() {
				_, err := f.connector.Dispatch(context.Background(), f.target, op, func(ctx context.Context, ready Ready, deliver Deliver) error {
					if calls.Load() != 2 {
						t.Error("snapshot was not refreshed before authorization")
					}
					authorizing.Store(true)
					defer authorizing.Store(false)
					spec := f.spec(op.ID)
					if stale {
						spec.RunnerVersion++
					}
					_, err := deliver(spec)
					return err
				})
				done <- err
			}()
			grant, err := f.connector.Poll(context.Background(), f.credential)
			if stale {
				if err == nil || grant.Connection.Secret != "" {
					t.Fatal("stale runner proof delivered")
				}
				if !errors.Is(<-done, auth.ErrUnauthenticated) {
					t.Fatal("stale proof not rejected")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = f.connector.Complete(context.Background(), f.credential, grant.ID, grant.ResultCapability, Result{OperationID: op.ID, Capabilities: &forge.Capabilities{}}); err != nil {
				t.Fatal(err)
			}
			if err = <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestReplayWindowExpiresAcrossMoreThanLifetimeLimit(t *testing.T) {
	f := fixture(t)
	now := time.Now()
	f.connector.now = func() time.Time { return now }
	ctx := context.Background()
	for i := 0; i < 10001; i++ {
		if i%1000 == 0 {
			now = now.Add(6 * time.Minute)
		}
		op := Operation{ID: domain.NewID(), Kind: GiteaProbe}
		done := make(chan error, 1)
		go func() {
			_, err := f.connector.Dispatch(ctx, f.target, op, func(ctx context.Context, ready Ready, deliver Deliver) error {
				_, err := deliver(f.spec(op.ID))
				return err
			})
			done <- err
		}()
		grant, err := f.connector.Poll(ctx, f.credential)
		if err != nil {
			t.Fatalf("operation %d: %v", i, err)
		}
		if err = f.connector.Complete(ctx, f.credential, grant.ID, grant.ResultCapability, Result{OperationID: op.ID, Capabilities: &forge.Capabilities{}}); err != nil {
			t.Fatal(err)
		}
		if err = <-done; err != nil {
			t.Fatalf("operation %d: %v", i, err)
		}
		if _, err = f.connector.Dispatch(ctx, f.target, op, func(context.Context, Ready, Deliver) error { return nil }); !errors.Is(err, ErrConflict) {
			t.Fatal("live tombstone replay accepted")
		}
	}
	if len(f.connector.used) > 1000 {
		t.Fatal("expired tombstones retained")
	}
}

func TestGrantLivenessEndsOnCancellationAndRejectsOtherCapability(t *testing.T) {
	f := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	op := Operation{ID: domain.NewID(), Kind: GiteaProbe}
	done := make(chan error, 1)
	go func() {
		_, err := f.connector.Dispatch(ctx, f.target, op, func(ctx context.Context, r Ready, deliver Deliver) error {
			_, err := deliver(f.spec(op.ID))
			return err
		})
		done <- err
	}()
	grant, err := f.connector.Poll(context.Background(), f.credential)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.connector.Active(context.Background(), f.credential, grant.ID, grant.ResultCapability); err != nil {
		t.Fatal(err)
	}
	if err = f.connector.Active(context.Background(), f.credential, grant.ID, "other"); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("wrong capability observed grant")
	}
	cancel()
	<-done
	if err = f.connector.Active(context.Background(), f.credential, grant.ID, grant.ResultCapability); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("cancelled grant still active")
	}
}
