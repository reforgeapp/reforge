import { test, expect, type Page } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'
const base = `/api/v1/orgs/${org}`
const amount = { micro_usd: 0, tokens: 0, milliseconds: 0, requests: 0, concurrency: 0 }

test('USD budget input preserves sequential micro precision', async ({ page }) => {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture' }, organisations: [{ id: org, name: 'Fixture' }], memberships: [{ org_id: org, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route(`**${base}/repositories**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route(`**${base}/teams**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route(`**${base}/connections**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route(`**${base}/campaigns**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route(`**${base}/budgets/organisation/${org}`, route => route.fulfill({ json: { scope: { kind: 'organisation', id: org }, period: 'daily', caps: { micro_usd: null, tokens: null, milliseconds: null, requests: null, concurrency: null }, paused: false, version: 0, held: amount, spent: amount } }))
  let payload: any
  await page.route(`**${base}/budgets/organisation/${org}`, async route => { if (route.request().method() === 'PUT') { payload = await route.request().postDataJSON(); await route.fulfill({ json: payload }) } else await route.fallback() })
  await page.goto(`/org/${org}/usage`)
  await page.getByRole('tab', { name: 'Budgets' }).click()
  await page.getByLabel('USD cap').pressSequentially('1.234567')
  await page.getByRole('button', { name: 'Save budget' }).click()
  await expect.poll(() => payload?.caps.micro_usd).toBe(1234567)
})
