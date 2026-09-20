CREATE TABLE merge_execution_checks (
 org_id uuid NOT NULL,
 id uuid NOT NULL,
 operation_id uuid NOT NULL,
 gate_id uuid NOT NULL,
 sha text NOT NULL CHECK(sha ~ '^[0-9a-f]{40}$'),
 phase text NOT NULL CHECK(phase IN ('queue_admission','queue_execution')),
 state text NOT NULL CHECK(state IN ('prepared','dispatching','confirmed','uncertain')),
 native_id text NOT NULL DEFAULT '',
 dispatch_id uuid,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(org_id,id),
 UNIQUE(org_id,operation_id,sha),
 FOREIGN KEY(org_id,operation_id) REFERENCES merge_operations(org_id,id),
 FOREIGN KEY(org_id,gate_id) REFERENCES merge_gates(org_id,id)
);
ALTER TABLE merge_execution_checks ENABLE ROW LEVEL SECURITY;
ALTER TABLE merge_execution_checks FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON merge_execution_checks USING(org_id=nullif(current_setting('reforge.org_id',true),'')::uuid) WITH CHECK(org_id=nullif(current_setting('reforge.org_id',true),'')::uuid);
