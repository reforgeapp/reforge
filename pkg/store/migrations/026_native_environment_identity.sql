CREATE UNIQUE INDEX deployment_configuration_native_environment ON deployment_configurations(org_id,repository_id,(COALESCE(NULLIF(document->>'native_environment',''),environment)));
ALTER TABLE deployments ADD COLUMN native_environment text NOT NULL DEFAULT '';
UPDATE deployments d SET native_environment=COALESCE(g.document#>>'{pipeline,environment}',d.environment) FROM deployment_gates g WHERE g.org_id=d.org_id AND g.id=d.gate_id;
CREATE UNIQUE INDEX deployment_native_environment_active ON deployments(org_id,repository_id,native_environment) WHERE finished_at IS NULL AND state IN ('requested','dispatching','awaiting_gates','running','verifying','reconciling','completed_unverified','recovery_requested','recovering');
