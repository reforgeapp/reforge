ALTER TABLE repair_runs ADD COLUMN candidate_artifacts jsonb NOT NULL DEFAULT '[]';
