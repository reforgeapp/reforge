ALTER TABLE autopilot_attempts DROP CONSTRAINT autopilot_attempts_outcome_check;
ALTER TABLE autopilot_attempts ADD CONSTRAINT autopilot_attempts_outcome_check CHECK (outcome IN ('queued','skipped','retry','rebase','blocked'));
ALTER TABLE autopilot_attempts ADD COLUMN capability text NOT NULL DEFAULT '';
ALTER TABLE autopilot_attempts NO FORCE ROW LEVEL SECURITY;
UPDATE autopilot_attempts SET outcome = 'blocked' WHERE outcome = 'skipped' AND reason NOT LIKE 'Closed:%' AND reason NOT LIKE 'Dependabot rebase did not clear%';
ALTER TABLE autopilot_attempts FORCE ROW LEVEL SECURITY;
