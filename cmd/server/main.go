package main

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"reforge/internal/agent"
	"reforge/internal/artifact"
	"reforge/internal/auth"
	"reforge/internal/budget"
	"reforge/internal/campaign"
	"reforge/internal/config"
	"reforge/internal/connections"
	"reforge/internal/control"
	"reforge/internal/customcmd"
	"reforge/internal/deployment"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/githubapp"
	"reforge/internal/gitops"
	"reforge/internal/httpapi"
	"reforge/internal/insights"
	"reforge/internal/inventory"
	"reforge/internal/maintenance/discovery"
	"reforge/internal/maintenance/repair"
	"reforge/internal/mergecontrol"
	"reforge/internal/modelbroker"
	"reforge/internal/policy"
	"reforge/internal/privateconnector"
	"reforge/internal/providers"
	"reforge/internal/runner"
	"reforge/internal/secrets"
	"reforge/internal/store"
	"reforge/internal/workflow"
	"sync"
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
	identity.SetOrgOIDCVault(vault)
	app.RegisterIdentity(identity)
	connectionService := connections.New(db, identity, vault, cfg.Development)
	providers.Factory{Development: cfg.Development}.Register(connectionService)
	app.RegisterConnections(connectionService)
	qualifications := agent.NewQualificationService(db, identity)
	app.RegisterAgentQualification(qualifications, connectionService)
	agentLauncher := agent.NewCommandLauncher(agent.CommandConfig{Executable: os.Getenv("REFORGE_AGENT_RUNTIME"), Args: []string{"app-server"}, ExpectedSHA256: os.Getenv("REFORGE_AGENT_RUNTIME_SHA256")})
	if image := os.Getenv("REFORGE_AGENT_RUNTIME_IMAGE"); image != "" {
		executable := os.Getenv("REFORGE_AGENT_RUNTIME_EXECUTABLE")
		if executable == "" {
			executable = "/app/codex"
		}
		if container := agent.NewContainerLauncher(agent.ContainerConfig{Image: image, Executable: executable, Args: []string{"app-server"}}); container != nil {
			agentLauncher = container
		}
	}
	app.RegisterAgentAuth(agent.NewAuthService(db, identity, connectionService, qualifications, agent.NewFactory(agentLauncher, nil)))
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
	var campaigns *campaign.Service
	budgets := budget.New(db, identity, func(ctx context.Context, tx pgx.Tx, lease budget.Lease) error {
		_, err := workflows.ValidateFenceTx(ctx, tx, workflow.Lease(lease), "budget")
		return err
	}, func(ctx context.Context, tx pgx.Tx, org, id string) error {
		if campaigns == nil {
			return budget.ErrUnknown
		}
		return campaigns.CheckScopeTx(ctx, tx, org, "campaign", id)
	})
	app.RegisterBudget(budgets)
	app.RegisterInsights(insights.New(identity))
	artifacts, err := artifact.NewLocal(db, cfg.ArtifactDirectory)
	if err != nil {
		return err
	}
	defer artifacts.Close()
	runners := runner.New(db, identity, workflows, artifacts)
	workflows.RegisterScopeCheck(runners.CheckScopeTx)
	connectionService.RegisterRunnerCheck(runners.CheckRunnerTx)
	app.RegisterRunner(runners)
	private, err := privateconnector.New(privateconnector.Config{Authenticate: runners.AuthenticateSupervisor, Development: cfg.Development, MaxConcurrent: 8})
	if err != nil {
		return err
	}
	defer private.Close()
	providers.RegisterPrivate(connectionService, private, runners)
	app.RegisterPrivateConnector(private)
	authority.Register("model.turn", func(ctx context.Context, tx pgx.Tx, t workflow.Task, p policy.Resolved) error {
		if t.State != domain.TaskPlanning && t.State != domain.TaskRepairing {
			return workflow.ErrPolicy
		}
		return nil
	})
	authority.Register("repair.custom", func(ctx context.Context, tx pgx.Tx, t workflow.Task, p policy.Resolved) error {
		if t.State != domain.TaskPlanning && t.State != domain.TaskRepairing {
			return workflow.ErrPolicy
		}
		return nil
	})
	modelBroker := modelbroker.New(db, runners, connectionService, budgets, private, vault, cfg.Development)
	app.RegisterModelBroker(modelBroker)
	providerReads := providers.New(db, connectionService, private, runners, cfg.Development)
	portfolio := inventory.New(db, identity, vault, providerReads, providers.DecodeWebhook)
	app.RegisterInventory(portfolio)
	hostedGitHub, err := githubapp.LoadHosted(cfg.GitHubApp)
	if err != nil {
		return err
	}
	githubApp := githubapp.New(db, identity, vault, connectionService, portfolio, githubapp.Options{PublicURL: cfg.PublicURL, Edition: cfg.Edition, Development: cfg.Development, Hosted: hostedGitHub})
	app.RegisterGitHubApp(githubApp)
	githubAppContext, stopGitHubApp := context.WithCancel(ctx)
	var githubAppDone sync.WaitGroup
	githubAppDone.Add(1)
	go func() { defer githubAppDone.Done(); githubApp.Run(githubAppContext) }()
	defer func() { stopGitHubApp(); githubAppDone.Wait() }()
	inventoryContext, stopInventory := context.WithCancel(ctx)
	var inventoryDone sync.WaitGroup
	for i := 0; i < 4; i++ {
		inventoryDone.Add(1)
		go func() { defer inventoryDone.Done(); _ = portfolio.Run(inventoryContext, "inventory-"+domain.NewID()) }()
	}
	defer func() { stopInventory(); inventoryDone.Wait() }()
	discoveries := discovery.New(db, identity, providerReads)
	app.RegisterDiscovery(discoveries)
	profiles := customcmd.New(db, identity)
	repairs := repair.New(db, identity, discoveries, workflows, runners, policies, budgets, connectionService, profiles, providerReads.ForExecution(), cfg.RepairImages)
	app.RegisterRepair(repairs)
	merges := mergecontrol.New(db, identity, connectionService, policies, providerReads)
	app.RegisterMerge(merges)
	promotions := gitops.New(db, identity, connectionService, policies, providerReads, merges)
	app.RegisterGitOps(promotions)
	gitopsContext, stopGitops := context.WithCancel(ctx)
	gitopsDone := make(chan struct{})
	defer func() { stopGitops(); <-gitopsDone }()
	deliveries := deployment.New(db, identity, connectionService, policies, providerReads)
	app.RegisterDeployment(deliveries)
	campaigns = campaign.New(db, identity, policies, workflows, cfg.RepairImages, nil)
	campaigns.ConfigureExecution(repairs, deliveries, promotions, merges)
	workflows.RegisterScopeCheck(func(ctx context.Context, tx pgx.Tx, org, kind, id string) error {
		if kind == "campaign" {
			return campaigns.CheckScopeTx(ctx, tx, org, kind, id)
		}
		return runners.CheckScopeTx(ctx, tx, org, kind, id)
	})
	workflows.RegisterCampaignAuthority(campaigns.TaskAuthorityTx)
	deliveries.RegisterGateAuthority(campaigns.GateAuthorityTx)
	promotions.RegisterGateAuthority(campaigns.GateAuthorityTx)
	merges.RegisterChangeAuthority(func(ctx context.Context, tx pgx.Tx, org, repo string, snapshot forge.MergeEvidence, a domain.Actor) (string, error) {
		ref, e := promotions.CheckMergeTx(ctx, tx, org, repo, snapshot, a)
		if e != nil {
			return ref, e
		}
		return ref, campaigns.CheckMergeTx(ctx, tx, org, repo, snapshot)
	})
	app.RegisterCampaigns(campaigns)
	app.RegisterCustomProfiles(profiles)
	app.RegisterCustomDispatch(customcmd.NewDispatcher(db, runners, profiles, budgets))
	campaignContext, stopCampaign := context.WithCancel(ctx)
	campaignDone := make(chan struct{})
	defer func() { stopCampaign(); <-campaignDone }()

	deliveryContext, stopDelivery := context.WithCancel(ctx)
	deliveryDone := make(chan struct{})
	defer func() { stopDelivery(); <-deliveryDone }()

	mergeContext, stopMerge := context.WithCancel(ctx)
	mergeDone := make(chan struct{})
	defer func() { stopMerge(); <-mergeDone }()
	runners.CompletionCheck = repairs.CheckCompletion
	authority.Register("repair.stage", repairs.CheckStage)
	authority.Register("stage", repairs.CheckStage)
	authority.Register("publish", repairs.CheckPublish)
	authority.Register("outbox.dispatch", func(ctx context.Context, tx pgx.Tx, t workflow.Task, p policy.Resolved) error {
		if t.State == domain.TaskValidating {
			return repairs.CheckStage(ctx, tx, t, p)
		}
		return repairs.CheckPublish(ctx, tx, t, p)
	})
	modelBroker.AuthorizeReservation = repairs.CheckModelTx
	for _, action := range []string{"repair.source", "repair.report"} {
		authority.Register(action, func(ctx context.Context, tx pgx.Tx, t workflow.Task, p policy.Resolved) error {
			if t.State != domain.TaskReproducing && t.State != domain.TaskPlanning && t.State != domain.TaskRepairing && t.State != domain.TaskValidating && t.State != domain.TaskPublishing {
				return workflow.ErrPolicy
			}
			return nil
		})
	}
	discoveryContext, stopDiscovery := context.WithCancel(ctx)
	discoveryDone := make(chan struct{})
	go func() { defer close(gitopsDone); _ = promotions.Run(gitopsContext) }()
	go func() { defer close(campaignDone); _ = campaigns.Run(campaignContext) }()
	go func() { defer close(deliveryDone); _ = deliveries.Run(deliveryContext) }()
	go func() { defer close(mergeDone); _ = merges.Run(mergeContext) }()
	go func() { defer close(discoveryDone); _ = discoveries.Run(discoveryContext) }()
	defer func() { stopDiscovery(); <-discoveryDone }()
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
