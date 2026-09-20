CREATE TABLE deployment_configurations (
 org_id uuid NOT NULL,
 environment text NOT NULL,
 repository_id uuid NOT NULL,
 version bigint NOT NULL CHECK(version>0),
 document jsonb NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(org_id,environment),
 FOREIGN KEY(org_id,repository_id) REFERENCES repositories(org_id,id)
);
CREATE TABLE deployment_gates (
 org_id uuid NOT NULL,
 id uuid NOT NULL,
 repository_id uuid NOT NULL,
 environment text NOT NULL,
 document jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(org_id,id),
 UNIQUE(org_id,id,repository_id,environment),
 FOREIGN KEY(org_id,repository_id) REFERENCES repositories(org_id,id),
 FOREIGN KEY(org_id,environment) REFERENCES deployment_configurations(org_id,environment)
);
CREATE TABLE deployments (
 org_id uuid NOT NULL,
 id uuid NOT NULL,
 repository_id uuid NOT NULL,
 environment text NOT NULL,
 gate_id uuid NOT NULL,
 requested_by uuid NOT NULL REFERENCES users(id),
 idempotency_key text NOT NULL,
 state text NOT NULL CHECK(state IN ('requested','dispatching','awaiting_gates','running','verifying','healthy','blocked','reconciling','failed','completed_unverified','recovery_requested','recovering','recovered','recovery_failed','cancelled')),
 reason text NOT NULL DEFAULT '',
 dispatch_id uuid,
 native_result jsonb,
 recovery_of uuid,
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 finished_at timestamptz,
 first_healthy_at timestamptz,
 last_health_at timestamptz,
 observe_after timestamptz NOT NULL DEFAULT now(),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(org_id,id),
 UNIQUE(org_id,idempotency_key),
 FOREIGN KEY(org_id,gate_id,repository_id,environment) REFERENCES deployment_gates(org_id,id,repository_id,environment),
 FOREIGN KEY(org_id,recovery_of) REFERENCES deployments(org_id,id)
);
CREATE UNIQUE INDEX deployment_environment_active ON deployments(org_id,environment) WHERE finished_at IS NULL AND state IN ('requested','dispatching','awaiting_gates','running','verifying','reconciling','completed_unverified','recovery_requested','recovering');
CREATE INDEX deployment_observe_due ON deployments(org_id,observe_after,id);
CREATE TABLE deployment_health_reports (
 org_id uuid NOT NULL,
 deployment_id uuid NOT NULL,
 nonce uuid NOT NULL,
 document jsonb NOT NULL,
 received_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(org_id,deployment_id,nonce),
 FOREIGN KEY(org_id,deployment_id) REFERENCES deployments(org_id,id)
);
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['deployment_configurations','deployment_gates','deployments','deployment_health_reports'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',t);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',t);
  EXECUTE format('CREATE POLICY tenant ON %I USING(org_id=nullif(current_setting(''reforge.org_id'',true),'''')::uuid) WITH CHECK(org_id=nullif(current_setting(''reforge.org_id'',true),'''')::uuid)',t);
 END LOOP;
END $$;
