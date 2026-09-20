import { test, expect, type Page } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'

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
    await expect(page.getByRole('navigation', { name: /primary navigation/i })).toBeVisible()
    await expect(page.getByText('Development environment')).toBeVisible()
  })

  test('supports deep links, URL search and keyboard focus', async ({ page }) => {
    await signIn(page)
    await page.goto('/org/org00000000-0000-4000-8000-000000000001/repositories?q=payments')
    await expect(page.getByRole('heading', { name: 'Repositories' })).toBeVisible()
    await expect(page.getByLabel('Search repositories')).toHaveValue('payments')
    await page.getByLabel('Search repositories').press('End')
    await page.getByLabel('Search repositories').pressSequentially(' api')
    await expect(page).toHaveURL(/q=payments(?:\+|%20)api/)
    await page.getByRole('link', { name: 'Overview' }).focus()
    await expect(page.getByRole('link', { name: 'Overview' })).toBeFocused()
  })

  test('keeps organisation switching in an accessible dialog', async ({ page }) => {
    await signIn(page)
    await page.goto('/org/org00000000-0000-4000-8000-000000000001/overview')
    await page.getByRole('button', { name: 'Switch organisation' }).click()
    await expect(page.getByRole('dialog', { name: 'Switch organisation' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Close dialog' })).toBeFocused()
    await page.keyboard.press('Escape')
    await expect(page.getByRole('dialog', { name: 'Switch organisation' })).toBeHidden()
  })

  test('has no detectable accessibility violations on the shell', async ({ page }) => {
    await signIn(page)
    await page.goto('/org/org00000000-0000-4000-8000-000000000001/overview')
    await expect(page.getByRole('heading', { name: 'Portfolio overview' })).toBeVisible()
    const results = await new AxeBuilder({ page }).analyze()
    expect(results.violations).toEqual([])
  })
})
