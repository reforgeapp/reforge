import { test, expect, type Page, type Route } from '@playwright/test'

const organisation = '00000000-0000-4000-8000-000000000001'
const handoff = '/auth/github/setup/setup-1'
test.use({ trace: 'off' })

type Role = 'owner' | 'admin' | 'viewer'
type GithubAppState = {
  mode: 'manifest' | 'hosted' | 'unavailable'
  reason?: string
  pending?: { id: string; phase: string; app_slug?: string; expires_at: string; resume_url?: string }
}

const session = (role: Role) => ({
  user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' },
  organisations: [{ id: organisation, name: 'Fixture', version: 1, paused: false }],
  memberships: [{ org_id: organisation, role, team_ids: [], repository_ids: [], all_repositories: true }],
  csrf_token: 'csrf-1',
})

const meta = { name: 'Reforge', version: 'test', edition: 'self-hosted', development: true, fixture_auth: true }

async function signIn(page: Page, role: Role = 'owner') {
  await page.route('**/api/v1/session', route => route.fulfill({ json: session(role) }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: meta }))
  await page.goto('/')
  await expect(page).toHaveURL(/\/org\/[^/]+\/overview/)
}

type GithubCalls = { creates: Array<{ headers: Record<string, string>; body: Record<string, unknown> }>; deletes: string[] }

function githubAppRoutes(page: Page, state: GithubAppState, startError?: { status: number; message: string }): GithubCalls {
  const calls: GithubCalls = { creates: [], deletes: [] }
  void page.route(`**/api/v1/orgs/${organisation}/connections**`, route => route.fulfill({ json: { items: [], complete: true } }))
  void page.route(`**/api/v1/orgs/${organisation}/github-app**`, route => handleGithubApp(route, state, calls, startError))
  return calls
}

async function handleGithubApp(route: Route, state: GithubAppState, calls: GithubCalls, startError?: { status: number; message: string }) {
  const url = new URL(route.request().url())
  const method = route.request().method()
  if (method === 'POST' && url.pathname.endsWith('/setups')) {
    calls.creates.push({ headers: route.request().headers(), body: route.request().postDataJSON() as Record<string, unknown> })
    if (startError) return route.fulfill({ status: startError.status, json: { code: 'forbidden', message: startError.message, request_id: 'req-1', retryable: false } })
    if (state.mode === 'unavailable') return route.fulfill({ status: 409, json: { code: 'github_app_unavailable', message: state.reason ?? 'GitHub App setup is unavailable.', request_id: 'req-1', retryable: false } })
    return route.fulfill({ status: 201, json: { id: 'setup-1', handoff_url: handoff } })
  }
  if (method === 'DELETE') {
    calls.deletes.push(url.pathname.split('/').pop() ?? '')
    return route.fulfill({ status: 204 })
  }
  return route.fulfill({ json: state })
}

async function mockHandoff(page: Page) {
  await page.route('**/auth/github/setup/**', route => route.fulfill({ contentType: 'text/html', body: '<!doctype html><html><head><title>Authorize</title></head><body><form method="post" action="https://github.com/settings/apps/new"></form></body></html>' }))
}

async function openAddConnection(page: Page) {
  await page.getByRole('button', { name: 'Add connection' }).click()
  return page.getByRole('dialog', { name: 'Add connection' })
}

test.describe('GitHub App guided setup', () => {
  test('defaults to the guided GitHub App flow and starts a same-origin handoff', async ({ page }) => {
    await signIn(page)
    const calls = githubAppRoutes(page, { mode: 'manifest' })
    await mockHandoff(page)
    await page.goto(`/org/${organisation}/connections`)
    const origin = new URL(page.url()).origin
    const dialog = await openAddConnection(page)
    const guided = dialog.getByRole('region', { name: 'GitHub App setup' })
    await expect(guided).toBeVisible()
    await expect(dialog.getByLabel('Authentication')).toHaveValue('guided')
    await guided.getByRole('button', { name: 'Set up GitHub App' }).click()
    await expect.poll(() => calls.creates.length).toBe(1)
    expect(calls.creates[0].headers['x-csrf-token']).toBe('csrf-1')
    expect(calls.creates[0].body.name).toBe('Reforge')
    await expect.poll(() => page.url()).toContain(handoff)
    expect(new URL(page.url()).origin).toBe(origin)
  })

  test('offers hosted mode as a guided handoff and keeps token setup available', async ({ page }) => {
    await signIn(page)
    const calls = githubAppRoutes(page, { mode: 'hosted' })
    await mockHandoff(page)
    await page.goto(`/org/${organisation}/connections`)
    const dialog = await openAddConnection(page)
    const guided = dialog.getByRole('region', { name: 'GitHub App setup' })
    await expect(guided.getByLabel('Connection name')).toBeVisible()
    await expect(guided.getByRole('button', { name: 'Connect GitHub' })).toBeVisible()
    await dialog.getByLabel('Authentication').selectOption('token')
    await expect(dialog.getByLabel('Personal access token', { exact: true })).toBeVisible()
    await dialog.getByLabel('Authentication').selectOption('guided')
    await guided.getByRole('button', { name: 'Connect GitHub' }).click()
    await expect.poll(() => calls.creates.length).toBe(1)
    await expect.poll(() => page.url()).toContain(handoff)
  })

  test('shows an actionable unavailable reason while token setup stays available', async ({ page }) => {
    await signIn(page)
    githubAppRoutes(page, { mode: 'unavailable', reason: 'not_configured' })
    await page.goto(`/org/${organisation}/connections`)
    const dialog = await openAddConnection(page)
    const unavailable = dialog.getByRole('region', { name: 'GitHub App unavailable' })
    await expect(unavailable).toContainText('GitHub App setup is unavailable')
    await expect(unavailable).toContainText('not configured')
    await expect(dialog.getByRole('button', { name: 'Set up GitHub App' })).toHaveCount(0)
    await unavailable.getByRole('button', { name: 'Use a personal access token' }).click()
    await expect(dialog.getByLabel('Personal access token', { exact: true })).toBeVisible()
  })

  test('surfaces the owner-only server rejection without navigating to GitHub', async ({ page }) => {
    await signIn(page, 'viewer')
    const calls = githubAppRoutes(page, { mode: 'manifest' }, { status: 403, message: 'Only an organisation owner can start GitHub App setup.' })
    await mockHandoff(page)
    await page.goto(`/org/${organisation}/connections`)
    const dialog = await openAddConnection(page)
    await dialog.getByRole('region', { name: 'GitHub App setup' }).getByRole('button', { name: 'Set up GitHub App' }).click()
    await expect(dialog.getByRole('alert')).toContainText('Only an organisation owner can start GitHub App setup.')
    await expect.poll(() => calls.creates.length).toBe(1)
    await expect(page).not.toHaveURL(/auth\/github\/setup/)
  })

  test('resumes an in-progress setup and cancels it without leaving pending state', async ({ page }) => {
    await signIn(page)
    const calls = githubAppRoutes(page, { mode: 'manifest', pending: { id: 'setup-1', phase: 'handed_off', app_slug: 'reforge-fixture', expires_at: '2026-09-24T12:15:00Z', resume_url: handoff } })
    await mockHandoff(page)
    await page.goto(`/org/${organisation}/connections`)
    const dialog = await openAddConnection(page)
    const pending = dialog.getByRole('region', { name: 'GitHub App setup pending' })
    await expect(pending).toContainText('Setup in progress')
    await expect(pending).toContainText('reforge-fixture')
    await pending.getByRole('button', { name: 'Resume setup' }).click()
    await expect.poll(() => page.url()).toContain(handoff)
    await page.goto(`/org/${organisation}/connections`)
    await openAddConnection(page)
    await page.getByRole('region', { name: 'GitHub App setup pending' }).getByRole('button', { name: 'Cancel setup' }).click()
    await expect.poll(() => calls.deletes.length).toBe(1)
    expect(calls.deletes[0]).toBe('setup-1')
  })
})

test.describe('GitHub App guided setup fencing', () => {
  test('does not navigate to GitHub when the start response arrives after an organisation switch', async ({ page }) => {
    const org2 = '00000000-0000-4000-8000-000000000002'
    const twoOrgs = {
      ...session('owner'),
      organisations: [
        { id: organisation, name: 'Fixture', version: 1, paused: false },
        { id: org2, name: 'Second', version: 1, paused: false },
      ],
      memberships: [
        { org_id: organisation, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true },
        { org_id: org2, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true },
      ],
    }
    await page.route('**/api/v1/session', route => route.fulfill({ json: twoOrgs }))
    await page.route('**/api/v1/meta', route => route.fulfill({ json: { name: 'Reforge', version: 'test', edition: 'self-hosted', development: true, fixture_auth: true } }))
    await page.route(`**/api/v1/orgs/${organisation}/connections**`, route => route.fulfill({ json: { items: [], complete: true } }))
    await page.route(`**/api/v1/orgs/${org2}/connections**`, route => route.fulfill({ json: { items: [], complete: true } }))
    await page.route(`**/api/v1/orgs/${org2}/github-app**`, route => route.fulfill({ json: { mode: 'manifest' } }))
    let release: (() => void) | undefined
    const gate = new Promise<void>(resolve => { release = resolve })
    await page.route(`**/api/v1/orgs/${organisation}/github-app/setups`, async route => {
      await gate
      return route.fulfill({ status: 201, json: { id: 'setup-1', handoff_url: handoff } })
    })
    await page.route(`**/api/v1/orgs/${organisation}/github-app**`, route => route.fulfill({ json: { mode: 'manifest' } }))
    await mockHandoff(page)
    await page.goto('/')
    await expect(page).toHaveURL(/\/org\/[^/]+\/overview/)
    await page.goto(`/org/${organisation}/connections`)
    const dialog = await openAddConnection(page)
    await dialog.getByRole('region', { name: 'GitHub App setup' }).getByRole('button', { name: 'Set up GitHub App' }).click()
    await page.evaluate(next => {
      window.history.pushState({}, '', next)
      window.dispatchEvent(new PopStateEvent('popstate'))
    }, `/org/${org2}/connections`)
    await expect(page).toHaveURL(new RegExp(org2))
    release?.()
    await page.waitForTimeout(400)
    await expect(page).not.toHaveURL(/auth\/github\/setup/)
  })
})

test.describe('GitHub setup callback outcomes', () => {
  for (const [result, message] of [['failed', 'GitHub setup failed'], ['expired', 'GitHub setup expired'], ['pending_approval', 'pending approval']] as const) {
    test(`renders the ${result} outcome with a bounded reason and no raw provider text`, async ({ page }) => {
      await signIn(page)
      githubAppRoutes(page, { mode: 'manifest' })
      await page.goto(`/org/${organisation}/connections?github_result=${result}&github_reason=github_org_admin_required`)
      const outcome = page.getByRole('status').filter({ hasText: message })
      await expect(outcome).toBeVisible()
      await expect(outcome).toContainText('A GitHub organisation owner must approve the installation.')
      await expect(outcome).not.toContainText('github_org_admin_required')
      await expect(page.locator('body')).not.toContainText('raw-provider-response')
      await expect(page.locator('body')).not.toContainText('csrf-1')
    })
  }

  test('opens the repository selection step after a connected callback', async ({ page }) => {
    await signIn(page)
    githubAppRoutes(page, { mode: 'manifest' })
    const connection = { id: 'conn-1', org_id: organisation, kind: 'forge', provider: 'github', name: 'GitHub fixture', endpoint: 'https://api.github.com', state: 'healthy', reason: '', version: 2, credential_version: 1, settings: { auth_kind: 'github_app_platform' }, capabilities: {}, verified_at: '2026-09-24T10:00:00Z' }
    await page.route(`**/api/v1/orgs/${organisation}/connections/conn-1`, route => route.fulfill({ json: connection }))
    await page.route(`**/api/v1/orgs/${organisation}/connections/conn-1/test`, route => route.fulfill({ json: connection }))
    await page.route(`**/api/v1/orgs/${organisation}/teams**`, route => route.fulfill({ json: { items: [], complete: true } }))
    await page.route(`**/api/v1/orgs/${organisation}/inventory-syncs**`, route => {
      const url = new URL(route.request().url())
      if (url.pathname.endsWith('/inventory-syncs') && route.request().method() === 'POST') return route.fulfill({ status: 201, json: { id: 'sync-1', kind: 'scan', state: 'complete', version: 4, processed: 2, pages: 1, failures: 0 } })
      if (url.pathname.endsWith('/candidates')) return route.fulfill({ json: { items: [{ native_id: '1', full_name: 'fixture/alpha', archived: false }, { native_id: '2', full_name: 'fixture/beta', archived: false }], complete: true } })
      return route.fulfill({ json: { id: 'sync-1', kind: 'scan', state: 'complete', version: 4, processed: 2, pages: 1, failures: 0 } })
    })
    await page.goto(`/org/${organisation}/connections?github_result=connected&connection=conn-1`)
    await expect(page.getByRole('status').filter({ hasText: 'GitHub App connected' })).toBeVisible()
    const dialog = page.getByRole('dialog', { name: 'Sync forge inventory' })
    await expect(dialog).toBeVisible()
    await expect(dialog.getByRole('checkbox', { name: /fixture\/alpha/ })).toBeVisible()
  })

  test('shows the callback outcome once and clears it from the URL', async ({ page }) => {
    await signIn(page)
    githubAppRoutes(page, { mode: 'manifest' })
    await page.goto(`/org/${organisation}/connections?github_result=expired`)
    await expect(page.getByRole('status').filter({ hasText: 'GitHub setup expired' })).toBeVisible()
    await expect(page).not.toHaveURL(/github_result/)
    await page.goto(`/org/${organisation}/connections?github_result=expired`)
    await expect(page.getByRole('status').filter({ hasText: 'GitHub setup expired' })).toBeVisible()
  })
})
