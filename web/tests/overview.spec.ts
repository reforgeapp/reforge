import { test, expect, type Page } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'

const org = '00000000-0000-4000-8000-000000000001'
const counts = { needs_decision: 4, running: 2, ready_for_review: 1, blocked: 3, verified_deployments: 5, accessible_repositories: 25, stale_repositories: 2, queued_jobs: 7 }
const attention = [{ id: 'finding-1', repository_id: 'repo-1', repository_name: 'payments', title: 'Default branch checks failed', severity: 'high', state: 'open', age_seconds: 7200 }]
const portfolio = [{ repository_id: 'repo-1', repository_name: 'payments', provider: 'github', accessible: true, open_findings: 4, open_changes: 1, blocked: 0, last_synced_at: '2026-09-21T00:00:00Z' }]
const capacity = { queued_jobs: 7, running_jobs: 2, active_pools: 1, active_runners: 3, reserved_micro_usd: 250000 }

async function mock(page: Page, staleRepositories = counts.stale_repositories) {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }], memberships: [{ org_id: org, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { name: 'Reforge', version: 'test', edition: 'self-hosted', development: true, fixture_auth: true } }))
  await page.route(`**/api/v1/orgs/${org}/overview`, route => route.fulfill({ json: { counts: { ...counts, stale_repositories: staleRepositories }, attention, portfolio, capacity } }))
  await page.goto(`/org/${org}/overview`)
}

test('renders persisted counts, opens the filtered queue and exposes on-demand help', async ({ page }) => {
  await mock(page)
  const metrics = page.getByRole('region', { name: 'Portfolio counts' })
  await expect(metrics.getByText('Needs you')).toBeVisible()
  await expect(metrics.getByText('4', { exact: true })).toBeVisible()
  const syncStatus = page.getByRole('status')
  const syncLink = syncStatus.getByRole('link', { name: '2 repositories need sync Review stale repositories' })
  await expect(syncLink).toBeVisible()
  await expect(syncLink).toHaveAttribute('href', `/org/${org}/repositories?status=stale`)
  await syncLink.focus()
  await expect(syncLink).toBeFocused()
  await expect(page.getByRole('link', { name: 'Default branch checks failed' })).toBeHidden()
  await page.getByRole('button', { name: 'More details', exact: true }).click()
  const row = page.getByRole('link', { name: 'Default branch checks failed' })
  await expect(row).toHaveAttribute('href', /findings\?repository=repo-1&finding=finding-1/)
  await page.getByRole('button', { name: 'Help' }).click()
  await expect(page.getByRole('dialog', { name: 'Help · overview' })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Triage findings' })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog', { name: 'Help · overview' })).toBeHidden()
})

test('uses singular wording for one stale repository', async ({ page }) => {
  await mock(page, 1)
  const syncLink = page.getByRole('link', { name: '1 repository needs sync Review stale repositories' })
  await expect(syncLink).toBeVisible()
  await expect(page.getByText('1 repositories need sync')).toHaveCount(0)
})

test('uses readable sync color in both themes', async ({ page }) => {
  await mock(page)
  const link = page.getByRole('link', { name: '2 repositories need sync Review stale repositories' })
  const lightBackground = await link.evaluate(element => getComputedStyle(element).backgroundColor)
  await page.getByRole('button', { name: 'Switch to dark theme' }).click()
  const darkBackground = await link.evaluate(element => getComputedStyle(element).backgroundColor)
  expect(darkBackground).not.toBe(lightBackground)
})

test('omits sync attention when no repository is stale', async ({ page }) => {
  await mock(page, 0)
  await expect(page.locator('.overview-sync-attention')).toHaveCount(0)
})

test('stacks counts at a narrow width without horizontal overflow', async ({ page }) => {
  await mock(page)
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(page.getByRole('region', { name: 'Portfolio counts' })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  const syncLink = page.getByRole('link', { name: '2 repositories need sync Review stale repositories' })
  const box = await syncLink.boundingBox()
  expect(box).not.toBeNull()
  expect(box!.x + box!.width).toBeLessThanOrEqual(390)
  const results = await new AxeBuilder({ page }).analyze()
  expect(results.violations).toEqual([])
})


test('waiting work is explained without claiming it is running or asking the owner to fix it', async ({ page }) => {
  await page.route(`**/api/v1/orgs/${org}/overview`, route => route.fulfill({ json: { counts: { ...counts, needs_decision: 0, verified_deployments: 0 }, attention, portfolio, capacity: { ...capacity, running_jobs: 0 }, severity: [{ severity: 'high', count: 3 }], trend: [{ day: '2026-10-22', findings: 4, runs: 200, merges: 6, deployments: 0 }], maintenance: [{ repository_id: 'repo-1', repository_name: 'payments', state: 'waiting_limit', count: 1 }, { repository_id: 'repo-1', repository_name: 'payments', state: 'checking', count: 2 }] } }))
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture' }, organisations: [{ id: org, name: 'Fixture' }], memberships: [{ org_id: org, role: 'owner', all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { development: true, fixture_auth: true } }))
  await page.goto(`/org/${org}/overview`)
  await expect(page.getByRole('heading', { name: 'Maintenance is waiting for allowance' })).toBeVisible()
  await expect(page.getByText(/Nothing needs your decision.*resumes automatically/)).toBeVisible()
  await expect(page.getByText('3 higher-priority issues remain open. Progress and waits are shown below.')).toBeVisible()
  const countsRegion = page.getByRole('region', { name: 'Portfolio counts' })
  await expect(countsRegion.getByRole('link', { name: /In progress 0/ })).toBeVisible()
  await expect(countsRegion.getByRole('link', { name: /Fixes merged 6/ })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Your repositories' }).getByText('1 waiting for budget or provider allowance')).toBeVisible()
  await expect(page.getByText('Healthy environments', { exact: true })).toHaveCount(0)
  await expect(page.getByRole('heading', { name: 'Open findings' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Activity · 14 days' })).toBeVisible()
  const details = page.getByRole('button', { name: 'More details', exact: true })
  await expect(details).toHaveAttribute('aria-expanded', 'false')
  await expect(page.getByRole('link', { name: 'Default branch checks failed' })).toBeHidden()
  await details.click()
  await expect(page.getByRole('button', { name: 'Hide details' })).toHaveAttribute('aria-expanded', 'true')
  await expect(page.getByRole('link', { name: 'Default branch checks failed' })).toBeVisible()
  await page.getByRole('button', { name: 'Hide details' }).click()
  await expect(page.getByRole('link', { name: 'Default branch checks failed' })).toBeHidden()
  await expect(page.getByRole('heading', { name: 'Open findings' })).toBeVisible()
})
