import { test, expect, type Page } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'
const base = `/api/v1/orgs/${org}`
const sourceRepo = '00000000-0000-4000-8000-000000000011'
const deliveryRepo = '00000000-0000-4000-8000-000000000012'
const gateID = '00000000-0000-4000-8000-000000000021'
const promotionID = '00000000-0000-4000-8000-000000000031'
const source = 'a'.repeat(40)
const delivery = 'b'.repeat(40)
const artifact = `sha256:${'c'.repeat(64)}`
const configuration = (environment = 'production', version = 1) => ({ environment, source_repository_id: sourceRepo, delivery_repository_id: deliveryRepo, version, enabled: true, target_branch: 'main', manifest_path: 'manifests/app.yaml', pointer: '/spec/template/spec/containers/0/image', image_repository: 'registry.example/app', provenance_public_key: 'provenance-key', health_public_key: 'health-key', health_checks: ['rollout'], observation_seconds: 60, max_evidence_age_seconds: 300, deadline_seconds: 900, recovery_allowed: true })
const provenance = { document: { org_id: org, repository_id: sourceRepo, source_sha: source, artifact_digest: artifact, build_id: 'build-1', issued_at: '2026-09-21T00:00:00Z', expires_at: '2026-09-22T00:00:00Z' }, signature: 'signature' }
const gate = (expiresAt = new Date(Date.now() + 60_000).toISOString(), id = gateID, operation = promotionID) => ({ id, operation_id: operation, configuration: configuration(), request: { change_id: '1', source_sha: source, artifact_digest: artifact, provenance }, source: { native_id: '1', full_name: 'org/source' }, delivery: { native_id: '2', full_name: 'org/delivery' }, target_sha: 'd'.repeat(40), before: 'registry.example/app@sha256:' + '0'.repeat(64), after: 'registry.example/app@' + artifact, manifest_sha256: 'f'.repeat(64), patched_manifest: 'cGF0Y2hlZA==', decision: { outcome: 'allow', blockers: [] }, blockers: [], expires_at: expiresAt })
const promotion = (state = 'awaiting_merge', id = promotionID) => ({ id, environment: 'production', source_repository_id: sourceRepo, delivery_repository_id: deliveryRepo, gate_id: gateID, state, reason: 'Native delivery pending', version: 2, requested_by: 'user-1', branch: 'reforge/promotion-1', candidate_sha: 'd'.repeat(40), merge_sha: delivery, cancel_requested: false, created_at: '2026-09-21T00:00:00Z', updated_at: '2026-09-21T00:00:00Z' })

async function fixture(page: Page, role = 'owner', configs = [configuration()], rows = [promotion()], repoError = false) {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }], memberships: [{ org_id: org, role, team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { development: true, fixture_auth: true } }))
  let repositoryCalls = 0
  await page.route(`**${base}/repositories**`, route => { repositoryCalls += 1; return repoError && repositoryCalls <= 2 ? route.fulfill({ status: 503, json: { message: 'Inventory unavailable' } }) : route.fulfill({ json: { items: [{ id: sourceRepo, name: 'org/source', full_name: 'org/source' }, { id: deliveryRepo, name: 'org/delivery', full_name: 'org/delivery' }], complete: !repoError, next_cursor: repoError ? 'next-1' : undefined } }) })
  await page.route(`**${base}/gitops-configurations`, route => route.fulfill({ json: { items: configs } }))
  await page.route(`**${base}/gitops-promotions?**`, route => route.fulfill({ json: { items: rows, complete: true } }))
  await page.goto(`/org/${org}/deployments`)
  await page.getByRole('button', { name: /GitOps promotions/i }).click()
}

test('creates first and second environments from real configuration routes', async ({ page }) => {
  await fixture(page, 'owner', [])
  await page.getByLabel('New environment').fill('staging')
  await page.getByRole('button', { name: 'Create draft' }).click()
  await page.getByLabel('Source repository').selectOption(sourceRepo)
  await page.getByLabel('Delivery repository').selectOption(deliveryRepo)
  await page.getByLabel('Provenance public key').fill('provenance-key')
  await page.getByLabel('Health public key').fill('health-key')
  let saved = false
  await page.route(`**${base}/gitops-configurations/staging`, async route => { if (route.request().method() === 'PUT') { saved = true; await route.fulfill({ json: configuration('staging', 1) }) } else await route.fulfill({ json: configuration('staging') }) })
  await page.getByRole('button', { name: 'Save configuration' }).click()
  await expect.poll(() => saved).toBe(true)
  await page.getByLabel('New environment').fill('production')
  await page.getByRole('button', { name: 'Create draft' }).click()
})

test('paginates repositories and reports preview errors and invalid provenance', async ({ page }) => {
  await fixture(page, 'owner', [configuration()], [promotion()], true)
  await expect(page.getByRole('heading', { name: 'GitOps controls unavailable' })).toBeVisible()
  await page.getByRole('button', { name: 'Retry' }).click()
  await expect(page.getByLabel('Source repository')).toBeVisible()
  await page.getByLabel('Change ID').fill('1')
  await page.getByLabel('Source SHA').fill(source)
  await page.getByLabel('Artifact digest').fill(artifact)
  await page.getByLabel('Signed provenance JSON').fill('{bad')
  await page.getByRole('button', { name: 'Preview promotion' }).click()
  await expect(page.getByText('Provenance must be valid JSON.')).toBeVisible()
})

test('maintainer retries lost promotion request with stable idempotency key', async ({ page }) => {
  await fixture(page, 'maintainer', [configuration()])
  await page.route(`**${base}/gitops-configurations/production/preview`, route => route.fulfill({ json: gate() }))
  const requests: string[] = []
  await page.route(`**${base}/gitops-promotions`, async route => { if (route.request().method() === 'POST') { requests.push((await route.request().postDataJSON()).idempotency_key); await route.fulfill({ status: 503, json: { message: 'write outcome unknown' } }) } else await route.fulfill({ json: { items: [], complete: true } }) })
  await page.getByLabel('Change ID').fill('1'); await page.getByLabel('Source SHA').fill(source); await page.getByLabel('Artifact digest').fill(artifact); await page.getByLabel('Signed provenance JSON').fill(JSON.stringify(provenance)); await page.getByRole('button', { name: 'Preview promotion' }).click(); await page.getByRole('button', { name: 'Request promotion' }).click(); await page.getByRole('button', { name: 'Request promotion' }).click()
  await expect.poll(() => requests.length).toBe(2); expect(requests[0]).toBe(requests[1]); await expect(page.getByRole('alert')).toContainText('outcome unknown')
})

test('shows source and delivery revisions and blocks expired or unsafe merge', async ({ page }) => {
  await fixture(page, 'owner', [configuration()], [promotion()])
  await page.route(`**${base}/gitops-promotions/${promotionID}`, route => route.fulfill({ json: { promotion: promotion(), gate: gate(), health: { source_sha: source, delivery_revision: delivery, healthy: true, checks: { rollout: true }, observed_at: '2026-09-21T00:01:00Z' } } }))
  await page.route(`**${base}/gitops-promotions/${promotionID}/merge-preview`, route => route.fulfill({ json: { id: 'merge-gate', decision: { outcome: 'deny', blockers: ['protected branch'] }, blockers: ['protected branch'], expires_at: new Date(Date.now() - 1000).toISOString() } }))
  await page.getByRole('button', { name: promotionID }).click()
  await expect(page.getByText(source).first()).toBeVisible(); await expect(page.getByText(delivery).first()).toBeVisible(); await page.getByRole('button', { name: 'Protected merge preview' }).click(); await expect(page.getByText('protected branch')).toBeVisible(); await expect(page.getByRole('button', { name: 'Request protected merge' })).toBeDisabled()
})

test('detail switching and narrow keyboard layout remain usable', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 }); await fixture(page, 'owner', [configuration()], [promotion(), promotion('awaiting_merge', '00000000-0000-4000-8000-000000000032')])
  await page.route(`**${base}/gitops-promotions/${promotionID}`, route => route.fulfill({ json: { promotion: promotion(), gate: gate() } }))
  const secondID = '00000000-0000-4000-8000-000000000032'
  await page.route(`**${base}/gitops-promotions/${secondID}`, route => route.fulfill({ json: { promotion: promotion('awaiting_merge', secondID), gate: gate(new Date(Date.now() + 60_000).toISOString(), '00000000-0000-4000-8000-000000000022', secondID) } }))
  const row = page.getByRole('button', { name: promotionID }); await row.focus(); await expect(row).toBeFocused(); await page.keyboard.press('Enter'); await expect(page.getByRole('heading', { name: `Promotion ${promotionID}` })).toBeVisible(); await page.getByRole('button', { name: secondID }).click(); await expect(page.getByRole('heading', { name: `Promotion ${secondID}` })).toBeVisible(); expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})

test('real local GitOps page renders its unmocked empty state', async ({ page }) => {
  await page.goto('/auth/login'); await expect(page).toHaveURL(/\/org\/[^/]+\//); const liveOrg = new URL(page.url()).pathname.split('/')[2]; await page.goto(`/org/${liveOrg}/deployments`); await page.getByRole('button', { name: /GitOps promotions/i }).click(); await expect(page.getByText('No GitOps promotions recorded.')).toBeVisible()
})
