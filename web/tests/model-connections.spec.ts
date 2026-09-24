import { mkdir } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { test, expect, type Locator, type Page, type Route } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'
const orgTwo = '00000000-0000-4000-8000-000000000002'
const apiKey = 'sk-test-model-onboarding-not-a-real-key'
const artifactDir = process.env.REFORGE_MODEL_CONNECTIONS_ARTIFACTS || fileURLToPath(new URL('../../.local/model-connections/browser/', import.meta.url))

test.use({ trace: 'off' })

type CatalogBody = {
  provider: string
  endpoint: string
  secret: string
  settings: { profile: string; auth_kind: string; billing_route: string; ca_pem?: string }
}
type CreateBody = {
  kind: string
  provider: string
  name: string
  endpoint: string
  settings: Record<string, unknown>
  secret?: string
}
type CatalogResult = { status?: number; json?: unknown; abort?: boolean }

const session = (orgs: string[] = [org]) => ({
  user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' },
  organisations: orgs.map((id, index) => ({ id, name: `Fixture ${index}`, version: 1, paused: false })),
  memberships: orgs.map(id => ({ org_id: id, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true })),
  csrf_token: 'csrf-1',
})

const connectionRow = (overrides: Record<string, unknown> = {}) => ({
  id: 'conn-model-1',
  org_id: org,
  kind: 'model',
  provider: 'openai',
  name: 'OpenAI',
  endpoint: 'https://api.openai.com/v1',
  state: 'unverified',
  reason: '',
  version: 1,
  credential_version: 1,
  settings: {},
  capabilities: {},
  verified_at: null,
  ...overrides,
})

async function mockSession(page: Page, orgs: string[] = [org]) {
  await page.route('**/api/v1/session', route => route.fulfill({ json: session(orgs) }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { name: 'Reforge', version: 'test', edition: 'self-hosted', development: true, fixture_auth: true } }))
}

type Captured = {
  catalog: CatalogBody[]
  create: CreateBody[]
  test: Array<{ id: string; ifMatch: string }>
  get: string[]
  urls: string[]
}

type MockConnectionsConfig = {
  rows?: () => unknown[]
  catalog?: (body: CatalogBody) => Promise<CatalogResult> | CatalogResult
  create?: (body: CreateBody) => Promise<CatalogResult> | CatalogResult
  test?: (id: string, ifMatch: string) => Promise<CatalogResult> | CatalogResult
  connection?: (id: string) => Promise<CatalogResult> | CatalogResult
}

async function fulfill(route: Route, result: CatalogResult) {
  if (result.abort) return route.abort('connectionreset')
  try { await route.fulfill(result) } catch { /* aborted or navigated request */ }
}

async function mockConnections(page: Page, config: MockConnectionsConfig = {}): Promise<Captured> {
  const captured: Captured = { catalog: [], create: [], test: [], get: [], urls: [] }
  page.on('request', request => captured.urls.push(request.url()))
  await page.route('**/api/v1/orgs/*/runner-pools**', route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route('**/api/v1/orgs/*/connections**', async route => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname
    const method = request.method()
    if (path.endsWith('/model-catalog') && method === 'POST') {
      const body = request.postDataJSON() as CatalogBody
      captured.catalog.push(body)
      return fulfill(route, config.catalog ? await config.catalog(body) : { json: { items: [] } })
    }
    if (path.endsWith('/connections/models') && method === 'POST') {
      const body = request.postDataJSON() as CreateBody
      captured.create.push(body)
      return fulfill(route, config.create ? await config.create(body) : { status: 201, json: connectionRow({ provider: body.provider, name: body.name, endpoint: body.endpoint }) })
    }
    const testMatch = path.match(/\/connections\/([^/]+)\/test$/)
    if (testMatch && method === 'POST') {
      const ifMatch = request.headers()['if-match'] ?? ''
      captured.test.push({ id: testMatch[1], ifMatch })
      return fulfill(route, config.test ? await config.test(testMatch[1], ifMatch) : { json: connectionRow({ version: 2, state: 'healthy' }) })
    }
    const connectionMatch = path.match(/\/connections\/([^/]+)$/)
    if (connectionMatch && method === 'GET') {
      captured.get.push(connectionMatch[1])
      return fulfill(route, config.connection ? await config.connection(connectionMatch[1]) : { json: connectionRow() })
    }
    if (method === 'GET' && path.endsWith('/connections')) {
      return route.fulfill({ json: { items: config.rows ? config.rows() : [], complete: true } })
    }
    return route.fulfill({ json: { items: [], complete: true } })
  })
  return captured
}

async function openModelForm(page: Page) {
  await page.goto(`/org/${org}/connections`)
  await page.getByRole('button', { name: 'Add connection' }).click()
  const dialog = page.getByRole('dialog', { name: 'Add connection' })
  await expect(dialog).toBeVisible()
  await selectOption(dialog.getByLabel('Kind'), 'model', 'Model API')
  await expect(dialog.getByLabel('Provider')).toBeVisible()
  return dialog
}

async function selectOption(select: Locator, value: string, label: string) {
  const values = await select.locator('option').evaluateAll(options => options.map(option => (option as HTMLOptionElement).value))
  if (values.includes(value)) await select.selectOption(value)
  else await select.selectOption({ label })
}

function modelSelect(dialog: Locator) {
  return dialog.getByRole('combobox', { name: 'Model', exact: true })
}

async function pickCatalogModel(dialog: Locator, id: string, name: string) {
  const model = modelSelect(dialog)
  const values = await model.locator('option').evaluateAll(options => options.map(option => (option as HTMLOptionElement).value))
  await model.selectOption(values.includes(id) ? id : { label: name })
}

async function fillApiKey(dialog: Locator) {
  await dialog.getByLabel(/api key/i).fill(apiKey)
}

async function openAdvanced(dialog: Locator) {
  const summary = dialog.getByText('Advanced', { exact: true })
  if (await summary.count()) await summary.first().click()
}

const providerCases = [
  { label: 'OpenAI', id: 'openai', provider: 'openai', profile: 'responses', endpoint: 'https://api.openai.com/v1' },
  { label: 'Claude (Anthropic)', id: 'anthropic', provider: 'anthropic', profile: 'messages', endpoint: 'https://api.anthropic.com' },
  { label: 'Gemini (Google)', id: 'google', provider: 'google', profile: 'gemini', endpoint: 'https://generativelanguage.googleapis.com' },
  { label: 'OpenCode Zen', id: 'opencode_zen', provider: 'compatible', profile: 'opencode_zen', endpoint: 'https://opencode.ai/zen/v1' },
  { label: 'OpenCode Go', id: 'opencode_go', provider: 'compatible', profile: 'opencode_go', endpoint: 'https://opencode.ai/zen/go/v1' },
]

test.describe('model connection onboarding', () => {
  test('selects a provider, lists models, saves with the frozen request contract and reloads without leaking the key', async ({ page }) => {
    await mockSession(page)
    let rows: unknown[] = []
    const captured = await mockConnections(page, {
      rows: () => rows,
      catalog: () => ({ json: { items: [{ id: 'gpt-5', name: 'GPT-5' }, { id: 'gpt-5-mini', name: 'GPT-5 mini' }] } }),
    })
    const dialog = await openModelForm(page)
    await fillApiKey(dialog)
    await dialog.getByRole('button', { name: 'Test connection' }).click()
    await expect.poll(() => captured.catalog.length).toBe(1)
    expect(captured.catalog[0]).toMatchObject({ provider: 'openai', endpoint: 'https://api.openai.com/v1', secret: apiKey, settings: { profile: 'responses', auth_kind: 'api_key', billing_route: 'direct_api' } })
    await expect(modelSelect(dialog)).toBeVisible()
    await expect(dialog.getByText(/models loaded/i)).toBeVisible()
    await expect(dialog.getByText(/key verified|verified key|healthy/i)).toHaveCount(0)

    await pickCatalogModel(dialog, 'gpt-5', 'GPT-5')
    rows = [connectionRow({ name: 'OpenAI', settings: { model: 'gpt-5' } })]
    await dialog.getByRole('button', { name: 'Save connection' }).click()
    await expect.poll(() => captured.create.length).toBe(1)
    expect(captured.create[0]).toMatchObject({ kind: 'model', provider: 'openai', endpoint: 'https://api.openai.com/v1', secret: apiKey, settings: { auth_kind: 'api_key', billing_route: 'direct_api', profile: 'responses', model: 'gpt-5' } })

    await expect.poll(() => captured.test.length).toBe(1)
    expect(captured.test[0]).toEqual({ id: 'conn-model-1', ifMatch: '"1"' })

    await page.reload()
    await page.getByRole('button', { name: 'Models & agents' }).click()
    await expect(page.getByRole('row', { name: /OpenAI/ })).toBeVisible()
    expect(await page.getByText(apiKey).count()).toBe(0)
    expect(captured.urls.filter(url => url.includes(apiKey))).toEqual([])
  })

  test('shows a saved public-gateway connection as amber catalog only, not green healthy', async ({ page }) => {
    await mockSession(page)
    let rows: unknown[] = []
    const captured = await mockConnections(page, {
      rows: () => rows,
      catalog: () => ({ json: { items: [{ id: 'zen-1', name: 'Zen Model' }] } }),
      create: () => ({ status: 201, json: connectionRow({ provider: 'compatible', endpoint: 'https://opencode.ai/zen/v1', settings: { profile: 'opencode_zen', model: 'zen-1' } }) }),
      test: () => ({ json: connectionRow({ version: 2, state: 'healthy' }) }),
    })
    const dialog = await openModelForm(page)
    await selectOption(dialog.getByLabel('Provider'), 'opencode_zen', 'OpenCode Zen')
    await fillApiKey(dialog)
    await dialog.getByRole('button', { name: 'Test connection' }).click()
    await expect(modelSelect(dialog)).toBeVisible()
    await pickCatalogModel(dialog, 'zen-1', 'Zen Model')
    rows = [connectionRow({ name: 'OpenCode', provider: 'compatible', state: 'healthy', settings: { profile: 'opencode_zen', model: 'zen-1' } })]
    await dialog.getByRole('button', { name: 'Save connection' }).click()
    await expect.poll(() => captured.test.length).toBe(1)

    await page.reload()
    await page.getByRole('button', { name: 'Models & agents' }).click()
    const row = page.getByRole('row', { name: /OpenCode/ })
    await expect(row).toBeVisible()
    await expect(row.locator('.status-badge')).toHaveText('Catalog only')
    await expect(row.locator('.status-badge')).toHaveClass(/status-amber/)
    await expect(row.getByText('healthy')).toHaveCount(0)
  })

  test('ignores a delayed catalog response after the provider changes and clears key and model on reopen', async ({ page }) => {
    await mockSession(page)
    let release!: () => void
    const gate = new Promise<void>(resolveGate => { release = resolveGate })
    let delay = true
    const captured = await mockConnections(page, {
      catalog: async () => {
        if (delay) await gate
        return { json: { items: [{ id: 'stale-model', name: 'Stale Model' }] } }
      },
    })
    const dialog = await openModelForm(page)
    await fillApiKey(dialog)
    await dialog.getByRole('button', { name: 'Test connection' }).click()
    await expect.poll(() => captured.catalog.length).toBe(1)
    delay = false
    await selectOption(dialog.getByLabel('Provider'), 'anthropic', 'Claude (Anthropic)')
    release()
    await expect(modelSelect(dialog)).toBeHidden()
    expect(await dialog.getByText('Stale Model').count()).toBe(0)
    await expect(dialog.getByRole('button', { name: 'Save connection' })).toBeDisabled()

    await fillApiKey(dialog)
    await dialog.getByRole('button', { name: 'Cancel' }).click()
    await expect(dialog).toBeHidden()
    await page.getByRole('button', { name: 'Add connection' }).click()
    const reopened = page.getByRole('dialog', { name: 'Add connection' })
    await selectOption(reopened.getByLabel('Kind'), 'model', 'Model API')
    await expect(reopened.getByLabel(/api key/i)).toHaveValue('')
    await expect(modelSelect(reopened)).toBeHidden()
  })

  test('clears key and model after switching organisation', async ({ page }) => {
    await mockSession(page, [org, orgTwo])
    await mockConnections(page)
    const dialog = await openModelForm(page)
    await fillApiKey(dialog)
    await page.getByRole('button', { name: 'Cancel' }).click()
    await expect(dialog).toBeHidden()
    await page.goto(`/org/${orgTwo}/connections`)
    await page.getByRole('button', { name: 'Add connection' }).click()
    const next = page.getByRole('dialog', { name: 'Add connection' })
    await selectOption(next.getByLabel('Kind'), 'model', 'Model API')
    await expect(next.getByLabel(/api key/i)).toHaveValue('')
    await expect(modelSelect(next)).toBeHidden()
  })

  test('surfaces failed authentication, an empty catalog and disabled models without creating a connection', async ({ page }) => {
    await mockSession(page)
    const denied = await mockConnections(page, { catalog: () => ({ status: 401, json: { code: 'unauthorized', message: 'The provider rejected this API key.', request_id: 'req-1', retryable: false } }) })
    let dialog = await openModelForm(page)
    await fillApiKey(dialog)
    await dialog.getByRole('button', { name: 'Test connection' }).click()
    await expect(dialog.getByRole('alert')).toContainText(/rejected|unauthor|invalid/i)
    await expect(modelSelect(dialog)).toBeHidden()
    await expect(dialog.getByRole('button', { name: 'Save connection' })).toBeDisabled()
    expect(denied.create).toEqual([])

    await dialog.getByRole('button', { name: 'Cancel' }).click()
    const empty = await mockConnections(page, { catalog: () => ({ json: { items: [] } }) })
    dialog = await openModelForm(page)
    await fillApiKey(dialog)
    await dialog.getByRole('button', { name: 'Test connection' }).click()
    await expect(dialog.getByRole('alert')).toContainText(/no models/i)
    await expect(dialog.getByRole('button', { name: 'Save connection' })).toBeDisabled()
    expect(empty.create).toEqual([])

    await dialog.getByRole('button', { name: 'Cancel' }).click()
    const disabled = await mockConnections(page, { catalog: () => ({ json: { items: [{ id: 'retired', name: 'Retired model', disabled: true, reason: 'Not available for this key' }] } }) })
    dialog = await openModelForm(page)
    await fillApiKey(dialog)
    await dialog.getByRole('button', { name: 'Test connection' }).click()
    await expect(modelSelect(dialog)).toContainText('Not available for this key')
    await expect(modelSelect(dialog).locator('option', { hasText: 'Retired model' })).toHaveJSProperty('disabled', true)
    await expect(dialog.getByRole('button', { name: 'Save connection' })).toBeDisabled()
    expect(disabled.create).toEqual([])
  })

  test('keeps the normal form keyboard accessible and overflow-free at 390px with advanced collapsed and the forge form intact', async ({ page }) => {
    await mkdir(artifactDir, { recursive: true })
    await page.setViewportSize({ width: 390, height: 844 })
    await mockSession(page)
    await mockConnections(page, { catalog: () => ({ json: { items: [{ id: 'gpt-5', name: 'GPT-5' }] } }) })
    const dialog = await openModelForm(page)
    await expect(dialog.getByLabel('Endpoint')).toBeHidden()
    await expect(dialog.getByLabel('Name', { exact: true })).toBeHidden()
    await expect(dialog.getByLabel('Model ID')).toBeHidden()
    await expect.poll(() => dialog.evaluate(element => element.contains(document.activeElement))).toBe(true)
    const widths = await dialog.evaluate(element => ({ scrollWidth: element.scrollWidth, clientWidth: element.clientWidth }))
    expect(widths.scrollWidth).toBeLessThanOrEqual(widths.clientWidth)
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390)

    await page.screenshot({ path: `${artifactDir}model-form-390.png`, mask: [dialog.locator('input[type="password"]')] })
    await page.evaluate(() => { document.documentElement.dataset.theme = 'dark' })
    await expect(dialog).toBeVisible()
    await page.screenshot({ path: `${artifactDir}model-form-390-dark.png`, mask: [dialog.locator('input[type="password"]')] })
    await page.evaluate(() => { document.documentElement.dataset.theme = 'light' })

    await openAdvanced(dialog)
    await expect(dialog.getByLabel('Endpoint')).toBeVisible()
    await expect(dialog.getByLabel('Name', { exact: true })).toBeVisible()

    await selectOption(dialog.getByLabel('Kind'), 'forge', 'Forge')
    await expect(dialog.getByLabel(/secret/i)).toHaveAttribute('type', 'password')
    await expect(dialog.getByRole('button', { name: 'Create connection' })).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(dialog).toBeHidden()
  })

  for (const item of providerCases) {
    test(`maps ${item.label} to its native profile and endpoint without any provider outbound call`, async ({ page }) => {
      await mockSession(page)
      const captured = await mockConnections(page, { catalog: () => ({ json: { items: [{ id: 'm-1', name: 'Model One' }] } }) })
      const dialog = await openModelForm(page)
      await selectOption(dialog.getByLabel('Provider'), item.id, item.label)
      await expect(dialog.getByLabel('Endpoint')).toBeHidden()
      await fillApiKey(dialog)
      await dialog.getByRole('button', { name: 'Test connection' }).click()
      await expect.poll(() => captured.catalog.length).toBe(1)
      expect(captured.catalog[0]).toMatchObject({ provider: item.provider, endpoint: item.endpoint, settings: { profile: item.profile, auth_kind: 'api_key', billing_route: 'direct_api' } })
      expect(captured.urls.some(url => /api\.openai\.com|api\.anthropic\.com|generativelanguage\.googleapis\.com|opencode\.ai/.test(url))).toBe(false)
    })
  }

  test('saves a manual model under advanced without calling the catalog controller and preserves private-route gating', async ({ page }) => {
    await mockSession(page)
    const captured = await mockConnections(page)
    const dialog = await openModelForm(page)
    await selectOption(dialog.getByLabel('Provider'), 'compatible', 'Custom / compatible')
    await openAdvanced(dialog)
    await dialog.getByLabel('Model ID').fill('local-llama-3')
    await dialog.getByLabel('Endpoint').fill('http://127.0.0.1:11434/v1')
    await dialog.getByRole('button', { name: 'Save connection' }).click()
    await expect.poll(() => captured.create.length).toBe(1)
    expect(captured.create[0]).toMatchObject({ kind: 'model', endpoint: 'http://127.0.0.1:11434/v1', settings: { model: 'local-llama-3', auth_kind: 'api_key', billing_route: 'direct_api' } })
    expect(captured.catalog).toEqual([])

    await expect(dialog).toBeHidden()
    await page.getByRole('button', { name: 'Add connection' }).click()
    const next = page.getByRole('dialog', { name: 'Add connection' })
    await selectOption(next.getByLabel('Kind'), 'model', 'Model API')
    await selectOption(next.getByLabel('Provider'), 'compatible', 'Custom / compatible')
    await openAdvanced(next)
    await next.getByLabel('Model ID').fill('local-llama-3')
    await next.getByLabel('Private route via enrolled runner').check()
    await expect(next.getByRole('button', { name: 'Save connection' })).toBeDisabled()
    expect(captured.create.length).toBe(1)
  })

  test('locks the form while creating and retries verification on the same connection at the latest version', async ({ page }) => {
    await mockSession(page)
    let releaseCreate!: () => void
    const createGate = new Promise<void>(resolve => { releaseCreate = resolve })
    let testCalls = 0
    const captured = await mockConnections(page, {
      catalog: () => ({ json: { items: [{ id: 'gpt-5', name: 'GPT-5' }] } }),
      create: async () => { await createGate; return { status: 201, json: connectionRow({ version: 1, settings: { model: 'gpt-5' } }) } },
      test: () => (++testCalls === 1
        ? { status: 500, json: { code: 'probe_failed', message: 'Probe failed', request_id: 'req-probe', retryable: true } }
        : { json: connectionRow({ version: 5, state: 'healthy' }) }),
      connection: () => ({ json: connectionRow({ version: 5, state: 'unverified' }) }),
    })
    const dialog = await openModelForm(page)
    await fillApiKey(dialog)
    await dialog.getByRole('button', { name: 'Test connection' }).click()
    await expect(modelSelect(dialog)).toBeVisible()
    await pickCatalogModel(dialog, 'gpt-5', 'GPT-5')
    await dialog.getByRole('button', { name: 'Save connection' }).click()

    await expect.poll(() => captured.create.length).toBe(1)
    await expect(dialog.getByRole('button', { name: 'Saving…' })).toBeDisabled()
    await expect(dialog.getByLabel('Kind')).toBeDisabled()
    await expect(dialog.getByLabel('Provider')).toBeDisabled()
    await expect(dialog.getByLabel(/api key/i)).toBeDisabled()
    await expect(dialog.getByRole('button', { name: 'Cancel' })).toBeDisabled()

    releaseCreate()
    await expect(dialog.getByText(/verification unavailable/i)).toBeVisible()
    await expect(dialog.getByRole('alert')).toContainText(/probe failed/i)
    expect(captured.create.length).toBe(1)
    expect(captured.test).toEqual([{ id: 'conn-model-1', ifMatch: '"1"' }])

    await dialog.getByRole('button', { name: 'Retry verification' }).click()
    await expect.poll(() => captured.test.length).toBe(2)
    expect(captured.get).toContain('conn-model-1')
    expect(captured.test[1]).toEqual({ id: 'conn-model-1', ifMatch: '"5"' })
    expect(captured.create.length).toBe(1)
    await expect(dialog).toBeHidden()
  })

  test('clears manual, CA and private-route state on provider change and refuses a disabled manual model', async ({ page }) => {
    await mockSession(page)
    const captured = await mockConnections(page, {
      catalog: () => ({ json: { items: [{ id: 'gpt-5', name: 'GPT-5' }, { id: 'retired', name: 'Retired model', disabled: true, reason: 'Not available for this key' }] } }),
    })
    let dialog = await openModelForm(page)
    await selectOption(dialog.getByLabel('Provider'), 'compatible', 'Custom / compatible')
    await openAdvanced(dialog)
    await dialog.getByLabel('Model ID').fill('local-llama-3')
    await dialog.getByLabel('CA certificate').fill('-----BEGIN CERTIFICATE-----')
    await dialog.getByLabel('Private route via enrolled runner').check()
    await expect(dialog.getByLabel('Model ID')).toHaveValue('local-llama-3')
    await expect(dialog.getByLabel('CA certificate')).toHaveValue('-----BEGIN CERTIFICATE-----')
    await expect(dialog.getByLabel('Private route via enrolled runner')).toBeChecked()

    await selectOption(dialog.getByLabel('Provider'), 'openai', 'OpenAI')
    await expect(dialog.getByLabel('Model ID')).toHaveValue('')
    await expect(dialog.getByLabel('CA certificate')).toHaveValue('')
    await expect(dialog.getByLabel('Private route via enrolled runner')).not.toBeChecked()
    await expect(dialog.getByRole('button', { name: 'Save connection' })).toBeDisabled()

    await dialog.getByRole('button', { name: 'Cancel' }).click()
    dialog = await openModelForm(page)
    await fillApiKey(dialog)
    await dialog.getByRole('button', { name: 'Test connection' }).click()
    await expect(modelSelect(dialog)).toContainText('Not available for this key')
    await openAdvanced(dialog)
    await dialog.getByLabel('Model ID').fill('retired')
    await expect(dialog.getByRole('alert')).toContainText(/unavailable for this provider/i)
    await expect(dialog.getByRole('button', { name: 'Save connection' })).toBeDisabled()
    expect(captured.create).toEqual([])
  })

  test('reports a terminal uncertain save when the create response is lost without a second POST', async ({ page }) => {
    await mockSession(page)
    const captured = await mockConnections(page, {
      catalog: () => ({ json: { items: [{ id: 'gpt-5', name: 'GPT-5' }] } }),
      create: () => ({ abort: true }),
    })
    const dialog = await openModelForm(page)
    await fillApiKey(dialog)
    await dialog.getByRole('button', { name: 'Test connection' }).click()
    await expect(modelSelect(dialog)).toBeVisible()
    await pickCatalogModel(dialog, 'gpt-5', 'GPT-5')
    await dialog.getByRole('button', { name: 'Save connection' }).click()

    await expect(dialog.getByRole('status')).toContainText(/save outcome unknown/i)
    await expect(dialog.getByRole('button', { name: 'View connections' })).toBeVisible()
    await expect(dialog.getByRole('button', { name: /retry save|refresh list/i })).toHaveCount(0)
    await expect(dialog.getByRole('button', { name: 'Save connection' })).toHaveCount(0)
    expect(captured.create.length).toBe(1)

    await dialog.getByRole('button', { name: 'View connections' }).click()
    await expect(dialog).toBeHidden()
    await page.waitForTimeout(150)
    expect(captured.create.length).toBe(1)
  })

  test('does not let a create that resolves after an organisation switch touch the new dialog', async ({ page }) => {
    await mockSession(page, [org, orgTwo])
    let releaseCreate!: () => void
    const createGate = new Promise<void>(resolve => { releaseCreate = resolve })
    const captured = await mockConnections(page, {
      catalog: () => ({ json: { items: [{ id: 'gpt-5', name: 'GPT-5' }] } }),
      create: async () => { await createGate; return { status: 201, json: connectionRow({ version: 1 }) } },
    })
    await page.goto(`/org/${org}/overview`)
    await page.getByRole('link', { name: 'Connections' }).click()
    const dialog = page.getByRole('dialog', { name: 'Add connection' })
    await page.getByRole('button', { name: 'Add connection' }).click()
    await selectOption(dialog.getByLabel('Kind'), 'model', 'Model API')
    await fillApiKey(dialog)
    await dialog.getByRole('button', { name: 'Test connection' }).click()
    await expect(modelSelect(dialog)).toBeVisible()
    await pickCatalogModel(dialog, 'gpt-5', 'GPT-5')
    await dialog.getByRole('button', { name: 'Save connection' }).click()
    await expect.poll(() => captured.create.length).toBe(1)

    await page.goBack()
    await expect(page).toHaveURL(new RegExp(`/org/${org}/overview`))
    await page.getByRole('button', { name: 'Switch organisation' }).click()
    await page.getByRole('menuitem', { name: /Fixture 1/ }).click()
    await expect(page).toHaveURL(new RegExp(`/org/${orgTwo}/overview`))
    await page.getByRole('link', { name: 'Connections' }).click()
    await page.getByRole('button', { name: 'Add connection' }).click()
    const next = page.getByRole('dialog', { name: 'Add connection' })
    await selectOption(next.getByLabel('Kind'), 'model', 'Model API')
    await next.getByLabel(/api key/i).fill('sk-second-org-not-a-real-key')

    releaseCreate()
    await page.waitForTimeout(250)
    await expect(next).toBeVisible()
    await expect(next.getByLabel(/api key/i)).toHaveValue('sk-second-org-not-a-real-key')
    expect(captured.create.length).toBe(1)
    expect(captured.test).toEqual([])
  })
})
