CREATE TABLE workflow_tasks (
 org_id uuid NOT NULL REFERENCES organisations(id),
 id uuid NOT NULL,
 repository_id uuid NOT NULL,
 operation_id uuid NOT NULL,
 idempotency_key text NOT NULL CHECK(length(idempotency_key) BETWEEN 1 AND 200),
 request_hash text NOT NULL,
 recipe text NOT NULL,
 recipe_version text NOT NULL,
 target_branch text NOT NULL,
 model_connection_id uuid,
 campaign_id uuid,
 runner_pool_id uuid,
 policy_hash text NOT NULL,
 starting_policy_hash text NOT NULL,
 state text NOT NULL CHECK(state IN ('queued','reproducing','planning','repairing','validating','publishing','completed','blocked','failed','cancelling','cancelled','reconciling')),
 reason text NOT NULL DEFAULT '',
 version bigint NOT NULL DEFAULT 1,
 cancel_version bigint NOT NULL DEFAULT 0,
 cancellation_requested boolean NOT NULL DEFAULT false,
 max_attempts integer NOT NULL CHECK(max_attempts BETWEEN 1 AND 10),
 created_by uuid NOT NULL REFERENCES users(id),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(org_id,id),
 UNIQUE(org_id,operation_id),
 UNIQUE(org_id,idempotency_key),
 FOREIGN KEY(org_id,repository_id) REFERENCES repositories(org_id,id),
 FOREIGN KEY(org_id,model_connection_id) REFERENCES connections(org_id,id)
);
CREATE TABLE workflow_jobs (
 org_id uuid NOT NULL,
 id uuid NOT NULL,
 task_id uuid NOT NULL,
 operation_id uuid NOT NULL,
 state text NOT NULL CHECK(state IN ('queued','running','completed','failed','blocked','cancelled','reconciling')),
 fence bigint NOT NULL DEFAULT 0,
 attempts integer NOT NULL DEFAULT 0,
 lease_owner text NOT NULL DEFAULT '',
 lease_expires_at timestamptz,
 available_at timestamptz NOT NULL DEFAULT now(),
 priority integer NOT NULL DEFAULT 0 CHECK(priority BETWEEN -10 AND 10),
 PRIMARY KEY(org_id,id),
 UNIQUE(org_id,task_id),
 UNIQUE(org_id,operation_id),
 FOREIGN KEY(org_id,task_id) REFERENCES workflow_tasks(org_id,id)
);
CREATE INDEX workflow_ready ON workflow_jobs(org_id,state,available_at);
CREATE TABLE workflow_attempts (
 org_id uuid NOT NULL,
 id uuid NOT NULL,
 task_id uuid NOT NULL,
 job_id uuid NOT NULL,
 number integer NOT NULL,
 fence bigint NOT NULL,
 lease_owner text NOT NULL,
 state text NOT NULL CHECK(state IN ('running','completed','failed','lost','cancelled','uncertain')),
 started_at timestamptz NOT NULL DEFAULT now(),
 ended_at timestamptz,
 PRIMARY KEY(org_id,id),
 UNIQUE(org_id,job_id,fence),
 FOREIGN KEY(org_id,job_id) REFERENCES workflow_jobs(org_id,id),
 FOREIGN KEY(org_id,task_id) REFERENCES workflow_tasks(org_id,id)
);
CREATE TABLE workflow_pauses (
 org_id uuid NOT NULL REFERENCES organisations(id),
 scope_kind text NOT NULL CHECK(scope_kind IN ('organisation','repository','recipe','campaign','model','runner_pool')),
 scope_id text NOT NULL,
 paused boolean NOT NULL,
 version bigint NOT NULL DEFAULT 1,
 PRIMARY KEY(org_id,scope_kind,scope_id)
);
CREATE TABLE workflow_repo_fairness (
 org_id uuid NOT NULL,
 repository_id uuid NOT NULL,
 last_claimed_at timestamptz NOT NULL DEFAULT 'epoch',
 PRIMARY KEY(org_id,repository_id),
 FOREIGN KEY(org_id,repository_id) REFERENCES repositories(org_id,id)
);
CREATE TABLE workflow_scheduler (
 org_id uuid PRIMARY KEY REFERENCES organisations(id),
 last_claimed_at timestamptz NOT NULL DEFAULT 'epoch'
);
ALTER TABLE workflow_scheduler ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_scheduler FORCE ROW LEVEL SECURITY;
CREATE POLICY scheduler ON workflow_scheduler USING (org_id=nullif(current_setting('reforge.org_id',true),'')::uuid OR current_setting('reforge.scheduler',true)='on') WITH CHECK (org_id=nullif(current_setting('reforge.org_id',true),'')::uuid OR current_setting('reforge.scheduler',true)='on');
CREATE TABLE workflow_event_heads (
 org_id uuid PRIMARY KEY REFERENCES organisations(id),
 last_id bigint NOT NULL DEFAULT 0,
 retained_after bigint NOT NULL DEFAULT 0
);
CREATE TABLE workflow_events (
 org_id uuid NOT NULL,
 id bigint NOT NULL,
 repository_id uuid,
 type text NOT NULL,
 aggregate_type text NOT NULL,
 aggregate_id uuid NOT NULL,
 aggregate_version bigint NOT NULL,
 occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 request_id text NOT NULL,
 data_version integer NOT NULL DEFAULT 1,
 data jsonb NOT NULL,
 PRIMARY KEY(org_id,id),
 FOREIGN KEY(org_id,repository_id) REFERENCES repositories(org_id,id),
 FOREIGN KEY(org_id) REFERENCES organisations(id)
);
CREATE TABLE workflow_outbox (
 org_id uuid NOT NULL,
 id uuid NOT NULL,
 task_id uuid NOT NULL,
 repository_id uuid NOT NULL,
 operation_id uuid NOT NULL,
 kind text NOT NULL,
 idempotency_key text NOT NULL,
 payload jsonb NOT NULL,
 payload_hash text NOT NULL,
 state text NOT NULL CHECK(state IN ('pending','dispatching','succeeded','absent','unknown','failed')),
 dispatch_count integer NOT NULL DEFAULT 0,
 evidence text NOT NULL DEFAULT '',
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(org_id,id),
 UNIQUE(org_id,operation_id),
 UNIQUE(org_id,task_id,kind,idempotency_key),
 FOREIGN KEY(org_id,task_id) REFERENCES workflow_tasks(org_id,id),
 FOREIGN KEY(org_id,repository_id) REFERENCES repositories(org_id,id)
);
DO $$
DECLARE name text;
BEGIN
 FOREACH name IN ARRAY ARRAY['workflow_tasks','workflow_jobs','workflow_attempts','workflow_pauses','workflow_repo_fairness','workflow_event_heads','workflow_events','workflow_outbox'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',name);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',name);
  EXECUTE format('CREATE POLICY tenant ON %I USING (org_id=nullif(current_setting(''reforge.org_id'',true),'''')::uuid) WITH CHECK (org_id=nullif(current_setting(''reforge.org_id'',true),'''')::uuid)',name);
 END LOOP;
END $$;
