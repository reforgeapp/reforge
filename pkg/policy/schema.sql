CREATE TABLE policy_versions (
    org_id uuid NOT NULL REFERENCES organisations(id),
    id uuid NOT NULL,
    scope_kind text NOT NULL CHECK(scope_kind IN ('organisation','team','repository')),
    scope_id uuid NOT NULL,
    document jsonb NOT NULL,
    policy_hash text NOT NULL,
    actor_id uuid NOT NULL REFERENCES users(id),
    reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 1000),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(org_id,id),
    UNIQUE(org_id,id,scope_kind,scope_id),
    CHECK(scope_kind != 'organisation' OR scope_id=org_id)
);
ALTER TABLE policy_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_versions FORCE ROW LEVEL SECURITY;
CREATE POLICY policy_read ON policy_versions FOR SELECT USING (org_id=nullif(current_setting('reforge.org_id',true),'')::uuid);
CREATE POLICY policy_insert ON policy_versions FOR INSERT WITH CHECK (org_id=nullif(current_setting('reforge.org_id',true),'')::uuid);

CREATE TABLE policy_bindings (
    org_id uuid NOT NULL REFERENCES organisations(id),
    scope_kind text NOT NULL CHECK(scope_kind IN ('organisation','team','repository')),
    scope_id uuid NOT NULL,
    policy_version_id uuid NOT NULL,
    version bigint NOT NULL CHECK(version>0),
    primary_team_id uuid,
    simulation_hash text NOT NULL,
    PRIMARY KEY(org_id,scope_kind,scope_id),
    FOREIGN KEY(org_id,policy_version_id,scope_kind,scope_id) REFERENCES policy_versions(org_id,id,scope_kind,scope_id),
    FOREIGN KEY(org_id,primary_team_id) REFERENCES teams(org_id,id),
    CHECK(scope_kind='repository' OR primary_team_id IS NULL),
    CHECK(scope_kind != 'organisation' OR scope_id=org_id)
);
ALTER TABLE policy_bindings ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_bindings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON policy_bindings USING (org_id=nullif(current_setting('reforge.org_id',true),'')::uuid) WITH CHECK (org_id=nullif(current_setting('reforge.org_id',true),'')::uuid);
