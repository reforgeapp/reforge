CREATE TABLE org_oidc_configs (
    org_id uuid PRIMARY KEY REFERENCES organisations(id) ON DELETE CASCADE,
    id uuid NOT NULL,
    issuer text NOT NULL CHECK(length(issuer) BETWEEN 1 AND 2048),
    client_id text NOT NULL CHECK(length(client_id) BETWEEN 1 AND 512),
    status text NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','disabled')),
    version bigint NOT NULL DEFAULT 1 CHECK(version > 0),
    verified_version bigint,
    verified_at timestamptz,
    probe_attempt_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id)
);
ALTER TABLE org_oidc_configs ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_oidc_configs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON org_oidc_configs USING (org_id = nullif(current_setting('reforge.org_id',true),'')::uuid) WITH CHECK (org_id = nullif(current_setting('reforge.org_id',true),'')::uuid);

CREATE TABLE org_oidc_secrets (
    org_id uuid NOT NULL,
    config_id uuid NOT NULL,
    version bigint NOT NULL CHECK(version > 0),
    envelope jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(org_id,config_id),
    FOREIGN KEY(org_id,config_id) REFERENCES org_oidc_configs(org_id,id) ON DELETE CASCADE
);
ALTER TABLE org_oidc_secrets ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_oidc_secrets FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON org_oidc_secrets USING (org_id = nullif(current_setting('reforge.org_id',true),'')::uuid) WITH CHECK (org_id = nullif(current_setting('reforge.org_id',true),'')::uuid);
