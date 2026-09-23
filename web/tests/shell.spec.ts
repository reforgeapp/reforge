import { test, expect, type Page } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'

const developmentOrganisation = '00000000-0000-4000-8000-000000000001'
const secondOrganisation = '00000000-0000-4000-8000-000000000002'

async function twoOrganisationFixture(page: Page) {
  const organisations = [{ id: developmentOrganisation, name: 'Development', version: 1, paused: false }, { id: secondOrganisation, name: 'Second', version: 1, paused: false }]
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations, memberships: organisations.map(item => ({ org_id: item.id, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true })), csrf_token: 'csrf-1' } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { name: 'Reforge', version: 'test', edition: 'self-hosted', development: true, fixture_auth: true } }))
  await page.route('**/api/v1/orgs/*/overview', route => route.fulfill({ json: { counts: { needs_decision: 0, running: 0, ready_for_review: 0, blocked: 0, verified_deployments: 0, accessible_repositories: 0, stale_repositories: 0, queued_jobs: 0 }, attention: [], portfolio: [], capacity: { queued_jobs: 0, running_jobs: 0, active_pools: 0, active_runners: 0, reserved_micro_usd: 0 } } }))
}

test.describe('T04 application shell', () => {
  async function signIn(page: Page) {
    await page.goto('/auth/login')
    await expect(page).toHaveURL(/\/org\/[^/]+\/overview/)
  }

  test('shows the server session and fixture state after sign in', async ({ page }) => {
    await page.goto('/')
    await expect(page.getByRole('heading', { name: 'Sign in', exact: true })).toBeVisible()
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
    const search = page.getByLabel('Search repositories')
    await expect(search).toHaveValue('payments')
    await expect(search).toHaveAttribute('type', 'search')
    await expect(search).toHaveAttribute('autoComplete', 'off')
    await expect(search).toHaveAttribute('autocorrect', 'off')
    await expect(search).toHaveAttribute('spellcheck', 'false')
    await search.press('End')
    await search.pressSequentially(' api')
    await search.press('Enter')
    await expect(page).toHaveURL(/q=payments(?:\+|%20)api/)
    await search.fill('payments')
    await page.getByRole('button', { name: 'Submit repository search' }).click()
    await expect(page).toHaveURL(/\/repositories\?q=payments$/)
    await page.getByRole('link', { name: 'Overview' }).focus()
    await expect(page.getByRole('link', { name: 'Overview' })).toBeFocused()
    await page.getByRole('link', { name: 'Overview' }).click()
    await expect(page).toHaveURL(/\/overview$/)
    await page.goBack()
    await expect(page).toHaveURL(/repositories\?q=payments$/)
    await expect(search).toHaveValue('payments')
    await search.fill('')
    await page.getByRole('button', { name: 'Submit repository search' }).click()
    await expect(page).toHaveURL(/\/repositories$/)
    await expect(page.getByRole('heading', { name: 'Repositories', exact: true })).toBeVisible()
  })

  test('does not render a protected deep link without a session', async ({ page }) => {
    await page.goto(`/org/${developmentOrganisation}/overview`)
    await expect(page.getByRole('heading', { name: 'Sign in', exact: true })).toBeVisible()
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
    await page.getByRole('link', { name: 'Repositories', exact: true }).first().click()
    await expect(page).toHaveURL(/\/repositories$/)
  })

  test('keeps the navigation off-canvas after resizing from desktop to mobile', async ({ page }) => {
    await signIn(page)
    await page.setViewportSize({ width: 390, height: 844 })
    const sidebar = page.locator('#primary-navigation')
    await expect.poll(() => sidebar.evaluate(element => element.getBoundingClientRect().right <= 0)).toBe(true)
    await expect(page.locator('.nav-backdrop')).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'Menu' })).toHaveAttribute('aria-expanded', 'false')
  })

  test('purges scoped data after a fixture session revocation', async ({ page }) => {
    let revoked = false
    let repositoryRequests = 0
    await page.route('**/api/v1/session', route => revoked
      ? route.fulfill({ status: 401, contentType: 'application/json', body: JSON.stringify({ code: 'unauthenticated', message: 'Authentication required', request_id: 'fixture-revoked', retryable: false }) })
      : route.continue())
    page.on('requestfinished', request => { if (/\/api\/v1\/orgs\/[^/]+\/repositories$/.test(new URL(request.url()).pathname)) repositoryRequests += 1 })
    await page.route('**/api/v1/orgs/**/repositories*', route => {
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
    await expect(page.getByRole('heading', { name: 'Sign in', exact: true })).toBeVisible()
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
    await expect(page.getByRole('menu', { name: 'Quick switch organisation' })).toBeVisible()
    await page.getByRole('menuitem', { name: 'More / manage organisations' }).click()
    await expect(page.getByRole('dialog', { name: 'Switch organisation' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Close dialog' })).toBeFocused()
    await page.keyboard.press('Escape')
    await expect(page.getByRole('dialog', { name: 'Switch organisation' })).toBeHidden()
  })

  test('keeps a long organisation list aligned and bounded', async ({ page }) => {
    await page.route('**/api/v1/session', async route => {
      const response = await route.fetch()
      const body = await response.json()
      body.organisations = [...body.organisations, ...Array.from({ length: 12 }, (_, index) => ({
        id: `00000000-0000-4000-8000-${String(index + 10).padStart(12, '0')}`,
        name: `Organisation_${'x'.repeat(80)}_${index}`,
        paused: index === 0,
        version: 1,
      }))]
      await route.fulfill({ response, json: body })
    })
    await signIn(page)
    for (const width of [1280, 390]) {
      await page.setViewportSize({ width, height: 844 })
      for (const theme of ['light', 'dark']) {
        await page.evaluate(value => window.localStorage.setItem('reforge-theme', value), theme)
        await page.reload()
        await page.getByRole('button', { name: 'Switch organisation' }).click()
        const menu = page.getByRole('menu', { name: 'Quick switch organisation' })
        const list = menu.locator('.org-menu-list')
        const footer = menu.getByRole('menuitem', { name: 'More / manage organisations' })
        const trigger = page.getByRole('button', { name: 'Switch organisation' })
        const searchBox = page.getByRole('search')
        const searchInput = page.getByLabel('Search repositories')
        await expect(menu).toBeVisible()
        await expect(menu.locator('.org-menu-item.selected')).toContainText('✓')
        await expect(footer).toBeVisible()
        const menuBox = await menu.boundingBox()
        const triggerBox = await trigger.boundingBox()
        const footerBox = await footer.boundingBox()
        expect(menuBox).toBeTruthy()
        expect(triggerBox).toBeTruthy()
        expect(footerBox).toBeTruthy()
        const searchBoxRect = await searchBox.boundingBox()
        const searchInputRect = await searchInput.boundingBox()
        expect(searchBoxRect).toBeTruthy()
        expect(searchInputRect).toBeTruthy()
        expect(searchInputRect!.width).toBeGreaterThanOrEqual(24)
        const horizontalOverlap = Math.max(0, Math.min(triggerBox!.x + triggerBox!.width, searchBoxRect!.x + searchBoxRect!.width) - Math.max(triggerBox!.x, searchBoxRect!.x))
        const verticalOverlap = Math.max(0, Math.min(triggerBox!.y + triggerBox!.height, searchBoxRect!.y + searchBoxRect!.height) - Math.max(triggerBox!.y, searchBoxRect!.y))
        expect(horizontalOverlap > 0 && verticalOverlap > 0).toBe(false)
        if (width > 720) expect(Math.abs(menuBox!.x - triggerBox!.x)).toBeLessThanOrEqual(1)
        else {
          expect(menuBox!.x).toBeGreaterThanOrEqual(0)
          expect(menuBox!.x + menuBox!.width).toBeLessThanOrEqual(width)
        }
        expect(footerBox!.y).toBeGreaterThanOrEqual(menuBox!.y)
        expect(footerBox!.y + footerBox!.height).toBeLessThanOrEqual(menuBox!.y + menuBox!.height)
        expect(menuBox!.width).toBeLessThanOrEqual(width)
        expect(await list.evaluate(element => element.scrollWidth)).toBeLessThanOrEqual(await list.evaluate(element => element.clientWidth))
        expect(await list.evaluate(element => element.scrollHeight)).toBeGreaterThan(await list.evaluate(element => element.clientHeight))
        await list.focus()
        await list.press('ArrowUp')
        await expect(footer).toBeFocused()
        await list.focus()
        await list.press('ArrowDown')
        await expect(menu.locator('.org-menu-item').first()).toBeFocused()
        await list.evaluate(element => { element.scrollTop = element.scrollHeight })
        const scrolledFooterBox = await footer.boundingBox()
        expect(scrolledFooterBox).toBeTruthy()
        expect(scrolledFooterBox!.y).toBeGreaterThanOrEqual(menuBox!.y)
        expect(scrolledFooterBox!.y + scrolledFooterBox!.height).toBeLessThanOrEqual(menuBox!.y + menuBox!.height)
        await page.getByRole('button', { name: 'Switch organisation' }).click()
      }
    }
  })

  test('dismisses quick switch from button child and restores focus after manage', async ({ page }) => {
    await twoOrganisationFixture(page)
    await page.goto(`/org/${developmentOrganisation}/overview`)
    const button = page.getByRole('button', { name: 'Switch organisation' })
    await button.click()
    await expect(page.getByRole('menu', { name: 'Quick switch organisation' })).toBeVisible()
    await button.locator('svg').click()
    await expect(page.getByRole('menu', { name: 'Quick switch organisation' })).toBeHidden()
    await button.click()
    const menuItems = page.getByRole('menuitem')
    await expect(menuItems).toHaveText(['Development✓', 'Second', 'More…'])
    await menuItems.first().press('ArrowDown')
    await expect(menuItems.nth(1)).toBeFocused()
    await menuItems.nth(1).press('Enter')
    await expect(page.getByRole('menu', { name: 'Quick switch organisation' })).toBeHidden()
    await expect(page).toHaveURL(`/org/${secondOrganisation}/overview`)
    await expect(button).toContainText('Second')
    await button.click()
    await menuItems.first().press('Tab')
    await expect(page.getByLabel('Search repositories')).toBeFocused()
    await button.click()
    await menuItems.first().press('Shift+Tab')
    await expect(page.getByRole('link', { name: 'Skip to content' })).toBeFocused()
    await page.setViewportSize({ width: 390, height: 844 })
    await button.click()
    await menuItems.first().press('Shift+Tab')
    await expect(page.getByRole('button', { name: 'Menu' })).toBeFocused()
    await button.click()
    await page.getByRole('menuitem', { name: 'More / manage organisations' }).press('Enter')
    await expect(page.getByRole('dialog', { name: 'Switch organisation' })).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(button).toBeFocused()
  })

  test('has no detectable accessibility violations on the shell', async ({ page }) => {
    await signIn(page)
    await page.goto(`/org/${developmentOrganisation}/overview`)
    await expect(page.getByRole('heading', { name: 'Overview', exact: true })).toBeVisible()
    const results = await new AxeBuilder({ page }).analyze()
    expect(results.violations).toEqual([])
  })
})
