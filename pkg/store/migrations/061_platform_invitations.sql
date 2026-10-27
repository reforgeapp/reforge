CREATE TABLE platform_invitations (
    id uuid PRIMARY KEY,
    token_hash text NOT NULL UNIQUE,
    email text NOT NULL,
    org_name text NOT NULL CHECK (length(org_name) BETWEEN 1 AND 160),
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    redeemed_at timestamptz,
    redeemed_by uuid REFERENCES users(id),
    org_id uuid REFERENCES organisations(id)
);
ALTER TABLE platform_invitations ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform_invitations FORCE ROW LEVEL SECURITY;
CREATE POLICY platform_invitation ON platform_invitations
    USING (token_hash = current_setting('reforge.platform_invitation_hash', true))
    WITH CHECK (token_hash = current_setting('reforge.platform_invitation_hash', true));
