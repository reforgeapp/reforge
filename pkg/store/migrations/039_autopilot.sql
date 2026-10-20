CREATE TABLE autopilot_settings (
    org_id uuid PRIMARY KEY REFERENCES organisations(id) ON DELETE CASCADE,
    enabled boolean NOT NULL DEFAULT false,
    enabled_by uuid REFERENCES users(id),
    status text NOT NULL DEFAULT '',
    checked_at timestamptz,
    version bigint NOT NULL DEFAULT 1
);
ALTER TABLE autopilot_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE autopilot_settings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON autopilot_settings USING (org_id = nullif(current_setting('reforge.org_id',true),'')::uuid) WITH CHECK (org_id = nullif(current_setting('reforge.org_id',true),'')::uuid);
CREATE POLICY scheduler_read ON autopilot_settings FOR SELECT USING (enabled);

CREATE TABLE autopilot_attempts (
    org_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    finding_id uuid NOT NULL,
    finding_version bigint NOT NULL,
    task_id uuid,
    outcome text NOT NULL CHECK (outcome IN ('queued','skipped','retry')),
    reason text NOT NULL DEFAULT '',
    retry_after timestamptz,
    merge_reason text NOT NULL DEFAULT '',
    merge_after timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, finding_id, finding_version)
);
CREATE INDEX autopilot_attempts_task ON autopilot_attempts(org_id, task_id);
ALTER TABLE autopilot_attempts ENABLE ROW LEVEL SECURITY;
ALTER TABLE autopilot_attempts FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON autopilot_attempts USING (org_id = nullif(current_setting('reforge.org_id',true),'')::uuid) WITH CHECK (org_id = nullif(current_setting('reforge.org_id',true),'')::uuid);
