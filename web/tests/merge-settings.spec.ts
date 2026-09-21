import { test, expect, type Page } from '@playwright/test'
const org = '00000000-0000-4000-8000-000000000001'; const repo = 'repo-merge'; const change = 'change-1'
const config = { repository_id: repo, version: 4, enabled: false, inspector_connection_id: 'forge-2', check_publishers: { ci: 'native-ci' }, cooperation_reference: 'gitea-policy', qualification: { provider: 'gitea', server_version: '1.24', connection_version: 4, inspector_version: 7, evidence_reference: 'evidence://merge', evidence_sha256: 'a'.repeat(64), verified_at: '2026-09-20T00:00:00Z', expires_at: '2026-10-20T00:00:00Z', exact_head: true, strict_target: true, queue_execution_gate: false } }
const repository = { id: repo, org_id: org, connection_id: 'forge-1', native_id: '1', name: 'repo-merge', url: 'http://127.0.0.1:53000/reforge-bot/repo-merge', provider: 'gitea', default_branch: 'main', archived: false, paused: false, accessible: true, team_ids: [], last_synced_at: new Date().toISOString(), version: 1 }
test.use({ trace: 'off' })
const giteaPrimary = { id: 'forge-1', name: 'Primary forge', provider: 'gitea', endpoint: 'http://127.0.0.1:53000', version: 4, server_version: '1.24', state: 'healthy' }
const giteaInspector = { id: 'forge-2', name: 'Inspector forge', provider: 'gitea', endpoint: 'http://127.0.0.1:53000', version: 7, server_version: '1.24', state: 'healthy' }
async function common(page: Page, role = 'owner', primary: Record<string, unknown> = giteaPrimary, list: Array<Record<string, unknown>> = [giteaPrimary, giteaInspector]) {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }], memberships: [{ org_id: org, role, team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route(`**/api/v1/orgs/${org}/repositories?*`, route => route.fulfill({ json: { items: [repository], complete: true } }))
  await page.route(`**/api/v1/orgs/${org}/repositories/${repo}`, route => route.fulfill({ json: repository }))
  await page.route(`**/api/v1/orgs/${org}/repositories/${repo}/changes?*`, route => route.fulfill({ json: { items: [{ id: change, title: 'Native change', body: '', url: 'http://127.0.0.1:53000/change/1', head_sha: 'a'.repeat(40), target_sha: 'b'.repeat(40), head_branch: 'repair', target_branch: 'main', state: 'open', merge_status: 'unknown', operation_id: '', repository: { native_id: '1', full_name: 'reforge-bot/repo-merge' }, head_repository: { native_id: '1', full_name: 'reforge-bot/repo-merge' }, target_repository: { native_id: '1', full_name: 'reforge-bot/repo-merge' }, author_login: 'bot', author_type: 'bot' }], complete: true } }))
  await page.route(`**/api/v1/orgs/${org}/repositories/${repo}/merge-configuration`, route => route.request().method() === 'GET' ? route.fulfill({ json: config }) : route.fulfill({ json: { ...config, version: 5, enabled: true } }))
  await page.route(`**/api/v1/orgs/${org}/connections?*`, route => route.fulfill({ json: { items: list } }))
  await page.route(`**/api/v1/orgs/${org}/connections/forge-1`, route => route.fulfill({ json: primary }))
  await page.goto(`/org/${org}/changes?repository=${repo}&change=${change}`)
  await page.getByRole('button', { name: 'Merge settings', exact: true }).click()
}
test('saves merge settings with current connection metadata and CSRF', async ({ page }) => { let body: Record<string, unknown> | undefined; await common(page); await page.route(`**/api/v1/orgs/${org}/repositories/${repo}/merge-configuration`, async route => { if (route.request().method() === 'PUT') { expect(route.request().headers()['if-match']).toBe('"4"'); expect(route.request().headers()['x-csrf-token']).toBeTruthy(); body = route.request().postDataJSON(); return route.fulfill({ json: { ...config, version: 5, enabled: true } }) } return route.fulfill({ json: config }) }); await page.getByRole('checkbox', { name: 'Enabled' }).check(); await page.getByRole('button', { name: 'Save merge settings' }).click(); await expect.poll(() => body).toBeTruthy(); expect(body?.qualification).toMatchObject({ provider: 'gitea', connection_version: 4, inspector_version: 7, queue_execution_gate: false }); await expect(page.getByRole('button', { name: 'Merge settings', exact: true })).toBeVisible() })
test('member cannot edit', async ({ page }) => { await common(page, 'viewer'); await expect(page.getByRole('button', { name: 'Save merge settings' })).toBeDisabled() })
test('publisher deletion is included in saved configuration', async ({ page }) => {
  let body: Record<string, unknown> | undefined
  await common(page)
  await page.route(`**/api/v1/orgs/${org}/repositories/${repo}/merge-configuration`, async route => { if (route.request().method() === 'PUT') { body = route.request().postDataJSON(); return route.fulfill({ json: { ...config, check_publishers: {}, version: 5 } }) }; return route.fulfill({ json: config }) })
  await page.getByText('Check publishers').click()
  await page.getByRole('button', { name: 'Remove' }).click()
  await page.getByRole('button', { name: 'Save merge settings' }).click()
  await expect.poll(() => body).toBeTruthy()
  expect(body?.check_publishers).toEqual({})
})

test('disabled configuration saves without an inspector', async ({ page }) => {
  let body: Record<string, unknown> | undefined
  await common(page)
  await page.route(`**/api/v1/orgs/${org}/repositories/${repo}/merge-configuration`, async route => { if (route.request().method() === 'PUT') { body = route.request().postDataJSON(); return route.fulfill({ json: { ...config, enabled: false, inspector_connection_id: undefined, version: 5 } }) }; return route.fulfill({ json: config }) })
  await page.getByLabel('Protection reader').selectOption('')
  await page.getByRole('checkbox', { name: 'Enabled' }).uncheck()
  await page.getByRole('button', { name: 'Save merge settings' }).click()
  await expect.poll(() => body).toBeTruthy()
  expect(body?.inspector_connection_id).toBeUndefined()
})

test('non-Gitea configuration can enable without a protection reader', async ({ page }) => {
  let body: Record<string, unknown> | undefined
  const githubPrimary = { id: 'forge-1', name: 'Primary forge', provider: 'github', endpoint: 'https://github.example', version: 4, server_version: '1', state: 'healthy' }
  await common(page, 'owner', githubPrimary, [githubPrimary, giteaInspector])
  await page.route(`**/api/v1/orgs/${org}/repositories/${repo}/merge-configuration`, async route => { if (route.request().method() === 'PUT') { body = route.request().postDataJSON(); return route.fulfill({ json: { ...config, enabled: true, inspector_connection_id: undefined, version: 5 } }) }; return route.fulfill({ json: config }) })
  await expect(page.getByText(/Current provider:/)).toHaveText(/Current provider: github/)
  await page.getByRole('checkbox', { name: 'Enabled' }).check()
  await page.getByRole('button', { name: 'Save merge settings' }).click()
  await expect.poll(() => body).toBeTruthy()
  expect(body?.qualification).toMatchObject({ provider: 'github', inspector_version: 0 })
})

test('version conflict remains visible and does not claim saved', async ({ page }) => {
  await common(page)
  await page.route(`**/api/v1/orgs/${org}/repositories/${repo}/merge-configuration`, async route => { if (route.request().method() === 'PUT') return route.fulfill({ status: 409, json: { code: 'conflict', message: 'configuration changed; reload' } }); return route.fulfill({ json: config }) })
  await page.getByRole('checkbox', { name: 'Enabled' }).check()
  await page.getByRole('button', { name: 'Save merge settings' }).click()
  await expect(page.getByRole('alert')).toContainText('configuration changed; reload')
  await expect(page.getByRole('status', { name: 'Merge settings saved.' })).toHaveCount(0)
})
