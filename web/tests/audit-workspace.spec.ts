import { test, expect, type Page } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'
const event = { id: 'event-1', actor_id: 'user-1', action: 'finding.dismiss', object_id: 'finding-1', request_id: 'request-1', repository_id: 'repo-1', created_at: '2026-09-22T00:00:00Z', data: { reason: 'accepted risk' } }

async function mock(page: Page, events = [event]) {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }], memberships: [{ org_id: org, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { name: 'Reforge', version: 'test', edition: 'self-hosted', development: false } }))
  await page.route(`**/api/v1/orgs/${org}/repositories**`, route => route.fulfill({ json: { items: [{ id: 'repo-1', name: 'payments' }], complete: true } }))
  await page.route(`**/api/v1/orgs/${org}/audit-events**`, route => route.fulfill({ json: { items: events, complete: true } }))
  await page.goto(`/org/${org}/audit`)
}

test('audit workspace keeps concise filters and URL selected detail', async ({ page }) => {
  await page.setViewportSize({ width: 900, height: 800 })
  await mock(page)
  await expect(page.getByRole('combobox', { name: 'Repository filter' })).toBeVisible()
  await expect(page.getByRole('textbox', { name: 'Search loaded events' })).toHaveAttribute('placeholder', 'Search loaded events')
  await expect(page.getByText('Finding Dismiss')).toBeVisible()
  await expect(page.getByRole('row').filter({ hasText: 'Finding Dismiss' }).getByText('Fixture')).toBeVisible()
  await page.getByRole('row', { name: /Finding Dismiss/ }).getByRole('button').click()
  await expect(page).toHaveURL(/event=event-1/)
  await expect(page.getByRole('region', { name: 'Audit event details' })).toBeVisible()
  await page.getByRole('button', { name: 'Close event details' }).click()
  await expect(page).not.toHaveURL(/event=event-1/)
})

test('advanced audit filters stay collapsed until requested', async ({ page }) => {
  await mock(page)
  const advanced = page.locator('.audit-advanced')
  await expect(advanced).not.toHaveAttribute('open', '')
  await advanced.locator('summary').click()
  await expect(page.getByRole('textbox', { name: 'Actor filter' })).toBeVisible()
})

test('activity opens events and object labels distinguish runner entities', async ({ page }) => {
  const enrollment = { ...event, id: 'event-enrollment', action: 'runner.enrollment_created', object_id: 'enrollment-1' }
  const pool = { ...event, id: 'event-pool', action: 'runner.pool_changed', object_id: 'pool-1' }
  await mock(page, [enrollment, pool])
  const enrollmentRow = page.getByRole('row', { name: /Runner Enrollment Created/ })
  const poolRow = page.getByRole('row', { name: /Runner Pool Changed/ })
  await expect(enrollmentRow.getByText('Runner Enrollment', { exact: true })).toBeVisible()
  await expect(poolRow.getByText('Runner Pool', { exact: true })).toBeVisible()
  await expect(enrollmentRow.locator('td[data-label="Time"] button')).toHaveCount(0)
  const opener = enrollmentRow.getByRole('button', { name: 'Open Runner Enrollment Created event' })
  await opener.focus()
  await page.keyboard.press('Enter')
  await expect(page).toHaveURL(/event=event-enrollment/)
})

test('selecting another event returns close focus to the selected event', async ({ page }) => {
  const second = { ...event, id: 'event-2', action: 'policy.changed', object_id: 'policy-2' }
  await page.setViewportSize({ width: 1200, height: 850 })
  await mock(page, [event, second])
  const firstOpener = page.getByRole('button', { name: 'Open Finding Dismiss event' })
  const secondOpener = page.getByRole('button', { name: 'Open Policy Changed event' })
  await firstOpener.click()
  await expect(page.getByRole('button', { name: 'Close event details' })).toBeFocused()
  await secondOpener.click()
  await expect(page).toHaveURL(/event=event-2/)
  await expect(page.getByRole('button', { name: 'Close event details' })).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(secondOpener).toBeFocused()
})

test('audit event IDs stay in detail, not the activity row', async ({ page }) => {
  await mock(page)
  const row = page.getByRole('row', { name: /Finding Dismiss/ })
  await expect(row).not.toContainText('event-1')
  await expect(row).not.toContainText('request-1')
  await row.getByRole('button').click()
  await expect(page.getByRole('region', { name: 'Audit event details' })).toContainText('event-1')
  await expect(page.getByRole('region', { name: 'Audit event details' })).toContainText('request-1')
})

test('audit rows fit narrow viewport without horizontal page overflow', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await mock(page)
  await expect(page.getByRole('row', { name: /Finding Dismiss/ })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390)
  await page.getByRole('button', { name: 'Open Finding Dismiss event' }).focus()
  await page.keyboard.press('Enter')
  await expect(page).toHaveURL(/event=event-1/)
  await page.keyboard.press('Escape')
  await expect(page).not.toHaveURL(/event=event-1/)
  await expect(page.getByRole('button', { name: 'Open Finding Dismiss event' })).toBeFocused()
})
