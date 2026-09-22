import { test, expect } from '@playwright/test'

const organisation = '00000000-0000-4000-8000-000000000001'
const finding = { id: 'finding-repair', org_id: organisation, repository_id: 'repo-1', source: 'advisory', source_id: 'CVE-REPAIR', category: 'security_advisory', severity: 'high', title: 'Repair preview fixture', evidence: { provenance: 'gitea', connection_id: 'forge-1', connection_version: 1, config_version: 1, head_sha: 'a'.repeat(40), target_sha: 'b'.repeat(40), target_branch: 'main', checks: [], dependencies: [], ownership: 'bot-owned', complete: true, blockers: [], merge_blockers: [] }, fingerprint: 'repair-fingerprint', evidence_digest: 'repair-digest', state: 'open', reason: 'Fixture finding', version: 3, first_seen: new Date().toISOString(), last_seen: new Date().toISOString() }

test.use({ trace: 'off' })

async function openFinding(page: import('@playwright/test').Page) {
  await page.goto('/auth/login')
  await expect(page).toHaveURL(/\/org\/[^/]+\/overview/)
  await page.goto(`/org/${organisation}/findings`)
  await page.getByRole('button', { name: finding.title }).click()
  return page.getByRole('region', { name: finding.title })
}

function installFindingRoutes(page: import('@playwright/test').Page) {
  void page.route(`**/api/v1/orgs/${organisation}/findings**`, route => route.fulfill({ json: { items: [finding], complete: true } }))
  void page.route(`**/api/v1/orgs/${organisation}/findings/${finding.id}`, route => route.fulfill({ json: finding }))
}

test('repair preview blocks when approved options are unavailable', async ({ page }) => {
  installFindingRoutes(page)
  await page.route(`**/api/v1/orgs/${organisation}/repair-recipes`, route => route.fulfill({ json: {} }))
  await page.route(`**/api/v1/orgs/${organisation}/connections*`, route => route.fulfill({ json: { items: [] } }))
  await page.route(`**/api/v1/orgs/${organisation}/custom-profiles*`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route(`**/api/v1/orgs/${organisation}/runner-pools`, route => route.fulfill({ json: { items: [] } }))
  const dialog = await openFinding(page)
  await expect(dialog.getByText('No operator images are configured.')).toBeVisible()
  await expect(dialog.getByRole('button', { name: 'Generate preview' })).toBeDisabled()
})

test('repair preview freezes evidence and retries queue with stable idempotency', async ({ page }) => {
  installFindingRoutes(page)
  await page.route(`**/api/v1/orgs/${organisation}/repair-recipes`, route => route.fulfill({ json: { go: 'sha256:' + 'c'.repeat(64) } }))
  await page.route(`**/api/v1/orgs/${organisation}/connections*`, route => route.fulfill({ json: { items: [{ id: 'model-1', name: 'Local model', kind: 'model', state: 'healthy', settings: { auth_kind: 'token', billing_route: 'direct_api' } }] } }))
  await page.route(`**/api/v1/orgs/${organisation}/custom-profiles*`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route(`**/api/v1/orgs/${organisation}/runner-pools`, route => route.fulfill({ json: { items: [{ id: 'pool-1', name: 'Repair pool', state: 'active', repository_ids: ['repo-1'] }] } }))
  const previewRequests: unknown[] = []
  await page.route(`**/api/v1/orgs/${organisation}/repair-preview`, async route => {
    previewRequests.push(JSON.parse(route.request().postData() ?? '{}'))
    await route.fulfill({ json: { context: { plan: { digest: 'plan-digest', baseline_sha: 'a'.repeat(40), target_sha: 'b'.repeat(40), max_changed_lines: 500, recipe: { max_files: 20, max_patch_bytes: 65536, max_turns: 4, timeout_seconds: 300, commands: [{ id: 'go-01', args: ['go', 'test', './...'], directory: '.', timeout_seconds: 60, report_format: 'json' }] } }, policy_hash: 'policy-hash', max_output_tokens: 4096, turn_timeout_ms: 120000 }, blockers: [], expires_at: new Date(Date.now() + 300000).toISOString() } })
  })
  const runRequests: Array<Record<string, unknown>> = []
  let runAttempts = 0
  await page.route(`**/api/v1/orgs/${organisation}/repair-runs`, async route => {
    runAttempts++
    runRequests.push(JSON.parse(route.request().postData() ?? '{}') as Record<string, unknown>)
    if (runAttempts === 1) {
      await route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ code: 'busy', message: 'retry', request_id: 'fixture', retryable: true }) })
      return
    }
    await route.fulfill({ json: { task: { id: 'task-1' }, state: 'queued', version: 1 } })
  })
  const dialog = await openFinding(page)
  await dialog.getByLabel('Repair recipe').selectOption('go')
  await dialog.getByLabel('Model connection').selectOption('model-1')
  await dialog.getByLabel('Runner pool').selectOption('pool-1')
  const generate = dialog.getByRole('button', { name: 'Generate preview' })
  await expect(generate).toBeEnabled()
  await generate.focus()
  await expect(generate).toBeFocused()
  await generate.click()
  await expect(dialog).toContainText('Head ' + 'a'.repeat(40))
  await expect(dialog).toContainText('Target ' + 'b'.repeat(40))
  await expect(dialog).toContainText('go-01: go test ./...')
  await expect(dialog).toContainText('500 changed lines')
  expect(previewRequests).toHaveLength(1)
  const queue = dialog.getByRole('button', { name: 'Confirm and queue repair' })
  await queue.click()
  await expect(dialog.getByText('retry')).toBeVisible()
  await queue.click()
  await expect(dialog.getByRole('link', { name: /Queued run task-1/ })).toBeVisible()
  expect(runRequests).toHaveLength(2)
  expect(runRequests[0].plan_digest).toBe('plan-digest')
  expect(runRequests[1].idempotency_key).toBe(runRequests[0].idempotency_key)
  await dialog.getByLabel('Model route').fill('alternate')
  await expect(dialog.getByRole('button', { name: 'Confirm and queue repair' })).toHaveCount(0)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
})

test('a repair can select an approved custom command profile', async ({ page }) => {
  installFindingRoutes(page)
  await page.route(`**/api/v1/orgs/${organisation}/repair-recipes`, route => route.fulfill({ json: { go: 'sha256:' + 'c'.repeat(64) } }))
  await page.route(`**/api/v1/orgs/${organisation}/connections*`, route => {
    const kind = new URL(route.request().url()).searchParams.get('kind')
    if (kind === 'agent') return route.fulfill({ json: { items: [{ id: 'agent-1', name: 'Custom runtime', kind: 'agent', provider: 'custom_command', state: 'healthy', settings: { auth_kind: 'official_runtime', billing_route: 'subscription', model: 'custom-model' } }] } })
    return route.fulfill({ json: { items: [{ id: 'model-1', name: 'Local model', kind: 'model', state: 'healthy', settings: { auth_kind: 'token', billing_route: 'direct_api' } }] } })
  })
  await page.route(`**/api/v1/orgs/${organisation}/custom-profiles*`, route => route.fulfill({ json: { items: [{ id: 'profile-1', name: 'Reviewed command', version: 4, approved_at: new Date().toISOString(), revoked_at: null, image_digest: 'sha256:' + 'b'.repeat(64), executable: '/bin/review', argv: [], protocol_version: 1, max_wall_seconds: 60, max_output_bytes: 1048576, max_turns: 2, concurrency: 1 }], complete: true } }))
  await page.route(`**/api/v1/orgs/${organisation}/runner-pools`, route => route.fulfill({ json: { items: [{ id: 'pool-1', name: 'Repair pool', state: 'active', repository_ids: ['repo-1'] }] } }))
  const previewRequests: Array<Record<string, unknown>> = []
  await page.route(`**/api/v1/orgs/${organisation}/repair-preview`, async route => {
    previewRequests.push(JSON.parse(route.request().postData() ?? '{}') as Record<string, unknown>)
    await route.fulfill({ json: { context: { plan: { digest: 'custom-plan', baseline_sha: 'a'.repeat(40), target_sha: 'b'.repeat(40), max_changed_lines: 500, recipe: { max_files: 20, max_patch_bytes: 65536, max_turns: 2, timeout_seconds: 60, commands: [] } }, policy_hash: 'policy-hash', max_output_tokens: 0, turn_timeout_ms: 60000 }, blockers: [], expires_at: new Date(Date.now() + 300000).toISOString() } })
  })
  const dialog = await openFinding(page)
  await dialog.getByLabel('Repair recipe').selectOption('go')
  await dialog.getByLabel('Custom command profile').selectOption('profile-1')
  await dialog.getByLabel('Model connection').selectOption('agent-1')
  await dialog.getByLabel('Runner pool').selectOption('pool-1')
  await dialog.getByRole('button', { name: 'Generate preview' }).click()
  await expect(dialog).toContainText('policy-hash')
  expect(previewRequests).toHaveLength(1)
  expect(previewRequests[0].custom_profile_id).toBe('profile-1')
  expect(previewRequests[0].custom_profile_version).toBe(4)
})

test('expired repair preview cannot be queued', async ({ page }) => {
  installFindingRoutes(page)
  await page.route(`**/api/v1/orgs/${organisation}/repair-recipes`, route => route.fulfill({ json: { go: 'sha256:' + 'c'.repeat(64) } }))
  await page.route(`**/api/v1/orgs/${organisation}/connections*`, route => route.fulfill({ json: { items: [{ id: 'model-1', name: 'Local model', kind: 'model', state: 'healthy', settings: { auth_kind: 'token', billing_route: 'direct_api' } }] } }))
  await page.route(`**/api/v1/orgs/${organisation}/custom-profiles*`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route(`**/api/v1/orgs/${organisation}/runner-pools`, route => route.fulfill({ json: { items: [{ id: 'pool-1', name: 'Repair pool', state: 'active', repository_ids: ['repo-1'] }] } }))
  await page.route(`**/api/v1/orgs/${organisation}/repair-preview`, route => route.fulfill({ json: { context: { plan: { digest: 'expired-plan', baseline_sha: 'a'.repeat(40), target_sha: 'b'.repeat(40), max_changed_lines: 1, recipe: { max_files: 1, max_patch_bytes: 1, max_turns: 1, timeout_seconds: 1, commands: [] } }, policy_hash: 'policy', max_output_tokens: 1, turn_timeout_ms: 1 }, blockers: [], expires_at: new Date(Date.now() - 1000).toISOString() } }))
  const dialog = await openFinding(page)
  await dialog.getByLabel('Repair recipe').selectOption('go'); await dialog.getByLabel('Model connection').selectOption('model-1'); await dialog.getByLabel('Runner pool').selectOption('pool-1')
  await dialog.getByRole('button', { name: 'Generate preview' }).click()
  const queue = dialog.getByRole('button', { name: 'Confirm and queue repair' })
  await expect(queue).toBeDisabled()
  await expect(dialog).toContainText('expired')
})
