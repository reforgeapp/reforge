ALTER TABLE organisations ADD COLUMN slug text UNIQUE CHECK (slug ~ '^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$');
CREATE POLICY login_slug ON organisations FOR SELECT USING (slug = current_setting('reforge.login_slug', true));
ALTER TABLE platform_invitations ADD COLUMN slug text CHECK (slug IS NULL OR slug ~ '^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$');
