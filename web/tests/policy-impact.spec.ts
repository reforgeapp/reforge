import { test, expect, type Page } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'
const repoA = '00000000-0000-4000-8000-000000000011'
const repoB = '00000000-0000-4000-8000-000000000012'
const base = `/api/v1/orgs/${org}`
const policy = { schema: 'maintenance/v1', allow: { recipes: ['repair'], models: ['local'], routes: ['default'], merge_methods: [], environments: null, workflows: null }, deny: [], forbidden_paths: [], limits: {}, required: [], defaults: { model: 'local', route: 'default' }, paused: false }
const version = { id: 'version-impact', scope: { kind: 'organisation', id: org }, policy, hash: 'candidate-hash', actor_id: 'actor-1', reason: 'impact candidate', created_at: '2026-09-21T00:00:00Z' }

async function fixture(page: Page, options: { paginated?: boolean; repeated?: boolean; missingCursor?: boolean; sourceFailure?: string; effectiveFailure?: string; emptySource?: string; noRepositories?: boolean } = {}) {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture' }], memberships: [{ org_id: org, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { development: true, fixture_auth: true } }))
  await page.route(`**${base}/repositories**`, route => {
    const pathname = new URL(route.request().url()).pathname
    if (pathname.endsWith(`/repositories/${repoA}`)) return route.fulfill({ json: { id: repoA, name: 'payments' } })
    if (options.noRepositories) return route.fulfill({ json: { items: [], complete: true } })
    const cursor = new URL(route.request().url()).searchParams.get('cursor')
    if (options.paginated && !cursor) return route.fulfill({ json: { items: [{ id: repoA, name: 'payments' }], complete: false, ...(options.missingCursor ? {} : { next_cursor: 'repo-next' }) } })
    if (options.repeated && cursor === 'repo-next') return route.fulfill({ json: { items: [{ id: repoB, name: 'catalog' }], complete: false, next_cursor: 'repo-next' } })
    return route.fulfill({ json: { items: [{ id: repoA, name: 'payments' }, { id: repoB, name: 'catalog' }], complete: true } })
  })
  await page.route(`**${base}/policies/effective**`, route => { const id = new URL(route.request().url()).searchParams.get('repository_id'); if (id === options.effectiveFailure) return route.fulfill({ status: 503, json: { message: 'effective unavailable' } }); return route.fulfill({ json: { hash: `effective-${id}`, layers: [], repository_id: id, primary_team_id: 'team-1', paused: false, scope_paused: false, policy } }) })
  await page.route(`**${base}/policies/versions?**`, route => route.fulfill({ json: { items: [version], complete: true } }))
  await page.route(`**${base}/policies/versions/${version.id}`, route => route.fulfill({ json: version }))
  await page.route(`**${base}/teams**`, route => route.fulfill({ json: { items: [{ id: 'team-1', name: 'Platform', repository_ids: [repoA] }], complete: true } }))
  await page.route(`**${base}/findings**`, route => { const id = new URL(route.request().url()).searchParams.get('repository_id'); if (id === options.sourceFailure) return route.fulfill({ status: 503, json: { message: 'source unavailable' } }); if (id === options.emptySource) return route.fulfill({ json: { items: [], complete: true } }); return route.fulfill({ json: { items: [{ id: `finding-${id}`, repository_id: id, title: `${id} finding`, version: 2, state: 'open', last_seen: new Date().toISOString(), evidence: { head_sha: id === repoA ? 'head-a' : 'head-b', target_sha: id === repoA ? 'target-a' : 'target-b', complete: true, blockers: [] } }], complete: true } }) })
  await page.goto(`/org/${org}/policies`)
  if (!options.noRepositories) await page.getByLabel('Policy repository').selectOption(repoA)
  await page.getByRole('button', { name: version.id }).click()
  await page.getByRole('tab', { name: 'Review' }).click()
  await page.getByRole('button', { name: 'Open simulation' }).click()
}

test('runs sequential repository simulations with source bindings and real effective hashes', async ({ page }) => {
  await fixture(page)
  const requests: Array<Record<string, unknown>> = []
  let inflight = 0
  let maxInflight = 0
  await page.route(`**${base}/policies/versions/${version.id}/simulate`, async route => {
    inflight += 1; maxInflight = Math.max(maxInflight, inflight)
    const body = route.request().postDataJSON() as Record<string, unknown>
    requests.push(body)
    const current = (body.input as { current: { head: string; target: string; policy_hash: string; tested: string; provider_rules: string; source_sha: string; artifact: string } }).current
    await route.fulfill({ json: { hash: `proof-${body.repository_id}`, resolved: { hash: current.policy_hash, layers: [], repository_id: body.repository_id, paused: false, scope_paused: false, problems: [], missing_defaults: [], policy }, decision: { outcome: 'allow', blockers: [], bindings: [], evidence_references: [], starting_policy_hash: current.policy_hash } } }); inflight -= 1
  })
  await page.getByRole('button', { name: 'Run impact preview' }).click()
  await expect.poll(() => requests.length).toBe(2)
  expect(requests.map(request => request.repository_id)).toEqual([repoA, repoB])
  const first = requests[0].input as { current: { head: string; target: string; policy_hash: string; tested: string; provider_rules: string; source_sha: string; artifact: string }; evidence: unknown[]; starting_policy_hash: string }
  expect(first.current).toMatchObject({ head: 'head-a', target: 'target-a', policy_hash: `effective-${repoA}`, tested: '', provider_rules: '', source_sha: '', artifact: '' })
  expect(first.evidence).toEqual([])
  expect(first.starting_policy_hash).toBe(`effective-${repoA}`)
  expect(maxInflight).toBe(1)
  await expect(page.getByText('No allow, deny, or limit changes.').first()).toBeVisible()
})

test('keeps pagination partial and rejects repeated repository cursors', async ({ page }) => {
  await fixture(page, { paginated: true, repeated: true })
  await page.route(`**${base}/policies/versions/${version.id}/simulate`, route => route.fulfill({ json: { hash: 'proof', resolved: { hash: 'effective', layers: [], repository_id: repoA, paused: false, scope_paused: false, problems: [], missing_defaults: [], policy }, decision: { outcome: 'deny', blockers: ['fixture'], bindings: [], evidence_references: [], starting_policy_hash: 'effective' } } }))
  const impact = page.getByRole('region', { name: 'Policy impact preview' })
  await impact.getByRole('button', { name: 'Run impact preview' }).click()
  await expect(impact.getByText(/Coverage: 1 repositories, partial/)).toBeVisible()
  await impact.getByRole('button', { name: 'Load more repositories' }).click()
  await expect(page.getByText('catalog')).toBeVisible()
  await impact.getByRole('button', { name: 'Load more repositories' }).click()
  await expect(impact.getByRole('alert')).toContainText('cursor repeated')
})

test('discards late simulation response after navigation', async ({ page }) => {
  let release!: () => void
  const delayed = new Promise<void>(resolve => { release = resolve })
  await fixture(page)
  let posts = 0
  await page.route(`**${base}/policies/versions/${version.id}/simulate`, async route => { posts += 1; await delayed; await route.fulfill({ json: { hash: 'late', resolved: { hash: version.hash, layers: [], repository_id: repoA, paused: false, scope_paused: false, problems: [], missing_defaults: [], policy }, decision: { outcome: 'allow', blockers: [], bindings: [], evidence_references: [], starting_policy_hash: `effective-${repoA}` } } }) })
  await page.getByRole('button', { name: 'Run impact preview' }).click()
  await page.goto(`/org/${org}/overview`)
  release()
  await expect(page.getByRole('region', { name: 'Policy impact preview' })).toHaveCount(0)
  expect(posts).toBeLessThanOrEqual(1)
})

test('retains valid repositories when first page cursor is missing', async ({ page }) => {
  await fixture(page, { paginated: true, missingCursor: true })
  await page.getByRole('region', { name: 'Policy impact preview' }).getByRole('button', { name: 'Run impact preview' }).click()
  await expect(page.getByRole('region', { name: 'Policy impact preview' }).getByRole('cell', { name: 'payments' })).toBeVisible()
  await expect(page.getByRole('alert')).toContainText('pagination stopped')
  await expect(page.getByRole('button', { name: 'Load more repositories' })).toHaveCount(0)
})

test('continues other repositories after source failure and reports partial coverage', async ({ page }) => {
  await fixture(page, { sourceFailure: repoA })
  await page.getByRole('region', { name: 'Policy impact preview' }).getByRole('button', { name: 'Run impact preview' }).click()
  const impact = page.getByRole('region', { name: 'Policy impact preview' })
  await expect(impact.getByText('source unavailable')).toBeVisible()
  await expect(impact.getByText(`${repoB} finding`)).toBeVisible()
  await expect(impact.getByText(/Coverage: 2 repositories, partial/)).toBeVisible()
})

test('simulates empty repository evidence without fabricating source IDs', async ({ page }) => {
  await fixture(page, { emptySource: repoB })
  const emptyRequests: Array<Record<string, unknown>> = []
  await page.route(`**${base}/policies/versions/${version.id}/simulate`, async route => { const body = route.request().postDataJSON() as Record<string, unknown>; emptyRequests.push(body); await route.fulfill({ json: { hash: 'proof', resolved: { hash: `effective-${body.repository_id}`, layers: [], repository_id: body.repository_id, paused: false, scope_paused: false, problems: [], missing_defaults: [], policy }, decision: { outcome: 'allow', blockers: [], bindings: [], evidence_references: [], starting_policy_hash: `effective-${body.repository_id}` } } }) })
  await page.getByRole('region', { name: 'Policy impact preview' }).getByRole('button', { name: 'Run impact preview' }).click()
  const impact = page.getByRole('region', { name: 'Policy impact preview' })
  const emptyRow = impact.getByRole('row').filter({ hasText: 'catalog' })
  await expect(emptyRow).toContainText('No stored findings or changes.')
  await expect.poll(() => emptyRequests.filter(request => request.repository_id === repoB).length).toBe(1)
  const emptyRequest = emptyRequests.find(request => request.repository_id === repoB)!
  expect((emptyRequest.input as { current: { head: string; target: string } }).current).toMatchObject({ head: '', target: '' })
  await expect(emptyRow).toContainText('unknown')
})

test('reports completed preview with no affected repositories', async ({ page }) => {
  await fixture(page, { noRepositories: true })
  await page.getByRole('region', { name: 'Policy impact preview' }).getByRole('button', { name: 'Run impact preview' }).click()
  const impact = page.getByRole('region', { name: 'Policy impact preview' })
  await expect(impact.getByText('No affected repositories.')).toBeVisible()
  await expect(impact.getByText(/Coverage: 0 repositories, complete/)).toBeVisible()
})

test('honors team and repository scope boundaries and forwards repository primary team', async ({ page }) => {
  await fixture(page)
  const teamRequests: string[] = []
  const simulations: Array<Record<string, unknown>> = []
  await page.route(`**${base}/repositories**`, route => { const url = new URL(route.request().url()); if (url.pathname.endsWith(`/repositories/${repoA}`)) return route.fulfill({ json: { id: repoA, name: 'payments' } }); const teamID = url.searchParams.get('team_id'); if (teamID) teamRequests.push(teamID); return route.fulfill({ json: { items: [{ id: repoA, name: 'payments' }], complete: true } }) })
  await page.route(`**${base}/policies/versions/${version.id}/simulate`, async route => { const body = route.request().postDataJSON() as Record<string, unknown>; simulations.push(body); await route.fulfill({ json: { hash: 'proof', resolved: { hash: 'effective', layers: [], repository_id: body.repository_id, paused: false, scope_paused: false, problems: [], missing_defaults: [], policy }, decision: { outcome: 'allow', blockers: [], bindings: [], evidence_references: [], starting_policy_hash: 'effective' } } }) })
  await page.getByRole('group', { name: 'Policy scope' }).getByRole('button', { name: 'Team' }).click()
  await page.getByLabel('Policy team').selectOption('team-1')
  await page.getByRole('button', { name: version.id }).click()
  await page.getByRole('button', { name: 'Open simulation' }).click()
  await page.getByRole('region', { name: 'Policy impact preview' }).getByRole('button', { name: 'Run impact preview' }).click()
  await expect.poll(() => teamRequests.length).toBeGreaterThan(0)
  await expect.poll(() => simulations.length).toBeGreaterThan(0)
  expect(new Set(teamRequests)).toEqual(new Set(['team-1']))

  simulations.length = 0
  await page.getByLabel('Policy repository').selectOption(repoA)
  await page.getByRole('group', { name: 'Policy scope' }).getByRole('button', { name: 'Repository' }).click()
  await page.getByRole('button', { name: version.id }).click()
  await page.getByRole('button', { name: 'Open simulation' }).click()
  await page.getByRole('region', { name: 'Policy impact preview' }).getByRole('button', { name: 'Run impact preview' }).click()
  await expect.poll(() => simulations.length).toBe(1)
  const body = simulations[0] as { repository_id: string; primary_team_id: string }
  expect(body.repository_id).toBe(repoA)
  expect(body.primary_team_id).toBe('team-1')
})

test('preserves stale native snapshot and backend deny while showing gate value deltas', async ({ page }) => {
  await fixture(page)
  await page.getByRole('region', { name: 'Policy impact preview' }).getByLabel('Preview action').selectOption('publish')
  await page.route(`**${base}/repositories/${repoA}/changes**`, route => route.fulfill({ json: { items: [{ id: 'change-stale', title: 'Stale native change', head_sha: 'head-change', target_sha: 'target-change', state: 'open' }], snapshot_state: 'stale', complete: true } }))
  await page.route(`**${base}/policies/versions/${version.id}/simulate`, route => route.fulfill({ json: { hash: 'proof', resolved: { hash: 'effective', layers: [], repository_id: repoA, paused: false, scope_paused: false, problems: [], missing_defaults: [], policy: { ...policy, allow: { ...policy.allow, environments: ['production'] }, deny: ['merge'], limits: { budget: 0 } } }, decision: { outcome: 'deny', blockers: ['protected branch'], bindings: [], evidence_references: [], starting_policy_hash: `effective-${repoA}` } } }))
  await page.getByRole('region', { name: 'Policy impact preview' }).getByRole('button', { name: 'Run impact preview' }).click()
  const impact = page.getByRole('region', { name: 'Policy impact preview' })
  const staleRow = impact.getByRole('row').filter({ hasText: 'Stale native change' })
  await expect(staleRow).toContainText('Snapshot stale')
  await expect(staleRow).toContainText('deny')
  await expect(staleRow).toContainText('allow.environments: inherit / unrestricted → production')
  await expect(staleRow).toContainText('deny: none → merge')
  await expect(staleRow).toContainText('limits.budget: unlimited → 0')
})

test('recipe edit after pending repository pagination permits a fresh run', async ({ page }) => {
  await fixture(page, { paginated: true })
  const impact = page.getByRole('region', { name: 'Policy impact preview' })
  let release!: () => void
  const pageBarrier = new Promise<void>(resolve => { release = resolve })
  let responseHandled!: () => void
  const responseDone = new Promise<void>(resolve => { responseHandled = resolve })
  let pageStarted = false
  let simulationPosts = 0
  await page.route(`**${base}/repositories?*`, async route => { const cursor = new URL(route.request().url()).searchParams.get('cursor'); if (cursor !== 'repo-next') return route.fallback(); pageStarted = true; await pageBarrier; await route.fulfill({ json: { items: [{ id: repoB, name: 'catalog' }], complete: true } }); responseHandled() })
  await page.route(`**${base}/policies/versions/${version.id}/simulate`, async route => { simulationPosts += 1; await route.fulfill({ json: { hash: 'proof', resolved: { hash: 'effective', layers: [], repository_id: repoA, paused: false, scope_paused: false, problems: [], missing_defaults: [], policy }, decision: { outcome: 'allow', blockers: [], bindings: [], evidence_references: [], starting_policy_hash: `effective-${repoA}` } } }) })
  await impact.getByRole('button', { name: 'Run impact preview' }).click()
  await impact.getByRole('button', { name: 'Load more repositories' }).click()
  await expect.poll(() => pageStarted).toBe(true)
  await page.getByRole('textbox', { name: 'Recipe', exact: true }).fill('dependency-update-v2')
  await expect(impact.getByRole('button', { name: 'Run impact preview' })).toBeEnabled()
  expect(simulationPosts).toBe(1)
  release()
  await responseDone
  await page.evaluate(() => new Promise<void>(resolve => requestAnimationFrame(() => resolve())))
  await expect(impact.getByText('catalog')).toHaveCount(0)
  expect(simulationPosts).toBe(1)
})
