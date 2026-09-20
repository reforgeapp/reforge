import { test, expect, type Page } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'

const developmentOrganisation = '00000000-0000-4000-8000-000000000001'

test.describe('T04 application shell', () => {
  async function signIn(page: Page) {
    await page.goto('/auth/login')
    await expect(page).toHaveURL(/\/org\/[^/]+\/overview/)
  }

  test('shows the server session and fixture state after sign in', async ({ page }) => {
    await page.goto('/')
    await expect(page.getByRole('heading', { name: /keep maintenance work moving/i })).toBeVisible()
    await expect(page.getByRole('button', { name: /sign in with your organisation/i })).toBeVisible()
    await page.getByRole('button', { name: /sign in with your organisation/i }).click()
    await expect(page).toHaveURL(/\/org\/[^/]+\/overview/)
    await expect(page.getByRole('complementary', { name: /primary navigation/i })).toBeVisible()
    await expect(page.getByText('Development environment')).toBeVisible()
  })

  test('supports deep links, URL search and keyboard focus', async ({ page }) => {
    await signIn(page)
    await page.goto(`/org/${developmentOrganisation}/repositories?q=payments`)
    await expect(page.getByRole('heading', { name: 'Repositories', exact: true })).toBeVisible()
    await expect(page.getByLabel('Search repositories')).toHaveValue('payments')
    await page.getByLabel('Search repositories').press('End')
    await page.getByLabel('Search repositories').pressSequentially(' api')
    await expect(page).toHaveURL(/q=payments(?:\+|%20)api/)
    await page.getByRole('link', { name: 'Overview' }).focus()
    await expect(page.getByRole('link', { name: 'Overview' })).toBeFocused()
    await page.getByRole('link', { name: 'Overview' }).click()
    await expect(page).toHaveURL(/\/overview$/)
    await page.goBack()
    await expect(page).toHaveURL(/repositories\?q=payments(?:\+|%20)api/)
    await expect(page.getByLabel('Search repositories')).toHaveValue('payments api')
  })

  test('does not render a protected deep link without a session', async ({ page }) => {
    await page.goto(`/org/${developmentOrganisation}/overview`)
    await expect(page.getByRole('heading', { name: /keep maintenance work moving/i })).toBeVisible()
    await expect(page.getByRole('complementary', { name: /primary navigation/i })).toHaveCount(0)
  })

  test('denies an organisation outside the session scope', async ({ page }) => {
    await signIn(page)
    await page.goto('/org/00000000-0000-4000-8000-000000000099/overview')
    await expect(page.getByRole('heading', { name: 'Organisation access denied' })).toBeVisible()
    await expect(page.getByRole('complementary', { name: /primary navigation/i })).toHaveCount(0)
  })

  test('opens the navigation at a narrow viewport', async ({ page }) => {
    await signIn(page)
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto(`/org/${developmentOrganisation}/overview`)
    const menu = page.getByRole('button', { name: 'Menu' })
    await expect(menu).toBeVisible()
    await expect(menu).toHaveAttribute('aria-expanded', 'false')
    await menu.click()
    await expect(menu).toHaveAttribute('aria-expanded', 'true')
    await page.getByRole('link', { name: 'Repositories' }).click()
    await expect(page).toHaveURL(/\/repositories$/)
  })

  test('purges scoped data after a fixture session revocation', async ({ page }) => {
    let revoked = false
    let repositoryRequests = 0
    await page.route('**/api/v1/session', route => revoked
      ? route.fulfill({ status: 401, contentType: 'application/json', body: JSON.stringify({ code: 'unauthenticated', message: 'Authentication required', request_id: 'fixture-revoked', retryable: false }) })
      : route.continue())
    await page.route('**/api/v1/orgs/**/repositories*', route => {
      repositoryRequests += 1
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [{ id: 'fixture-sensitive-repository', org_id: developmentOrganisation, connection_id: 'fixture', native_id: 'fixture', name: 'sensitive-fixture-repository', url: 'https://example.invalid/fixture', provider: 'github', default_branch: 'main', archived: false, paused: false, accessible: true, team_ids: [], last_synced_at: null, version: 1 }], complete: true }) })
    })
    await page.clock.install()
    await signIn(page)
    await page.goto(`/org/${developmentOrganisation}/repositories`)
    await expect(page.getByRole('heading', { name: 'Repositories', exact: true })).toBeVisible()
    await expect(page.getByText('sensitive-fixture-repository')).toBeVisible()
    expect(repositoryRequests).toBe(1)
    revoked = true
    await page.clock.fastForward(31_000)
    await expect(page.getByRole('heading', { name: /keep maintenance work moving/i })).toBeVisible()
    revoked = false
    await page.clock.fastForward(31_000)
    await expect(page.getByRole('heading', { name: 'Repositories', exact: true })).toBeVisible()
    await expect(page.getByText('sensitive-fixture-repository')).toBeVisible()
    expect(repositoryRequests).toBe(2)
  })

  test('keeps organisation switching in an accessible dialog', async ({ page }) => {
    await signIn(page)
    await page.goto(`/org/${developmentOrganisation}/overview`)
    await page.getByRole('button', { name: 'Switch organisation' }).click()
    await expect(page.getByRole('dialog', { name: 'Switch organisation' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Close dialog' })).toBeFocused()
    await page.keyboard.press('Escape')
    await expect(page.getByRole('dialog', { name: 'Switch organisation' })).toBeHidden()
  })

  test('has no detectable accessibility violations on the shell', async ({ page }) => {
    await signIn(page)
    await page.goto(`/org/${developmentOrganisation}/overview`)
    await expect(page.getByRole('heading', { name: 'Portfolio overview' })).toBeVisible()
    const results = await new AxeBuilder({ page }).analyze()
    expect(results.violations).toEqual([])
  })
})
