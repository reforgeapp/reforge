DROP POLICY platform_admin_read ON organisations;
DROP POLICY platform_admin_read ON memberships;
DROP POLICY platform_admin_read ON users;
DROP POLICY platform_admin_read ON repositories;
DROP POLICY platform_invitation_admin ON platform_invitations;
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'reforge_staff') THEN
        GRANT USAGE ON SCHEMA public TO reforge_staff;
        GRANT SELECT ON organisations, memberships, users, repositories TO reforge_staff;
        GRANT SELECT, INSERT, UPDATE ON platform_invitations TO reforge_staff;
        CREATE POLICY staff_read ON organisations FOR SELECT TO reforge_staff USING (true);
        CREATE POLICY staff_read ON memberships FOR SELECT TO reforge_staff USING (true);
        CREATE POLICY staff_read ON users FOR SELECT TO reforge_staff USING (true);
        CREATE POLICY staff_read ON repositories FOR SELECT TO reforge_staff USING (true);
        CREATE POLICY staff_invitations ON platform_invitations TO reforge_staff
            USING (target_org_id IS NULL) WITH CHECK (target_org_id IS NULL);
    END IF;
END
$$;
