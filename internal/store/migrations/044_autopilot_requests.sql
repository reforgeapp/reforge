CREATE TABLE autopilot_requests (
    org_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    repository_id uuid NOT NULL,
    requested_by uuid NOT NULL REFERENCES users(id),
    requested_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, repository_id),
    FOREIGN KEY (org_id, repository_id) REFERENCES repositories(org_id, id) ON DELETE CASCADE
);
ALTER TABLE autopilot_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE autopilot_requests FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON autopilot_requests USING (org_id = nullif(current_setting('reforge.org_id',true),'')::uuid) WITH CHECK (org_id = nullif(current_setting('reforge.org_id',true),'')::uuid);
CREATE POLICY scheduler_read ON autopilot_requests FOR SELECT USING (true);
