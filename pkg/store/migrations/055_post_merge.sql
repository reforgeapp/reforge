ALTER TABLE repair_runs ADD COLUMN post_merge_state text NOT NULL DEFAULT '' CHECK (post_merge_state IN ('','pending','verified','regressed'));
ALTER TABLE repair_runs ADD COLUMN post_merge_since timestamptz;
