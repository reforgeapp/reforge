CREATE TABLE environment_health (
    org_id uuid NOT NULL,
    repository_id uuid NOT NULL,
    environment text NOT NULL,
    kind text NOT NULL CHECK(kind IN ('native','gitops')),
    operation_id uuid NOT NULL,
    configuration_version bigint NOT NULL,
    release_created_at timestamptz NOT NULL,
    source_sha text NOT NULL,
    artifact_digest text NOT NULL,
    healthy boolean NOT NULL,
    observed_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    document jsonb NOT NULL,
    PRIMARY KEY(org_id,repository_id,environment,kind),
    FOREIGN KEY(org_id,repository_id) REFERENCES repositories(org_id,id)
);
ALTER TABLE environment_health ENABLE ROW LEVEL SECURITY;
ALTER TABLE environment_health FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON environment_health USING (org_id=nullif(current_setting('reforge.org_id',true),'')::uuid) WITH CHECK (org_id=nullif(current_setting('reforge.org_id',true),'')::uuid);
