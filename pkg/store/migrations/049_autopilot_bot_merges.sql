CREATE TABLE autopilot_bot_merges (
    org_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    repository_id uuid NOT NULL,
    change_id text NOT NULL,
    head_sha text NOT NULL,
    reason text NOT NULL DEFAULT '',
    merge_after timestamptz NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, repository_id, change_id)
);
ALTER TABLE autopilot_bot_merges ENABLE ROW LEVEL SECURITY;
ALTER TABLE autopilot_bot_merges FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON autopilot_bot_merges USING (org_id = nullif(current_setting('reforge.org_id',true),'')::uuid) WITH CHECK (org_id = nullif(current_setting('reforge.org_id',true),'')::uuid);
