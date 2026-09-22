import { test, expect, type Page } from '@playwright/test'
import { readFileSync, writeFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { createRepairFixture } from './helpers/repair-fixture'
import { configureRepairAuthority } from './helpers/repair-authority'

test.use({ trace: 'off' })
const retainDemo = process.env.REFORGE_KEEP_DEMO === '1'

test('executes a real repair and exposes native publication evidence', async ({ page }) => {
  test.skip(process.env.REFORGE_LIVE_REPAIR_BROWSER !== '1', 'requires disposable Gitea, Ollama, runner and gVisor')
  test.setTimeout(360_000)
  if (process.env.REFORGE_ISOLATED_BROWSER_DATABASE !== '1' || process.env.REFORGE_BASE_URL !== 'http://127.0.0.1:8081') throw new Error('Full repair browser test requires disposable controller/database')
  await page.setViewportSize({ width: 390, height: 844 })
  await signIn(page)
  const fixture = await createRepairFixture(page)
  if (process.env.REFORGE_DEMO_MANIFEST) {
    const manifestPath = process.env.REFORGE_DEMO_MANIFEST
    const prior = JSON.parse(readFileSync(manifestPath, 'utf8')) as Record<string, unknown>
    writeFileSync(manifestPath, `${JSON.stringify({ ...prior, org_id: fixture.orgID, repository_id: fixture.repositoryID, native_full_name: fixture.nativeFullName, runner_id: fixture.runnerID, runner_pid: fixture.runnerPID, runner_starttime: fixture.runnerStarttime, runner_credentials: fixture.runnerCredentials, fixture_directory: fixture.fixtureDirectory }, null, 2)}\n`, { mode: 0o600 })
  }
  let authority: Awaited<ReturnType<typeof configureRepairAuthority>> | undefined
  try {
    authority = await configureRepairAuthority(page, fixture)
    const finding = await findFinding(page, fixture.repositoryID)
    await page.goto(`/org/${fixture.orgID}/findings?finding=${encodeURIComponent(finding.id)}`)
    const details = page.getByRole('region', { name: finding.title })
    await expect(details).toBeVisible()
    await expect.poll(async () => details.evaluate(element => element.contains(document.activeElement))).toBe(true)
    await expect.poll(async () => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    await details.getByRole('combobox', { name: 'Recipe' }).selectOption('go')
    await details.getByRole('combobox', { name: 'Model' }).selectOption(authority.modelID)
    await details.getByRole('combobox', { name: 'Runner pool' }).selectOption(fixture.poolID)
    const previewResponse = page.waitForResponse(response => response.request().method() === 'POST' && response.url().includes(`/api/v1/orgs/${fixture.orgID}/repair-preview`), { timeout: 90_000 })
    await details.getByRole('button', { name: 'Generate preview' }).click()
    const previewResult = await previewResponse
    expect(previewResult.ok()).toBe(true)
    const preview = details.getByRole('group', { name: 'Repair preview' })
    await expect(preview).toBeVisible({ timeout: 30_000 })
    await expect(preview).toContainText('Head')
    await expect(preview).toContainText('Target')
    await expect(preview).toContainText('Limits')
    await expect(preview).toContainText('go test')
    await expect(preview.getByRole('button', { name: 'Confirm and queue repair' })).toBeEnabled()
    await page.keyboard.press('Tab')
    await page.keyboard.press('Shift+Tab')
    await preview.getByRole('button', { name: 'Confirm and queue repair' }).click()
    const runLink = page.getByRole('link', { name: /Queued run/ })
    await expect(runLink).toBeVisible({ timeout: 15_000 })
    const href = await runLink.getAttribute('href')
    const taskID = new URL(href!, 'http://127.0.0.1').searchParams.get('run')
    expect(taskID).toBeTruthy()
    if (process.env.REFORGE_DEMO_MANIFEST) {
      const manifestPath = process.env.REFORGE_DEMO_MANIFEST
      const prior = JSON.parse(readFileSync(manifestPath, 'utf8')) as Record<string, unknown>
      writeFileSync(manifestPath, `${JSON.stringify({ ...prior, run_id: taskID, run_url: `${process.env.REFORGE_BASE_URL}/org/${fixture.orgID}/runs?run=${taskID}` }, null, 2)}\n`, { mode: 0o600 })
    }
    await runLink.click()
    const run = page.getByRole('region', { name: /Run / })
    await expect(run).toBeVisible()
    let detail: { task: { state: string; reason?: string }; report?: { reason?: string; baseline?: unknown[]; candidate?: unknown[]; target?: unknown[] } }
    const deadline = Date.now() + 300_000
    for (;;) {
      detail = await page.request.get(`/api/v1/orgs/${fixture.orgID}/repair-runs/${taskID}`).then(response => response.json() as Promise<typeof detail>)
      if (detail.task.state === 'completed') break
      if (['failed', 'blocked', 'cancelled'].includes(detail.task.state)) throw new Error(`repair terminal ${detail.task.state}: ${`${detail.task.reason ?? 'no task reason'}; ${detail.report?.reason ?? 'no report reason'}`}; checks=${JSON.stringify({ baseline: detail.report?.baseline?.length ?? 0, candidate: detail.report?.candidate?.length ?? 0, target: detail.report?.target?.length ?? 0 })}`)
      if (Date.now() >= deadline) throw new Error(`repair did not complete; state=${detail.task.state}; reason=${detail.task.reason ?? detail.report?.reason ?? 'none'}`)
      await page.waitForTimeout(1000)
    }
    const runData = await page.request.get(`/api/v1/orgs/${fixture.orgID}/repair-runs/${taskID}`).then(response => response.json() as Promise<{ candidate_sha: string; change?: { url: string }; candidate_artifacts?: string[] }>)
    await expect(run).toContainText('Native change lifecycle:')
    await expect(run).toContainText('Source diff')
    await expect(run).toContainText('Download authorized artifact')
    expect(runData.change?.url).toMatch(/^https?:\/\//)
    expect(runData.candidate_artifacts?.length).toBeGreaterThan(0)
    const artifact = run.getByRole('link', { name: /Download authorized artifact/ }).first()
    const artifactHref = await artifact.getAttribute('href')
    expect(artifactHref).toBeTruthy()
    expect((await page.request.get(artifactHref!)).status()).toBe(200)
    const botToken = readFileSync(resolve(process.cwd(), '..', '.local/gitea/reforge-bot.token'), 'utf8').trim()
    const native = await fetch(`http://127.0.0.1:53000/api/v1/repos/${fixture.nativeFullName}/pulls?state=all`, { headers: { Authorization: `token ${botToken}` } }).then(response => response.json() as Promise<Array<{ head: { sha: string }; html_url?: string }>>)
    const publication = native.filter(pull => pull.head.sha === runData.candidate_sha)
    expect(publication).toHaveLength(1)
    const manifestPath = process.env.REFORGE_DEMO_MANIFEST
    if (manifestPath) {
      const prior = JSON.parse(readFileSync(manifestPath, 'utf8')) as Record<string, unknown>
      writeFileSync(manifestPath, `${JSON.stringify({
        ...prior,
        controller_url: process.env.REFORGE_BASE_URL,
        org_id: fixture.orgID,
        repository_id: fixture.repositoryID,
        native_full_name: fixture.nativeFullName,
        runner_id: fixture.runnerID,
        runner_pid: fixture.runnerPID,
        runner_starttime: fixture.runnerStarttime,
        runner_credentials: fixture.runnerCredentials,
        fixture_directory: fixture.fixtureDirectory,
        run_id: taskID,
        run_url: `${process.env.REFORGE_BASE_URL}/org/${fixture.orgID}/runs?run=${taskID}`,
        native_change_url: publication[0].html_url ?? runData.change?.url,
        candidate_sha: runData.candidate_sha,
        status: 'completed',
      }, null, 2)}\n`, { mode: 0o600 })
    }
  } finally {
    if (!retainDemo) {
      try { await authority?.cleanup() } finally { await fixture.cleanup() }
    }
  }
})

async function signIn(page: Page) {
  await page.goto('/auth/login')
  await expect(page).toHaveURL(/\/org\/[^/]+\/overview/)
}

async function findFinding(page: Page, repositoryID: string) {
  const orgID = process.env.REFORGE_LIVE_REPAIR_ORG ?? '00000000-0000-4000-8000-000000000001'
  let item: { id: string; title: string; version: number } | undefined
  await expect.poll(async () => {
    const response = await page.request.get(`/api/v1/orgs/${orgID}/findings?limit=100&repository_id=${encodeURIComponent(repositoryID)}&state=open&category=ci_failure`)
    expect(response.ok()).toBe(true)
    item = ((await response.json()) as { items?: Array<{ id: string; title: string; version: number }> }).items?.[0]
    return Boolean(item)
  }, { timeout: 90_000, intervals: [1000, 2500, 5000] }).toBe(true)
  expect(item?.id).toBeTruthy()
  return item!
}
