CREATE TABLE github_app_setups (
    org_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    id uuid NOT NULL,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id uuid NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    mode text NOT NULL CHECK(mode IN ('manifest','hosted')),
    phase text NOT NULL CHECK(phase IN ('created','handed_off','converting','awaiting_install','authorizing','completing')),
    name text NOT NULL CHECK(length(name) BETWEEN 1 AND 160),
    github_org text NOT NULL DEFAULT '' CHECK(github_org ~ '^([A-Za-z0-9-]{1,39})?$'),
    state_hash text UNIQUE CHECK(state_hash IS NULL OR length(state_hash)=64),
    connection_id uuid NOT NULL,
    webhook_id uuid NOT NULL,
    app_id bigint CHECK(app_id IS NULL OR app_id>0),
    app_slug text NOT NULL DEFAULT '' CHECK(app_slug ~ '^[a-z0-9-]{0,64}$'),
    owner_id bigint,
    installation_id bigint CHECK(installation_id IS NULL OR installation_id>0),
    envelope jsonb,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(org_id,id)
);
CREATE INDEX github_app_setups_user ON github_app_setups(org_id,user_id);

CREATE TABLE github_installation_bindings (
    app_id bigint NOT NULL CHECK(app_id>0),
    installation_id bigint NOT NULL CHECK(installation_id>0),
    account_id bigint NOT NULL CHECK(account_id>0),
    org_id uuid NOT NULL,
    connection_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(app_id,installation_id),
    UNIQUE(app_id,account_id),
    UNIQUE(org_id,connection_id),
    FOREIGN KEY(org_id,connection_id) REFERENCES connections(org_id,id) ON DELETE CASCADE
);

ALTER TABLE github_app_setups ENABLE ROW LEVEL SECURITY;
ALTER TABLE github_app_setups FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON github_app_setups FOR ALL
    USING (org_id = nullif(current_setting('reforge.org_id',true),'')::uuid AND nullif(current_setting('reforge.user_id',true),'') IS NOT NULL)
    WITH CHECK (org_id = nullif(current_setting('reforge.org_id',true),'')::uuid AND nullif(current_setting('reforge.user_id',true),'') IS NOT NULL);
CREATE POLICY state_lookup ON github_app_setups FOR SELECT
    USING (state_hash = current_setting('reforge.github_state_hash',true) AND user_id = nullif(current_setting('reforge.user_id',true),'')::uuid);
CREATE POLICY setup_lookup ON github_app_setups FOR SELECT
    USING (id = nullif(current_setting('reforge.github_setup_id',true),'')::uuid AND user_id = nullif(current_setting('reforge.user_id',true),'')::uuid);
CREATE POLICY expiry_cleanup ON github_app_setups FOR DELETE
    USING (expires_at <= now());

ALTER TABLE github_installation_bindings ENABLE ROW LEVEL SECURITY;
ALTER TABLE github_installation_bindings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON github_installation_bindings FOR ALL
    USING (org_id = nullif(current_setting('reforge.org_id',true),'')::uuid)
    WITH CHECK (org_id = nullif(current_setting('reforge.org_id',true),'')::uuid);
CREATE POLICY webhook_route ON github_installation_bindings FOR SELECT
    USING (app_id = nullif(current_setting('reforge.github_app_id',true),'')::bigint AND installation_id = nullif(current_setting('reforge.github_installation_id',true),'')::bigint);
