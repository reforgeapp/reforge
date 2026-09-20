import { test, expect, type Page } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'
const repo = 'repo-revalidation'
const change = 'change-revalidation'
const base = `/api/v1/orgs/${org}`
const changeBody = { id: change, title: 'Revalidate dependency', body: '', url: 'https://forge.example/change/1', head_sha: 'a'.repeat(40), target_sha: 'b'.repeat(40), head_branch: 'repair', target_branch: 'main', state: 'open', merge_status: 'clean', operation_id: '', author_login: 'bot', author_type: 'bot', repository: { native_id: 'repo-1', full_name: 'team/repo' }, head_repository: { native_id: 'repo-1', full_name: 'team/repo' }, target_repository: { native_id: 'repo-1', full_name: 'team/repo' } }

test.use({ trace: 'off' })

async function fixture(page: Page, items: unknown[], role = 'viewer') {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }], memberships: [{ org_id: org, role, team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route(`**${base}/repositories?**`, route => route.fulfill({ json: { items: [{ id: repo, name: 'team/repo', provider: 'gitea', team_ids: [], accessible: true }], complete: true } }))
  await page.route(`**${base}/repositories/${repo}/changes?**`, route => route.fulfill({ json: { items: [changeBody], complete: true } }))
  await page.route(`**${base}/merge-operations*`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route(`**${base}/repositories/${repo}/changes/${change}/revalidations`, route => route.fulfill({ json: { items } }))
  await page.goto(`/org/${org}/changes?repository=${repo}&change=${change}`)
  await page.getByRole('button', { name: changeBody.title }).click()
}

function revalidation(state: string, gate?: Record<string, unknown>) {
  return { task_id: 'task-revalidation', change_id: change, companion_id: 'companion-1', state, reason: state === 'blocked' ? 'Companion native change is not merged.' : 'Native gate refreshed after companion merge.', observed_at: '2026-09-21T01:02:03Z', ...(gate ? { gate } : {}) }
}

function gate(expiresAt: string) {
  return { id: 'gate-revalidation', expires_at: expiresAt, snapshot: { train_gate: { pipeline_id: 'pipeline-1', job_id: 'job-1', sha: 'a'.repeat(40), state: 'passed', ci_config_sha256: 'b'.repeat(64), checks_ready: true } } }
}

test('ready expired revalidation is stale and read-only', async ({ page }) => {
  await fixture(page, [revalidation('ready', gate('2026-09-20T00:00:00Z'))])
  await expect(page.getByRole('heading', { name: 'Automatic bot revalidation' })).toBeVisible()
  await expect(page.getByText('stale · refresh needed')).toBeVisible()
  await expect(page.getByText(/Observed/)).toBeVisible()
  await expect(page.getByText(/Pipeline pipeline-1 · job job-1 · passed/)).toBeVisible()
  await expect(page.getByRole('button', { name: 'Preview merge gate' })).toBeDisabled()
})

test('ready revalidation without gate is stale', async ({ page }) => {
  await fixture(page, [revalidation('ready')])
  await expect(page.getByText('stale · refresh needed')).toBeVisible()
})

test('ready revalidation becomes stale after gate expiry', async ({ page }) => {
  await page.clock.install({ time: new Date('2026-09-21T00:00:00Z') })
  await fixture(page, [revalidation('ready', gate('2026-09-21T00:00:10Z'))])
  await expect(page.getByText('ready', { exact: true })).toBeVisible()
  await page.clock.fastForward(11_000)
  await expect(page.getByText('stale · refresh needed')).toBeVisible()
})

test('blocked revalidation shows reason without approval action', async ({ page }) => {
  await fixture(page, [revalidation('blocked')])
  await expect(page.getByText('blocked')).toBeVisible()
  await expect(page.getByText('Companion native change is not merged.')).toBeVisible()
  await expect(page.getByRole('region', { name: 'Automatic bot revalidation' }).getByRole('button')).toHaveCount(0)
})

test('empty revalidation response adds no panel and stays usable at narrow width', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await fixture(page, [])
  await expect(page.getByRole('heading', { name: 'Automatic bot revalidation' })).toHaveCount(0)
  await page.getByRole('button', { name: 'Merge settings', exact: true }).focus()
  await expect(page.getByRole('button', { name: 'Merge settings', exact: true })).toBeFocused()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})

test('nonempty revalidation stays usable with long task ID at narrow width', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await fixture(page, [{ ...revalidation('waiting_companion'), task_id: 'revalidation-task-1234567890-abcdef0123456789' }])
  await expect(page.getByText('revalidation-task-1234567890-abcdef0123456789')).toBeVisible()
  await page.getByRole('button', { name: 'Merge settings', exact: true }).focus()
  await page.keyboard.press('Tab')
  await page.keyboard.press('Shift+Tab')
  await expect(page.getByRole('button', { name: 'Merge settings', exact: true })).toBeFocused()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})

test('revalidation failure offers retry', async ({ page }) => {
  const revalidationURL = `**${base}/repositories/${repo}/changes/${change}/revalidations`
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }], memberships: [{ org_id: org, role: 'viewer', team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route(`**${base}/repositories?**`, route => route.fulfill({ json: { items: [{ id: repo, name: 'team/repo', provider: 'gitea', team_ids: [], accessible: true }], complete: true } }))
  await page.route(`**${base}/repositories/${repo}/changes?**`, route => route.fulfill({ json: { items: [changeBody], complete: true } }))
  await page.route(`**${base}/merge-operations*`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route(revalidationURL, route => route.fulfill({ status: 503, json: { message: 'revalidation service unavailable' } }))
  await page.goto(`/org/${org}/changes?repository=${repo}&change=${change}`)
  await page.getByRole('button', { name: changeBody.title }).click()
  await expect(page.getByRole('alert')).toContainText('Automatic revalidation unavailable', { timeout: 15_000 })
  const retry = page.getByRole('button', { name: 'Retry' })
  await expect(retry).toBeVisible()
  await page.unroute(revalidationURL)
  await page.route(revalidationURL, route => route.fulfill({ json: { items: [] } }))
  await retry.click()
  await expect(page.getByRole('region', { name: 'Automatic bot revalidation' })).toHaveCount(0, { timeout: 10_000 })
})
