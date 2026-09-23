import { test, expect, type Page } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'

async function fixture(page: Page) {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }], memberships: [{ org_id: org, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { name: 'Reforge', version: 'test', edition: 'self-hosted', development: true, fixture_auth: true } }))
  await page.route(`**/api/v1/orgs/${org}/repositories**`, route => route.fulfill({ json: { items: [{ id: 'repo-1', name: 'payments', provider: 'gitea', team_ids: [], accessible: true }], complete: true } }))
}

test('findings cached filter navigation keeps rows after returning', async ({ page }) => {
  await fixture(page)
  await page.route(`**/api/v1/orgs/${org}/findings**`, route => {
    const url = new URL(route.request().url())
    const state = url.searchParams.get('state') ?? 'open'
    return route.fulfill({ json: { items: [{ id: `finding-${state}`, title: `${state} finding`, category: 'dependency', severity: 'high', source: 'advisory', state, assigned_to: '', last_seen: new Date().toISOString(), repository_id: 'repo-1' }], complete: true } })
  })
  await page.goto(`/org/${org}/findings?finding_state=open`)
  await expect(page.getByText('open finding')).toBeVisible()
  await page.getByLabel('State').selectOption('dismissed')
  await expect(page.getByText('dismissed finding')).toBeVisible()
  await page.goBack()
  await expect(page.getByText('open finding')).toBeVisible()
})

test('runs cached state navigation keeps rows after returning', async ({ page }) => {
  await fixture(page)
  await page.route(`**/api/v1/orgs/${org}/tasks**`, route => {
    const state = new URL(route.request().url()).searchParams.get('state') ?? 'completed'
    return route.fulfill({ json: { items: [{ id: `task-${state}`, repository_id: 'repo-1', recipe: 'go', recipe_version: '1', model_route: '', max_attempts: 1, state, reason: '', created_at: new Date().toISOString() }], complete: true } })
  })
  await page.goto(`/org/${org}/runs?state=completed`)
  await expect(page.getByRole('button', { name: 'task-com' })).toBeVisible()
  await page.getByLabel('State').selectOption('failed')
  await expect(page.getByRole('button', { name: 'task-fai' })).toBeVisible()
  await page.goBack()
  await expect(page.getByRole('button', { name: 'task-com' })).toBeVisible()
})

test('findings pagination deduplicates overlap and preserves cached pages', async ({ page }) => {
  await fixture(page)
  await page.route(`**/api/v1/orgs/${org}/repositories**`, route => route.fulfill({ json: { items: [], complete: true } }))
  let requests = 0
  await page.route(`**/api/v1/orgs/${org}/findings**`, route => {
    requests += 1
    const url = new URL(route.request().url())
    if (url.searchParams.get('state') === 'resolved') return route.fulfill({ json: { items: [], complete: true } })
    if (url.searchParams.get('cursor')) return route.fulfill({ json: { items: [{ id: 'finding-2', title: 'Second finding', category: 'dependency', severity: 'medium', source: 'advisory', state: 'open', assigned_to: '', last_seen: new Date().toISOString(), repository_id: 'repo-1' }, { id: 'finding-3', title: 'Third finding', category: 'dependency', severity: 'low', source: 'advisory', state: 'open', assigned_to: '', last_seen: new Date().toISOString(), repository_id: 'repo-1' }], complete: true } })
    return route.fulfill({ json: { items: [{ id: 'finding-1', title: 'First finding', category: 'dependency', severity: 'high', source: 'advisory', state: 'open', assigned_to: '', last_seen: new Date().toISOString(), repository_id: 'repo-1' }, { id: 'finding-2', title: 'Second finding', category: 'dependency', severity: 'medium', source: 'advisory', state: 'open', assigned_to: '', last_seen: new Date().toISOString(), repository_id: 'repo-1' }], next_cursor: 'page-2', complete: false } })
  })
  await page.goto(`/org/${org}/findings?finding_state=open`)
  await expect(page.getByText('First finding')).toBeVisible()
  await page.getByRole('button', { name: 'Load more findings' }).click()
  await expect(page.getByText('Third finding')).toBeVisible()
  await expect(page.getByText('Second finding')).toHaveCount(1)
  await page.getByLabel('State').selectOption('resolved')
  await expect(page.getByText('No findings match these filters.')).toBeVisible()
  await page.goBack()
  await expect(page.getByText('Third finding')).toBeVisible()
  expect(requests).toBeGreaterThanOrEqual(3)
})

test('repositories saved view reload keeps cached rows', async ({ page }) => {
  await fixture(page)
  await page.route(`**/api/v1/orgs/${org}/repositories**`, route => route.fulfill({ json: { items: [{ id: 'repo-1', name: 'payments', provider: 'gitea', team_ids: [], accessible: true, archived: false, paused: false, default_branch: 'main', sync_state: 'active' }], complete: true } }))
  await page.route(`**/api/v1/orgs/${org}/teams**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route(`**/api/v1/orgs/${org}/connections**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.goto(`/org/${org}/repositories`)
  await expect(page.getByText('payments')).toBeVisible()
  await page.getByLabel('Saved view name').fill('all-repositories')
  await page.getByRole('button', { name: 'Save view' }).click()
  await page.getByLabel('Saved views').selectOption('all-repositories')
  await expect(page.getByText('payments')).toBeVisible()
})

test('repository search keeps control mounted during sequential typing', async ({ page }) => {
  await fixture(page)
  await page.route(`**/api/v1/orgs/${org}/teams**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route(`**/api/v1/orgs/${org}/connections**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route(`**/api/v1/orgs/${org}/repositories**`, async route => { await new Promise(resolve => setTimeout(resolve, 25)); await route.fulfill({ json: { items: [], complete: true } }) })
  await page.goto(`/org/${org}/repositories`)
  const search = page.getByLabel('Search', { exact: true })
  await search.focus()
  await page.keyboard.type('ab')
  await expect(search).toHaveValue('ab')
  await expect(search).toBeFocused()
})

test('finding search keeps control mounted during sequential typing', async ({ page }) => {
  await fixture(page)
  await page.route(`**/api/v1/orgs/${org}/findings**`, async route => { await new Promise(resolve => setTimeout(resolve, 25)); await route.fulfill({ json: { items: [], complete: true } }) })
  await page.goto(`/org/${org}/findings`)
  const search = page.getByLabel('Search', { exact: true })
  await search.focus()
  await page.keyboard.type('ab')
  await expect(search).toHaveValue('ab')
  await expect(search).toBeFocused()
})
