package integration

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/store"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func testDatabase(t *testing.T, key string) string {
	t.Helper()
	value := os.Getenv(key)
	if value == "" {
		t.Skip("requires disposable PostgreSQL: " + key)
	}
	identity := ""
	keys := []string{"REFORGE_TEST_DATABASE_URL", "REFORGE_TEST_MIGRATION_DATABASE_URL"}
	if os.Getenv("REFORGE_TEST_ADMIN_DATABASE_URL") != "" {
		keys = append(keys, "REFORGE_TEST_ADMIN_DATABASE_URL")
	}
	for _, name := range keys {
		u, err := url.Parse(os.Getenv(name))
		if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Path != "/reforge_test" || u.Fragment != "" {
			t.Fatal("test connection must explicitly target disposable reforge_test: " + name)
		}
		query, err := url.ParseQuery(u.RawQuery)
		if err != nil {
			t.Fatal("invalid test database parameters")
		}
		for option, values := range query {
			if (option != "host" && option != "port" && option != "sslmode") || len(values) != 1 {
				t.Fatal("unsupported test database parameter")
			}
		}
		host, port := u.Hostname(), u.Port()
		if query.Has("host") {
			host = query.Get("host")
		}
		if query.Has("port") {
			port = query.Get("port")
		}
		if port == "" {
			port = "5432"
		}
		current := host + ":" + port + u.Path
		if identity != "" && current != identity {
			t.Fatal("test connections target different databases")
		}
		identity = current
	}
	return value
}

func TestMigrationRollbackAndChecksum(t *testing.T) {
	u := testDatabase(t, "REFORGE_TEST_MIGRATION_DATABASE_URL")
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	name := "migration_probe_" + strings.ReplaceAll(domain.NewID(), "-", "")
	file := filepath.Join(t.TempDir(), name+".sql")
	defer conn.Exec(ctx, `DROP TABLE IF EXISTS `+name)
	defer conn.Exec(ctx, `DELETE FROM schema_migrations WHERE name=$1`, name+".sql")
	if err := os.WriteFile(file, []byte(`CREATE TABLE `+name+`(value integer); SELECT missing_migration_function();`), 0600); err != nil {
		t.Fatal(err)
	}
	if store.Migrate(ctx, u, filepath.Dir(file)) == nil {
		t.Fatal("broken migration succeeded")
	}
	var absent bool
	if err := conn.QueryRow(ctx, `SELECT to_regclass($1) IS NULL`, name).Scan(&absent); err != nil || !absent {
		t.Fatalf("failed migration was not atomic: %v", err)
	}
	if err := os.WriteFile(file, []byte(`CREATE TABLE `+name+`(value integer);`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx, u, filepath.Dir(file)); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx, u, filepath.Dir(file)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(`SELECT 1;`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx, u, filepath.Dir(file)); err == nil || !strings.Contains(err.Error(), "checksum changed") {
		t.Fatalf("applied migration edit accepted: %v", err)
	}
}

func TestRuntimeRejectsInheritedOwner(t *testing.T) {
	u := testDatabase(t, "REFORGE_TEST_ADMIN_DATABASE_URL")
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, `GRANT reforge_migrator TO reforge_runtime`); err != nil {
		t.Fatal(err)
	}
	defer conn.Exec(ctx, `REVOKE reforge_migrator FROM reforge_runtime`)
	if db, err := store.Open(ctx, os.Getenv("REFORGE_TEST_DATABASE_URL")); err == nil {
		db.Close()
		t.Fatal("inherited migration authority accepted")
	}
}

func TestRuntimeIsolationAndPooledContext(t *testing.T) {
	u := testDatabase(t, "REFORGE_TEST_DATABASE_URL")
	ctx := context.Background()
	db, err := store.Open(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ids := []string{domain.NewID(), domain.NewID()}
	for _, id := range ids {
		err = db.Tenant(ctx, id, "", func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'isolation fixture')`, id)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := ids[i%2]
			err := db.Tenant(ctx, id, "", func(tx pgx.Tx) error {
				var count int
				var found string
				if err := tx.QueryRow(ctx, `SELECT count(*) FROM organisations`).Scan(&count); err != nil {
					return err
				}
				if count != 1 {
					t.Errorf("tenant %s saw %d organisations", id, count)
				}
				if err := tx.QueryRow(ctx, `SELECT id::text FROM organisations`).Scan(&found); err != nil {
					return err
				}
				if found != id {
					t.Errorf("tenant %s saw %s", id, found)
				}
				return nil
			})
			if err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	var count int
	if err = db.Pool.QueryRow(ctx, `SELECT count(*) FROM organisations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("pooled context leaked %d rows", count)
	}
	if _, err = db.Pool.Exec(ctx, `INSERT INTO organisations(id,name) VALUES($1,'unscoped')`, domain.NewID()); err == nil {
		t.Fatal("unscoped insert allowed")
	}
	ownerURL := os.Getenv("REFORGE_TEST_MIGRATION_DATABASE_URL")
	if ownerURL != "" {
		if unsafe, err := store.Open(ctx, ownerURL); err == nil {
			unsafe.Close()
			t.Fatal("migration credentials accepted for runtime")
		}
	}
}
