CREATE TABLE gitops_configurations (
 org_id uuid NOT NULL,
 environment text NOT NULL,
 source_repository_id uuid NOT NULL,
 delivery_repository_id uuid NOT NULL,
 version bigint NOT NULL CHECK(version>0),
 document jsonb NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(org_id,environment),
 FOREIGN KEY(org_id,source_repository_id) REFERENCES repositories(org_id,id),
 FOREIGN KEY(org_id,delivery_repository_id) REFERENCES repositories(org_id,id)
);
CREATE UNIQUE INDEX gitops_target_identity ON gitops_configurations(org_id,delivery_repository_id,(document->>'target_branch'),(document->>'manifest_path'),(document->>'pointer'));
CREATE TABLE gitops_gates (
 org_id uuid NOT NULL,
 id uuid NOT NULL,
 environment text NOT NULL,
 source_repository_id uuid NOT NULL,
 delivery_repository_id uuid NOT NULL,
 document jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(org_id,id),
 UNIQUE(org_id,id,environment,source_repository_id,delivery_repository_id),
 FOREIGN KEY(org_id,environment) REFERENCES gitops_configurations(org_id,environment),
 FOREIGN KEY(org_id,source_repository_id) REFERENCES repositories(org_id,id),
 FOREIGN KEY(org_id,delivery_repository_id) REFERENCES repositories(org_id,id)
);
CREATE TABLE gitops_promotions (
 org_id uuid NOT NULL,
 id uuid NOT NULL,
 environment text NOT NULL,
 source_repository_id uuid NOT NULL,
 delivery_repository_id uuid NOT NULL,
 gate_id uuid NOT NULL,
 requested_by uuid NOT NULL REFERENCES users(id),
 idempotency_key text NOT NULL,
 state text NOT NULL CHECK(state IN ('requested','staging','stage_uncertain','staged','publishing','publish_uncertain','awaiting_merge','verifying','healthy','blocked','failed','completed_unverified','recovered','recovery_failed','cancelled')),
 reason text NOT NULL DEFAULT '',
 branch text NOT NULL,
 target_branch text NOT NULL,
 manifest_path text NOT NULL,
 pointer text NOT NULL,
 cancel_requested boolean NOT NULL DEFAULT false,
 candidate_sha text NOT NULL DEFAULT '',
 native_change jsonb,
 merge_sha text NOT NULL DEFAULT '',
 stage_dispatch_id uuid,
 publish_dispatch_id uuid,
 recovery_of uuid,
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 first_healthy_at timestamptz,
 last_health_at timestamptz,
 finished_at timestamptz,
 observe_after timestamptz NOT NULL DEFAULT now(),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(org_id,id),
 UNIQUE(org_id,idempotency_key),
 FOREIGN KEY(org_id,gate_id,environment,source_repository_id,delivery_repository_id) REFERENCES gitops_gates(org_id,id,environment,source_repository_id,delivery_repository_id),
 FOREIGN KEY(org_id,recovery_of) REFERENCES gitops_promotions(org_id,id)
);
CREATE UNIQUE INDEX gitops_environment_active ON gitops_promotions(org_id,environment) WHERE finished_at IS NULL AND state NOT IN ('blocked','failed','recovery_failed','cancelled');
CREATE UNIQUE INDEX gitops_target_active ON gitops_promotions(org_id,delivery_repository_id,target_branch,manifest_path,pointer) WHERE finished_at IS NULL AND state NOT IN ('blocked','failed','recovery_failed','cancelled');
CREATE UNIQUE INDEX gitops_change_identity ON gitops_promotions(org_id,delivery_repository_id,(native_change->>'id')) WHERE native_change IS NOT NULL;
CREATE INDEX gitops_observe_due ON gitops_promotions(org_id,observe_after,id);
CREATE TABLE gitops_health_reports (
 org_id uuid NOT NULL,
 promotion_id uuid NOT NULL,
 nonce uuid NOT NULL,
 document jsonb NOT NULL,
 received_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(org_id,promotion_id,nonce),
 FOREIGN KEY(org_id,promotion_id) REFERENCES gitops_promotions(org_id,id)
);
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['gitops_configurations','gitops_gates','gitops_promotions','gitops_health_reports'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',t);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',t);
  EXECUTE format('CREATE POLICY tenant ON %I USING(org_id=nullif(current_setting(''reforge.org_id'',true),'''')::uuid) WITH CHECK(org_id=nullif(current_setting(''reforge.org_id'',true),'''')::uuid)',t);
 END LOOP;
END $$;
