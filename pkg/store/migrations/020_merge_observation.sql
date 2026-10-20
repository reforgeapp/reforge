ALTER TABLE merge_operations ADD COLUMN observe_after timestamptz NOT NULL DEFAULT now();
CREATE INDEX merge_observation_due ON merge_operations(org_id,observe_after,id) WHERE state IN ('requested','dispatching','queued','reconciling');
