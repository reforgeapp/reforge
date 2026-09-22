import { test, expect, type Page } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'

const routes = ['overview', 'repositories', 'findings', 'runs', 'changes', 'deployments', 'campaigns', 'policies', 'connections', 'runners', 'usage', 'audit', 'organisation'] as const

function channel(value: number) {
  const scaled = value / 255
  return scaled <= 0.03928 ? scaled / 12.92 : ((scaled + 0.055) / 1.055) ** 2.4
}

function parseColor(value: string): [number, number, number] {
  const match = value.match(/rgba?\(([^)]+)\)/)
  if (!match) throw new Error(`Unsupported color: ${value}`)
  const [r, g, b] = match[1].split(',').map(part => Number(part.trim()))
  return [r, g, b]
}

function relativeLuminance([r, g, b]: [number, number, number]) {
  return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b)
}

async function signIn(page: Page) {
  await page.goto('/auth/login')
  await expect(page).toHaveURL(/\/org\/[^/]+\/overview/)
}

function themeToggle(page: Page) {
  return page.getByRole('button', { name: /^Switch to (light|dark) theme$/ })
}

async function backgroundLuminance(page: Page, selector: string) {
  const background = await page.$eval(selector, element => getComputedStyle(element).backgroundColor)
  return relativeLuminance(parseColor(background))
}

async function contrastRatio(page: Page, selector: string) {
  const { color, background } = await page.$eval(selector, element => ({ color: getComputedStyle(element).color, background: getComputedStyle(element).backgroundColor }))
  const [light, dark] = [relativeLuminance(parseColor(color)), relativeLuminance(parseColor(background))].sort((a, b) => b - a)
  return (light + 0.05) / (dark + 0.05)
}

test.describe('T29m dark theme', () => {
  test('follows the system preference before an explicit choice', async ({ page }) => {
    await page.emulateMedia({ colorScheme: 'dark' })
    await signIn(page)
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
    await expect(themeToggle(page)).toHaveAccessibleName('Switch to light theme')
    expect(await page.evaluate(() => localStorage.getItem('reforge-theme'))).toBeNull()
  })

  test('persists an explicit choice across reload', async ({ page }) => {
    await page.emulateMedia({ colorScheme: 'light' })
    await signIn(page)
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
    await themeToggle(page).click()
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
    expect(await page.evaluate(() => localStorage.getItem('reforge-theme'))).toBe('dark')
    await page.reload()
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
    await expect(themeToggle(page)).toHaveAccessibleName('Switch to light theme')
  })

  test('toggles from the keyboard and stays reachable at 390px', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await signIn(page)
    const toggle = themeToggle(page)
    await expect(toggle).toBeVisible()
    await toggle.focus()
    await page.keyboard.press('Enter')
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
    await page.keyboard.press('Space')
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
  })

  test('covers tables, forms and dialogs with readable contrast in dark', async ({ page }) => {
    await signIn(page)
    await themeToggle(page).click()
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
    const orgID = new URL(page.url()).pathname.split('/')[2]
    await page.goto(`/org/${orgID}/repositories`)
    await expect(page.locator('thead th').first()).toBeVisible()
    expect(await backgroundLuminance(page, 'body')).toBeLessThan(0.3)
    expect(await backgroundLuminance(page, 'thead th')).toBeLessThan(0.3)
    expect(await contrastRatio(page, 'thead th')).toBeGreaterThanOrEqual(4.5)
    expect(await contrastRatio(page, 'label')).toBeGreaterThanOrEqual(4.5)
    await page.getByRole('button', { name: 'Switch organisation' }).click()
    await page.getByRole('menuitem', { name: 'More / manage organisations' }).click()
    await expect(page.getByRole('dialog', { name: 'Switch organisation' })).toBeVisible()
    expect(await backgroundLuminance(page, '.dialog')).toBeLessThan(0.3)
    expect(await contrastRatio(page, '.dialog')).toBeGreaterThanOrEqual(4.5)
  })

  test('has no detectable accessibility violations in dark', async ({ page }) => {
    await signIn(page)
    await themeToggle(page).click()
    const orgID = new URL(page.url()).pathname.split('/')[2]
    await page.goto(`/org/${orgID}/repositories`)
    await expect(page.locator('thead th').first()).toBeVisible()
    const results = await new AxeBuilder({ page }).analyze()
    expect(results.violations).toEqual([])
  })

  test('every route meets accessibility checks in dark at a narrow width', async ({ page }) => {
    await signIn(page)
    await themeToggle(page).click()
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
    const orgID = new URL(page.url()).pathname.split('/')[2]
    await page.setViewportSize({ width: 390, height: 844 })
    for (const route of routes) {
      await page.goto(`/org/${orgID}/${route}`)
      const results = await new AxeBuilder({ page }).analyze()
      expect(results.violations, `${route} dark accessibility violations`).toEqual([])
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1), `${route} dark horizontal overflow`).toBe(true)
    }
  })

  test('survives unavailable localStorage', async ({ page }) => {
    await page.addInitScript(() => {
      Object.defineProperty(window, 'localStorage', { configurable: true, get() { throw new Error('localStorage disabled') } })
    })
    await signIn(page)
    await expect(page.locator('html')).toHaveAttribute('data-theme', /light|dark/)
    await themeToggle(page).click()
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  })
})
