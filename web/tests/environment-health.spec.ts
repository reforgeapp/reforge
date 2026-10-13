import { test, expect } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'
const base = `/api/v1/orgs/${org}`

test('current health expires in UI and environment pagination includes GitOps', async ({ page }) => {
  await page.route(`**${base}/**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'owner', name: 'Owner' }, organisations: [{ id: org, name: 'Local' }], memberships: [{ org_id: org, role: 'owner', all_repositories: true }], csrf_token: 'csrf' } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { development: true, fixture_auth: true } }))
  await page.route(`**${base}/environment-health?**`, route => {
    const next = new URL(route.request().url()).searchParams.get('cursor')
    return route.fulfill({ json: { items: [{ repository_id: 'repo', repository: 'shop', environment: next ? 'staging' : 'production', kind: next ? 'gitops' : 'native', operation_id: 'rollout', state: next ? 'unhealthy' : 'healthy', observed_at: new Date().toISOString(), expires_at: new Date(Date.now() + (next ? 60_000 : 2_000)).toISOString() }], complete: !!next, ...(!next && { next_cursor: 'next' }) } })
  })
  await page.goto(`/org/${org}/deployments`)
  const health = page.getByRole('region', { name: 'Application health', exact: true })
  await expect(health.getByText('healthy', { exact: true })).toBeVisible()
  await expect(health.getByText('unknown', { exact: true })).toBeVisible({ timeout: 5000 })
  await health.getByRole('button', { name: 'Load more environments' }).click()
  await expect(health.getByText('shop · staging · gitops')).toBeVisible()
  await expect(health.getByText('unhealthy', { exact: true })).toBeVisible()
})
