import { test, expect, type Page } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'
const repo1 = '00000000-0000-4000-8000-000000000011'
const repo2 = '00000000-0000-4000-8000-000000000012'
const campaignID = '00000000-0000-4000-8000-000000000021'
const base = `/api/v1/orgs/${org}`
const csrf = 'csrf-1'
const sha = (value: string) => value.repeat(40).slice(0, 40)
const future = () => new Date(Date.now() + 3_600_000).toISOString()

const repairMember = (repositoryID = repo1) => [{ repository_id: repositoryID, repair: { finding_id: '00000000-0000-4000-8000-000000000031', finding_version: 2, recipe: 'go', model_connection_id: '00000000-0000-4000-8000-000000000041', model_route: 'default', runner_pool_id: '00000000-0000-4000-8000-000000000051' } }]
const campaign = (state = 'planned', version = 1) => ({ id: campaignID, name: 'Weekly repair', kind: 'repair', state, reason: 'planned', version, stage: state === 'planned' ? 0 : 1, requested_by: 'user-1', grant_expires_at: future(), spec: { name: 'Weekly repair', kind: 'repair', success: 'published', observation_seconds: 60, failure_limit: 1, failure_percent: 20 }, counts: { total: 1, excluded: 0, pending: state === 'planned' ? 1 : 0, running: state === 'canary' ? 1 : 0, succeeded: 0, failed: 0, unknown: 0 }, created_at: '2026-09-21T00:00:00Z', updated_at: '2026-09-21T00:00:00Z' })
const preview = (blockers: string[] = [], members = [{ id: '00000000-0000-4000-8000-000000000061', repository_id: repo1, repository_name: 'payments', repositories: [repo1], group: 'go', canary: true, pins: {}, state: blockers.length ? 'excluded' : 'pending', reason: blockers[0] ?? '', stage: blockers.length ? 0 : 1, input: repairMember()[0] }]) => ({ id: '00000000-0000-4000-8000-000000000071', hash: 'preview-hash', input: {}, members, blockers, expires_at: future() })

async function install(page: Page, role = 'owner', options: { previewResponse?: unknown; delayedPreview?: (route: import('@playwright/test').Route) => Promise<void> } = {}) {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }], memberships: [{ org_id: org, role, team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: csrf } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { development: true, fixture_auth: true } }))
  await page.route(`**${base}/campaigns?**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route(`**${base}/findings**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route(`**${base}/repair-recipes`, route => route.fulfill({ json: { go: 'reforge/go:1' } }))
  await page.route(`**${base}/connections**`, route => route.fulfill({ json: { items: [{ id: '00000000-0000-4000-8000-000000000041', name: 'Local model', kind: 'model', state: 'healthy', settings: { model: 'local' } }], complete: true } }))
  await page.route(`**${base}/runner-pools**`, route => route.fulfill({ json: { items: [{ id: '00000000-0000-4000-8000-000000000051', name: 'Local runners', state: 'active' }], complete: true } }))
  await page.route(`**${base}/repositories**`, route => route.fulfill({ json: { items: [{ id: repo1, name: 'payments' }, { id: repo2, name: 'catalog' }], complete: true } }))
  await page.route(`**${base}/deployment-configurations`, route => route.fulfill({ json: { items: [{ environment: 'production', repository_id: repo1, version: 1, enabled: true }, { environment: 'staging', repository_id: repo2, version: 1, enabled: true }] } }))
  await page.route(`**${base}/gitops-configurations`, route => route.fulfill({ json: { items: [] } }))
  await page.route(`**${base}/campaign-previews`, async route => { if (options.delayedPreview) return options.delayedPreview(route); await route.fulfill({ json: options.previewResponse ?? preview() }) })
  await page.goto(`/org/${org}/campaigns`)
}

test('repair advanced JSON previews, creates, starts, pauses and resumes with CAS', async ({ page }) => {
  await install(page)
  await page.getByText('Plan campaign', { exact: true }).click()
  await page.getByText('Advanced member JSON import', { exact: true }).click()
  let actionCalls: Array<{ action: string; version: string; body: Record<string, unknown> }> = []
  let current = campaign()
  let pauseAttempts = 0
  await page.route(`**${base}/campaigns/${campaignID}**`, async route => {
    const request = route.request(); const url = new URL(request.url()); const action = url.pathname.split('/').pop() ?? ''
    if (request.method() === 'GET' && url.pathname.endsWith('/members')) return route.fulfill({ json: { items: [{ id: 'member-1', repository_id: repo1, repository_name: 'payments', repositories: [repo1], group: 'go', canary: current.state !== 'planned', pins: {}, state: current.state === 'canary' ? 'running' : 'pending', reason: '', stage: 1, input: repairMember()[0] }], complete: true } })
    if (request.method() === 'GET') return route.fulfill({ headers: { ETag: `"${current.version}"` }, json: current })
    const body = request.postDataJSON() as Record<string, unknown>; actionCalls.push({ action, version: request.headers()['if-match'] ?? '', body })
    if (action === 'pause' && pauseAttempts++ === 0) return route.fulfill({ status: 409, json: { code: 'conflict', message: 'stale campaign version' } })
    const nextState: string = action === 'start' ? 'canary' : action === 'pause' ? 'paused' : action === 'resume' ? 'expanding' : 'cancelled'
    current = { ...current, state: nextState, version: current.version + 1, counts: { ...current.counts, pending: nextState === 'planned' ? 1 : 0, running: nextState === 'canary' ? 1 : 0 } }
    await route.fulfill({ headers: { ETag: `"${current.version}"` }, json: current })
  })
  await page.route(`**${base}/campaigns`, async route => { if (route.request().method() === 'POST') await route.fulfill({ status: 201, headers: { ETag: '"1"' }, json: current }); else await route.fallback() })
  await page.getByLabel('Name').fill('Weekly repair')
  await page.getByLabel('Advanced member JSON').fill(JSON.stringify(repairMember()))
  await page.getByRole('button', { name: 'Next' }).click()
  await page.getByRole('button', { name: 'Next' }).click()
  await page.getByLabel('Batch size').fill('2')
  await page.getByRole('button', { name: 'Preview campaign' }).click()
  await expect(page.getByRole('region', { name: 'Campaign preview' })).toBeVisible()
  await page.getByRole('button', { name: 'Create planned campaign' }).click()
  await expect(page.getByRole('heading', { name: 'Weekly repair' })).toBeVisible()
  await page.getByLabel('Campaign action reason').fill('start reviewed canary')
  await page.getByRole('button', { name: 'Start canary' }).click()
  await expect.poll(() => actionCalls.some(call => call.action === 'start')).toBe(true)
  await expect(page.getByRole('button', { name: 'Pause' })).toBeVisible()
  await page.getByLabel('Campaign action reason').fill('pause for review')
  await page.getByRole('button', { name: 'Pause' }).click()
  await expect(page.getByRole('alert')).toContainText('changed on the server')
  await page.getByRole('button', { name: 'Pause' }).click()
  await expect(page.getByRole('button', { name: 'Resume current stage' })).toBeVisible()
  await page.getByLabel('Campaign action reason').fill('continue reviewed stage')
  await page.getByRole('button', { name: 'Resume current stage' }).click()
  await expect.poll(() => actionCalls.filter(call => call.action === 'resume').length).toBe(1)
  expect(actionCalls.find(call => call.action === 'resume')?.version).toBe('"3"')
  expect(actionCalls.find(call => call.action === 'resume')?.body.stage_decision).toBe('continue_current_stage')
})

test('viewer cannot plan, while narrow keyboard preview remains bounded', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await install(page, 'viewer')
  await expect(page.getByRole('button', { name: 'Preview campaign' })).toHaveCount(0)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})

test('stale preview response does not replace edited form state', async ({ page }) => {
  let release!: () => void
  const wait = new Promise<void>(resolve => { release = resolve })
  await install(page, 'owner', { delayedPreview: async route => { await wait; await route.fulfill({ json: preview() }) } })
  await page.getByText('Plan campaign', { exact: true }).click()
  await page.getByText('Advanced member JSON import', { exact: true }).click()
  await page.getByLabel('Name').fill('Before response')
  await page.getByLabel('Advanced member JSON').fill(JSON.stringify(repairMember()))
  await page.getByRole('button', { name: 'Next' }).click()
  await page.getByRole('button', { name: 'Next' }).click()
  await page.getByRole('button', { name: 'Preview campaign' }).click()
  await page.getByRole('button', { name: 'Back' }).click()
  await page.getByRole('button', { name: 'Back' }).click()
  await page.getByLabel('Name').fill('Edited while waiting')
  release()
  await expect(page.getByRole('region', { name: 'Campaign preview' })).toHaveCount(0)
  await expect(page.getByLabel('Name')).toHaveValue('Edited while waiting')
})

test('native campaign captures distinct provenance for two environments', async ({ page }) => {
  let captured: Record<string, unknown> | undefined
  await install(page)
  await page.getByText('Plan campaign', { exact: true }).click()
  await page.getByText('Advanced member JSON import', { exact: true }).click()
  await page.route(`**${base}/campaign-previews`, async route => { captured = route.request().postDataJSON() as Record<string, unknown>; await route.fulfill({ json: preview() }) })
  await page.getByLabel('Name').fill('Native rollout')
  await page.getByLabel('Kind').selectOption('pipeline')
  await page.getByLabel('Configured environment').selectOption({ label: 'payments · production' })
  await page.getByRole('button', { name: 'Next' }).click()
  const add = async (environment: string, source: string, artifact: string, signature: string) => {
    await page.getByLabel('Change ID').fill(`change-${environment}`)
    await page.getByLabel('Source SHA').fill(source)
    await page.getByLabel('Artifact digest').fill(artifact)
    await page.getByLabel('Provenance JSON').fill(JSON.stringify({ document: { source_sha: source, artifact_digest: artifact }, signature }))
    await page.getByRole('button', { name: 'Add configured member' }).click()
  }
  await add('production', sha('a'), `sha256:${'b'.repeat(64)}`, 'sig-production')
  await page.getByRole('button', { name: 'Back' }).click()
  await page.getByLabel('Configured environment').selectOption({ label: 'catalog · staging' })
  await page.getByRole('button', { name: 'Next' }).click()
  await add('staging', sha('c'), `sha256:${'d'.repeat(64)}`, 'sig-staging')
  await page.getByRole('button', { name: 'Next' }).click()
  await page.getByLabel('payments · production').check()
  await page.getByRole('button', { name: 'Preview campaign' }).click()
  await expect.poll(() => captured).toBeTruthy()
  const members = captured?.members as Array<{ pipeline: { provenance: { document: { source_sha: string }; signature: string } } }>
  expect(members).toHaveLength(2)
  expect(members[0].pipeline.provenance.document.source_sha).not.toBe(members[1].pipeline.provenance.document.source_sha)
  expect(new Set(members.map(member => member.pipeline.provenance.signature))).toEqual(new Set(['sig-production', 'sig-staging']))
  expect(captured?.canary_ids).toEqual([repo1])
})

test('UTC window and unknown canary blocker remain explicit', async ({ page }) => {
  await install(page, 'owner', { previewResponse: preview(['Canary size exceeds eligible members; resolve exclusions or create a new selection'], [{ ...preview().members[0], state: 'excluded', reason: 'Unknown canary evidence' }]) })
  await page.getByText('Plan campaign', { exact: true }).click()
  await page.getByText('Advanced member JSON import', { exact: true }).click()
  await page.getByLabel('Name').fill('Blocked campaign')
  await page.getByLabel('Advanced member JSON').fill(JSON.stringify(repairMember()))
  await page.getByRole('button', { name: 'Next' }).click()
  await page.getByRole('button', { name: 'Next' }).click()
  await page.getByRole('button', { name: 'Add UTC window' }).click()
  await page.getByLabel('Window 1 weekdays').fill('0')
  await page.getByLabel('Start minute').fill('60')
  await page.getByLabel('End minute').fill('120')
  await page.getByRole('button', { name: 'Preview campaign' }).click()
  await expect(page.getByText('Canary size exceeds eligible members', { exact: false })).toBeVisible()
  await expect(page.getByText('Unknown canary evidence', { exact: false }).first()).toBeVisible()
  await expect(page.getByRole('button', { name: 'Create planned campaign' })).toBeDisabled()
})
