CREATE TABLE merge_configurations (
 org_id uuid NOT NULL,
 repository_id uuid NOT NULL,
 version bigint NOT NULL CHECK(version>0),
 document jsonb NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(org_id,repository_id),
 FOREIGN KEY(org_id,repository_id) REFERENCES repositories(org_id,id)
);
CREATE TABLE merge_gates (
 org_id uuid NOT NULL,
 id uuid NOT NULL,
 repository_id uuid NOT NULL,
 change_id text NOT NULL,
 configuration_version bigint NOT NULL,
 document jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(org_id,id),
 UNIQUE(org_id,id,repository_id),
 FOREIGN KEY(org_id,repository_id) REFERENCES repositories(org_id,id)
);
CREATE TABLE merge_operations (
 org_id uuid NOT NULL,
 id uuid NOT NULL,
 repository_id uuid NOT NULL,
 gate_id uuid NOT NULL,
 requested_gate_id uuid NOT NULL,
 change_id text NOT NULL,
 target_branch text NOT NULL,
 idempotency_key text NOT NULL,
 requested_by uuid NOT NULL REFERENCES users(id),
 state text NOT NULL CHECK(state IN ('requested','dispatching','queued','reconciling','merged','blocked','cancelled','closed')),
 reason text NOT NULL DEFAULT '',
 native_result jsonb,
 cancel_requested boolean NOT NULL DEFAULT false,
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(org_id,id),
 UNIQUE(org_id,repository_id,idempotency_key),
 FOREIGN KEY(org_id,gate_id,repository_id) REFERENCES merge_gates(org_id,id,repository_id)
);
CREATE UNIQUE INDEX merge_one_target ON merge_operations(org_id,repository_id,target_branch) WHERE state IN ('requested','dispatching','queued','reconciling');
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['merge_configurations','merge_gates','merge_operations'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',t);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',t);
  EXECUTE format('CREATE POLICY tenant ON %I USING(org_id=nullif(current_setting(''reforge.org_id'',true),'''')::uuid) WITH CHECK(org_id=nullif(current_setting(''reforge.org_id'',true),'''')::uuid)',t);
 END LOOP;
END $$;
