CREATE TABLE campaign_previews (
 org_id uuid NOT NULL REFERENCES organisations(id),
 id uuid NOT NULL,
 requested_by uuid NOT NULL REFERENCES users(id),
 document jsonb NOT NULL,
 expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(org_id,id)
);
CREATE TABLE campaigns (
 org_id uuid NOT NULL REFERENCES organisations(id),
 id uuid NOT NULL,
 preview_id uuid NOT NULL,
 idempotency_key text NOT NULL CHECK(length(idempotency_key) BETWEEN 1 AND 128),
 requested_by uuid NOT NULL REFERENCES users(id),
 requested_session_id uuid NOT NULL REFERENCES sessions(id),
 grant_expires_at timestamptz NOT NULL DEFAULT (now()+interval '30 days'),
 spec jsonb NOT NULL,
 name text NOT NULL,
 kind text NOT NULL CHECK(kind IN ('repair','pipeline','gitops')),
 state text NOT NULL DEFAULT 'planned' CHECK(state IN ('planned','canary','observing','expanding','paused','completed','cancelled','failed')),
 reason text NOT NULL DEFAULT '',
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 stage integer NOT NULL DEFAULT 0 CHECK(stage>=0),
 observing_since timestamptz,
 observe_due timestamptz NOT NULL DEFAULT now(),
 controller_id uuid,
 controller_until timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(org_id,id),
 UNIQUE(org_id,idempotency_key),
 FOREIGN KEY(org_id,preview_id) REFERENCES campaign_previews(org_id,id)
);
CREATE INDEX campaign_controller_due ON campaigns(org_id,observe_due,id);
CREATE TABLE campaign_members (
 org_id uuid NOT NULL,
 campaign_id uuid NOT NULL,
 id uuid NOT NULL,
 repository_id uuid NOT NULL,
 document jsonb NOT NULL,
 state text NOT NULL CHECK(state IN ('pending','excluded','queued','running','observing','succeeded','failed','blocked','unknown','cancelled')),
 reason text NOT NULL DEFAULT '',
 stage integer NOT NULL DEFAULT 0 CHECK(stage>=0),
 action_id uuid,
 gate_id uuid,
 succeeded_at timestamptz,
 halt boolean NOT NULL DEFAULT false,
 stop_complete boolean NOT NULL DEFAULT false,
 resume_requested boolean NOT NULL DEFAULT false,
 observe_due timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(org_id,campaign_id,id),
 UNIQUE(org_id,campaign_id,repository_id),
 FOREIGN KEY(org_id,campaign_id) REFERENCES campaigns(org_id,id),
 FOREIGN KEY(org_id,repository_id) REFERENCES repositories(org_id,id)
);
CREATE UNIQUE INDEX campaign_member_gate ON campaign_members(org_id,gate_id) WHERE gate_id IS NOT NULL;
CREATE UNIQUE INDEX campaign_member_action ON campaign_members(org_id,action_id) WHERE action_id IS NOT NULL;
CREATE TABLE campaign_repository_scopes (
 org_id uuid NOT NULL,
 campaign_id uuid NOT NULL,
 repository_id uuid NOT NULL,
 PRIMARY KEY(org_id,campaign_id,repository_id),
 FOREIGN KEY(org_id,campaign_id) REFERENCES campaigns(org_id,id),
 FOREIGN KEY(org_id,repository_id) REFERENCES repositories(org_id,id)
);
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['campaign_previews','campaigns','campaign_members','campaign_repository_scopes'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',t);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',t);
  EXECUTE format('CREATE POLICY tenant ON %I USING(org_id=nullif(current_setting(''reforge.org_id'',true),'''')::uuid) WITH CHECK(org_id=nullif(current_setting(''reforge.org_id'',true),'''')::uuid)',t);
 END LOOP;
END $$;
