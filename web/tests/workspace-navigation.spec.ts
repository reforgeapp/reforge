import { test, expect } from '@playwright/test'

const org = '00000000-0000-4000-8000-0000000000de'

test.describe('workspace navigation', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/auth/login')
    await page.waitForURL(/\/overview/)
  })

  test('search submits to repositories and keeps URL state', async ({ page }) => {
    await page.goto(`/org/${org}/repositories?provider=github&status=monitored&finding=selected`)
    const search = page.getByRole('search').getByRole('textbox', { name: 'Search repositories' })
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
    await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }], memberships: [{ org_id: org, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
    await page.route('**/api/v1/meta', route => route.fulfill({ json: { name: 'Reforge', version: 'test', edition: 'self-hosted' } }))
    await page.route(`**/api/v1/orgs/${org}/repositories**`, route => route.fulfill({ json: { items: [], complete: true } }))
    await page.route(`**/api/v1/orgs/${org}/tasks**`, route => route.fulfill({ json: { items: [{ id: 'run-1', repository_id: 'repo-1', recipe: 'dependency-update', recipe_version: 1, model_route: 'fixture', max_attempts: 1, state: 'completed', created_at: '2026-09-22T00:00:00Z', reason: '' }], complete: true } }))
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
