import { test, expect, type Page } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'
const finding = { id: 'finding-1', org_id: org, repository_id: 'repo-1', source: 'advisory', source_id: 'CVE-TEST', category: 'dependency', severity: 'high', title: 'Upgrade dependency', evidence: { provenance: 'gitea', connection_id: 'connection-1', connection_version: 1, config_version: 1, head_sha: 'a'.repeat(40), target_sha: 'b'.repeat(40), target_branch: 'main', checks: [], dependencies: [], ownership: 'bot-owned', complete: true, blockers: [], merge_blockers: [] }, fingerprint: 'fingerprint', evidence_digest: 'digest', state: 'open', reason: 'Needs review', version: 1, first_seen: new Date().toISOString(), last_seen: new Date().toISOString() }

async function mock(page: Page) {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Fixture', email: 'fixture@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1, paused: false }], memberships: [{ org_id: org, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { name: 'Reforge', version: 'test', edition: 'self-hosted', development: true, fixture_auth: true } }))
  await page.route(`**/api/v1/orgs/${org}/findings**`, route => route.fulfill({ json: { items: [finding], complete: true } }))
  await page.route(`**/api/v1/orgs/${org}/findings/finding-1`, route => route.fulfill({ json: finding }))
  await page.route(`**/api/v1/orgs/${org}/repositories**`, route => route.fulfill({ json: { items: [{ id: 'repo-1', name: 'payments', provider: 'gitea', team_ids: [], accessible: true }], complete: true } }))
}

const focusedList = (page: Page) => page.evaluate(() => document.activeElement?.classList.contains('split-list') ?? false)

for (const width of [1440, 390]) {
  test(`split view leaves cold-load focus alone and restores the opener at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 844 })
    await mock(page)
    await page.goto(`/org/${org}/findings`)
    const opener = page.locator('.split-list .link-button').first()
    await expect(opener).toBeVisible()
    expect(await focusedList(page)).toBe(false)
    await opener.focus()
    await page.keyboard.press('Enter')
    const close = page.getByRole('button', { name: 'Close finding details' })
    await expect(close).toBeFocused()
    await page.keyboard.press('Escape')
    await expect(page).not.toHaveURL(/finding=/)
    await expect(opener).toBeFocused()
    await expect(opener).toBeInViewport()
    await page.keyboard.press('Enter')
    await expect(close).toBeFocused()
    await page.goBack()
    await expect(page).not.toHaveURL(/finding=/)
    await expect(opener).toBeFocused()
  })
}

test('closing a deep-linked detail moves focus to the list', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await mock(page)
  await page.goto(`/org/${org}/findings?finding=finding-1`)
  const close = page.getByRole('button', { name: 'Close finding details' })
  await expect(close).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(page).not.toHaveURL(/finding=/)
  await expect.poll(() => focusedList(page)).toBe(true)
})
