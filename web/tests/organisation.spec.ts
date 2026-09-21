import { test, expect, type Page } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'
const user = '00000000-0000-4000-8000-0000000000aa'
const team = '00000000-0000-4000-8000-0000000000bb'

async function mock(page: Page, role: string) {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }], memberships: [{ org_id: org, role, team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { name: 'Reforge', version: 'test', edition: 'self-hosted', development: true, fixture_auth: true } }))
  await page.route(`**/api/v1/orgs/${org}/memberships**`, route => route.fulfill({ json: { items: [{ org_id: org, user_id: user, role: 'viewer', team_ids: [], repository_ids: [], all_repositories: false, version: 2 }], complete: true } }))
  await page.route(`**/api/v1/orgs/${org}/teams**`, route => route.fulfill({ json: { items: [{ id: team, name: 'Platform', repository_ids: [], version: 1 }], complete: true } }))
  await page.route(`**/api/v1/orgs/${org}/repositories**`, route => route.fulfill({ json: { items: [{ id: 'repo-1', name: 'payments', provider: 'github', default_branch: 'main', archived: false, paused: false, accessible: true, team_ids: [], version: 1, connection_id: 'forge-1', native_id: '1', url: 'https://github.example/payments', org_id: org, last_synced_at: null }], complete: true } }))
  await page.goto(`/org/${org}/organisation`)
}

test('an owner reviews members and teams with version-checked edits', async ({ page }) => {
  let saved: { user: string; role: string; ifMatch: string } | undefined
  await mock(page, 'owner')
  await page.route(`**/api/v1/orgs/${org}/memberships/${user}`, route => { saved = { user, role: (route.request().postDataJSON() as { role: string }).role, ifMatch: route.request().headers()['if-match'] ?? '' }; return route.fulfill({ json: { org_id: org, user_id: user, role: 'maintainer', team_ids: [], repository_ids: [], all_repositories: false, version: 3 } }) })
  await expect(page.getByRole('heading', { name: 'Teams', exact: true })).toBeVisible()
  await expect(page.getByRole('textbox', { name: `Team name ${team}` })).toHaveValue('Platform')
  await page.getByLabel(`Role for ${user}`).selectOption('maintainer')
  await expect.poll(() => saved?.role).toBe('maintainer')
  expect(saved?.ifMatch).toBe('"2"')
})

test('a viewer sees an actionable blocked state instead of administration controls', async ({ page }) => {
  await mock(page, 'viewer')
  await expect(page.getByRole('heading', { name: 'Organisation administration restricted' })).toBeVisible()
  await expect(page.getByText(/requires the owner or administrator role/)).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Teams', exact: true })).toHaveCount(0)
})
