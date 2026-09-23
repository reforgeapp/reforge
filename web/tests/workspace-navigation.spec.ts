import { test, expect } from '@playwright/test'

const org = '00000000-0000-4000-8000-0000000000de'

test.describe('workspace navigation', () => {
  test.beforeEach(async ({ page }) => {
    await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }], memberships: [{ org_id: org, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
    await page.route('**/api/v1/meta', route => route.fulfill({ json: { name: 'Reforge', version: 'test', edition: 'self-hosted' } }))
    await page.route(`**/api/v1/orgs/${org}/repositories**`, route => route.fulfill({ json: { items: [], complete: true } }))
    await page.route(`**/api/v1/orgs/${org}/teams**`, route => route.fulfill({ json: { items: [], complete: true } }))
    await page.route(`**/api/v1/orgs/${org}/overview`, route => route.fulfill({ json: { counts: { needs_decision: 0, running: 0, ready_for_review: 0, blocked: 0, verified_deployments: 0, accessible_repositories: 0, stale_repositories: 0, queued_jobs: 0 }, attention: [], portfolio: [], capacity: { queued_jobs: 0, running_jobs: 0, active_pools: 0, active_runners: 0, reserved_micro_usd: 0 } } }))
  })

  test('search submits to repositories and keeps URL state', async ({ page }) => {
    await page.goto(`/org/${org}/repositories?provider=github&status=monitored&finding=selected`)
    const search = page.getByRole('searchbox', { name: 'Search repositories' })
    await search.fill('payments')
    await search.press('Enter')
    await expect(page).toHaveURL(/\/repositories\?/)
    expect(new URL(page.url()).searchParams.get('q')).toBe('payments')
    expect(new URL(page.url()).searchParams.get('provider')).toBe('github')
    expect(new URL(page.url()).searchParams.get('status')).toBe('monitored')
    expect(new URL(page.url()).searchParams.get('finding')).toBeNull()
  })

  test('detail view traps no focus when closed and returns focus to list', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await page.route(`**/api/v1/orgs/${org}/repair-runs/run-1`, route => route.fulfill({ status: 404, json: { code: 'not_found', message: 'Repair run not found', request_id: '', retryable: false } }))
    await page.route(`**/api/v1/orgs/${org}/events**`, route => route.fulfill({ status: 200, headers: { 'content-type': 'text/event-stream' }, body: ': keepalive\n\n' }))
    const task = { id: 'run-1', org_id: org, repository_id: 'repo-1', operation_id: 'operation-1', recipe: 'dependency-update', recipe_version: '1', target_branch: 'main', model_route: 'fixture', policy_hash: 'policy', starting_policy_hash: 'policy', state: 'completed', reason: '', version: 1, cancel_version: 1, max_attempts: 1, created_at: '2026-09-22T00:00:00Z' }
    await page.route(`**/api/v1/orgs/${org}/tasks?**`, route => route.fulfill({ json: { items: [task], complete: true } }))
    await page.route(`**/api/v1/orgs/${org}/tasks/run-1`, route => route.fulfill({ json: task }))
    await page.goto(`/org/${org}/runs`)
    const row = page.locator('.split-list .link-button').first()
    await expect(row).toBeVisible()
    await row.click()
    const detail = page.locator('.split-detail')
    await expect(detail).toBeVisible()
    await expect(detail).toBeFocused()
    await page.getByRole('button', { name: 'Back to list' }).click()
    await expect(detail).toBeHidden()
    await expect(row).toBeFocused()
  })

  test('closed mobile navigation cannot receive focus and Escape restores menu focus', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto(`/org/${org}/overview`)
    const sidebar = page.locator('#primary-navigation')
    const menu = page.getByRole('button', { name: 'Menu' })
    await expect(sidebar).toHaveJSProperty('inert', true)
    await menu.click()
    await expect(sidebar).toHaveJSProperty('inert', false)
    await page.keyboard.press('Escape')
    await expect(sidebar).toHaveJSProperty('inert', true)
    await expect(menu).toBeFocused()
  })
})
