package forge

import "context"

type TrainGate struct {
	QueueID        string `json:"queue_id"`
	PipelineID     string `json:"pipeline_id"`
	JobID          string `json:"job_id"`
	HeadSHA        string `json:"head_sha"`
	TargetSHA      string `json:"target_sha"`
	SHA            string `json:"sha"`
	CIConfigSHA256 string `json:"ci_config_sha256"`
	Name           string `json:"name"`
	PublisherID    string `json:"publisher_id"`
	State          string `json:"state"`
	ChecksReady    bool   `json:"checks_ready"`
}

type TrainGateRequest struct {
	RulesHash   string    `json:"rules_hash"`
	Repository  RepoRef   `json:"repository"`
	ChangeID    string    `json:"change_id"`
	Gate        TrainGate `json:"gate"`
	OperationID string    `json:"operation_id"`
}

type ForgeTrainGate interface {
	ReadTrainGate(context.Context, RepoRef, string) (TrainGate, error)
	ReleaseTrainGate(context.Context, TrainGateRequest) (TrainGate, error)
}
