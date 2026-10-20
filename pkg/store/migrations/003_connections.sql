CREATE TABLE connections (
    org_id uuid NOT NULL REFERENCES organisations(id),
    id uuid NOT NULL,
    kind text NOT NULL CHECK(kind IN ('forge','model','agent','delivery')),
    provider text NOT NULL,
    name text NOT NULL CHECK(length(name) BETWEEN 1 AND 160),
    endpoint text NOT NULL,
    settings jsonb NOT NULL,
    state text NOT NULL DEFAULT 'unverified' CHECK(state IN ('unverified','healthy','degraded','disabled','revoked')),
    reason text NOT NULL DEFAULT 'Run a capability test before use',
    capabilities jsonb NOT NULL DEFAULT '{}',
    server_version text NOT NULL DEFAULT '',
    verified_at timestamptz,
    secret_id uuid,
    credential_version bigint NOT NULL DEFAULT 1,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    PRIMARY KEY(org_id,id)
);

CREATE TABLE connection_secrets (
    org_id uuid NOT NULL,
    id uuid NOT NULL,
    connection_id uuid NOT NULL,
    version bigint NOT NULL,
    envelope jsonb NOT NULL,
    rotated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(org_id,id),
    UNIQUE(org_id,connection_id),
    FOREIGN KEY(org_id,connection_id) REFERENCES connections(org_id,id) ON DELETE CASCADE
);
ALTER TABLE connections ADD CONSTRAINT connection_secret_tenant FOREIGN KEY(org_id,secret_id) REFERENCES connection_secrets(org_id,id) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE repositories ADD CONSTRAINT repository_connection_tenant FOREIGN KEY(org_id,connection_id) REFERENCES connections(org_id,id);

CREATE TABLE connection_routes (
    org_id uuid NOT NULL,
    connection_id uuid NOT NULL,
    runner_id uuid NOT NULL,
    hostname text NOT NULL,
    cidrs text[] NOT NULL CHECK(cardinality(cidrs)>0),
    approved_by uuid NOT NULL REFERENCES users(id),
    approved_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    PRIMARY KEY(org_id,connection_id),
    FOREIGN KEY(org_id,connection_id) REFERENCES connections(org_id,id) ON DELETE CASCADE
);

DO $$
DECLARE name text;
BEGIN
    FOREACH name IN ARRAY ARRAY['connections','connection_secrets','connection_routes'] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',name);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',name);
        EXECUTE format('CREATE POLICY tenant ON %I USING (org_id = nullif(current_setting(''reforge.org_id'',true),'''')::uuid) WITH CHECK (org_id = nullif(current_setting(''reforge.org_id'',true),'''')::uuid)',name);
    END LOOP;
END $$;
