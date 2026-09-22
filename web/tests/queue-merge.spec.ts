import { test, expect, type Page } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'
const repo = 'repo-queue'
const change = 'change-queue'
const base = `/api/v1/orgs/${org}`
const changeBody = { id: change, title: 'Queue dependency', body: '', url: 'https://forge.example/change/1', head_sha: 'a'.repeat(40), target_sha: 'b'.repeat(40), head_branch: 'repair', target_branch: 'main', state: 'open', merge_status: 'clean', operation_id: '', author_login: 'bot', author_type: 'bot', repository: { native_id: 'repo-1', full_name: 'team/repo' }, head_repository: { native_id: 'repo-1', full_name: 'team/repo' }, target_repository: { native_id: 'repo-1', full_name: 'team/repo' } }
const repository = { id: repo, org_id: org, connection_id: 'forge-1', native_id: '1', name: 'team/repo', url: 'https://forge.example/team/repo', provider: 'github', default_branch: 'main', archived: false, paused: false, accessible: true, team_ids: [], last_synced_at: new Date().toISOString(), version: 1 }
const config = { repository_id: repo, version: 4, enabled: true, inspector_connection_id: undefined, check_publishers: { 'reforge/merge-policy': '42' }, cooperation_reference: 'github-policy', qualification: { provider: 'github', server_version: '1', connection_version: 4, inspector_version: 0, evidence_reference: 'evidence://queue', evidence_sha256: 'a'.repeat(64), verified_at: '2026-09-20T00:00:00Z', expires_at: '2026-10-20T00:00:00Z', exact_head: true, strict_target: false, queue_execution_gate: true } }

test.use({ trace: 'off' })

async function signIn(page: Page, authKind = 'github_app', provider = 'github') {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }], memberships: [{ org_id: org, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route(`**${base}/repositories?**`, route => route.fulfill({ json: { items: [repository], complete: true } }))
  await page.route(`**${base}/repositories/${repo}`, route => route.fulfill({ json: { ...repository, provider } }))
  await page.route(`**${base}/repositories/${repo}/changes?**`, route => route.fulfill({ json: { items: [changeBody], complete: true } }))
  await page.route(`**${base}/connections/forge-1`, route => route.fulfill({ json: { id: 'forge-1', name: 'Primary', provider, endpoint: 'https://forge.example', version: 4, state: 'healthy', server_version: '1', settings: { auth_kind: authKind, app_id: authKind === 'github_app' ? '42' : undefined } } }))
  await page.route(`**${base}/connections?*`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.goto(`/org/${org}/changes?repository=${repo}&change=${change}`)
}

function gate(phase: 'queue_admission' | 'queue_execution') {
  return { id: 'gate-1', phase, repository_id: repo, connection_id: 'forge-1', connection_version: 4, configuration_version: 4, method: 'merge', expires_at: new Date(Date.now() + 60_000).toISOString(), snapshot: { change: changeBody, queue: { id: phase === 'queue_execution' ? 'queue-1' : '', state: phase === 'queue_execution' ? 'queued' : 'not_queued', head_sha: changeBody.head_sha, target_sha: changeBody.target_sha, tested_sha: phase === 'queue_execution' ? 'c'.repeat(40) : '' }, execution_check: { name: 'reforge/merge-policy', publisher_id: '42' }, rules: { state: 'supported', reason: '', hash: 'rules', required_checks: [], required_approvals: 0, require_code_owners: false }, checks: [], approvals: [], native: { state: 'eligible', head_sha: changeBody.head_sha, target_sha: changeBody.target_sha }, capabilities: { provider: 'github', server_version: '1' } }, decision: { outcome: 'allow', blockers: [], required_actions: [], rules: [] }, binding: { head: changeBody.head_sha, target: changeBody.target_sha, tested: phase === 'queue_execution' ? 'c'.repeat(40) : changeBody.head_sha, policy_hash: 'policy', provider_rules: 'rules' } }
}

test('requests queue admission and shows queue evidence', async ({ page }) => {
  let requested = false
  await page.setViewportSize({ width: 390, height: 844 })
  await signIn(page)
  await page.route(`**${base}/merge-operations*`, async route => { if (route.request().method() === 'POST') { requested = true; return route.fulfill({ json: { id: 'operation-1', repository_id: repo, gate_id: 'gate-1', change_id: change, state: 'queued', reason: '', cancel_requested: false, version: 1, created_at: new Date().toISOString(), updated_at: new Date().toISOString() } }) }; return route.fulfill({ json: { items: [], complete: true } }) })
  await page.route(`**${base}/repositories/${repo}/changes/${change}/merge-preview`, route => route.fulfill({ json: gate('queue_admission') }))
  await page.getByRole('button', { name: 'Preview merge gate' }).click()
  await expect(page.getByRole('button', { name: 'Request queue admission' })).toBeEnabled()
  await page.getByRole('button', { name: 'Request queue admission' }).click()
  await expect.poll(() => requested).toBe(true)
  await expect(page.getByText(/not_queued/)).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})

test('queue execution cannot create another merge operation', async ({ page }) => {
  let requested = false
  await signIn(page)
  await page.route(`**${base}/merge-operations*`, async route => { if (route.request().method() === 'POST') requested = true; return route.fulfill({ json: { items: [], complete: true } }) })
  await page.route(`**${base}/repositories/${repo}/changes/${change}/merge-preview`, route => route.fulfill({ json: gate('queue_execution') }))
  await page.getByRole('button', { name: 'Preview merge gate' }).click()
  await expect(page.getByText(/Queue execution is controlled by the existing merge operation/)).toBeVisible()
  await expect(page.getByText('Native authority')).toBeVisible()
  await expect(page.getByText('Execution check')).toBeVisible()
  await expect(page.getByText('c'.repeat(40), { exact: true }).first()).toBeVisible()
  await expect(page.getByRole('button', { name: 'Queue execution handled by existing operation' })).toBeDisabled()
  expect(requested).toBe(false)
})

test('unsupported profile disables queue execution gate with guidance', async ({ page }) => {
  await signIn(page, 'token', 'gitea')
  await page.route(`**${base}/repositories/${repo}/merge-configuration`, route => route.fulfill({ json: { ...config, qualification: { ...config.qualification, queue_execution_gate: false } } }))
  await page.getByRole('button', { name: 'Merge settings', exact: true }).click()
  await page.getByText('Operator qualification evidence', { exact: true }).click()
  await expect(page.getByRole('checkbox', { name: 'Queue execution gate' })).toBeDisabled()
  await expect(page.getByText(/requires a healthy GitHub App or GitLab token connection/)).toBeVisible()
})

test('GitLab token saves queue qualification and CI identity', async ({ page }) => {
  await signIn(page, 'token', 'gitlab')
  let body: Record<string, unknown> | undefined
  await page.route(`**${base}/repositories/${repo}/merge-configuration`, async route => { if (route.request().method() === 'PUT') { body = route.request().postDataJSON(); return route.fulfill({ json: { ...config, version: 5, qualification: { ...config.qualification, provider: 'gitlab', queue_execution_gate: true, ci_config_sha256: 'a'.repeat(64) } } }) } return route.fulfill({ json: { ...config, qualification: { ...config.qualification, provider: 'gitlab', queue_execution_gate: false, ci_config_sha256: '' } } }) })
  await page.getByRole('button', { name: 'Merge settings', exact: true }).click()
  await page.getByText('Operator qualification evidence', { exact: true }).click()
  await page.getByRole('checkbox', { name: 'Queue execution gate' }).check()
  await page.getByRole('textbox', { name: 'CI configuration SHA-256' }).fill('a'.repeat(64))
  await page.getByText('Check publishers', { exact: true }).click()
  await page.getByRole('textbox', { name: 'Publisher 1' }).fill('12345')
  await page.getByRole('button', { name: 'Save merge settings' }).click()
  await expect.poll(() => body).toBeTruthy()
  expect(body?.qualification).toMatchObject({ provider: 'gitlab', queue_execution_gate: true, ci_config_sha256: 'a'.repeat(64) })
  expect(body?.check_publishers).toEqual({ 'reforge/merge-policy': '12345' })
})

test('GitLab queue rejection remains actionable', async ({ page }) => {
  await signIn(page, 'token', 'gitlab')
  await page.route(`**${base}/repositories/${repo}/merge-configuration`, async route => { if (route.request().method() === 'PUT') return route.fulfill({ status: 400, json: { message: 'GitLab queue execution requires a protected blocking reforge/merge-policy job.' } }); return route.fulfill({ json: { ...config, qualification: { ...config.qualification, provider: 'gitlab', queue_execution_gate: false, ci_config_sha256: '' } } }) })
  await page.getByRole('button', { name: 'Merge settings', exact: true }).click()
  await page.getByText('Operator qualification evidence', { exact: true }).click()
  await page.getByRole('checkbox', { name: 'Queue execution gate' }).check()
  await page.getByRole('textbox', { name: 'CI configuration SHA-256' }).fill('a'.repeat(64))
  await page.getByText('Check publishers', { exact: true }).click()
  await page.getByRole('textbox', { name: 'Publisher 1' }).fill('12345')
  await page.getByRole('button', { name: 'Save merge settings' }).click()
  await expect(page.getByRole('alert')).toContainText('protected blocking reforge/merge-policy job')
})
