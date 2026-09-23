import { test, expect, type Page } from '@playwright/test'
import { spawn, type ChildProcess } from 'node:child_process'
import { mkdir, readFile, writeFile, rm } from 'node:fs/promises'
import { resolve } from 'node:path'

const org = '00000000-0000-4000-8000-000000000001'
const targetURL = 'http://127.0.0.1:8091'
const enabled = process.env.REFORGE_REVIEWER_ACCEPTANCE_URL === targetURL && process.env.REFORGE_BASE_URL === targetURL
test.use({ trace: 'off' })
const root = process.cwd().endsWith('/web') ? resolve(process.cwd(), '..') : process.cwd()
const artifacts = resolve(root, '.local/t28a-reviewer-journey')
type NativeFixture = { repository: string; repo_id: number; default_branch: string; target_sha: string; branch: string; head_sha: string; change_id: string; url: string }

test('connected reviewer evidence stays blocked without merge qualification', async ({ page: initialPage }) => {
  let page = initialPage
  test.skip(!enabled, 'requires isolated Go/PostgreSQL server at 127.0.0.1:8091 and disposable Gitea 1.27.3')
  test.setTimeout(240_000)
  await mkdir(artifacts, { recursive: true })
  let fixture: NativeFixture
  const steps: string[] = []
  const mark = async (step: string) => { steps.push(`${new Date().toISOString()} ${step}`); await writeFile(resolve(artifacts, 'steps.log'), steps.join('\n') + '\n') }
  const endpoint = 'http://127.0.0.1:53000/api/v1'
  const adminToken = (await readFile(resolve(root, '.local/gitea/reforge-admin.token'), 'utf8')).trim()
  const botToken = (await readFile(resolve(root, '.local/gitea/reforge-bot.token'), 'utf8')).trim()
  const reviewerToken = (await readFile(resolve(root, '.local/gitea/reforge-reviewer.token'), 'utf8')).trim()
  const inspectorToken = (await readFile(resolve(root, '.local/gitea/reforge-inspector.token'), 'utf8')).trim()
  const runDir = resolve(artifacts, 'runner')
  await mkdir(runDir, { recursive: true, mode: 0o700 })
  const enrollmentFile = resolve(runDir, 'enrollment-token')
  const credentialFile = resolve(runDir, 'runner-credentials')
  let connector: ChildProcess | undefined
  let connectorExit: number | null = null
  const nativeCall = async (method: string, path: string, token: string, body?: unknown) => {
    const response = await fetch(`${endpoint}${path}`, { method, headers: { Authorization: `token ${token}`, 'Content-Type': 'application/json' }, ...(body === undefined ? {} : { body: JSON.stringify(body) }) })
    if (!response.ok) throw new Error(`disposable Gitea ${method} ${path} HTTP${response.status}`)
    return response.status === 204 ? undefined : response.json() as Promise<Record<string, unknown>>
  }
  const createNativeFixture = async (): Promise<NativeFixture> => {
    const name = `reviewer-journey-${Date.now()}`
    const repo = `reforge-bot/${name}`
    const created = await nativeCall('POST', '/admin/users/reforge-bot/repos', adminToken, { name, private: true, auto_init: true, default_branch: 'main', description: 'Disposable reviewer acceptance fixture' }) as { id: number }
    const main = await nativeCall('GET', `/repos/${repo}/branches/main`, botToken) as { commit: { id: string } }
    await nativeCall('POST', `/repos/${repo}/branches`, botToken, { new_branch_name: 'reviewer', old_branch_name: 'main' })
    const content = await nativeCall('POST', `/repos/${repo}/contents/reviewer-fixture.txt`, botToken, { message: 'Add disposable reviewer change', content: 'cmV2aWV3ZXIgam91cm5leSBmaXh0dXJlCg==', branch: 'reviewer' }) as { commit: { sha: string } }
    const pull = await nativeCall('POST', `/repos/${repo}/pulls`, botToken, { title: 'Review disposable acceptance change', head: 'reviewer', base: 'main', body: 'Disposable local protected reviewer acceptance.' }) as { number: number; html_url: string }
    await nativeCall('PUT', `/repos/${repo}/collaborators/reforge-reviewer`, adminToken, { permission: 'write' })
    await nativeCall('POST', `/repos/${repo}/branch_protections`, adminToken, { rule_name: 'main', enable_push: false, required_approvals: 1, dismiss_stale_approvals: true, block_on_outdated_branch: true, block_on_rejected_reviews: true, block_on_official_review_requests: true, block_admin_merge_override: true, enable_status_check: false })
    return { repository: repo, repo_id: created.id, default_branch: 'main', target_sha: main.commit.id, branch: 'reviewer', head_sha: content.commit.sha, change_id: String(pull.number), url: pull.html_url }
  }
  const connection = async (name: string, secret: string, runnerName: string, poolName: string) => {
    await page.goto(`/org/${org}/connections`)
    await page.getByRole('button', { name: 'Add connection' }).click()
    const form = page.getByRole('dialog', { name: 'Add connection' })
    await mark(`${name}: form open`)
    await form.getByLabel('Provider').selectOption('gitea')
    await form.getByLabel('Name', { exact: true }).fill(name)
    await form.getByLabel('Endpoint').fill('http://127.0.0.1:53000')
    await form.getByLabel('Secret').fill(secret)
    await form.getByLabel('Private route via enrolled runner').check()
    const poolSelect = form.getByRole('combobox', { name: 'Runner pool' })
    await expect.poll(async () => poolSelect.locator('option').allTextContents()).toContain(poolName)
    await poolSelect.selectOption({ label: poolName })
    const runnerSelect = form.getByRole('combobox', { name: 'Runner', exact: true })
    await expect.poll(async () => runnerSelect.locator('option').allTextContents(), { timeout: 20_000 }).toContain(`${runnerName} · ${poolName}`)
    await runnerSelect.selectOption({ label: `${runnerName} · ${poolName}` })
    await form.getByLabel('Route host').fill('127.0.0.1')
    await form.getByLabel('Approved CIDRs').fill('127.0.0.1/32')
    await mark(`${name}: route selected`)
    await form.getByRole('button', { name: 'Create connection' }).click()
    await expect(form).toBeHidden({ timeout: 15_000 })
    await mark(`${name}: created`)
    const response = await page.request.get(`/api/v1/orgs/${org}/connections?limit=100`)
    expect(response.ok()).toBeTruthy()
    const items = (await response.json() as { items?: Array<{ id: string; name: string }> }).items ?? []
    const created = items.find(item => item.name === name)
    expect(created).toBeTruthy()
    const row = page.getByRole('row', { name: new RegExp(name) })
    await expect(row).toBeVisible()
    await row.getByRole('button', { name: 'Open' }).click()
    const probeResponse = page.waitForResponse(response => response.request().method() === 'POST' && response.url().includes(`/connections/${created!.id}/test`))
    await page.getByRole('button', { name: 'Test capability' }).click()
    const probe = await probeResponse
    expect(probe.status()).toBe(200)
    const verified = await probe.json() as { state: string; reason?: string }
    await mark(`${name}: provider probe returned ${verified.state}${verified.reason ? ` (${verified.reason})` : ''}`)
    expect(verified.state, 'newly tested forge connection must be healthy before inventory sync').toBe('healthy')
    return created!.id
  }

  try {
    fixture = await createNativeFixture()
    await mark('native fixture created')
    await writeFile(resolve(artifacts, 'native.json'), JSON.stringify(fixture, null, 2))
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto('/auth/login')
    await expect(page).toHaveURL(new RegExp(`/org/${org}/overview$`))
    const sessionResponse = await page.request.get('/api/v1/session')
    expect(sessionResponse.ok()).toBeTruthy()
    const session = await sessionResponse.json() as { csrf_token: string }
    await mark('fixture login completed')
    const [existingConnections, existingPools, existingRepositories] = await Promise.all([
      page.request.get(`/api/v1/orgs/${org}/connections?limit=100`),
      page.request.get(`/api/v1/orgs/${org}/runner-pools?limit=100`),
      page.request.get(`/api/v1/orgs/${org}/repositories?limit=100`),
    ])
    expect(existingConnections.ok() && existingPools.ok() && existingRepositories.ok()).toBeTruthy()
    const [connectionRows, poolRows, repositoryRows] = await Promise.all([existingConnections.json(), existingPools.json(), existingRepositories.json()]) as Array<{ items?: unknown[] }>
    expect([connectionRows.items?.length ?? 0, poolRows.items?.length ?? 0, repositoryRows.items?.length ?? 0], 'requires a fresh disposable Reforge database for this organization').toEqual([0, 0, 0])

    await page.goto(`/org/${org}/runners`)
    const poolName = `review-${Date.now()}`
    await page.getByRole('button', { name: 'Create pool' }).click()
    await page.getByLabel('Name').fill(poolName)
    await page.getByRole('button', { name: 'Save pool' }).click()
    const poolRow = page.getByRole('row', { name: new RegExp(poolName) })
    await expect(poolRow).toBeVisible()
    const pools = await page.request.get(`/api/v1/orgs/${org}/runner-pools?limit=100`).then(response => response.json() as Promise<{ items: Array<{ id: string; name: string }> }>)
    const pool = pools.items.find(item => item.name === poolName)
    expect(pool).toBeTruthy()
    await poolRow.getByRole('button', { name: 'Enroll runner' }).click()
    const enrollment = page.getByRole('dialog', { name: 'Runner enrollment token' })
    const token = await enrollment.getByLabel('One-use token').inputValue()
    await writeFile(enrollmentFile, token, { mode: 0o600 })
    await enrollment.getByRole('button', { name: 'Clear token' }).click()
    const runnerName = `review-${Date.now()}`
    await runCommand(resolve(root, 'bin/reforge-runner'), ['enroll', '--endpoint', targetURL, '--credentials', credentialFile, '--token-file', enrollmentFile, '--name', runnerName, '--development'], root)
    await mark('runner enrolled')
    connector = spawn(resolve(root, 'bin/reforge-runner'), ['connector', '--endpoint', targetURL, '--credentials', credentialFile, '--development'], { cwd: root, stdio: 'ignore' })
    connector.once('exit', code => { connectorExit = code })
    await expect.poll(async () => {
      const response = await page.request.get(`/api/v1/orgs/${org}/runner-pools/${pool!.id}/runners?limit=100`)
      if (!response.ok()) return ''
      const runners = await response.json() as { items?: Array<{ name: string; state: string }> }
      return runners.items?.find(item => item.name === runnerName)?.state ?? ''
    }, { timeout: 20_000 }).toBe('active')
    expect(connectorExit).toBeNull()
    await mark('runner connected')

    const forgeName = `review-forge-${Date.now()}`
    const forgeID = await connection(forgeName, botToken, runnerName, poolName)
    const inspectorID = await connection(`review-inspector-${Date.now()}`, inspectorToken, runnerName, poolName)
    expect(forgeID).not.toBe(inspectorID)
    await mark('forge connections healthy')
    await page.goto(`/org/${org}/connections`)
    await page.getByRole('button', { name: 'Refresh connections' }).click()
    const refreshedConnections = await page.request.get(`/api/v1/orgs/${org}/connections?kind=forge&limit=100`).then(response => response.json() as Promise<{ items: Array<{ name: string }> }>)
    expect(refreshedConnections.items.map(item => item.name)).toContain(forgeName)
    await mark('backend kind-filtered connection list includes latest Gitea connection')
    const foreignConnections = await page.request.get('/api/v1/orgs/00000000-0000-4000-8000-000000000002/connections?limit=100')
    expect([200, 403, 404]).toContain(foreignConnections.status())
    if (foreignConnections.ok()) {
      const foreignConnectionItems = await foreignConnections.json() as { items?: Array<{ name: string }> }
      expect(foreignConnectionItems.items?.some(item => item.name === forgeName)).toBeFalsy()
    }
    await mark('second tenant does not expose new Gitea connection')
    const pickerResponses: string[] = []
    page.on('response', async response => {
      if (!response.url().includes('/connections?') || !response.url().includes('kind=forge')) return
      try {
        const body = await response.json() as { items?: Array<{ name: string; state?: string }> }
        pickerResponses.push(`${response.status()}: ${(body.items ?? []).map(item => `${item.name} [${item.state ?? 'no-state'}]`).join(' | ')}`)
      } catch { pickerResponses.push(`${response.status()}: invalid response`) }
    })
    await page.goto(`/org/${org}/repositories`)
    await page.reload()
    await page.getByRole('button', { name: 'Sync inventory' }).click()
    let sync = page.getByRole('dialog', { name: 'Sync forge inventory' })
    let forgeSelect = sync.getByLabel('Forge connection')
    await expect.poll(async () => forgeSelect.locator('option').allTextContents(), { timeout: 15_000 }).toContain(`${forgeName} · gitea`)
    await mark('reloaded page picker contains latest healthy Gitea connection')
    await sync.getByRole('button', { name: 'Close' }).click()

    page = await page.context().newPage()
    await page.goto(`/org/${org}/repositories`)
    await page.getByRole('button', { name: 'Sync inventory' }).click()
    sync = page.getByRole('dialog', { name: 'Sync forge inventory' })
    forgeSelect = sync.getByLabel('Forge connection')
    await expect.poll(async () => forgeSelect.locator('option').allTextContents(), { timeout: 15_000 }).toContain(`${forgeName} · gitea`)
    await mark(`fresh browser page picker contains latest healthy Gitea connection; API: ${pickerResponses.join(' || ')}`)
    await forgeSelect.selectOption({ label: `${forgeName} · gitea` })
    await sync.getByRole('button', { name: 'Start preview' }).click()
    await expect(sync.getByText('complete', { exact: true })).toBeVisible({ timeout: 60_000 })
    await mark('native inventory scan completed')
    const candidate = sync.getByRole('checkbox', { name: new RegExp(fixture.repository) })
    await expect(candidate).toBeVisible({ timeout: 15_000 })
    await candidate.check()
    await sync.getByRole('button', { name: /Import selected/ }).click()
    await expect(sync.getByText('Import complete. Repository inventory will refresh.')).toBeVisible({ timeout: 90_000 })
    await mark('repository import completed')
    await sync.getByRole('button', { name: 'Close' }).click()

    const repoResponse = await page.request.get(`/api/v1/orgs/${org}/repositories?q=${encodeURIComponent(fixture.repository)}`)
    expect(repoResponse.ok()).toBeTruthy()
    const repositoryID = ((await repoResponse.json()) as { items: Array<{ id: string; name: string }> }).items.find(item => item.name === fixture.repository)?.id
    expect(repositoryID).toBeTruthy()
    const otherTenant = await page.request.get(`/api/v1/orgs/00000000-0000-4000-8000-000000000002/repositories?q=${encodeURIComponent(fixture.repository)}`)
    expect([200, 403, 404]).toContain(otherTenant.status())
    if (otherTenant.ok()) {
      const foreignRepositories = await otherTenant.json() as { items?: Array<{ name: string }> }
      expect(foreignRepositories.items?.some(item => item.name === fixture.repository)).toBeFalsy()
    }
    await mark('second tenant cannot read imported repository')
    const mergeConfig = await page.request.put(`/api/v1/orgs/${org}/repositories/${repositoryID}/merge-configuration`, {
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': session.csrf_token, Origin: targetURL, 'If-Match': '"0"' },
      data: { enabled: false, inspector_connection_id: inspectorID, check_publishers: {}, cooperation_reference: '', qualification: {} },
    })
    expect(mergeConfig.status(), await mergeConfig.text()).toBe(200)
    const changesURL = `/api/v1/orgs/${org}/repositories/${repositoryID}/changes?limit=100`
    await expect.poll(async () => {
      const response = await page.request.get(changesURL)
      if (!response.ok()) return ''
      const changes = await response.json() as { items?: Array<{ id: string; title: string }> }
      return changes.items?.find(item => item.id === fixture.change_id)?.title ?? ''
    }, { timeout: 60_000 }).toBe('Review disposable acceptance change')
    const beforeResponse = await page.request.get(changesURL)
    const beforeChanges = await beforeResponse.json() as { items: Array<{ id: string; head_sha: string; target_sha: string }> }
    const before = beforeChanges.items.find(item => item.id === fixture.change_id)
    expect(before?.head_sha).toBe(fixture.head_sha)
    expect(before?.target_sha).toBe(fixture.target_sha)
    await mark('native change observed')

    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto(`/org/${org}/changes?repository=${repositoryID}`)
    await expect(page.getByRole('heading', { name: 'Review disposable acceptance change' })).toBeVisible({ timeout: 30_000 })
    const preview = page.getByRole('button', { name: 'Preview merge gate' })
    await expect(preview).toBeEnabled({ timeout: 30_000 })
    await preview.focus()
    await expect(preview).toBeFocused()
    const previewResponse = page.waitForResponse(response => response.request().method() === 'POST' && response.url().includes(`/repositories/${repositoryID}/changes/${fixture.change_id}/merge-preview`))
    await page.keyboard.press('Enter')
    const firstGateResponse = await previewResponse
    expect(firstGateResponse.status(), await firstGateResponse.text()).toBe(200)
    const firstGate = await firstGateResponse.json() as { id: string; decision: { outcome: string; blockers: string[] }; binding: { head: string; target: string; tested: string }; snapshot: { native: { state: string; head_sha: string; target_sha: string }; rules: { state: string }; approvals: unknown[] } }
    expect(firstGate.binding).toMatchObject({ head: fixture.head_sha, target: fixture.target_sha, tested: fixture.head_sha })
    expect(firstGate.snapshot.native.head_sha).toBe(fixture.head_sha)
    expect(firstGate.snapshot.native.target_sha).toBe(fixture.target_sha)
    expect(firstGate.decision.outcome).not.toBe('allow')
    await mark('native gate inspected')
    await expect(page.getByText('H head', { exact: true })).toBeVisible()
    await expect(page.getByText('T target', { exact: true })).toBeVisible()
    await expect(page.getByText('Checked revision', { exact: true })).toBeVisible()
    await expect(page.getByText('Native authority', { exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Request protected merge' })).toBeDisabled()
    await page.screenshot({ path: resolve(artifacts, 'blocked-review-390.png'), fullPage: true })

    const priorFile = await nativeCall('GET', `/repos/${fixture.repository}/contents/reviewer-fixture.txt?ref=${fixture.branch}`, botToken) as { sha: string }
    const changed = await nativeCall('PUT', `/repos/${fixture.repository}/contents/reviewer-fixture.txt`, botToken, {
      message: 'Advance disposable reviewer head', content: 'cmV2aWV3ZXIgam91cm5leSBmaXh0dXJlIC0gYWR2YW5jZWQK', branch: fixture.branch, sha: priorFile.sha,
    }) as { commit: { sha: string } }
    expect(changed.commit.sha).not.toBe(fixture.head_sha)
    await page.reload()
    await expect(preview).toBeEnabled({ timeout: 30_000 })
    const secondResponse = page.waitForResponse(response => response.request().method() === 'POST' && response.url().includes(`/repositories/${repositoryID}/changes/${fixture.change_id}/merge-preview`))
    await preview.click()
    const secondGateResponse = await secondResponse
    expect(secondGateResponse.status(), await secondGateResponse.text()).toBe(200)
    const secondGate = await secondGateResponse.json() as { id: string; decision: { outcome: string }; binding: { head: string; target: string; tested: string }; snapshot: { native: { head_sha: string; target_sha: string } } }
    expect(secondGate.id).not.toBe(firstGate.id)
    expect(secondGate.binding).toMatchObject({ head: changed.commit.sha, target: fixture.target_sha, tested: changed.commit.sha })
    expect(secondGate.snapshot.native.head_sha).toBe(changed.commit.sha)
    expect(secondGate.decision.outcome).not.toBe('allow')
    const staleRequest = await page.request.post(`/api/v1/orgs/${org}/merge-operations`, {
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': session.csrf_token, Origin: targetURL },
      data: { gate_id: firstGate.id, idempotency_key: crypto.randomUUID() },
    })
    expect(staleRequest.status()).toBe(409)
    await expect(page.getByRole('button', { name: 'Request protected merge' })).toBeDisabled()
    await page.screenshot({ path: resolve(artifacts, 'stale-review-390.png'), fullPage: true })
    await mark('stale gate rejected')

    await nativeCall('POST', `/repos/${fixture.repository}/pulls/${fixture.change_id}/requested_reviewers`, botToken, { reviewers: ['reforge-reviewer'] })
    const pullWithRequest = await nativeCall('GET', `/repos/${fixture.repository}/pulls/${fixture.change_id}`, botToken) as { requested_reviewers?: Array<{ login: string }> }
    expect(pullWithRequest.requested_reviewers?.map(item => item.login)).toContain('reforge-reviewer')
    await mark('native reviewer request persisted')

    await page.reload()
    await expect(preview).toBeEnabled({ timeout: 30_000 })
    await preview.focus()
    const blockedAfterReloadResponse = page.waitForResponse(response => response.request().method() === 'POST' && response.url().includes(`/repositories/${repositoryID}/changes/${fixture.change_id}/merge-preview`))
    await page.keyboard.press('Enter')
    const blockedAfterReload = await blockedAfterReloadResponse
    expect(blockedAfterReload.status()).toBe(200)
    const blockedAgain = await blockedAfterReload.json() as { decision: { outcome: string }; snapshot: { approvals: Array<{ state: string }> } }
    expect(blockedAgain.decision.outcome).not.toBe('allow')
    expect(blockedAgain.snapshot.approvals.filter(approval => approval.state === 'approved')).toHaveLength(0)
    await expect(page.getByRole('button', { name: 'Request protected merge' })).toBeDisabled()

    await nativeCall('POST', `/repos/${fixture.repository}/pulls/${fixture.change_id}/reviews`, reviewerToken, {
      body: 'Approved by the disposable native reviewer.', commit_id: changed.commit.sha, event: 'APPROVED',
    })
    await mark('native reviewer approved current head')
    await page.reload()
    await expect(preview).toBeEnabled({ timeout: 30_000 })
    const approvedGateResponse = page.waitForResponse(response => response.request().method() === 'POST' && response.url().includes(`/repositories/${repositoryID}/changes/${fixture.change_id}/merge-preview`))
    await preview.click()
    const approvedGateHTTP = await approvedGateResponse
    expect(approvedGateHTTP.status(), await approvedGateHTTP.text()).toBe(200)
    const approvedGate = await approvedGateHTTP.json() as { id: string; decision: { outcome: string; blockers: string[] }; snapshot: { approvals: Array<{ state: string; head_sha: string; dismissed: boolean }>; native: { state: string }; rules: { state: string } } }
    expect(approvedGate.snapshot.approvals.filter(approval => approval.state === 'approved' && approval.head_sha === changed.commit.sha && !approval.dismissed)).toHaveLength(1)
    expect(approvedGate.decision.outcome).not.toBe('allow')
    await expect(page.getByRole('button', { name: 'Request protected merge' })).toBeDisabled()
    const approvedButUnqualified = await page.request.post(`/api/v1/orgs/${org}/merge-operations`, {
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': session.csrf_token, Origin: targetURL },
      data: { gate_id: approvedGate.id, idempotency_key: crypto.randomUUID() },
    })
    expect(approvedButUnqualified.status()).toBe(409)
    await mark('approval visible; merge remains blocked without operator qualification')

    await writeFile(resolve(artifacts, 'gate-evidence.json'), JSON.stringify({ repository: fixture.repository, change: fixture.change_id, target: fixture.target_sha, old_head: fixture.head_sha, refreshed_head: changed.commit.sha, first_outcome: firstGate.decision.outcome, first_blockers: firstGate.decision.blockers, first_native_state: firstGate.snapshot.native.state, first_rules_state: firstGate.snapshot.rules.state, refreshed_outcome: secondGate.decision.outcome, stale_gate_request_http: staleRequest.status(), blocked_after_reload: blockedAgain.decision.outcome, blocked_approved_reviews: blockedAgain.snapshot.approvals.filter(approval => approval.state === 'approved').length, native_review_request: 'reforge-reviewer', approved_reviews: approvedGate.snapshot.approvals.filter(approval => approval.state === 'approved' && approval.head_sha === changed.commit.sha && !approval.dismissed).length, approved_outcome: approvedGate.decision.outcome, approved_blockers: approvedGate.decision.blockers, approved_native_state: approvedGate.snapshot.native.state, approved_rules_state: approvedGate.snapshot.rules.state, merge_request_status: approvedButUnqualified.status(), native_merged: false, other_tenant_status: otherTenant.status() }, null, 2))
  } finally {
    if (connector && connector.exitCode === null) {
      connector.kill('SIGTERM')
      await new Promise(resolveExit => connector!.once('exit', resolveExit))
    }
    await rm(enrollmentFile, { force: true })
    await rm(credentialFile, { force: true })
    try { if (fixture!) await nativeCall('DELETE', `/repos/${fixture.repository}`, adminToken) } catch (error) { process.stderr.write(`disposable repository cleanup failed: ${error instanceof Error ? error.message : 'unknown'}\n`) }
  }
})

async function runCommand(command: string, args: string[], cwd: string) {
  await new Promise<void>((resolveCommand, rejectCommand) => {
    const child = spawn(command, args, { cwd, stdio: 'ignore' })
    const timer = setTimeout(() => { child.kill('SIGTERM'); rejectCommand(new Error('runner enrollment timed out')) }, 20_000)
    child.once('error', error => { clearTimeout(timer); rejectCommand(error) })
    child.once('exit', code => { clearTimeout(timer); if (code === 0) resolveCommand(); else rejectCommand(new Error(`runner enrollment exited with ${code ?? 'signal'}`)) })
  })
}
