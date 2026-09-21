import { test, expect, type Page } from '@playwright/test'

const organisation = '00000000-0000-4000-8000-000000000001'
const repository = { id: 'repo-1', name: 'payments', connection_id: 'forge-1' }
const configuration = {
  environment: 'production', repository_id: repository.id, version: 3, enabled: true, mode: 'pipeline',
  workflow: { id: 'build', path: '.github/workflows/build.yml', ref: 'main', sha: 'a'.repeat(40), config_sha256: 'b'.repeat(64), inputs: { region: 'au' } },
  provenance_public_key: 'provenance-key', health_public_key: 'health-key', health_checks: ['https://health.example.test'], observation_seconds: 60, max_evidence_age_seconds: 3600, deadline_seconds: 900,
  qualification: { provider: 'github', server_version: '3', connection_version: 7, evidence_reference: 'ticket-7', evidence_sha256: 'c'.repeat(64), verified_at: '2026-09-20T00:00:00Z', expires_at: '2026-12-20T00:00:00Z', pinned_inputs: true, native_enforcement: true, environment_serialization: true, no_bypass: true },
}

async function mockSession(page: Page) {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: organisation, name: 'Fixture', version: 1, paused: false }], memberships: [{ org_id: organisation, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { development: true, fixture_auth: true } }))
}

test('loads scoped settings and saves with version and CSRF', async ({ page }) => {
  await mockSession(page)
  await page.route(`**/api/v1/orgs/${organisation}/deployments**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route(`**/api/v1/orgs/${organisation}/deployment-configurations`, route => route.fulfill({ json: { items: [configuration] } }))
  await page.route(`**/api/v1/orgs/${organisation}/repositories**`, route => route.fulfill({ json: { items: [repository], complete: true } }))
  await page.route(`**/api/v1/orgs/${organisation}/connections**`, route => route.fulfill({ json: { items: [{ id: 'forge-1', name: 'GitHub', provider: 'github', version: 7, state: 'healthy' }], complete: true } }))
  await page.route(`**/api/v1/orgs/${organisation}/repositories/${repository.id}/delivery-workflows`, route => route.fulfill({ json: { items: [{ id: 'build', name: 'Build', path: '.github/workflows/build.yml', ref: 'main', url: 'https://github.test/build' }] } }))
  let savedBody: Record<string, unknown> | undefined
  await page.route(`**/api/v1/orgs/${organisation}/deployment-configurations/production`, async route => { savedBody = JSON.parse(route.request().postData() ?? '{}') as Record<string, unknown>; expect(route.request().headers()['if-match']).toBe('"3"'); expect(route.request().headers()['x-csrf-token']).toBeTruthy(); await route.fulfill({ json: { ...configuration, ...(savedBody ?? {}), version: 4 } }) })
  await page.goto(`/org/${organisation}/deployments`)
  await page.getByRole('button', { name: /Deployment settings/i }).click()
  await expect(page.getByRole('heading', { name: 'Deployment settings' })).toBeVisible()
  await page.getByRole('combobox', { name: 'Mode' }).selectOption('observe')
  await page.getByRole('button', { name: 'Save deployment settings' }).click()
  await expect(page.getByText('Deployment settings saved.', { exact: true })).toBeVisible()
  expect(savedBody?.mode).toBe('observe')
})

test('keeps edited form on optimistic concurrency conflict', async ({ page }) => {
  await mockSession(page)
  await page.route(`**/api/v1/orgs/${organisation}/deployments**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route(`**/api/v1/orgs/${organisation}/deployment-configurations`, route => route.fulfill({ json: { items: [configuration] } }))
  await page.route(`**/api/v1/orgs/${organisation}/repositories**`, route => route.fulfill({ json: { items: [repository], complete: true } }))
  await page.route(`**/api/v1/orgs/${organisation}/connections**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route(`**/api/v1/orgs/${organisation}/repositories/${repository.id}/delivery-workflows`, route => route.fulfill({ json: { items: [] } }))
  await page.route(`**/api/v1/orgs/${organisation}/deployment-configurations/production`, route => route.fulfill({ status: 409, contentType: 'application/json', body: JSON.stringify({ code: 'stale', message: 'Settings changed; reload before saving.', request_id: 'req-1', retryable: false }) }))
  await page.goto(`/org/${organisation}/deployments`)
  await page.getByRole('button', { name: /Deployment settings/i }).click()
  await expect(page.getByRole('heading', { name: 'Deployment settings' })).toBeVisible()
  await page.getByText('Keys and health', { exact: true }).click()
  await page.getByLabel('Deadline seconds').fill('901')
  await page.getByRole('button', { name: 'Save deployment settings' }).click()
  await expect(page.getByRole('alert')).toContainText('Settings changed')
  await expect(page.getByLabel('Deadline seconds')).toHaveValue('901')
})

test('creates a new environment with valid workflow inputs', async ({ page }) => {
  await mockSession(page)
  await page.route(`**/api/v1/orgs/${organisation}/deployments**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route(`**/api/v1/orgs/${organisation}/deployment-configurations`, route => route.fulfill({ json: { items: [configuration] } }))
  await page.route(`**/api/v1/orgs/${organisation}/repositories**`, route => route.fulfill({ json: { items: [repository], complete: true } }))
  await page.route(`**/api/v1/orgs/${organisation}/connections**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route(`**/api/v1/orgs/${organisation}/repositories/${repository.id}/delivery-workflows`, route => route.fulfill({ json: { items: [{ id: 'build', name: 'Build', path: '.github/workflows/build.yml', ref: 'main', url: 'https://forge.test/build' }] } }))
  let saved = false
  await page.route(`**/api/v1/orgs/${organisation}/deployment-configurations/staging`, async route => { saved = true; await route.fulfill({ json: { ...configuration, environment: 'staging', version: 1 } }) })
  await page.goto(`/org/${organisation}/deployments`)
  await page.getByRole('button', { name: /Deployment settings/i }).click()
  await page.locator('section[aria-label="Deployment settings"] select').nth(1).selectOption('')
  await page.getByLabel('Environment name').fill('staging')
  await page.getByLabel('Workflow ID').fill('build')
  await page.getByLabel('Path').fill('.github/workflows/build.yml')
  await page.locator('section[aria-label="Deployment settings"] details').first().getByRole('textbox').nth(2).fill('refs/heads/main')
  await page.getByRole('button', { name: 'Save deployment settings' }).click()
  await expect.poll(() => saved).toBe(true)
})

test('rejects invalid fixed inputs before saving', async ({ page }) => {
  await mockSession(page)
  await page.route(`**/api/v1/orgs/${organisation}/deployments**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route(`**/api/v1/orgs/${organisation}/deployment-configurations`, route => route.fulfill({ json: { items: [configuration] } }))
  await page.route(`**/api/v1/orgs/${organisation}/repositories**`, route => route.fulfill({ json: { items: [repository], complete: true } }))
  await page.route(`**/api/v1/orgs/${organisation}/connections**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route(`**/api/v1/orgs/${organisation}/repositories/${repository.id}/delivery-workflows`, route => route.fulfill({ json: { items: [] } }))
  let saved = false
  await page.route(`**/api/v1/orgs/${organisation}/deployment-configurations/production`, async route => { saved = true; await route.fulfill({ json: configuration }) })
  await page.goto(`/org/${organisation}/deployments`)
  await page.getByRole('button', { name: /Deployment settings/i }).click()
  await page.getByLabel('Fixed inputs (JSON)').fill('[]')
  await page.getByRole('button', { name: 'Save deployment settings' }).click()
  await expect(page.getByText('Fixed inputs must be a JSON object of string values.')).toBeVisible()
  expect(saved).toBe(false)
})
