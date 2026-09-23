import { test, expect, type Page } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'
const team = '00000000-0000-4000-8000-0000000000bb'

async function setup(page: Page) {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }], memberships: [{ org_id: org, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { name: 'Reforge', version: 'test', edition: 'self-hosted', development: true, fixture_auth: true } }))
  await page.route(`**/api/v1/orgs/${org}/teams**`, route => route.fulfill({ json: { items: [{ id: team, name: 'Platform', repository_ids: ['repo-1'], version: 1 }], complete: true } }))
}

test('repository scope status follows loading, partial, failure, retry, and completion', async ({ page }) => {
  await setup(page)
  let releaseFirstPage: (() => void) | undefined
  const firstPageReady = new Promise<void>(resolve => { releaseFirstPage = resolve })
  let repositoryCalls = 0
  let failedPageCalls = 0
  await page.route(`**/api/v1/orgs/${org}/repositories**`, async route => {
    repositoryCalls += 1
    const cursor = new URL(route.request().url()).searchParams.get('cursor')
    if (!cursor) {
      await firstPageReady
      return route.fulfill({ json: { items: [{ id: 'repo-1', name: 'payments' }], complete: false, next_cursor: 'repo-next' } })
    }
    failedPageCalls += 1
    if (failedPageCalls <= 3) return route.fulfill({ status: 503, json: { message: 'Inventory unavailable' } })
    return route.fulfill({ json: { items: [{ id: 'repo-2', name: 'catalog' }], complete: true } })
  })

  await page.goto(`/org/${org}/organisation`)
  await expect(page.getByText('Loading repository scope')).toBeVisible()
  releaseFirstPage?.()
  await expect(page.getByText('Repository scope partial')).toBeVisible()
  await expect(page.getByLabel('payments')).toBeChecked()
  await page.getByRole('button', { name: 'Load more repositories' }).click()
  await expect(page.getByText('Repository scope unknown')).toBeVisible()
  await expect(page.getByText('More repositories unavailable.')).toBeVisible()
  await page.getByRole('button', { name: 'Retry' }).click()
  await expect(page.getByText('Repository scope complete')).toBeVisible()
  await expect(page.getByLabel('catalog')).toBeVisible()
})
