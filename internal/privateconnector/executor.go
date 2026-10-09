package privateconnector

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"reforge/internal/auth"
	"reforge/internal/domain"
	"reforge/internal/forge"
	"reforge/internal/forge/gitea"
	"reforge/internal/forge/github"
	"reforge/internal/forge/gitlab"
	"reforge/internal/model"
	"reforge/internal/model/anthropic"
	"reforge/internal/model/compatible"
	"reforge/internal/model/google"
	"reforge/internal/model/openai"
	"reforge/internal/network"
)

type Executor struct {
	Authorize   func(context.Context) error
	Target      Target
	Development bool
}

func validateConnection(c Connection, target Target, development bool) error {
	if !auth.ValidID(c.ID) || c.OrgID != target.OrgID || c.Version < 1 || c.CredentialVersion < 1 || (c.Secret == "" && c.Provider != "compatible") || len(c.Secret) > 32768 || len(c.CAPEM) > 128<<10 {
		return ErrInvalid
	}
	if len(c.CheckPublishers) > 100 {
		return ErrInvalid
	}
	for name, publisher := range c.CheckPublishers {
		if len(name) == 0 || len(name) > 256 || len(publisher) == 0 || len(publisher) > 128 {
			return ErrInvalid
		}
	}
	if p := c.Protection; p != nil {
		if c.Provider != "gitea" || (c.Kind != "forge" && c.Kind != "delivery") || !auth.ValidID(p.ID) || p.ID == c.ID || p.Version < 1 || p.CredentialVersion < 1 || p.Secret == "" || len(p.Secret) > 32768 {
			return ErrInvalid
		}
	}
	validProvider := c.Provider == "gitea" || c.Provider == "gitlab" || c.Provider == "github"
	if c.Kind == "model" {
		validProvider = c.Provider == "openai" || c.Provider == "anthropic" || c.Provider == "google" || c.Provider == "compatible"
		if c.Model == "" || len(c.Model) > 512 || len(c.Profile) > 64 {
			return ErrInvalid
		}
	} else if c.Kind != "" && c.Kind != "forge" && c.Kind != "delivery" {
		return ErrInvalid
	}
	if !validProvider {
		return ErrInvalid
	}
	if c.Route.OrgID != target.OrgID || c.Route.RunnerID != target.RunnerID || c.Route.ConnectionID != c.ID {
		return ErrInvalid
	}
	u, e := url.Parse(c.Endpoint)
	if e != nil || len(c.Endpoint) > 2048 {
		return ErrInvalid
	}
	if u.Scheme != "https" {
		ip := net.ParseIP(u.Hostname())
		if !development || u.Scheme != "http" || ip == nil || !ip.IsLoopback() {
			return ErrInvalid
		}
	}
	if _, e = network.ValidateEndpoint(c.Endpoint, network.Options{RunnerID: target.RunnerID, PrivateRoute: &c.Route, CAPEM: c.CAPEM, Development: development}); e != nil {
		return ErrInvalid
	}
	return nil
}
func (e Executor) Execute(ctx context.Context, grant Grant) Result {
	result := Result{OperationID: grant.Operation.ID}
	fail := func(code string, uncertain bool) Result {
		return Result{OperationID: grant.Operation.ID, Failure: &Failure{Code: code, Uncertain: uncertain}}
	}
	if grant.RunnerVersion < 1 || grant.Target != e.Target || !auth.ValidID(grant.ID) || !auth.ValidID(grant.AuthorityID) || len(grant.ResultCapability) != 43 || grant.Operation.validate() != nil || validateConnection(grant.Connection, e.Target, e.Development) != nil {
		return fail("invalid", false)
	}
	remaining := time.Until(grant.executionDeadline)
	if remaining <= 0 || remaining > grant.Operation.MaximumTTL() || grant.TimeoutMS < 1 || grant.TimeoutMS > grant.Operation.MaximumTTL().Milliseconds() || !grant.ExpiresAt.After(time.Now()) {
		return fail("expired", false)
	}
	ctx, cancel := context.WithDeadline(ctx, grant.executionDeadline)
	defer cancel()
	client, err := network.NewClient(grant.Connection.Endpoint, network.Options{RunnerID: e.Target.RunnerID, PrivateRoute: &grant.Connection.Route, CAPEM: grant.Connection.CAPEM, Development: e.Development})
	if err != nil {
		return fail("invalid", false)
	}
	defer client.CloseIdleConnections()
	if grant.Connection.Kind == "model" {
		if grant.Operation.Kind == ModelTurn {
			if grant.Operation.Turn.Model != grant.Connection.Model {
				return fail("invalid", false)
			}
			scoped, err := network.WithTimeout(client, grant.Operation.MaximumTTL())
			if err != nil {
				return fail("invalid", false)
			}
			defer scoped.CloseIdleConnections()
			client = scoped
		}
		var provider model.ModelProvider
		cfg := model.Config{Endpoint: grant.Connection.Endpoint, APIKey: grant.Connection.Secret, Model: grant.Connection.Model, Profile: grant.Connection.Profile, Client: client}
		switch grant.Connection.Provider {
		case "openai":
			provider, err = openai.New(cfg)
		case "anthropic":
			provider, err = anthropic.New(cfg)
		case "google":
			provider, err = google.New(cfg)
		case "compatible":
			provider, err = compatible.New(cfg)
		}
		if err == nil {
			switch grant.Operation.Kind {
			case ModelProbe:
				var caps model.Capabilities
				caps, err = provider.Probe(ctx)
				result.ModelCapabilities = &caps
			case ModelList:
				result.Models, err = provider.ListModels(ctx)
			case ModelTurn:
				var turn model.TurnResult
				turn, err = model.CollectTurn(ctx, provider, *grant.Operation.Turn)
				result.Turn = &turn
			default:
				return fail("unsupported", false)
			}
		}
	} else {
		var provider forge.Provider
		var transport forge.HTTPClient = client
		if grant.Operation.Mutation() {
			if e.Authorize == nil {
				return fail("forbidden", false)
			}
			transport = GuardHTTP(client, e.Authorize)
		}
		cfg := forge.Config{OrgID: e.Target.OrgID, ConnectionID: grant.Connection.ID, BaseURL: grant.Connection.Endpoint, Token: grant.Connection.Secret, Client: transport}
		switch grant.Connection.Provider {
		case "gitea":
			var p *gitea.Provider
			p, err = gitea.New(cfg)
			if err == nil {
				p = p.WithCheckPublishers(grant.Connection.CheckPublishers)
			}
			if err == nil && grant.Connection.Protection != nil {
				readerConfig := cfg
				readerConfig.ConnectionID = grant.Connection.Protection.ID
				readerConfig.Token = grant.Connection.Protection.Secret
				readerConfig.Client = ProtectionHTTP(client)
				var reader *gitea.Provider
				reader, err = gitea.New(readerConfig)
				if err == nil {
					p, err = p.WithProtectionReader(reader)
				}
			}
			provider = p
		case "gitlab":
			provider, err = gitlab.New(cfg)

		case "github":
			var p *github.Provider
			if grant.Connection.AuthKind == "github_app" {
				p, err = github.NewApp(ctx, cfg, github.AppConfig{AppID: grant.Connection.AppID, InstallationID: grant.Connection.InstallationID, PrivateKeyPEM: []byte(grant.Connection.Secret)})
			} else {
				p, err = github.New(cfg)
			}
			if err == nil {
				u, parseErr := url.Parse(grant.Connection.Endpoint)
				if parseErr != nil {
					return fail("configuration", false)
				}
				switch u.Path {
				case "", "/":
					u.Path = "/graphql"
				case "/api/v3", "/api/v3/":
					u.Path = "/api/graphql"
				default:
					return fail("configuration", false)
				}
				graph, graphErr := network.NewClient(u.String(), network.Options{RunnerID: e.Target.RunnerID, PrivateRoute: &grant.Connection.Route, CAPEM: grant.Connection.CAPEM, Development: e.Development})
				if graphErr != nil {
					return fail("configuration", false)
				}
				defer graph.CloseIdleConnections()
				var graphTransport forge.HTTPClient = graph
				if grant.Operation.Mutation() {
					graphTransport = GuardHTTP(graph, e.Authorize)
				}
				provider, err = p.WithGraphQL(u.String(), graphTransport)
			}

		}
		if err == nil {
			result, err = ReadForge(ctx, provider, grant.Operation)
		}
	}
	if err != nil {
		if p, ok := err.(*domain.ProviderError); ok {
			return Result{OperationID: grant.Operation.ID, Failure: &Failure{Code: p.Kind, Uncertain: p.Uncertain, RetryAfterMS: min(max(p.RetryAfter.Milliseconds(), 0), 3600000)}}
		}
		return fail("provider", false)
	}
	raw, err := json.Marshal(result)
	if err != nil || len(raw) > MaxResponse {
		return fail("response", false)
	}
	if credentialEcho(raw, grant.Connection) || result.File != nil && grant.Connection.Secret != "" && bytes.Contains(result.File.Content, []byte(grant.Connection.Secret)) {
		return fail("credential_echo", false)
	}
	return result
}
func containsSecret(data []byte, secret string) bool {
	if secret == "" {
		return false
	}
	quoted, _ := json.Marshal(secret)
	for _, form := range []string{secret, string(quoted[1 : len(quoted)-1]), url.QueryEscape(secret), url.PathEscape(secret), base64.StdEncoding.EncodeToString([]byte(secret)), base64.RawURLEncoding.EncodeToString([]byte(secret))} {
		if form != "" && bytes.Contains(data, []byte(form)) {
			return true
		}
	}
	return false
}
func safeControllerURL(endpoint string, development bool) (string, error) {
	u, e := url.Parse(endpoint)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return "", ErrInvalid
	}
	if u.Scheme != "https" {
		ip := net.ParseIP(u.Hostname())
		if u.Scheme != "http" || !development || ip == nil || !ip.IsLoopback() {
			return "", ErrInvalid
		}
	}
	return strings.TrimSuffix(u.String(), "/"), nil
}

func ReadForge(ctx context.Context, provider forge.Provider, op Operation) (Result, error) {
	result := Result{OperationID: op.ID}
	if closer, ok := provider.(interface{ CloseIdleConnections() }); ok {
		defer closer.CloseIdleConnections()
	}
	if err := op.validate(); err != nil {
		return result, err
	}
	var err error
	switch op.Kind {
	case ForgePipelineCancel:
		provider = BindForgeOperation(provider, op)
		control, ok := provider.(forge.ForgePipelineControl)
		if !ok {
			return result, ErrUnsupported
		}
		var value forge.DeploymentStatus
		value, err = control.CancelPipeline(ctx, *op.Pipeline)
		result.Deployment = &value

	case ForgePipelineInspect:
		provider = BindForgeOperation(provider, op)
		var gates forge.DeploymentGates
		gates, err = forge.InspectPipeline(ctx, provider, *op.Pipeline)
		result.DeploymentGates = &gates

	case ForgeDeliveryWorkflows:
		result.Workflows, err = provider.ListAllowedWorkflows(ctx, op.Delivery.Repository)
	case ForgeDeliveryGates:
		var gates forge.DeploymentGates
		gates, err = provider.ReadDeploymentGates(ctx, op.Delivery.Repository, op.Delivery.Environment)
		result.DeploymentGates = &gates
	case ForgeDeliveryStatus:
		var value forge.DeploymentStatus
		value, err = provider.ReadDeploymentStatus(ctx, op.Delivery.Repository, op.Delivery.RunID)
		result.Deployment = &value
	case ForgePipelineObserve, ForgePipelineTrigger, ForgePipelineRecover:
		provider = BindForgeOperation(provider, op)
		var value forge.DeploymentStatus
		if op.Kind == ForgePipelineRecover {
			value, err = provider.RequestAllowedRecovery(ctx, *op.Pipeline)
		} else {
			value, err = provider.TriggerOrObservePipeline(ctx, *op.Pipeline)
		}
		result.Deployment = &value
	case ForgeReadTrainGate, ForgeReleaseTrainGate:
		provider = BindForgeOperation(provider, op)
		train, ok := provider.(forge.ForgeTrainGate)
		if !ok {
			return result, ErrUnsupported
		}
		var value forge.TrainGate
		if op.Kind == ForgeReadTrainGate {
			value, err = train.ReadTrainGate(ctx, op.Change.Repository, op.Change.ChangeID)
		} else {
			value, err = train.ReleaseTrainGate(ctx, *op.TrainGate)
		}
		result.TrainGate = &value
	case ForgeReadExecutionCheck, ForgeWriteExecutionCheck:
		checks, ok := provider.(forge.ForgeExecutionChecks)
		if !ok {
			return result, ErrUnsupported
		}
		var value forge.ExecutionCheck
		if op.Kind == ForgeReadExecutionCheck {
			value, err = checks.ReadExecutionCheck(ctx, op.ExecutionCheck.Repository, op.ExecutionCheck.CheckID)
		} else {
			value, err = checks.WriteExecutionCheck(ctx, *op.ExecutionCheck)
		}
		result.ExecutionCheck = &value
	case ForgeQueueState:
		var queue forge.QueueState
		queue, err = provider.ReadQueueState(ctx, op.Change.Repository, op.Change.ChangeID)
		result.Queue = &queue
	case ForgeCloseChange:
		closer, ok := provider.(forge.ForgeChangeCloser)
		if !ok {
			return result, ErrUnsupported
		}
		var change forge.Change
		change, err = closer.CloseChange(ctx, *op.Close)
		result.Change = &change
	case ForgeCommentChange:
		commenter, ok := provider.(forge.ForgeChangeCommenter)
		if !ok {
			return result, ErrUnsupported
		}
		var change forge.Change
		change, err = commenter.CommentChange(ctx, *op.Comment)
		result.Change = &change
	case ForgeCancelQueue:
		control, ok := provider.(forge.ForgeQueueControl)
		if !ok {
			return result, ErrUnsupported
		}
		var queue forge.QueueState
		queue, err = control.CancelNativeQueue(ctx, *op.CancelQueue)
		result.Queue = &queue
	case ForgeMergeInspect:
		provider = BindForgeOperation(provider, op)
		result.MergeEvidence, err = inspectMerge(ctx, provider, *op.Change)
	case ForgeQueueInspect:
		provider = BindForgeOperation(provider, op)
		result.MergeEvidence, err = inspectQueue(ctx, provider, *op.Change)
	case ForgeMerge:
		provider = BindForgeOperation(provider, op)
		var merged forge.MergeResult
		merged, err = provider.RequestNativeMergeOrQueue(ctx, *op.Merge)
		result.Merge = &merged
	case ForgeMergeResult:
		var merged forge.MergeResult
		merged, err = provider.ReadMergeResult(ctx, op.Change.Repository, op.Change.ChangeID)
		result.Merge = &merged
	case ForgeCommitProof:
		reader, ok := provider.(forge.ForgeCommits)
		if !ok {
			return result, ErrUnsupported
		}
		var proof forge.CommitProof
		proof, err = reader.ReadCommitProof(ctx, op.Commit.Repository, op.Commit.CommitSHA)
		result.Commit = &proof
	case ForgeRefreshBranch:
		provider = BindForgeOperation(provider, op)
		refresher, ok := provider.(forge.ForgeBranchRefresher)
		if !ok {
			return result, ErrUnsupported
		}
		err = refresher.RefreshAppBranch(ctx, *op.Refresh)
	case ForgeUpdateBranch:
		provider = BindForgeOperation(provider, op)
		result.SHA, err = provider.UpdateAppBranch(ctx, *op.Branch)
	case ForgeCreateChange:
		provider = BindForgeOperation(provider, op)
		var change forge.Change
		change, err = provider.CreateChange(ctx, *op.Create)
		result.Change = &change
	case ForgeFindChange:
		result.Change, err = provider.FindChangeByOperation(ctx, op.Find.Repository, op.Find.OperationID, op.Find.HeadBranch, op.Find.TargetBranch)
	case ForgeSourceManifest:
		reader, ok := provider.(forge.ForgeSource)
		if !ok {
			return result, ErrUnsupported
		}
		var v forge.SourceManifest
		v, err = reader.ReadSourceManifest(ctx, op.Source.Repository, op.Source.CommitSHA)
		result.Manifest = &v
	case ForgeProbe:
		var v forge.Capabilities
		v, err = provider.ProbeCapabilities(ctx)
		result.Capabilities = &v
	case ForgeInventory:
		var v domain.Page[forge.Repository]
		v, err = provider.ListRepositories(ctx, forge.InventoryRequest{Namespace: op.Inventory.Namespace, Cursor: op.Inventory.Cursor, Limit: op.Inventory.Limit})
		result.Inventory = &v
	case ForgeRepository:
		var v forge.Repository
		v, err = provider.GetRepository(ctx, op.Repository.Repository)
		result.Repository = &v
	case ForgeResolveRef:
		result.SHA, err = provider.ResolveRef(ctx, op.Ref.Repository, op.Ref.Ref)
	case ForgeReadFile:
		var v forge.File
		v, err = provider.ReadFileAtRef(ctx, op.File.Repository, op.File.Path, op.File.CommitSHA)
		result.File = &v
	case ForgeReadFiles:
		result.Files, err = readFiles(ctx, provider, *op.Files)
	case ForgeBehind:
		freshness, ok := provider.(forge.ForgeFreshness)
		if !ok {
			return result, ErrUnsupported
		}
		var behind int
		behind, err = freshness.Behind(ctx, op.Compare.Repository, op.Compare.Base, op.Compare.Head)
		result.Behind = &behind
	case ForgeCheckLog:
		reader, ok := provider.(forge.CheckLogReader)
		if !ok {
			return result, ErrUnsupported
		}
		result.Log, err = reader.ReadCheckLog(ctx, op.CheckLog.Repository, op.CheckLog.CheckID)
	case ForgeReadChange:
		var v forge.Change
		v, err = provider.ReadChange(ctx, op.Change.Repository, op.Change.ChangeID)
		result.Change = &v
	case ForgeChecks:
		result.Checks, err = provider.ListChecks(ctx, op.Checks.Repository, op.Checks.CommitSHA)
	case ForgeIssues:
		reader, ok := provider.(forge.IssueReader)
		if !ok {
			return result, ErrUnsupported
		}
		result.Issues, err = reader.ListIssues(ctx, op.Repository.Repository)
	case ForgePermissionsURL:
		linker, ok := provider.(forge.PermissionLinker)
		if !ok {
			return result, ErrUnsupported
		}
		result.Link, err = linker.PermissionsURL(ctx)
	case ForgeAdvisories:
		reader, ok := provider.(forge.AdvisoryReader)
		if !ok {
			return result, ErrUnsupported
		}
		result.Advisories, err = reader.ListAdvisories(ctx, op.Repository.Repository)
	case ForgeApprovals:
		result.Approvals, err = provider.ReadApprovals(ctx, op.Change.Repository, op.Change.ChangeID)
	case ForgeReconcileChanges:
		var v domain.Page[forge.Change]
		v, err = provider.ReconcileChanges(ctx, op.Changes.Repository, op.Changes.Cursor)
		result.Changes = &v
	default:
		return result, ErrUnsupported
	}
	return result, err
}

func ContainsSecret(data []byte, secret string) bool { return containsSecret(data, secret) }

func readFiles(ctx context.Context, provider forge.Provider, in FilesArgs) ([]forge.File, error) {
	out := []forge.File{}
	total := 0
	for start := 0; start < len(in.Paths) && total < batchBytes; start += 8 {
		paths := in.Paths[start:min(start+8, len(in.Paths))]
		files := make([]forge.File, len(paths))
		errs := make([]error, len(paths))
		var wg sync.WaitGroup
		for i, path := range paths {
			wg.Add(1)
			go func() {
				defer wg.Done()
				files[i], errs[i] = provider.ReadFileAtRef(ctx, in.Repository, path, in.CommitSHA)
			}()
		}
		wg.Wait()
		for i, file := range files {
			if errs[i] != nil {
				return nil, errs[i]
			}
			if len(out) > 0 && total+len(file.Content) > batchBytes {
				return out, nil
			}
			out = append(out, file)
			total += len(file.Content)
		}
	}
	return out, nil
}
