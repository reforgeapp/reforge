import { test, expect, type Page } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'
const user = '00000000-0000-4000-8000-0000000000aa'
const team = '00000000-0000-4000-8000-0000000000bb'

async function mock(page: Page, role: string) {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }], memberships: [{ org_id: org, role, team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { name: 'Reforge', version: 'test', edition: 'self-hosted', development: true, fixture_auth: true } }))
  await page.route(`**/api/v1/orgs/${org}/memberships**`, route => route.fulfill({ json: { items: [{ org_id: org, user_id: user, role: 'viewer', all_repositories: false, team_ids: [], repository_ids: [], version: 2 }], complete: true } }))
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

test('viewer sees an actionable permission state, identity avoids a fake configuration form, and mobile layout fits', async ({ page }) => {
  await mock(page, 'viewer')
  await expect(page.getByRole('heading', { name: 'Organisation access restricted' })).toBeVisible()
  await expect(page.getByText(/Ask an organisation owner or administrator/)).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Teams', exact: true })).toHaveCount(0)
  await mock(page, 'owner')
  await page.getByRole('tab', { name: 'Identity' }).click()
  await expect(page.getByText('Identity settings are not available yet.')).toBeVisible()
  await expect(page.getByText(/OIDC issuer, session lifetime and audit retention/)).toHaveCount(0)
  await page.setViewportSize({ width: 390, height: 844 })
  const width = await page.evaluate(() => document.documentElement.scrollWidth)
  expect(width).toBeLessThanOrEqual(390)
  await expect(page.getByRole('button', { name: 'Menu' })).toHaveAttribute('aria-expanded', 'false')
})
