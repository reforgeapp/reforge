import { spawn, type ChildProcess } from 'node:child_process'
import { appendFileSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import type { Page } from '@playwright/test'

type Identity = { id: string; version: number }
type Job = { id: string; version: number; state: string; kind: string; repository_id?: string; reason?: string }

export type RepairFixture = {
  orgID: string
  repositoryID: string
  repositoryName: string
  nativeFullName: string
  pinnedSHA: string
  poolID: string
  runnerID: string
  connectionID: string
  poolName: string
  runnerPID: number
  runnerStarttime: string
  runnerCredentials: string
  fixtureDirectory: string
  cleanup: () => Promise<void>
}

export async function createRepairFixture(page: Page): Promise<RepairFixture> {
  const orgID = process.env.REFORGE_LIVE_REPAIR_ORG ?? '00000000-0000-4000-8000-000000000001'
  const controller = controllerOrigin()
  if (!process.env.REFORGE_BROWSER_RUNTIME_CONFIG) throw new Error('Verified runtime config required')
  const root = resolve(process.cwd(), '..')
  const temp = mkdtempSync(join(tmpdir(), 'reforge-repair-fixture-'))
  const enrollmentFile = join(temp, 'enrollment-token')
  const credentialFile = join(temp, 'runner-credentials')
  const repositoryName = `browser-repair-${Date.now()}`
  const runnerName = `browser-repair-runner-${Date.now()}`
  const poolName = `browser-repair-pool-${Date.now()}`
  const adminToken = readFileSync(resolve(root, '.local/gitea/reforge-admin.token'), 'utf8').trim()
  const botToken = readFileSync(resolve(root, '.local/gitea/reforge-bot.token'), 'utf8').trim()
  const children: ChildProcess[] = []
  const childExit: Promise<void>[] = []
  let runnerStderr = ''
  let poolID = ''; let runnerID = ''; let connectionID = ''; let repositoryID = ''
  const session = await page.request.get('/api/v1/session').then(response => response.json() as Promise<{ csrf_token: string }>)
  const csrf = session.csrf_token
  const api = async <T>(method: string, path: string, data?: unknown, headers: Record<string, string> = {}) => {
    const response = await page.request.fetch(path, { method, data, timeout: 10_000, headers: { Accept: 'application/json', Origin: controller, ...(data ? { 'Content-Type': 'application/json' } : {}), ...(method !== 'GET' ? { 'X-CSRF-Token': csrf } : {}), ...headers } })
    if (!response.ok()) throw new Error(`repair fixture ${method} ${path} failed HTTP${response.status()}`)
    return response.status() === 204 ? undefined as T : await response.json() as T
  }
  const gitea = async (method: string, path: string, data?: unknown) => {
    const abort = new AbortController()
    const timer = setTimeout(() => abort.abort(), 10_000)
    let response: Response
    try {
      response = await fetch(`http://127.0.0.1:53000/api/v1${path}`, { method, signal: abort.signal, headers: { Authorization: `token ${botToken}`, ...(data ? { 'Content-Type': 'application/json' } : {}) }, body: data ? JSON.stringify(data) : undefined })
    } finally {
      clearTimeout(timer)
    }
    if (!response.ok) throw new Error(`repair fixture Gitea ${method} failed HTTP${response.status}`)
    return response.status === 204 ? undefined : await response.json() as Record<string, unknown>
  }
  try {
    const repo = await fetch('http://127.0.0.1:53000/api/v1/admin/users/reforge-bot/repos', { method: 'POST', signal: AbortSignal.timeout(10_000), headers: { Authorization: `token ${adminToken}`, 'Content-Type': 'application/json' }, body: JSON.stringify({ name: repositoryName, auto_init: false, private: true, default_branch: 'main' }) })
    if (!repo.ok) throw new Error(`repair fixture repository failed HTTP${repo.status}`)
    const native = await repo.json() as { id: number; full_name: string }
    for (const [name, body] of Object.entries({ 'go.mod': 'module example.test/repair\n\ngo 1.23\n', 'value.go': 'package value\n\nfunc Add(a, b int) int { return a - b }\n', 'value_test.go': 'package value\n\nimport "testing"\n\nfunc TestAdd(t *testing.T) { for _, tt := range []struct { a, b, want int }{{2, 3, 5}, {-4, 2, -2}} { if got := Add(tt.a, tt.b); got != tt.want { t.Errorf("Add(%d,%d)=%d; want %d", tt.a, tt.b, got, tt.want) } } }\n' })) {
      await gitea('POST', `/repos/${native.full_name}/contents/${name}`, { branch: 'main', message: 'Add repair fixture', content: Buffer.from(body).toString('base64') })
    }
    const branch = await gitea('GET', `/repos/${native.full_name}/branches/main`) as { commit: { id: string } }
    await gitea('POST', `/repos/${native.full_name}/statuses/${branch.commit.id}`, { state: 'failure', context: 'fixture/go-test', description: 'fixture test fails on the original revision' })
    const pool = await api<{ id: string }>('POST', `/api/v1/orgs/${orgID}/runner-pools`, { name: poolName, repository_ids: [] })
    poolID = pool.id
    const enrollment = await api<{ token: string }>('POST', `/api/v1/orgs/${orgID}/runner-pools/${poolID}/enrollments`)
    writeFileSync(enrollmentFile, enrollment.token, { mode: 0o600 })
    await run(join(root, 'bin/reforge-runner'), ['enroll', '--endpoint', controller, '--credentials', credentialFile, '--token-file', enrollmentFile, '--name', runnerName, '--development'], root)
    const retainDemo = process.env.REFORGE_KEEP_DEMO === '1'
    const connector = spawn(join(root, 'bin/reforge-runner'), ['run', '--endpoint', controller, '--credentials', credentialFile, '--runtime-config', process.env.REFORGE_BROWSER_RUNTIME_CONFIG!, '--development'], { cwd: root, detached: retainDemo, stdio: retainDemo ? ['ignore', 'ignore', 'ignore'] : ['ignore', 'ignore', 'pipe'] })
    children.push(connector); childExit.push(new Promise(resolveExit => connector.once('exit', () => resolveExit())))
    if (!retainDemo) connector.stderr?.on('data', chunk => { runnerStderr = (runnerStderr + String(chunk)).slice(-8192) })
    if (retainDemo) connector.unref()
    const runnersPage = await waitFor(async () => api<{ items: Array<{ id: string; name: string }> }>('GET', `/api/v1/orgs/${orgID}/runner-pools/${poolID}/runners?limit=100`), result => Boolean(result.items.find(item => item.name === runnerName)))
    runnerID = runnersPage.items.find(item => item.name === runnerName)!.id
    const connection = await api<{ id: string }>('POST', `/api/v1/orgs/${orgID}/connections/forges`, { kind: 'forge', provider: 'gitea', name: `browser-repair-forge-${Date.now()}`, endpoint: 'http://127.0.0.1:53000', secret: botToken, settings: { auth_kind: 'token', billing_route: 'forge' }, private_route: { runner_id: runnerID, host: '127.0.0.1', cidrs: ['127.0.0.1/32'] } })
    connectionID = connection.id
    await api('POST', `/api/v1/orgs/${orgID}/connections/${connectionID}/test`, undefined, { 'If-Match': '"1"' })
    const sync = await api<Job>('POST', `/api/v1/orgs/${orgID}/inventory-syncs`, { connection_id: connectionID })
    const complete = await waitJob(api, orgID, sync.id)
    const candidates = await api<{ items: Array<{ native_id: string }> }>('GET', `/api/v1/orgs/${orgID}/inventory-syncs/${complete.id}/candidates?limit=200`)
    const imported = await api<Job>('POST', `/api/v1/orgs/${orgID}/inventory-syncs/${complete.id}/import`, { native_ids: candidates.items.filter(item => item.native_id === String(native.id)).map(item => item.native_id) }, { 'If-Match': `"${complete.version}"` })
    await waitJob(api, orgID, imported.id)
    const repositoriesPage = await waitFor(async () => api<{ items: Array<{ id: string; name: string }> }>('GET', `/api/v1/orgs/${orgID}/repositories?limit=100`), result => Boolean(result.items.find(item => item.name === `reforge-bot/${repositoryName}`)))
    repositoryID = repositoriesPage.items.find(item => item.name === `reforge-bot/${repositoryName}`)!.id
    const poolPage = await api<{ items: Array<{ id: string; version: number }> }>('GET', `/api/v1/orgs/${orgID}/runner-pools?limit=100`)
    const poolData = poolPage.items.find(item => item.id === poolID)
    if (!poolData) throw new Error('repair fixture pool disappeared before repository assignment')
    await api('PUT', `/api/v1/orgs/${orgID}/runner-pools/${poolID}`, { name: poolName, repository_ids: [repositoryID] }, { 'If-Match': `"${poolData.version}"` })
    const maintenance = await api<{ version: number; trusted_bots: unknown[]; merge_authority: string }>('GET', `/api/v1/orgs/${orgID}/repositories/${repositoryID}/maintenance`)
    await api('PUT', `/api/v1/orgs/${orgID}/repositories/${repositoryID}/maintenance`, { repository_id: repositoryID, trusted_bots: maintenance.trusted_bots, merge_authority: maintenance.merge_authority, version: maintenance.version }, { 'If-Match': `"${maintenance.version}"` })
    await api<Job>('POST', `/api/v1/orgs/${orgID}/repositories/${repositoryID}/discovery`)
    await waitFor(async () => api<Job>('GET', `/api/v1/orgs/${orgID}/repositories/${repositoryID}/discovery`), result => result.state === 'complete')
    const runnerStarttime = retainDemo ? readFileSync(`/proc/${connector.pid}/stat`, 'utf8').split(/\s+/)[21] : ''
    return { orgID, repositoryID, repositoryName, nativeFullName: native.full_name, pinnedSHA: branch.commit.id, poolID, runnerID, connectionID, poolName, runnerPID: connector.pid!, runnerStarttime, runnerCredentials: credentialFile, fixtureDirectory: temp, cleanup: async () => cleanup() }
  } catch (error) {
    await cleanup()
    throw error
  }
  async function cleanup() {
    const errors: string[] = []
    try {
    if (runnerStderr) { try { appendFileSync(join(root, '.local/repair-browser-runner.log'), runnerStderr.replace(/[A-Za-z0-9+/_=-]{40,}/g, '[redacted]')) } catch (error) { errors.push(String(error)) } }
    if (connectionID) { const current = await api<{ version: number }>('GET', `/api/v1/orgs/${orgID}/connections/${connectionID}`).catch(error => { errors.push(String(error)); return undefined }); if (current) await api('DELETE', `/api/v1/orgs/${orgID}/connections/${connectionID}`, undefined, { 'If-Match': `"${current.version}"` }).catch(error => errors.push(String(error))) }
    if (runnerID) { const current = await api<{ items: Array<{ id: string; version: number }> }>('GET', `/api/v1/orgs/${orgID}/runner-pools/${poolID}/runners?limit=100`).catch(error => { errors.push(String(error)); return undefined }); const runner = current?.items.find(item => item.id === runnerID); if (runner) await api('DELETE', `/api/v1/orgs/${orgID}/runners/${runner.id}`, undefined, { 'If-Match': `"${runner.version}"` }).catch(error => errors.push(String(error))) }
    if (poolID) { const current = await api<{ items: Array<{ id: string; version: number; name: string; repository_ids: string[] }> }>('GET', `/api/v1/orgs/${orgID}/runner-pools?limit=100`).catch(error => { errors.push(String(error)); return undefined }); const pool = current?.items.find(item => item.id === poolID); if (pool) await api('PUT', `/api/v1/orgs/${orgID}/runner-pools/${pool.id}`, { name: pool.name, state: 'revoked', repository_ids: pool.repository_ids }, { 'If-Match': `"${pool.version}"` }).catch(error => errors.push(String(error))) }
    for (let index = 0; index < children.length; index += 1) await stopChild(children[index], childExit[index], errors)
    const abort = new AbortController()
    const timer = setTimeout(() => abort.abort(), 10_000)
    try {
      const deleted = await fetch(`http://127.0.0.1:53000/api/v1/repos/reforge-bot/${encodeURIComponent(repositoryName)}`, { method: 'DELETE', signal: abort.signal, headers: { Authorization: `token ${adminToken}` } }).catch(error => { errors.push(String(error)); return undefined })
      if (deleted && !deleted.ok && deleted.status !== 404) errors.push(`Gitea cleanup HTTP${deleted.status}`)
    } finally { clearTimeout(timer) }
    } catch (error) { errors.push(String(error)) } finally {
      for (let index = 0; index < children.length; index += 1) await stopChild(children[index], childExit[index], errors)
      try { rmSync(temp, { recursive: true, force: true }) } catch (error) { errors.push(String(error)) }
    }
    if (errors.length) throw new Error(`repair fixture cleanup failed: ${errors.join('; ')}`)
  }
}

function controllerOrigin() {
  const value = process.env.REFORGE_BASE_URL ?? 'http://127.0.0.1:8080'
  let parsed: URL
  try { parsed = new URL(value) } catch { throw new Error('REFORGE_BASE_URL must be a valid URL') }
  if (parsed.protocol !== 'http:' || parsed.hostname !== '127.0.0.1' || parsed.pathname !== '/' || parsed.search || parsed.hash) throw new Error('REFORGE_BASE_URL must be an HTTP loopback origin')
  return parsed.origin
}

async function waitJob(api: <T>(method: string, path: string, data?: unknown, headers?: Record<string, string>) => Promise<T>, orgID: string, id: string) { return waitFor(() => api<Job>('GET', `/api/v1/orgs/${orgID}/inventory-syncs/${id}`), result => result.state === 'complete') }
async function waitFor<T>(read: () => Promise<T>, done: (value: T) => unknown) { for (let attempt = 0; attempt < 120; attempt += 1) { const value = await read(); if (done(value)) return value; await new Promise(resolve => setTimeout(resolve, 1000)) } throw new Error('repair fixture did not converge') }
async function run(command: string, args: string[], cwd: string) { await new Promise<void>((resolveRun, rejectRun) => { const child = spawn(command, args, { cwd, stdio: 'ignore' }); const timer = setTimeout(() => { child.kill('SIGTERM'); rejectRun(new Error('repair fixture runner enrollment timed out')) }, 30_000); child.once('error', error => { clearTimeout(timer); rejectRun(error) }); child.once('exit', code => { clearTimeout(timer); code === 0 ? resolveRun() : rejectRun(new Error(`repair fixture runner enrollment exited ${code ?? 'signal'}`)) }) }) }
async function stopChild(child: ChildProcess, exited: Promise<void>, errors: string[]) {
  if (child.exitCode !== null) return
  try { child.kill('SIGTERM') } catch (error) { errors.push(String(error)); return }
  await Promise.race([exited, new Promise(resolve => setTimeout(resolve, 5_000))])
  if (child.exitCode === null) { try { child.kill('SIGKILL') } catch (error) { errors.push(String(error)) } }
  await Promise.race([exited, new Promise(resolve => setTimeout(resolve, 5_000))])
}
