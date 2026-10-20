CREATE UNIQUE INDEX deployment_native_run_identity ON deployments(org_id,repository_id,(native_result->>'id')) WHERE native_result->>'id'<>'';
