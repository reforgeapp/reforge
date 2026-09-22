import { test, expect, type Page } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'
const event = { id: 'event-1', actor_id: 'user-1', action: 'finding.dismiss', object_id: 'finding-1', request_id: 'request-1', repository_id: 'repo-1', created_at: '2026-09-22T00:00:00Z', data: { reason: 'accepted risk' } }

async function mock(page: Page) {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }], memberships: [{ org_id: org, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { name: 'Reforge', version: 'test', edition: 'self-hosted', development: false } }))
  await page.route(`**/api/v1/orgs/${org}/repositories**`, route => route.fulfill({ json: { items: [{ id: 'repo-1', name: 'payments' }], complete: true } }))
  await page.route(`**/api/v1/orgs/${org}/audit-events**`, route => route.fulfill({ json: { items: [event], complete: true } }))
  await page.goto(`/org/${org}/audit`)
}

test('audit workspace keeps concise filters and URL selected detail', async ({ page }) => {
  await page.setViewportSize({ width: 900, height: 800 })
  await mock(page)
  await expect(page.getByRole('combobox', { name: 'Repository filter' })).toBeVisible()
  await expect(page.getByRole('textbox', { name: 'Audit search' })).toBeVisible()
  await page.getByRole('row', { name: /finding\.dismiss/ }).getByRole('button').click()
  await expect(page).toHaveURL(/event=event-1/)
  await expect(page.getByRole('region', { name: 'Audit event details' })).toBeVisible()
  await page.locator('.split-detail:not([hidden]) .back-link').click()
  await expect(page).not.toHaveURL(/event=event-1/)
})

test('advanced audit filters stay collapsed until requested', async ({ page }) => {
  await mock(page)
  const advanced = page.locator('.audit-advanced')
  await expect(advanced).not.toHaveAttribute('open', '')
  await advanced.locator('summary').click()
  await expect(page.getByRole('textbox', { name: 'Actor filter' })).toBeVisible()
})
