import { test, expect, type Page } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'
const repo = '00000000-0000-4000-8000-000000000011'
const other = '00000000-0000-4000-8000-000000000012'
const base = `/api/v1/orgs/${org}`
const policy = { schema: 'maintenance/v1', allow: { recipes: null, models: null, routes: null, merge_methods: null, environments: null, workflows: null }, deny: ['merge', 'deploy', 'recover'], forbidden_paths: [], limits: {}, required: [], defaults: {}, paused: false }
const layer = { scope: { kind: 'organisation', id: org }, version_id: 'active', binding_version: 1, policy }

async function fixture(page: Page) {
  await page.route(`**${base}/**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'owner', name: 'Owner' }, organisations: [{ id: org, name: 'Local' }], memberships: [{ org_id: org, role: 'owner', all_repositories: true }], csrf_token: 'csrf' } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { development: true, fixture_auth: true } }))
  await page.route(`**${base}/repositories?**`, route => route.fulfill({ json: { items: [{ id: repo, name: 'shop', provider: 'gitea', team_ids: [], accessible: true }, { id: other, name: 'blog', provider: 'gitea', team_ids: [], accessible: true }], complete: true } }))
  await page.route(`**${base}/repositories/${repo}`, route => route.fulfill({ json: { id: repo, name: 'shop', accessible: true, default_branch: 'main', provider: 'gitea', team_ids: [] } }))
  await page.route(`**${base}/autopilot`, route => route.fulfill({ json: { enabled: true, version: 1, queued: 0, status: 'Waiting' } }))
  await page.route(`**${base}/policies/effective?**`, route => route.fulfill({ json: { hash: 'current', layers: [layer], policy: { ...policy, deny: ['repair', 'publish', 'merge', 'deploy', 'recover'] }, problems: [], missing_defaults: [], paused: false } }))
  await page.route(`**${base}/budgets/organisation/${org}`, route => route.fulfill({ json: { period: 'monthly', caps: { micro_usd: 20_000_000 }, paused: false } }))
}

test('repository setup previews named scope and cap, then resumes without technical editors', async ({ page }) => {
  await fixture(page)
  await page.route(`**${base}/repositories/${repo}/maintenance-status`, route => route.fulfill({ json: { mode: 'observe', micro_usd: 10_000_000, continuous: false } }))
  let input: Record<string, unknown> | undefined
  const plan = { repository_id: repo, repository: 'shop', mode: 'propose', micro_usd: 7_500_000, continuous: true, actions: ['read', 'repair', 'publish'], decisions: {}, blockers: [], ready: true, proof: 'approved', model: 'local', route: 'default', budget_period: 'monthly', organisation_micro_usd: 20_000_000, organisation_period: 'monthly', creates_organisation_limit: false }
  await page.route(`**${base}/repositories/${repo}/set-up-maintenance/preview`, route => { input = route.request().postDataJSON(); return route.fulfill({ json: plan }) })
  let configured = false
  await page.route(`**${base}/repositories/${repo}/set-up-maintenance`, route => { expect(route.request().postDataJSON().proof).toBe('approved'); configured = true; return route.fulfill({ json: plan }) })
  await page.goto(`/org/${org}/repositories?repository=${repo}`)
  await page.getByRole('button', { name: 'Set up maintenance', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Maintain shop' })
  await dialog.getByLabel('Spending ceiling ($)').fill('7.50')
  await dialog.getByRole('button', { name: 'Check setup' }).click()
  await expect(dialog.getByText(/Only shop: scan, fix, open changes/)).toBeVisible()
  await expect(dialog.getByText(/Organisation ceiling: \$20.00 per monthly period/)).toBeVisible()
  expect(configured).toBe(false)
  expect(input).toMatchObject({ mode: 'propose', micro_usd: 7_500_000, continuous: true })
  await dialog.getByRole('button', { name: 'Set up maintenance', exact: true }).click()
  await expect(dialog.getByText('Maintenance configured. First scan started.')).toBeVisible()
  expect(configured).toBe(true)
})

test('organisation mode remains distinct from work enabled and previews every repository mutation', async ({ page }) => {
  await fixture(page)
  const candidate = { id: 'candidate', scope: { kind: 'organisation', id: org }, policy: { ...policy, deny: ['deploy', 'recover'] } }
  await page.route(`**${base}/policies/versions`, route => route.fulfill({ json: candidate }))
  const simulated: Array<{ repository_id: string; input: { action: string } }> = []
  await page.route(`**${base}/policies/versions/candidate/simulate`, route => { simulated.push(route.request().postDataJSON()); return route.fulfill({ json: { hash: 'preview', resolved: { policy: candidate.policy, problems: [], paused: false }, decision: { outcome: 'unknown', blockers: ['Native checks required'] } } }) })
  let activated = false
  await page.route(`**${base}/policies/versions/candidate/activate`, route => { activated = true; return route.fulfill({ json: { version: 2 } }) })
  await page.goto(`/org/${org}/policies`)
  const modes = page.getByRole('group', { name: 'Organisation permissions' })
  await expect(modes.getByRole('button', { name: 'Propose', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByText('Organisation work enabled', { exact: true })).toBeVisible()
  await modes.getByRole('button', { name: 'Merge', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Organisation maintenance preview' })
  await expect(dialog.getByText(/Affects 2 repositories. \$20.00 per month/)).toBeVisible()
  expect(activated).toBe(false)
  expect(new Set(simulated.map(item => item.repository_id))).toEqual(new Set([repo, other]))
  expect(simulated.every(item => item.input.action !== 'read')).toBe(true)
  await dialog.getByRole('button', { name: 'Apply across organisation' }).click()
  await expect(dialog).not.toBeVisible()
  expect(activated).toBe(true)
})

test('owner reports existing defect without hashes, package metadata or model choices', async ({ page }) => {
  await fixture(page)
  const finding = { id: 'reported', repository_id: repo, title: 'Checkout button does nothing on mobile', category: 'reported_defect', source: 'user_report', state: 'open', severity: 'medium', version: 1, last_seen: new Date().toISOString(), reason: '', evidence: { head_sha: 'a'.repeat(40), target_sha: 'a'.repeat(40), target_branch: 'main', complete: true, ownership: 'human', provenance: 'Owner-reported defect; reproduction required', checks: [], dependencies: [], blockers: [], report: { observed: 'Checkout button does nothing on mobile', expected: 'Checkout opens payment' } } }
  let submitted: Record<string, unknown> | undefined
  await page.route(`**${base}/findings/reports`, route => { submitted = route.request().postDataJSON(); return route.fulfill({ status: 201, json: finding }) })
  await page.route(`**${base}/findings/reported`, route => route.fulfill({ json: finding }))
  await page.goto(`/org/${org}/findings?repository=${repo}`)
  await page.getByRole('button', { name: 'Fix this', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Fix this' })
  await dialog.getByLabel('What stopped working?').fill(finding.evidence.report.observed)
  await dialog.getByLabel('What should happen?').fill(finding.evidence.report.expected)
  await dialog.getByRole('button', { name: 'Fix this', exact: true }).click()
  await expect(dialog).not.toBeVisible()
  await expect(page.getByText('Reported. Fix starts when checks and maintenance permissions allow.')).toBeVisible()
  await expect(page.getByText('Expected: Checkout opens payment')).toBeVisible()
  expect(submitted).toMatchObject({ repository_id: repo, kind: 'defect', observed: finding.evidence.report.observed, expected: finding.evidence.report.expected })
  expect(submitted).not.toHaveProperty('head_sha')
  expect(submitted).not.toHaveProperty('model_connection_id')
})
