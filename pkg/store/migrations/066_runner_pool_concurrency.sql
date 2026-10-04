ALTER TABLE runner_pools ADD COLUMN max_concurrent integer CHECK (max_concurrent BETWEEN 1 AND 256);
