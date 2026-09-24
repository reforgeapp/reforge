import { readFileSync } from 'node:fs'
import { test, expect, type Locator, type Page } from '@playwright/test'

const live = process.env.REFORGE_MODEL_LIVE === '1'
const env = (name: string) => process.env[name] ?? ''
const key = env('REFORGE_MODEL_LIVE_KEY')
const badKey = env('REFORGE_MODEL_LIVE_BAD_KEY')
const artifacts = env('REFORGE_MODEL_LIVE_ARTIFACTS')
const endpoint = 'https://93.184.216.34:8443/v1'

test.skip(!live, 'run via scripts/acceptance/model-connections/run.sh')
test.describe.configure({ mode: 'serial' })
test.use({ trace: 'off' })

type Hit = { method: string; path: string; auth: string }
const hits = (): string[] => readFileSync(env('REFORGE_MODEL_LIVE_FIXTURE_LOG'), 'utf8').split('\n').filter(Boolean)
  .map(line => JSON.parse(line) as Hit).map(hit => `${hit.method} ${hit.path} ${hit.auth}`)

async function signIn(page: Page) {
  await page.goto('/auth/login')
  await expect(page).toHaveURL(/\/org\/[^/]+\//)
  return new URL(page.url()).pathname.split('/')[2]
}

async function openForm(page: Page, org: string) {
  await page.goto(`/org/${org}/connections`)
  await page.getByRole('button', { name: 'Models & agents' }).click()
  await page.getByRole('button', { name: 'Add connection' }).click()
  const dialog = page.getByRole('dialog', { name: 'Add connection' })
  await dialog.getByLabel('Provider').selectOption('compatible')
  await dialog.getByText('Advanced', { exact: true }).click()
  await dialog.getByLabel('Profile').selectOption('responses')
  await dialog.getByLabel('CA certificate').fill(readFileSync(env('REFORGE_MODEL_LIVE_CA'), 'utf8'))
  return dialog
}

async function catalog(page: Page, dialog: Locator, url: string, secret: string) {
  await dialog.getByLabel('Endpoint', { exact: true }).fill(url)
  await dialog.getByLabel(/api key/i).fill(secret)
  const response = page.waitForResponse(res => res.url().endsWith('/connections/model-catalog'))
  await dialog.getByRole('button', { name: 'Test connection' }).click()
  const res = await response
  return { status: res.status(), body: await res.text() }
}

test('creates, verifies and reloads a model connection through the real backend and fixture provider', async ({ page }) => {
  const org = await signIn(page)
  const before = hits().length
  const dialog = await openForm(page, org)
  const result = await catalog(page, dialog, endpoint, key)
  expect(result.status).toBe(200)
  expect(JSON.parse(result.body)).toEqual({ items: [{ id: 'fixture-alpha', name: 'fixture-alpha' }, { id: 'fixture-beta', name: 'fixture-beta' }] })
  await expect(dialog.getByText('Models loaded.')).toBeVisible()
  await dialog.getByRole('combobox', { name: 'Model', exact: true }).selectOption('fixture-beta')
  await dialog.getByRole('button', { name: 'Save connection' }).click()
  await expect(dialog).toBeHidden()
  expect(hits().slice(before)).toEqual(['GET /v1/models valid', 'GET /v1/models/fixture-beta valid'])

  await page.reload()
  await page.getByRole('button', { name: 'Models & agents' }).click()
  await expect(page.getByRole('row', { name: /fixture-beta/ })).toBeVisible()
  const listed = await page.request.get(`/api/v1/orgs/${org}/connections?kind=model`)
  const text = await listed.text()
  expect(text).not.toContain(key)
  const row = (JSON.parse(text).items as Array<Record<string, any>>).find(item => item.settings?.model === 'fixture-beta')
  expect(row).toMatchObject({ provider: 'compatible', endpoint, state: 'healthy', settings: { profile: 'responses', auth_kind: 'api_key', billing_route: 'direct_api' } })
  await expect(page.locator('body')).not.toContainText(key)
})

test('rejects an invalid key without echoing it', async ({ page }) => {
  const org = await signIn(page)
  const before = hits().length
  const dialog = await openForm(page, org)
  const result = await catalog(page, dialog, endpoint, badKey)
  expect(result.status).toBe(502)
  expect(result.body).not.toContain(badKey)
  expect(result.body).not.toContain('invalid key')
  await expect(dialog.getByRole('alert')).toBeVisible()
  await expect(page.locator('body')).not.toContainText(badKey)
  await expect(dialog.getByRole('button', { name: 'Save connection' })).toBeDisabled()
  expect(hits().slice(before)).toEqual(['GET /v1/models invalid'])
})

test('blocks private and metadata catalog destinations before any provider traffic', async ({ page }) => {
  const org = await signIn(page)
  const before = hits().length
  const dialog = await openForm(page, org)
  for (const url of ['https://127.0.0.1:8443/v1', 'https://10.0.0.8:8443/v1', 'https://169.254.169.254/v1']) {
    const result = await catalog(page, dialog, url, key)
    expect(result.status, url).toBe(400)
    await expect(dialog.getByRole('alert')).toBeVisible()
    await expect(dialog.getByRole('button', { name: 'Save connection' })).toBeDisabled()
  }
  expect(hits().slice(before)).toEqual([])
})

test('captures loaded catalog and saved row in light and dark at 390 and desktop', async ({ page }) => {
  test.setTimeout(120_000)
  const org = await signIn(page)
  for (const theme of ['light', 'dark']) {
    await page.evaluate(value => localStorage.setItem('reforge-theme', value), theme)
    for (const [label, width, height] of [['390', 390, 844], ['desktop', 1280, 800]] as const) {
      await page.setViewportSize({ width, height })
      await page.goto(`/org/${org}/connections`)
      await page.getByRole('button', { name: 'Models & agents' }).click()
      await expect(page.getByRole('row', { name: /fixture-beta/ })).toBeVisible()
      await page.screenshot({ path: `${artifacts}/row-${theme}-${label}.png`, fullPage: true })
      const dialog = await openForm(page, org)
      expect((await catalog(page, dialog, endpoint, key)).status).toBe(200)
      await dialog.getByRole('combobox', { name: 'Model', exact: true }).selectOption('fixture-alpha')
      await page.screenshot({ path: `${artifacts}/catalog-${theme}-${label}.png` })
      await dialog.getByRole('button', { name: 'Cancel' }).click()
    }
  }
})
