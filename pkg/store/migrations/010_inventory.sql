CREATE TABLE inventory_tenants (
    org_id uuid PRIMARY KEY REFERENCES organisations(id) ON DELETE CASCADE
);
ALTER TABLE inventory_tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE inventory_tenants FORCE ROW LEVEL SECURITY;
CREATE POLICY scheduler_read ON inventory_tenants FOR SELECT USING (true);
CREATE POLICY tenant_write ON inventory_tenants FOR ALL USING (org_id=nullif(current_setting('reforge.org_id',true),'')::uuid) WITH CHECK (org_id=nullif(current_setting('reforge.org_id',true),'')::uuid);
CREATE FUNCTION register_inventory_tenant() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO inventory_tenants(org_id) VALUES(NEW.id) ON CONFLICT DO NOTHING;
    RETURN NEW;
END;
$$;
CREATE TRIGGER inventory_tenant_created AFTER INSERT ON organisations FOR EACH ROW EXECUTE FUNCTION register_inventory_tenant();
INSERT INTO inventory_tenants(org_id) SELECT id FROM organisations ON CONFLICT DO NOTHING;

ALTER TABLE repositories ADD CONSTRAINT repositories_connection_binding UNIQUE(org_id,id,connection_id);
CREATE TABLE inventory_sources (
    org_id uuid NOT NULL,
    connection_id uuid NOT NULL,
    namespace text NOT NULL DEFAULT '',
    poll_due timestamptz NOT NULL DEFAULT now(),
    last_served timestamptz NOT NULL DEFAULT '-infinity',
    PRIMARY KEY(org_id,connection_id),
    FOREIGN KEY(org_id,connection_id) REFERENCES connections(org_id,id) ON DELETE CASCADE
);
CREATE TABLE inventory_jobs (
    org_id uuid NOT NULL,
    id uuid NOT NULL,
    connection_id uuid NOT NULL,
    connection_version bigint NOT NULL CHECK(connection_version>0),
    namespace text NOT NULL DEFAULT '',
    kind text NOT NULL CHECK(kind IN ('scan','import','refresh')),
    repository_id uuid,
    parent_id uuid,
    requested_by uuid REFERENCES users(id),
    input jsonb NOT NULL DEFAULT '{}',
    state text NOT NULL DEFAULT 'queued' CHECK(state IN ('queued','running','complete','stale','failed','cancelled')),
    reason text NOT NULL DEFAULT '',
    cursor text NOT NULL DEFAULT '',
    phase text NOT NULL DEFAULT '',
    pages integer NOT NULL DEFAULT 0 CHECK(pages BETWEEN 0 AND 1000),
    processed integer NOT NULL DEFAULT 0 CHECK(processed BETWEEN 0 AND 100000),
    failures integer NOT NULL DEFAULT 0 CHECK(failures BETWEEN 0 AND 8),
    fencing_token bigint NOT NULL DEFAULT 0 CHECK(fencing_token>=0),
    lease_owner text NOT NULL DEFAULT '',
    lease_until timestamptz,
    available_at timestamptz NOT NULL DEFAULT now(),
    last_claimed timestamptz NOT NULL DEFAULT '-infinity',
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(org_id,id),
    UNIQUE(org_id,id,connection_id),
    UNIQUE(org_id,id,repository_id),
    FOREIGN KEY(org_id,connection_id) REFERENCES connections(org_id,id) ON DELETE CASCADE,
    FOREIGN KEY(org_id,repository_id) REFERENCES repositories(org_id,id) ON DELETE CASCADE,
    FOREIGN KEY(org_id,repository_id,connection_id) REFERENCES repositories(org_id,id,connection_id) ON DELETE CASCADE,
    FOREIGN KEY(org_id,parent_id,connection_id) REFERENCES inventory_jobs(org_id,id,connection_id),
    CHECK((kind='refresh')=(repository_id IS NOT NULL)),
    CHECK((kind='import')=(parent_id IS NOT NULL))
);
CREATE INDEX inventory_jobs_due ON inventory_jobs(org_id,available_at,last_claimed,id) WHERE state IN ('queued','running');
CREATE UNIQUE INDEX inventory_one_scan ON inventory_jobs(org_id,connection_id) WHERE kind='scan' AND state IN ('queued','running');
CREATE UNIQUE INDEX inventory_one_refresh ON inventory_jobs(org_id,repository_id) WHERE kind='refresh' AND state IN ('queued','running');
CREATE TABLE inventory_candidates (
    org_id uuid NOT NULL,
    job_id uuid NOT NULL,
    native_id text NOT NULL CHECK(length(native_id) BETWEEN 1 AND 256),
    repository jsonb NOT NULL,
    PRIMARY KEY(org_id,job_id,native_id),
    FOREIGN KEY(org_id,job_id) REFERENCES inventory_jobs(org_id,id) ON DELETE CASCADE
);
CREATE TABLE inventory_repository_state (
    org_id uuid NOT NULL,
    repository_id uuid NOT NULL,
    connection_id uuid NOT NULL,
    namespace text NOT NULL,
    connection_version bigint NOT NULL,
    scan_id uuid NOT NULL,
    state text NOT NULL DEFAULT 'fresh' CHECK(state IN ('fresh','stale','missing')),
    reason text NOT NULL DEFAULT '',
    refresh_due timestamptz NOT NULL DEFAULT now(),
    changes_observed_at timestamptz,
    PRIMARY KEY(org_id,repository_id),
    FOREIGN KEY(org_id,repository_id) REFERENCES repositories(org_id,id) ON DELETE CASCADE,
    FOREIGN KEY(org_id,repository_id,connection_id) REFERENCES repositories(org_id,id,connection_id) ON DELETE CASCADE,
    FOREIGN KEY(org_id,connection_id) REFERENCES connections(org_id,id) ON DELETE CASCADE,
    FOREIGN KEY(org_id,scan_id,connection_id) REFERENCES inventory_jobs(org_id,id,connection_id)
);
CREATE INDEX inventory_repository_due ON inventory_repository_state(org_id,refresh_due,repository_id);
CREATE TABLE inventory_changes (
    org_id uuid NOT NULL,
    repository_id uuid NOT NULL,
    native_id text NOT NULL,
    snapshot jsonb NOT NULL,
    job_id uuid NOT NULL,
    observed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(org_id,repository_id,native_id),
    FOREIGN KEY(org_id,repository_id) REFERENCES repositories(org_id,id) ON DELETE CASCADE,
    FOREIGN KEY(org_id,job_id,repository_id) REFERENCES inventory_jobs(org_id,id,repository_id)
);
CREATE TABLE inventory_webhooks (
    org_id uuid NOT NULL,
    id uuid NOT NULL,
    connection_id uuid NOT NULL,
    envelope jsonb NOT NULL,
    version bigint NOT NULL DEFAULT 1,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(org_id,id),
    UNIQUE(org_id,connection_id),
    FOREIGN KEY(org_id,connection_id) REFERENCES connections(org_id,id) ON DELETE CASCADE
);
CREATE TABLE inventory_deliveries (
    org_id uuid NOT NULL,
    endpoint_id uuid NOT NULL,
    delivery_key text NOT NULL,
    digest text NOT NULL CHECK(length(digest)=64),
    received_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(org_id,endpoint_id,delivery_key),
    FOREIGN KEY(org_id,endpoint_id) REFERENCES inventory_webhooks(org_id,id) ON DELETE CASCADE
);
CREATE INDEX inventory_delivery_retention ON inventory_deliveries(org_id,received_at);
DO $$
DECLARE name text;
BEGIN
    FOREACH name IN ARRAY ARRAY['inventory_sources','inventory_jobs','inventory_candidates','inventory_repository_state','inventory_changes','inventory_webhooks','inventory_deliveries'] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',name);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',name);
        EXECUTE format('CREATE POLICY tenant ON %I USING (org_id=nullif(current_setting(''reforge.org_id'',true),'''')::uuid) WITH CHECK (org_id=nullif(current_setting(''reforge.org_id'',true),'''')::uuid)',name);
    END LOOP;
END;
$$;
