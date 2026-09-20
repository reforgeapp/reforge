import { test, expect, type Page } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'
const repo = '00000000-0000-4000-8000-000000000002'
const base = `/api/v1/orgs/${org}`
const config = { environment: 'staging', repository_id: repo, version: 2, enabled: true, mode: 'observe', workflow: { id: '123', path: '.github/workflows/observe.yml', ref: 'refs/heads/main', sha: 'a'.repeat(40), config_sha256: 'b'.repeat(64), inputs: {} }, provenance_public_key: 'key', health_public_key: '', health_checks: [], observation_seconds: 60, max_evidence_age_seconds: 300, deadline_seconds: 600, qualification: { provider: '', server_version: '', connection_version: 0, evidence_reference: '', evidence_sha256: '', verified_at: '', expires_at: '', pinned_inputs: false, native_enforcement: false, environment_serialization: false, no_bypass: false } }
const request = { change_id: '12345', source_sha: 'c'.repeat(40), artifact_digest: 'sha256:' + 'd'.repeat(64), provenance: { document: { org_id: org, repository_id: repo, source_sha: 'c'.repeat(40), artifact_digest: 'sha256:' + 'd'.repeat(64), build_id: 'observe-build', issued_at: '2026-09-21T00:00:00Z', expires_at: '2026-09-22T00:00:00Z' }, signature: 'signed' } }
const operation = { id: '00000000-0000-4000-8000-000000000003', environment: 'staging', repository_id: repo, gate_id: '00000000-0000-4000-8000-000000000004', state: 'running', reason: 'Native run tracked', version: 1, requested_by: 'user-1', cancel_requested: false, cancel_state: '', created_at: '2026-09-21T00:00:00Z', updated_at: '2026-09-21T00:00:00Z', native: { workflow_sha: 'a'.repeat(40), created_at: '2026-09-21T00:00:00Z', id: '67890', state: 'in_progress', source_sha: request.source_sha, artifact_digest: request.artifact_digest, environment: 'staging', correlation_id: 'observe-correlation', workflow_id: '123', workflow_path: '.github/workflows/observe.yml', ref: 'refs/heads/main', event: 'workflow_dispatch', run_attempt: 1, url: 'https://forge.example/runs/42', health: 'unknown', observed_at: '', updated_at: '2026-09-21T00:00:00Z' } }
const gate = { id: '00000000-0000-4000-8000-000000000004', environment: 'staging', repository_id: repo, connection_id: 'forge-1', connection_version: 1, configuration_version: 2, request, pipeline: { workflow_sha: 'a'.repeat(40), rules_hash: 'rules', repository: { id: repo }, workflow_id: '123', workflow_path: '.github/workflows/observe.yml', config_sha256: 'b'.repeat(64), ref: 'refs/heads/main', source_sha: request.source_sha, artifact_digest: request.artifact_digest, environment: 'staging', correlation_id: 'observe-correlation', inputs: {}, observe_only: true, run_id: '67890', requested_at: '2026-09-21T00:00:00Z' }, native: { state: 'observed', native_enforced: 'unchanged', blockers: [], approval_url: '', environment: 'staging', rules_hash: 'rules' }, binding: {}, decision: { outcome: 'allow', blockers: [], required_actions: [] }, blockers: [], expires_at: '2026-09-22T00:00:00Z' }

async function fixture(page: Page, configuration = config) {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }], memberships: [{ org_id: org, role: 'maintainer', team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { development: true, fixture_auth: true } }))
  await page.route(`**${base}/deployment-configurations`, route => route.fulfill({ json: { items: [configuration] } }))
  await page.route(`**${base}/deployments?**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.goto(`/org/${org}/deployments`)
}

test('tracks native run without dispatch and keeps readonly authority', async ({ page }) => {
  await fixture(page)
  let tracked: Record<string, unknown> | undefined
  await page.route(`**${base}/deployment-configurations/staging/track`, async route => { tracked = route.request().postDataJSON() as Record<string, unknown>; await route.fulfill({ status: 201, json: operation }) })
  await page.route(`**${base}/deployments/00000000-0000-4000-8000-000000000003`, route => route.fulfill({ json: { operation, gate } }))
  await page.getByLabel('Change ID').fill(request.change_id)
  await page.getByLabel('Source SHA').fill(request.source_sha)
  await page.getByLabel('Artifact digest').fill(request.artifact_digest)
  await page.getByLabel('Native run ID').fill('67890')
  await page.getByLabel('Signed provenance JSON').fill(JSON.stringify(request.provenance))
  await page.getByRole('button', { name: 'Track native run' }).click()
  await expect.poll(() => tracked).toBeTruthy()
  expect(tracked).toMatchObject({ ...request, run_id: '67890' })
  expect(typeof tracked?.idempotency_key).toBe('string')
  await expect(page.getByText('Observe only · native authority unchanged')).toBeVisible()
  await expect(page.getByRole('link', { name: 'Open native pipeline' })).toHaveAttribute('href', operation.native.url)
  await expect(page.getByRole('button', { name: 'Cancel native pipeline' })).toHaveCount(0)
})

test('disabled observe configuration blocks tracking', async ({ page }) => {
  await fixture(page, { ...config, enabled: false })
  await expect(page.getByText('Blocked configuration: configuration disabled.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Track native run' })).toBeDisabled()
})

test('retries lost track response with same key and rotates after input change', async ({ page }) => {
  await fixture(page)
  const bodies: Array<Record<string, unknown>> = []
  let attempts = 0
  await page.route(`**${base}/deployment-configurations/staging/track`, async route => { bodies.push(route.request().postDataJSON() as Record<string, unknown>); attempts += 1; if (attempts === 1) return route.fulfill({ status: 503, json: { message: 'response lost' } }); return route.fulfill({ status: 201, json: operation }) })
  await page.route(`**${base}/deployments/00000000-0000-4000-8000-000000000003`, route => route.fulfill({ json: { operation, gate } }))
  await page.getByLabel('Change ID').fill(request.change_id)
  await page.getByLabel('Source SHA').fill(request.source_sha)
  await page.getByLabel('Artifact digest').fill(request.artifact_digest)
  await page.getByLabel('Native run ID').fill('67890')
  await page.getByLabel('Signed provenance JSON').fill(JSON.stringify(request.provenance))
  await page.getByRole('button', { name: 'Track native run' }).click()
  await expect(page.getByRole('alert')).toContainText('Track outcome unknown')
  await page.getByRole('button', { name: 'Track native run' }).click()
  await expect.poll(() => attempts).toBe(2)
  expect(bodies[0].idempotency_key).toBe(bodies[1].idempotency_key)
  await page.getByLabel('Source SHA').fill('f'.repeat(40))
  await page.getByRole('button', { name: 'Track native run' }).click()
  await expect.poll(() => attempts).toBe(3)
  expect(bodies[2].idempotency_key).not.toBe(bodies[1].idempotency_key)
})
