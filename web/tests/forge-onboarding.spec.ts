import { test, expect, type Page, type Route } from '@playwright/test'

const organisation = '00000000-0000-4000-8000-000000000001'
test.use({ trace: 'off' })

type Role = 'owner' | 'admin' | 'viewer'

const session = (role: Role) => ({
  user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' },
  organisations: [{ id: organisation, name: 'Fixture', version: 1, paused: false }],
  memberships: [{ org_id: organisation, role, team_ids: [], repository_ids: [], all_repositories: true }],
  csrf_token: 'csrf-1',
})

const pem = ['-----BEGIN RSA PRIVATE KEY-----', 'MIIEowIBAAKCAQEAxFixtureOnlyNotARealKeyMaterialForBrowserTests0000000000000', 'MIIEowIBAAKCAQEAxFixtureOnlyNotARealKeyMaterialForBrowserTests0000000000000', '-----END RSA PRIVATE KEY-----', ''].join('\n')

const meta = { name: 'Reforge', version: 'test', edition: 'self-hosted', development: true, fixture_auth: true }

async function signIn(page: Page, role: Role = 'owner') {
  await page.route('**/api/v1/session', route => route.fulfill({ json: session(role) }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: meta }))
  await page.goto('/')
  await expect(page).toHaveURL(/\/org\/[^/]+\/overview/)
}

type Calls = {
  creates: Array<{ headers: Record<string, string>; body: Record<string, unknown> }>
  tests: string[]
  imports: Array<{ headers: Record<string, string>; body: Record<string, unknown> }>
  rotates: Array<{ headers: Record<string, string>; body: Record<string, unknown> }>
}

function connection(id: string, state: string, overrides: Record<string, unknown> = {}) {
  return {
    id,
    org_id: organisation,
    kind: 'forge',
    provider: 'github',
    name: `forge-${id}`,
    endpoint: 'https://api.github.com',
    state,
    reason: state === 'revoked' ? 'Installation revoked by the account owner.' : '',
    version: 3,
    credential_version: 2,
    settings: { auth_kind: 'token' },
    capabilities: {},
    verified_at: '2026-09-24T10:00:00Z',
    ...overrides,
  }
}

function baseRoutes(page: Page): Calls {
  const calls: Calls = { creates: [], tests: [], imports: [], rotates: [] }
  void page.route(`**/api/v1/orgs/${organisation}/connections?**`, route => route.fulfill({ json: { items: [], complete: true } }))
  void page.route(`**/api/v1/orgs/${organisation}/github-app**`, route => route.fulfill({ json: { mode: 'manifest' } }))
  void page.route(`**/api/v1/orgs/${organisation}/connections/forges`, route => {
    calls.creates.push({ headers: route.request().headers(), body: route.request().postDataJSON() as Record<string, unknown> })
    return route.fulfill({ status: 201, json: connection('conn-1', 'unverified') })
  })
  return calls
}

function connectionList(page: Page, items: Array<Record<string, unknown>>) {
  return page.route(`**/api/v1/orgs/${organisation}/connections?**`, route => route.fulfill({ json: { items, complete: true } }))
}

function connectionDetail(page: Page, item: Record<string, unknown>) {
  void page.route(`**/api/v1/orgs/${organisation}/connections/${item.id}`, route => route.fulfill({ json: item }))
  void page.route(`**/api/v1/orgs/${organisation}/connections/${item.id}/webhook`, route => route.fulfill({ status: 404, json: { code: 'webhook_unconfigured', message: 'No webhook configured.', request_id: 'req-hook', retryable: false } }))
}

function emptyInventory(page: Page) {
  void page.route(`**/api/v1/orgs/${organisation}/inventory-syncs**`, route => {
    const url = new URL(route.request().url())
    if (url.pathname.endsWith('/inventory-syncs') && route.request().method() === 'POST') return route.fulfill({ status: 201, json: { id: 'sync-1', kind: 'scan', state: 'complete', version: 4, processed: 0, pages: 0, failures: 0 } })
    if (url.pathname.endsWith('/candidates')) return route.fulfill({ json: { items: [], complete: true } })
    return route.fulfill({ json: { id: 'sync-1', kind: 'scan', state: 'complete', version: 4, processed: 0, pages: 0, failures: 0 } })
  })
}

async function openAddConnection(page: Page) {
  await page.getByRole('button', { name: 'Add connection' }).click()
  return page.getByRole('dialog', { name: 'Add connection' })
}

test.describe('forge provider onboarding', () => {
  test('matches the add form to the active category without a Kind field', async ({ page }) => {
    await signIn(page)
    const calls = baseRoutes(page)
    let deliveryKind: string | undefined
    await page.route(`**/api/v1/orgs/${organisation}/connections/delivery`, route => {
      deliveryKind = String((route.request().postDataJSON() as Record<string, unknown>).kind)
      return route.fulfill({ status: 201, json: connection('conn-delivery', 'unverified', { kind: 'delivery' }) })
    })
    await page.route(`**/api/v1/orgs/${organisation}/connections/conn-delivery/test`, route => route.fulfill({ json: connection('conn-delivery', 'degraded', { kind: 'delivery', reason: 'Delivery test failed.' }) }))
    await page.route(`**/api/v1/orgs/${organisation}/connections/conn-delivery`, route => route.fulfill({ json: connection('conn-delivery', 'unverified', { kind: 'delivery' }) }))
    await page.goto(`/org/${organisation}/connections`)
    await page.getByRole('button', { name: 'Delivery' }).click()
    const delivery = await openAddConnection(page)
    await expect(delivery.getByLabel('Kind')).toHaveCount(0)
    await expect(delivery.getByLabel('Provider')).toBeVisible()
    await delivery.getByLabel('Authentication').selectOption('token')
    await delivery.getByLabel('Name', { exact: true }).fill('delivery-conn')
    await delivery.getByLabel('Personal access token', { exact: true }).fill('ghp_fixture_token')
    await delivery.getByRole('button', { name: 'Create connection' }).click()
    await expect.poll(() => deliveryKind).toBe('delivery')
    expect(calls.creates).toHaveLength(0)
    await expect(delivery.getByText('Connection saved; capability test failed.')).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(delivery).toBeHidden()

    await page.getByRole('button', { name: 'Models & agents' }).click()
    const models = await openAddConnection(page)
    await expect(models.getByLabel('Connection type')).toHaveValue('model')
    await expect(models.getByLabel('Kind')).toHaveCount(0)
  })

  test('rejects a repository URL entered as the API address before any request', async ({ page }) => {
    await signIn(page)
    const calls = baseRoutes(page)
    await page.goto(`/org/${organisation}/connections`)
    const dialog = await openAddConnection(page)
    await dialog.getByLabel('Authentication').selectOption('token')
    await dialog.getByLabel('Name', { exact: true }).fill('repo-url-guard')
    await dialog.getByText('Advanced', { exact: true }).click()
    await dialog.getByLabel('API address').fill('https://github.com/octocat/Hello-World')
    await dialog.getByLabel('Repository filter').fill('https://github.com/octocat/Hello-World')
    await dialog.getByLabel('Personal access token', { exact: true }).fill('ghp_fixture_token')
    await expect(dialog.getByRole('alert')).toContainText('That looks like a repository URL')
    await dialog.getByRole('button', { name: 'Create connection' }).click()
    expect(calls.creates).toHaveLength(0)
  })

  test('keeps a failed token connection for retry instead of creating a duplicate', async ({ page }) => {
    await signIn(page)
    const calls = baseRoutes(page)
    await page.route(`**/api/v1/orgs/${organisation}/connections/forges`, route => {
      calls.creates.push({ headers: route.request().headers(), body: route.request().postDataJSON() as Record<string, unknown> })
      return route.fulfill({ status: 201, json: connection('conn-retry', 'unverified') })
    })
    let attempts = 0
    await page.route(`**/api/v1/orgs/${organisation}/connections/conn-retry/test`, route => {
      attempts += 1
      calls.tests.push('conn-retry')
      if (attempts === 1) return route.fulfill({ status: 502, json: { code: 'probe_failed', message: 'GitHub rejected the personal access token.', request_id: 'req-2', retryable: true } })
      return route.fulfill({ json: connection('conn-retry', 'healthy') })
    })
    await page.route(`**/api/v1/orgs/${organisation}/connections/conn-retry`, route => route.fulfill({ json: connection('conn-retry', attempts > 1 ? 'healthy' : 'unverified') }))
    await page.route(`**/api/v1/orgs/${organisation}/teams**`, route => route.fulfill({ json: { items: [], complete: true } }))
    emptyInventory(page)
    await page.goto(`/org/${organisation}/connections`)
    const dialog = await openAddConnection(page)
    await dialog.getByLabel('Authentication').selectOption('token')
    await dialog.getByLabel('Name', { exact: true }).fill('token-retry')
    await dialog.getByLabel('Personal access token', { exact: true }).fill('ghp_fixture_token')
    await dialog.getByRole('button', { name: 'Create connection' }).click()
    await expect(dialog.getByText('Connection saved; capability test failed.')).toBeVisible()
    await expect(dialog.getByRole('alert')).toContainText('GitHub rejected the personal access token.')
    await dialog.getByRole('button', { name: 'Retry test' }).click()
    await expect.poll(() => attempts).toBe(2)
    expect(calls.creates).toHaveLength(1)
    expect(calls.tests).toEqual(['conn-retry', 'conn-retry'])
  })

  test('previews candidates and imports through the real inventory contract', async ({ page }) => {
    await signIn(page)
    const calls = baseRoutes(page)
    const healthy = connection('conn-1', 'healthy')
    connectionList(page, [healthy])
    connectionDetail(page, healthy)
    void page.route(`**/api/v1/orgs/${organisation}/teams**`, route => route.fulfill({ json: { items: [], complete: true } }))
    let importVersion: string | undefined
    void page.route(`**/api/v1/orgs/${organisation}/inventory-syncs**`, route => {
      const url = new URL(route.request().url())
      const method = route.request().method()
      if (url.pathname.endsWith('/inventory-syncs') && method === 'POST') return route.fulfill({ status: 201, json: { id: 'sync-1', kind: 'scan', state: 'complete', version: 4, processed: 2, pages: 1, failures: 0 } })
      if (url.pathname.endsWith('/candidates')) return route.fulfill({ json: { items: [{ native_id: '1', full_name: 'octocat/Hello-World', archived: false }, { native_id: '2', full_name: 'octocat/Spoon-Knife', archived: false }], complete: true } })
      if (url.pathname.endsWith('/import') && method === 'POST') {
        importVersion = route.request().headers()['if-match']
        calls.imports.push({ headers: route.request().headers(), body: route.request().postDataJSON() as Record<string, unknown> })
        return route.fulfill({ json: { id: 'sync-1', kind: 'import', state: 'complete', version: 5, processed: 2, pages: 1, failures: 0 } })
      }
      return route.fulfill({ json: { id: 'sync-1', kind: 'scan', state: 'complete', version: 4, processed: 2, pages: 1, failures: 0 } })
    })
    await page.goto(`/org/${organisation}/connections?connection=conn-1`)
    const detail = page.getByRole('region', { name: 'forge-conn-1' })
    await expect(detail).toBeVisible()
    await detail.getByRole('button', { name: 'Add repositories' }).click()
    const dialog = page.getByRole('dialog', { name: 'Sync forge inventory' })
    await expect(dialog.getByRole('checkbox', { name: /octocat\/Hello-World/ })).toBeVisible()
    await dialog.getByRole('button', { name: /Import selected/ }).click()
    await expect.poll(() => calls.imports.length).toBe(1)
    expect(importVersion).toBe('"4"')
    await expect(dialog.getByText('Import complete. Repository inventory will refresh.')).toBeVisible()
  })

  test('accepts a multiline RSA PEM from a file and preserves newlines', async ({ page }) => {
    await signIn(page)
    const calls = baseRoutes(page)
    await page.route(`**/api/v1/orgs/${organisation}/connections/forges`, route => {
      calls.creates.push({ headers: route.request().headers(), body: route.request().postDataJSON() as Record<string, unknown> })
      return route.fulfill({ status: 201, json: connection('conn-pem', 'unverified', { settings: { auth_kind: 'github_app', app_id: '123', installation_id: '456' } }) })
    })
    await page.goto(`/org/${organisation}/connections`)
    const dialog = await openAddConnection(page)
    await dialog.getByLabel('Authentication').selectOption('app')
    await dialog.getByLabel('Name', { exact: true }).fill('manual-app')
    await dialog.getByLabel('App ID').fill('123')
    await dialog.getByLabel('Installation ID').fill('456')
    await dialog.getByLabel('Private key file').setInputFiles({ name: 'app.pem', mimeType: 'application/x-pem-file', buffer: Buffer.from(pem) })
    await expect(dialog.getByText('Loaded app.pem.')).toBeVisible()
    await dialog.getByRole('button', { name: 'Create connection' }).click()
    await expect.poll(() => calls.creates.length).toBe(1)
    const secret = String(calls.creates[0].body.secret ?? '')
    expect(secret).toContain('-----BEGIN RSA PRIVATE KEY-----')
    expect(secret).toContain('-----END RSA PRIVATE KEY-----')
    expect(secret).toContain('\n')
  })

  test('bounds oversized PEM input and refuses to submit', async ({ page }) => {
    await signIn(page)
    const calls = baseRoutes(page)
    await page.goto(`/org/${organisation}/connections`)
    const dialog = await openAddConnection(page)
    await dialog.getByLabel('Authentication').selectOption('app')
    await dialog.getByLabel('Name', { exact: true }).fill('oversized-app')
    await dialog.getByLabel('App ID').fill('123')
    await dialog.getByLabel('Installation ID').fill('456')
    await dialog.getByLabel('Private key file').setInputFiles({ name: 'big.pem', mimeType: 'text/plain', buffer: Buffer.from('A'.repeat(70 * 1024)) })
    await expect(dialog.getByRole('alert')).toContainText('larger than 64 KiB')
    await expect(dialog.getByRole('button', { name: 'Create connection' })).toBeDisabled()
    expect(calls.creates).toHaveLength(0)
  })

  test('keeps a Gitea instance URL separate from the repository filter', async ({ page }) => {
    await signIn(page)
    const calls = baseRoutes(page)
    await page.route(`**/api/v1/orgs/${organisation}/connections/forges`, route => {
      calls.creates.push({ headers: route.request().headers(), body: route.request().postDataJSON() as Record<string, unknown> })
      return route.fulfill({ status: 201, json: connection('conn-gitea', 'unverified', { provider: 'gitea', endpoint: 'https://gitea.example.com' }) })
    })
    await page.goto(`/org/${organisation}/connections`)
    const dialog = await openAddConnection(page)
    await dialog.getByLabel('Provider').selectOption('gitea')
    await dialog.getByLabel('Server URL').fill('https://gitea.example.com')
    await dialog.getByLabel('Name', { exact: true }).fill('gitea-instance')
    await dialog.getByLabel('Personal access token', { exact: true }).fill('gitea_fixture_token')
    await dialog.getByLabel('Repository filter').fill('https://gitea.example.com/owner/repo')
    await expect(dialog.getByRole('alert')).toHaveCount(0)
    await dialog.getByRole('button', { name: 'Create connection' }).click()
    await expect.poll(() => calls.creates.length).toBe(1)
    expect(calls.creates[0].body.endpoint).toBe('https://gitea.example.com')
    expect(calls.creates[0].body.provider).toBe('gitea')
  })

  test('links the token page to the entered self-managed instance host', async ({ page }) => {
    await signIn(page)
    baseRoutes(page)
    await page.goto(`/org/${organisation}/connections`)
    const dialog = await openAddConnection(page)
    await dialog.getByLabel('Provider').selectOption('gitea')
    await dialog.getByLabel('Server URL').fill('https://gitea.internal.example')
    await expect(dialog.getByRole('link', { name: 'Get token' })).toHaveAttribute('href', 'https://gitea.internal.example/user/settings/applications')
    await dialog.getByLabel('Provider').selectOption('gitlab')
    await dialog.getByText('Advanced', { exact: true }).click()
    await dialog.getByLabel('API address').fill('https://gitlab.internal.example')
    await expect(dialog.getByRole('link', { name: 'Get token' })).toHaveAttribute('href', 'https://gitlab.internal.example/-/user_settings/personal_access_tokens')
    await dialog.getByLabel('API address').fill('javascript:alert(1)')
    await expect(dialog.getByRole('link', { name: 'Get token' })).toHaveAttribute('href', 'https://gitlab.com/-/user_settings/personal_access_tokens')
  })
})

test.describe('forge connection detail', () => {
  test('shows provider, API address, auth, last checked and status without model fields', async ({ page }) => {
    await signIn(page)
    baseRoutes(page)
    const healthy = connection('conn-1', 'healthy')
    connectionList(page, [healthy])
    connectionDetail(page, healthy)
    await page.goto(`/org/${organisation}/connections?connection=conn-1`)
    const detail = page.getByRole('region', { name: 'forge-conn-1' })
    await expect(detail).toBeVisible()
    await expect(detail).toContainText('api.github.com')
    await expect(detail.getByText('API address')).toBeVisible()
    await expect(detail.getByText('Authentication')).toBeVisible()
    await expect(detail.getByText('Last checked')).toBeVisible()
    await expect(detail.getByText('Billing route')).toHaveCount(0)
    await expect(detail.getByText('Runtime / protocol')).toHaveCount(0)
    await expect(detail.getByRole('button', { name: 'Add repositories' })).toBeEnabled()
    await expect(detail.getByRole('button', { name: 'Test connection' })).toBeEnabled()
  })

  test('disables test, revoke and import for a revoked forge and shows the reason', async ({ page }) => {
    await signIn(page)
    baseRoutes(page)
    const revoked = connection('conn-revoked', 'revoked')
    connectionList(page, [revoked])
    connectionDetail(page, revoked)
    await page.goto(`/org/${organisation}/connections?connection=conn-revoked`)
    const detail = page.getByRole('region', { name: 'forge-conn-revoked' })
    await expect(detail).toBeVisible()
    await expect(detail.getByRole('button', { name: 'Test connection' })).toBeDisabled()
    await expect(detail.getByRole('button', { name: 'Revoke', exact: true })).toBeDisabled()
    await expect(detail.getByRole('button', { name: 'Add repositories' })).toBeDisabled()
    await expect(detail).toContainText('Installation revoked by the account owner.')
  })

  test('shows managed GitHub App authentication and a webhook configuration link', async ({ page }) => {
    await signIn(page)
    baseRoutes(page)
    const managed = connection('conn-managed', 'healthy', { settings: { auth_kind: 'github_app', managed: 'github_manifest' } })
    connectionList(page, [managed])
    await page.route(`**/api/v1/orgs/${organisation}/connections/conn-managed`, route => route.fulfill({ json: managed }))
    await page.route(`**/api/v1/orgs/${organisation}/connections/conn-managed/webhook`, route => route.fulfill({ json: { path: '/hooks/github/app', version: 1, revoked: false } }))
    await page.goto(`/org/${organisation}/connections?connection=conn-managed`)
    const detail = page.getByRole('region', { name: 'forge-conn-managed' })
    await expect(detail).toContainText('GitHub App')
    await expect(detail).not.toContainText('GitHub App (manual)')
    await detail.getByRole('button', { name: 'Actions', exact: true }).click()
    await expect(detail.getByRole('link', { name: 'Configure on GitHub' })).toBeVisible()
    await expect(detail.getByRole('button', { name: 'Rotate credential' })).toHaveCount(0)
    await expect(detail.getByRole('button', { name: /webhook secret/i })).toHaveCount(0)
  })

  test('restores the selected forge connection after reload', async ({ page }) => {
    await signIn(page)
    baseRoutes(page)
    const healthy = connection('conn-1', 'healthy')
    connectionList(page, [healthy])
    connectionDetail(page, healthy)
    await page.goto(`/org/${organisation}/connections?connection=conn-1`)
    await expect(page.getByRole('region', { name: 'forge-conn-1' })).toBeVisible()
    await page.reload()
    await expect(page.getByRole('region', { name: 'forge-conn-1' })).toBeVisible()
    await expect(page).toHaveURL(/connection=conn-1/)
  })
})

test.describe('forge onboarding negative cases', () => {
  test('does not open repository import when a 200 capability test is unhealthy', async ({ page }) => {
    await signIn(page)
    const calls = baseRoutes(page)
    await page.route(`**/api/v1/orgs/${organisation}/connections/forges`, route => {
      calls.creates.push({ headers: route.request().headers(), body: route.request().postDataJSON() as Record<string, unknown> })
      return route.fulfill({ status: 201, json: connection('conn-unhealthy', 'unverified') })
    })
    await page.route(`**/api/v1/orgs/${organisation}/connections/conn-unhealthy/test`, route => route.fulfill({ json: connection('conn-unhealthy', 'degraded', { reason: 'The token is missing repository read access.' }) }))
    await page.route(`**/api/v1/orgs/${organisation}/connections/conn-unhealthy`, route => route.fulfill({ json: connection('conn-unhealthy', 'unverified') }))
    await page.goto(`/org/${organisation}/connections`)
    const dialog = await openAddConnection(page)
    await dialog.getByLabel('Authentication').selectOption('token')
    await dialog.getByLabel('Name', { exact: true }).fill('unhealthy-token')
    await dialog.getByLabel('Personal access token', { exact: true }).fill('ghp_fixture_token')
    await dialog.getByRole('button', { name: 'Create connection' }).click()
    await expect(dialog.getByText('Connection saved; capability test failed.')).toBeVisible()
    await expect(dialog.getByRole('alert')).toContainText('The token is missing repository read access.')
    await expect(dialog.getByRole('button', { name: 'Continue to repositories' })).toHaveCount(0)
    await expect(page.getByRole('dialog', { name: 'Sync forge inventory' })).toHaveCount(0)
    expect(calls.creates).toHaveLength(1)
  })

  test('updates a failed token credential in place and retests the same connection', async ({ page }) => {
    await signIn(page)
    const calls = baseRoutes(page)
    await page.route(`**/api/v1/orgs/${organisation}/connections/forges`, route => {
      calls.creates.push({ headers: route.request().headers(), body: route.request().postDataJSON() as Record<string, unknown> })
      return route.fulfill({ status: 201, json: connection('conn-rotate', 'unverified', { version: 4 }) })
    })
    let testAttempts = 0
    await page.route(`**/api/v1/orgs/${organisation}/connections/conn-rotate/test`, route => {
      testAttempts += 1
      calls.tests.push('conn-rotate')
      if (testAttempts === 1) return route.fulfill({ json: connection('conn-rotate', 'degraded', { reason: 'The token is missing repository read access.' }) })
      return route.fulfill({ json: connection('conn-rotate', 'healthy', { version: 5 }) })
    })
    await page.route(`**/api/v1/orgs/${organisation}/connections/conn-rotate/rotate`, route => {
      calls.rotates.push({ headers: route.request().headers(), body: route.request().postDataJSON() as Record<string, unknown> })
      return route.fulfill({ json: connection('conn-rotate', 'unverified', { version: 5 }) })
    })
    await page.route(`**/api/v1/orgs/${organisation}/connections/conn-rotate`, route => route.fulfill({ json: connection('conn-rotate', 'unverified', { version: 4 }) }))
    await page.route(`**/api/v1/orgs/${organisation}/teams**`, route => route.fulfill({ json: { items: [], complete: true } }))
    emptyInventory(page)
    await page.goto(`/org/${organisation}/connections`)
    const dialog = await openAddConnection(page)
    await dialog.getByLabel('Authentication').selectOption('token')
    await dialog.getByLabel('Name', { exact: true }).fill('token-update')
    await dialog.getByLabel('Personal access token', { exact: true }).fill('ghp_wrong_token')
    await dialog.getByRole('button', { name: 'Create connection' }).click()
    await expect(dialog.getByText('Connection saved; capability test failed.')).toBeVisible()
    await expect(dialog.getByRole('button', { name: 'Continue to repositories' })).toHaveCount(0)
    await dialog.getByLabel('Personal access token', { exact: true }).fill('ghp_corrected_token')
    await dialog.getByRole('button', { name: 'Update credentials' }).click()
    await expect.poll(() => calls.rotates.length).toBe(1)
    expect(calls.rotates[0].body.secret).toBe('ghp_corrected_token')
    expect(calls.creates).toHaveLength(1)
    expect(calls.tests).toEqual(['conn-rotate', 'conn-rotate'])
    await expect(page.getByRole('dialog', { name: 'Sync forge inventory' })).toBeVisible()
  })

  test('imports only the filtered visible repositories and hides Import all', async ({ page }) => {
    await signIn(page)
    const calls = baseRoutes(page)
    const healthy = connection('conn-filter', 'healthy')
    await page.route(`**/api/v1/orgs/${organisation}/connections/forges`, route => {
      calls.creates.push({ headers: route.request().headers(), body: route.request().postDataJSON() as Record<string, unknown> })
      return route.fulfill({ status: 201, json: connection('conn-filter', 'unverified') })
    })
    await page.route(`**/api/v1/orgs/${organisation}/connections/conn-filter/test`, route => route.fulfill({ json: healthy }))
    await page.route(`**/api/v1/orgs/${organisation}/connections/conn-filter`, route => route.fulfill({ json: healthy }))
    await page.route(`**/api/v1/orgs/${organisation}/teams**`, route => route.fulfill({ json: { items: [], complete: true } }))
    await page.route(`**/api/v1/orgs/${organisation}/inventory-syncs**`, route => {
      const url = new URL(route.request().url())
      const method = route.request().method()
      if (url.pathname.endsWith('/inventory-syncs') && method === 'POST') return route.fulfill({ status: 201, json: { id: 'sync-f', kind: 'scan', state: 'complete', version: 4, processed: 3, pages: 1, failures: 0 } })
      if (url.pathname.endsWith('/candidates')) return route.fulfill({ json: { items: [{ native_id: '1', full_name: 'octocat/Hello-World', archived: false }, { native_id: '2', full_name: 'octocat/Spoon-Knife', archived: false }, { native_id: '3', full_name: 'other/Hidden-Repo', archived: false }], complete: true } })
      if (url.pathname.endsWith('/import') && method === 'POST') {
        calls.imports.push({ headers: route.request().headers(), body: route.request().postDataJSON() as Record<string, unknown> })
        return route.fulfill({ json: { id: 'sync-f', kind: 'import', state: 'complete', version: 5, processed: 1, pages: 1, failures: 0 } })
      }
      return route.fulfill({ json: { id: 'sync-f', kind: 'scan', state: 'complete', version: 4, processed: 3, pages: 1, failures: 0 } })
    })
    await page.goto(`/org/${organisation}/connections`)
    const dialog = await openAddConnection(page)
    await dialog.getByLabel('Authentication').selectOption('token')
    await dialog.getByLabel('Name', { exact: true }).fill('filtered-import')
    await dialog.getByLabel('Personal access token', { exact: true }).fill('ghp_fixture_token')
    await dialog.getByLabel('Repository filter').fill('https://github.com/octocat/Hello-World')
    await dialog.getByRole('button', { name: 'Create connection' }).click()
    const sync = page.getByRole('dialog', { name: 'Sync forge inventory' })
    await expect(sync.getByRole('checkbox', { name: /octocat\/Hello-World/ })).toBeVisible()
    await expect(sync.getByRole('checkbox', { name: /other\/Hidden-Repo/ })).toHaveCount(0)
    await expect(sync.getByRole('button', { name: 'Import all' })).toHaveCount(0)
    await sync.getByRole('button', { name: /Import selected/ }).click()
    await expect.poll(() => calls.imports.length).toBe(1)
    expect(calls.imports[0].body.native_ids).toEqual(['1'])
  })
})

test.describe('forge onboarding accessibility', () => {
  test('is keyboard operable without horizontal overflow at 390px', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await signIn(page)
    baseRoutes(page)
    await page.goto(`/org/${organisation}/connections`)
    const dialog = await openAddConnection(page)
    await expect.poll(() => dialog.evaluate(element => element.contains(document.activeElement))).toBe(true)
    await dialog.getByLabel('Provider').focus()
    await page.keyboard.press('Tab')
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390)
    expect(await dialog.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true)
    await page.keyboard.press('Escape')
    await expect(dialog).toBeHidden()
  })

  test('keeps the forge dialog visible in dark mode', async ({ page }) => {
    await signIn(page)
    baseRoutes(page)
    await page.getByRole('button', { name: /switch to dark theme/i }).click()
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
    await page.goto(`/org/${organisation}/connections`)
    const dialog = await openAddConnection(page)
    const background = await dialog.evaluate(element => getComputedStyle(element).backgroundColor)
    expect(background).not.toBe('rgba(0, 0, 0, 0)')
    await expect(dialog.getByLabel('Provider')).toBeVisible()
  })
})
