import { test, expect, type Page } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'
const finding = { id: 'finding-1', org_id: org, repository_id: 'repo-1', source: 'advisory', source_id: 'CVE-TEST', category: 'dependency', severity: 'high', title: 'Upgrade dependency', evidence: { provenance: 'gitea', connection_id: 'connection-1', connection_version: 1, config_version: 1, head_sha: 'a'.repeat(40), target_sha: 'b'.repeat(40), target_branch: 'main', checks: [], dependencies: [], ownership: 'bot-owned', complete: true, blockers: [], merge_blockers: [] }, fingerprint: 'fingerprint', evidence_digest: 'digest', state: 'open', reason: 'Needs review', version: 1, first_seen: new Date().toISOString(), last_seen: new Date().toISOString() }

async function mock(page: Page) {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }], memberships: [{ org_id: org, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { name: 'Reforge', version: 'test', edition: 'self-hosted', development: true, fixture_auth: true } }))
  await page.route(`**/api/v1/orgs/${org}/findings**`, route => route.fulfill({ json: { items: [finding], complete: true } }))
  await page.route(`**/api/v1/orgs/${org}/findings/finding-1`, route => route.fulfill({ json: finding }))
  await page.route(`**/api/v1/orgs/${org}/repositories**`, route => route.fulfill({ json: { items: [{ id: 'repo-1', name: 'payments', provider: 'gitea', team_ids: [], accessible: true }], complete: true } }))
}

test('findings detail closes with one X, restores row focus and keeps filters', async ({ page }) => {
  await mock(page)
  await page.goto(`/org/${org}/findings?finding_state=open&q=upgrade`)
  const row = page.locator('.split-list .link-button').first()
  await expect(row).toBeVisible()
  await row.focus()
  await page.keyboard.press('Enter')
  await expect(page).toHaveURL(/finding=finding-1/)
  await expect(page.getByRole('button', { name: 'Close finding details' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Back to list' })).toHaveCount(0)
  await page.getByRole('button', { name: 'Close finding details' }).focus()
  await page.keyboard.press('Enter')
  await expect(page).not.toHaveURL(/finding=/)
  const params = new URL(page.url()).searchParams
  expect(params.get('finding_state')).toBe('open')
  expect(params.get('q')).toBe('upgrade')
  await expect(row).toBeFocused()
})

test('findings close control is usable at a narrow width', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await mock(page)
  await page.goto(`/org/${org}/findings?finding=finding-1`)
  const close = page.getByRole('button', { name: 'Close finding details' })
  await expect(close).toBeVisible()
  await expect(page.getByRole('button', { name: 'Back to list' })).toHaveCount(0)
  await close.click()
  await expect(page).not.toHaveURL(/finding=/)
})

test('other routes keep the back-to-list control', async ({ page }) => {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }], memberships: [{ org_id: org, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { name: 'Reforge', version: 'test', edition: 'self-hosted', development: true, fixture_auth: true } }))
  const task = { id: 'task-1', org_id: org, repository_id: 'repo-1', operation_id: 'op-1', recipe: 'go', recipe_version: '1', target_branch: 'main', model_route: '', policy_hash: 'policy', starting_policy_hash: 'policy', state: 'queued', reason: '', version: 1, cancel_version: 1, max_attempts: 1, created_at: '2026-09-22T00:00:00Z' }
  await page.route(`**/api/v1/orgs/${org}/tasks**`, route => route.fulfill({ json: { items: [task], complete: true } }))
  await page.route(`**/api/v1/orgs/${org}/tasks/task-1`, route => route.fulfill({ json: task }))
  await page.route(`**/api/v1/orgs/${org}/repair-runs/task-1`, route => route.fulfill({ status: 404, json: { error: 'not found' } }))
  await page.route(`**/api/v1/orgs/${org}/events**`, route => route.fulfill({ status: 200, headers: { 'content-type': 'text/event-stream' }, body: ': keepalive\n\n' }))
  await page.route(`**/api/v1/orgs/${org}/repositories**`, route => route.fulfill({ json: { items: [{ id: 'repo-1', name: 'payments' }], complete: true } }))
  await page.goto(`/org/${org}/runs`)
  await page.getByRole('row', { name: /task-1/ }).getByRole('button').first().click()
  await expect(page.getByRole('button', { name: 'Back to list' })).toBeVisible()
})
