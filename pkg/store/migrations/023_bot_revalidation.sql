ALTER TABLE repair_runs ADD COLUMN bot_gate_id uuid;
ALTER TABLE repair_runs ADD COLUMN bot_revalidation_version bigint NOT NULL DEFAULT 0;
ALTER TABLE repair_runs ADD COLUMN bot_revalidation_state text NOT NULL DEFAULT 'pending' CHECK(bot_revalidation_state IN ('pending','waiting_companion','blocked','ready','merged','closed'));
ALTER TABLE repair_runs ADD COLUMN bot_revalidation_reason text NOT NULL DEFAULT '';
ALTER TABLE repair_runs ADD COLUMN bot_observe_after timestamptz NOT NULL DEFAULT now();
ALTER TABLE repair_runs ADD COLUMN bot_observed_at timestamptz;
ALTER TABLE repair_runs ADD CONSTRAINT repair_bot_gate FOREIGN KEY(org_id,bot_gate_id,repository_id) REFERENCES merge_gates(org_id,id,repository_id);
CREATE INDEX repair_bot_due ON repair_runs(org_id,bot_observe_after,task_id) WHERE state='published' AND bot_revalidation_state NOT IN ('merged','closed');
