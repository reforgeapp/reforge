import { test, expect, type Page } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'
const repo = '00000000-0000-4000-8000-000000000011'
const base = `/api/v1/orgs/${org}`
const policy = { schema: 'maintenance/v1', allow: { recipes: ['repair'], models: ['local'], routes: ['default'], merge_methods: [], environments: null, workflows: null }, deny: ['read'], forbidden_paths: ['.env'], limits: { budget: 0, concurrency: 0 }, required: [{ id: 'approval' }], defaults: { model: 'local' }, paused: false }
const version = { id: 'version-1', scope: { kind: 'organisation', id: org }, policy, hash: 'hash-1', actor_id: 'actor-1', reason: 'baseline', created_at: '2026-09-21T00:00:00Z' }

async function fixture(page: Page, role = 'owner') {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture' }], memberships: [{ org_id: org, role, team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route(`**${base}/repositories**`, route => route.fulfill({ json: { items: [{ id: repo, name: 'payments' }], complete: true } }))
  await page.route(`**${base}/teams**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route(`**${base}/policies/effective**`, route => route.fulfill({ json: { hash: 'effective-1', layers: [], paused: false, scope_paused: false, policy } }))
  await page.route(`**${base}/policies/versions?**`, route => route.fulfill({ json: { items: [version], complete: true } }))
  await page.route(`**${base}/policies/versions/${version.id}`, route => route.fulfill({ json: version }))
  await page.goto(`/org/${org}/policies`)
  await page.getByLabel('Policy repository').selectOption(repo)
  await page.getByRole('button', { name: version.id }).click()
}

test('policy tabs show only selected editor section', async ({ page }) => {
  await fixture(page)
  await expect(page.getByRole('tabpanel', { name: 'Scope' })).toBeVisible()
  await page.getByRole('tab', { name: 'Models & spend' }).click()
  await expect(page.getByRole('tabpanel', { name: 'Models & spend' })).toBeVisible()
  await expect(page.getByLabel('Budget', { exact: true })).toBeVisible()
  await expect(page.getByLabel('Schema')).toBeHidden()
})

test('zero policy limits survive save payload', async ({ page }) => {
  await fixture(page)
  let payload: any
  await page.route(`**${base}/policies/versions`, async route => { payload = await route.request().postDataJSON(); await route.fulfill({ json: version }) })
  await page.getByLabel('Reason').fill('zero caps')
  await page.getByRole('tab', { name: 'Models & spend' }).click()
  await expect(page.getByLabel('Budget', { exact: true })).toHaveValue('0')
  await page.getByRole('tab', { name: 'Scope' }).click()
  await page.getByRole('button', { name: 'Save immutable version' }).click()
  await expect.poll(() => payload?.policy?.limits).toEqual({ budget: 0, concurrency: 0 })
})

test('draft preset preserves policy evidence and waits for explicit save', async ({ page }) => {
  await fixture(page)
  let payload: any
  await page.route(`**${base}/policies/versions`, async route => { payload = await route.request().postDataJSON(); await route.fulfill({ json: version }) })
  await page.getByRole('button', { name: 'Apply preset' }).click()
  await expect(page.getByText(/Draft allows: none · blocks: read, repair, publish, merge, deploy, recover/)).toBeVisible()
  expect(payload).toBeUndefined()
  await page.getByLabel('Draft preset').selectOption('propose')
  await page.getByRole('button', { name: 'Apply preset' }).click()
  await expect(page.getByText(/Draft allows: repair, publish · blocks: read, merge, deploy, recover/)).toBeVisible()
  await page.getByLabel('Draft preset').selectOption('deliver')
  await page.getByRole('button', { name: 'Apply preset' }).click()
  await page.getByRole('button', { name: 'Save immutable version' }).click()
  await expect.poll(() => payload?.policy).toMatchObject({ deny: ['read', 'recover'], allow: { environments: [], workflows: [] }, forbidden_paths: ['.env'], limits: { budget: 0, concurrency: 0 }, required: [{ id: 'approval' }], defaults: { model: 'local' } })
})

test('viewer cannot apply a draft preset', async ({ page }) => {
  await fixture(page, 'viewer')
  await expect(page.getByRole('button', { name: 'Apply preset' })).toBeDisabled()
})

test('dirty draft cannot simulate or apply a preset', async ({ page }) => { await fixture(page); await page.getByText('Advanced policy JSON import/export').click(); await page.getByLabel('Raw policy JSON').fill('{'); await expect(page.getByRole('button', { name: 'Apply preset' })).toBeDisabled(); await page.getByRole('button', { name: 'Open simulation' }).click(); await expect(page.getByRole('button', { name: 'Simulate candidate rollout' })).toBeDisabled() })
test('deliver preset preserves existing environment and workflow allowlists', async ({ page }) => {
  await fixture(page)
  let payload: any
  await page.route(`**${base}/policies/versions`, async route => { payload = await route.request().postDataJSON(); await route.fulfill({ json: version }) })
  await page.getByRole('tab', { name: 'Deploy', exact: true }).click()
  await page.getByLabel('Allowed environments').selectOption('explicit')
  await page.getByLabel('environments values').fill('production')
  await page.getByLabel('Allowed workflows').selectOption('explicit')
  await page.getByLabel('workflows values').fill('release')
  await page.getByRole('tab', { name: 'Scope', exact: true }).click()
  await page.getByLabel('Draft preset').selectOption('deliver')
  await page.getByRole('button', { name: 'Apply preset' }).click()
  await page.getByRole('button', { name: 'Save immutable version' }).click()
  await expect.poll(() => payload?.policy?.allow).toMatchObject({ environments: ['production'], workflows: ['release'] })
})
test('organisation admin cannot write organisation policy', async ({ page }) => { await fixture(page, 'admin'); await expect(page.getByRole('button', { name: 'Save immutable version' })).toBeDisabled() })
test('invalid advanced JSON reports editor error without crashing', async ({ page }) => { await fixture(page); await page.getByText('Advanced policy JSON import/export').click(); await page.getByLabel('Raw policy JSON').fill('{'); await page.getByRole('button', { name: 'Save immutable version' }).click(); await expect(page.getByRole('alert')).toContainText('Policy JSON must be valid JSON.') })
