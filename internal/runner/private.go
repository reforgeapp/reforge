package runner

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"github.com/jackc/pgx/v5"
	"reforge/internal/auth"
	"time"
)

func (s *Service) AuthenticateSupervisor(ctx context.Context, raw string) (Runner, error) {
	org, id, err := parse(raw, "sup")
	if err != nil {
		return Runner{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var r Runner
	err = s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
		var digest string
		var valid bool
		err := tx.QueryRow(ctx, `SELECT r.id::text,r.org_id::text,r.pool_id::text,r.name,r.state,r.version,r.credential_expires_at,r.credential_hash,r.state='active' AND r.credential_expires_at>clock_timestamp() AND p.state='active' FROM runners r JOIN runner_pools p ON p.org_id=r.org_id AND p.id=r.pool_id WHERE r.org_id=$1 AND r.id=$2`, org, id).Scan(&r.ID, &r.OrgID, &r.PoolID, &r.Name, &r.State, &r.Version, &r.CredentialExpiresAt, &digest, &valid)
		match := matches(raw, digest)
		if err != nil || !valid || !match {
			return auth.ErrUnauthenticated
		}
		return nil
	})
	if err != nil {
		return Runner{}, err
	}
	return r, nil
}

func (s *Service) ValidatePrivateSupervisorTx(ctx context.Context, tx pgx.Tx, expected Runner, credentialHash string) error {
	digestBytes, err := hex.DecodeString(credentialHash)
	if err != nil || len(digestBytes) != 32 || !auth.ValidID(expected.OrgID) || !auth.ValidID(expected.ID) || !auth.ValidID(expected.PoolID) || expected.Version < 1 {
		return auth.ErrUnauthenticated
	}
	var pool, digest string
	var version int64
	var valid bool
	err = tx.QueryRow(ctx, `SELECT r.pool_id::text,r.version,r.credential_hash,r.state='active' AND r.credential_expires_at>clock_timestamp() AND p.state='active' FROM runners r JOIN runner_pools p ON p.org_id=r.org_id AND p.id=r.pool_id WHERE r.org_id=$1 AND r.id=$2 FOR SHARE OF r,p`, expected.OrgID, expected.ID).Scan(&pool, &version, &digest, &valid)
	match := subtle.ConstantTimeCompare([]byte(digest), []byte(credentialHash)) == 1
	if err != nil || !valid || !match || pool != expected.PoolID || version != expected.Version {
		return auth.ErrUnauthenticated
	}
	return nil
}
