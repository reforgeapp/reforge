import { test, expect } from '@playwright/test'
import { mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'

const organisation = '00000000-0000-4000-8000-000000000001'
const outDir = process.env.REFORGE_VISUAL_DIR || join(process.cwd(), '..', '.local', 'visual')
const now = new Date().toISOString()
const blocked = { id: 'task-blocked', org_id: organisation, repository_id: 'repo-1', operation_id: 'op-1', recipe: 'go', recipe_version: '1', target_branch: 'main', model_route: '', policy_hash: 'policy', starting_policy_hash: 'policy', state: 'blocked', reason: 'Frozen validation failed; review the candidate evidence', version: 3, cancel_version: 1, max_attempts: 1, created_at: now }
const staleRepository = { id: 'repo-1', org_id: organisation, connection_id: 'forge-1', native_id: '1', name: 'team/payments', provider: 'gitea', url: 'https://gitea.example/team/payments', default_branch: 'main', archived: false, paused: false, accessible: true, team_ids: [], version: 2, last_synced_at: '2026-09-20T00:00:00Z', sync_state: 'stale', sync_reason: 'Inventory is stale' }

test.describe('T29 shell and state captures', () => {
  test.skip(process.env.REFORGE_VISUAL !== '1', 'set REFORGE_VISUAL=1 to capture state baselines')

  test('captures the sign-in and detail surfaces', async ({ page }) => {
    await page.route('**/api/v1/meta', route => route.fulfill({ json: { name: 'Reforge', version: 'test', edition: 'self-hosted', development: true, fixture_auth: true } }))
    await page.route('**/api/v1/session', route => route.fulfill({ status: 401, contentType: 'application/json', body: JSON.stringify({ code: 'unauthenticated', message: 'Authentication required', request_id: 'capture', retryable: false }) }))
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto('/')
    await page.getByRole('button', { name: /sign in with your organisation/i }).waitFor()
    await page.screenshot({ path: join(outDir, 'sign-in-1440.png'), fullPage: true })

    const finding = { id: 'finding-detail', org_id: organisation, repository_id: 'repo-1', source: 'advisory', source_id: 'CVE-1', category: 'security_advisory', severity: 'high', title: 'Upgrade vulnerable dependency', evidence: { provenance: 'gitea', connection_id: 'forge-1', connection_version: 1, config_version: 1, head_sha: 'a'.repeat(40), target_sha: 'b'.repeat(40), target_branch: 'main', checks: [], dependencies: [{ ecosystem: 'npm', name: 'lodash', manifest: 'package.json', from: '4.17.0', to: '4.17.21' }], ownership: 'bot-owned', complete: true, blockers: [], merge_blockers: [] }, fingerprint: 'fp', evidence_digest: 'digest', state: 'open', reason: 'Fixture finding', version: 2, first_seen: now, last_seen: now }
    await page.route('**/api/v1/orgs/**/findings**', route => route.fulfill({ json: { items: [finding], complete: true } }))
    await page.route('**/api/v1/orgs/**/findings/finding-detail', route => route.fulfill({ json: finding }))
    await page.unroute('**/api/v1/session')
    await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'user-1', name: 'Operator', email: 'operator@example.test' }, organisations: [{ id: organisation, name: 'Example', version: 1, paused: false }], memberships: [{ org_id: organisation, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
    await page.route('**/api/v1/orgs/**/inventory-repositories**', route => route.fulfill({ json: { items: [], complete: true } }))
    await page.goto(`/org/${organisation}/findings`)
    await page.getByRole('button', { name: finding.title }).click()
    await page.getByRole('region', { name: finding.title }).waitFor()
    await page.screenshot({ path: join(outDir, 'findings-detail-1440.png'), fullPage: true })
  })

  test('captures blocked run and stale repository states', async ({ page, browser }) => {
    test.setTimeout(60_000)
    mkdirSync(outDir, { recursive: true })
    await page.goto('/auth/login')
    await page.waitForURL(/\/overview/)

    await page.route(`**/api/v1/orgs/${organisation}/tasks**`, route => route.fulfill({ json: { items: [blocked], complete: true } }))
    await page.route(`**/api/v1/orgs/${organisation}/run-repositories**`, route => route.fulfill({ json: { items: [staleRepository], complete: true } }))
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto(`/org/${organisation}/runs`)
    await expect(page.getByText('Frozen validation failed; review the candidate evidence')).toBeVisible()
    await page.screenshot({ path: join(outDir, 'runs-blocked-1440.png'), fullPage: true })

    await page.route(`**/api/v1/orgs/${organisation}/repositories**`, route => route.fulfill({ json: { items: [staleRepository], complete: true } }))
    await page.goto(`/org/${organisation}/repositories`)
    await expect(page.locator('table').getByText('stale', { exact: true }).first()).toBeVisible()
    await page.screenshot({ path: join(outDir, 'repositories-stale-1440.png'), fullPage: true })

    writeFileSync(join(outDir, 'states.json'), JSON.stringify({
      browser: browser.browserType().name(),
      browser_version: browser.version(),
      captured_at: new Date().toISOString(),
      captures: [
        { route: 'runs', state: 'blocked', file: 'runs-blocked-1440.png', viewport: '1440x900' },
        { route: 'repositories', state: 'stale', file: 'repositories-stale-1440.png', viewport: '1440x900' },
      ],
    }, null, 2))
  })
})
