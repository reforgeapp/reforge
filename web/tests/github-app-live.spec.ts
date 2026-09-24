import { existsSync, readFileSync, writeFileSync } from 'node:fs'
import { test, expect, type Locator, type Page } from '@playwright/test'

const live = process.env.REFORGE_GITHUB_LIVE === '1'
const env = (name: string) => process.env[name] ?? ''
const pat = env('REFORGE_GITHUB_LIVE_PAT')
const badPat = env('REFORGE_GITHUB_LIVE_BAD_PAT')
const ssoPat = env('REFORGE_GITHUB_LIVE_SSO_PAT')
const control = env('REFORGE_GITHUB_LIVE_CONTROL')
const artifacts = env('REFORGE_GITHUB_LIVE_ARTIFACTS')

test.skip(!live, 'run via scripts/acceptance/github-app-onboarding/run.sh')
test.describe.configure({ mode: 'serial' })
test.use({
  trace: 'off',
  screenshot: 'off',
  video: 'off',
  ignoreHTTPSErrors: false,
  launchOptions: {
    ...(process.env.PLAYWRIGHT_CHROMIUM_PATH ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_PATH } : {}),
    args: [`--ignore-certificate-errors-spki-list=${env('REFORGE_GITHUB_LIVE_SPKI')}`],
  },
})

type Hit = { host?: string; method?: string; path?: string; auth?: string; status?: number; event?: string; [key: string]: unknown }
const hits = (): Hit[] => readFileSync(env('REFORGE_GITHUB_LIVE_FIXTURE_LOG'), 'utf8').split('\n').filter(Boolean).map(line => JSON.parse(line) as Hit)
const secrets = () => [pat, badPat, ssoPat, ...(existsSync(`${control}/secrets.txt`) ? readFileSync(`${control}/secrets.txt`, 'utf8').split('\n').filter(Boolean) : [])]

let org = ''
let manifestConnection = ''
let oauthReplay = ''
const apiBodies: string[] = []

async function signIn(page: Page) {
  page.on('response', async response => {
    if (!new URL(response.url()).pathname.startsWith('/api/')) return
    try { apiBodies.push(await response.text()) } catch { /* redirects and aborted bodies */ }
  })
  await page.goto('/auth/login')
  await expect(page).toHaveURL(/\/org\/[^/]+\//)
  org = new URL(page.url()).pathname.split('/')[2]
}

async function forgeConnections(page: Page) {
  const res = await page.request.get(`/api/v1/orgs/${org}/connections?kind=forge&limit=100`)
  expect(res.status()).toBe(200)
  return (await res.json()).items as Array<Record<string, any>>
}

async function repositories(page: Page) {
  const res = await page.request.get(`/api/v1/orgs/${org}/repositories?limit=100`)
  expect(res.status()).toBe(200)
  return ((await res.json()).items as Array<{ name: string }>).map(item => item.name).sort()
}

async function openForge(page: Page, auth?: string) {
  await page.goto(`/org/${org}/connections`)
  await page.getByRole('button', { name: 'Add connection' }).click()
  const dialog = page.getByRole('dialog', { name: 'Add connection' })
  await expect(dialog.getByLabel('Provider')).toHaveValue('github')
  if (auth) await dialog.getByLabel('Authentication').selectOption({ label: auth })
  return dialog
}

async function createToken(page: Page, name: string, token: string) {
  const dialog = await openForge(page, 'Personal access token')
  await dialog.getByLabel('Name', { exact: true }).fill(name)
  await dialog.getByLabel('Personal access token', { exact: true }).and(dialog.locator('input')).fill(token)
  await dialog.getByRole('button', { name: /^Create connection/ }).click()
  return dialog
}

async function importDialog(page: Page) {
  const dialog = page.getByRole('dialog', { name: /inventory|repositories/i })
  await expect(dialog).toBeVisible({ timeout: 20_000 })
  const start = dialog.getByRole('button', { name: 'Start preview' })
  if (await start.isVisible()) await start.click()
  return dialog
}

async function importChoosing(page: Page, dialog: Locator, expected: string[], skip: string) {
  for (const name of expected) await expect(dialog.getByRole('checkbox', { name: new RegExp(name) })).toBeVisible({ timeout: 30_000 })
  const box = dialog.getByRole('checkbox', { name: new RegExp(skip) })
  await box.focus()
  await page.keyboard.press('Space')
  await expect(box).not.toBeChecked()
  await dialog.getByRole('button', { name: /^Import selected/ }).click()
  await expect(dialog.getByRole('status').filter({ hasText: /Import complete/i })).toBeVisible({ timeout: 30_000 })
}

async function guidedStart(page: Page, appName: string, githubOrg = '') {
  const dialog = await openForge(page)
  await expect(dialog.getByLabel('Authentication')).toHaveValue('guided')
  await dialog.getByLabel('App name').fill(appName)
  if (githubOrg) await dialog.getByLabel('GitHub organisation').fill(githubOrg)
  await dialog.getByRole('button', { name: 'Set up GitHub App' }).click()
  await expect(page).toHaveURL(/\/auth\/github\/setup\//)
  await page.getByRole('button', { name: 'Continue to GitHub' }).click()
  await expect(page).toHaveURL(/^https:\/\/github\.com\/(organizations\/[^/]+\/)?settings\/apps\/new/)
  await page.getByRole('button', { name: 'Create GitHub App' }).click()
  await expect(page).toHaveURL(/^https:\/\/github\.com\/apps\/[a-z0-9-]+\/installations\/new\?state=/)
}

async function authorize(page: Page) {
  await expect(page).toHaveURL(/^https:\/\/github\.com\/login\/oauth\/authorize\?/)
  const authorizeURL = new URL(page.url())
  expect(authorizeURL.searchParams.get('code_challenge_method')).toBe('S256')
  expect(authorizeURL.searchParams.get('redirect_uri')).toMatch(/\/auth\/github\/oauth\/callback$/)
  const callback = page.waitForRequest(request => request.url().includes('/auth/github/oauth/callback'))
  await page.getByRole('button', { name: 'Authorize' }).click()
  return (await callback).url()
}

function assertNoSecrets(text: string, where: string) {
  for (const secret of secrets()) expect(text.includes(secret), `${where} leaked a secret`).toBe(false)
}

async function waitForFile(path: string, timeout: number) {
  const start = Date.now()
  while (Date.now() - start < timeout) {
    if (existsSync(path)) return
    await new Promise(resolve => setTimeout(resolve, 250))
  }
  throw new Error(`timed out after ${timeout}ms waiting for ${path}`)
}

async function assertBrowserClean(page: Page) {
  const storage = await page.evaluate(async () => {
    const dbs = 'databases' in indexedDB ? await indexedDB.databases() : []
    return JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage }, dbs })
  })
  assertNoSecrets(storage, 'browser storage')
  assertNoSecrets(JSON.stringify(await page.context().cookies()), 'cookies')
  assertNoSecrets(await page.locator('body').innerText(), 'page text')
  assertNoSecrets(apiBodies.join('\n'), 'API responses')
}

test('token: wrong token and SSO retry reuse one connection, then preview, select, import and reload', async ({ page }) => {
  test.setTimeout(120_000)
  await signIn(page)
  const before = hits().length

  const wrong = await createToken(page, 'Wrong token GitHub', badPat)
  await expect(wrong.getByText('Connection saved; capability test failed.')).toBeVisible()
  await expect(wrong.getByRole('alert')).toBeVisible()
  await expect(wrong.getByRole('button', { name: /Continue to repositories/i })).toHaveCount(0)
  await wrong.getByRole('button', { name: 'Retry test' }).click()
  await expect(wrong.getByRole('button', { name: 'Retry test' })).toBeEnabled()
  await expect(wrong.getByRole('alert')).toBeVisible()
  await wrong.getByRole('button', { name: 'Close', exact: true }).click()
  let rows = await forgeConnections(page)
  expect(rows.filter(row => row.name === 'Wrong token GitHub')).toHaveLength(1)
  expect(rows.find(row => row.name === 'Wrong token GitHub')?.state).not.toBe('healthy')

  const sso = await createToken(page, 'Token GitHub', ssoPat)
  await expect(sso.getByText('Connection saved; capability test failed.')).toBeVisible()
  await page.screenshot({ path: `${artifacts}/token-test-failed-desktop.png` })
  writeFileSync(`${control}/sso-authorized`, '')
  await sso.getByRole('button', { name: 'Retry test' }).click()
  rows = await forgeConnections(page)
  expect(rows.filter(row => row.name === 'Token GitHub')).toHaveLength(1)

  const dialog = await importDialog(page)
  await importChoosing(page, dialog, ['fixture-owner/token-alpha', 'fixture-owner/token-beta', 'fixture-owner/token-archive'], 'fixture-owner/token-beta')
  expect(await repositories(page)).toEqual(['fixture-owner/token-alpha', 'fixture-owner/token-archive'])

  await page.reload()
  await page.goto(`/org/${org}/repositories`)
  await expect(page.getByText('fixture-owner/token-alpha')).toBeVisible()
  await expect(page.getByText('fixture-owner/token-beta')).toHaveCount(0)
  rows = await forgeConnections(page)
  expect(rows.find(row => row.name === 'Token GitHub')).toMatchObject({ state: 'healthy', endpoint: 'https://api.github.com' })
  expect(rows.filter(row => row.name === 'Token GitHub')).toHaveLength(1)

  const traffic = hits().slice(before).filter(hit => hit.path)
  expect(traffic.filter(hit => hit.auth === 'bad').map(hit => `${hit.path} ${hit.status}`)).toEqual(['/user 401', '/user 401'])
  expect(traffic.some(hit => hit.auth === 'sso' && hit.status === 403)).toBe(true)
  expect(traffic.some(hit => hit.path === '/user/repos' && hit.auth === 'pat' && hit.status === 200)).toBe(true)
  expect(traffic.every(hit => hit.host === 'api.github.com')).toBe(true)
  await assertBrowserClean(page)
})

test('guided manifest: create, install, OAuth PKCE, persisted connection and selected repository import', async ({ page }) => {
  test.setTimeout(120_000)
  await signIn(page)
  const before = hits().length
  const connectionsBefore = (await forgeConnections(page)).length
  await guidedStart(page, 'Reforge Fixture')
  await page.getByRole('button', { name: 'Install' }).click()
  oauthReplay = await authorize(page)
  await expect(page).toHaveURL(new RegExp(`/org/${org}/connections\\?`))
  const final = new URL(page.url())
  expect(page.url()).not.toContain('code=')

  const dialog = await importDialog(page)
  await importChoosing(page, dialog, ['fixture-owner/app-alpha', 'fixture-owner/app-beta', 'fixture-owner/app-gamma'], 'fixture-owner/app-gamma')
  const repos = await repositories(page)
  expect(repos).toEqual(expect.arrayContaining(['fixture-owner/app-alpha', 'fixture-owner/app-beta']))
  expect(repos).not.toContain('fixture-owner/app-gamma')

  await page.reload()
  const rows = await forgeConnections(page)
  expect(rows).toHaveLength(connectionsBefore + 1)
  const managed = rows.find(row => row.name === 'Reforge Fixture')
  expect(managed, `final URL ${final.pathname}${final.search}`).toBeTruthy()
  manifestConnection = managed!.id
  expect(managed).toMatchObject({ provider: 'github', endpoint: 'https://api.github.com', settings: expect.objectContaining({ auth_kind: 'github_app', managed: 'github_manifest', namespace: 'installation' }) })
  expect(managed!.reason ?? '').toMatch(/webhook/i)

  const traffic = hits().slice(before)
  const manifest = traffic.find(hit => hit.event === 'manifest')!
  expect(manifest).toMatchObject({ owner: 'fixture-owner', hook_active: false, request_oauth_on_install: false, public: false, setup_on_update: true })
  expect(Object.entries(manifest.permissions as Record<string, string>).filter(([key, value]) => value === 'admin' || (key === 'administration' && value !== 'read'))).toEqual([])
  expect(traffic.filter(hit => hit.event === 'token_exchange')).toEqual([{ event: 'token_exchange', ok: true, pkce: true }])
  const api = traffic.filter(hit => hit.host === 'api.github.com').map(hit => `${hit.method} ${hit.path} ${hit.auth} ${hit.status}`)
  for (const call of ['GET /user user 200', 'GET /user/installations user 200', 'POST /app-manifests/:code/conversions none 201', '/access_tokens jwt 201', 'GET /installation/repositories installation 200'])
    expect(api.some(line => line.includes(call)), call).toBe(true)
  await assertBrowserClean(page)
})

test('guided replay of a consumed OAuth callback does not create another connection', async ({ page }) => {
  await signIn(page)
  const count = (await forgeConnections(page)).length
  await page.goto(oauthReplay)
  await expect(page).toHaveURL(/github_result=(expired|failed)|\/connections(\?|$)/)
  expect(page.url()).not.toContain('github_result=connected')
  expect(await forgeConnections(page)).toHaveLength(count)
})

test('guided negative: spoofed installation ID from another App fails without a connection', async ({ page }) => {
  test.setTimeout(90_000)
  await signIn(page)
  const count = (await forgeConnections(page)).length
  const other = (await forgeConnections(page)).find(row => row.id === manifestConnection)?.settings?.installation_id
  expect(other).toBeTruthy()
  await guidedStart(page, 'Spoof Fixture')
  const form = page.locator('form')
  const setupURL = new URL(await form.getAttribute('action') ?? '')
  setupURL.searchParams.set('installation_id', String(other))
  setupURL.searchParams.set('setup_action', 'install')
  setupURL.searchParams.set('state', await form.locator('input[name=state]').inputValue())
  await page.goto(setupURL.toString())
  await authorize(page)
  await expect(page).toHaveURL(/github_result=failed/)
  expect(new URL(page.url()).searchParams.get('github_reason') ?? '').toMatch(/installation_unavailable/)
  await expect(page.getByRole('status').filter({ hasText: /GitHub/ })).toBeVisible()
  await page.screenshot({ path: `${artifacts}/blocked-spoofed-installation-desktop.png` })
  expect(await forgeConnections(page)).toHaveLength(count)
})

test('guided negative: organisation member without admin role is refused', async ({ page }) => {
  test.setTimeout(90_000)
  await signIn(page)
  const count = (await forgeConnections(page)).length
  await guidedStart(page, 'Member Fixture', 'member-org')
  await page.getByRole('button', { name: 'Install' }).click()
  await authorize(page)
  await expect(page).toHaveURL(/github_result=failed/)
  expect(new URL(page.url()).searchParams.get('github_reason') ?? '').toMatch(/not_owner/)
  expect(await forgeConnections(page)).toHaveLength(count)
  expect(hits().some(hit => hit.path === '/user/memberships/orgs/member-org' && hit.status === 200)).toBe(true)
})

test('keyboard: add connection dialog opens, closes with Escape and restores focus', async ({ page }) => {
  await signIn(page)
  await page.goto(`/org/${org}/connections`)
  const add = page.getByRole('button', { name: 'Add connection' })
  await add.focus()
  await page.keyboard.press('Enter')
  const dialog = page.getByRole('dialog', { name: 'Add connection' })
  await expect(dialog).toBeVisible()
  await page.keyboard.press('Tab')
  await expect(dialog.locator(':focus')).toHaveCount(1)
  await page.keyboard.press('Escape')
  await expect(dialog).toBeHidden()
  await expect(add).toBeFocused()
})

test('recovery: failed token verification is repaired inline on the same connection and only selected repositories import', async ({ page }) => {
  test.setTimeout(120_000)
  await signIn(page)
  const dialog = await openForge(page, 'Personal access token')
  await dialog.getByLabel('Name', { exact: true }).fill('Recovered token GitHub')
  await dialog.getByLabel('Personal access token', { exact: true }).fill(badPat)
  await dialog.getByRole('button', { name: /^Create connection/ }).click()
  await expect(dialog.getByText('Connection saved; capability test failed.')).toBeVisible()
  await expect(dialog.getByRole('button', { name: /Continue to repositories/i })).toHaveCount(0)
  const created = (await forgeConnections(page)).find(row => row.name === 'Recovered token GitHub')
  expect(created).toBeTruthy()
  const connectionID = created!.id

  await dialog.getByLabel('Personal access token', { exact: true }).fill(pat)
  await dialog.getByRole('button', { name: 'Update credentials' }).click()
  const picker = await importDialog(page)
  await importChoosing(page, picker, ['fixture-owner/token-alpha', 'fixture-owner/token-beta', 'fixture-owner/token-archive'], 'fixture-owner/token-beta')

  const rows = await forgeConnections(page)
  const recovered = rows.filter(row => row.name === 'Recovered token GitHub')
  expect(recovered).toHaveLength(1)
  expect(recovered[0].id).toBe(connectionID)
  expect(recovered[0].state).toBe('healthy')
  const repos = await repositories(page)
  expect(repos).toContain('fixture-owner/token-alpha')
  expect(repos).toContain('fixture-owner/token-archive')
  expect(repos).not.toContain('fixture-owner/token-beta')
})

test('recovery: a real server restart during pending App setup resumes install and OAuth', async ({ page }) => {
  test.setTimeout(180_000)
  await signIn(page)
  const before = (await forgeConnections(page)).length
  await guidedStart(page, 'Restart Fixture')
  writeFileSync(`${control}/restart-requested`, '')
  await waitForFile(`${control}/restart-done`, 45_000)
  await page.getByRole('button', { name: 'Install' }).click()
  await authorize(page)
  await expect(page).toHaveURL(new RegExp(`/org/${org}/connections\\?`))
  expect(page.url()).not.toContain('code=')
  const dialog = await importDialog(page)
  await importChoosing(page, dialog, ['fixture-owner/app-alpha', 'fixture-owner/app-beta', 'fixture-owner/app-gamma'], 'fixture-owner/app-gamma')
  await page.reload()
  const rows = await forgeConnections(page)
  expect(rows).toHaveLength(before + 1)
  const managed = rows.find(row => row.name === 'Restart Fixture')
  expect(managed, `final URL ${page.url()}`).toBeTruthy()
  expect(managed).toMatchObject({ provider: 'github', state: 'healthy', settings: expect.objectContaining({ auth_kind: 'github_app', managed: 'github_manifest', namespace: 'installation' }) })
})

test('captures desktop 1440 and 390 in light and dark', async ({ page }) => {
  test.setTimeout(240_000)
  await signIn(page)
  for (const theme of ['light', 'dark']) {
    await page.evaluate(value => localStorage.setItem('reforge-theme', value), theme)
    for (const [label, width, height] of [['1440', 1440, 900], ['390', 390, 844]] as const) {
      await page.setViewportSize({ width, height })
      await page.goto(`/org/${org}/connections`)
      await expect(page.getByText('Reforge Fixture').first()).toBeVisible()
      await page.screenshot({ path: `${artifacts}/list-${label}-${theme}.png`, fullPage: true })

      const guided = await openForge(page)
      await expect(guided.getByRole('button', { name: 'Set up GitHub App' })).toBeVisible()
      await page.screenshot({ path: `${artifacts}/guided-${label}-${theme}.png` })
      await guided.getByLabel('Authentication').selectOption({ label: 'Personal access token' })
      await page.screenshot({ path: `${artifacts}/token-${label}-${theme}.png` })
      await page.keyboard.press('Escape')
      await expect(guided).toBeHidden()

      await page.getByRole('button', { name: 'Token GitHub', exact: true }).first().click()
      await expect(page.getByRole('heading', { name: 'Token GitHub' })).toBeVisible()
      await page.screenshot({ path: `${artifacts}/forge-detail-${label}-${theme}.png` })
      await page.getByRole('button', { name: 'Add repositories' }).click()
      const picker = page.getByRole('dialog', { name: /inventory|repositories/i })
      await expect(picker.getByRole('checkbox', { name: /fixture-owner\/token-alpha/ })).toBeVisible({ timeout: 30_000 })
      await page.screenshot({ path: `${artifacts}/repository-picker-${label}-${theme}.png` })
      await page.keyboard.press('Escape')
      await expect(picker).toBeHidden()
    }
  }
  await assertBrowserClean(page)
})
