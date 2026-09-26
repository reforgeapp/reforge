import { test, expect, type Page } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'
const repo = '00000000-0000-4000-8000-000000000011'
const base = `/api/v1/orgs/${org}`
const csrf = 'csrf-1'

async function session(page: Page, role = 'owner') {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }], memberships: [{ org_id: org, role, team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: csrf } }))
  await page.route(`**${base}/teams**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { development: true, fixture_auth: true } }))
}

const repositories = { items: [{ id: repo, name: 'payments', full_name: 'org/payments' }], complete: true }
const amount = { micro_usd: 100, tokens: 20, milliseconds: 30, requests: 1, concurrency: 0 }
const usageSummary = { records: 2, settled: 1, unknown: 1, reserved: 0, dispatched: 0, cancelled: 0, estimated_cost_micro_usd: 100, known_tokens: 20, unknown_maximum: { ...amount, micro_usd: 900 }, held: { ...amount, micro_usd: 900 } }
const routeInfo = { connection_id: 'conn-1', model: 'local', name: 'default', mode: 'priced', pricing_version: 'v1', input_micro_usd_per_million: 0, output_micro_usd_per_million: 0, request_micro_usd: 0, max_input_tokens: 1000, max_output_tokens: 1000, max_milliseconds: 1000, max_requests: 10, qualified: true, qualification_ref: 'local', paused: false, version: 1 }
const usageRows = { items: [{ reservation: { id: 'settled-1', connection_id: 'conn-1', maximum: amount, actual: amount, state: 'settled', created_at: '2026-09-21T00:00:00Z', quote: { model: 'local', route: 'default', input_tokens: 20, max_output_tokens: 20 }, lease: { repository_id: repo, task_id: 'task-1', operation_id: 'op-1' }, route: routeInfo }, provider: 'local', recipe: 'repair', repository_name: 'payments' }, { reservation: { id: 'unknown-1', connection_id: 'conn-1', maximum: { ...amount, micro_usd: 900 }, state: 'unknown', created_at: '2026-09-21T00:00:00Z', quote: { model: 'local', route: 'default', input_tokens: 20, max_output_tokens: 20 }, lease: { repository_id: repo, task_id: 'task-2', operation_id: 'op-2' }, route: routeInfo }, provider: 'local', recipe: 'repair', repository_name: 'payments' }], complete: true }

async function usageFixture(page: Page, role = 'owner') {
  await session(page, role)
  await page.route(`**${base}/repositories**`, route => route.fulfill({ json: repositories }))
  await page.route(`**${base}/usage/summary**`, route => {
    if (new URL(route.request().url()).searchParams.get('repository_id') === 'bad') return route.fulfill({ status: 400, json: { message: 'repository_id must be a UUID' } })
    return route.fulfill({ json: usageSummary })
  })
  await page.route(`**${base}/usage?**`, route => {
    if (new URL(route.request().url()).searchParams.get('repository_id') === 'bad') return route.fulfill({ status: 400, json: { message: 'repository_id must be a UUID' } })
    return route.fulfill({ json: usageRows })
  })
  await page.route(`**${base}/budget-routes/**`, route => route.fulfill({ json: routeInfo }))
  await page.route(`**${base}/budgets/organisation/${org}`, route => route.fulfill({ json: { scope: { kind: 'organisation', id: org }, period: 'daily', caps: { tokens: 10 }, paused: false, version: 3, held: amount, spent: amount } }))
  await page.route(`**${base}/budgets/repository/${repo}`, route => route.fulfill({ json: { scope: { kind: 'repository', id: repo }, period: 'daily', caps: {}, paused: false, version: 7, held: amount, spent: amount } }))
  await page.goto(`/org/${org}/usage`)
}

test('audit filters submit by keyboard, render escaped JSON, and export exact loaded page', async ({ page }) => {
  await session(page)
  await page.route(`**${base}/repositories**`, route => route.fulfill({ json: repositories }))
  let exportURL = ''
  await page.route(`**${base}/audit-events/export**`, async route => { exportURL = route.request().url(); await route.fulfill({ contentType: 'application/x-ndjson', headers: { 'X-Export-Complete': 'false', 'X-Next-Cursor': 'cursor-3' }, body: '{"id":"evt-2"}\n' }) })
  await page.route(`**${base}/audit-events**`, route => {
    const url = new URL(route.request().url())
    if (url.pathname.endsWith('/export')) return route.fallback()
    const cursor = url.searchParams.get('cursor')
    return route.fulfill({ json: cursor === 'cursor-2' ? { items: [{ id: 'evt-2', actor_id: 'actor-2', action: 'policy.activate', object_id: 'version-2', request_id: 'req-2', repository_id: repo, created_at: '2026-09-21T00:01:00Z', data: { html: '<script>alert(1)</script>' } }], complete: true } : { items: [{ id: 'evt-1', actor_id: 'actor-1', action: 'policy.save', object_id: 'version-1', request_id: 'req-1', created_at: '2026-09-21T00:00:00Z', data: { note: 'saved' } }], complete: false, next_cursor: 'cursor-2' } })
  })
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto(`/org/${org}/audit`)
  await page.getByLabel('Action filter').fill('policy.activate')
  await page.getByLabel('Action filter').press('Enter')
  await expect(page).toHaveURL(/\/audit(?:\?.*)?$/)
  await page.getByRole('button', { name: 'Load more events' }).click()
  await page.getByLabel('Audit export page').selectOption('1')
  await page.getByRole('button', { name: 'Export server page' }).click()
  await expect.poll(() => exportURL).toContain('cursor=cursor-2')
  await expect(page.getByText(/^Page 2 exported/)).toContainText('Page 2')
  await page.getByRole('row').filter({ hasText: 'Policy Activate' }).getByRole('button').click()
  await page.getByText('Event data', { exact: true }).click()
  await expect(page.locator('pre').filter({ hasText: '<script>alert(1)</script>' })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})

test('usage named selectors separate unknown holds from settled estimates', async ({ page }) => {
  await usageFixture(page)
  await page.getByRole('combobox', { name: 'Repository', exact: true }).selectOption(repo)
  await page.getByRole('button', { name: 'Apply filters' }).click()
  await expect(page.getByText('Unknown usage maximum')).toBeVisible()
  await expect(page.getByText(/Held maximum/)).toBeVisible()
  await expect(page.getByRole('cell', { name: /20 tokens/ }).first()).toBeVisible()
  await expect(page.locator('dd').filter({ hasText: 'USD (estimate)' }).first()).toBeVisible()
})

test('usage budget scope change discards old draft and saves with fresh version', async ({ page }) => {
  await usageFixture(page)
  await page.getByRole('tab', { name: 'Budgets' }).click()
  await page.route(`**${base}/budgets/organisation/${org}`, route => route.fulfill({ json: { scope: { kind: 'organisation', id: org }, period: 'daily', caps: { tokens: 10 }, paused: false, version: 3, held: amount, spent: amount } }))
  await page.route(`**${base}/budgets/repository/${repo}`, route => route.fulfill({ json: { scope: { kind: 'repository', id: repo }, period: 'daily', caps: {}, paused: false, version: 7, held: amount, spent: amount } }))
  let saveIfMatch = ''
  await page.route(`**${base}/budgets/repository/${repo}`, async route => { if (route.request().method() === 'PUT') { saveIfMatch = route.request().headers()['if-match'] ?? ''; await route.fulfill({ json: { scope: { kind: 'repository', id: repo }, period: 'daily', caps: { tokens: 11 }, paused: false, version: 8, held: amount, spent: amount } }) } else await route.fallback() })
  await page.getByRole('combobox', { name: 'Scope', exact: true }).selectOption('repository')
  await page.getByRole('combobox', { name: 'Named scope', exact: true }).selectOption(repo)
  await expect(page.getByText(/Version 7/)).toBeVisible()
  await expect(page.getByLabel('tokens')).toHaveValue('')
  await page.getByLabel('tokens').fill('11')
  await page.getByRole('button', { name: 'Save budget' }).click()
  await expect.poll(() => saveIfMatch).toBe('"7"')
})

test('viewer cannot edit usage budget', async ({ page }) => {
  await usageFixture(page, 'viewer')
  await page.getByRole('tab', { name: 'Budgets' }).click()
  await page.route(`**${base}/budgets/organisation/${org}`, route => route.fulfill({ json: { scope: { kind: 'organisation', id: org }, period: 'daily', caps: { tokens: 10 }, paused: false, version: 3, held: amount, spent: amount } }))
  await expect(page.getByLabel('tokens')).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Save budget' })).toBeDisabled()
})

const policy = { schema: 'maintenance/v1', allow: { recipes: ['repair'], models: ['local'], routes: ['default'], merge_methods: [], environments: [], workflows: [] }, deny: [], forbidden_paths: [], limits: {}, required: [], defaults: {}, paused: false }
const version = { id: 'version-1', scope: { kind: 'organisation', id: org }, policy, hash: 'hash-1', actor_id: 'actor-1', reason: 'baseline', created_at: '2026-09-21T00:00:00Z' }

async function policyFixture(page: Page, role = 'owner') {
  await session(page, role)
  await page.route(`**${base}/repositories**`, route => route.fulfill({ json: repositories }))
  await page.route(`**${base}/policies/effective**`, route => route.fulfill({ json: { hash: 'effective-1', layers: [], repository_id: repo, paused: false, scope_paused: false, policy } }))
  await page.route(`**${base}/policies/versions?**`, route => route.fulfill({ json: { items: [version], complete: true } }))
  await page.route(`**${base}/policies/versions/${version.id}`, route => route.fulfill({ json: version }))
  await page.goto(`/org/${org}/policies`)
  await page.getByLabel('Policy repository').selectOption(repo)
  await page.getByRole('button', { name: version.id }).click()
}

test('policy dirty draft blocks simulation, then save/simulate/activate uses hash and CAS zero', async ({ page }) => {
  await policyFixture(page)
  await page.getByText('Advanced policy JSON', { exact: true }).click()
  await page.getByLabel('Raw policy JSON').fill(JSON.stringify({ ...policy, paused: true }))
  await page.getByRole('tab', { name: 'Review' }).click()
  await page.getByRole('button', { name: 'Open simulation' }).click()
  await expect(page.getByRole('button', { name: 'Simulate candidate rollout' })).toBeDisabled()
  await page.getByRole('tab', { name: 'Scope' }).click()
  await page.getByLabel('Raw policy JSON').fill('')
  await page.getByRole('tab', { name: 'Review' }).click()
  await page.getByLabel('Reason').fill('updated')
  let saved = false
  await page.route(`**${base}/policies/versions`, async route => { saved = true; await route.fulfill({ json: { ...version, id: 'version-2', hash: 'hash-2', reason: 'updated' } }) })
  await page.route(`**${base}/policies/versions/version-2`, route => route.fulfill({ json: { ...version, id: 'version-2', hash: 'hash-2', reason: 'updated' } }))
  await page.getByRole('button', { name: 'Save immutable version' }).click()
  await expect.poll(() => saved).toBe(true)
  await page.route(`**${base}/policies/versions/version-2/simulate`, route => route.fulfill({ json: { hash: 'simulation-2', resolved: { hash: 'effective-1', paused: false, scope_paused: false }, decision: { outcome: 'allow', blockers: [], required_actions: [] } } }))
  let activationIfMatch = ''
  await page.route(`**${base}/policies/versions/version-2/activate`, async route => { activationIfMatch = route.request().headers()['if-match'] ?? ''; await route.fulfill({ json: { version: 1 } }) })
  await page.getByRole('button', { name: 'Open simulation' }).click()
  await page.getByRole('button', { name: 'Simulate candidate rollout' }).click()
  await page.getByRole('button', { name: 'Activate exact simulation' }).click()
  await expect.poll(() => activationIfMatch).toBe('"0"')
})

test('organisation policy simulation excludes the repository primary team', async ({ page }) => {
  await policyFixture(page)
  const team = '00000000-0000-4000-8000-000000000012'
  await page.route(`**${base}/teams**`, route => route.fulfill({ json: { items: [{ id: team, name: 'Payments', repository_ids: [repo] }], complete: true } }))
  await page.route(`**${base}/policies/effective**`, route => route.fulfill({ json: { hash: 'effective-1', layers: [], primary_team_id: team, repository_id: repo, paused: false, scope_paused: false, policy } }))
  await page.reload()
  await page.getByRole('button', { name: version.id }).click()
  await page.getByRole('tab', { name: 'Review' }).click()
  let primary: string | undefined
  await page.route(`**${base}/policies/versions/${version.id}/simulate`, async route => {
    primary = (await route.request().postDataJSON()).primary_team_id
    await route.fulfill({ json: { hash: 'simulation-team', resolved: { hash: 'next', paused: false, scope_paused: false, problems: [] }, decision: { outcome: 'allow', blockers: [] } } })
  })
  await page.getByRole('button', { name: 'Open simulation' }).click()
  await page.getByRole('button', { name: 'Simulate candidate rollout' }).click()
  await expect.poll(() => primary).toBe('')
  await page.getByRole('group', { name: 'Policy scope' }).getByRole('button', { name: 'Repository' }).click()
  await page.getByRole('tab', { name: 'Scope' }).click()
  await expect(page.getByRole('combobox', { name: 'Primary team' })).toHaveValue(team)
})
