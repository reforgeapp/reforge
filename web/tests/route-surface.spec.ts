import { test, expect } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'

const org = '00000000-0000-4000-8000-000000000001'
const routes = ['overview', 'repositories', 'findings', 'runs', 'changes', 'deployments', 'campaigns', 'policies', 'connections', 'runners', 'usage', 'audit', 'organisation'] as const

test.describe('T29 route surface gate', () => {
  test('every route keeps one title, one primary toolbar and no horizontal overflow', async ({ page }) => {
    await page.goto('/auth/login')
    await page.waitForURL(/\/overview/)
    for (const route of routes) {
      await page.setViewportSize({ width: 390, height: 844 })
      await page.goto(`/org/${org}/${route}`)
      await expect(page.locator('h1')).toHaveCount(1)
      expect(await page.locator('.repository-toolbar').count(), `${route} primary toolbars`).toBeLessThanOrEqual(1)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1), `${route} horizontal overflow`).toBe(true)
    }
  })

  test('administration routes meet accessibility checks at a narrow width', async ({ page }) => {
    await page.goto('/auth/login')
    await page.waitForURL(/\/overview/)
    await page.setViewportSize({ width: 390, height: 844 })
    for (const route of ['findings', 'policies', 'usage', 'audit', 'runners'] as const) {
      await page.goto(`/org/${org}/${route}`)
      const results = await new AxeBuilder({ page }).analyze()
      expect(results.violations, `${route} accessibility violations`).toEqual([])
    }
  })
})
