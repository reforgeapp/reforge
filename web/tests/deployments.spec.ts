import { test, expect, type Page } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'
const repo = 'repo-1'
const base = `/api/v1/orgs/${org}`
const configuration = {
  environment: 'production', repository_id: repo, version: 3, enabled: true, mode: 'pipeline',
  workflow: { id: 'build', path: '.github/workflows/build.yml', ref: 'main', sha: 'a'.repeat(40), config_sha256: 'b'.repeat(64), inputs: { region: 'au' } },
  provenance_public_key: 'provenance-key', health_public_key: 'health-key', health_checks: ['rollout'], observation_seconds: 60, max_evidence_age_seconds: 3600, deadline_seconds: 900,
  qualification: { provider: 'github', server_version: '3', connection_version: 7, evidence_reference: 'ticket-7', evidence_sha256: 'c'.repeat(64), verified_at: '2026-09-20T00:00:00Z', expires_at: '2026-12-20T00:00:00Z', pinned_inputs: true, native_enforcement: true, environment_serialization: true, no_bypass: true },
}
const request = { change_id: 'change-1', source_sha: 'd'.repeat(40), artifact_digest: 'sha256:' + 'e'.repeat(64), provenance: { document: { org_id: org, repository_id: repo, source_sha: 'd'.repeat(40), artifact_digest: 'sha256:' + 'e'.repeat(64), build_id: 'build-1', issued_at: '2026-09-21T00:00:00Z', expires_at: '2026-09-22T00:00:00Z' }, signature: 'signature' } }
const operation = (state = 'running') => ({ id: 'deployment-1', environment: 'production', repository_id: repo, gate_id: 'gate-1', state, reason: state === 'cancelled' ? 'Cancelled by operator' : 'Native execution pending', version: 2, requested_by: 'user-1', created_at: '2026-09-21T00:00:00Z', updated_at: '2026-09-21T00:00:00Z', native: { workflow_sha: 'f'.repeat(40), created_at: '2026-09-21T00:00:00Z', id: 'run-1', state: 'in_progress', source_sha: request.source_sha, artifact_digest: request.artifact_digest, environment: 'production', correlation_id: 'corr-1', workflow_id: 'build', workflow_path: '.github/workflows/build.yml', ref: 'main', event: 'workflow_dispatch', run_attempt: 1, url: 'https://forge.example/runs/1', health: 'unknown', observed_at: '', updated_at: '2026-09-21T00:00:00Z' } })
const gate = (expiresAt = new Date(Date.now() + 60_000).toISOString()) => ({ id: 'gate-1', environment: 'production', repository_id: repo, connection_id: 'forge-1', connection_version: 7, configuration_version: 3, request, pipeline: { workflow_sha: 'f'.repeat(40), rules_hash: 'rules-1', repository: { id: repo }, workflow_id: 'build', workflow_path: '.github/workflows/build.yml', config_sha256: 'b'.repeat(64), ref: 'main', source_sha: request.source_sha, artifact_digest: request.artifact_digest, environment: 'production', correlation_id: 'corr-1', inputs: {}, observe_only: false, run_id: '', requested_at: '2026-09-21T00:00:00Z' }, native: { state: 'eligible', native_enforced: 'enforced', blockers: [], approval_url: 'https://forge.example/approvals/1', environment: 'production', rules_hash: 'rules-1' }, binding: {}, decision: { outcome: 'allow', blockers: [], required_actions: [] }, blockers: [], expires_at: expiresAt })

async function fixture(page: Page, operations: unknown[] = [], config = configuration) {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }], memberships: [{ org_id: org, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { development: true, fixture_auth: true } }))
  await page.route(`**${base}/deployment-configurations`, route => route.fulfill({ json: { items: [config] } }))
  await page.route(`**${base}/deployments?**`, route => route.fulfill({ json: { items: operations, complete: true } }))
  await page.goto(`/org/${org}/deployments`)
}

test('previews, requests, and observes native pending then signed health', async ({ page }) => {
  await fixture(page)
  await page.route(`**${base}/deployment-configurations/production/preview`, route => route.fulfill({ json: gate() }))
  await page.route(`**${base}/deployments`, async route => route.request().method() === 'POST' ? route.fulfill({ json: operation() }) : route.fulfill({ json: { items: [], complete: true } }))
  await page.route(`**${base}/deployments/deployment-1`, route => route.fulfill({ json: { operation: operation(), gate: gate(), health: { org_id: org, deployment_id: 'deployment-1', environment: 'production', configuration_version: 3, source_sha: request.source_sha, artifact_digest: request.artifact_digest, run_id: 'run-1', run_attempt: 1, revision: 'rev-1', healthy: true, checks: { rollout: true }, observed_at: '2026-09-21T00:01:00Z', nonce: 'nonce-1' } } }))
  await page.getByLabel('Change ID').fill(request.change_id)
  await page.getByLabel('Source SHA').fill(request.source_sha)
  await page.getByLabel('Artifact digest').fill(request.artifact_digest)
  await page.getByLabel('Signed provenance JSON').fill(JSON.stringify(request.provenance))
  await page.getByRole('button', { name: 'Preview deployment gate' }).click()
  await expect(page.getByText('Native pipeline')).toBeVisible()
  await expect(page.getByRole('link', { name: 'Open native approval' })).toHaveAttribute('href', gate().native.approval_url)
  await page.getByRole('button', { name: 'Request deployment' }).click()
  await expect(page.getByRole('heading', { name: 'Deployment deployment-1' })).toBeVisible()
  await expect(page.getByText('Healthy')).toBeVisible()
  await expect(page.getByText(/rollout: pass/)).toBeVisible()
  await expect(page.getByText(/Workflow SHA/)).toBeVisible()
})

test('expired and invalid gates remain disabled', async ({ page }) => {
  await fixture(page)
  await page.route(`**${base}/deployment-configurations/production/preview`, route => route.fulfill({ json: gate('invalid-date') }))
  await page.getByLabel('Change ID').fill(request.change_id)
  await page.getByLabel('Source SHA').fill(request.source_sha)
  await page.getByLabel('Artifact digest').fill(request.artifact_digest)
  await page.getByRole('button', { name: 'Preview deployment gate' }).click()
  await expect(page.getByText('Preview expired; run a new preview.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Request deployment' })).toBeDisabled()
})

test('observe failure is actionable', async ({ page }) => {
  await fixture(page, [operation()])
  await page.route(`**${base}/deployments/deployment-1`, route => route.fulfill({ json: { operation: operation(), gate: gate() } }))
  await page.route(`**${base}/deployments/deployment-1/observe`, route => route.fulfill({ status: 503, json: { message: 'Native health unavailable' } }))
  await page.getByRole('button', { name: 'deployment-1' }).click()
  await page.getByRole('button', { name: 'Refresh native health' }).click()
  await expect(page.getByRole('alert')).toContainText('Native health unavailable')
})

test('cancelled operation remains visible with its reason', async ({ page }) => {
  await fixture(page, [operation('cancelled')])
  await page.route(`**${base}/deployments/deployment-1`, route => route.fulfill({ json: { operation: operation('cancelled'), gate: gate() } }))
  await expect(page.getByText('cancelled')).toBeVisible()
  await page.getByRole('button', { name: 'deployment-1' }).click()
  await expect(page.getByText('Cancelled by operator')).toBeVisible()
})

test('deployment detail stays usable at narrow width and supports keyboard focus', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await fixture(page, [operation()])
  await page.route(`**${base}/deployments/deployment-1`, route => route.fulfill({ json: { operation: operation(), gate: gate() } }))
  await page.getByRole('button', { name: 'deployment-1' }).focus()
  await expect(page.getByRole('button', { name: 'deployment-1' })).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('heading', { name: 'Deployment deployment-1' })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})

test('real development deployment state stays actionable without fixture routes', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/auth/login')
  await expect(page).toHaveURL(/\/org\//)
  await page.goto(`/org/${org}/deployments`)
  await expect(page.getByRole('button', { name: 'Deployment settings' })).toBeVisible()
  await expect(page.getByText('No deployments have been requested.')).toBeVisible()
  await page.getByRole('button', { name: 'Deployment settings' }).focus()
  await expect(page.getByRole('button', { name: 'Deployment settings' })).toBeFocused()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})
