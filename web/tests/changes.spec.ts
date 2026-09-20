import { test, expect, type Page } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'
const otherOrg = '00000000-0000-4000-8000-000000000002'
const repo = '11111111-1111-4111-8111-111111111111'
const change = 'change-1'
const base = `/api/v1/orgs/${org}`
const changeBody = { id: change, title: 'Repair dependency', body: 'Details', url: 'https://forge.example/change/1', head_sha: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', target_sha: 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', head_branch: 'repair', target_branch: 'main', state: 'open', merge_status: 'clean', operation_id: '', author_login: 'bot', author_type: 'bot', repository: { native_id: 'repo-1', full_name: 'team/repo' }, head_repository: { native_id: 'repo-1', full_name: 'team/repo' }, target_repository: { native_id: 'repo-1', full_name: 'team/repo' } }

async function signIn(page: Page) { await page.goto('/auth/login'); await expect(page).toHaveURL(/\/org\/[^/]+\/overview/) }
async function fixture(page: Page, outcome = 'allow', expiresAt = Date.now() + 60_000, companions: Array<{ task_id: string; change_id: string; head_sha: string; merge_sha: string; state: 'repair_pending' | 'merge_pending' | 'merged' }> = []) {
  await page.route(`**${base}/repositories?**`, route => route.fulfill({ json: { items: [{ id: repo, name: 'team/repo', provider: 'gitea', team_ids: [], accessible: true }], complete: true } }))
  await page.route(`**${base}/repositories/${repo}/changes?**`, route => route.fulfill({ json: { items: [changeBody], complete: true, snapshot_state: 'fresh', connection_version: 1 } }))
  await page.route(`**${base}/merge-operations*`, async route => { if (route.request().method() === 'GET') return route.fulfill({ json: { items: [], complete: true } }); if (!route.request().headers()['x-csrf-token']) throw new Error('merge request missing CSRF'); await route.fulfill({ json: { id: 'operation-1', repository_id: repo, gate_id: 'fresh-gate', requested_gate_id: 'gate-1', change_id: change, state: 'queued', reason: '', cancel_requested: false, version: 1, created_at: new Date().toISOString(), updated_at: new Date().toISOString() } }) })
  await page.route(`**${base}/repositories/${repo}/changes/${change}/merge-preview`, async route => { if (route.request().method() !== 'POST' || !route.request().headers()['x-csrf-token']) throw new Error('preview missing CSRF'); await route.fulfill({ json: { id: 'gate-1', repository_id: repo, connection_id: 'conn-1', connection_version: 1, configuration_version: 1, method: 'merge', expires_at: new Date(expiresAt).toISOString(), companions, snapshot: { change: changeBody, rules: { state: 'supported', reason: '', hash: 'rules', required_checks: [], required_approvals: 0, require_code_owners: false }, checks: [], approvals: [], native: { state: 'eligible', head_sha: changeBody.head_sha, target_sha: changeBody.target_sha }, capabilities: { provider: 'gitea', server_version: '1' } }, decision: { outcome, blockers: outcome === 'allow' ? [] : ['native gate blocked'], required_actions: [] }, binding: { head: changeBody.head_sha, target: changeBody.target_sha, tested: changeBody.head_sha, policy_hash: 'policy', provider_rules: 'rules' } } }) })
}

test('scopes changes and shows H T C gate evidence', async ({ page }) => { await signIn(page); await fixture(page); await page.goto(`/org/${org}/changes`); await expect(page.getByRole('heading', { name: 'Changes', exact: true })).toBeVisible(); await page.getByRole('button', { name: 'Repair dependency' }).click(); await page.getByRole('button', { name: 'Preview merge gate' }).click(); await expect(page.getByText('H head')).toBeVisible(); await expect(page.getByText('T target')).toBeVisible(); await expect(page.getByText('C tested')).toBeVisible(); await expect(page.getByRole('link', { name: 'Open native change' })).toHaveAttribute('href', changeBody.url); await page.getByRole('button', { name: 'Request protected merge' }).click(); await expect(page.getByText('queued')).toBeVisible() })
test('keeps blocked preview and hostile native URL safe', async ({ page }) => { await signIn(page); await fixture(page, 'blocked'); await page.route(`**${base}/repositories/${repo}/changes?**`, route => route.fulfill({ json: { items: [{ ...changeBody, url: 'javascript:alert(1)' }], complete: true, snapshot_state: 'fresh', connection_version: 1 } })); await page.goto(`/org/${org}/changes`); await page.getByRole('button', { name: 'Repair dependency' }).click(); await page.getByRole('button', { name: 'Preview merge gate' }).click(); await expect(page.getByText(/Blocked: native gate blocked/)).toBeVisible(); await expect(page.getByRole('button', { name: 'Request protected merge' })).toBeDisabled(); await expect(page.getByText('Native change link unavailable')).toBeVisible() })

test('disables expired gate without dispatch', async ({ page }) => { await signIn(page); await fixture(page, 'allow', Date.now() - 1000); await page.goto(`/org/${org}/changes`); await page.getByRole('button', { name: 'Repair dependency' }).click(); await page.getByRole('button', { name: 'Preview merge gate' }).click(); await expect(page.getByText('Preview expired; run a new preview.')).toBeVisible(); await expect(page.getByRole('button', { name: 'Request protected merge' })).toBeDisabled() })

test('readonly member cannot preview or request merge', async ({ page }) => {
  await page.route('**/api/v1/session', async route => { const response = await route.fetch(); const body = await response.json() as { memberships: Array<Record<string, unknown>> }; body.memberships = [{ org_id: org, role: 'viewer', team_ids: [], repository_ids: [], all_repositories: true }]; await route.fulfill({ response, json: body }) })
  await signIn(page); await fixture(page); await page.goto(`/org/${org}/changes`); await page.getByRole('button', { name: 'Repair dependency' }).click(); await expect(page.getByRole('button', { name: 'Preview merge gate' })).toBeDisabled()
})

test('switching change clears prior gate evidence', async ({ page }) => {
  await signIn(page); await fixture(page); await page.route(`**${base}/repositories/${repo}/changes?**`, route => route.fulfill({ json: { items: [changeBody, { ...changeBody, id: 'change-2', title: 'Second change', head_sha: 'cccccccccccccccccccccccccccccccccccccc' }], complete: true, snapshot_state: 'fresh', connection_version: 1 } })); await page.goto(`/org/${org}/changes`); await page.getByRole('button', { name: 'Repair dependency' }).click(); await page.getByRole('button', { name: 'Preview merge gate' }).click(); await expect(page.getByText('H head')).toBeVisible(); await page.getByRole('button', { name: 'Second change' }).click(); await expect(page.getByText('H head')).toHaveCount(0)
})

test('reload recovers active operation and blocks redispatch', async ({ page }) => {
  await signIn(page); await fixture(page); await page.route(`**${base}/merge-operations*`, route => route.fulfill({ json: { items: [{ id: 'operation-reload', repository_id: repo, gate_id: 'gate-old', change_id: change, state: 'queued', reason: 'Recovered', cancel_requested: false, version: 2, created_at: new Date().toISOString(), updated_at: new Date().toISOString() }], complete: true } })); await page.goto(`/org/${org}/changes`); await page.getByRole('button', { name: 'Repair dependency' }).click(); await expect(page.getByText('Recovered')).toBeVisible(); await expect(page.getByRole('button', { name: 'Preview merge gate' })).toBeDisabled()
})

test('lost merge response recovers persisted operation before retry', async ({ page }) => {
  let persisted = false
  await signIn(page); await fixture(page); await page.route(`**${base}/merge-operations*`, async route => { if (route.request().method() === 'GET') return route.fulfill({ json: { items: persisted ? [{ id: 'operation-lost', repository_id: repo, gate_id: 'fresh-gate', requested_gate_id: 'gate-1', change_id: change, state: 'queued', reason: 'Recovered after lost response', cancel_requested: false, version: 1, created_at: new Date().toISOString(), updated_at: new Date().toISOString() }] : [], complete: true } }); persisted = true; await route.fulfill({ status: 503, json: { code: 'timeout', message: 'response lost' } }) }); await page.goto(`/org/${org}/changes`); await page.getByRole('button', { name: 'Repair dependency' }).click(); await page.getByRole('button', { name: 'Preview merge gate' }).click(); await page.getByRole('button', { name: 'Request protected merge' }).click(); await expect(page.getByText('Recovered after lost response')).toBeVisible()
})

test('merge actions remain keyboard reachable at narrow width', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 }); await signIn(page); await fixture(page); await page.goto(`/org/${org}/changes`); await page.getByRole('button', { name: 'Repair dependency' }).focus(); await page.keyboard.press('Enter'); await page.getByRole('button', { name: 'Preview merge gate' }).focus(); await expect(page.getByRole('button', { name: 'Preview merge gate' })).toBeFocused()
})

test('loads repository and native change cursor pages', async ({ page }) => {
  await signIn(page)
  const secondRepository = { id: '22222222-2222-4222-8222-222222222222', name: 'team/second', provider: 'gitea', team_ids: [], accessible: true }
  const secondChange = { ...changeBody, id: 'change-2', title: 'Second page change' }
  await page.route(`**${base}/repositories?**`, route => route.fulfill({ json: route.request().url().includes('cursor=repo-next') ? { items: [secondRepository], complete: true } : { items: [{ id: repo, name: 'team/repo', provider: 'gitea', team_ids: [], accessible: true }], next_cursor: 'repo-next', complete: false } }))
  await page.route(`**${base}/repositories/${repo}/changes?**`, route => route.fulfill({ json: { items: [changeBody], next_cursor: 'change-next', complete: false, snapshot_state: 'fresh', connection_version: 1 } }))
  await page.route(`**${base}/repositories/${secondRepository.id}/changes?**`, route => route.fulfill({ json: route.request().url().includes('cursor=change-next') ? { items: [secondChange], complete: true, snapshot_state: 'fresh', connection_version: 1 } : { items: [changeBody], next_cursor: 'change-next', complete: false, snapshot_state: 'fresh', connection_version: 1 } }))
  await page.goto(`/org/${org}/changes`)
  await page.getByRole('button', { name: 'Load more repositories' }).click()
  await page.getByRole('combobox', { name: 'Repository' }).selectOption(secondRepository.id)
  await page.getByRole('button', { name: 'Load more changes' }).click()
  await expect(page.getByRole('button', { name: 'Second page change' })).toBeVisible()
})

test('organization change selection does not carry gate state', async ({ page }) => {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }, { id: otherOrg, name: 'Other', version: 1, paused: false }], memberships: [{ org_id: org, role: 'maintainer', team_ids: [], repository_ids: [], all_repositories: true }, { org_id: otherOrg, role: 'maintainer', team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await signIn(page)
  await fixture(page)
  await page.route(`**/api/v1/orgs/${otherOrg}/repositories?**`, route => route.fulfill({ json: { items: [{ id: 'other-repo', name: 'other/repo', provider: 'gitea', team_ids: [], accessible: true }], complete: true } }))
  await page.route(`**/api/v1/orgs/${otherOrg}/repositories/other-repo/changes?**`, route => route.fulfill({ json: { items: [], complete: true, snapshot_state: 'fresh', connection_version: 1 } }))
  await page.goto(`/org/${org}/changes`)
  await page.getByRole('button', { name: 'Repair dependency' }).click()
  await expect(page.getByRole('button', { name: 'Preview merge gate' })).toBeEnabled()
  await page.getByRole('button', { name: 'Preview merge gate' }).click()
  await expect(page.getByText('H head')).toBeVisible()
  await page.goto(`/org/${otherOrg}/changes`)
  await expect(page.getByText('H head')).toHaveCount(0)
  await expect(page.getByText('No persisted changes observed.')).toBeVisible()
})

test('blocked companion evidence shows companion-first order and fresh original validation', async ({ page }) => {
  await signIn(page)
  await fixture(page, 'blocked', Date.now() + 60_000, [{ task_id: 'task-1', change_id: 'companion-1', head_sha: 'c'.repeat(40), merge_sha: 'd'.repeat(40), state: 'merge_pending' }])
  await page.goto(`/org/${org}/changes`)
  await page.getByRole('button', { name: 'Repair dependency' }).click()
  await page.getByRole('button', { name: 'Preview merge gate' }).click()
  await expect(page.getByRole('heading', { name: 'Companion merge order' })).toBeVisible()
  await expect(page.getByText('Original update · requires fresh native gate validation after companion merges.')).toBeVisible()
  await expect(page.getByText('merge pending')).toBeVisible()
})

test('merged companion still requires fresh original native gate', async ({ page }) => {
  await signIn(page)
  await fixture(page, 'allow', Date.now() + 60_000, [{ task_id: 'task-merged', change_id: 'companion-merged', head_sha: 'c'.repeat(40), merge_sha: 'd'.repeat(40), state: 'merged' }])
  await page.goto(`/org/${org}/changes`)
  await page.getByRole('button', { name: 'Repair dependency' }).click()
  await page.getByRole('button', { name: 'Preview merge gate' }).click()
  await expect(page.getByText('Original update · requires fresh native gate validation after companion merges.')).toBeVisible()
  await expect(page.getByText(/already validated/i)).toHaveCount(0)
})
