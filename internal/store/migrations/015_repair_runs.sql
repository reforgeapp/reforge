CREATE TABLE repair_runs (
 org_id uuid NOT NULL,
 task_id uuid NOT NULL,
 repository_id uuid NOT NULL,
 finding_id uuid NOT NULL,
 requested_by uuid NOT NULL,
 finding_version bigint NOT NULL,
 finding_digest text NOT NULL,
 context jsonb NOT NULL,
 report jsonb,
 state text NOT NULL DEFAULT 'queued' CHECK(state IN ('queued','validated','handoff','publishing','published','uncertain')),
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(org_id,task_id),
 FOREIGN KEY(org_id,task_id,repository_id) REFERENCES workflow_tasks(org_id,id,repository_id),
 FOREIGN KEY(org_id,finding_id) REFERENCES maintenance_findings(org_id,id)
);
ALTER TABLE repair_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE repair_runs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON repair_runs USING(org_id=nullif(current_setting('reforge.org_id',true),'')::uuid) WITH CHECK(org_id=nullif(current_setting('reforge.org_id',true),'')::uuid);
CREATE INDEX repair_runs_repository ON repair_runs(org_id,repository_id,created_at);
