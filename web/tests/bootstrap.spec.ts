import { test, expect } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'

test('self-hosted bootstrap creates the first organisation and opens it', async ({ page }) => {
  let hasOrg = false
  let body: { name?: string; token?: string } | undefined
  await page.route('**/api/v1/session', route => route.fulfill({
    json: {
      user: { id: 'user-1', name: 'Owner', email: 'owner@example.test' },
      organisations: hasOrg ? [{ id: org, name: 'Bootstrapped', version: 1, paused: false }] : [],
      memberships: hasOrg ? [{ org_id: org, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true }] : [],
      csrf_token: 'csrf-1',
    },
  }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { name: 'Reforge', version: 'test', edition: 'self-hosted', development: true, fixture_auth: true } }))
  await page.route('**/api/v1/orgs/**', route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route('**/auth/bootstrap', route => {
    body = route.request().postDataJSON() as { name?: string; token?: string }
    hasOrg = true
    return route.fulfill({ status: 201, json: { id: org, name: 'Bootstrapped' } })
  })
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Create the first organisation' })).toBeVisible()
  await page.getByLabel('Organisation name').fill('Bootstrapped')
  await page.getByLabel('Bootstrap token').fill('one-time-token-value')
  await page.getByRole('button', { name: 'Create organisation' }).click()
  await expect.poll(() => body?.name).toBe('Bootstrapped')
  expect(body?.token).toBe('one-time-token-value')
  await expect(page).toHaveURL(new RegExp(`/org/${org}/overview`))
})
