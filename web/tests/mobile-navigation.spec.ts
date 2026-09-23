import { mkdir } from 'node:fs/promises'
import { resolve } from 'node:path'
import { test, expect, type Page } from '@playwright/test'

const org = '00000000-0000-4000-8000-0000000000de'
const artifactDir = resolve(process.cwd(), '../.local/t29t')

async function waitForDrawer(page: Page) {
  const sidebar = page.locator('#primary-navigation')
  await expect.poll(async () => {
    const box = await sidebar.boundingBox()
    return box ? [Math.round(box.x), Math.round(box.width)] : null
  }).toEqual([0, 245])
}

test.describe('mobile navigation drawer', () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await page.route('**/api/v1/**', route => route.fulfill({ json: { items: [], complete: true } }))
    await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }], memberships: [{ org_id: org, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
    await page.route('**/api/v1/meta', route => route.fulfill({ json: { name: 'Reforge', version: 'test', edition: 'self-hosted' } }))
    await page.route(`**/api/v1/orgs/${org}/overview`, route => route.fulfill({ json: { counts: { needs_decision: 0, running: 0, ready_for_review: 0, blocked: 0, verified_deployments: 0, accessible_repositories: 0, stale_repositories: 0, queued_jobs: 0 }, attention: [], portfolio: [], capacity: { queued_jobs: 0, running_jobs: 0, active_pools: 0, active_runners: 0, reserved_micro_usd: 0 } } }))
    await page.goto(`/org/${org}/overview`)
    await expect(page.getByRole('heading', { name: 'Overview', exact: true })).toBeVisible()
  })

  test('moves focus into drawer, traps Tab, isolates background, and restores focus on Escape', async ({ page }) => {
    const menu = page.getByRole('button', { name: 'Menu' })
    const sidebar = page.locator('#primary-navigation')
    const topbar = page.locator('.topbar')
    const main = page.locator('.main-column')
    await menu.click()
    await waitForDrawer(page)
    const links = sidebar.getByRole('link')
    await expect(links.first()).toBeFocused()
    expect(await sidebar.evaluate(element => Array.from(element.querySelectorAll('.nav-link')).every(link => link.scrollWidth <= link.clientWidth))).toBe(true)
    await expect(sidebar).toHaveJSProperty('inert', false)
    await expect(topbar).toHaveJSProperty('inert', true)
    await expect(main).toHaveJSProperty('inert', true)
    await links.last().focus()
    await page.keyboard.press('Tab')
    await expect(links.first()).toBeFocused()
    await page.keyboard.press('Shift+Tab')
    await expect(links.last()).toBeFocused()
    await page.keyboard.press('Escape')
    await expect(menu).toHaveAttribute('aria-expanded', 'false')
    await expect(menu).toBeFocused()
    await expect(topbar).toHaveJSProperty('inert', false)
    await expect(main).toHaveJSProperty('inert', false)
  })

  test('outside click dismisses drawer and returns focus to Menu', async ({ page }) => {
    const menu = page.getByRole('button', { name: 'Menu' })
    await menu.click()
    await waitForDrawer(page)
    const backdrop = page.getByRole('button', { name: 'Close navigation' })
    await expect(backdrop).toBeVisible()
    await backdrop.click({ position: { x: 340, y: 180 } })
    await expect(page.locator('#primary-navigation')).toHaveJSProperty('inert', true)
    await expect(menu).toHaveAttribute('aria-expanded', 'false')
    await expect(menu).toBeFocused()
  })

  test('route navigation closes drawer in light and dark themes', async ({ page }) => {
    await mkdir(artifactDir, { recursive: true })
    const menu = page.getByRole('button', { name: 'Menu' })
    for (const theme of ['light', 'dark']) {
      await page.evaluate(value => {
        document.documentElement.dataset.theme = value
        localStorage.setItem('reforge-theme', value)
      }, theme)
      await menu.click()
      await waitForDrawer(page)
      await expect(page.getByRole('button', { name: 'Close navigation' })).toBeVisible()
      expect(await page.locator('#primary-navigation').evaluate(element => Array.from(element.querySelectorAll('.nav-link')).every(link => link.scrollWidth <= link.clientWidth))).toBe(true)
      await page.screenshot({ path: resolve(artifactDir, `navigation-${theme}-390.png`), fullPage: true })
      await page.getByRole('link', { name: 'Repositories', exact: true }).click()
      await expect(page).toHaveURL(new RegExp(`/org/${org}/repositories$`))
      await expect(page.getByRole('heading', { name: 'Repositories', exact: true })).toBeVisible()
      await expect(page.locator('.topbar')).toHaveJSProperty('inert', false)
      await expect(menu).toHaveAttribute('aria-expanded', 'false')
      await expect(menu).toBeFocused()
      await page.goto(`/org/${org}/overview`)
    }
  })

  test('desktop navigation remains a persistent sidebar without a backdrop', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 900 })
    const sidebar = page.locator('#primary-navigation')
    await expect(sidebar).toBeVisible()
    await expect(sidebar).toHaveJSProperty('inert', false)
    await expect(page.getByRole('button', { name: 'Menu' })).toBeHidden()
    await expect(page.getByRole('button', { name: 'Close navigation' })).toHaveCount(0)
  })
})
