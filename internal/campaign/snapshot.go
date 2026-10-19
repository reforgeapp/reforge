package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/budget"
	"github.com/reforgeapp/reforge/internal/deployment"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/gitops"
	"github.com/reforgeapp/reforge/internal/maintenance/recipes"
	"github.com/reforgeapp/reforge/internal/policy"
	"github.com/reforgeapp/reforge/internal/store"
	"github.com/reforgeapp/reforge/internal/workflow"
)

type Service struct {
	db       *store.Store
	auth     *auth.Service
	policies *policy.Service
	workflow *workflow.Service
	images   map[string]string
	executor Executor
}

func New(db *store.Store, identity *auth.Service, policies *policy.Service, jobs *workflow.Service, images map[string]string, executor Executor) *Service {
	copyImages := map[string]string{}
	for k, v := range images {
		copyImages[k] = v
	}
	return &Service{db: db, auth: identity, policies: policies, workflow: jobs, images: copyImages, executor: executor}
}

func manage(a domain.Actor) bool {
	return a.Role == domain.Owner || a.Role == domain.Admin || a.Role == domain.Maintainer
}

func (s *Service) Preview(ctx context.Context, session auth.Session, org string, in Input) (Preview, error) {
	out := Preview{ID: domain.NewID(), Input: in, Members: []Member{}, Blockers: []string{}, ExpiresAt: time.Now().UTC().Add(time.Hour)}
	if session.AutomationID() != "" {
		return out, auth.ErrForbidden
	}
	if e := validate(in); e != nil {
		return out, e
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !manage(a) {
			return auth.ErrForbidden
		}
		for _, input := range in.Members {
			m, e := s.snapshot(ctx, tx, org, a, in, input)
			if e != nil {
				return e
			}
			out.Members = append(out.Members, m)
		}
		slices.SortFunc(out.Members, func(a, b Member) int {
			if a.RepositoryID < b.RepositoryID {
				return -1
			}
			if a.RepositoryID > b.RepositoryID {
				return 1
			}
			return 0
		})
		out.Blockers = assignCanaries(in, out.Members)
		out.Hash = fingerprint(struct {
			Input   Input
			Members []Member
		}{in, out.Members})
		raw, e := json.Marshal(out)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO campaign_previews(org_id,id,requested_by,document,expires_at) VALUES($1,$2,$3,$4,$5)`, org, out.ID, a.UserID, raw, out.ExpiresAt)
		return e
	})
	return out, err
}

func (s *Service) repositoryPins(ctx context.Context, tx pgx.Tx, org string, a domain.Actor, repo string, m *Member) (string, error) {
	if !auth.CanReadRepository(a, repo) {
		return "", auth.ErrForbidden
	}
	var name, connection, provider, server, state string
	var version int64
	var archived, paused, accessible bool
	e := tx.QueryRow(ctx, `SELECT r.name,coalesce(r.connection_id::text,''),r.archived,r.paused,r.accessible,coalesce(c.provider,''),coalesce(c.server_version,''),coalesce(c.state,''),coalesce(c.version,0) FROM repositories r LEFT JOIN connections c ON c.org_id=r.org_id AND c.id=r.connection_id WHERE r.org_id=$1 AND r.id=$2`, org, repo).Scan(&name, &connection, &archived, &paused, &accessible, &provider, &server, &state, &version)
	if errors.Is(e, pgx.ErrNoRows) {
		return "", auth.ErrForbidden
	}
	if e != nil {
		return "", e
	}
	m.Repositories = append(m.Repositories, repo)
	if repo == m.RepositoryID {
		m.RepositoryName = name
	}
	m.Pins["connection/"+repo] = connection + ":" + strconv.FormatInt(version, 10)
	if archived || paused || !accessible || state != "healthy" || connection == "" {
		m.State = "excluded"
		m.Reason = "Repository or forge connection is paused, inaccessible or unverified"
	}
	resolved, e := s.policies.ResolveTx(ctx, tx, org, repo)
	if e != nil {
		return "", e
	}
	m.Pins["policy/"+repo] = resolved.Hash
	if resolved.Paused || resolved.ScopePaused || len(resolved.Problems) > 0 {
		m.State = "excluded"
		m.Reason = "Resolve paused or invalid effective policy before a new preview"
	}
	return provider + "/" + server, nil
}

func (s *Service) snapshot(ctx context.Context, tx pgx.Tx, org string, a domain.Actor, spec Input, input MemberInput) (Member, error) {
	return s.snapshotCurrent(ctx, tx, org, a, spec, input, false)
}
func (s *Service) snapshotMember(ctx context.Context, tx pgx.Tx, org string, a domain.Actor, spec Input, old Member) (Member, error) {
	m, e := s.snapshotCurrent(ctx, tx, org, a, spec, old.Input, old.State == "succeeded")
	if old.State == "succeeded" && spec.Kind == "repair" {
		m.Pins["finding"] = old.Pins["finding"]
	}
	return m, e
}
func (s *Service) snapshotCurrent(ctx context.Context, tx pgx.Tx, org string, a domain.Actor, spec Input, input MemberInput, completed bool) (Member, error) {
	m := Member{ID: domain.NewID(), RepositoryID: input.RepositoryID, Repositories: []string{}, Input: input, Pins: map[string]string{}, State: "pending"}
	group, e := s.repositoryPins(ctx, tx, org, a, input.RepositoryID, &m)
	if e != nil {
		return m, e
	}
	m.Group = spec.Kind + "/" + group
	blocked := func(reason string) (Member, error) { m.State = "excluded"; m.Reason = reason; return m, nil }
	if spec.Kind == "repair" {
		r := input.Repair
		var provenance string
		var digest, state string
		var version int64
		e = tx.QueryRow(ctx, `SELECT coalesce(evidence->>'provenance',''),evidence_digest,state,version FROM maintenance_findings WHERE org_id=$1 AND repository_id=$2 AND id=$3`, org, input.RepositoryID, r.FindingID).Scan(&provenance, &digest, &state, &version)
		if errors.Is(e, pgx.ErrNoRows) {
			return blocked("Finding is unavailable; refresh discovery and selection")
		}
		if e != nil {
			return m, e
		}
		m.Pins["finding"] = digest + ":" + strconv.FormatInt(version, 10)
		m.Pins["recipe"] = r.Recipe + ":" + recipes.CurrentVersion
		m.Pins["image"] = s.images[r.Recipe]
		m.Group += "/" + r.Recipe + "/" + provenance
		if !completed && (version != r.FindingVersion || state != "open") {
			return blocked("Finding version or ownership state changed; refresh discovery")
		}
		if m.Pins["image"] == "" {
			return blocked("Register a verified runner image for this recipe")
		}
		var model, modelState string
		var routeRaw []byte
		var modelVersion, poolVersion int64
		var poolState string
		e = tx.QueryRow(ctx, `SELECT coalesce(settings->>'model',''),state,version FROM connections WHERE org_id=$1 AND id=$2 AND kind IN ('model','agent')`, org, r.ModelConnectionID).Scan(&model, &modelState, &modelVersion)
		if errors.Is(e, pgx.ErrNoRows) {
			return blocked("Model connection is unavailable")
		}
		if e != nil {
			return m, e
		}
		m.Pins["model"] = r.ModelConnectionID + ":" + strconv.FormatInt(modelVersion, 10)
		if modelState != "healthy" {
			return blocked("Verify the model connection before a new preview")
		}
		e = tx.QueryRow(ctx, `SELECT config FROM budget_routes WHERE org_id=$1 AND connection_id=$2 AND model=$3 AND name=$4`, org, r.ModelConnectionID, model, r.ModelRoute).Scan(&routeRaw)
		if errors.Is(e, pgx.ErrNoRows) {
			return blocked("Configure the model billing route")
		}
		if e != nil {
			return m, e
		}
		var route budget.Route
		if e = json.Unmarshal(routeRaw, &route); e != nil {
			return m, e
		}
		m.Pins["route"] = fingerprint(route)
		if route.Paused || route.Mode == "quota" && !route.Qualified {
			return blocked("Billing route is paused or requires qualification")
		}
		e = tx.QueryRow(ctx, `SELECT version,state FROM runner_pools WHERE org_id=$1 AND id=$2 AND EXISTS(SELECT 1 FROM runner_pool_repositories p WHERE p.org_id=$1 AND p.pool_id=$2 AND p.repository_id=$3)`, org, r.RunnerPoolID, input.RepositoryID).Scan(&poolVersion, &poolState)
		if errors.Is(e, pgx.ErrNoRows) {
			return blocked("Runner pool is unavailable")
		}
		if e != nil {
			return m, e
		}
		m.Pins["runner_pool"] = r.RunnerPoolID + ":" + strconv.FormatInt(poolVersion, 10)
		if poolState != "active" {
			return blocked("Runner pool is draining")
		}
		return m, nil
	}
	var raw []byte
	if spec.Kind == "pipeline" {
		e = tx.QueryRow(ctx, `SELECT document FROM deployment_configurations WHERE org_id=$1 AND environment=$2`, org, input.Environment).Scan(&raw)
		if errors.Is(e, pgx.ErrNoRows) {
			return blocked("Configure the deployment environment")
		}
		if e != nil {
			return m, e
		}
		var c deployment.Configuration
		if e = json.Unmarshal(raw, &c); e != nil {
			return m, e
		}
		if c.RepositoryID != input.RepositoryID {
			return m, auth.ErrInvalid
		}
		m.Pins["configuration"] = fingerprint(c)
		m.Group += "/" + c.Workflow.Path
		q := c.Qualification
		var connectionVersion int64
		var provider, server string
		if e = tx.QueryRow(ctx, `SELECT c.version,c.provider,c.server_version FROM connections c JOIN repositories r ON r.org_id=c.org_id AND r.connection_id=c.id WHERE r.org_id=$1 AND r.id=$2`, org, input.RepositoryID).Scan(&connectionVersion, &provider, &server); e != nil {
			return m, e
		}
		if !c.Enabled || c.Mode != "pipeline" || c.ObservationSeconds < spec.ObservationSeconds || q.Provider != provider || q.ServerVersion != server || q.ConnectionVersion != connectionVersion || !q.ExpiresAt.After(time.Now()) || q.VerifiedAt.After(time.Now()) || !q.PinnedInputs || !q.NativeEnforcement || !q.EnvironmentSerialization || !q.NoBypass {
			return blocked("Enable qualified native enforcement and a health observation window at least as long as the campaign window")
		}
		if !completed && !deployment.ValidProvenance(c, org, *input.Pipeline, time.Now()) {
			return blocked("Supply current signed provenance for the exact source and immutable artifact")
		}
	} else {
		e = tx.QueryRow(ctx, `SELECT document FROM gitops_configurations WHERE org_id=$1 AND environment=$2`, org, input.Environment).Scan(&raw)
		if errors.Is(e, pgx.ErrNoRows) {
			return blocked("Configure the GitOps environment")
		}
		if e != nil {
			return m, e
		}
		var c gitops.Configuration
		if e = json.Unmarshal(raw, &c); e != nil {
			return m, e
		}
		if c.SourceRepositoryID != input.RepositoryID {
			return m, auth.ErrInvalid
		}
		if c.DeliveryRepositoryID != input.RepositoryID {
			deliveryGroup, err := s.repositoryPins(ctx, tx, org, a, c.DeliveryRepositoryID, &m)
			if err != nil {
				e = err
				return m, e
			}
			m.Group += "/" + deliveryGroup
		}
		m.Pins["configuration"] = fingerprint(c)
		m.Group += "/" + c.ManifestPath
		if !c.Enabled || c.ObservationSeconds < spec.ObservationSeconds {
			return blocked("Enable the GitOps environment with a health observation window at least as long as the campaign window")
		}
		proof := deployment.PreviewRequest{SourceSHA: input.GitOps.SourceSHA, ArtifactDigest: input.GitOps.ArtifactDigest, Provenance: input.GitOps.Provenance}
		if !completed && (input.GitOps.ChangeID == "" || !deployment.ValidProvenance(deployment.Configuration{RepositoryID: c.SourceRepositoryID, ProvenancePublicKey: c.ProvenancePublicKey, MaxEvidenceAgeSeconds: c.MaxEvidenceAgeSeconds}, org, proof, time.Now())) {
			return blocked("Supply the merged source change and current signed artifact provenance")
		}
	}
	return m, nil
}

func changedPins(old, current Member) error {
	if current.State == "excluded" {
		return fmt.Errorf("%w: %s", auth.ErrConflict, current.Reason)
	}
	if fingerprint(old.Pins) != fingerprint(current.Pins) || fingerprint(old.Repositories) != fingerprint(current.Repositories) {
		return fmt.Errorf("%w: campaign inputs changed; create a reviewed snapshot with renewed canaries", auth.ErrConflict)
	}
	return nil
}
