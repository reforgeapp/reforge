package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/reforgeapp/reforge/internal/auth"
	"github.com/reforgeapp/reforge/internal/domain"
	"github.com/reforgeapp/reforge/internal/forge"
)

const trainGateName = "reforge/merge-policy"

type trainGateJob struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Stage        string `json:"stage"`
	Status       string `json:"status"`
	AllowFailure *bool  `json:"allow_failure"`
	Commit       struct {
		ID string `json:"id"`
	} `json:"commit"`
	Pipeline struct {
		ID        int64  `json:"id"`
		ProjectID int64  `json:"project_id"`
		SHA       string `json:"sha"`
	} `json:"pipeline"`
}

func (p *Provider) WithTrainGateAuthorizer(fn func(context.Context, forge.TrainGateRequest) error) *Provider {
	q := *p
	q.authorizeTrain = fn
	return &q
}

func (p *Provider) ReadTrainGate(ctx context.Context, r forge.RepoRef, id string) (forge.TrainGate, error) {
	if !safeID(r.NativeID) || !safeID(id) {
		return forge.TrainGate{}, failure("invalid", "Immutable project and merge request are required")
	}
	actor, err := p.authenticatedActor(ctx)
	if err != nil {
		return forge.TrainGate{}, err
	}
	if p.publishers[trainGateName] == "" || p.publishers[trainGateName] != actor {
		return forge.TrainGate{}, failure("unsupported", "Authenticated actor is not the qualified train-gate publisher")
	}
	change, entry, pipeline, err := p.readTrainSnapshot(ctx, r, id)
	if err != nil {
		var providerErr *domain.ProviderError
		if errors.As(err, &providerErr) && providerErr.Kind == "not_found" {
			if change.State != "opened" && change.State != "open" || change.HeadSHA == "" || change.TargetSHA == "" {
				return forge.TrainGate{}, failure("unsupported", "Merge request is not an open unqueued candidate")
			}
			configHash, configErr := p.verifyTrainConfiguration(ctx, r, change, change.HeadSHA)
			if configErr != nil {
				return forge.TrainGate{}, configErr
			}
			fresh, readErr := p.ReadChange(ctx, r, id)
			if readErr != nil {
				return forge.TrainGate{}, readErr
			}
			if fresh.HeadSHA != change.HeadSHA || fresh.TargetSHA != change.TargetSHA || fresh.State != change.State {
				return forge.TrainGate{}, failure("conflict", "Merge request changed during train inspection")
			}
			return forge.TrainGate{HeadSHA: change.HeadSHA, TargetSHA: change.TargetSHA, CIConfigSHA256: configHash, Name: trainGateName, PublisherID: actor, State: "not_queued"}, nil
		}
		return forge.TrainGate{}, err
	}
	if change.State != "opened" && change.State != "open" || change.Draft {
		return forge.TrainGate{}, failure("unsupported", "Merge request is not an open candidate")
	}
	if entry.Status != "idle" && entry.Status != "fresh" {
		return forge.TrainGate{}, failure("unsupported", "Merge train candidate is not active")
	}
	if pipeline.SHA == change.HeadSHA || !validSHA(pipeline.SHA) || pipeline.Source != "merge_request_event" || !validTrainPipelineRef(pipeline.Ref) || !validTrainPipelineStatus(pipeline.Status) {
		return forge.TrainGate{}, failure("unsupported", "Merge train candidate is not an exact merged result")
	}
	parents, err := p.commitParents(ctx, r, pipeline.SHA)
	if err != nil {
		return forge.TrainGate{}, err
	}
	if len(parents) != 2 || !containsPair(parents, change.HeadSHA, change.TargetSHA) {
		return forge.TrainGate{}, failure("unsupported", "Only the first exact merge-train candidate is qualified")
	}
	if pipeline.ID <= 0 || pipeline.ProjectID != mustIntID(r.NativeID) || pipeline.SHA != entry.Pipeline.SHA {
		return forge.TrainGate{}, failure("identity", "Merge-train pipeline identity changed")
	}
	configHash, err := p.verifyTrainConfiguration(ctx, r, change, pipeline.SHA)
	if err != nil {
		return forge.TrainGate{}, err
	}
	jobs, err := p.pipelineJobs(ctx, r, pipeline.ID)
	if err != nil {
		return forge.TrainGate{}, err
	}
	gateJob, ready, err := qualifyTrainJobs(jobs, pipeline, actor, p.publishers[trainGateName])
	if err != nil {
		return forge.TrainGate{}, err
	}
	change2, entry2, pipeline2, err := p.readTrainSnapshot(ctx, r, id)
	if err != nil {
		return forge.TrainGate{}, err
	}
	if change2.HeadSHA != change.HeadSHA || change2.TargetSHA != change.TargetSHA || entry2.ID != entry.ID || entry2.Status != entry.Status || pipeline2.ID != pipeline.ID || pipeline2.SHA != pipeline.SHA {
		return forge.TrainGate{}, failure("conflict", "Merge-train candidate changed during inspection")
	}
	state := gateJob.Status
	return forge.TrainGate{QueueID: stringID(entry.ID), PipelineID: stringID(pipeline.ID), JobID: stringID(gateJob.ID), HeadSHA: change.HeadSHA, TargetSHA: change.TargetSHA, SHA: pipeline.SHA, CIConfigSHA256: configHash, Name: gateJob.Name, PublisherID: actor, State: state, ChecksReady: ready}, nil
}

func (p *Provider) ReleaseTrainGate(ctx context.Context, request forge.TrainGateRequest) (forge.TrainGate, error) {
	if p.authorizeTrain == nil || !auth.ValidID(request.OperationID) {
		return forge.TrainGate{}, failure("unsupported", "Persisted train-gate authority is required")
	}
	current, err := p.ReadTrainGate(ctx, request.Repository, request.ChangeID)
	if err != nil {
		return forge.TrainGate{}, err
	}
	if !sameTrainGate(current, request.Gate) || current.State != "manual" || !current.ChecksReady {
		return forge.TrainGate{}, failure("conflict", "Train gate is stale or not ready")
	}
	if p.mergeGuard == nil || request.RulesHash == "" {
		return forge.TrainGate{}, failure("unsupported", "Qualified native merge authority is required")
	}
	change, err := p.ReadChange(ctx, request.Repository, request.ChangeID)
	if err != nil {
		return forge.TrainGate{}, err
	}
	if change.HeadSHA != current.HeadSHA || change.TargetSHA != current.TargetSHA {
		return forge.TrainGate{}, failure("conflict", "Merge request source or target changed")
	}
	rules, err := p.ReadEffectiveRules(ctx, request.Repository, change.TargetBranch)
	if err != nil {
		return forge.TrainGate{}, err
	}
	if rules.State != domain.Supported || rules.Hash != request.RulesHash || !rules.RequireQueue || rules.ActorCanBypass {
		return forge.TrainGate{}, failure("conflict", "Native merge rules changed or are bypassable")
	}
	eligibility, err := p.evaluateNative(ctx, request.Repository, request.ChangeID, &current, true)
	if err != nil {
		return forge.TrainGate{}, err
	}
	if eligibility.State != "eligible" || eligibility.HeadSHA != current.HeadSHA || eligibility.TargetSHA != current.TargetSHA {
		return forge.TrainGate{}, failure("conflict", "Native approvals or eligibility changed")
	}
	if err = p.authorizeTrain(ctx, request); err != nil {
		return forge.TrainGate{}, err
	}
	project, err := p.projectSegment(request.Repository)
	if err != nil {
		return forge.TrainGate{}, err
	}
	status, headers, _, err := p.request(ctx, http.MethodPost, []string{"projects", project, "jobs", request.Gate.JobID, "play"}, nil, &requestBody{data: []byte("{}")})
	if err != nil {
		return forge.TrainGate{}, err
	}
	if status < 200 || status >= 300 {
		return forge.TrainGate{}, uncertain(responseError(status, headers))
	}
	final, err := p.ReadTrainGate(ctx, request.Repository, request.ChangeID)
	if err != nil {
		return forge.TrainGate{}, uncertain(err)
	}
	if !sameTrainGate(final, request.Gate) || final.State != "pending" && final.State != "running" && final.State != "success" {
		return forge.TrainGate{}, uncertain(failure("conflict", "Train gate changed after release"))
	}
	return final, nil
}

func (p *Provider) readTrainSnapshot(ctx context.Context, r forge.RepoRef, id string) (forge.Change, trainEntry, pipelineRecord, error) {
	change, err := p.ReadChange(ctx, r, id)
	if err != nil {
		return forge.Change{}, trainEntry{}, pipelineRecord{}, err
	}
	if (change.State != "opened" && change.State != "open") || change.Draft {
		return change, trainEntry{}, pipelineRecord{}, failure("unsupported", "Merge request is not an open candidate")
	}
	var entry trainEntry
	project, err := p.projectSegment(r)
	if err != nil {
		return forge.Change{}, trainEntry{}, pipelineRecord{}, err
	}
	if err = p.read(ctx, []string{"projects", project, "merge_trains", "merge_requests", id}, &entry); err != nil {
		return change, trainEntry{}, pipelineRecord{}, err
	}
	if !validTrain(entry, r, id, change.TargetBranch) || entry.Pipeline == nil {
		return change, trainEntry{}, pipelineRecord{}, failure("unsupported", "Active merge-train pipeline is unavailable")
	}
	if entry.Pipeline.ID <= 0 || entry.Pipeline.ProjectID != mustIntID(r.NativeID) || !validSHA(entry.Pipeline.SHA) {
		return change, trainEntry{}, pipelineRecord{}, failure("identity", "Merge-train entry pipeline identity is incomplete")
	}
	var pipeline pipelineRecord
	if err = p.read(ctx, []string{"projects", project, "pipelines", stringID(entry.Pipeline.ID)}, &pipeline); err != nil {
		return forge.Change{}, trainEntry{}, pipelineRecord{}, err
	}
	if pipeline.ID != entry.Pipeline.ID || pipeline.ProjectID != mustIntID(r.NativeID) || pipeline.SHA != entry.Pipeline.SHA || !validSHA(pipeline.SHA) {
		return change, trainEntry{}, pipelineRecord{}, failure("identity", "Merge-train pipeline identity is incomplete")
	}
	return change, entry, pipeline, nil
}

func (p *Provider) commitParents(ctx context.Context, r forge.RepoRef, sha string) ([]string, error) {
	project, err := p.projectSegment(r)
	if err != nil {
		return nil, err
	}
	var commit struct {
		Parents []string `json:"parent_ids"`
	}
	if err = p.read(ctx, []string{"projects", project, "repository", "commits", sha}, &commit); err != nil {
		return nil, err
	}
	return commit.Parents, nil
}

func (p *Provider) pipelineJobs(ctx context.Context, r forge.RepoRef, id int64) ([]trainGateJob, error) {
	project, err := p.projectSegment(r)
	if err != nil {
		return nil, err
	}
	rows, err := p.pages(ctx, []string{"projects", project, "pipelines", stringID(id), "jobs"}, url.Values{})
	if err != nil {
		return nil, err
	}
	jobs := make([]trainGateJob, 0, len(rows))
	for _, raw := range rows {
		var job trainGateJob
		if json.Unmarshal(raw, &job) != nil || job.ID <= 0 || job.Name == "" || job.AllowFailure == nil {
			return nil, failure("identity", "Pipeline job identity or optionality is incomplete")
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func qualifyTrainJobs(jobs []trainGateJob, pipeline pipelineRecord, actor, publisher string) (trainGateJob, bool, error) {
	var gate trainGateJob
	required := 0
	ready := true
	for _, job := range jobs {
		if job.Pipeline.ID != pipeline.ID || job.Pipeline.ProjectID != pipeline.ProjectID || job.Pipeline.SHA != pipeline.SHA || job.Commit.ID != pipeline.SHA {
			return trainGateJob{}, false, failure("identity", "Pipeline job does not bind to the train candidate")
		}
		if job.Name == trainGateName {
			if gate.ID != 0 || job.Stage != ".post" || *job.AllowFailure || publisher == "" || publisher != actor {
				return trainGateJob{}, false, failure("unsupported", "The Reforge train gate job is not uniquely protected")
			}
			gate = job
			if job.Status != "manual" && job.Status != "pending" && job.Status != "running" && job.Status != "success" {
				return trainGateJob{}, false, failure("unsupported", "The Reforge train gate job has an unsupported state")
			}
			continue
		}
		if !*job.AllowFailure {
			required++
			switch job.Status {
			case "success":
			case "pending", "running", "manual":
				ready = false
			default:
				return trainGateJob{}, false, failure("unsupported", "A required train job has an unsupported or failed state")
			}
		}
	}
	if gate.ID == 0 || required == 0 {
		return trainGateJob{}, false, failure("unsupported", "Required train jobs are incomplete")
	}
	return gate, ready, nil
}

func sameTrainGate(a, b forge.TrainGate) bool {
	return a.QueueID == b.QueueID && a.PipelineID == b.PipelineID && a.JobID == b.JobID && a.HeadSHA == b.HeadSHA && a.TargetSHA == b.TargetSHA && a.SHA == b.SHA && a.CIConfigSHA256 == b.CIConfigSHA256 && a.Name == b.Name && a.PublisherID == b.PublisherID
}

func containsPair(values []string, first, second string) bool {
	return len(values) == 2 && ((values[0] == first && values[1] == second) || (values[0] == second && values[1] == first))
}

func mustIntID(value string) int64 {
	id, _ := strconv.ParseInt(value, 10, 64)
	return id
}

func validTrainPipelineRef(ref string) bool {
	return strings.HasPrefix(ref, "refs/merge-requests/") && strings.HasSuffix(ref, "/train")
}

func validTrainPipelineStatus(status string) bool {
	switch status {
	case "manual", "pending", "running", "success":
		return true
	default:
		return false
	}
}
