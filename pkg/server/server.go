package server

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/reforgeapp/reforge/pkg/agent"
	"github.com/reforgeapp/reforge/pkg/alerts"
	"github.com/reforgeapp/reforge/pkg/artifact"
	"github.com/reforgeapp/reforge/pkg/auth"
	"github.com/reforgeapp/reforge/pkg/autopilot"
	"github.com/reforgeapp/reforge/pkg/budget"
	"github.com/reforgeapp/reforge/pkg/campaign"
	"github.com/reforgeapp/reforge/pkg/config"
	"github.com/reforgeapp/reforge/pkg/connections"
	"github.com/reforgeapp/reforge/pkg/control"
	"github.com/reforgeapp/reforge/pkg/customcmd"
	"github.com/reforgeapp/reforge/pkg/deployment"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/forge"
	"github.com/reforgeapp/reforge/pkg/githubapp"
	"github.com/reforgeapp/reforge/pkg/gitops"
	"github.com/reforgeapp/reforge/pkg/httpapi"
	"github.com/reforgeapp/reforge/pkg/insights"
	"github.com/reforgeapp/reforge/pkg/inventory"
	"github.com/reforgeapp/reforge/pkg/maintenance/discovery"
	"github.com/reforgeapp/reforge/pkg/maintenance/repair"
	"github.com/reforgeapp/reforge/pkg/mergecontrol"
	"github.com/reforgeapp/reforge/pkg/model"
	"github.com/reforgeapp/reforge/pkg/modelbroker"
	"github.com/reforgeapp/reforge/pkg/policy"
	"github.com/reforgeapp/reforge/pkg/privateconnector"
	"github.com/reforgeapp/reforge/pkg/providers"
	"github.com/reforgeapp/reforge/pkg/runner"
	"github.com/reforgeapp/reforge/pkg/secrets"
	"github.com/reforgeapp/reforge/pkg/store"
	"github.com/reforgeapp/reforge/pkg/workflow"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"
)

type App struct {
	Config      config.Config
	DB          *store.Store
	HTTP        *httpapi.Server
	Identity    *auth.Service
	Vault       *secrets.Vault
	Connections *connections.Service
	Policies    *policy.Service
	Workflows   *workflow.Service
	Budgets     *budget.Service
	Runners     *runner.Service
	ModelBroker *modelbroker.Service
	Providers   *providers.Service
	Inventory   *inventory.Service
	Discovery   *discovery.Service
	Repairs     *repair.Service
	Merges      *mergecontrol.Service
	Autopilot   *autopilot.Service
	Alerts      *alerts.Service
}

func Run(ctx context.Context, cfg config.Config, setup func(*App) error) error {
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
	if cfg.KMSKeyARN != "" {
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
	overview := insights.New(identity)
	app.RegisterInsights(overview)
	app.RegisterSetup(overview, policies.ResolveTx, len(cfg.RepairImages) > 0, cfg.BuiltinRunnerToken != "")
	var artifacts *artifact.Store
	if s3 := cfg.ArtifactS3; s3.Bucket != "" {
		artifacts, err = artifact.NewS3(ctx, db, artifact.S3Config{Endpoint: s3.Endpoint, Region: s3.Region, Bucket: s3.Bucket, AccessKeyID: s3.AccessKeyID, SecretAccessKey: s3.SecretAccessKey})
	} else {
		artifacts, err = artifact.NewLocal(db, cfg.ArtifactDirectory)
	}
	if err != nil {
		return err
	}
	defer artifacts.Close()
	runners := runner.New(db, identity, workflows, artifacts)
	workflows.RegisterScopeCheck(runners.CheckScopeTx)
	connectionService.RegisterRunnerCheck(runners.CheckRunnerTx)
	app.RegisterRunner(runners)
	if cfg.BuiltinRunnerToken != "" {
		app.RegisterBuiltinRunner(runners, cfg.BuiltinRunnerToken)
	}
	var routes privateconnector.Routes
	if cfg.PodAddress != "" {
		routes = privateconnector.NewPostgresRoutes(db, cfg.PodAddress)
	}
	private, err := privateconnector.New(privateconnector.Config{Authenticate: runners.AuthenticateSupervisor, Development: cfg.Development, MaxConcurrent: 256, Routes: routes})
	if err != nil {
		return err
	}
	defer private.Close()
	if postgres, ok := routes.(privateconnector.PostgresRoutes); ok {
		go postgres.Listen(ctx, private.Notify)
	}
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
	autopilots := autopilot.New(db, identity, repairs, merges, budgets, policies)
	autopilots.Tasks = workflows
	autopilots.Connections = connectionService
	autopilots.Deployments = deliveries
	autopilots.Promotions = promotions
	notices := alerts.New(db, identity, vault, alerts.SMTP{Address: cfg.SMTPAddress, Username: cfg.SMTPUsername, Password: cfg.SMTPPassword, From: cfg.SMTPFrom, Security: cfg.SMTPSecurity, ServerName: cfg.SMTPTLSServerName}, cfg.PublicURL)
	autopilots.Notify = notices.Notify
	autopilots.Scan = func(ctx context.Context, session auth.Session, org, repo, request string) error {
		_, err := discoveries.StartScan(ctx, session, org, repo, request)
		return err
	}
	app.RegisterAutopilot(autopilots)
	app.RegisterAlerts(notices)
	autopilotContext, stopAutopilot := context.WithCancel(ctx)
	autopilotDone := make(chan struct{})
	defer func() { stopAutopilot(); <-autopilotDone }()
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
	if setup != nil {
		if err = setup(&App{Config: cfg, DB: db, HTTP: app, Identity: identity, Vault: vault, Connections: connectionService, Policies: policies, Workflows: workflows, Budgets: budgets, Runners: runners, ModelBroker: modelBroker, Providers: providerReads, Inventory: portfolio, Discovery: discoveries, Repairs: repairs, Merges: merges, Autopilot: autopilots, Alerts: notices}); err != nil {
			return err
		}
	}
	discoveryContext, stopDiscovery := context.WithCancel(ctx)
	discoveryDone := make(chan struct{})
	go func() { defer close(gitopsDone); _ = promotions.Run(gitopsContext) }()
	go func() { defer close(campaignDone); _ = campaigns.Run(campaignContext) }()
	go func() { defer close(deliveryDone); _ = deliveries.Run(deliveryContext) }()
	go func() { defer close(mergeDone); _ = merges.Run(mergeContext) }()
	go func() { defer close(discoveryDone); _ = discoveries.Run(discoveryContext) }()
	go func() { defer close(autopilotDone); _ = autopilots.Run(autopilotContext) }()
	defer func() { stopDiscovery(); <-discoveryDone }()
	artifactContext, stopArtifacts := context.WithCancel(ctx)
	artifactDone := make(chan struct{})
	go func() { defer close(artifactDone); artifacts.Run(artifactContext) }()
	defer func() { stopArtifacts(); <-artifactDone }()
	srv := &http.Server{Addr: cfg.Address, Handler: app.Router, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	closing := make(chan struct{})
	app.Closing = closing
	srv.RegisterOnShutdown(func() { close(closing) })
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
	shutdown, cancel := context.WithTimeout(context.Background(), model.MaxTurnTimeout+time.Minute)
	defer cancel()
	return srv.Shutdown(shutdown)
}
