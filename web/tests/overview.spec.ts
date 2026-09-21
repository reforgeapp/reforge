import { test, expect, type Page } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'

const org = '00000000-0000-4000-8000-000000000001'
const counts = { needs_decision: 4, running: 2, ready_for_review: 1, blocked: 3, verified_deployments: 5, accessible_repositories: 25, stale_repositories: 2, queued_jobs: 7 }
const attention = [{ id: 'finding-1', repository_id: 'repo-1', repository_name: 'payments', title: 'Default branch checks failed', severity: 'high', state: 'open', age_seconds: 7200 }]
const portfolio = [{ repository_id: 'repo-1', repository_name: 'payments', provider: 'github', accessible: true, open_findings: 4, open_changes: 1, blocked: 0, last_synced_at: '2026-09-21T00:00:00Z' }]
const capacity = { queued_jobs: 7, running_jobs: 2, active_pools: 1, active_runners: 3, reserved_micro_usd: 250000 }

async function mock(page: Page) {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }], memberships: [{ org_id: org, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { name: 'Reforge', version: 'test', edition: 'self-hosted', development: true, fixture_auth: true } }))
  await page.route(`**/api/v1/orgs/${org}/overview`, route => route.fulfill({ json: { counts, attention, portfolio, capacity } }))
  await page.goto(`/org/${org}/overview`)
}

test('renders persisted counts, opens the filtered queue and exposes on-demand help', async ({ page }) => {
  await mock(page)
  const metrics = page.getByRole('region', { name: 'Portfolio counts' })
  await expect(metrics.getByText('Needs decision')).toBeVisible()
  await expect(metrics.getByText('4')).toBeVisible()
  await expect(page.getByText('Stale/unsynced repositories: 2')).toBeVisible()
  const row = page.getByRole('link', { name: 'Default branch checks failed' })
  await expect(row).toHaveAttribute('href', /findings\?repository=repo-1&finding=finding-1/)
  await page.getByRole('button', { name: 'Help' }).click()
  await expect(page.getByRole('dialog', { name: 'Help · overview' })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Triage findings' })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog', { name: 'Help · overview' })).toBeHidden()
})

test('stacks counts at a narrow width without horizontal overflow', async ({ page }) => {
  await mock(page)
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(page.getByRole('region', { name: 'Portfolio counts' })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  const results = await new AxeBuilder({ page }).analyze()
  expect(results.violations).toEqual([])
})
