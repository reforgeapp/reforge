CREATE TABLE org_oidc_invitations (
    org_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    id uuid NOT NULL,
    email text NOT NULL CHECK(length(email) BETWEEN 3 AND 320 AND email=lower(btrim(email))),
    role text NOT NULL CHECK(role IN ('owner','admin','maintainer','reviewer','viewer')),
    oidc_config_id uuid NOT NULL,
    oidc_config_version bigint NOT NULL CHECK(oidc_config_version>0),
    issuer text NOT NULL CHECK(length(issuer) BETWEEN 1 AND 2048),
    token_hash text NOT NULL UNIQUE CHECK(length(token_hash)=64),
    created_by uuid NOT NULL REFERENCES users(id),
    expires_at timestamptz NOT NULL,
    redeemed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(org_id,id),
    FOREIGN KEY(org_id,oidc_config_id) REFERENCES org_oidc_configs(org_id,id) ON DELETE CASCADE
);
CREATE INDEX org_oidc_invitations_expiry ON org_oidc_invitations(org_id,expires_at) WHERE redeemed_at IS NULL;
ALTER TABLE org_oidc_invitations ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_oidc_invitations FORCE ROW LEVEL SECURITY;
CREATE POLICY invitation_manage ON org_oidc_invitations FOR ALL
    USING (
        org_id = nullif(current_setting('reforge.org_id',true),'')::uuid
        AND nullif(current_setting('reforge.user_id',true),'') IS NOT NULL
        AND nullif(current_setting('reforge.invitation_hash',true),'') IS NULL
    )
    WITH CHECK (
        org_id = nullif(current_setting('reforge.org_id',true),'')::uuid
        AND nullif(current_setting('reforge.user_id',true),'') IS NOT NULL
        AND nullif(current_setting('reforge.invitation_hash',true),'') IS NULL
    );
CREATE POLICY invitation_claim ON org_oidc_invitations FOR SELECT
    USING (token_hash = current_setting('reforge.invitation_hash',true));
CREATE POLICY invitation_redeem ON org_oidc_invitations FOR UPDATE
    USING (token_hash = current_setting('reforge.invitation_hash',true))
    WITH CHECK (token_hash = current_setting('reforge.invitation_hash',true));

ALTER TABLE oidc_logins
    ADD COLUMN invitation_org_id uuid,
    ADD COLUMN invitation_id uuid,
    ADD COLUMN invitation_hash text,
    ADD CONSTRAINT oidc_logins_invitation_binding CHECK (
        (invitation_org_id IS NULL AND invitation_id IS NULL AND invitation_hash IS NULL) OR
        (invitation_org_id IS NOT NULL AND invitation_id IS NOT NULL AND invitation_hash IS NOT NULL AND length(invitation_hash)=64 AND invitation_org_id=org_id)
    ),
    ADD CONSTRAINT oidc_logins_invitation_fk FOREIGN KEY(invitation_org_id,invitation_id) REFERENCES org_oidc_invitations(org_id,id) ON DELETE CASCADE;
