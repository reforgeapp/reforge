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
  await page.route(`**/api/v1/orgs/${org}/audit-events**`, route => route.fulfill({ json: { items: [{ id: 'event-1', actor_id: 'user-1', action: 'finding.dismiss', object_id: 'finding-1', request_id: 'request-1', repository_id: 'repo-1', created_at: '2026-09-22T00:00:00Z', data: {} }], complete: true } }))
  await page.route(`**/api/v1/orgs/${org}/repositories**`, route => route.fulfill({ json: { items: [{ id: 'repo-1', name: 'payments' }], complete: true } }))
  await page.goto(`/org/${org}/audit`)
  await page.getByRole('row', { name: /finding\.dismiss/ }).getByRole('button').click()
  await expect(page.getByRole('button', { name: 'Back to list' })).toBeVisible()
})
