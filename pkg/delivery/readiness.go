package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/deployment"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/mergecontrol"
	"github.com/reforgeapp/reforge/pkg/policy"
	"github.com/reforgeapp/reforge/pkg/release"
)

func ReadinessTx(ctx context.Context, tx pgx.Tx, a domain.Actor, repo, mode string, controls policy.Lists, resolve func(context.Context, pgx.Tx, string, string) (policy.Resolved, error)) ([]string, []string, error) {
	blockers, bindings := []string{}, []string{}
	if !slices.Contains([]string{"merge", "deliver", "recover"}, mode) {
		return blockers, bindings, nil
	}
	var raw []byte
	var provider, server string
	var version int64
	err := tx.QueryRow(ctx, `SELECT m.document,c.provider,c.server_version,c.version FROM merge_configurations m JOIN repositories r ON r.org_id=m.org_id AND r.id=m.repository_id JOIN connections c ON c.org_id=r.org_id AND c.id=r.connection_id WHERE m.org_id=$1 AND m.repository_id=$2 AND c.state='healthy' AND r.accessible AND NOT r.archived AND NOT r.paused`, a.OrgID, repo).Scan(&raw, &provider, &server, &version)
	mergeReady := false
	if err == nil {
		var c mergecontrol.Configuration
		if err = json.Unmarshal(raw, &c); err != nil {
			return nil, nil, err
		}
		mergeReady, err = qualifiedMergeTx(ctx, tx, a.OrgID, c, provider, server, version)
		if err != nil {
			return nil, nil, err
		}
		bindings = append(bindings, string(raw))
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, err
	}
	if !mergeReady {
		blockers = append(blockers, "Operator: qualify this repository's protected merge path")
	}
	if mode == "merge" {
		return blockers, bindings, nil
	}
	rows, err := tx.Query(ctx, `SELECT 'native',document FROM deployment_configurations WHERE org_id=$1 AND repository_id=$2 UNION ALL SELECT 'gitops',document FROM gitops_configurations WHERE org_id=$1 AND source_repository_id=$2 AND ($3 OR delivery_repository_id=ANY($4::uuid[])) ORDER BY 1 LIMIT 100`, a.OrgID, repo, a.AllRepositories, a.RepositoryIDs)
	if err != nil {
		return nil, nil, err
	}
	type item struct {
		kind string
		raw  []byte
	}
	var configs []item
	for rows.Next() {
		var c item
		if err = rows.Scan(&c.kind, &c.raw); err != nil {
			rows.Close()
			return nil, nil, err
		}
		configs = append(configs, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	deliveryReady, recoveryReady := false, false
	for _, item := range configs {
		var c struct {
			DeliveryRepositoryID string                   `json:"delivery_repository_id"`
			Environment          string                   `json:"environment"`
			Version              int64                    `json:"version"`
			Enabled              bool                     `json:"enabled"`
			Mode                 string                   `json:"mode"`
			Workflow             deployment.Workflow      `json:"workflow"`
			RecoveryWorkflow     *deployment.Workflow     `json:"recovery_workflow"`
			RecoveryAllowed      bool                     `json:"recovery_allowed"`
			ProvenancePublicKey  string                   `json:"provenance_public_key"`
			BuildWorkflowSHA256  string                   `json:"build_workflow_sha256"`
			Qualification        deployment.Qualification `json:"qualification"`
		}
		if err = json.Unmarshal(item.raw, &c); err != nil {
			return nil, nil, err
		}
		bindings = append(bindings, string(item.raw))
		workflow := "gitops:" + c.Environment
		if item.kind == "native" {
			workflow = c.Workflow.ID
			q := c.Qualification
			if c.Mode != "pipeline" || q.Provider != provider || q.ServerVersion != server || q.ConnectionVersion != version || !q.ExpiresAt.After(time.Now()) || !q.PinnedInputs || !q.NativeEnforcement || !q.EnvironmentSerialization || !q.NoBypass {
				continue
			}
		}
		if item.kind == "gitops" && c.Enabled {
			if !slices.Contains(controls.Environments, c.Environment) || !slices.Contains(controls.Workflows, workflow) {
				continue
			}
			var document []byte
			var deliveryProvider, deliveryServer string
			var deliveryVersion int64
			err := tx.QueryRow(ctx, `SELECT m.document,c.provider,c.server_version,c.version FROM merge_configurations m JOIN repositories r ON r.org_id=m.org_id AND r.id=m.repository_id JOIN connections c ON c.org_id=r.org_id AND c.id=r.connection_id WHERE m.org_id=$1 AND m.repository_id=$2 AND r.accessible AND NOT r.archived AND NOT r.paused AND c.state='healthy'`, a.OrgID, c.DeliveryRepositoryID).Scan(&document, &deliveryProvider, &deliveryServer, &deliveryVersion)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, nil, err
			}
			var config mergecontrol.Configuration
			if err == nil {
				err = json.Unmarshal(document, &config)
			}
			ready := false
			if err == nil {
				ready, err = qualifiedMergeTx(ctx, tx, a.OrgID, config, deliveryProvider, deliveryServer, deliveryVersion)
			}
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, nil, err
			}
			bindings = append(bindings, string(document))
			if !ready {
				blockers = append(blockers, "Operator: qualify the protected GitOps delivery repository for "+c.Environment)
				continue
			}
			target, err := resolve(ctx, tx, a.OrgID, c.DeliveryRepositoryID)
			if err != nil {
				return nil, nil, err
			}
			bindings = append(bindings, target.Hash)
			if target.Paused || len(target.Problems) > 0 || slices.Contains(target.Policy.Deny, policy.Publish) || slices.Contains(target.Policy.Deny, policy.Deploy) || mode == "recover" && slices.Contains(target.Policy.Deny, policy.Recover) || !slices.Contains(target.Policy.Allow.Environments, c.Environment) || !slices.Contains(target.Policy.Allow.Workflows, workflow) || target.Policy.Allow.Recipes != nil && !slices.Contains(target.Policy.Allow.Recipes, "gitops-manifest") {
				blockers = append(blockers, "Operator: approve the GitOps delivery repository policy for "+c.Environment)
				continue
			}
		}
		if !c.Enabled || controls.Environments != nil && !slices.Contains(controls.Environments, c.Environment) || controls.Workflows != nil && !slices.Contains(controls.Workflows, workflow) {
			continue
		}
		r, err := tx.Query(ctx, `SELECT id::text,document FROM release_receipts WHERE org_id=$1 AND repository_id=$2 AND kind=$3 AND environment=$4 AND configuration_version=$5 ORDER BY received_at DESC LIMIT 10`, a.OrgID, repo, item.kind, c.Environment, c.Version)
		if err != nil {
			return nil, nil, err
		}
		type report struct {
			id  string
			raw []byte
		}
		var reports []report
		for r.Next() {
			var body report
			if err = r.Scan(&body.id, &body.raw); err != nil {
				r.Close()
				return nil, nil, err
			}
			reports = append(reports, body)
		}
		err = r.Err()
		r.Close()
		if err != nil {
			return nil, nil, err
		}
		for _, report := range reports {
			var receipt deployment.SignedProvenance
			if json.Unmarshal(report.raw, &receipt) != nil {
				continue
			}
			p := receipt.Document
			if !release.Meaningful(p) || p.WorkflowSHA256 != c.BuildWorkflowSHA256 || !p.ExpiresAt.After(time.Now()) || time.Since(p.IssuedAt) < 0 || time.Since(p.IssuedAt) > 24*time.Hour || !deployment.VerifySignature(c.ProvenancePublicKey, p, receipt.Signature) {
				continue
			}
			bindings = append(bindings, report.id)
			deliveryReady = true
			allowed := item.kind == "native" && c.RecoveryWorkflow != nil || item.kind == "gitops" && c.RecoveryAllowed
			if allowed && p.Recovery != nil && (p.Recovery.State == "stateless" || p.Recovery.State == "compatible" && len(p.Recovery.CompatibleArtifacts) > 0) {
				recoveryReady = true
			}
		}
	}
	if !deliveryReady {
		blockers = append(blockers, "Operator: connect and qualify a CI release reporter and permitted deployment path")
	}
	if mode == "recover" && !recoveryReady {
		blockers = append(blockers, "Operator: qualify recovery and signed database compatibility before automatic restoration")
	}
	return blockers, bindings, nil
}

func qualifiedMergeTx(ctx context.Context, tx pgx.Tx, org string, c mergecontrol.Configuration, provider, server string, version int64) (bool, error) {
	q := c.Qualification
	ready := c.Enabled && c.CooperationReference != "" && q.Provider == provider && q.ServerVersion == server && q.ConnectionVersion == version && q.ExactHead && (q.StrictTarget || q.QueueExecutionGate) && !q.VerifiedAt.After(time.Now()) && q.ExpiresAt.After(time.Now())
	if c.InspectorConnectionID == "" {
		return ready && provider != "gitea", nil
	}
	var current bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM connections WHERE org_id=$1 AND id=$2 AND state='healthy' AND version=$3)`, org, c.InspectorConnectionID, q.InspectorVersion).Scan(&current)
	return ready && current, err
}

func qualifiedRepositoryMergeTx(ctx context.Context, tx pgx.Tx, org, repo string) (bool, error) {
	var raw []byte
	var provider, server string
	var version int64
	err := tx.QueryRow(ctx, `SELECT m.document,c.provider,c.server_version,c.version FROM merge_configurations m JOIN repositories r ON r.org_id=m.org_id AND r.id=m.repository_id JOIN connections c ON c.org_id=r.org_id AND c.id=r.connection_id WHERE m.org_id=$1 AND m.repository_id=$2 AND r.accessible AND NOT r.archived AND NOT r.paused AND c.state='healthy'`, org, repo).Scan(&raw, &provider, &server, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var cfg mergecontrol.Configuration
	if err = json.Unmarshal(raw, &cfg); err != nil {
		return false, err
	}
	return qualifiedMergeTx(ctx, tx, org, cfg, provider, server, version)
}
