CREATE TABLE run_logs (
    org_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    task_id uuid NOT NULL,
    seq bigint NOT NULL,
    attempt_id uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('stage','model','tool','result')),
    message text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, task_id, seq)
);
ALTER TABLE run_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE run_logs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON run_logs USING (org_id = nullif(current_setting('reforge.org_id',true),'')::uuid) WITH CHECK (org_id = nullif(current_setting('reforge.org_id',true),'')::uuid);
