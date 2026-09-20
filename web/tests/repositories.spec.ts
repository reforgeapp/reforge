import { test, expect, type Page } from '@playwright/test'

const organisation = '00000000-0000-4000-8000-000000000001'
test.use({ trace: 'off' })

async function signIn(page: Page) {
  await page.goto('/auth/login')
  await expect(page).toHaveURL(/\/org\/[^/]+\/overview/)
}

test.describe('repository inventory', () => {
  test('supports keyboard filters and narrow layout', async ({ page }) => {
    await signIn(page)
    await page.goto(`/org/${organisation}/repositories`)
    await expect(page.getByRole('heading', { name: 'Repositories', exact: true })).toBeVisible()
    await page.getByLabel('Search', { exact: true }).focus()
    await page.keyboard.type('literal-name')
    await expect(page.getByLabel('Search', { exact: true })).toHaveValue('literal-name')
    await page.getByRole('button', { name: 'Sync inventory' }).press('Enter')
    await expect(page.getByRole('dialog', { name: 'Sync forge inventory' })).toBeVisible()
    await page.getByRole('button', { name: 'Close dialog' }).press('Enter')
    await page.setViewportSize({ width: 390, height: 844 })
    await expect(page.getByLabel('Repository filters')).toBeVisible()
  })

  test('shows actionable backend error and retry control', async ({ page }) => {
    await signIn(page)
    await page.route('**/api/v1/orgs/*/repositories*', route => route.abort('failed'))
    await page.goto(`/org/${organisation}/repositories`)
    await expect(page.getByRole('heading', { name: 'Repositories could not be loaded' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Retry' })).toBeVisible()
  })

  test('restores deep-link filters and saved view on back navigation', async ({ page }) => {
    await signIn(page)
    await page.goto(`/org/${organisation}/repositories?q=deep-link&provider=gitea`)
    await expect(page.getByLabel('Search', { exact: true })).toHaveValue('deep-link')
    await expect(page.locator('[aria-label="Repository filters"]').getByLabel('Forge')).toHaveValue('gitea')
    await page.getByLabel('Saved view name').fill('narrow-view')
    await page.getByRole('button', { name: 'Save view' }).click()
    await expect(page.getByText('Saved view “narrow-view”.')).toBeVisible()
    await page.getByLabel('Saved views').selectOption('narrow-view')
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto(`/org/${organisation}/repositories?q=other`)
    await page.goBack()
    await expect(page.getByLabel('Search', { exact: true })).toHaveValue('deep-link')
  })

})
