import { test, expect, type Page } from '@playwright/test'

const organisation = '00000000-0000-4000-8000-000000000001'
const task = (id: string, state = 'completed', version = 2) => ({ id, org_id: organisation, repository_id: 'repo-1', operation_id: 'op-1', recipe: 'go', recipe_version: '1', target_branch: 'main', model_route: '', policy_hash: 'policy', starting_policy_hash: 'policy', state, reason: '', version, cancel_version: 1, max_attempts: 1, created_at: new Date().toISOString() })
const run = (item: ReturnType<typeof task>, state = item.state) => ({ task: { ...item, state }, state, version: item.version, branch: 'repair/' + item.id, candidate_sha: 'abc', candidate_checks: [{ command_id: 'test', exit_code: 0, output_sha256: 'sha', complete: true, reason: '', cases: { 'TestCase/one': 'passed' } }], candidate_artifacts: ['candidate-log'], context: { native_head_sha: 'head', model: 'claude-sonnet-4-5', finding: { title: 'Repair failing Go tests', category: 'ci', source: 'native_ci' }, policy_hash: 'policy', plan: { digest: 'plan', baseline_sha: 'base', target_sha: 'target', recipe: { commands: [{ id: 'go-test', args: ['go', 'test', './...'], directory: '/workspace', timeout_seconds: 30, report_format: 'text' }] } } }, report: { state: 'passed', reason: '', diff: '@@ -1 +1 @@\n+\u202e<script>alert(1)</script>', plan_digest: 'plan', baseline: [{ command_id: 'baseline-test', exit_code: 0, output_sha256: 'h', complete: true, reason: '', cases: { 'baseline case': 'passed' } }], candidate: [], target: [], patches: [{ path: 'main.go', content: 'PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==' }], artifacts: ['artifact-1'], turns: 1 }, updated_at: new Date().toISOString() })

async function login(page: Page) {
  await page.goto('/auth/login')
  await expect(page).toHaveURL(/\/org\/[^/]+\/overview/)
}

test.use({ trace: 'off' })

test('runs detail exposes frozen evidence and supports keyboard deep links', async ({ page }) => {
  await login(page)
  const item = task('task-1')
  await page.route(`**/api/v1/orgs/${organisation}/tasks**`, route => route.fulfill({ json: { items: [item], complete: true } }))
  await page.route(`**/api/v1/orgs/${organisation}/repair-runs/task-1`, route => route.fulfill({ json: run(item) }))
  await page.route(`**/api/v1/orgs/${organisation}/events**`, route => route.fulfill({ status: 200, headers: { 'content-type': 'text/event-stream' }, body: ': keepalive\\n\\n' }))
  await page.goto(`/org/${organisation}/runs?run=task-1`)
  await expect(page.getByRole('heading', { name: /go · task-1/ })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Run task-1' })).toContainText('Repair failing Go tests')
  await expect(page.getByText('claude-sonnet-4-5')).toBeVisible()
  await page.getByRole('tab', { name: 'Evidence' }).click()
  await page.getByText('Source diff').click()
  await expect(page.getByText('H baseline')).toBeVisible()
  await expect(page.getByText('baseline case: passed')).toBeVisible()
  await expect(page.getByText('TestCase/one: passed')).toBeVisible()
  await expect(page.getByText('<script>alert(1)</script>')).toBeVisible()
  await expect(page.getByText('PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==')).toHaveCount(0)
  await page.setViewportSize({ width: 390, height: 844 })
  await page.getByRole('button', { name: 'Back to list' }).focus()
  await expect(page.getByRole('button', { name: 'Back to list' })).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(page).not.toHaveURL(/run=task-1/)
  await page.goBack()
  await expect(page).toHaveURL(/run=task-1/)
})

test('cancel and resume send CSRF and quoted versions', async ({ page }) => {
  await login(page)
  const item = task('task-2', 'queued', 4)
  await page.route(`**/api/v1/orgs/${organisation}/tasks**`, route => route.fulfill({ json: { items: [item], complete: true } }))
  let currentState = 'queued'
  await page.route(`**/api/v1/orgs/${organisation}/repair-runs/task-2`, route => route.fulfill({ json: run(item, currentState) }))
  const actions: Array<{ path: string; match: string; csrf: string }> = []
  for (const action of ['cancel', 'resume']) await page.route(`**/api/v1/orgs/${organisation}/tasks/task-2/${action}`, async route => { actions.push({ path: action, match: route.request().headers()['if-match'] ?? '', csrf: route.request().headers()['x-csrf-token'] ?? '' }); currentState = action === 'cancel' ? 'blocked' : 'completed'; await route.fulfill({ json: { ...item, state: currentState } }) })
  await page.route(`**/api/v1/orgs/${organisation}/events**`, route => route.fulfill({ status: 200, headers: { 'content-type': 'text/event-stream' }, body: ': keepalive\\n\\n' }))
  await page.goto(`/org/${organisation}/runs?run=task-2`)
  await page.getByRole('button', { name: 'Cancel run' }).click()
  await expect.poll(() => actions.length).toBe(1)
  expect(actions[0]).toMatchObject({ path: 'cancel', match: '"4"' }); expect(actions[0].csrf).toBeTruthy()
  await page.getByRole('button', { name: 'Resume run' }).click()
  await expect.poll(() => actions.length).toBe(2)
  expect(actions[1]).toMatchObject({ path: 'resume', match: '"4"' }); expect(actions[1].csrf).toBeTruthy()
})

test('event stream deduplicates and surfaces access revocation', async ({ page }) => {
  await login(page)
  const item = task('task-3')
  await page.route(`**/api/v1/orgs/${organisation}/tasks**`, route => route.fulfill({ json: { items: [item], complete: true } }))
  await page.route(`**/api/v1/orgs/${organisation}/repair-runs/task-3`, route => route.fulfill({ json: run(item) }))
  let eventRequests = 0
  await page.route(`**/api/v1/orgs/${organisation}/events**`, route => { eventRequests++; const body = eventRequests === 1 ? 'id: 7\nevent: reforge\ndata: {"id":7,"type":"repair.stage","aggregate_type":"task","aggregate_id":"task-3","aggregate_version":1,"occurred_at":"2026-01-01T00:00:00Z","data":{}}\n\nid: 7\nevent: reforge\ndata: {"id":7,"type":"repair.stage","aggregate_type":"task","aggregate_id":"task-3","aggregate_version":1,"occurred_at":"2026-01-01T00:00:00Z","data":{}}\n\n' : 'event: reset\ndata: {"code":"access_revoked"}\n\n'; return route.fulfill({ status: 200, headers: { 'content-type': 'text/event-stream' }, body }) })
  await page.goto(`/org/${organisation}/runs?run=task-3`)
  await page.getByRole('tab', { name: 'Activity' }).click()
  await expect(page.getByText('repair.stage')).toHaveCount(1)
  await expect(page.getByText('Event access revoked; reload to authenticate again.')).toBeVisible()
  expect(eventRequests).toBeGreaterThan(0)
})
