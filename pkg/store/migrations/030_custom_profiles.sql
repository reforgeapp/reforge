CREATE TABLE custom_profiles (
 org_id uuid NOT NULL REFERENCES organisations(id),
 id uuid NOT NULL,
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 120),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 image_digest text NOT NULL CHECK(image_digest ~ '^sha256:[a-f0-9]{64}$'),
 executable text NOT NULL CHECK(length(executable) BETWEEN 1 AND 512),
 argv jsonb NOT NULL,
 protocol_version integer NOT NULL CHECK(protocol_version=1),
 max_wall_seconds integer NOT NULL CHECK(max_wall_seconds BETWEEN 1 AND 3600),
 max_output_bytes bigint NOT NULL CHECK(max_output_bytes BETWEEN 1 AND 16777216),
 max_turns integer NOT NULL CHECK(max_turns BETWEEN 1 AND 128),
 concurrency integer NOT NULL CHECK(concurrency BETWEEN 1 AND 8),
 approval_evidence text NOT NULL DEFAULT '',
 approved_by uuid REFERENCES users(id),
 approved_at timestamptz,
 revoked_at timestamptz,
 created_by uuid NOT NULL REFERENCES users(id),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(org_id,id)
);
CREATE INDEX custom_profiles_page ON custom_profiles(org_id,id);

CREATE TABLE custom_profile_runs (
 org_id uuid NOT NULL,
 id uuid NOT NULL,
 profile_id uuid NOT NULL,
 profile_version bigint NOT NULL,
 image_digest text NOT NULL,
 task_id uuid,
 requested_by uuid NOT NULL REFERENCES users(id),
 state text NOT NULL CHECK(state IN ('queued','running','completed_unverified','failed','unknown','timed_out','cancelled','blocked')),
 usage jsonb NOT NULL DEFAULT '{}'::jsonb,
 reason text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(org_id,id),
 FOREIGN KEY(org_id,profile_id) REFERENCES custom_profiles(org_id,id)
);
CREATE INDEX custom_profile_runs_page ON custom_profile_runs(org_id,profile_id,id);

DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['custom_profiles','custom_profile_runs'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',t);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',t);
  EXECUTE format('CREATE POLICY tenant ON %I USING(org_id=nullif(current_setting(''reforge.org_id'',true),'''')::uuid) WITH CHECK(org_id=nullif(current_setting(''reforge.org_id'',true),'''')::uuid)',t);
 END LOOP;
END $$;
