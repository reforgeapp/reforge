ALTER TABLE workflow_tasks ADD COLUMN model_route text NOT NULL DEFAULT 'default' CHECK(length(model_route) BETWEEN 1 AND 200);
