package store

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"github.com/jackc/pgx/v5"
	"io/fs"
	"os"
	"sort"
)

//go:embed migrations/*.sql
var embedded embed.FS

func Migrations() fs.FS {
	sub, _ := fs.Sub(embedded, "migrations")
	return sub
}

func Migrate(ctx context.Context, databaseURL, dir string) error {
	return MigrateFS(ctx, databaseURL, os.DirFS(dir))
}

func MigrateFS(ctx context.Context, databaseURL string, migrations fs.FS) error {
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connect migration database: %w", err)
	}
	defer conn.Close(ctx)
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock(718326)`); err != nil {
		return err
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(718326)`)
	if _, err = conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(name text PRIMARY KEY,checksum text NOT NULL,applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	files, err := fs.Glob(migrations, "*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	if len(files) == 0 {
		return fmt.Errorf("no migrations found")
	}
	for _, name := range files {
		body, err := fs.ReadFile(migrations, name)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(body)
		checksum := hex.EncodeToString(sum[:])
		var existing string
		err = conn.QueryRow(ctx, `SELECT checksum FROM schema_migrations WHERE name=$1`, name).Scan(&existing)
		if err == nil {
			if existing != checksum {
				return fmt.Errorf("migration checksum changed: %s", name)
			}
			continue
		}
		if err != pgx.ErrNoRows {
			return err
		}
		err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(body)); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO schema_migrations(name,checksum) VALUES($1,$2)`, name, checksum)
			return err
		})
		if err != nil {
			return fmt.Errorf("migration %s rolled back: %w", name, err)
		}
	}
	return nil
}
