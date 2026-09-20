CREATE TABLE model_turns (
 org_id uuid NOT NULL,
 id uuid NOT NULL,
 task_id uuid NOT NULL,
 attempt_id uuid NOT NULL,
 repository_id uuid NOT NULL,
 reservation_id uuid NOT NULL,
 request_hash text NOT NULL CHECK(length(request_hash)=64),
 state text NOT NULL CHECK(state IN ('dispatched','complete','unknown')),
 result_envelope jsonb,
 usage jsonb,
 created_at timestamptz NOT NULL DEFAULT now(),
 completed_at timestamptz,
 PRIMARY KEY(org_id,id),
 FOREIGN KEY(org_id,task_id,repository_id) REFERENCES workflow_tasks(org_id,id,repository_id),
 FOREIGN KEY(org_id,attempt_id) REFERENCES workflow_attempts(org_id,id),
 FOREIGN KEY(org_id,reservation_id) REFERENCES budget_reservations(org_id,id)
);
ALTER TABLE model_turns ENABLE ROW LEVEL SECURITY;
ALTER TABLE model_turns FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON model_turns USING(org_id=nullif(current_setting('reforge.org_id',true),'')::uuid) WITH CHECK(org_id=nullif(current_setting('reforge.org_id',true),'')::uuid);
CREATE INDEX model_turns_history ON model_turns(org_id,task_id,created_at);
