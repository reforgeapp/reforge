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
    await page.getByLabel('Search', { exact: true }).fill('literal-name')
    await page.getByLabel('Search', { exact: true }).press('Tab')
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


  test('refreshes forge picker and follows pagination through new connections after reload', async ({ page }) => {
    let includeNewest = false
    const pageRequests: string[] = []
    await page.route(`**/api/v1/orgs/${organisation}/connections**`, async route => {
      const url = new URL(route.request().url())
      const cursor = url.searchParams.get('cursor') ?? ''
      pageRequests.push(cursor)
      if (includeNewest && cursor === 'after-100') {
        await route.fulfill({ json: { items: [{ id: 'new-gitea', name: 'Newest Gitea', provider: 'gitea', kind: 'forge', endpoint: 'https://gitea.example.test', state: 'healthy', version: 1 }], complete: true } })
        return
      }
      const items = Array.from({ length: includeNewest ? 100 : 1 }, (_, index) => ({
        id: `old-${index}`,
        name: `Older forge ${index}`,
        provider: 'gitea',
        kind: 'forge',
        endpoint: 'https://gitea.example.test',
        state: 'healthy',
        version: 1,
      }))
      await route.fulfill({ json: { items, ...(includeNewest ? { next_cursor: 'after-100' } : {}), complete: !includeNewest } })
    })

    await signIn(page)
    await page.goto(`/org/${organisation}/repositories`)
    await page.getByRole('button', { name: 'Sync inventory' }).click()
    const dialog = page.getByRole('dialog', { name: 'Sync forge inventory' })
    await expect(dialog.getByRole('option', { name: /Older forge 0/ })).toBeAttached()
    await dialog.getByRole('button', { name: 'Close dialog' }).click()

    includeNewest = true
    await page.getByRole('button', { name: 'Sync inventory' }).click()
    await expect(dialog.getByRole('option', { name: 'Newest Gitea · gitea' })).toBeAttached()
    expect(pageRequests.slice(-2)).toEqual(['', 'after-100'])
    await dialog.getByLabel('Forge connection').selectOption('new-gitea')
    const startPreview = dialog.getByRole('button', { name: 'Start preview' })
    await expect(startPreview).toBeEnabled()
    includeNewest = false
    await page.evaluate(() => window.dispatchEvent(new Event('offline')))
    await page.evaluate(() => window.dispatchEvent(new Event('online')))
    await expect(dialog.getByRole('option', { name: /Older forge 0/ })).toBeAttached()
    await expect(dialog.getByRole('option', { name: 'Newest Gitea · gitea' })).toHaveCount(0)
    await expect(startPreview).toBeDisabled()
    await dialog.getByRole('button', { name: 'Close dialog' }).click()

    includeNewest = true
    await page.reload()
    await page.getByRole('button', { name: 'Sync inventory' }).click()
    await expect(dialog.getByRole('option', { name: 'Newest Gitea · gitea' })).toBeAttached()
    expect(pageRequests.slice(-2)).toEqual(['', 'after-100'])
  })

})
