CREATE TABLE organisations (
    id uuid PRIMARY KEY,
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 160),
    version bigint NOT NULL DEFAULT 1,
    paused boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE organisations ENABLE ROW LEVEL SECURITY;
ALTER TABLE organisations FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON organisations USING (id = nullif(current_setting('reforge.org_id', true), '')::uuid) WITH CHECK (id = nullif(current_setting('reforge.org_id', true), '')::uuid);

CREATE TABLE audit_events (
    id uuid PRIMARY KEY,
    org_id uuid NOT NULL REFERENCES organisations(id),
    repository_id uuid,
    actor_id text NOT NULL,
    action text NOT NULL,
    object_id text NOT NULL,
    request_id text NOT NULL,
    data jsonb NOT NULL DEFAULT '{}',
    occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_events_org_time ON audit_events(org_id, occurred_at, id);
ALTER TABLE audit_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_events FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON audit_events USING (org_id = nullif(current_setting('reforge.org_id', true), '')::uuid) WITH CHECK (org_id = nullif(current_setting('reforge.org_id', true), '')::uuid);
