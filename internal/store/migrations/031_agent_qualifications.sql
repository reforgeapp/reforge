CREATE TABLE agent_qualifications (
 org_id uuid NOT NULL REFERENCES organisations(id),
 connection_id uuid NOT NULL,
 evidence_id uuid NOT NULL,
 document jsonb NOT NULL,
 checked_at timestamptz NOT NULL,
 expires_at timestamptz NOT NULL,
 created_by uuid NOT NULL REFERENCES users(id),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(org_id,connection_id),
 FOREIGN KEY(org_id,connection_id) REFERENCES connections(org_id,id) ON DELETE CASCADE
);
ALTER TABLE agent_qualifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_qualifications FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON agent_qualifications USING(org_id=nullif(current_setting('reforge.org_id',true),'')::uuid) WITH CHECK(org_id=nullif(current_setting('reforge.org_id',true),'')::uuid);
