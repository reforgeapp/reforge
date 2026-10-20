CREATE TABLE maintenance_configs (
    org_id uuid NOT NULL,
    repository_id uuid NOT NULL,
    trusted_bots jsonb NOT NULL DEFAULT '[]',
    merge_authority text NOT NULL DEFAULT 'observe' CHECK(merge_authority IN ('observe','reforge','bot')),
    version bigint NOT NULL DEFAULT 1 CHECK(version>0),
    PRIMARY KEY(org_id,repository_id),
    FOREIGN KEY(org_id,repository_id) REFERENCES repositories(org_id,id) ON DELETE CASCADE
);
CREATE TABLE maintenance_findings (
    org_id uuid NOT NULL,
    id uuid NOT NULL,
    repository_id uuid NOT NULL,
    fingerprint text NOT NULL CHECK(length(fingerprint)=64),
    fingerprint_version integer NOT NULL DEFAULT 1 CHECK(fingerprint_version=1),
    source text NOT NULL,
    source_id text NOT NULL,
    category text NOT NULL,
    severity text NOT NULL CHECK(severity IN ('info','low','medium','high','critical')),
    title text NOT NULL,
    evidence jsonb NOT NULL,
    evidence_digest text NOT NULL CHECK(length(evidence_digest)=64),
    state text NOT NULL DEFAULT 'open' CHECK(state IN ('open','dismissed','snoozed','resolved','superseded')),
    reason text NOT NULL DEFAULT '',
    assigned_to uuid REFERENCES users(id),
    snooze_until timestamptz,
    superseded_by uuid,
    version bigint NOT NULL DEFAULT 1 CHECK(version>0),
    first_seen timestamptz NOT NULL DEFAULT now(),
    last_seen timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(org_id,id),
    UNIQUE(org_id,repository_id,fingerprint),
    UNIQUE(org_id,id,repository_id),
    FOREIGN KEY(org_id,repository_id) REFERENCES repositories(org_id,id) ON DELETE CASCADE,
    FOREIGN KEY(org_id,superseded_by) REFERENCES maintenance_findings(org_id,id)
);
CREATE INDEX maintenance_findings_page ON maintenance_findings(org_id,repository_id,state,id);
CREATE TABLE maintenance_observations (
    org_id uuid NOT NULL,
    id uuid NOT NULL,
    finding_id uuid NOT NULL,
    evidence_digest text NOT NULL,
    evidence jsonb NOT NULL,
    observed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(org_id,id),
    UNIQUE(org_id,finding_id,evidence_digest),
    FOREIGN KEY(org_id,finding_id) REFERENCES maintenance_findings(org_id,id) ON DELETE CASCADE
);
CREATE TABLE maintenance_repairs (
    org_id uuid NOT NULL,
    finding_id uuid NOT NULL,
    repository_id uuid NOT NULL,
    task_id uuid NOT NULL,
    evidence_digest text NOT NULL,
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(org_id,task_id),
    FOREIGN KEY(org_id,finding_id,repository_id) REFERENCES maintenance_findings(org_id,id,repository_id),
    FOREIGN KEY(org_id,task_id,repository_id) REFERENCES workflow_tasks(org_id,id,repository_id)
);
CREATE UNIQUE INDEX maintenance_one_active_repair ON maintenance_repairs(org_id,finding_id) WHERE active;
CREATE TABLE maintenance_scans (
    org_id uuid NOT NULL,
    repository_id uuid NOT NULL,
    requested_by uuid REFERENCES users(id),
    state text NOT NULL DEFAULT 'queued' CHECK(state IN ('queued','running','complete','failed','stale')),
    reason text NOT NULL DEFAULT '',
    fence bigint NOT NULL DEFAULT 0,
    lease_until timestamptz,
    available_at timestamptz NOT NULL DEFAULT now(),
    observed_at timestamptz,
    version bigint NOT NULL DEFAULT 1,
    PRIMARY KEY(org_id,repository_id),
    FOREIGN KEY(org_id,repository_id) REFERENCES repositories(org_id,id) ON DELETE CASCADE
);
CREATE INDEX maintenance_scans_due ON maintenance_scans(org_id,available_at,repository_id);
CREATE TABLE maintenance_scheduler_cursor (
    id boolean PRIMARY KEY DEFAULT true CHECK(id),
    org_id uuid NOT NULL
);
ALTER TABLE maintenance_scheduler_cursor ENABLE ROW LEVEL SECURITY;
ALTER TABLE maintenance_scheduler_cursor FORCE ROW LEVEL SECURITY;
CREATE POLICY scheduler_read ON maintenance_scheduler_cursor FOR SELECT USING(true);
CREATE POLICY scheduler_write ON maintenance_scheduler_cursor FOR ALL USING(current_setting('reforge.maintenance_scheduler',true)='true') WITH CHECK(current_setting('reforge.maintenance_scheduler',true)='true');
DO $$
DECLARE name text;
BEGIN
    FOREACH name IN ARRAY ARRAY['maintenance_configs','maintenance_findings','maintenance_observations','maintenance_repairs','maintenance_scans'] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',name);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',name);
        EXECUTE format('CREATE POLICY tenant ON %I USING (org_id=nullif(current_setting(''reforge.org_id'',true),'''')::uuid) WITH CHECK (org_id=nullif(current_setting(''reforge.org_id'',true),'''')::uuid)',name);
    END LOOP;
END;
$$;
