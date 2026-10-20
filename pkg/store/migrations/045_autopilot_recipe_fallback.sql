ALTER TABLE autopilot_attempts NO FORCE ROW LEVEL SECURITY;
DELETE FROM autopilot_attempts WHERE outcome = 'skipped' AND reason = 'No supported recipe for this repository';
ALTER TABLE autopilot_attempts FORCE ROW LEVEL SECURITY;
