import { test, expect, type Page } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'
const otherOrg = '00000000-0000-4000-8000-000000000002'

const connection = (id: string, name: string, kind = 'forge', state = 'healthy', orgID = org) => ({ id, org_id: orgID, kind, provider: 'gitea', name, endpoint: `https://${id}.example.test`, state, reason: '', version: 1, credential_version: 1, settings: {}, capabilities: {}, verified_at: null })
const pool = (id: string, name: string, state = 'active') => ({ id, org_id: org, name, state, version: 1, runner_count: 0, busy_slots: 0, repository_ids: [] })

async function session(page: Page) {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }, { id: otherOrg, name: 'Other fixture', version: 1, paused: false }], memberships: [{ org_id: org, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true }, { org_id: otherOrg, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { name: 'Reforge', version: 'test', edition: 'self-hosted', development: true, fixture_auth: true } }))
}

test('connection refresh replaces cached pages and handles pagination', async ({ page }) => {
  await session(page)
  let calls = 0
  await page.route(`**/api/v1/orgs/${org}/connections*`, route => {
    calls += 1
    const cursor = new URL(route.request().url()).searchParams.get('cursor')
    if (cursor) return route.fulfill({ json: { items: [connection('connection-three', 'Three')], complete: true } })
    return route.fulfill({ json: calls === 1 ? { items: [connection('connection-one', 'One')], next_cursor: 'connection-one', complete: false } : { items: [connection('connection-two', 'Two')], complete: true } })
  })
  await page.goto(`/org/${org}/connections`)
  await expect(page.getByRole('row', { name: /One/ })).toBeVisible()
  await page.getByRole('button', { name: 'Load more connections' }).click()
  await expect(page.getByRole('row', { name: /Three/ })).toBeVisible()
  await page.getByRole('button', { name: 'Refresh' }).click()
  await expect(page.getByRole('row', { name: /Two/ })).toBeVisible()
  await expect(page.getByRole('row', { name: /One/ })).toHaveCount(0)
  await expect(page.getByRole('row', { name: /Three/ })).toHaveCount(0)
})

test('connection retry recovers after an initial request failure', async ({ page }) => {
  await session(page)
  let fail = true
  await page.route(`**/api/v1/orgs/${org}/connections*`, async route => {
    if (fail) return route.fulfill({ status: 503, json: { code: 'unavailable', message: 'retry', request_id: '', retryable: true } })
    return route.fulfill({ json: { items: [connection('connection-current', 'Current')], complete: true } })
  })
  await page.goto(`/org/${org}/connections`)
  await expect(page.getByRole('heading', { name: 'Connections could not be loaded' })).toBeVisible()
  fail = false
  await page.getByRole('button', { name: 'Retry' }).click()
  await expect(page.getByRole('row', { name: /Current/ })).toBeVisible()
})

test('connection refresh keeps rows visible when response is unchanged and cached tabs survive navigation', async ({ page }) => {
  await session(page)
  let calls = 0
  let releaseRefresh: (() => void) | undefined
  let refreshStarted: (() => void) | undefined
  const refreshSeen = new Promise<void>(resolve => { refreshStarted = resolve })
  await page.route(`**/api/v1/orgs/${org}/connections*`, async route => {
    calls += 1
    if (calls === 2) {
      refreshStarted?.()
      await new Promise<void>(resolve => { releaseRefresh = resolve })
    }
    return route.fulfill({ json: { items: [connection('connection-stable', 'Stable forge')], complete: true } })
  })
  await page.goto(`/org/${org}/connections`)
  await expect(page.getByRole('row', { name: /Stable forge/ })).toBeVisible()
  await page.getByRole('button', { name: 'Refresh' }).click()
  await refreshSeen
  await expect(page.getByRole('row', { name: /Stable forge/ })).toBeVisible()
  const refetchResponse = page.waitForResponse(response => response.url().includes(`/api/v1/orgs/${org}/connections`) && response.request().method() === 'GET')
  releaseRefresh?.()
  await (await refetchResponse).finished()
  await expect.poll(() => calls).toBe(2)
  await page.getByRole('button', { name: 'Forges' }).click()
  await expect(page.getByRole('row', { name: /Stable forge/ })).toBeVisible()
  await page.getByRole('button', { name: 'Models & agents' }).click()
  await expect(page.getByText(/No models & agents match these filters/i)).toBeVisible()
  await page.getByRole('button', { name: 'Forges' }).click()
  await expect(page.getByRole('row', { name: /Stable forge/ })).toBeVisible()
})

test('runner pool refresh replaces cached rows without clearing visible data', async ({ page }) => {
  await session(page)
  let calls = 0
  await page.route(`**/api/v1/orgs/${org}/runner-pools*`, route => {
    calls += 1
    return route.fulfill({ json: { items: [pool('pool-1', calls === 1 ? 'Old pool' : 'New pool')], complete: true } })
  })
  await page.goto(`/org/${org}/runners`)
  await expect(page.getByRole('row', { name: /Old pool/ })).toBeVisible()
  await page.getByRole('button', { name: 'Refresh' }).click()
  await expect(page.getByRole('row', { name: /New pool/ })).toBeVisible()
  await expect(page.getByRole('row', { name: /Old pool/ })).toHaveCount(0)
})

test('runner pool refresh keeps rows visible when response is unchanged', async ({ page }) => {
  await session(page)
  let calls = 0
  let releaseRefresh: (() => void) | undefined
  let refreshStarted: (() => void) | undefined
  const refreshSeen = new Promise<void>(resolve => { refreshStarted = resolve })
  await page.route(`**/api/v1/orgs/${org}/runner-pools*`, async route => {
    calls += 1
    if (calls === 2) {
      refreshStarted?.()
      await new Promise<void>(resolve => { releaseRefresh = resolve })
    }
    return route.fulfill({ json: { items: [pool('pool-stable', 'Stable pool')], complete: true } })
  })
  await page.goto(`/org/${org}/runners`)
  await expect(page.getByRole('row', { name: /Stable pool/ })).toBeVisible()
  await page.getByRole('button', { name: 'Refresh' }).click()
  await refreshSeen
  await expect(page.getByRole('row', { name: /Stable pool/ })).toBeVisible()
  const refetchResponse = page.waitForResponse(response => response.url().includes(`/api/v1/orgs/${org}/runner-pools`) && response.request().method() === 'GET')
  releaseRefresh?.()
  await (await refetchResponse).finished()
  await expect(page.getByRole('row', { name: /Stable pool/ })).toBeVisible()
  await expect.poll(() => calls).toBe(2)
})

test('runner pool search keeps focus during sequential loading', async ({ page }) => {
  await session(page)
  const pending: Array<() => void> = []
  await page.route(`**/api/v1/orgs/${org}/runner-pools*`, async route => {
    const query = new URL(route.request().url()).searchParams.get('q')
    if (query) await new Promise<void>(resolve => pending.push(resolve))
    await route.fulfill({ json: { items: [], complete: true } })
  })
  await page.goto(`/org/${org}/runners`)
  const search = page.getByLabel('Search', { exact: true })
  await search.click()
  await search.pressSequentially('demo', { delay: 100 })
  await expect(search).toBeFocused()
  pending.splice(0).forEach(resolve => resolve())
  await expect.poll(() => pending.length).toBe(0)
})

test('late old organisation response cannot replace current organisation rows', async ({ page }) => {
  await session(page)
  let releaseOld: (() => void) | undefined
  let oldRefreshStarted: (() => void) | undefined
  const oldRefresh = new Promise<void>(resolve => { oldRefreshStarted = resolve })
  let oldCalls = 0
  await page.route('**/api/v1/orgs/*/connections*', async route => {
    const orgID = new URL(route.request().url()).pathname.split('/')[4]
    if (orgID === org) {
      oldCalls += 1
      if (oldCalls === 2) {
        oldRefreshStarted?.()
        await new Promise<void>(resolve => { releaseOld = resolve })
      }
      return route.fulfill({ json: { items: [connection('old-connection', 'Old organisation')], complete: true } })
    }
    return route.fulfill({ json: { items: [connection('new-connection', 'Current organisation', 'forge', 'healthy', otherOrg)], complete: true } })
  })
  await page.goto(`/org/${org}/connections`)
  await expect(page.getByRole('row', { name: /Old organisation/ })).toBeVisible()
  await page.getByRole('button', { name: 'Refresh' }).click()
  await oldRefresh
  const oldRequest = Promise.race([
    page.waitForEvent('requestfinished', { predicate: request => request.url().includes(`/api/v1/orgs/${org}/connections`) && request.method() === 'GET' }),
    page.waitForEvent('requestfailed', { predicate: request => request.url().includes(`/api/v1/orgs/${org}/connections`) && request.method() === 'GET' }),
  ])
  await page.getByRole('button', { name: 'Switch organisation' }).click()
  await page.getByRole('menuitem', { name: 'Other fixture' }).click()
  await expect(page.getByRole('row', { name: /Current organisation/ })).toBeVisible()
  releaseOld?.()
  await oldRequest
  await expect(page.getByRole('row', { name: /Current organisation/ })).toBeVisible()
  await expect(page.getByRole('row', { name: /Old organisation/ })).toHaveCount(0)
})
