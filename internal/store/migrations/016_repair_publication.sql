ALTER TABLE repair_runs ADD COLUMN branch text NOT NULL DEFAULT '';
ALTER TABLE repair_runs ADD COLUMN candidate_sha text NOT NULL DEFAULT '';
ALTER TABLE repair_runs ADD COLUMN candidate_checks jsonb;
ALTER TABLE repair_runs ADD COLUMN native_change jsonb;
