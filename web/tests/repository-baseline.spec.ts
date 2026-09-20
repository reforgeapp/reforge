import { test, expect } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'
const repo = 'repo-1'
const prefix = '/api/v1/orgs/' + org
test.use({ trace: 'off' })

test('repository baseline shows scoped evidence and honest empty state', async ({ page }) => {
  await page.goto('/auth/login')
  await expect(page).toHaveURL(/\/org\/[^/]+\/overview/)
  await page.route('**' + prefix + '/repositories?**', route => route.fulfill({ json: { items: [{ id: repo, org_id: org, name: 'repo', provider: 'gitea', url: 'https://forge.test/repo', default_branch: 'main', team_ids: [], archived: false, paused: false, accessible: true, version: 1 }], complete: true } }))
  await page.route('**' + prefix + '/repositories/' + repo, route => route.fulfill({ json: { id: repo, org_id: org, name: 'repo', provider: 'gitea', url: 'https://forge.test/repo', default_branch: 'main', team_ids: [], archived: false, paused: false, accessible: true, version: 1 } }))
  await page.route('**' + prefix + '/repositories/' + repo + '/changes**', route => route.fulfill({ json: { items: [], complete: true, snapshot_state: 'fresh' } }))
  await page.route('**' + prefix + '/repositories/' + repo + '/maintenance', route => route.fulfill({ json: { repository_id: repo, trusted_bots: [], merge_authority: 'observe', version: 1 } }))
  await page.route('**' + prefix + '/repositories/' + repo + '/discovery', route => route.fulfill({ json: { repository_id: repo, state: 'not_started' } }))
  await page.route('**' + prefix + '/repositories/' + repo + '/repair-baseline', route => route.fulfill({ json: { run: { task: { id: 'task-1', model_route: 'local/model', runner_pool_id: 'pool-1' }, state: 'completed', version: 2, candidate_sha: 'candidate', candidate_checks: [], context: { native_head_sha: 'head', model: 'local/model', plan: { digest: 'plan', baseline_sha: 'base', target_sha: 'target', recipe: { commands: [{ id: 'test', args: ['go', 'test', './...'], directory: '/workspace', timeout_seconds: 30, report_format: 'text' }] } } } } } }))
  await page.route('**' + prefix + '/policies/effective**', route => route.fulfill({ json: { hash: 'policy', layers: [], repository_id: repo, paused: false, scope_paused: false, missing_defaults: [], problems: [], policy: { schema: 'maintenance/v1' } } }))
  await page.route('**' + prefix + '/findings**', route => route.fulfill({ json: { items: [{ id: 'finding-1', title: 'Dependency issue', severity: 'high', state: 'open', evidence: { checks: [], dependencies: [] } }], complete: true } }))
  await page.goto('/org/' + org + '/repositories')
  await page.getByRole('button', { name: 'repo', exact: true }).click()
  await expect(page.getByText('Observed execution evidence')).toBeVisible()
  await expect(page.getByText('test: go test ./...')).toBeVisible()
  await expect(page.getByText('View findings scoped to this repository')).toBeVisible()
})

test('repository baseline reports empty persisted evidence', async ({ page }) => {
  await page.goto('/auth/login')
  await expect(page).toHaveURL(/\/org\/[^/]+\/overview/)
  await page.route('**' + prefix + '/repositories?**', route => route.fulfill({ json: { items: [{ id: repo, org_id: org, name: 'repo', provider: 'gitea', url: 'https://forge.test/repo', default_branch: 'main', team_ids: [], archived: false, paused: false, accessible: true, version: 1 }], complete: true } }))
  await page.route('**' + prefix + '/repositories/' + repo, route => route.fulfill({ json: { id: repo, org_id: org, name: 'repo', provider: 'gitea', url: 'https://forge.test/repo', default_branch: 'main', team_ids: [], archived: false, paused: false, accessible: true, version: 1 } }))
  await page.route('**' + prefix + '/repositories/' + repo + '/changes**', route => route.fulfill({ json: { items: [], complete: true, snapshot_state: 'fresh' } }))
  await page.route('**' + prefix + '/repositories/' + repo + '/maintenance', route => route.fulfill({ json: { repository_id: repo, trusted_bots: [], merge_authority: 'observe', version: 1 } }))
  await page.route('**' + prefix + '/repositories/' + repo + '/discovery', route => route.fulfill({ json: { repository_id: repo, state: 'not_started' } }))
  await page.route('**' + prefix + '/repositories/' + repo + '/repair-baseline', route => route.fulfill({ json: { run: null } }))
  await page.route('**' + prefix + '/policies/effective**', route => route.fulfill({ json: { hash: 'policy', layers: [], repository_id: repo, paused: false, scope_paused: false, missing_defaults: [], problems: [], policy: { schema: 'maintenance/v1' } } }))
  await page.route('**' + prefix + '/findings**', route => route.fulfill({ json: { items: [], complete: true } }))
  await page.goto('/org/' + org + '/repositories')
  await page.getByRole('button', { name: 'repo', exact: true }).click()
  await expect(page.getByText('No recorded repair run')).toBeVisible()
  await expect(page.getByText('No findings recorded for this repository.')).toBeVisible()
})
