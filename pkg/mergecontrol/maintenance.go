package mergecontrol

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/privateconnector"
)

type MaintenanceReadiness struct {
	Ready                bool   `json:"ready"`
	Reason               string `json:"reason"`
	SettingsURL          string `json:"settings_url,omitempty"`
	RulesHash            string `json:"rules_hash,omitempty"`
	ConnectionVersion    int64  `json:"connection_version"`
	ConfigurationVersion int64  `json:"configuration_version"`
}

func (s *Service) MaintenanceReadiness(ctx context.Context, session auth.Session, org, repo string) (MaintenanceReadiness, error) {
	out := MaintenanceReadiness{}
	var ref forge.RepoRef
	var id, provider, endpoint string
	var cfg Configuration
	var serverVersion, authority string
	var inspectorCurrent bool
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		if !auth.ValidID(repo) || !auth.CanReadRepository(a, repo) {
			return auth.ErrForbidden
		}
		var err error
		ref, id, err = repository(ctx, tx, org, repo)
		if err != nil {
			return err
		}
		cfg, err = configTx(ctx, tx, org, repo)
		if err != nil {
			return err
		}
		c, err := s.connections.MetadataTx(ctx, tx, org, id)
		if err != nil {
			return err
		}
		provider, endpoint, out.ConnectionVersion, out.ConfigurationVersion = c.Provider, c.Endpoint, c.Version, cfg.Version
		serverVersion = c.ServerVersion
		if err := tx.QueryRow(ctx, `SELECT coalesce((SELECT merge_authority FROM maintenance_configs WHERE org_id=$1 AND repository_id=$2),'')`, org, repo).Scan(&authority); err != nil {
			return err
		}
		inspectorCurrent = cfg.InspectorConnectionID == ""
		if cfg.InspectorConnectionID != "" {
			inspector, err := s.connections.MetadataTx(ctx, tx, org, cfg.InspectorConnectionID)
			if err != nil {
				return err
			}
			inspectorCurrent = inspector.State == "healthy" && inspector.Version == cfg.Qualification.InspectorVersion
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	if provider == "github" {
		base, err := url.Parse(endpoint)
		if err != nil {
			return out, err
		}
		if base.Hostname() == "api.github.com" {
			base.Host = "github.com"
		}
		base.Path, base.RawPath, base.RawQuery, base.Fragment = "/"+ref.FullName+"/settings/rules", "", "", ""
		out.SettingsURL = base.String()
	}
	if provider == "gitea" {
		out.SettingsURL = strings.TrimRight(endpoint, "/") + "/" + ref.FullName + "/settings/branches"
	}
	if provider == "gitlab" {
		out.SettingsURL = strings.TrimRight(endpoint, "/") + "/" + ref.FullName + "/-/settings/repository"
	}
	result, err := s.providers.ForProtection(cfg.InspectorConnectionID, cfg.CheckPublishers).Read(ctx, org, id, privateconnector.Operation{ID: domain.NewID(), Kind: privateconnector.ForgeReadRules, Repository: &privateconnector.RepositoryArgs{Repository: ref}}, func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		a, err := s.auth.ActorTx(ctx, tx, session, org)
		if err != nil {
			return err
		}
		if !auth.CanReadRepository(a, repo) {
			return auth.ErrForbidden
		}
		currentRef, currentID, err := repository(ctx, tx, org, repo)
		if err != nil {
			return err
		}
		current, err := configTx(ctx, tx, org, repo)
		if err != nil {
			return err
		}
		if currentRef != ref || currentID != id || c.ID != id || c.Version != out.ConnectionVersion || current.Version != cfg.Version {
			return auth.ErrConflict
		}
		return nil
	})
	if err != nil {
		out.Reason = "Could not verify repository merge permissions. Recheck before enabling Autopilot."
		return out, nil
	}
	if result.Rules == nil {
		return out, privateconnector.ErrInvalid
	}
	rules := *result.Rules
	out.RulesHash = rules.Hash
	switch {
	case !rules.ActorCanBypass && (rules.RequiredApprovals > 0 || rules.RequireCodeOwners):
		out.Reason = "Your repository requires a person's approval. Allow the Reforge app through that review rule to merge verified fixes automatically."
	case rules.State != domain.Supported:
		out.Reason = "Repository merge rules need attention before Reforge can merge unattended: " + rules.Reason
	case len(rules.AllowedMergeMethods) == 0:
		out.Reason = "Enable a merge method in repository settings."
	case !cfg.Enabled && (rules.Unprotected || rules.ActorCanBypass):
		out.Ready = true
	default:
		q := cfg.Qualification
		out.Ready = cfg.Enabled && authority == "reforge" && cfg.CooperationReference != "" && q.Provider == provider && q.ServerVersion == serverVersion && inspectorCurrent && q.ConnectionVersion == out.ConnectionVersion && q.ExactHead && !q.VerifiedAt.After(time.Now()) && q.ExpiresAt.After(time.Now()) && (q.StrictTarget && !rules.RequireQueue || q.QueueExecutionGate && rules.RequireQueue)
		if !out.Ready {
			out.Reason = "Protected merge checks need operator setup before Reforge can merge unattended."
		}
	}
	return out, nil
}
