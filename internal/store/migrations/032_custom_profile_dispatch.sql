ALTER TABLE custom_profile_runs
 ADD COLUMN attempt_id uuid,
 ADD COLUMN repository_id uuid,
 ADD COLUMN events jsonb NOT NULL DEFAULT '[]'::jsonb,
 ADD COLUMN completed_at timestamptz;

ALTER TABLE custom_profile_runs
 ADD CONSTRAINT custom_profile_runs_attempt_unique UNIQUE(org_id,task_id,attempt_id);

ALTER TABLE custom_profile_runs
 ADD CONSTRAINT custom_profile_runs_task_fk FOREIGN KEY(org_id,task_id) REFERENCES workflow_tasks(org_id,id);

CREATE INDEX custom_profile_runs_dispatch ON custom_profile_runs(org_id,profile_id,state) WHERE state IN ('queued','running');
