ALTER TABLE deployments ADD COLUMN cancel_requested boolean NOT NULL DEFAULT false;
ALTER TABLE deployments ADD COLUMN cancel_dispatch_id uuid;
ALTER TABLE deployments ADD COLUMN cancel_state text NOT NULL DEFAULT '' CHECK(cancel_state IN ('','pending','dispatching','uncertain','confirmed'));
