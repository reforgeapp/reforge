package main

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"reforge/internal/artifact"
	"reforge/internal/auth"
	"reforge/internal/budget"
	"reforge/internal/config"
	"reforge/internal/connections"
	"reforge/internal/control"
	"reforge/internal/httpapi"
	"reforge/internal/policy"
	"reforge/internal/runner"
	"reforge/internal/secrets"
	"reforge/internal/store"
	"reforge/internal/workflow"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	app := httpapi.New(cfg, db)
	startup, startupCancel := context.WithTimeout(ctx, 20*time.Second)
	identity, err := auth.New(startup, db, auth.Config{
		PublicURL: cfg.PublicURL, Edition: cfg.Edition, Development: cfg.Development,
		FixtureAuth: cfg.FixtureAuth, ListenAddress: cfg.Address, OIDCIssuer: cfg.OIDCIssuer,
		OIDCClientID: cfg.OIDCClientID, OIDCClientSecret: cfg.OIDCClientSecret,
		BootstrapToken: cfg.BootstrapToken, BootstrapExpiresAt: cfg.BootstrapExpiresAt,
	})
	startupCancel()
	if err != nil {
		return err
	}
	app.RegisterIdentity(identity)
	var vault *secrets.Vault
	if cfg.Edition == "hosted" {
		kmsContext, cancel := context.WithTimeout(ctx, 20*time.Second)
		vault, err = secrets.NewKMS(kmsContext, secrets.KMSConfig{Region: cfg.KMSRegion, KeyID: cfg.KMSKeyARN, PreviousKeyIDs: cfg.KMSPreviousKeyARNs})
		cancel()
	} else {
		vault, err = secrets.New(cfg.EncryptionKeyID, cfg.EncryptionKeys)
	}
	if err != nil {
		return err
	}
	connectionService := connections.New(db, identity, vault, cfg.Development)
	app.RegisterConnections(connectionService)
	deploymentPolicy := policy.Policy{Schema: "maintenance/v1"}
	if cfg.PolicyFile != "" {
		body, readErr := os.ReadFile(cfg.PolicyFile)
		if readErr != nil {
			return errors.New("operator policy file unavailable")
		}
		deploymentPolicy, err = policy.Parse(body)
		if err != nil {
			return errors.New("operator policy file invalid")
		}
	}
	policies, err := policy.New(db, identity, deploymentPolicy)
	if err != nil {
		return err
	}
	app.RegisterPolicy(policies)
	authority := control.NewAuthority(policies)
	workflows := workflow.New(db, identity, authority.Check)
	app.RegisterWorkflow(workflows)
	budgets := budget.New(db, identity, func(ctx context.Context, tx pgx.Tx, lease budget.Lease) error {
		_, err := workflows.ValidateFenceTx(ctx, tx, workflow.Lease(lease), "budget")
		return err
	}, nil)
	app.RegisterBudget(budgets)
	artifacts, err := artifact.NewLocal(db, cfg.ArtifactDirectory)
	if err != nil {
		return err
	}
	defer artifacts.Close()
	runners := runner.New(db, identity, workflows, artifacts)
	workflows.RegisterScopeCheck(runners.CheckScopeTx)
	connectionService.RegisterRunnerCheck(runners.CheckRunnerTx)
	app.RegisterRunner(runners)
	srv := &http.Server{Addr: cfg.Address, Handler: app.Router, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	slog.Info("server ready", "address", cfg.Address, "edition", cfg.Edition, "development", cfg.Development)
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}
