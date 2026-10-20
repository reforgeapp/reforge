ALTER TABLE autopilot_attempts DROP CONSTRAINT autopilot_attempts_outcome_check;
ALTER TABLE autopilot_attempts ADD CONSTRAINT autopilot_attempts_outcome_check CHECK (outcome IN ('queued','skipped','retry','rebase'));
