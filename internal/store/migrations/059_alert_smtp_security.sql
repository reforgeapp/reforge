ALTER TABLE alert_settings ADD COLUMN smtp_security text NOT NULL DEFAULT 'starttls' CHECK (smtp_security IN ('starttls','tls','none'));
