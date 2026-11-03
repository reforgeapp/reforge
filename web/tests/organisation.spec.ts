import { test, expect, type Page } from '@playwright/test'
import type { OrgOIDCSettings } from '../src/organisation-api'
import { mkdir } from 'node:fs/promises'
import { resolve } from 'node:path'

const org = '00000000-0000-4000-8000-000000000001'
const user = '00000000-0000-4000-8000-0000000000aa'
const team = '00000000-0000-4000-8000-0000000000bb'

async function mock(page: Page, role: string) {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }], memberships: [{ org_id: org, role, team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { name: 'Reforge', version: 'test', edition: 'self-hosted', development: true, fixture_auth: true } }))
  await page.route(`**/api/v1/orgs/${org}/memberships**`, route => route.fulfill({ json: { items: [{ org_id: org, user_id: user, role: 'viewer', all_repositories: false, team_ids: [], repository_ids: [], version: 2 }], complete: true } }))
  await page.route(`**/api/v1/orgs/${org}/identity/oidc/invitations**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route(`**/api/v1/orgs/${org}/teams**`, route => route.fulfill({ json: { items: [{ id: team, name: 'Platform', repository_ids: [], version: 1 }], complete: true } }))
  await page.route(`**/api/v1/orgs/${org}/repositories**`, route => route.fulfill({ json: { items: [{ id: '00000000-0000-4000-8000-000000000022', name: 'payments', provider: 'github', default_branch: 'main', archived: false, paused: false, accessible: true, team_ids: [], version: 1, connection_id: '00000000-0000-4000-8000-000000000033', native_id: '1', url: 'https://github.example/payments', org_id: org, last_synced_at: null }], complete: true } }))
  await page.goto(`/org/${org}/organisation`)
}

test('owner edits a team through a version-checked save', async ({ page }) => {
  let saved: { name: string; ifMatch: string; csrf: string } | undefined
  await mock(page, 'owner')
  await expect(page.getByRole('heading', { name: 'Platform', exact: true })).toBeVisible()
  await page.route(`**/api/v1/orgs/${org}/teams/${team}`, async route => {
    if (route.request().method() !== 'PUT') return route.fallback()
    const body = route.request().postDataJSON() as { name: string }
    saved = { name: body.name, ifMatch: route.request().headers()['if-match'] ?? '', csrf: route.request().headers()['x-csrf-token'] ?? '' }
    return route.fulfill({ json: { id: team, name: body.name, repository_ids: [], version: 2 } })
  })
  await page.getByLabel(`Team name ${team}`).fill('Platform Services')
  await page.getByRole('button', { name: 'Save changes' }).click()
  await expect.poll(() => saved?.name).toBe('Platform Services')
  expect(saved?.ifMatch).toBe('"1"')
  expect(saved?.csrf).toBe('csrf-1')
})

test('owner changes member role and repository scope in one version-checked save', async ({ page }) => {
  let saved: { user: string; role: string; teamIDs: string[]; repositories: string[]; ifMatch: string } | undefined
  await mock(page, 'owner')
  await page.getByRole('tab', { name: 'Members' }).click()
  await expect(page.getByRole('heading', { name: /Member/ })).toBeVisible()
  await page.route(`**/api/v1/orgs/${org}/memberships/${user}`, async route => {
    if (route.request().method() !== 'PUT') return route.fallback()
    const body = route.request().postDataJSON() as { role: string; team_ids: string[]; repository_ids: string[] }
    saved = { user, role: body.role, teamIDs: body.team_ids, repositories: body.repository_ids, ifMatch: route.request().headers()['if-match'] ?? '' }
    return route.fulfill({ json: { org_id: org, user_id: user, role: body.role, all_repositories: false, team_ids: body.team_ids, repository_ids: body.repository_ids, version: 3 } })
  })
  await page.getByLabel(`Role for ${user}`).selectOption('maintainer')
  await page.getByLabel('payments').check()
  await page.getByRole('button', { name: 'Save changes' }).click()
  await expect.poll(() => saved?.role).toBe('maintainer')
  expect(saved?.repositories).toContain('00000000-0000-4000-8000-000000000022')
  expect(saved?.teamIDs).toEqual([])
  expect(saved?.ifMatch).toBe('"2"')
})

test('all-repository access preserves saved scoped grants for later rollback', async ({ page }) => {
  const repoID = '00000000-0000-4000-8000-000000000022'
  let member = { org_id: org, user_id: user, role: 'viewer', all_repositories: false, team_ids: [team], repository_ids: [repoID], version: 2 }
  let saved: { all: boolean; teamIDs: string[]; repositoryIDs: string[] } | undefined
  await mock(page, 'owner')
  await page.route(`**/api/v1/orgs/${org}/memberships**`, async route => {
    if (route.request().method() === 'PUT') {
      const body = route.request().postDataJSON() as { role: string; all_repositories: boolean; team_ids: string[]; repository_ids: string[] }
      saved = { all: body.all_repositories, teamIDs: body.team_ids, repositoryIDs: body.repository_ids }
      member = { ...member, ...body, version: member.version + 1 }
      return route.fulfill({ json: member })
    }
    return route.fulfill({ json: { items: [member], complete: true } })
  })
  await page.getByRole('tab', { name: 'Members' }).click()
  await expect(page.getByLabel('Platform')).toBeChecked()
  await page.getByLabel('All repositories').check()
  await page.getByRole('button', { name: 'Save changes' }).click()
  await expect.poll(() => saved?.all).toBe(true)
  expect(saved?.teamIDs).toEqual([team])
  expect(saved?.repositoryIDs).toEqual([repoID])
  await page.getByLabel('Selected scope').check()
  await expect(page.getByLabel(`Role for ${user}`)).toHaveValue('viewer')
  await expect(page.getByLabel('Platform')).toBeChecked()
  await expect(page.getByLabel('payments')).toBeChecked()
})

test('team creation uses the deliberate create dialog and server API', async ({ page }) => {
  let created = false
  await mock(page, 'owner')
  await page.getByRole('button', { name: 'Create team' }).click()
  const dialog = page.getByRole('dialog', { name: 'Create team' })
  await dialog.getByLabel('Team name').fill('Release Engineering')
  await page.route(`**/api/v1/orgs/${org}/teams/**`, async route => {
    if (route.request().method() !== 'PUT') return route.fallback()
    const body = route.request().postDataJSON() as { name: string }
    created = body.name === 'Release Engineering' && route.request().headers()['if-match'] === '"0"'
    return route.fulfill({ json: { id: '00000000-0000-4000-8000-0000000000cc', name: body.name, repository_ids: [], version: 1 } })
  })
  await dialog.getByRole('button', { name: 'Create team' }).click()
  await expect.poll(() => created).toBe(true)
})

test('does not show restricted access while an owner session is pending', async ({ page }) => {
  let releaseSession!: () => void
  let sessionRequested!: () => void
  const sessionGate = new Promise<void>(resolve => { releaseSession = resolve })
  const requested = new Promise<void>(resolve => { sessionRequested = resolve })
  await page.route('**/api/v1/session', async route => {
    sessionRequested()
    await sessionGate
    return route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1 }], memberships: [{ org_id: org, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } })
  })
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { name: 'Reforge', version: 'test', edition: 'self-hosted', development: true, fixture_auth: true } }))
  await page.route(`**/api/v1/orgs/${org}/teams**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route(`**/api/v1/orgs/${org}/repositories**`, route => route.fulfill({ json: { items: [], complete: true } }))
  const navigation = page.goto(`/org/${org}/organisation`)
  await requested
  await expect(page.getByRole('heading', { name: 'Opening Reforge' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Organisation access restricted' })).toHaveCount(0)
  releaseSession()
  await navigation
  await expect(page.getByRole('tab', { name: 'Teams' })).toBeVisible()
})

test('shows retryable session error instead of a false restricted state', async ({ page }) => {
  await page.route('**/api/v1/session', route => route.fulfill({ status: 503, json: { error: 'session service unavailable' } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { name: 'Reforge', version: 'test', edition: 'self-hosted', development: true, fixture_auth: true } }))
  await page.goto(`/org/${org}/organisation`)
  await expect(page.getByRole('heading', { name: 'Session unavailable' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Retry' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Organisation access restricted' })).toHaveCount(0)
})

test('empty team state offers one create action', async ({ page }) => {
  await mock(page, 'owner')
  await page.route(`**/api/v1/orgs/${org}/teams**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.reload()
  await expect(page.getByText('No teams yet')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Create team' })).toHaveCount(1)
})

test('viewer is restricted and administrators do not see owner-only settings', async ({ page }) => {
  await mock(page, 'viewer')
  await expect(page.getByRole('heading', { name: 'Organisation access restricted' })).toBeVisible()
  await expect(page.getByText(/Ask an organisation owner or administrator/)).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Teams', exact: true })).toHaveCount(0)
  await mock(page, 'admin')
  await expect(page.getByRole('tab', { name: 'Teams' })).toBeVisible()
  await expect(page.getByRole('tab', { name: 'Members' })).toHaveCount(0)
  await expect(page.getByRole('tab', { name: 'Identity' })).toHaveCount(0)
})

test('identity read failure keeps a retry path', async ({ page }) => {
  let available = false
  const settings: OrgOIDCSettings = { configured: true, secret_present: true, issuer: 'https://idp.example.test', client_id: 'reforge', status: 'draft', version: 1, verified: false, activation_available: false }
  await mock(page, 'owner')
  await page.route(`**/api/v1/orgs/${org}/identity/oidc`, async route => {
    if (!available) return route.fulfill({ status: 503, json: { code: 'secret_storage_unavailable', message: 'Identity settings unavailable', request_id: 'identity-test', retryable: true } })
    return route.fulfill({ json: settings })
  })
  await page.getByRole('tab', { name: 'Identity' }).click()
  await expect(page.getByText(/Identity settings unavailable/)).toBeVisible()
  available = true
  await page.getByRole('button', { name: 'Retry' }).click()
  await expect(page.getByLabel('Issuer URL')).toHaveValue('https://idp.example.test')
})

test('same-version status refresh preserves unsaved identity edits', async ({ page }) => {
  let reads = 0
  const draft: OrgOIDCSettings = { configured: true, secret_present: true, issuer: 'https://idp.example.test', client_id: 'reforge', status: 'draft', version: 3, verified: false, activation_available: false }
  await mock(page, 'owner')
  await page.route(`**/api/v1/orgs/${org}/identity/oidc`, route => {
    reads += 1
    return route.fulfill({ json: reads === 1 ? draft : { ...draft, status: 'probe_verified', verified: true, verified_at: '2026-09-23T02:00:00Z' } })
  })
  await page.getByRole('tab', { name: 'Identity' }).click()
  await expect(page.getByLabel('Issuer URL')).toHaveValue('https://idp.example.test')
  await page.getByLabel('Issuer URL').fill('https://edited.example.test')
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await expect.poll(() => reads).toBeGreaterThan(1)
  await expect(page.locator('.identity-status').getByText('Unsaved changes')).toBeVisible()
  await expect(page.locator('.identity-status').getByText('Metadata verified')).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Probe issuer metadata' })).toBeDisabled()
  await expect(page.getByLabel('Issuer URL')).toHaveValue('https://edited.example.test')
})

test('failed metadata probe remains visibly unverified', async ({ page }) => {
  let settings: OrgOIDCSettings = { configured: true, secret_present: true, issuer: 'https://idp.example.test', client_id: 'reforge', status: 'probe_verified', version: 4, verified: true, verified_at: '2026-09-22T00:00:00Z', activation_available: false }
  let failRefresh = false
  await mock(page, 'owner')
  await page.route(`**/api/v1/orgs/${org}/identity/oidc`, route => failRefresh
    ? route.fulfill({ status: 503, json: { code: 'secret_storage_unavailable', message: 'Identity settings unavailable', request_id: 'refresh-test', retryable: true } })
    : route.fulfill({ json: settings }))
  await page.route(`**/api/v1/orgs/${org}/identity/oidc/probe`, route => {
    failRefresh = true
    settings = { ...settings, status: 'draft', verified: false, verified_at: undefined }
    return route.fulfill({ status: 422, json: { code: 'issuer_unverified', message: 'Issuer metadata could not be verified', request_id: 'probe-test', retryable: false } })
  })
  await page.getByRole('tab', { name: 'Identity' }).click()
  await expect(page.locator('.identity-status').getByText('Metadata verified')).toBeVisible()
  const failedProbe = page.waitForResponse(response => response.url().endsWith('/identity/oidc/probe'))
  await page.getByRole('button', { name: 'Probe issuer metadata' }).click()
  const probeResponse = await failedProbe
  expect(probeResponse.status()).toBe(422)
  await expect(page.getByRole('alert').getByText('Issuer metadata could not be verified')).toBeVisible()
  await expect(page.getByText('Refresh failed: Identity settings unavailable')).toBeVisible()
  await expect(page.locator('.identity-status').getByText('Draft')).toBeVisible()
  await expect(page.locator('.identity-status').getByText('Metadata verified')).toHaveCount(0)
})

test('owner saves, probes, activates, signs in, reloads and disables OIDC without exposing the secret', async ({ page }) => {
  let settings: OrgOIDCSettings = { configured: false, secret_present: false, status: 'unconfigured', version: 0, verified: false, activation_available: false, activation_blocked: 'Organisation login is not available until org-aware login support is implemented' }
  let put: { body: { issuer: string; client_id: string; client_secret: string }; ifMatch: string; csrf: string } | undefined
  let probe: { ifMatch: string; csrf: string } | undefined
  let disabled: { ifMatch: string; csrf: string } | undefined
  let activated: { ifMatch: string; csrf: string } | undefined
  await mock(page, 'owner')
  await page.route(`**/api/v1/orgs/${org}/identity/oidc`, async route => {
    if (route.request().method() === 'GET') return route.fulfill({ json: settings })
    const body = route.request().postDataJSON() as { issuer: string; client_id: string; client_secret: string }
    put = { body, ifMatch: route.request().headers()['if-match'] ?? '', csrf: route.request().headers()['x-csrf-token'] ?? '' }
    settings = { ...settings, configured: true, secret_present: true, issuer: body.issuer, client_id: body.client_id, status: 'draft', version: 1 }
    return route.fulfill({ json: settings })
  })
  await page.route(`**/api/v1/orgs/${org}/identity/oidc/probe`, async route => {
    probe = { ifMatch: route.request().headers()['if-match'] ?? '', csrf: route.request().headers()['x-csrf-token'] ?? '' }
    settings = { ...settings, status: 'probe_verified', verified: true, activation_available: true, verified_at: '2026-09-23T02:00:00Z' }
    return route.fulfill({ json: settings })
  })
  await page.route(`**/api/v1/orgs/${org}/identity/oidc/activate`, async route => {
    activated = { ifMatch: route.request().headers()['if-match'] ?? '', csrf: route.request().headers()['x-csrf-token'] ?? '' }
    settings = { ...settings, status: 'active', activation_available: false }
    return route.fulfill({ status: 204 })
  })
  await page.route(`**/api/v1/orgs/${org}/identity/oidc/disable`, async route => {
    disabled = { ifMatch: route.request().headers()['if-match'] ?? '', csrf: route.request().headers()['x-csrf-token'] ?? '' }
    settings = { ...settings, status: 'disabled', version: 2, verified: false, verified_at: undefined }
    return route.fulfill({ json: settings })
  })

  await page.getByRole('tab', { name: 'Teams' }).focus()
  await page.getByRole('tab', { name: 'Teams' }).press('ArrowRight')
  await expect(page.getByRole('tab', { name: 'Members' })).toHaveAttribute('aria-selected', 'true')
  await page.getByRole('tab', { name: 'Members' }).press('ArrowRight')
  await expect(page.getByRole('tab', { name: 'Identity' })).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByLabel('Issuer URL')).toBeVisible()
  await page.getByLabel('Issuer URL').fill('https://idp.example.test')
  await page.getByLabel('Client ID').fill('reforge')
  await page.getByLabel('Client secret').fill('never-render-this-secret')
  await page.getByRole('button', { name: 'Save draft' }).click()
  await expect.poll(() => put?.body.client_id).toBe('reforge')
  expect(put?.body.client_secret).toBe('never-render-this-secret')
  expect(put?.ifMatch).toBe('"0"')
  expect(put?.csrf).toBe('csrf-1')
  await expect(page.getByLabel('Client secret')).toHaveValue('')
  await expect(page.locator('.identity-status').getByText('Draft')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Activate login' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Probe issuer metadata' })).toBeEnabled()
  await expect(page.getByText('never-render-this-secret')).toHaveCount(0)

  await page.getByRole('button', { name: 'Probe issuer metadata' }).click()
  await expect.poll(() => probe?.ifMatch).toBe('"1"')
  expect(probe?.csrf).toBe('csrf-1')
  await expect(page.locator('.identity-status').getByText('Metadata verified')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Activate login' })).toBeEnabled()
  const activationResponse = page.waitForResponse(response => response.url().endsWith('/identity/oidc/activate'))
  await page.getByRole('button', { name: 'Activate login' }).click()
  expect((await activationResponse).status()).toBe(204)
  await expect.poll(() => activated?.ifMatch).toBe('"1"')
  expect(activated?.csrf).toBe('csrf-1')
  await expect(page.locator('.identity-status').getByText('Login active')).toBeVisible()
  await expect(page.getByText('Draft', { exact: true })).toHaveCount(0)
  await expect(page.getByText('Login unavailable', { exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Activate login' })).toHaveCount(0)
  await expect(page.locator('.identity-signin-link code')).toHaveText(new URL(`/sign-in?org=${org}`, page.url()).toString())
  await expect(page.getByRole('button', { name: 'Copy sign-in link' })).toBeVisible()

  const captures = resolve(process.cwd(), '../.local/t29-identity-gui')
  await mkdir(captures, { recursive: true })
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.screenshot({ path: resolve(captures, 'identity-light-1440.png'), fullPage: true })
  await page.getByRole('button', { name: 'Switch to dark theme' }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await page.screenshot({ path: resolve(captures, 'identity-dark-1440.png'), fullPage: true })
  await page.setViewportSize({ width: 390, height: 844 })
  const menu = page.getByRole('button', { name: 'Menu' })
  await expect(menu).toHaveAttribute('aria-expanded', 'false')
  await expect.poll(() => page.locator('.sidebar').evaluate(element => element.getBoundingClientRect().right)).toBeLessThanOrEqual(0)
  await page.screenshot({ path: resolve(captures, 'identity-dark-390.png'), fullPage: true })
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390)
  await page.getByRole('button', { name: 'Switch to light theme' }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
  await page.screenshot({ path: resolve(captures, 'identity-light-390.png'), fullPage: true })

  await page.reload()
  await page.getByRole('tab', { name: 'Identity' }).click()
  await expect(page.getByLabel('Issuer URL')).toHaveValue('https://idp.example.test')
  await expect(page.getByLabel('Client ID')).toHaveValue('reforge')
  await expect(page.getByLabel('Client secret')).toHaveValue('')
  await expect(page.locator('.identity-status').getByText('Login active')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Activate login' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Copy sign-in link' })).toBeVisible()
  await page.getByRole('button', { name: 'Disable', exact: true }).click()
  await expect(page.getByRole('dialog', { name: 'Disable organisation login' })).toBeVisible()
  await page.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click()
  expect(disabled).toBeUndefined()
  await page.getByRole('button', { name: 'Disable', exact: true }).click()
  await page.getByRole('dialog').getByRole('button', { name: 'Disable', exact: true }).click()
  await expect.poll(() => disabled?.ifMatch).toBe('"1"')
  expect(disabled?.csrf).toBe('csrf-1')
  await expect(page.locator('.identity-status').getByText('Disabled')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Copy sign-in link' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Activate login' })).toHaveCount(0)

  await page.setViewportSize({ width: 390, height: 844 })
  const width = await page.evaluate(() => document.documentElement.scrollWidth)
  expect(width).toBeLessThanOrEqual(390)
  await expect(page.getByRole('button', { name: 'Menu' })).toHaveAttribute('aria-expanded', 'false')
})
