CREATE TABLE runner_pools (
 org_id uuid NOT NULL REFERENCES organisations(id),
 id uuid NOT NULL,
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 160),
 state text NOT NULL CHECK(state IN ('active','draining','revoked')) DEFAULT 'active',
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(org_id,id)
);
CREATE TABLE runner_pool_repositories (
 org_id uuid NOT NULL,
 pool_id uuid NOT NULL,
 repository_id uuid NOT NULL,
 PRIMARY KEY(org_id,pool_id,repository_id),
 FOREIGN KEY(org_id,pool_id) REFERENCES runner_pools(org_id,id),
 FOREIGN KEY(org_id,repository_id) REFERENCES repositories(org_id,id)
);
CREATE TABLE runner_enrollments (
 org_id uuid NOT NULL,
 id uuid NOT NULL,
 pool_id uuid NOT NULL,
 token_hash text NOT NULL,
 expires_at timestamptz NOT NULL,
 consumed_at timestamptz,
 created_by uuid NOT NULL REFERENCES users(id),
 PRIMARY KEY(org_id,id),
 FOREIGN KEY(org_id,pool_id) REFERENCES runner_pools(org_id,id)
);
CREATE TABLE runners (
 org_id uuid NOT NULL,
 id uuid NOT NULL,
 pool_id uuid NOT NULL,
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 160),
 state text NOT NULL CHECK(state IN ('active','revoked')) DEFAULT 'active',
 credential_hash text NOT NULL,
 credential_expires_at timestamptz NOT NULL,
 credential_version bigint NOT NULL DEFAULT 1,
 version bigint NOT NULL DEFAULT 1,
 enrolled_at timestamptz NOT NULL DEFAULT now(),
 last_seen_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(org_id,id),
 UNIQUE(org_id,id,pool_id),
 FOREIGN KEY(org_id,pool_id) REFERENCES runner_pools(org_id,id)
);
CREATE TABLE runner_job_credentials (
 org_id uuid NOT NULL,
 id uuid NOT NULL,
 runner_id uuid NOT NULL,
 pool_id uuid NOT NULL,
 repository_id uuid NOT NULL,
 task_id uuid NOT NULL,
 job_id uuid NOT NULL,
 attempt_id uuid NOT NULL,
 operation_id uuid NOT NULL,
 fencing_token bigint NOT NULL,
 policy_hash text NOT NULL,
 token_hash text NOT NULL,
 methods text[] NOT NULL,
 expires_at timestamptz NOT NULL,
 revoked_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(org_id,id),
 FOREIGN KEY(org_id,runner_id,pool_id) REFERENCES runners(org_id,id,pool_id),
 FOREIGN KEY(org_id,repository_id) REFERENCES repositories(org_id,id),
 FOREIGN KEY(org_id,task_id) REFERENCES workflow_tasks(org_id,id),
 FOREIGN KEY(org_id,job_id,task_id) REFERENCES workflow_jobs(org_id,id,task_id),
 FOREIGN KEY(org_id,attempt_id) REFERENCES workflow_attempts(org_id,id)
);
CREATE INDEX runner_job_by_runner ON runner_job_credentials(org_id,runner_id);
CREATE TABLE artifacts (
 org_id uuid NOT NULL,
 id uuid NOT NULL,
 repository_id uuid NOT NULL,
 task_id uuid NOT NULL,
 attempt_id uuid NOT NULL,
 name text NOT NULL,
 media_type text NOT NULL,
 size bigint NOT NULL CHECK(size BETWEEN 0 AND 1048576),
 sha256 text NOT NULL CHECK(length(sha256)=64),
 created_at timestamptz NOT NULL DEFAULT now(),
 expires_at timestamptz NOT NULL,
 PRIMARY KEY(org_id,id),
 FOREIGN KEY(org_id,repository_id) REFERENCES repositories(org_id,id),
 FOREIGN KEY(org_id,task_id) REFERENCES workflow_tasks(org_id,id),
 FOREIGN KEY(org_id,attempt_id) REFERENCES workflow_attempts(org_id,id)
);
CREATE INDEX artifact_retention ON artifacts(org_id,expires_at);
DO $$
DECLARE name text;
BEGIN
 FOREACH name IN ARRAY ARRAY['runner_pools','runner_pool_repositories','runner_enrollments','runners','runner_job_credentials','artifacts'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',name);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',name);
  EXECUTE format('CREATE POLICY tenant ON %I USING (org_id=nullif(current_setting(''reforge.org_id'',true),'''')::uuid) WITH CHECK (org_id=nullif(current_setting(''reforge.org_id'',true),'''')::uuid)',name);
 END LOOP;
END $$;

ALTER TABLE workflow_tasks ADD CONSTRAINT task_runner_pool_tenant FOREIGN KEY(org_id,runner_pool_id) REFERENCES runner_pools(org_id,id);
ALTER TABLE workflow_tasks ADD CONSTRAINT workflow_task_repository_unique UNIQUE(org_id,id,repository_id);
ALTER TABLE workflow_attempts ADD CONSTRAINT workflow_attempt_task_unique UNIQUE(org_id,id,task_id);
ALTER TABLE runner_job_credentials ADD CONSTRAINT runner_job_task_repository FOREIGN KEY(org_id,task_id,repository_id) REFERENCES workflow_tasks(org_id,id,repository_id);
ALTER TABLE runner_job_credentials ADD CONSTRAINT runner_job_attempt_task FOREIGN KEY(org_id,attempt_id,task_id) REFERENCES workflow_attempts(org_id,id,task_id);
ALTER TABLE artifacts ADD CONSTRAINT artifact_task_repository FOREIGN KEY(org_id,task_id,repository_id) REFERENCES workflow_tasks(org_id,id,repository_id);
ALTER TABLE artifacts ADD CONSTRAINT artifact_attempt_task FOREIGN KEY(org_id,attempt_id,task_id) REFERENCES workflow_attempts(org_id,id,task_id);
