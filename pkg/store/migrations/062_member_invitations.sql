ALTER TABLE platform_invitations
    ADD COLUMN target_org_id uuid REFERENCES organisations(id) ON DELETE CASCADE,
    ADD COLUMN role text NOT NULL DEFAULT 'owner' CHECK (role IN ('owner','admin','maintainer','reviewer','viewer')),
    ADD COLUMN invited_by uuid REFERENCES users(id),
    ADD COLUMN revoked_at timestamptz;
CREATE INDEX platform_invitations_target ON platform_invitations(target_org_id) WHERE target_org_id IS NOT NULL;
CREATE INDEX platform_invitations_email ON platform_invitations(email);
CREATE POLICY platform_invitation_org ON platform_invitations
    USING (target_org_id = nullif(current_setting('reforge.org_id', true), '')::uuid)
    WITH CHECK (target_org_id = nullif(current_setting('reforge.org_id', true), '')::uuid);
CREATE POLICY platform_invitation_admin ON platform_invitations
    USING (current_setting('reforge.platform_admin', true) = 'true')
    WITH CHECK (current_setting('reforge.platform_admin', true) = 'true');
CREATE POLICY platform_admin_read ON organisations FOR SELECT USING (current_setting('reforge.platform_admin', true) = 'true');
CREATE POLICY platform_admin_read ON memberships FOR SELECT USING (current_setting('reforge.platform_admin', true) = 'true');
CREATE POLICY platform_admin_read ON users FOR SELECT USING (current_setting('reforge.platform_admin', true) = 'true');
CREATE POLICY platform_admin_read ON repositories FOR SELECT USING (current_setting('reforge.platform_admin', true) = 'true');
