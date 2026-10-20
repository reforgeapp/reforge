package privateconnector

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/runner"
)

type Config struct {
	Authenticate  Authenticate
	TTL           time.Duration
	MaxConcurrent int
	Development   bool
}
type Connector struct {
	auth        Authenticate
	development bool
	ttl         time.Duration
	limit       int
	mu          sync.Mutex
	ready       map[string]*readiness
	active      map[string]bool
	pending     map[string]*pending
	used        map[string]time.Time
	now         func() time.Time
	changed     chan struct{}
	closed      chan struct{}
	stopped     bool
}
type readiness struct {
	identity   runner.Runner
	credential string
	grants     chan Grant
	ctx        context.Context
}
type pending struct {
	check          func(context.Context) error
	grant          Grant
	credentialHash [32]byte
	capabilityHash [32]byte
	result         chan Result
	consumed       bool
}

func New(cfg Config) (*Connector, error) {
	if cfg.Authenticate == nil {
		return nil, ErrInvalid
	}
	if cfg.TTL == 0 {
		cfg.TTL = DefaultTTL
	}
	if cfg.TTL < time.Second || cfg.TTL > MaxTTL {
		return nil, ErrInvalid
	}
	if cfg.MaxConcurrent == 0 {
		cfg.MaxConcurrent = 8
	}
	if cfg.MaxConcurrent < 1 || cfg.MaxConcurrent > 1024 {
		return nil, ErrInvalid
	}
	return &Connector{auth: cfg.Authenticate, development: cfg.Development, ttl: cfg.TTL, limit: cfg.MaxConcurrent, ready: map[string]*readiness{}, active: map[string]bool{}, pending: map[string]*pending{}, used: map[string]time.Time{}, now: time.Now, changed: make(chan struct{}), closed: make(chan struct{})}, nil
}
func (c *Connector) pruneUsed() {
	now := c.now()
	for key, expiry := range c.used {
		if !expiry.After(now) {
			delete(c.used, key)
		}
	}
}
func targetKey(t Target) string { return t.OrgID + "/" + t.RunnerID }

const perOrgLimit = 16

func (c *Connector) orgSlots(org string) int {
	n := 0
	for key := range c.ready {
		if strings.HasPrefix(key, org+"/") {
			n++
		}
	}
	for key := range c.active {
		if strings.HasPrefix(key, org+"/") {
			n++
		}
	}
	return n
}
func (c *Connector) signal() { close(c.changed); c.changed = make(chan struct{}) }
func (c *Connector) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.stopped {
		c.stopped = true
		close(c.closed)
		c.ready = map[string]*readiness{}
		c.signal()
	}
}
func (c *Connector) Poll(ctx context.Context, credential string) (Grant, error) {
	if len(credential) > 256 {
		return Grant{}, auth.ErrUnauthenticated
	}
	identity, e := c.auth(ctx, credential)
	if e != nil {
		return Grant{}, e
	}
	if !auth.ValidID(identity.OrgID) || !auth.ValidID(identity.ID) || identity.State != "active" {
		return Grant{}, auth.ErrUnauthenticated
	}
	ctx, cancel := context.WithTimeout(ctx, c.ttl)
	defer cancel()
	ready := &readiness{identity: identity, credential: credential, grants: make(chan Grant), ctx: ctx}
	key := targetKey(Target{OrgID: identity.OrgID, RunnerID: identity.ID})
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return Grant{}, ErrUnavailable
	}
	if c.ready[key] != nil || c.active[key] {
		c.mu.Unlock()
		return Grant{}, ErrConflict
	}
	if len(c.ready)+len(c.active) >= c.limit || c.orgSlots(identity.OrgID) >= perOrgLimit {
		c.mu.Unlock()
		return Grant{}, ErrUnavailable
	}
	c.ready[key] = ready
	c.signal()
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		if c.ready[key] == ready {
			delete(c.ready, key)
			c.signal()
		}
		c.mu.Unlock()
	}()
	select {
	case grant := <-ready.grants:
		return grant, nil
	case <-ctx.Done():
		return Grant{}, ErrUnavailable
	case <-c.closed:
		return Grant{}, ErrUnavailable
	}
}
func (c *Connector) Dispatch(ctx context.Context, target Target, operation Operation, authorize Authorize) (Result, error) {
	if !auth.ValidID(target.OrgID) || !auth.ValidID(target.RunnerID) || authorize == nil {
		return Result{}, ErrInvalid
	}
	if e := operation.validate(); e != nil {
		return Result{}, e
	}
	raw, _ := json.Marshal(operation)
	var copyOperation Operation
	if e := json.Unmarshal(raw, &copyOperation); e != nil {
		return Result{}, ErrInvalid
	}
	operation = copyOperation
	wait, cancelWait := context.WithTimeout(ctx, c.ttl)
	defer cancelWait()
	key := targetKey(target)
	opKey := target.OrgID + "/" + operation.ID
	var ready *readiness
	for ready == nil {
		c.mu.Lock()
		if c.stopped {
			c.mu.Unlock()
			return Result{}, ErrUnavailable
		}
		c.pruneUsed()
		if _, exists := c.used[opKey]; exists {
			c.mu.Unlock()
			return Result{}, ErrConflict
		}
		if r := c.ready[key]; r != nil && !c.active[key] {
			ready = r
			delete(c.ready, key)
			c.active[key] = true
			c.signal()
		}
		changed := c.changed
		c.mu.Unlock()
		if ready != nil {
			break
		}
		select {
		case <-wait.Done():
			return Result{}, ErrUnavailable
		case <-c.closed:
			return Result{}, ErrUnavailable
		case <-changed:
		}
	}
	defer func() { c.mu.Lock(); delete(c.active, key); c.signal(); c.mu.Unlock() }()
	operationTTL := operation.ttl(c.ttl)
	authctx, cancel := context.WithTimeout(ctx, operationTTL)
	defer cancel()
	fresh, e := c.auth(authctx, ready.credential)
	if e != nil {
		return Result{}, e
	}
	if fresh.ID != ready.identity.ID || fresh.OrgID != ready.identity.OrgID || fresh.PoolID != ready.identity.PoolID || fresh.Version != ready.identity.Version || fresh.State != "active" || fresh.Version < 1 || !fresh.CredentialExpiresAt.After(time.Now()) {
		return Result{}, auth.ErrUnauthenticated
	}
	credentialHash := sha256.Sum256([]byte(ready.credential))
	subject := Ready{Runner: fresh, CredentialHash: hex.EncodeToString(credentialHash[:])}
	var result Result
	var deliveryErr error
	var once sync.Once
	called := false
	attempted := false
	deliver := func(spec GrantSpec) (Result, error) {
		if operation.Mutation() && spec.Check == nil {
			return Result{}, ErrInvalid
		}
		didCall := false
		once.Do(func() {
			didCall = true
			called = true
			if spec.OperationID != operation.ID || !auth.ValidID(spec.AuthorityID) || validateConnection(spec.Connection, target, c.development) != nil {
				deliveryErr = ErrInvalid
				return
			}
			if spec.RunnerVersion != subject.Version || subtle.ConstantTimeCompare([]byte(spec.CredentialHash), []byte(subject.CredentialHash)) != 1 {
				deliveryErr = auth.ErrUnauthenticated
				return
			}
			var e error
			if e = ready.ctx.Err(); e != nil {
				deliveryErr = ErrUnavailable
				return
			}
			if e = authctx.Err(); e != nil {
				deliveryErr = ErrUnavailable
				return
			}
			secret := make([]byte, 32)
			if _, e = rand.Read(secret); e != nil {
				deliveryErr = ErrUnavailable
				return
			}
			capability := base64.RawURLEncoding.EncodeToString(secret)
			clear(secret)
			grant := Grant{ID: domain.NewID(), RunnerVersion: subject.Version, Target: target, Operation: operation, AuthorityID: spec.AuthorityID, Connection: spec.Connection, ExpiresAt: time.Now().UTC().Add(operationTTL), ResultCapability: capability}
			if deadline, ok := authctx.Deadline(); ok && deadline.Before(grant.ExpiresAt) {
				grant.ExpiresAt = deadline
			}
			grant.TimeoutMS = time.Until(grant.ExpiresAt).Milliseconds()
			grant.executionDeadline = time.Now().Add(time.Duration(grant.TimeoutMS) * time.Millisecond)
			if grant.TimeoutMS < 1 {
				deliveryErr = ErrUnavailable
				return
			}
			wire, e := grant.MarshalWire()
			if e != nil || len(wire) > MaxGrant {
				deliveryErr = ErrInvalid
				return
			}
			item := &pending{check: spec.Check, grant: grant, credentialHash: credentialHash, capabilityHash: sha256.Sum256([]byte(capability)), result: make(chan Result, 1)}
			c.mu.Lock()
			c.pruneUsed()
			_, exists := c.used[opKey]
			if c.stopped || exists || len(c.used) >= 10000 {
				c.mu.Unlock()
				deliveryErr = ErrUnavailable
				return
			}
			c.used[opKey] = c.now().Add(5 * time.Minute)
			c.pending[grant.ID] = item
			c.mu.Unlock()
			defer func() {
				c.mu.Lock()
				delete(c.pending, grant.ID)
				c.mu.Unlock()
				item.grant.Connection.Secret = ""
				if item.grant.Connection.Protection != nil {
					item.grant.Connection.Protection.Secret = ""
				}
				item.grant.ResultCapability = ""
			}()
			attempted = true
			select {
			case ready.grants <- grant:
			case <-ready.ctx.Done():
				deliveryErr = ErrUncertain
				return
			case <-authctx.Done():
				deliveryErr = ErrUncertain
				return
			case <-c.closed:
				deliveryErr = ErrUncertain
				return
			}
			select {
			case result = <-item.result:
				if result.Failure != nil {
					if result.Failure.Uncertain {
						deliveryErr = ErrUncertain
					} else {
						deliveryErr = &domain.ProviderError{Kind: result.Failure.Code, Message: "Private provider operation failed", RetryAfter: time.Duration(result.Failure.RetryAfterMS) * time.Millisecond}
					}
				}
			case <-authctx.Done():
				deliveryErr = ErrUncertain
			case <-c.closed:
				deliveryErr = ErrUncertain
			}
		})
		if !didCall {
			return Result{}, ErrConflict
		}
		return result, deliveryErr
	}
	e = authorize(authctx, subject, deliver)
	if e != nil {
		if attempted && (deliveryErr == nil || !errors.Is(e, deliveryErr)) {
			return result, ErrUncertain
		}
		return Result{}, e
	}
	if !called {
		return Result{}, ErrInvalid
	}
	if deliveryErr != nil {
		return result, deliveryErr
	}
	return result, nil
}
func (c *Connector) Complete(ctx context.Context, credential, grantID, capability string, result Result) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if !auth.ValidID(grantID) || len(credential) > 256 || len(capability) > 64 {
		return auth.ErrUnauthenticated
	}
	credentialHash := sha256.Sum256([]byte(credential))
	capabilityHash := sha256.Sum256([]byte(capability))
	c.mu.Lock()
	defer c.mu.Unlock()
	item := c.pending[grantID]
	if c.stopped || item == nil || item.consumed || !item.grant.ExpiresAt.After(time.Now()) {
		return auth.ErrUnauthenticated
	}
	validCredential := subtle.ConstantTimeCompare(credentialHash[:], item.credentialHash[:])
	validCapability := subtle.ConstantTimeCompare(capabilityHash[:], item.capabilityHash[:])
	if validCredential&validCapability != 1 {
		return auth.ErrUnauthenticated
	}
	if result.OperationID != item.grant.Operation.ID {
		return ErrInvalid
	}
	b, e := json.Marshal(result)
	if e != nil || len(b) > MaxResponse || !result.valid(item.grant.Operation.Kind) {
		return ErrInvalid
	}
	if credentialEcho(b, item.grant.Connection) || result.File != nil && item.grant.Connection.Secret != "" && bytes.Contains(result.File.Content, []byte(item.grant.Connection.Secret)) {
		return ErrInvalid
	}
	item.consumed = true
	item.result <- result
	return nil
}
func strictJSON(b []byte, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil || decoder.Decode(new(any)) != io.EOF {
		return ErrInvalid
	}
	return nil
}
func (r Result) valid(kind Kind) bool {
	count := 0
	for _, present := range []bool{r.Workflows != nil, r.DeploymentGates != nil, r.Deployment != nil, r.TrainGate != nil, r.ExecutionCheck != nil, r.Queue != nil, r.MergeEvidence != nil, r.Merge != nil, r.Commit != nil, r.Capabilities != nil, r.Inventory != nil, r.Repository != nil, r.SHA != "", r.Behind != nil, r.File != nil, r.Files != nil, r.Log != "", r.Change != nil, r.Checks != nil, r.Approvals != nil, r.Changes != nil, r.ModelCapabilities != nil, r.Models != nil, r.Manifest != nil, r.Turn != nil, r.Issues != nil, r.Advisories != nil, r.Link != ""} {
		if present {
			count++
		}
	}
	if r.Failure != nil {
		if count != 0 || r.Failure.RetryAfterMS < 0 || r.Failure.RetryAfterMS > 3600000 {
			return false
		}
		switch r.Failure.Code {
		case "auth", "scope", "rate_limit", "protocol", "transient", "model_unsupported", "capability_unknown", "quota", "context", "canceled", "invalid_request", "configuration", "invalid", "identity", "pagination", "unauthorized", "forbidden", "conflict", "provider", "response", "not_found", "rate_limited", "transport", "uncertain", "unsupported", "credential_echo", "expired":
			return true
		}
		return false
	}
	switch kind {
	case ForgeRefreshBranch:
		return count == 0
	case ForgeDeliveryWorkflows:
		return count == 0 || count == 1 && r.Workflows != nil
	case ForgePipelineInspect, ForgeDeliveryGates:
		return count == 1 && r.DeploymentGates != nil
	case ForgePipelineCancel, ForgeDeliveryStatus, ForgePipelineObserve, ForgePipelineTrigger, ForgePipelineRecover:
		return count == 1 && r.Deployment != nil
	case ForgeReadTrainGate, ForgeReleaseTrainGate:
		return count == 1 && r.TrainGate != nil
	case ForgeReadExecutionCheck, ForgeWriteExecutionCheck:
		return count == 1 && r.ExecutionCheck != nil
	case ForgeQueueState, ForgeCancelQueue:
		return count == 1 && r.Queue != nil
	case ForgeMergeInspect, ForgeQueueInspect:
		return count == 1 && r.MergeEvidence != nil
	case ForgeMerge, ForgeMergeResult:
		return count == 1 && r.Merge != nil
	case ForgeCommitProof:
		return count == 1 && r.Commit != nil
	case ModelTurn:
		return count == 1 && r.Turn != nil && r.Turn.Usage.Known
	case ForgeFindChange:
		return count == 0 || count == 1 && r.Change != nil
	case ForgeSourceManifest:
		return count == 1 && r.Manifest != nil
	case ForgeReconcileChanges:
		return count == 1 && r.Changes != nil
	case ModelProbe:
		return count == 1 && r.ModelCapabilities != nil
	case ModelList:
		return count == 1 && r.Models != nil
	case GiteaProbe:
		return count == 1 && r.Capabilities != nil
	case GiteaInventory:
		return count == 1 && r.Inventory != nil
	case GiteaRepository:
		return count == 1 && r.Repository != nil
	case GiteaResolveRef, ForgeUpdateBranch:
		return count == 1 && r.SHA != ""
	case GiteaReadFile:
		return count == 1 && r.File != nil
	case ForgeReadFiles:
		return count == 1 && len(r.Files) > 0
	case ForgeCheckLog:
		return count <= 1
	case ForgeBehind:
		return count == 1 && r.Behind != nil
	case GiteaReadChange, ForgeCreateChange, ForgeCloseChange, ForgeCommentChange:
		return count == 1 && r.Change != nil
	case GiteaChecks:
		return count == 0 || count == 1 && r.Checks != nil
	case ForgeIssues:
		return count == 0 || count == 1 && r.Issues != nil
	case ForgeAdvisories:
		return count == 0 || count == 1 && r.Advisories != nil
	case ForgePermissionsURL:
		return count == 1 && r.Link != ""
	case GiteaApprovals:
		return count == 0 || count == 1 && r.Approvals != nil
	}
	return false
}

func (c *Connector) Active(ctx context.Context, credential, grantID, capability string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if !auth.ValidID(grantID) || len(credential) > 256 || len(capability) > 64 {
		return auth.ErrUnauthenticated
	}
	key := sha256.Sum256([]byte(credential))
	cap := sha256.Sum256([]byte(capability))
	c.mu.Lock()
	item := c.pending[grantID]
	if c.stopped || item == nil || item.consumed || !item.grant.ExpiresAt.After(time.Now()) {
		c.mu.Unlock()
		return auth.ErrUnauthenticated
	}
	if subtle.ConstantTimeCompare(key[:], item.credentialHash[:])&subtle.ConstantTimeCompare(cap[:], item.capabilityHash[:]) != 1 {
		c.mu.Unlock()
		return auth.ErrUnauthenticated
	}
	check := item.check
	c.mu.Unlock()
	if check != nil {
		return check(ctx)
	}
	return nil
}
