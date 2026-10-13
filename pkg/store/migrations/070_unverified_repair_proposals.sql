ALTER TABLE repair_runs DROP CONSTRAINT repair_runs_state_check;
ALTER TABLE repair_runs ADD CONSTRAINT repair_runs_state_check CHECK(state IN ('queued','validated','unverified','handoff','publishing','published','uncertain'));
