import { createHash } from 'node:crypto'
import { readFile, mkdir, writeFile } from 'node:fs/promises'
import os from 'node:os'
import { chromium } from '@playwright/test'

const baseURL = process.env.REFORGE_BASE_URL ?? 'http://127.0.0.1:8080'
const sessions = 50
const rounds = 5
const deadlineMs = 180_000
const started = Date.now()
const timestamp = new Date().toISOString().replaceAll(':', '-').replaceAll('.', '-')
const outputDir = process.env.REFORGE_LOAD_DIR ?? `.local/t28b-2026-09-23/${timestamp}`
const samples = []
const errors = []
const responseFailures = []
const contexts = []
let retries = 0
let browser

function percentile(values, p) {
  if (!values.length) return null
  const sorted = [...values].sort((a, b) => a - b)
  return Number(sorted[Math.ceil(p * sorted.length) - 1].toFixed(2))
}

async function hashAsset(url) {
  const response = await fetch(url)
  if (!response.ok) throw new Error(`asset ${url} returned ${response.status}`)
  return { url: new URL(url).pathname, sha256: createHash('sha256').update(Buffer.from(await response.arrayBuffer())).digest('hex') }
}

async function readLimits() {
  const paths = ['/proc/self/cgroup', '/sys/fs/cgroup/memory.max', '/sys/fs/cgroup/cpu.max', '/sys/fs/cgroup/pids.max']
  const values = {}
  for (const path of paths) {
    try { values[path] = (await readFile(path, 'utf8')).trim() } catch { values[path] = null }
  }
  return values
}

async function runRequest(context, path, type, round) {
  const start = performance.now()
  try {
    const response = await context.request.get(new URL(path, baseURL).href, { timeout: 15_000 })
    const elapsedMs = performance.now() - start
    const body = await response.json()
    samples.push({ type, round, status: response.status(), elapsed_ms: Number(elapsedMs.toFixed(2)), cache_control: response.headers()['cache-control'] ?? null })
    if (!response.ok()) errors.push({ type, round, status: response.status(), body })
    return { response, body }
  } catch (error) {
    samples.push({ type, round, status: 0, elapsed_ms: Number((performance.now() - start).toFixed(2)) })
    errors.push({ type, round, error: String(error) })
    return { response: null, body: null }
  }
}

async function main() {
  await mkdir(outputDir, { recursive: true })
  const metaResponse = await fetch(new URL('/api/v1/meta', baseURL))
  const meta = await metaResponse.json()
  if (!metaResponse.ok || meta.development !== true || meta.fixture_auth !== true) throw new Error('refusing run: expected explicit local development fixture authentication')
  const health = await fetch(new URL('/readyz', baseURL))
  if (!health.ok) throw new Error(`database readiness failed: ${health.status}`)
  const executablePath = process.env.PLAYWRIGHT_CHROMIUM_PATH
  browser = await chromium.launch({ headless: true, ...(executablePath ? { executablePath } : {}) })
  const assets = await hashAsset(new URL('/', baseURL).href)
  const htmlResponse = await fetch(new URL('/', baseURL))
  const html = await htmlResponse.text()
  const assetPaths = [...html.matchAll(/(?:src|href)="([^"]+\.(?:js|css))"/g)].map(match => new URL(match[1], baseURL).href)
  const assetHashes = await Promise.all(assetPaths.map(hashAsset))
  const environment = { captured_at: new Date().toISOString(), base_url: baseURL, product_meta: meta, page_html_sha256: assets.sha256, built_assets: assetHashes, node: process.version, platform: `${os.type()} ${os.release()} ${os.arch()}`, cpu_count: os.cpus().length, total_memory_bytes: os.totalmem(), free_memory_bytes: os.freemem(), cgroup_limits: await readLimits(), chromium_version: browser.version(), configured_executable: executablePath ?? null }
  const orgId = '00000000-0000-4000-8000-000000000001'
  const identities = []
  for (let i = 0; i < sessions; i += 1) {
    if (Date.now() - started > deadlineMs) throw new Error('time budget exceeded during authentication setup')
    const context = await browser.newContext()
    context.setDefaultTimeout(12_000)
    context.on('response', response => { if (response.status() >= 400) responseFailures.push({ url: response.url(), status: response.status() }) })
    contexts.push(context)
    const page = await context.newPage()
    const login = await page.goto(new URL('/auth/login', baseURL).href, { waitUntil: 'domcontentloaded', timeout: 15_000 })
    if (!login?.ok()) throw new Error(`fixture login failed for session ${i + 1}: ${login?.status() ?? 'no response'}`)
    const sessionResponse = await context.request.get(new URL('/api/v1/session', baseURL).href)
    if (!sessionResponse.ok()) throw new Error(`session validation failed for session ${i + 1}: ${sessionResponse.status()}`)
    const session = await sessionResponse.json()
    if (!session.user?.id || !session.organisations?.some(item => item.id === orgId)) throw new Error(`session ${i + 1} lacks expected seeded organisation`)
    const sessionCookie = (await context.cookies()).find(cookie => cookie.name.includes('session'))
    if (!sessionCookie?.value) throw new Error(`session ${i + 1} has no authenticated session cookie`)
    identities.push({ user_id: session.user.id, session_cookie_sha256: createHash('sha256').update(sessionCookie.value).digest('hex') })
  }
  const pageResults = await Promise.all(contexts.map(async (context, index) => {
    const page = context.pages()[0]
    const response = await page.goto(new URL(`/org/${orgId}/repositories`, baseURL).href, { waitUntil: 'domcontentloaded', timeout: 20_000 })
    if (!response?.ok()) throw new Error(`portfolio page failed for session ${index + 1}: ${response?.status() ?? 'no response'}`)
    await page.getByRole('heading', { name: 'Repositories', exact: true }).waitFor({ state: 'visible' })
    const rows = page.getByRole('region', { name: 'Repository inventory' }).locator('tbody tr')
    await rows.first().waitFor({ state: 'visible' })
    const count = await rows.count()
    const detailButton = rows.first().locator('button')
    const repositoryName = await detailButton.innerText()
    const repoIdResponse = await context.request.get(new URL(`/api/v1/orgs/${orgId}/repositories?limit=50`, baseURL).href)
    const list = await repoIdResponse.json()
    const repository = list.items?.[0]
    if (!repoIdResponse.ok() || !repository?.id || !count) throw new Error(`session ${index + 1} did not reach real persisted portfolio data`)
    return { context, page, repository_id: repository.id, repository_name: repositoryName, visible_rows: count }
  }))
  const repositoryIds = [...new Set(pageResults.map(result => result.repository_id))]
  if (repositoryIds.length !== 1) throw new Error(`fixture portfolio differed between sessions: ${repositoryIds.length} first repositories`)
  const orgPath = `/api/v1/orgs/${orgId}`
  const repoID = repositoryIds[0]
  const listPath = `${orgPath}/repositories?limit=50`
  const detailPath = `${orgPath}/repositories/${encodeURIComponent(repoID)}`
  for (const result of pageResults) {
    const list = await runRequest(result.context, listPath, 'list_warmup', -1)
    const detail = await runRequest(result.context, detailPath, 'detail_warmup', -1)
    if (!list.response?.ok() || !Array.isArray(list.body?.items) || !detail.response?.ok() || detail.body?.id !== repoID) throw new Error('warm-up read failed for a real portfolio endpoint')
  }
  for (let round = 0; round < rounds; round += 1) {
    if (Date.now() - started > deadlineMs) throw new Error('time budget exceeded during load rounds')
    const requests = pageResults.flatMap(result => [runRequest(result.context, listPath, 'list', round), runRequest(result.context, detailPath, 'detail', round)])
    await Promise.all(requests)
  }
  for (const result of pageResults) await result.page.title()
  retries = 0
  const successful = samples.filter(sample => sample.round >= 0 && sample.status >= 200 && sample.status < 300)
  const summary = {}
  for (const type of ['list', 'detail']) {
    const values = successful.filter(sample => sample.type === type).map(sample => sample.elapsed_ms)
    summary[type] = { count: values.length, errors: samples.filter(sample => sample.type === type && sample.status !== 200).length, p50_ms: percentile(values, 0.5), p95_ms: percentile(values, 0.95), max_ms: values.length ? Math.max(...values) : null }
  }
  const uniqueSessionCookies = new Set(identities.map(identity => identity.session_cookie_sha256)).size
  const targetMet = contexts.length === sessions && uniqueSessionCookies === sessions && responseFailures.length === 0 && errors.length === 0 && ['list', 'detail'].every(type => summary[type].count === sessions * rounds && summary[type].errors === 0 && summary[type].p95_ms !== null && summary[type].p95_ms < 500)
  const report = { status: errors.length || responseFailures.length > 0 ? 'failed' : targetMet ? 'target_met_local' : 'target_missed_local', claim_scope: '50 real Chromium browser contexts with independent local development fixture sessions, against running Go/PostgreSQL app; no intercepted or mocked APIs; no external providers called', target: 'cached portfolio list/detail API p95 <500ms at 50 concurrent interactive sessions', cache_note: 'API currently returns Cache-Control: no-store; samples are warm repeated persisted reads, not HTTP cache hits', environment, limits: { sessions, measurement_rounds: rounds, parallel_api_calls_per_round: sessions * 2, deadline_ms: deadlineMs, wall_time_ms: Date.now() - started }, auth: { mode: 'explicit local development fixture authentication', unique_contexts: contexts.length, unique_session_cookies: uniqueSessionCookies, user_ids: [...new Set(identities.map(identity => identity.user_id))] }, portfolio: { organisation_id: orgId, distinct_repository_ids: repositoryIds, min_visible_rows: Math.min(...pageResults.map(result => result.visible_rows)), max_visible_rows: Math.max(...pageResults.map(result => result.visible_rows)) }, summary, errors: { request_failures: errors, http_failures: responseFailures, harness_retry_attempts: retries }, samples }
  await writeFile(`${outputDir}/report.json`, `${JSON.stringify(report, null, 2)}\n`)
  process.stdout.write(`${JSON.stringify({ status: report.status, output: `${outputDir}/report.json`, sessions, summary, errors: report.errors, wall_time_ms: report.limits.wall_time_ms }, null, 2)}\n`)
  if (report.status !== 'target_met_local') process.exitCode = 1
}

try {
  await main()
} catch (error) {
  await mkdir(outputDir, { recursive: true })
  await writeFile(`${outputDir}/failure.json`, `${JSON.stringify({ status: 'failed', reason: String(error), sessions_opened: contexts.length, wall_time_ms: Date.now() - started, errors, response_failures: responseFailures }, null, 2)}\n`)
  process.stderr.write(`${String(error)}\nFailure evidence: ${outputDir}/failure.json\n`)
  process.exitCode = 1
} finally {
  await Promise.allSettled(contexts.map(context => context.close()))
  await browser?.close().catch(() => {})
}
