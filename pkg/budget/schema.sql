CREATE TABLE budget_limits (
    org_id uuid NOT NULL REFERENCES organisations(id),
    scope_kind text NOT NULL CHECK(scope_kind IN ('organisation','team','repository','campaign','connection')),
    scope_id uuid NOT NULL,
    period text NOT NULL CHECK(period IN ('daily','monthly','custom')),
    period_start timestamptz NOT NULL,
    period_end timestamptz NOT NULL,
    caps jsonb NOT NULL,
    held jsonb NOT NULL DEFAULT '{"micro_usd":0,"tokens":0,"milliseconds":0,"requests":0,"concurrency":0}',
    paused boolean NOT NULL DEFAULT false,
    version bigint NOT NULL CHECK(version>0),
    PRIMARY KEY(org_id,scope_kind,scope_id),
    CHECK(scope_kind != 'organisation' OR scope_id=org_id),
    CHECK(period <> 'custom' OR period_end > period_start)
);
CREATE TABLE budget_spend (
    org_id uuid NOT NULL,
    scope_kind text NOT NULL,
    scope_id uuid NOT NULL,
    period_start timestamptz NOT NULL,
    amount jsonb NOT NULL,
    PRIMARY KEY(org_id,scope_kind,scope_id,period_start),
    FOREIGN KEY(org_id,scope_kind,scope_id) REFERENCES budget_limits(org_id,scope_kind,scope_id)
);
CREATE TABLE budget_routes (
    org_id uuid NOT NULL,
    connection_id uuid NOT NULL,
    model text NOT NULL,
    name text NOT NULL,
    config jsonb NOT NULL,
    version bigint NOT NULL CHECK(version>0),
    PRIMARY KEY(org_id,connection_id,model,name),
    FOREIGN KEY(org_id,connection_id) REFERENCES connections(org_id,id)
);
CREATE TABLE budget_reservations (
    org_id uuid NOT NULL REFERENCES organisations(id),
    id uuid NOT NULL,
    operation_id uuid NOT NULL,
    repository_id uuid NOT NULL,
    task_id uuid NOT NULL,
    job_id uuid NOT NULL,
    attempt_id uuid NOT NULL,
    fingerprint text NOT NULL,
    record jsonb NOT NULL,
    state text NOT NULL CHECK(state IN ('reserved','dispatched','unknown','settled','cancelled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(org_id,id),
    UNIQUE(org_id,operation_id),
    FOREIGN KEY(org_id,repository_id) REFERENCES repositories(org_id,id),
    FOREIGN KEY(org_id,task_id) REFERENCES workflow_tasks(org_id,id),
    FOREIGN KEY(org_id,job_id) REFERENCES workflow_jobs(org_id,id),
    FOREIGN KEY(org_id,attempt_id) REFERENCES workflow_attempts(org_id,id)
);
CREATE INDEX budget_reservations_repository ON budget_reservations(org_id,repository_id,created_at,id);
DO $$
DECLARE name text;
BEGIN
    FOREACH name IN ARRAY ARRAY['budget_limits','budget_spend','budget_routes','budget_reservations'] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',name);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',name);
        EXECUTE format('CREATE POLICY tenant ON %I USING (org_id=nullif(current_setting(''reforge.org_id'',true),'''')::uuid) WITH CHECK (org_id=nullif(current_setting(''reforge.org_id'',true),'''')::uuid)',name);
    END LOOP;
END $$;
