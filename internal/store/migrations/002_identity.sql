CREATE TABLE users (
    id uuid PRIMARY KEY,
    issuer text NOT NULL,
    subject text NOT NULL,
    name text NOT NULL,
    email text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (issuer, subject)
);
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY;
CREATE POLICY identity ON users USING (
    id = nullif(current_setting('reforge.user_id',true),'')::uuid OR
    (issuer = current_setting('reforge.issuer',true) AND subject = current_setting('reforge.subject',true))
) WITH CHECK (
    id = nullif(current_setting('reforge.user_id',true),'')::uuid OR
    (issuer = current_setting('reforge.issuer',true) AND subject = current_setting('reforge.subject',true))
);

CREATE TABLE sessions (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id),
    token_hash text NOT NULL UNIQUE,
    csrf_token text NOT NULL,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_user ON sessions(user_id);
ALTER TABLE sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE sessions FORCE ROW LEVEL SECURITY;
CREATE POLICY identity ON sessions USING (
    token_hash = current_setting('reforge.session_hash',true) OR user_id = nullif(current_setting('reforge.user_id',true),'')::uuid
) WITH CHECK (user_id = nullif(current_setting('reforge.user_id',true),'')::uuid);

CREATE TABLE oidc_logins (
    state_hash text PRIMARY KEY,
    browser_hash text NOT NULL,
    nonce text NOT NULL,
    verifier text NOT NULL,
    expires_at timestamptz NOT NULL
);
ALTER TABLE oidc_logins ENABLE ROW LEVEL SECURITY;
ALTER TABLE oidc_logins FORCE ROW LEVEL SECURITY;
CREATE POLICY login ON oidc_logins USING (state_hash = current_setting('reforge.login_hash',true)) WITH CHECK (state_hash = current_setting('reforge.login_hash',true));

CREATE TABLE bootstrap (
    id boolean PRIMARY KEY DEFAULT true CHECK(id),
    token_hash text NOT NULL,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz
);
ALTER TABLE bootstrap ENABLE ROW LEVEL SECURITY;
ALTER TABLE bootstrap FORCE ROW LEVEL SECURITY;
CREATE POLICY bootstrap ON bootstrap USING (token_hash = current_setting('reforge.bootstrap_hash',true)) WITH CHECK (token_hash = current_setting('reforge.bootstrap_hash',true));

CREATE TABLE memberships (
    org_id uuid NOT NULL REFERENCES organisations(id),
    user_id uuid NOT NULL REFERENCES users(id),
    role text NOT NULL CHECK(role IN ('owner','admin','maintainer','reviewer','viewer')),
    all_repositories boolean NOT NULL DEFAULT false,
    version bigint NOT NULL DEFAULT 1,
    PRIMARY KEY(org_id,user_id)
);
ALTER TABLE memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE memberships FORCE ROW LEVEL SECURITY;
CREATE POLICY member_read ON memberships FOR SELECT USING (org_id = nullif(current_setting('reforge.org_id',true),'')::uuid OR user_id = nullif(current_setting('reforge.user_id',true),'')::uuid);
CREATE POLICY member_write ON memberships FOR ALL USING (org_id = nullif(current_setting('reforge.org_id',true),'')::uuid) WITH CHECK (org_id = nullif(current_setting('reforge.org_id',true),'')::uuid);

CREATE TABLE teams (
    org_id uuid NOT NULL REFERENCES organisations(id),
    id uuid NOT NULL,
    name text NOT NULL CHECK(length(name) BETWEEN 1 AND 160),
    version bigint NOT NULL DEFAULT 1,
    PRIMARY KEY(org_id,id),
    UNIQUE(org_id,name)
);
CREATE TABLE repositories (
    org_id uuid NOT NULL REFERENCES organisations(id),
    id uuid NOT NULL,
    connection_id uuid,
    native_id text NOT NULL,
    name text NOT NULL,
    url text NOT NULL DEFAULT '',
    default_branch text NOT NULL DEFAULT '',
    provider text NOT NULL DEFAULT '',
    archived boolean NOT NULL DEFAULT false,
    paused boolean NOT NULL DEFAULT false,
    accessible boolean NOT NULL DEFAULT true,
    last_synced_at timestamptz,
    version bigint NOT NULL DEFAULT 1,
    PRIMARY KEY(org_id,id),
    UNIQUE(org_id,connection_id,native_id)
);
CREATE TABLE team_memberships (
    org_id uuid NOT NULL,
    team_id uuid NOT NULL,
    user_id uuid NOT NULL,
    PRIMARY KEY(org_id,team_id,user_id),
    FOREIGN KEY(org_id,team_id) REFERENCES teams(org_id,id) ON DELETE CASCADE,
    FOREIGN KEY(org_id,user_id) REFERENCES memberships(org_id,user_id) ON DELETE CASCADE
);
CREATE TABLE team_repositories (
    org_id uuid NOT NULL,
    team_id uuid NOT NULL,
    repository_id uuid NOT NULL,
    PRIMARY KEY(org_id,team_id,repository_id),
    FOREIGN KEY(org_id,team_id) REFERENCES teams(org_id,id) ON DELETE CASCADE,
    FOREIGN KEY(org_id,repository_id) REFERENCES repositories(org_id,id) ON DELETE CASCADE
);
CREATE TABLE member_repositories (
    org_id uuid NOT NULL,
    user_id uuid NOT NULL,
    repository_id uuid NOT NULL,
    PRIMARY KEY(org_id,user_id,repository_id),
    FOREIGN KEY(org_id,user_id) REFERENCES memberships(org_id,user_id) ON DELETE CASCADE,
    FOREIGN KEY(org_id,repository_id) REFERENCES repositories(org_id,id) ON DELETE CASCADE
);
DO $$
DECLARE name text;
BEGIN
    FOREACH name IN ARRAY ARRAY['teams','repositories','team_memberships','team_repositories','member_repositories'] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',name);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',name);
        EXECUTE format('CREATE POLICY tenant ON %I USING (org_id = nullif(current_setting(''reforge.org_id'',true),'''')::uuid) WITH CHECK (org_id = nullif(current_setting(''reforge.org_id'',true),'''')::uuid)',name);
    END LOOP;
END $$;
