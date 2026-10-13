CREATE TABLE release_receipts (
 org_id uuid NOT NULL,
 id uuid NOT NULL,
 repository_id uuid NOT NULL,
 environment text NOT NULL,
 kind text NOT NULL CHECK(kind IN ('native','gitops')),
 configuration_version bigint NOT NULL,
 source_sha text NOT NULL,
 target_sha text NOT NULL DEFAULT '',
 artifact_digest text NOT NULL,
 build_id text NOT NULL,
 document jsonb NOT NULL,
 received_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(org_id,id),
 UNIQUE(org_id,repository_id,environment,kind,configuration_version,build_id,source_sha),
 FOREIGN KEY(org_id,repository_id) REFERENCES repositories(org_id,id)
);
ALTER TABLE release_receipts ENABLE ROW LEVEL SECURITY;
ALTER TABLE release_receipts FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON release_receipts USING(org_id=nullif(current_setting('reforge.org_id',true),'')::uuid) WITH CHECK(org_id=nullif(current_setting('reforge.org_id',true),'')::uuid);
CREATE INDEX release_receipts_source ON release_receipts(org_id,repository_id,source_sha,target_sha,received_at DESC);
