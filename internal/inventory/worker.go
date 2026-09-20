package inventory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"reforge/internal/connections"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/privateconnector"
)

func (s *Service) Claim(ctx context.Context, org, worker string) (*Job, error) {
	if !auth.ValidID(org) || worker == "" || len(worker) > 128 {
		return nil, auth.ErrInvalid
	}
	var out *Job
	err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		if err := lockOrg(ctx, tx, org); err != nil {
			return err
		}
		var id string
		err := tx.QueryRow(ctx, `SELECT j.id::text FROM inventory_jobs j JOIN inventory_sources s ON s.org_id=j.org_id AND s.connection_id=j.connection_id WHERE j.org_id=$1 AND (j.state='queued' AND j.available_at<=clock_timestamp() OR j.state='running' AND j.lease_until<=clock_timestamp()) ORDER BY s.last_served,j.last_claimed,j.created_at,j.id LIMIT 1 FOR UPDATE OF j SKIP LOCKED`, org).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		j, err := scanJob(tx.QueryRow(ctx, `UPDATE inventory_jobs SET state='running',fencing_token=fencing_token+1,lease_owner=$3,lease_until=clock_timestamp()+interval '30 seconds',last_claimed=clock_timestamp(),updated_at=clock_timestamp(),version=version+1 WHERE org_id=$1 AND id=$2 RETURNING `+jobColumns, org, id, worker))
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE inventory_sources SET last_served=clock_timestamp() WHERE org_id=$1 AND connection_id=$2`, org, j.ConnectionID); err != nil {
			return err
		}
		out = &j
		return nil
	})
	return out, err
}
func validateLease(ctx context.Context, tx pgx.Tx, lease Job) (Job, error) {
	j, err := loadJob(ctx, tx, lease.OrgID, lease.ID)
	if err != nil {
		return j, err
	}
	if j.State != "running" || j.Fence != lease.Fence || j.LeaseOwner != lease.LeaseOwner {
		return j, ErrStale
	}
	var active bool
	if err = tx.QueryRow(ctx, `SELECT lease_until>clock_timestamp() FROM inventory_jobs WHERE org_id=$1 AND id=$2`, j.OrgID, j.ID).Scan(&active); err != nil {
		return j, err
	}
	if j.State != "running" || j.Fence != lease.Fence || j.LeaseOwner != lease.LeaseOwner || !active {
		return j, ErrStale
	}
	if j.RequestedBy != "" {
		var permitted bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memberships WHERE org_id=$1 AND user_id=$2 AND role='owner' AND all_repositories)`, j.OrgID, j.RequestedBy).Scan(&permitted); err != nil {
			return j, err
		}
		if !permitted {
			return j, auth.ErrForbidden
		}
	}
	return j, nil
}
func validateConnection(j Job, c connections.Connection) error {
	if c.ID != j.ConnectionID || c.OrgID != j.OrgID || c.Version != j.ConnectionVersion || c.Settings.Namespace != j.Namespace || c.Kind != "forge" || c.State == "revoked" || c.State == "disabled" {
		return ErrStale
	}
	return nil
}
func (s *Service) Step(ctx context.Context, lease Job) error {
	if s.reader == nil {
		return errors.New("inventory reader is not configured")
	}
	if !auth.ValidID(lease.OrgID) || !auth.ValidID(lease.ID) || lease.Fence < 1 {
		return auth.ErrInvalid
	}
	if lease.Kind == "import" {
		err := s.persist(ctx, lease, func(tx pgx.Tx, j Job, c connections.Connection) error { return s.importPage(ctx, tx, j, c) })
		if err != nil {
			return errors.Join(err, s.fail(ctx, lease, err))
		}
		return nil
	}
	operation := privateconnector.Operation{ID: domain.NewID()}
	switch lease.Kind {
	case "scan":
		operation.Kind = privateconnector.GiteaInventory
		operation.Inventory = &privateconnector.InventoryArgs{Namespace: lease.Namespace, Cursor: lease.Cursor, Limit: 100}
	case "refresh":
		var ref forge.RepoRef
		err := s.db.Tenant(ctx, lease.OrgID, "", func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT native_id,name FROM repositories WHERE org_id=$1 AND id=$2 AND connection_id=$3`, lease.OrgID, lease.RepositoryID, lease.ConnectionID).Scan(&ref.NativeID, &ref.FullName)
		})
		if err != nil {
			return errors.Join(err, s.fail(ctx, lease, err))
		}
		if lease.Phase == "changes" {
			operation.Kind = privateconnector.ForgeReconcileChanges
			operation.Changes = &privateconnector.ChangesArgs{Repository: ref, Cursor: lease.Cursor}
		} else {
			operation.Kind = privateconnector.GiteaRepository
			operation.Repository = &privateconnector.RepositoryArgs{Repository: ref}
		}
	default:
		return auth.ErrInvalid
	}
	readCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	result, err := s.reader.Read(readCtx, lease.OrgID, lease.ConnectionID, operation, func(ctx context.Context, tx pgx.Tx, c connections.Connection) error {
		j, err := validateLease(ctx, tx, lease)
		if err != nil {
			return err
		}
		return validateConnection(j, c)
	})
	cancel()
	if err == nil && result.OperationID != operation.ID {
		err = ErrIncomplete
	}
	if err == nil && result.Failure != nil {
		err = &domain.ProviderError{Kind: result.Failure.Code, Message: "Read operation did not complete", Uncertain: result.Failure.Uncertain}
	}
	if err == nil {
		err = s.persist(ctx, lease, func(tx pgx.Tx, j Job, c connections.Connection) error {
			switch {
			case j.Kind == "scan" && result.Inventory != nil:
				return s.scanPage(ctx, tx, j, c, *result.Inventory)
			case j.Kind == "refresh" && j.Phase == "changes" && result.Changes != nil:
				return s.changePage(ctx, tx, j, *result.Changes)
			case j.Kind == "refresh" && j.Phase != "changes" && result.Repository != nil:
				return s.refreshRepository(ctx, tx, j, *result.Repository)
			default:
				return ErrIncomplete
			}
		})
	}
	if err != nil {
		return errors.Join(err, s.fail(ctx, lease, err))
	}
	return nil
}
func (s *Service) persist(ctx context.Context, lease Job, fn func(pgx.Tx, Job, connections.Connection) error) error {
	return s.db.Tenant(ctx, lease.OrgID, "", func(tx pgx.Tx) error {
		if err := lockOrg(ctx, tx, lease.OrgID); err != nil {
			return err
		}
		j, err := validateLease(ctx, tx, lease)
		if err != nil {
			return err
		}
		c, err := connection(ctx, tx, j.OrgID, j.ConnectionID)
		if err != nil {
			return err
		}
		if err = validateConnection(j, c); err != nil {
			return err
		}
		return fn(tx, j, c)
	})
}
func progress(ctx context.Context, tx pgx.Tx, j Job, cursor, phase string, processed int, complete bool, input []byte) error {
	state := "queued"
	if complete {
		state = "complete"
	}
	if input == nil {
		input = j.Input
	}
	tag, err := tx.Exec(ctx, `UPDATE inventory_jobs SET state=$3,cursor=$4,phase=$5,pages=pages+1,processed=processed+$6,lease_until=NULL,reason='',available_at=clock_timestamp(),input=$7,updated_at=clock_timestamp(),version=version+1 WHERE org_id=$1 AND id=$2 AND fencing_token=$8 AND state='running' AND lease_until>clock_timestamp()`, j.OrgID, j.ID, state, cursor, phase, processed, input, j.Fence)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrStale
	}
	if err == nil && complete {
		return emit(ctx, tx, j, "", "inventory.completed", j.Version+1)
	}
	return err
}
func pagination(cursor, next string, complete bool, count, pages int) error {
	if count > 100 || pages >= 1000 || len(next) > 32 || complete != (next == "") {
		return ErrIncomplete
	}
	if next != "" {
		n, err := strconv.Atoi(next)
		if err != nil || n <= 1 {
			return ErrIncomplete
		}
		old := 1
		if cursor != "" {
			old, err = strconv.Atoi(cursor)
			if err != nil {
				return ErrIncomplete
			}
		}
		if n != old+1 || count == 0 {
			return ErrIncomplete
		}
	}
	return nil
}
func validRepository(r forge.Repository) bool {
	u, err := url.Parse(r.URL)
	if err != nil || u.Scheme != "https" && u.Scheme != "http" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	return !strings.ContainsAny(r.FullName+r.DefaultBranch, "\x00\r\n") && validNative(r.NativeID) && r.FullName != "" && len(r.FullName) <= 1024 && len(r.URL) <= 4096 && len(r.CloneURL) <= 4096 && len(r.DefaultBranch) <= 1024
}
func (s *Service) scanPage(ctx context.Context, tx pgx.Tx, j Job, c connections.Connection, page domain.Page[forge.Repository]) error {
	if err := pagination(j.Cursor, page.NextCursor, page.Complete, len(page.Items), j.Pages); err != nil {
		return err
	}
	for _, r := range page.Items {
		if !validRepository(r) {
			return ErrIncomplete
		}
		r.CloneURL = ""
		body, _ := json.Marshal(r)
		if len(body) > 64<<10 {
			return ErrIncomplete
		}
		result, err := tx.Exec(ctx, `INSERT INTO inventory_candidates(org_id,job_id,native_id,repository) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, j.OrgID, j.ID, r.NativeID, body)
		if err != nil {
			return err
		}
		if result.RowsAffected() != 1 {
			return ErrIncomplete
		}
	}
	if page.Complete {
		_, err := tx.Exec(ctx, `UPDATE repositories r SET name=i.repository->>'full_name',url=i.repository->>'url',default_branch=i.repository->>'default_branch',archived=(i.repository->>'archived')::boolean,accessible=true,last_synced_at=clock_timestamp(),version=r.version+1 FROM inventory_candidates i,inventory_repository_state s WHERE i.org_id=$1 AND i.job_id=$2 AND r.org_id=i.org_id AND r.connection_id=$3 AND r.native_id=i.native_id AND s.org_id=r.org_id AND s.repository_id=r.id AND s.namespace=$4`, j.OrgID, j.ID, j.ConnectionID, j.Namespace)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE inventory_repository_state s SET connection_version=$4,scan_id=$2,state=CASE WHEN EXISTS(SELECT 1 FROM inventory_candidates i JOIN repositories r ON r.org_id=i.org_id AND r.native_id=i.native_id AND r.connection_id=$3 WHERE i.org_id=$1 AND i.job_id=$2 AND r.id=s.repository_id) THEN 'fresh' ELSE 'missing' END,reason=CASE WHEN EXISTS(SELECT 1 FROM inventory_candidates i JOIN repositories r ON r.org_id=i.org_id AND r.native_id=i.native_id AND r.connection_id=$3 WHERE i.org_id=$1 AND i.job_id=$2 AND r.id=s.repository_id) THEN '' ELSE 'Not returned by a completed inventory scan' END WHERE s.org_id=$1 AND s.connection_id=$3 AND s.namespace=$5`, j.OrgID, j.ID, j.ConnectionID, c.Version, j.Namespace)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE repositories r SET accessible=false,last_synced_at=clock_timestamp(),version=r.version+1 FROM inventory_repository_state s WHERE s.org_id=$1 AND s.scan_id=$2 AND s.state='missing' AND r.org_id=s.org_id AND r.id=s.repository_id`, j.OrgID, j.ID)
		if err != nil {
			return err
		}
	}
	return progress(ctx, tx, j, page.NextCursor, "", len(page.Items), page.Complete, nil)
}
func (s *Service) refreshRepository(ctx context.Context, tx pgx.Tx, j Job, r forge.Repository) error {
	if !validRepository(r) {
		return ErrIncomplete
	}
	var native string
	if err := tx.QueryRow(ctx, `SELECT native_id FROM repositories WHERE org_id=$1 AND id=$2`, j.OrgID, j.RepositoryID).Scan(&native); err != nil {
		return err
	}
	if native != r.NativeID {
		return ErrIncomplete
	}
	_, err := tx.Exec(ctx, `UPDATE repositories SET name=$3,url=$4,default_branch=$5,archived=$6,version=version+1 WHERE org_id=$1 AND id=$2`, j.OrgID, j.RepositoryID, r.FullName, r.URL, r.DefaultBranch, r.Archived)
	if err != nil {
		return err
	}
	return progress(ctx, tx, j, "", "changes", 0, false, nil)
}
func (s *Service) changePage(ctx context.Context, tx pgx.Tx, j Job, page domain.Page[forge.Change]) error {
	if err := pagination(j.Cursor, page.NextCursor, page.Complete, len(page.Items), j.Pages); err != nil {
		return err
	}
	var native string
	if err := tx.QueryRow(ctx, `SELECT native_id FROM repositories WHERE org_id=$1 AND id=$2`, j.OrgID, j.RepositoryID).Scan(&native); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, change := range page.Items {
		if !validNative(change.ID) || change.Repository.NativeID != native || seen[change.ID] {
			return ErrIncomplete
		}
		seen[change.ID] = true
		body, _ := json.Marshal(change)
		if len(body) > 256<<10 {
			return ErrIncomplete
		}
		var already bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM inventory_changes WHERE org_id=$1 AND repository_id=$2 AND native_id=$3 AND job_id=$4)`, j.OrgID, j.RepositoryID, change.ID, j.ID).Scan(&already); err != nil {
			return err
		}
		if already {
			return ErrIncomplete
		}
		_, err := tx.Exec(ctx, `INSERT INTO inventory_changes(org_id,repository_id,native_id,snapshot,job_id) VALUES($1,$2,$3,$4,$5) ON CONFLICT(org_id,repository_id,native_id) DO UPDATE SET snapshot=excluded.snapshot,job_id=excluded.job_id,observed_at=clock_timestamp()`, j.OrgID, j.RepositoryID, change.ID, body, j.ID)
		if err != nil {
			return err
		}
	}
	if page.Complete {
		_, err := tx.Exec(ctx, `UPDATE inventory_changes SET snapshot=jsonb_set(snapshot,'{state}','"unknown"'::jsonb) WHERE org_id=$1 AND repository_id=$2 AND job_id<>$3`, j.OrgID, j.RepositoryID, j.ID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE inventory_repository_state SET changes_observed_at=clock_timestamp() WHERE org_id=$1 AND repository_id=$2`, j.OrgID, j.RepositoryID)
		if err != nil {
			return err
		}
	}
	if err := progress(ctx, tx, j, page.NextCursor, "changes", len(page.Items), page.Complete, nil); err != nil {
		return err
	}
	if page.Complete {
		var input struct {
			Dirty bool `json:"dirty"`
		}
		if err := json.Unmarshal(j.Input, &input); err != nil {
			return err
		}
		var version int64
		if err := tx.QueryRow(ctx, `UPDATE repositories SET version=version+1 WHERE org_id=$1 AND id=$2 RETURNING version`, j.OrgID, j.RepositoryID).Scan(&version); err != nil {
			return err
		}
		if err := emit(ctx, tx, j, j.RepositoryID, "repository.changes_observed", version); err != nil {
			return err
		}
		if input.Dirty {
			c, err := connection(ctx, tx, j.OrgID, j.ConnectionID)
			if err != nil {
				return err
			}
			return queueRefresh(ctx, tx, c, j.RepositoryID)
		}
	}
	return nil
}
func (s *Service) fail(ctx context.Context, lease Job, cause error) error {
	return s.db.Tenant(ctx, lease.OrgID, "", func(tx pgx.Tx) error {
		if err := lockOrg(ctx, tx, lease.OrgID); err != nil {
			return err
		}
		j, err := loadJob(ctx, tx, lease.OrgID, lease.ID)
		if err != nil {
			return err
		}
		if j.State != "running" || j.Fence != lease.Fence || j.LeaseOwner != lease.LeaseOwner {
			return nil
		}
		var active bool
		if err = tx.QueryRow(ctx, `SELECT lease_until>clock_timestamp() FROM inventory_jobs WHERE org_id=$1 AND id=$2`, j.OrgID, j.ID).Scan(&active); err != nil {
			return err
		}
		if !active {
			return nil
		}
		state, reason := "queued", "Provider read failed; retry scheduled"
		var provider *domain.ProviderError
		delay := retryDelay(j.Failures, 0)
		if errors.As(cause, &provider) {
			delay = retryDelay(j.Failures, provider.RetryAfter)
			switch provider.Kind {
			case "auth", "forbidden", "not_found", "unsupported", "invalid":
				state = "failed"
				reason = "Provider access unavailable; verify connection and resync"
			}
		}
		if errors.Is(cause, ErrIncomplete) {
			state = "stale"
			reason = "Incomplete or inconsistent provider pagination; start a new full sync"
		}
		if errors.Is(cause, ErrStale) || errors.Is(cause, auth.ErrForbidden) || errors.Is(cause, auth.ErrConflict) {
			state = "stale"
			reason = "Connection, membership or import authorization changed; start a new sync"
		}
		if j.Failures >= 7 {
			state = "failed"
			reason = "Retry limit reached; verify provider connectivity and start a new sync"
		}
		delay += time.Duration(j.ID[0]%10) * 100 * time.Millisecond
		_, err = tx.Exec(ctx, `UPDATE inventory_jobs SET state=$3,reason=$4,failures=failures+1,lease_until=NULL,available_at=clock_timestamp()+$5::interval,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2`, j.OrgID, j.ID, state, reason, fmt.Sprintf("%f seconds", delay.Seconds()))
		if err != nil {
			return err
		}
		if provider != nil && state == "failed" {
			if j.Kind == "scan" || provider.Kind == "auth" {
				if _, err = tx.Exec(ctx, `UPDATE inventory_sources SET poll_due='infinity' WHERE org_id=$1 AND connection_id=$2`, j.OrgID, j.ConnectionID); err != nil {
					return err
				}
				if _, err = tx.Exec(ctx, `UPDATE inventory_repository_state SET state='stale',reason=$3,changes_observed_at=NULL,refresh_due='infinity' WHERE org_id=$1 AND connection_id=$2`, j.OrgID, j.ConnectionID, reason); err != nil {
					return err
				}
			} else if j.Kind == "refresh" {
				if _, err = tx.Exec(ctx, `UPDATE inventory_repository_state SET changes_observed_at=NULL,refresh_due='infinity',reason=$3 WHERE org_id=$1 AND repository_id=$2`, j.OrgID, j.RepositoryID, reason); err != nil {
					return err
				}
			}
		}
		if j.Kind == "scan" {
			_, err = tx.Exec(ctx, `UPDATE inventory_repository_state SET state='stale',reason=$4 WHERE org_id=$1 AND connection_id=$2 AND namespace=$3`, j.OrgID, j.ConnectionID, j.Namespace, reason)
		}
		return err
	})
}
