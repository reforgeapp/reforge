import { test, expect, type Page } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'
const id = 'conn-agent-1'
const connection = { id, org_id: org, kind: 'agent', provider: 'codex', name: 'Codex runtime', endpoint: 'https://codex.example', state: 'disabled', reason: 'Connect a documented official runtime', version: 1, credential_version: 1, settings: { auth_kind: 'official_runtime', billing_route: 'subscription', runtime_version: '0.150.1', namespace: 'workspace-1', model: 'gpt-5' }, capabilities: {}, verified_at: null }
const runtimeCapability = { state: 'unsupported', scope: 'configured official runtime', reason: 'managed app-server bridge implemented; no deployment certified', source: 'T16', version: '', last_checked: '2026-09-21T00:00:00Z' }

async function mock(page: Page) {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }], memberships: [{ org_id: org, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { name: 'Reforge', version: 'test', edition: 'self-hosted', development: true, fixture_auth: true } }))
  await page.route(`**/api/v1/orgs/${org}/connections**`, route => route.fulfill({ json: { items: [connection], complete: true } }))
  await page.route(`**/api/v1/orgs/${org}/connections/${id}`, route => route.fulfill({ json: connection }))
  await page.route(`**/api/v1/orgs/${org}/custom-profiles**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.goto(`/org/${org}/connections`)
}

test('agent runtime shows uncertified status until dated evidence is recorded', async ({ page }) => {
  let saved: Record<string, unknown> | undefined
  await mock(page)
  await page.route(`**/api/v1/orgs/${org}/connections/${id}/agent-qualification`, async route => {
    if (route.request().method() === 'PUT') { saved = route.request().postDataJSON() as Record<string, unknown> }
    const capabilities = saved ? { runtime: runtimeCapability, entitlement: { ...runtimeCapability, state: 'supported', reason: 'verified by dated deployment evidence' } } : { runtime: runtimeCapability }
    return route.fulfill({ json: { qualification: saved ? { ...saved, binding: {} } : undefined, capabilities } })
  })
  await page.getByRole('button', { name: 'Models & agents' }).click()
  await page.getByRole('button', { name: 'Codex runtime' }).click()
  await page.getByRole('button', { name: 'Qualification', exact: true }).click()
  const panel = page.getByRole('group', { name: 'Agent runtime qualification' })
  await expect(panel.getByText(/no deployment certified/)).toBeVisible()
  await panel.getByText('Record qualification evidence').click()
  await panel.getByLabel('Evidence reference').fill('00000000-0000-4000-8000-0000000000ee')
  await panel.getByLabel('Verified at').fill('2026-09-21T10:00')
  await panel.getByLabel('Expires at').fill('2026-12-21T10:00')
  await panel.getByLabel('Account entitlement').check()
  await panel.getByRole('button', { name: 'Save qualification' }).click()
  await expect.poll(() => saved?.evidence_id).toBe('00000000-0000-4000-8000-0000000000ee')
  expect(saved?.entitlement).toBe(true)
  await expect(panel.getByText(/verified by dated deployment evidence/)).toBeVisible()
})
