import { test, expect } from '@playwright/test'
import { mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'

const organisation = '00000000-0000-4000-8000-000000000001'
const outDir = process.env.REFORGE_VISUAL_DIR || join(process.cwd(), '..', '.local', 'visual')
const now = new Date().toISOString()
const blocked = { id: 'task-blocked', org_id: organisation, repository_id: 'repo-1', operation_id: 'op-1', recipe: 'go', recipe_version: '1', target_branch: 'main', model_route: '', policy_hash: 'policy', starting_policy_hash: 'policy', state: 'blocked', reason: 'Frozen validation failed; review the candidate evidence', version: 3, cancel_version: 1, max_attempts: 1, created_at: now }
const staleRepository = { id: 'repo-1', org_id: organisation, connection_id: 'forge-1', native_id: '1', name: 'team/payments', provider: 'gitea', url: 'https://gitea.example/team/payments', default_branch: 'main', archived: false, paused: false, accessible: true, team_ids: [], version: 2, last_synced_at: '2026-09-20T00:00:00Z', sync_state: 'stale', sync_reason: 'Inventory is stale' }

test.describe('T29 blocked and stale captures', () => {
  test.skip(process.env.REFORGE_VISUAL !== '1', 'set REFORGE_VISUAL=1 to capture state baselines')

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
