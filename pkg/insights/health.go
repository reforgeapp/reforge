package insights

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/environmenthealth"
)

type EnvironmentHealth struct {
	RepositoryID string     `json:"repository_id"`
	Repository   string     `json:"repository"`
	Environment  string     `json:"environment"`
	Kind         string     `json:"kind"`
	OperationID  string     `json:"operation_id"`
	State        string     `json:"state"`
	ObservedAt   *time.Time `json:"observed_at,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
}

func (s *Service) Health(ctx context.Context, session auth.Session, org string, f Filter) (domain.Page[EnvironmentHealth], error) {
	out := domain.Page[EnvironmentHealth]{Items: []EnvironmentHealth{}}
	if err := validate(f); err != nil {
		return out, err
	}
	if f.State != "" && f.State != "healthy" && f.State != "unhealthy" && f.State != "unknown" {
		return out, auth.ErrInvalid
	}
	key := []string{"", "", ""}
	if f.Cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(f.Cursor)
		if err != nil || json.Unmarshal(raw, &key) != nil || len(key) != 3 || !auth.ValidID(key[0]) {
			return out, auth.ErrInvalid
		}
	}
	err := s.auth.WithActor(ctx, session, org, func(tx pgx.Tx, a domain.Actor) error {
		rows, err := tx.Query(ctx, `WITH configured AS (SELECT org_id,repository_id,environment,'native' AS kind FROM deployment_configurations UNION ALL SELECT org_id,source_repository_id,environment,'gitops' FROM gitops_configurations), health AS (SELECT c.repository_id::text AS repository_id,r.name,c.environment,c.kind,coalesce(h.operation_id::text,'') AS operation_id,CASE WHEN `+environmenthealth.CurrentPredicate+` THEN CASE WHEN h.healthy THEN 'healthy' ELSE 'unhealthy' END ELSE 'unknown' END AS state,h.observed_at,h.expires_at FROM configured c JOIN repositories r ON r.org_id=c.org_id AND r.id=c.repository_id LEFT JOIN environment_health h ON h.org_id=c.org_id AND h.repository_id=c.repository_id AND h.environment=c.environment AND h.kind=c.kind WHERE c.org_id=$1 AND ($2 OR c.repository_id=ANY($3::uuid[])) AND ($4='' OR c.repository_id::text=$4)) SELECT * FROM health WHERE ($5='' OR state=$5) AND (repository_id,kind,environment)>($6,$7,$8) ORDER BY repository_id,kind,environment LIMIT $9`, org, a.AllRepositories, a.RepositoryIDs, f.RepositoryID, f.State, key[0], key[1], key[2], f.Limit+1)
		if err != nil {
			return err
		}
		out.Items, err = pgx.CollectRows(rows, pgx.RowToStructByPos[EnvironmentHealth])
		return err
	})
	out.Complete = len(out.Items) <= f.Limit
	if !out.Complete {
		out.Items = out.Items[:f.Limit]
		last := out.Items[len(out.Items)-1]
		raw, _ := json.Marshal([]string{last.RepositoryID, last.Kind, last.Environment})
		out.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	return out, err
}
