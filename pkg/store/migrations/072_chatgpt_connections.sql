CREATE TABLE chatgpt_setups (
    org_id uuid NOT NULL REFERENCES organisations(id),
    id uuid NOT NULL,
    user_id uuid NOT NULL REFERENCES users(id),
    session_hash text NOT NULL,
    browser_hash text NOT NULL,
    state_hash text NOT NULL,
    envelope jsonb NOT NULL,
    phase text NOT NULL CHECK(phase IN ('pending','exchanging','authenticated')),
    expires_at timestamptz NOT NULL DEFAULT now()+interval '10 minutes',
    PRIMARY KEY(org_id,id)
);
ALTER TABLE chatgpt_setups ENABLE ROW LEVEL SECURITY;
ALTER TABLE chatgpt_setups FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON chatgpt_setups USING(org_id=nullif(current_setting('reforge.org_id',true),'')::uuid) WITH CHECK(org_id=nullif(current_setting('reforge.org_id',true),'')::uuid);
