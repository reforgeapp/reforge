import { test, expect, type Page } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'
const digest = `sha256:${'a'.repeat(64)}`
const profile = { id: 'profile-1', name: 'reviewer', version: 1, image_digest: digest, executable: '/bin/review', argv: ['--mode', 'review'], protocol_version: 1, max_wall_seconds: 30, max_output_bytes: 1048576, max_turns: 4, concurrency: 1, approval_evidence: '', created_at: '2026-09-21T00:00:00Z' }

async function mock(page: Page, role = 'owner') {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }], memberships: [{ org_id: org, role, team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { name: 'Reforge', version: 'test', edition: 'self-hosted', development: true, fixture_auth: true } }))
  await page.route(`**/api/v1/orgs/${org}/connections**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route(`**/api/v1/orgs/${org}/custom-profiles**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.goto(`/org/${org}/connections`)
}

test('draft profile requires evidence before approval and records revocation', async ({ page }) => {
  let created: Record<string, unknown> | undefined
  let approved: { evidence: string; ifMatch: string } | undefined
  await mock(page)
  let current: Record<string, unknown> = { ...profile }
  await page.route(`**/api/v1/orgs/${org}/custom-profiles**`, async route => {
    if (route.request().method() === 'POST') { created = route.request().postDataJSON() as Record<string, unknown>; return route.fulfill({ status: 201, headers: { ETag: '"1"' }, json: current }) }
    return route.fulfill({ json: { items: [current], complete: true } })
  })
  await page.route(`**/api/v1/orgs/${org}/custom-profiles/profile-1/approve`, async route => { approved = { evidence: (route.request().postDataJSON() as { evidence: string }).evidence, ifMatch: route.request().headers()['if-match'] ?? '' }; current = { ...current, version: 2, approval_evidence: approved.evidence, approved_at: '2026-09-21T00:00:00Z' }; return route.fulfill({ json: current }) })
  await page.route(`**/api/v1/orgs/${org}/custom-profiles/profile-1/revoke`, async route => { current = { ...current, version: 3, revoked_at: '2026-09-21T00:00:00Z' }; return route.fulfill({ json: current }) })

  await page.getByRole('button', { name: 'Models & agents' }).click()
  await page.getByText('Custom command profiles', { exact: true }).click()
  const panel = page.getByRole('region', { name: 'Custom command profiles' })
  await panel.getByRole('button', { name: 'Create draft profile' }).click()
  await panel.getByLabel('Name').fill('reviewer')
  await panel.getByLabel('Image digest').fill(digest)
  await panel.getByLabel('Executable').fill('/bin/review')
  await panel.getByLabel('Fixed argv (one per line)').fill('--mode\nreview')
  await panel.getByRole('button', { name: 'Save draft profile' }).click()
  await expect.poll(() => created?.image_digest).toBe(digest)
  expect((created?.argv as string[]).join(' ')).toBe('--mode review')
  await panel.getByRole('button', { name: 'reviewer' }).click()

  const evidence = panel.getByLabel('Approval evidence for reviewer')
  await expect(panel.getByRole('button', { name: 'Approve profile', exact: true })).toBeDisabled()
  await evidence.fill('ticket-7')
  await panel.getByRole('button', { name: 'Approve profile', exact: true }).click()
  await expect.poll(() => approved?.evidence).toBe('ticket-7')
  expect(approved?.ifMatch).toBe('"1"')
  await panel.getByRole('button', { name: 'Revoke' }).click()
  await expect(panel.getByText(/revoked/i).last()).toBeVisible()
})

test('a viewer cannot create or approve profiles', async ({ page }) => {
  await mock(page, 'viewer')
  await page.getByRole('button', { name: 'Models & agents' }).click()
  await page.getByText('Custom command profiles', { exact: true }).click()
  const panel = page.getByRole('region', { name: 'Custom command profiles' })
  await expect(panel.getByText('New profile')).toHaveCount(0)
  await expect(panel.getByRole('button', { name: 'Approve' })).toHaveCount(0)
  await expect(panel.getByText(/Unapproved profiles stay disabled/)).toBeVisible()
})
