package main

import (
	"context"
	"github.com/reforgeapp/reforge/pkg/store"
	"log/slog"
	"os"
	"time"
)

func main() {
	u := os.Getenv("REFORGE_MIGRATION_DATABASE_URL")
	if u == "" {
		slog.Error("REFORGE_MIGRATION_DATABASE_URL required")
		os.Exit(1)
	}
	d := os.Getenv("REFORGE_MIGRATION_DIR")
	if d == "" {
		d = "pkg/store/migrations"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := store.Migrate(ctx, u, d); err != nil {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}
	slog.Info("migrations applied")
}
