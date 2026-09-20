package gitlab

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"reforge/internal/domain"
	"reforge/internal/forge"
)

func gateJob(id int64, name, status, sha string, pipeline int64, allow bool) trainGateJob {
	job := trainGateJob{ID: id, Name: name, Status: status}
	if name == trainGateName {
		job.Stage = ".post"
	}
	job.AllowFailure = &allow
	job.Commit.ID = sha
	job.Pipeline.ID = pipeline
	job.Pipeline.ProjectID = 17
	job.Pipeline.SHA = sha
	return job
}

func TestQualifyTrainJobsBindsCandidateAndOptionalJobs(t *testing.T) {
	sha := "3333333333333333333333333333333333333333"
	pipeline := pipelineRecord{ID: 7, ProjectID: 17, SHA: sha, Source: "merge_request_event"}
	optional := true
	jobs := []trainGateJob{
		gateJob(1, trainGateName, "manual", sha, 7, false),
		gateJob(2, "test", "success", sha, 7, false),
		gateJob(3, "lint", "failed", sha, 7, optional),
	}
	gate, ready, err := qualifyTrainJobs(jobs, pipeline, "81", "81")
	if err != nil || !ready || gate.ID != 1 {
		t.Fatalf("gate=%+v ready=%v err=%v", gate, ready, err)
	}
}

func TestQualifyTrainJobsReportsPendingRequiredCheck(t *testing.T) {
	sha := "3333333333333333333333333333333333333333"
	pipeline := pipelineRecord{ID: 7, ProjectID: 17, SHA: sha}
	j := gateJob(2, "test", "pending", sha, 7, false)
	gate, ready, err := qualifyTrainJobs([]trainGateJob{gateJob(1, trainGateName, "manual", sha, 7, false), j}, pipeline, "81", "81")
	if err != nil || ready || gate.ID != 1 {
		t.Fatalf("gate=%+v ready=%v err=%v", gate, ready, err)
	}
}

func TestReadAndReleaseTrainGateHTTPContract(t *testing.T) {
	fixture := newTrainGateFixture()
	p, err := New(forge.Config{OrgID: "org", ConnectionID: "connection", BaseURL: "https://gitlab.example", Token: "token", Client: fixture})
	if err != nil {
		t.Fatal(err)
	}
	p = p.WithCheckPublishers(map[string]string{trainGateName: "81"}).WithMergeGuard(func(context.Context, forge.MergeRequest, forge.Change, forge.Rules) error { return nil }).WithTrainGateAuthorizer(func(context.Context, forge.TrainGateRequest) error {
		fixture.authorized++
		return nil
	})
	gate, err := p.ReadTrainGate(context.Background(), forge.RepoRef{NativeID: "17", FullName: "group/repo"}, "9")
	if err != nil || gate.State != "manual" || !gate.ChecksReady || gate.JobID != "701" || gate.SHA != fixture.candidate {
		t.Fatalf("gate=%+v err=%v", gate, err)
	}
	rules, err := p.ReadEffectiveRules(context.Background(), forge.RepoRef{NativeID: "17", FullName: "group/repo"}, "main")
	if err != nil {
		t.Fatal(err)
	}
	gate, err = p.ReleaseTrainGate(context.Background(), forge.TrainGateRequest{RulesHash: rules.Hash, Repository: forge.RepoRef{NativeID: "17", FullName: "group/repo"}, ChangeID: "9", Gate: gate, OperationID: domain.NewID()})
	if err != nil || gate.State != "pending" || fixture.plays != 1 || fixture.authorized != 1 {
		t.Fatalf("released gate=%+v plays=%d auth=%d err=%v", gate, fixture.plays, fixture.authorized, err)
	}
	if fixture.jobsPages != 6 {
		t.Fatalf("expected paginated job reads, got %d", fixture.jobsPages)
	}
}

func TestReadTrainGateAbsentNativeTrainVerifiesUnqueuedCandidate(t *testing.T) {
	fixture := newTrainGateFixture()
	fixture.trainAbsent = true
	p, err := New(forge.Config{OrgID: "org", ConnectionID: "connection", BaseURL: "https://gitlab.example", Token: "token", Client: fixture})
	if err != nil {
		t.Fatal(err)
	}
	p = p.WithCheckPublishers(map[string]string{trainGateName: "81"})
	gate, err := p.ReadTrainGate(context.Background(), forge.RepoRef{NativeID: "17", FullName: "group/repo"}, "9")
	if err != nil || gate.State != "not_queued" || gate.HeadSHA != fixture.head || gate.SHA != "" {
		t.Fatalf("gate=%+v err=%v", gate, err)
	}
}

func TestReleaseTrainGateLostResponseIsUncertain(t *testing.T) {
	fixture := newTrainGateFixture()
	p, err := New(forge.Config{OrgID: "org", ConnectionID: "connection", BaseURL: "https://gitlab.example", Token: "token", Client: fixture})
	if err != nil {
		t.Fatal(err)
	}
	p = p.WithCheckPublishers(map[string]string{trainGateName: "81"}).WithMergeGuard(func(context.Context, forge.MergeRequest, forge.Change, forge.Rules) error { return nil }).WithTrainGateAuthorizer(func(context.Context, forge.TrainGateRequest) error { return nil })
	gate, err := p.ReadTrainGate(context.Background(), forge.RepoRef{NativeID: "17", FullName: "group/repo"}, "9")
	if err != nil {
		t.Fatal(err)
	}
	rules, err := p.ReadEffectiveRules(context.Background(), forge.RepoRef{NativeID: "17", FullName: "group/repo"}, "main")
	if err != nil {
		t.Fatal(err)
	}
	fixture.lostPlay = true
	_, err = p.ReleaseTrainGate(context.Background(), forge.TrainGateRequest{RulesHash: rules.Hash, Repository: forge.RepoRef{NativeID: "17", FullName: "group/repo"}, ChangeID: "9", Gate: gate, OperationID: domain.NewID()})
	var providerErr *domain.ProviderError
	if !errors.As(err, &providerErr) || !providerErr.Uncertain {
		t.Fatalf("lost response err=%v", err)
	}
}

func TestReleaseTrainGateRejectsRulesOrApprovalDrift(t *testing.T) {
	for _, drift := range []string{"rules", "approval"} {
		t.Run(drift, func(t *testing.T) {
			fixture := newTrainGateFixture()
			p, err := New(forge.Config{OrgID: "org", ConnectionID: "connection", BaseURL: "https://gitlab.example", Token: "token", Client: fixture})
			if err != nil {
				t.Fatal(err)
			}
			p = p.WithCheckPublishers(map[string]string{trainGateName: "81"}).WithMergeGuard(func(context.Context, forge.MergeRequest, forge.Change, forge.Rules) error { return nil }).WithTrainGateAuthorizer(func(context.Context, forge.TrainGateRequest) error { return nil })
			gate, err := p.ReadTrainGate(context.Background(), forge.RepoRef{NativeID: "17", FullName: "group/repo"}, "9")
			if err != nil {
				t.Fatal(err)
			}
			rules, err := p.ReadEffectiveRules(context.Background(), forge.RepoRef{NativeID: "17", FullName: "group/repo"}, "main")
			if err != nil {
				t.Fatal(err)
			}
			if drift == "rules" {
				fixture.driftRules = true
			} else {
				fixture.driftApproval = true
			}
			_, err = p.ReleaseTrainGate(context.Background(), forge.TrainGateRequest{RulesHash: rules.Hash, Repository: forge.RepoRef{NativeID: "17", FullName: "group/repo"}, ChangeID: "9", Gate: gate, OperationID: domain.NewID()})
			if err == nil || fixture.plays != 0 {
				t.Fatalf("drift accepted err=%v plays=%d", err, fixture.plays)
			}
		})
	}
}

type trainGateFixture struct {
	head, target, candidate   string
	status                    string
	trainAbsent, lostPlay     bool
	driftRules, driftApproval bool
	plays, authorized         int
	jobsPages                 int
}

func newTrainGateFixture() *trainGateFixture {
	return &trainGateFixture{head: strings.Repeat("1", 40), target: strings.Repeat("2", 40), candidate: strings.Repeat("3", 40), status: "manual"}
}

func (f *trainGateFixture) Do(req *http.Request) (*http.Response, error) {
	path := strings.TrimPrefix(req.URL.Path, "/api/v4")
	if req.Method == http.MethodPost && strings.HasSuffix(path, "/jobs/701/play") {
		f.plays++
		if f.lostPlay {
			return nil, io.ErrUnexpectedEOF
		}
		f.status = "pending"
		return jsonResponse(200, map[string]any{"id": 701}), nil
	}
	switch {
	case path == "/user":
		return jsonResponse(200, map[string]any{"id": 81, "is_admin": false}), nil
	case path == "/projects/17/merge_requests/9":
		return jsonResponse(200, map[string]any{"iid": 9, "project_id": 17, "source_project_id": 17, "target_project_id": 17, "sha": f.head, "diff_refs": map[string]string{"head_sha": f.head, "start_sha": f.target}, "source_branch": "feature", "target_branch": "main", "state": "opened", "draft": false, "work_in_progress": false, "detailed_merge_status": "mergeable", "has_conflicts": false, "blocking_discussions_resolved": true}), nil
	case path == "/projects/17/repository/branches/main":
		return jsonResponse(200, map[string]any{"name": "main", "protected": true, "can_push": false, "commit": map[string]string{"id": f.target}}), nil
	case path == "/projects/17/merge_trains/merge_requests/9":
		if f.trainAbsent {
			return jsonResponse(404, nil), nil
		}
		return jsonResponse(200, map[string]any{"id": 91, "status": "idle", "target_branch": "main", "merge_request": map[string]any{"iid": 9, "project_id": 17}, "pipeline": map[string]any{"id": 17, "project_id": 17, "sha": f.candidate}}), nil
	case path == "/projects/17/pipelines/17":
		return jsonResponse(200, map[string]any{"id": 17, "project_id": 17, "sha": f.candidate, "source": "merge_request_event", "ref": "refs/merge-requests/9/train", "status": "running"}), nil
	case path == "/projects/17/repository/commits/"+f.candidate:
		return jsonResponse(200, map[string]any{"parent_ids": []string{f.head, f.target}}), nil
	case path == "/projects/17/repository/files/.gitlab-ci.yml/raw":
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(trainConfigYAML))}, nil
	case path == "/projects/17":
		train := true
		if f.driftRules {
			train = false
		}
		return jsonResponse(200, map[string]any{"id": 17, "ci_config_path": nil, "merge_method": "merge", "squash_option": "default_off", "only_allow_merge_if_pipeline_succeeds": true, "allow_merge_on_skipped_pipeline": false, "only_allow_merge_if_all_discussions_are_resolved": true, "merge_trains_enabled": train, "merge_pipelines_enabled": train, "merge_trains_skip_train_allowed": false, "merge_train_enforcement": "enforce_for_all_users", "automatic_rebase_enabled": false, "only_allow_merge_if_all_status_checks_passed": false}), nil
	case path == "/projects/17/members/all/81":
		return jsonResponse(200, map[string]any{"access_level": 30}), nil
	case path == "/projects/17/protected_branches":
		return jsonResponse(200, []any{map[string]any{"id": 1, "name": "*", "allow_force_push": false, "code_owner_approval_required": false}}), nil
	case path == "/projects/17/approvals":
		return jsonResponse(200, map[string]any{"reset_approvals_on_push": true, "disable_overriding_approvers_per_merge_request": true}), nil
	case path == "/projects/17/approval_rules":
		return jsonResponse(200, []any{map[string]any{"id": 1, "name": "review", "rule_type": "regular", "approvals_required": 0, "contains_hidden_groups": false, "protected_branches": []any{}}}), nil
	case path == "/projects/17/merge_requests/9/approval_state":
		overwritten := false
		if f.driftApproval {
			overwritten = true
		}
		return jsonResponse(200, map[string]any{"approval_rules_overwritten": overwritten, "rules": []any{}}), nil
	case path == "/projects/17/repository/commits/"+f.candidate+"/statuses":
		return jsonResponse(200, []any{}), nil
	case path == "/projects/17/protected_environments":
		return jsonResponse(200, []any{map[string]any{"name": trainEnvironment, "required_approval_count": 0, "approval_rules": []any{}, "deploy_access_levels": []any{map[string]any{"id": 81, "user_id": 81, "group_id": nil, "access_level": 0}}}}), nil
	case path == "/projects/17/pipelines/17/jobs":
		f.jobsPages++
		if req.URL.Query().Get("page") == "2" {
			return jsonResponse(200, []any{}), nil
		}
		response := jsonResponse(200, []any{
			map[string]any{"id": 701, "name": trainGateName, "stage": ".post", "status": f.status, "allow_failure": false, "commit": map[string]string{"id": f.candidate}, "pipeline": map[string]any{"id": 17, "project_id": 17, "sha": f.candidate}},
			map[string]any{"id": 702, "name": "tests", "stage": "test", "status": "success", "allow_failure": false, "commit": map[string]string{"id": f.candidate}, "pipeline": map[string]any{"id": 17, "project_id": 17, "sha": f.candidate}},
		})
		response.Header.Set("X-Next-Page", "2")
		return response, nil
	default:
		return jsonResponse(404, nil), nil
	}
}

const trainConfigYAML = `reforge/merge-policy:
  stage: .post
  allow_failure: false
  inherit: false
  before_script: []
  after_script: []
  script: ["true"]
  environment:
    name: reforge-merge-policy
  rules:
    - if: '$CI_MERGE_REQUEST_EVENT_TYPE == "merge_train"'
      when: manual
`

func TestQualifyTrainJobsRejectsDuplicateOrWrongCandidate(t *testing.T) {
	sha := "3333333333333333333333333333333333333333"
	pipeline := pipelineRecord{ID: 7, ProjectID: 17, SHA: sha, Source: "merge_request_event"}
	allow := false
	cases := []struct {
		name string
		jobs []trainGateJob
	}{
		{name: "duplicate", jobs: []trainGateJob{gateJob(1, trainGateName, "manual", sha, 7, allow), gateJob(2, trainGateName, "manual", sha, 7, allow)}},
		{name: "wrong-pipeline", jobs: []trainGateJob{gateJob(1, trainGateName, "manual", "4444444444444444444444444444444444444444", 8, allow)}},
		{name: "failed-required", jobs: []trainGateJob{gateJob(1, trainGateName, "manual", sha, 7, allow), gateJob(2, "test", "failed", sha, 7, allow)}},
		{name: "retry-gate", jobs: []trainGateJob{gateJob(1, trainGateName, "failed", sha, 7, allow), gateJob(2, "test", "success", sha, 7, allow)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := qualifyTrainJobs(tc.jobs, pipeline, "81", "81"); err == nil {
				t.Fatal("unsafe train jobs accepted")
			}
		})
	}
}

func TestReleaseTrainGateRequiresAuthority(t *testing.T) {
	if _, err := (&Provider{}).ReleaseTrainGate(nil, forge.TrainGateRequest{OperationID: "op"}); err == nil {
		t.Fatal("release without authorizer accepted")
	}
}

func TestSameTrainGateRejectsIdentityDrift(t *testing.T) {
	base := forge.TrainGate{QueueID: "9", PipelineID: "7", JobID: "1", HeadSHA: "111", TargetSHA: "222", SHA: "333", CIConfigSHA256: "hash", Name: trainGateName, PublisherID: "81"}
	for name, mutate := range map[string]func(*forge.TrainGate){
		"pipeline":  func(g *forge.TrainGate) { g.PipelineID = "8" },
		"candidate": func(g *forge.TrainGate) { g.SHA = "444" },
		"config":    func(g *forge.TrainGate) { g.CIConfigSHA256 = "other" },
		"publisher": func(g *forge.TrainGate) { g.PublisherID = "82" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := base
			mutate(&changed)
			if sameTrainGate(base, changed) {
				t.Fatal("identity drift accepted")
			}
		})
	}
}
