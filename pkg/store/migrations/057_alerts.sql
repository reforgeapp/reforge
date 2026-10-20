CREATE TABLE alert_settings (
    org_id uuid PRIMARY KEY REFERENCES organisations(id) ON DELETE CASCADE,
    enabled boolean NOT NULL DEFAULT true,
    recipients jsonb NOT NULL DEFAULT '[]',
    version bigint NOT NULL DEFAULT 1
);
ALTER TABLE alert_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE alert_settings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON alert_settings USING (org_id = nullif(current_setting('reforge.org_id',true),'')::uuid) WITH CHECK (org_id = nullif(current_setting('reforge.org_id',true),'')::uuid);
CREATE TABLE alert_deliveries (
    org_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    key text NOT NULL,
    sent_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, key)
);
ALTER TABLE alert_deliveries ENABLE ROW LEVEL SECURITY;
ALTER TABLE alert_deliveries FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON alert_deliveries USING (org_id = nullif(current_setting('reforge.org_id',true),'')::uuid) WITH CHECK (org_id = nullif(current_setting('reforge.org_id',true),'')::uuid);
