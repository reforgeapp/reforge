ALTER TABLE org_oidc_configs DROP CONSTRAINT org_oidc_configs_status_check;
ALTER TABLE org_oidc_configs ADD CONSTRAINT org_oidc_configs_status_check CHECK(status IN ('draft','disabled','active'));

ALTER TABLE sessions
    ADD COLUMN org_id uuid REFERENCES organisations(id) ON DELETE CASCADE,
    ADD COLUMN oidc_config_id uuid,
    ADD COLUMN oidc_config_version bigint,
    ADD CONSTRAINT sessions_org_oidc_binding CHECK (
        (org_id IS NULL AND oidc_config_id IS NULL AND oidc_config_version IS NULL) OR
        (org_id IS NOT NULL AND oidc_config_id IS NOT NULL AND oidc_config_version IS NOT NULL AND oidc_config_version > 0)
    ),
    ADD CONSTRAINT sessions_org_oidc_config_fk FOREIGN KEY(org_id,oidc_config_id) REFERENCES org_oidc_configs(org_id,id) ON DELETE CASCADE;
CREATE INDEX sessions_org_user ON sessions(org_id,user_id) WHERE org_id IS NOT NULL;

ALTER TABLE oidc_logins
    ADD COLUMN org_id uuid,
    ADD COLUMN config_id uuid,
    ADD COLUMN config_version bigint,
    ADD COLUMN issuer text,
    ADD COLUMN client_id text,
    ADD CONSTRAINT oidc_logins_org_binding CHECK (
        (org_id IS NULL AND config_id IS NULL AND config_version IS NULL AND issuer IS NULL AND client_id IS NULL) OR
        (org_id IS NOT NULL AND config_id IS NOT NULL AND config_version IS NOT NULL AND config_version > 0 AND issuer IS NOT NULL AND client_id IS NOT NULL)
    ),
    ADD CONSTRAINT oidc_logins_org_config_fk FOREIGN KEY(org_id,config_id) REFERENCES org_oidc_configs(org_id,id) ON DELETE CASCADE;
CREATE INDEX oidc_logins_org_expiry ON oidc_logins(org_id,expires_at) WHERE org_id IS NOT NULL;

CREATE POLICY org_login_read ON org_oidc_configs FOR SELECT USING (
    org_id = nullif(current_setting('reforge.login_org_id',true),'')::uuid AND
    status = 'active' AND verified_version = version
);
CREATE POLICY pending_login_secret_read ON org_oidc_secrets FOR SELECT USING (
    org_id = nullif(current_setting('reforge.login_org_id',true),'')::uuid AND
    config_id = nullif(current_setting('reforge.login_config_id',true),'')::uuid AND
    version = nullif(current_setting('reforge.login_config_version',true),'')::bigint
);
