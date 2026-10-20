ALTER TABLE alert_settings ADD COLUMN smtp_address text NOT NULL DEFAULT '';
ALTER TABLE alert_settings ADD COLUMN smtp_username text NOT NULL DEFAULT '';
ALTER TABLE alert_settings ADD COLUMN smtp_from text NOT NULL DEFAULT '';
ALTER TABLE alert_settings ADD COLUMN smtp_password jsonb;
