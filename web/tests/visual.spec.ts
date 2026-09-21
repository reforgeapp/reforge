import { test, type Page } from '@playwright/test'
import { mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'

const org = '00000000-0000-4000-8000-000000000001'
const routes = ['overview', 'repositories', 'findings', 'runs', 'changes', 'deployments', 'campaigns', 'policies', 'connections', 'runners', 'usage', 'audit', 'organisation']
const outDir = process.env.REFORGE_VISUAL_DIR || join(process.cwd(), '..', '.local', 'visual')

test.describe('T29 visual captures', () => {
  test.skip(process.env.REFORGE_VISUAL !== '1', 'set REFORGE_VISUAL=1 to capture route baselines')

  test('captures every route with route and runtime metadata', async ({ page, browser }) => {
    mkdirSync(outDir, { recursive: true })
    const runtime = await page.evaluate(() => navigator.userAgent).catch(() => 'unknown')
    let meta: Record<string, unknown> = {}
    await page.goto('/api/v1/meta')
    try { meta = JSON.parse(await page.locator('body').innerText()) as Record<string, unknown> } catch { meta = {} }
    await page.goto('/auth/login')
    await page.waitForURL(/\/overview/)
    const captures: Array<Record<string, unknown>> = []
    const sizes = [[1440, 900], [390, 844]] as const
    for (const route of routes) for (const [width, height] of sizes) {
      await page.setViewportSize({ width, height })
      await page.goto(`/org/${org}/${route}`)
      await page.waitForTimeout(700)
      const file = `${route}-${width}.png`
      await page.screenshot({ path: join(outDir, file), fullPage: true })
      const title = await page.locator('h1').first().innerText().catch(() => '')
      const tables = await page.locator('table').count()
      captures.push({ route, viewport: `${width}x${height}`, title, tables, file, captured_at: new Date().toISOString() })
    }
    const metadata = { product: meta.name ?? 'Reforge', version: meta.version ?? 'unknown', edition: meta.edition ?? 'unknown', development: meta.development ?? false, browser: browser.browserType().name(), browser_version: browser.version(), runtime, route_count: routes.length, captures }
    writeFileSync(join(outDir, 'metadata.json'), JSON.stringify(metadata, null, 2))
  })
})
