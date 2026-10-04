CREATE TABLE private_routes (
 org_id uuid NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
 route text NOT NULL CHECK (length(route) BETWEEN 1 AND 100),
 pod text NOT NULL CHECK (length(pod) BETWEEN 1 AND 300),
 expires_at timestamptz NOT NULL,
 PRIMARY KEY (org_id, route, pod)
);
ALTER TABLE private_routes ENABLE ROW LEVEL SECURITY;
ALTER TABLE private_routes FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON private_routes USING (org_id=nullif(current_setting('reforge.org_id',true),'')::uuid) WITH CHECK (org_id=nullif(current_setting('reforge.org_id',true),'')::uuid);
