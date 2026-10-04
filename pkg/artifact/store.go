package artifact

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/reforgeapp/reforge/pkg/heartbeat"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/store"
)

const MaxSize int64 = 1 << 20
const MaxTenantSize int64 = 256 << 20

var ErrContent = errors.New("artifact content is not permitted")
var ErrQuota = errors.New("artifact storage limit reached")
var sensitive = regexp.MustCompile(`(?i)(authorization\s*[:=]|bearer\s+[a-z0-9._~+/-]{8,}|-----BEGIN [A-Z ]*PRIVATE KEY-----|(?:api[_-]?key|access[_-]?token|refresh[_-]?token|client[_-]?secret|password|credential)\s*["']?\s*[:=]\s*["']?[^\s"']{4,}|(?:gh[pousr]_|github_pat_|sk-[a-zA-Z0-9_-])[a-zA-Z0-9_-]{12,}|(?:sup|job|enr)/[a-f0-9-]{36}/[a-f0-9-]{36}/)`)

type blobs interface {
	put(context.Context, string, []byte) error
	get(context.Context, string) ([]byte, error)
	remove(context.Context, string) error
	list(context.Context, func(string, time.Time) error) error
	close() error
}

type Store struct {
	db    *store.Store
	blobs blobs
}

func NewLocal(db *store.Store, directory string) (*Store, error) {
	if !filepath.IsAbs(directory) {
		return nil, auth.ErrInvalid
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("artifact directory must be private and not a symlink")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	return &Store{db: db, blobs: fileBlobs{root}}, nil
}
func (s *Store) Close() error { return s.blobs.close() }
func validContent(media string, data []byte) bool {
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 || sensitive.Match(data) {
		return false
	}
	for _, r := range string(data) {
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	switch media {
	case "application/json":
		return json.Valid(data)
	case "text/plain", "text/x-diff":
		return true
	}
	return false
}
func blobName(org, id string) string { return org + "-" + id + ".data" }
func (s *Store) write(ctx context.Context, m *Metadata, input io.Reader) error {
	if !auth.ValidID(m.ID) || !auth.ValidID(m.OrgID) || !auth.ValidID(m.RepositoryID) || !auth.ValidID(m.TaskID) || len(m.Name) < 1 || len(m.Name) > 160 || m.Name == "." || m.Name == ".." || sensitive.MatchString(m.Name) || strings.ContainsAny(m.Name, "/\\\x00\r\n") {
		return auth.ErrInvalid
	}
	data, err := io.ReadAll(io.LimitReader(input, MaxSize+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > MaxSize {
		return ErrQuota
	}
	if !validContent(m.MediaType, data) {
		return ErrContent
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	m.SHA256 = hex.EncodeToString(digest[:])
	m.Size = int64(len(data))
	return s.blobs.put(ctx, blobName(m.OrgID, m.ID), data)
}
func (s *Store) Prepare(ctx context.Context, m Metadata, input io.Reader) (Metadata, error) {
	if m.ExpiresAt.Before(time.Now()) || m.ExpiresAt.After(time.Now().Add(30*24*time.Hour)) {
		return Metadata{}, auth.ErrInvalid
	}
	m.ID = domain.NewID()
	m.CreatedAt = time.Now().UTC()
	if err := s.write(ctx, &m, input); err != nil {
		return Metadata{}, err
	}
	return m, nil
}

func (s *Store) RecordPreparedTx(ctx context.Context, tx pgx.Tx, m Metadata, attemptID string) (Metadata, error) {
	if !auth.ValidID(attemptID) || !auth.ValidID(m.ID) || !auth.ValidID(m.OrgID) || !auth.ValidID(m.RepositoryID) || !auth.ValidID(m.TaskID) || m.ExpiresAt.Before(time.Now()) || m.ExpiresAt.After(time.Now().Add(30*24*time.Hour)) || m.Size < 0 || m.Size > MaxSize || len(m.SHA256) != 64 {
		return Metadata{}, auth.ErrInvalid
	}
	var used int64
	var count int
	if err := tx.QueryRow(ctx, `SELECT coalesce(sum(size),0),count(*) FROM artifacts WHERE org_id=$1 AND expires_at>clock_timestamp()`, m.OrgID).Scan(&used, &count); err != nil {
		return Metadata{}, err
	}
	if used+MaxSize > MaxTenantSize || count >= 4096 {
		return Metadata{}, ErrQuota
	}
	_, err := tx.Exec(ctx, `INSERT INTO artifacts(org_id,id,repository_id,task_id,attempt_id,name,media_type,size,sha256,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, m.OrgID, m.ID, m.RepositoryID, m.TaskID, attemptID, m.Name, m.MediaType, m.Size, m.SHA256, m.CreatedAt, m.ExpiresAt)
	if err != nil {
		return Metadata{}, err
	}
	return m, nil
}

func (s *Store) PutTx(ctx context.Context, tx pgx.Tx, m Metadata, attemptID string, input io.Reader) (Metadata, error) {
	if !auth.ValidID(attemptID) || m.ExpiresAt.Before(time.Now()) || m.ExpiresAt.After(time.Now().Add(30*24*time.Hour)) {
		return Metadata{}, auth.ErrInvalid
	}
	prepared, err := s.Prepare(ctx, m, input)
	if err != nil {
		return Metadata{}, err
	}
	recorded, err := s.RecordPreparedTx(ctx, tx, prepared, attemptID)
	if err != nil {
		_ = s.RemoveBlob(context.WithoutCancel(ctx), prepared.OrgID, prepared.ID)
		return Metadata{}, err
	}
	return recorded, nil
}

func (s *Store) MetadataTx(ctx context.Context, tx pgx.Tx, a domain.Actor, id string) (Metadata, error) {
	var m Metadata
	if !auth.ValidID(id) {
		return m, auth.ErrForbidden
	}
	err := tx.QueryRow(ctx, `SELECT id::text,org_id::text,repository_id::text,task_id::text,name,media_type,size,sha256,created_at,expires_at FROM artifacts WHERE org_id=$1 AND id=$2 AND expires_at>clock_timestamp()`, a.OrgID, id).Scan(&m.ID, &m.OrgID, &m.RepositoryID, &m.TaskID, &m.Name, &m.MediaType, &m.Size, &m.SHA256, &m.CreatedAt, &m.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !auth.CanReadRepository(a, m.RepositoryID) {
		return Metadata{}, auth.ErrForbidden
	}
	return m, err
}
func (s *Store) Open(ctx context.Context, m Metadata) (io.ReadCloser, error) {
	if !auth.ValidID(m.OrgID) || !auth.ValidID(m.ID) || m.Size < 0 || m.Size > MaxSize || !m.ExpiresAt.After(time.Now()) {
		return nil, auth.ErrForbidden
	}
	data, err := s.blobs.get(ctx, blobName(m.OrgID, m.ID))
	if errors.Is(err, os.ErrNotExist) {
		return nil, auth.ErrForbidden
	}
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != m.Size {
		return nil, ErrContent
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != m.SHA256 {
		return nil, ErrContent
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}
func (s *Store) RemoveBlob(ctx context.Context, org, id string) error {
	if !auth.ValidID(org) || !auth.ValidID(id) {
		return auth.ErrInvalid
	}
	return s.blobs.remove(ctx, blobName(org, id))
}
func (s *Store) DeleteByRetention(ctx context.Context, orgID string, before time.Time) (int, error) {
	if !auth.ValidID(orgID) || before.After(time.Now()) {
		return 0, auth.ErrInvalid
	}
	n := 0
	err := s.db.Tenant(ctx, orgID, "", func(tx pgx.Tx) error {
		var locked string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM organisations WHERE id=$1 FOR UPDATE`, orgID).Scan(&locked); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id::text FROM artifacts WHERE org_id=$1 AND expires_at<=$2 ORDER BY id LIMIT 200 FOR UPDATE`, orgID, before)
		if err != nil {
			return err
		}
		var ids []string
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err = s.RemoveBlob(ctx, orgID, id); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `DELETE FROM artifacts WHERE org_id=$1 AND id=$2`, orgID, id); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}

func (s *Store) Run(ctx context.Context) {
	for ctx.Err() == nil {
		var orgs []string
		err := pgx.BeginFunc(ctx, s.db.Pool, func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, `SELECT org_id::text FROM inventory_tenants ORDER BY 1`)
			if err != nil {
				return err
			}
			orgs, err = pgx.CollectRows(rows, pgx.RowTo[string])
			return err
		})
		for _, org := range orgs {
			for ctx.Err() == nil {
				n, e := s.DeleteByRetention(ctx, org, time.Now().Add(-time.Second))
				err = errors.Join(err, e)
				if e != nil || n < 200 {
					break
				}
			}
		}
		if _, e := s.SweepOrphans(ctx, time.Now().Add(-24*time.Hour)); e != nil {
			err = errors.Join(err, e)
		}
		heartbeat.Beat("artifacts", time.Hour, err)
		if err != nil && ctx.Err() == nil {
			slog.WarnContext(ctx, "artifact retention failed", "error", err)
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Hour):
		}
	}
}

func (s *Store) SweepOrphans(ctx context.Context, before time.Time) (int, error) {
	if s.db == nil || before.After(time.Now().Add(-time.Hour)) {
		return 0, auth.ErrInvalid
	}
	removed := 0
	err := s.blobs.list(ctx, func(name string, modified time.Time) error {
		if len(name) != 78 || name[36] != '-' || !strings.HasSuffix(name, ".data") || !modified.Before(before) {
			return nil
		}
		org, id := name[:36], name[37:73]
		if !auth.ValidID(org) || !auth.ValidID(id) {
			return nil
		}
		err := s.db.Tenant(ctx, org, "", func(tx pgx.Tx) error {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM artifacts WHERE org_id=$1 AND id=$2)`, org, id).Scan(&exists); err != nil {
				return err
			}
			if exists {
				return nil
			}
			if err := s.RemoveBlob(ctx, org, id); err != nil {
				return err
			}
			removed++
			return nil
		})
		if err == nil && removed >= 200 {
			return errSweepFull
		}
		return err
	})
	if errors.Is(err, errSweepFull) {
		err = nil
	}
	return removed, err
}

var errSweepFull = errors.New("sweep limit reached")

type fileBlobs struct{ root *os.Root }

func (f fileBlobs) close() error { return f.root.Close() }

func (f fileBlobs) put(_ context.Context, name string, data []byte) error {
	file, err := f.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	good := false
	defer func() {
		_ = file.Close()
		if !good {
			_ = f.root.Remove(name)
		}
	}()
	if _, err = file.Write(data); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	directory, err := f.root.Open(".")
	if err != nil {
		return err
	}
	err = directory.Sync()
	_ = directory.Close()
	if err != nil {
		return err
	}
	good = true
	return nil
}

func (f fileBlobs) get(_ context.Context, name string) ([]byte, error) {
	file, err := f.root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, os.ErrNotExist
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, ErrContent
	}
	return io.ReadAll(io.LimitReader(file, MaxSize+1))
}

func (f fileBlobs) remove(_ context.Context, name string) error {
	err := f.root.Remove(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (f fileBlobs) list(ctx context.Context, fn func(string, time.Time) error) error {
	directory, err := f.root.Open(".")
	if err != nil {
		return err
	}
	defer directory.Close()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, readErr := directory.ReadDir(200)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return readErr
		}
		if len(entries) == 0 {
			return nil
		}
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				continue
			}
			if err = fn(entry.Name(), info.ModTime()); err != nil {
				return err
			}
		}
	}
}
