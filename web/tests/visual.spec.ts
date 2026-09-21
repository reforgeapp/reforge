import { test, type Page } from '@playwright/test'
import { mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'

const org = '00000000-0000-4000-8000-000000000001'
const routes = ['overview', 'repositories', 'findings', 'runs', 'changes', 'deployments', 'campaigns', 'policies', 'connections', 'runners', 'usage', 'audit', 'organisation']
const outDir = process.env.REFORGE_VISUAL_DIR || join(process.cwd(), '..', '.local', 'visual')

test.describe('T29 visual captures', () => {
  test.skip(process.env.REFORGE_VISUAL !== '1', 'set REFORGE_VISUAL=1 to capture route baselines')

  test('captures every route with route and runtime metadata', async ({ page, browser }) => {
    test.setTimeout(180_000)
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
      const titles = await page.locator('h1').count()
      const toolbars = await page.locator('.repository-toolbar').count()
      const cardHeadings = await page.locator('.state-card h2').count()
      const tables = await page.locator('table').count()
      if (titles !== 1) throw new Error(`${route} must have exactly one page title, found ${titles}`)
      if (toolbars > 1) throw new Error(`${route} must have at most one primary toolbar, found ${toolbars}`)
      captures.push({ route, viewport: `${width}x${height}`, title, titles, toolbars, card_headings: cardHeadings, tables, file, captured_at: new Date().toISOString() })
    }
    const errorPage = await page.context().newPage()
    await errorPage.setViewportSize({ width: 1440, height: 900 })
    await errorPage.goto('/auth/login')
    await errorPage.waitForURL(/\/overview/)
    await errorPage.route('**/api/v1/orgs/**', route => route.abort('failed'))
    for (const route of routes) {
      await errorPage.goto(`/org/${org}/${route}`)
      await errorPage.getByRole('alert').or(errorPage.getByText(/unavailable|could not be loaded|failed to load/i)).first().waitFor({ state: 'visible', timeout: 10_000 }).catch(() => { throw new Error(`${route} error state is not actionable`) })
      const file = `${route}-error-1440.png`
      await errorPage.screenshot({ path: join(outDir, file), fullPage: true })
      captures.push({ route, viewport: '1440x900', state: 'error', file, actionable: true, captured_at: new Date().toISOString() })
    }
    await errorPage.close()
    const metadata = { product: meta.name ?? 'Reforge', version: meta.version ?? 'unknown', edition: meta.edition ?? 'unknown', development: meta.development ?? false, browser: browser.browserType().name(), browser_version: browser.version(), runtime, route_count: routes.length, captures }
    writeFileSync(join(outDir, 'metadata.json'), JSON.stringify(metadata, null, 2))
  })
})
