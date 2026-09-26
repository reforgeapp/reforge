import { test, expect, type Page } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'
const team = '00000000-0000-4000-8000-0000000000bb'

async function fixture(page: Page) {
  let teamName = 'Platform'
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }], memberships: [{ org_id: org, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { name: 'Reforge', version: 'test', edition: 'self-hosted', development: true, fixture_auth: true } }))
  await page.route(`**/api/v1/orgs/${org}/teams**`, async route => {
    if (route.request().method() === 'PUT') {
      const input = route.request().postDataJSON() as { name: string }
      teamName = input.name
      return route.fulfill({ json: { id: team, name: teamName, version: 2, repository_ids: [] } })
    }
    if (route.request().method() === 'DELETE') return route.fulfill({ status: 204, body: '' })
    return route.fulfill({ json: { items: [{ id: team, name: teamName, version: teamName === 'Platform' ? 1 : 2, repository_ids: [] }], complete: true } })
  })
  await page.route(`**/api/v1/orgs/${org}/repositories**`, route => route.fulfill({ json: { items: [{ id: 'repo-1', name: 'payments', provider: 'github', default_branch: 'main', archived: false, paused: false, accessible: true, team_ids: [team], version: 1, connection_id: 'forge-1', native_id: '1', url: 'https://github.example/payments', org_id: org, last_synced_at: null }], complete: true } }))
  await page.route(`**/api/v1/orgs/${org}/connections**`, route => route.fulfill({ json: { items: [{ id: 'forge-1', name: 'Fixture Forge', provider: 'github', endpoint: 'https://github.example', kind: 'forge', version: 1, state: 'healthy' }], complete: true } }))
}

test('repository to organisation keeps warmed team cache usable', async ({ page }) => {
  await fixture(page)
  await page.goto(`/org/${org}/repositories`)
  await expect(page.getByLabel('Team')).toContainText('Platform')
  await page.getByRole('link', { name: 'Organisation', exact: true }).click()
  await expect(page.getByRole('tab', { name: 'Teams', selected: true })).toBeVisible()
  await expect(page.getByRole('textbox', { name: `Team name ${team}` })).toHaveValue('Platform')
  await page.getByRole('link', { name: 'Repositories', exact: true }).click()
  await expect(page.getByRole('button', { name: 'payments', exact: true })).toBeVisible()
  await expect(page.getByLabel('Team')).toContainText('Platform')
})

test('organisation team mutation refreshes repository team options', async ({ page }) => {
  await fixture(page)
  await page.goto(`/org/${org}/organisation`)
  await page.getByRole('link', { name: 'Repositories', exact: true }).click()
  await expect(page.getByLabel('Team')).toContainText('Platform')
  await page.getByRole('link', { name: 'Organisation', exact: true }).click()
  const name = page.getByRole('textbox', { name: `Team name ${team}` })
  await expect(name).toHaveValue('Platform')
  const update = page.waitForRequest(request => request.method() === 'PUT' && request.url().includes(`/api/v1/orgs/${org}/teams/${team}`))
  const refresh = page.waitForResponse(response => response.request().method() === 'GET' && response.url().includes(`/api/v1/orgs/${org}/teams`))
  await name.fill('Core Platform')
  await page.getByRole('button', { name: 'Save changes' }).click()
  expect((await update).headers()['if-match']).toBe('"1"')
  await refresh
  await expect(name).toHaveValue('Core Platform')
  await page.getByRole('link', { name: 'Repositories', exact: true }).click()
  await expect(page.getByLabel('Team')).toContainText('Core Platform')
})

test('organisation team rename refreshes policy and usage team pickers', async ({ page }) => {
  await fixture(page)
  await page.goto(`/org/${org}/policies`)
  await page.getByRole('group', { name: 'Policy scope' }).getByRole('button', { name: 'Team' }).click()
  await expect(page.getByLabel('Policy team')).toContainText('Platform')
  await page.getByRole('link', { name: 'Usage', exact: true }).click()
  await expect(page.getByRole('combobox', { name: 'Team', exact: true })).toContainText('Platform')

  await page.getByRole('link', { name: 'Organisation', exact: true }).click()
  const name = page.getByRole('textbox', { name: `Team name ${team}` })
  await expect(name).toHaveValue('Platform')
  const refresh = page.waitForResponse(response => response.request().method() === 'GET' && response.url().includes(`/api/v1/orgs/${org}/teams`))
  await name.fill('Core Platform')
  await page.getByRole('button', { name: 'Save changes' }).click()
  await refresh

  await page.getByRole('link', { name: 'Policies', exact: true }).click()
  await page.getByRole('group', { name: 'Policy scope' }).getByRole('button', { name: 'Team' }).click()
  await expect(page.getByLabel('Policy team')).toContainText('Core Platform')
  await page.getByRole('link', { name: 'Usage', exact: true }).click()
  await expect(page.getByRole('combobox', { name: 'Team', exact: true })).toContainText('Core Platform')
})
