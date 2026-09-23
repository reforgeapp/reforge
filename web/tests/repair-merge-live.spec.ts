import { test, expect, type Page } from '@playwright/test'
import { createHash } from 'node:crypto'
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { isAbsolute, resolve } from 'node:path'
import { createRepairFixture, type RepairFixture } from './helpers/repair-fixture'
import { configureRepairAuthority, type RepairAuthority } from './helpers/repair-authority'

test.use({ trace: 'off' })
const enabled = process.env.REFORGE_LIVE_REPAIR_BROWSER === '1' && process.env.REFORGE_LIVE_REPAIR_MERGE_BROWSER === '1'
const root = resolve(process.cwd(), '..')
const giteaAPI = 'http://127.0.0.1:53000/api/v1'
const method = 'fast-forward-only'
const qualificationReference = 'local-gitea-1.27.3-protection-contract'

type Gate = { id: string; expires_at: string; configuration_version: number; connection_version: number; decision: { outcome: string; blockers?: string[]; required_actions?: string[]; evidence?: Array<{ id?: string; state?: string; reference?: string }> }; binding: Record<string, string>; snapshot: { native: { state: string; head_sha: string; target_sha: string }; rules: { state: string; reason: string }; approvals?: Array<{ actor_id: string; state: string; head_sha: string; dismissed: boolean }> } }
type Operation = { id: string; state: string; reason: string; version: number; native_result?: { state: string; merge_sha: string; head_sha: string; url: string } }

test('merges a real validated repair through native protection and Reforge authority', async ({ page }) => {
  test.skip(!enabled, 'requires REFORGE_LIVE_REPAIR_MERGE_BROWSER with disposable Gitea, Ollama, runner and gVisor')
  test.setTimeout(900_000)
  if (process.env.REFORGE_ISOLATED_BROWSER_DATABASE !== '1' || process.env.REFORGE_BASE_URL !== 'http://127.0.0.1:8081') throw new Error('Positive merge browser test requires disposable controller/database')
  const evidenceDirectory = process.env.REFORGE_REPAIR_MERGE_EVIDENCE ?? ''
  if (!isAbsolute(evidenceDirectory)) throw new Error('REFORGE_REPAIR_MERGE_EVIDENCE must be an absolute directory')
  mkdirSync(evidenceDirectory, { recursive: true, mode: 0o700 })
  const evidence: Record<string, unknown> = { started_at: new Date().toISOString(), model: process.env.REFORGE_LIVE_REPAIR_MODEL ?? 'qwen3:1.7b', steps: [] }
  const mark = (step: string) => { (evidence.steps as string[]).push(`${new Date().toISOString()} ${step}`); save() }
  const save = () => writeFileSync(resolve(evidenceDirectory, 'evidence.json'), `${JSON.stringify(evidence, null, 2)}\n`, { mode: 0o600 })
  const token = (actor: string) => readFileSync(resolve(root, `.local/gitea/reforge-${actor}.token`), 'utf8').trim()
  const native = async <T>(verb: string, path: string, actor: string, body?: unknown): Promise<T> => {
    const response = await fetch(`${giteaAPI}${path}`, { method: verb, signal: AbortSignal.timeout(10_000), headers: { Authorization: `token ${token(actor)}`, 'Content-Type': 'application/json' }, ...(body === undefined ? {} : { body: JSON.stringify(body) }) })
    if (!response.ok) throw new Error(`disposable Gitea ${verb} ${path} HTTP${response.status}`)
    return (response.status === 204 ? undefined : await response.json()) as T
  }

  await page.setViewportSize({ width: 1440, height: 900 })
  page.setDefaultTimeout(20_000)
  await page.emulateMedia({ colorScheme: 'light' })
  await page.goto('/auth/login')
  await expect(page).toHaveURL(/\/org\/[^/]+\/overview/)
  const { csrf_token: csrf } = await page.request.get('/api/v1/session').then(response => response.json() as Promise<{ csrf_token: string }>)
  const api = async <T>(verb: string, path: string, data?: unknown, headers: Record<string, string> = {}): Promise<T> => {
    const response = await page.request.fetch(path, { method: verb, data, headers: { Accept: 'application/json', Origin: 'http://127.0.0.1:8081', ...(data === undefined ? {} : { 'Content-Type': 'application/json' }), ...(verb === 'GET' ? {} : { 'X-CSRF-Token': csrf }), ...headers } })
    if (!response.ok()) throw new Error(`${verb} ${path} failed HTTP${response.status()}`)
    return (response.status() === 204 ? undefined : await response.json()) as T
  }

  const fixture: RepairFixture = await createRepairFixture(page)
  const org = fixture.orgID
  let authority: RepairAuthority | undefined
  let policyChanged = false
  let inspectorID = ''
  let journeyFailed = false
  try {
    evidence.fixture = { org_id: org, repository_id: fixture.repositoryID, native_repository: fixture.nativeFullName, pinned_sha: fixture.pinnedSHA, runner_id: fixture.runnerID, pool_id: fixture.poolID }
    mark('disposable repair fixture ready')
    authority = await configureRepairAuthority(page, fixture)
    mark(`repair authority configured for ${authority.modelName}`)

    const finding = await findFinding(page, org, fixture.repositoryID)
    await page.goto(`/org/${org}/findings?finding=${encodeURIComponent(finding.id)}`)
    const details = page.getByRole('region', { name: finding.title })
    await expect(details).toBeVisible()
    await details.getByRole('tab', { name: 'Repair', exact: true }).click()
    await details.getByRole('combobox', { name: 'Recipe' }).selectOption('go')
    await details.getByRole('combobox', { name: 'Model' }).selectOption(authority.modelID)
    await details.getByRole('combobox', { name: 'Runner pool' }).selectOption(fixture.poolID)
    const previewResponse = page.waitForResponse(response => response.request().method() === 'POST' && response.url().includes(`/api/v1/orgs/${org}/repair-preview`), { timeout: 90_000 })
    await details.getByRole('button', { name: 'Generate preview' }).click()
    expect((await previewResponse).ok()).toBe(true)
    const repairPreview = details.getByRole('group', { name: 'Repair preview' })
    await expect(repairPreview.getByRole('button', { name: 'Confirm and queue repair' })).toBeEnabled({ timeout: 30_000 })
    await repairPreview.getByRole('button', { name: 'Confirm and queue repair' }).click()
    const runLink = page.getByRole('link', { name: /Queued run/ })
    await expect(runLink).toBeVisible({ timeout: 15_000 })
    const taskID = new URL((await runLink.getAttribute('href'))!, 'http://127.0.0.1').searchParams.get('run')!
    expect(taskID).toBeTruthy()
    evidence.repair_task_id = taskID
    mark('repair queued through GUI')
    await runLink.click()
    await expect(page.getByRole('region', { name: /Run / })).toBeVisible()
    type RepairRun = { task: { state: string; reason?: string }; state?: string; candidate_sha?: string; change?: { id: string; url: string; head_sha: string }; candidate_artifacts?: string[]; report?: { reason?: string; turns?: number; patches?: Array<{ path: string }>; baseline?: unknown[]; candidate?: unknown[] } }
    let run: RepairRun
    const deadline = Date.now() + 480_000
    for (;;) {
      run = await api<RepairRun>('GET', `/api/v1/orgs/${org}/repair-runs/${taskID}`)
      if (run.task.state === 'completed') break
      if (['failed', 'blocked', 'cancelled'].includes(run.task.state) || Date.now() >= deadline) {
        evidence.repair_failure = { state: run.task.state, task_reason: run.task.reason ?? '', report_reason: run.report?.reason ?? '', turns: run.report?.turns ?? 0, baseline_checks: run.report?.baseline?.length ?? 0, candidate_checks: run.report?.candidate?.length ?? 0 }
        save()
        throw new Error(`repair did not complete: ${run.task.state}; ${run.task.reason ?? ''}; ${run.report?.reason ?? ''}`)
      }
      await page.waitForTimeout(1000)
    }
    const candidate = run.candidate_sha ?? ''
    const changeID = run.change?.id ?? ''
    expect(candidate).toMatch(/^[0-9a-f]{40}$/)
    expect(changeID).toBeTruthy()
    expect(run.candidate_artifacts?.length).toBeGreaterThan(0)
    const patchedPaths = run.report?.patches?.map(patch => patch.path) ?? []
    expect(patchedPaths).toEqual(['value.go'])
    evidence.repair = { candidate_sha: candidate, change_id: changeID, change_url: run.change?.url, turns: run.report?.turns, patched_paths: patchedPaths }
    const pulls = await native<Array<{ number: number; head: { sha: string } }>>('GET', `/repos/${fixture.nativeFullName}/pulls?state=all`, 'bot')
    expect(pulls.filter(pull => pull.head.sha === candidate).map(pull => String(pull.number))).toEqual([changeID])
    mark(`validated repair published native change ${changeID} at ${candidate}`)

    const primary = await api<{ name: string; provider: string }>('GET', `/api/v1/orgs/${org}/connections/${fixture.connectionID}`)
    await page.goto(`/org/${org}/repositories`)
    await page.getByRole('button', { name: 'Sync inventory', exact: true }).click()
    const syncDialog = page.getByRole('dialog', { name: 'Sync forge inventory' })
    await syncDialog.getByLabel('Forge connection').selectOption({ label: `${primary.name} · ${primary.provider}` })
    await syncDialog.getByRole('button', { name: 'Start preview', exact: true }).click()
    await expect(syncDialog.getByText('complete', { exact: true })).toBeVisible({ timeout: 180_000 })
    const targetRepository = syncDialog.getByLabel(fixture.nativeFullName, { exact: true })
    const preview = syncDialog.getByRole('group', { name: 'Preview candidates' })
    const candidateCheckboxes = preview.getByRole('checkbox')
    const loadCandidates = syncDialog.getByRole('button', { name: 'Load more candidates', exact: true })
    await expect.poll(async () => (await targetRepository.count()) + (await loadCandidates.count()), { timeout: 30_000 }).toBeGreaterThan(0)
    for (let pageNumber = 0; await targetRepository.count() === 0; pageNumber += 1) {
      if (pageNumber >= 20 || await loadCandidates.count() === 0) throw new Error('published repair repository was not in inventory preview')
      const previousCheckboxCount = await candidateCheckboxes.count()
      await loadCandidates.click()
      await expect.poll(async () => (await candidateCheckboxes.count()) > previousCheckboxCount || await loadCandidates.count() === 0, { timeout: 30_000 }).toBe(true)
    }
    await expect(targetRepository).toBeVisible()
    const selectAll = syncDialog.getByRole('checkbox', { name: /^Select all loaded/ })
    await expect(selectAll).toBeChecked()
    await selectAll.click()
    await targetRepository.check()
    await expect(targetRepository).toBeChecked()
    const selectedRepositoryCount = await candidateCheckboxes.evaluateAll(boxes => boxes.slice(1).filter(box => (box as HTMLInputElement).checked).length)
    expect(selectedRepositoryCount).toBe(1)
    const importResponse = page.waitForResponse(response => response.request().method() === 'POST' && response.url().includes(`/api/v1/orgs/${org}/inventory-syncs/`) && response.url().endsWith('/import'))
    await syncDialog.getByRole('button', { name: /^Import selected/ }).click()
    const imported = await importResponse
    expect(imported.status(), await imported.text()).toBe(202)
    const importJob = await imported.json() as { id: string; state: string }
    evidence.inventory_sync = { import_job_id: importJob.id, state: importJob.state, native_repository: fixture.nativeFullName }
    await expect(syncDialog.getByText('Import complete. Repository inventory will refresh.', { exact: true })).toBeVisible({ timeout: 180_000 })
    mark('published repair repository re-imported through Repositories GUI')

    await native('PUT', `/repos/${fixture.nativeFullName}/collaborators/reforge-reviewer`, 'admin', { permission: 'write' })
    await native('PATCH', `/repos/${fixture.nativeFullName}`, 'admin', { allow_fast_forward_only_merge: true })
    await native('POST', `/repos/${fixture.nativeFullName}/branch_protections`, 'admin', { rule_name: 'main', enable_push: false, required_approvals: 1, dismiss_stale_approvals: true, block_on_outdated_branch: true, block_on_rejected_reviews: true, block_on_official_review_requests: true, block_admin_merge_override: true, enable_status_check: false })
    mark('native protection configured by disposable Gitea administrator')

    await page.goto(`/org/${org}/connections`)
    await page.getByRole('row', { name: new RegExp(primary.name) }).getByRole('button', { name: 'Open' }).click()
    await page.getByRole('button', { name: 'Actions', exact: true }).click()
    await page.getByLabel('Rotate secret').fill(token('reviewer'))
    const rotated = page.waitForResponse(response => response.request().method() === 'POST' && response.url().includes(`/connections/${fixture.connectionID}/rotate`))
    await page.getByRole('button', { name: 'Rotate credential' }).click()
    expect((await rotated).ok()).toBe(true)
    await expectHealthy(page, fixture.connectionID)
    mark('primary forge connection rotated to merge actor and tested healthy through GUI')

    const runners = await api<{ items: Array<{ id: string; name: string }> }>('GET', `/api/v1/orgs/${org}/runner-pools/${fixture.poolID}/runners?limit=100`)
    const runnerName = runners.items.find(item => item.id === fixture.runnerID)!.name
    const inspectorName = `merge-inspector-${Date.now()}`
    await page.goto(`/org/${org}/connections`)
    await page.getByRole('button', { name: 'Add connection' }).click()
    const form = page.getByRole('dialog', { name: 'Add connection' })
    await form.getByLabel('Provider').selectOption('gitea')
    await form.getByLabel('Name', { exact: true }).fill(inspectorName)
    await form.getByLabel('Endpoint').fill('http://127.0.0.1:53000')
    await form.getByLabel('Secret').fill(token('inspector'))
    await form.getByLabel('Private route via enrolled runner').check()
    const poolSelect = form.getByRole('combobox', { name: 'Runner pool' })
    await expect.poll(async () => poolSelect.locator('option').allTextContents()).toContain(fixture.poolName)
    await poolSelect.selectOption({ label: fixture.poolName })
    const runnerSelect = form.getByRole('combobox', { name: 'Runner', exact: true })
    await expect.poll(async () => runnerSelect.locator('option').allTextContents(), { timeout: 20_000 }).toContain(`${runnerName} · ${fixture.poolName}`)
    await runnerSelect.selectOption({ label: `${runnerName} · ${fixture.poolName}` })
    await form.getByLabel('Route host').fill('127.0.0.1')
    await form.getByLabel('Approved CIDRs').fill('127.0.0.1/32')
    await form.getByRole('button', { name: 'Create connection' }).click()
    await expect(form).toBeHidden({ timeout: 15_000 })
    const connections = await api<{ items: Array<{ id: string; name: string }> }>('GET', `/api/v1/orgs/${org}/connections?kind=forge&limit=100`)
    inspectorID = connections.items.find(item => item.name === inspectorName)!.id
    await page.getByRole('row', { name: new RegExp(inspectorName) }).getByRole('button', { name: 'Open' }).click()
    await expectHealthy(page, inspectorID)
    mark('protection inspector connection created and tested healthy through GUI')

    await page.goto(`/org/${org}/repositories?repository=${encodeURIComponent(fixture.repositoryID)}`)
    await page.getByRole('tab', { name: 'Settings' }).click()
    await page.getByLabel('Requested merge authority').selectOption('reforge')
    await page.getByRole('button', { name: 'Save maintenance configuration' }).click()
    await expect(page.getByText('Maintenance configuration saved.')).toBeVisible()
    const maintenance = await api<{ merge_authority: string; version: number }>('GET', `/api/v1/orgs/${org}/repositories/${fixture.repositoryID}/maintenance`)
    expect(maintenance.merge_authority).toBe('reforge')
    evidence.maintenance = maintenance
    mark('merge authority set to Reforge through GUI')

    type Effective = { hash: string; layers: Array<{ scope: { kind: string; id: string }; version_id: string; binding_version: number }> }
    const before = await api<Effective>('GET', `/api/v1/orgs/${org}/policies/effective?repository_id=${encodeURIComponent(fixture.repositoryID)}`)
    const repairVersion = before.layers.find(layer => layer.scope.kind === 'organisation' && layer.scope.id === org)!.version_id
    await page.goto(`/org/${org}/policies`)
    await page.getByLabel('Policy repository').selectOption(fixture.repositoryID)
    await page.getByTitle(repairVersion).click()
    await page.getByRole('tab', { name: 'Merge', exact: true }).click()
    await page.getByLabel('Allowed merge methods').selectOption('explicit')
    await page.getByLabel('merge methods values').fill(method)
    await page.getByRole('tab', { name: 'Review', exact: true }).click()
    await page.getByLabel('Reason', { exact: true }).fill('Allow protected fast-forward merges')
    const created = page.waitForResponse(response => response.request().method() === 'POST' && response.url().endsWith('/policies/versions'))
    await page.getByRole('button', { name: 'Save immutable version' }).click()
    const createdResponse = await created
    expect(createdResponse.status()).toBe(201)
    const mergeVersion = await createdResponse.json() as { id: string; policy: { allow: Record<string, unknown> } }
    expect(mergeVersion.policy.allow.merge_methods).toEqual([method])
    const openSimulation = page.getByRole('button', { name: 'Open simulation', exact: true })
    await expect(openSimulation).toBeVisible()
    await openSimulation.click()
    await page.getByRole('combobox', { name: 'Action', exact: true }).selectOption('merge')
    await page.getByLabel('Merge method', { exact: true }).fill(method)
    await page.getByRole('button', { name: 'Simulate candidate rollout', exact: true }).click()
    await expect(page.getByRole('region', { name: 'Policy simulation' })).toBeVisible()
    await page.getByRole('button', { name: 'Activate exact simulation' }).click()
    policyChanged = true
    await expect.poll(async () => (await api<Effective>('GET', `/api/v1/orgs/${org}/policies/effective?repository_id=${encodeURIComponent(fixture.repositoryID)}`)).layers.find(layer => layer.scope.kind === 'organisation')?.version_id).toBe(mergeVersion.id)
    evidence.policy = { repair_version: repairVersion, merge_version: mergeVersion.id, allow: mergeVersion.policy.allow }
    mark('merge policy version saved, simulated and activated through GUI')

    const contractSHA = createHash('sha256').update(readFileSync(resolve(root, 'test/forge/gitea/contract_test.go'))).digest('hex')
    await page.goto(`/org/${org}/changes?repository=${encodeURIComponent(fixture.repositoryID)}`)
    await page.getByRole('tab', { name: 'Merge settings' }).click()
    const settings = page.getByRole('region', { name: 'Merge settings' }).last()
    await settings.getByLabel('Enabled').check()
    await settings.getByLabel('Protection reader').selectOption(inspectorID)
    await settings.getByLabel('Cooperation reference').fill('disposable fixture has no bot automerge')
    await settings.getByText('Operator qualification evidence').click()
    await settings.getByLabel('Evidence reference').fill(qualificationReference)
    await settings.getByLabel('Evidence SHA-256').fill(contractSHA)
    await settings.getByLabel('Verified at').fill(localMinute(Date.now() - 5 * 60_000))
    await settings.getByLabel('Expires at').fill(localMinute(Date.now() + 2 * 3600_000))
    await settings.getByLabel('Exact head').check()
    await settings.getByLabel('Strict target').check()
    const savedConfig = page.waitForResponse(response => response.request().method() === 'PUT' && response.url().endsWith(`/repositories/${fixture.repositoryID}/merge-configuration`))
    await settings.getByRole('button', { name: 'Save merge settings' }).click()
    const savedResponse = await savedConfig
    expect(savedResponse.status(), await savedResponse.text()).toBe(200)
    const mergeConfig = await savedResponse.json() as { version: number; enabled: boolean; inspector_connection_id: string; qualification: Record<string, unknown> }
    expect(mergeConfig.enabled).toBe(true)
    evidence.merge_configuration = { version: mergeConfig.version, inspector_connection_id: mergeConfig.inspector_connection_id, qualification: mergeConfig.qualification }
    mark('merge settings and operator qualification saved through GUI')

    await expect.poll(async () => (await api<{ items: Array<{ id: string; head_sha: string }> }>('GET', `/api/v1/orgs/${org}/repositories/${fixture.repositoryID}/changes?limit=100`)).items.find(item => item.id === changeID)?.head_sha, { timeout: 90_000 }).toBe(candidate)
    const changesURL = `/org/${org}/changes?repository=${encodeURIComponent(fixture.repositoryID)}&change=${encodeURIComponent(changeID)}`
    const review = page.getByRole('region', { name: 'Merge review' })
    const previewGate = async () => {
      await expect(review.getByRole('button', { name: 'Preview merge gate' })).toBeEnabled({ timeout: 30_000 })
      const response = page.waitForResponse(item => item.request().method() === 'POST' && item.url().includes(`/changes/${encodeURIComponent(changeID)}/merge-preview`), { timeout: 180_000 })
      await review.getByRole('button', { name: 'Preview merge gate' }).click()
      const http = await response
      expect(http.status(), await http.text()).toBe(200)
      return await http.json() as Gate
    }
    await page.goto(changesURL)
    await review.getByLabel('Merge method').selectOption(method)
    const blocked = await previewGate()
    expect(blocked.binding.head).toBe(candidate)
    expect(blocked.decision.outcome).not.toBe('allow')
    await expect(review.getByRole('button', { name: 'Request protected merge' })).toBeDisabled()
    evidence.gate_before_approval = summarize(blocked)
    await page.screenshot({ path: resolve(evidenceDirectory, 'gate-blocked-1440-light.png'), fullPage: true })
    mark(`gate before native approval: ${blocked.decision.outcome}`)

    await native('POST', `/repos/${fixture.nativeFullName}/pulls/${changeID}/reviews`, 'reviewer', { body: 'Disposable native reviewer approval of the validated candidate.', commit_id: candidate, event: 'APPROVED' })
    mark('independent native reviewer approved exact candidate head')

    await page.reload()
    await review.getByLabel('Merge method').selectOption(method)
    const allowed = await previewGate()
    evidence.gate_after_approval = summarize(allowed)
    save()
    expect(allowed.snapshot.approvals?.filter(approval => approval.state.toLowerCase() === 'approved' && approval.head_sha === candidate && !approval.dismissed)).toHaveLength(1)
    expect(allowed.decision.outcome, JSON.stringify(allowed.decision)).toBe('allow')
    expect(allowed.configuration_version).toBe(mergeConfig.version)
    await page.screenshot({ path: resolve(evidenceDirectory, 'gate-allow-1440-light.png'), fullPage: true })
    const requested = page.waitForResponse(item => item.request().method() === 'POST' && item.url().endsWith(`/api/v1/orgs/${org}/merge-operations`))
    await review.getByRole('button', { name: 'Request protected merge' }).click()
    const requestedHTTP = await requested
    expect(requestedHTTP.ok(), await requestedHTTP.text()).toBe(true)
    const operation = await requestedHTTP.json() as Operation
    mark(`protected merge requested through GUI: operation ${operation.id} ${operation.state}`)
    await expect(review.getByText('merged', { exact: true })).toBeVisible({ timeout: 120_000 })

    await page.reload()
    await expect(review.getByText('merged', { exact: true })).toBeVisible({ timeout: 30_000 })
    const observed = await api<Operation>('GET', `/api/v1/orgs/${org}/merge-operations/${operation.id}`)
    const branch = await native<{ commit: { id: string } }>('GET', `/repos/${fixture.nativeFullName}/branches/main`, 'bot')
    const pull = await native<{ merged: boolean; merge_commit_sha?: string; state: string }>('GET', `/repos/${fixture.nativeFullName}/pulls/${changeID}`, 'bot')
    evidence.merge = { operation_id: observed.id, state: observed.state, version: observed.version, native_result: observed.native_result, gitea_main_sha: branch.commit.id, gitea_pull: pull }
    save()
    expect(observed.state).toBe('merged')
    expect(observed.native_result?.merge_sha).toBe(candidate)
    expect(branch.commit.id).toBe(candidate)
    expect(pull.merged).toBe(true)
    mark('canonical native merge SHA matches candidate after reload')

    for (const [width, height] of [[1440, 900], [390, 844]]) {
      for (const scheme of ['light', 'dark'] as const) {
        await page.setViewportSize({ width, height })
        await page.emulateMedia({ colorScheme: scheme })
        await page.goto(changesURL)
        await expect(page.locator('html')).toHaveAttribute('data-theme', scheme)
        await expect(review.getByText('merged', { exact: true })).toBeVisible({ timeout: 30_000 })
        await page.screenshot({ path: resolve(evidenceDirectory, `merged-${width}-${scheme}.png`), fullPage: true })
      }
    }
    mark('merged state screenshots captured')
  } catch (error) {
    journeyFailed = true
    evidence.test_error = String(error)
    save()
    throw error
  } finally {
    const errors: string[] = []
    const revoke = async (connectionID: string) => {
      try {
        const current = await api<{ version: number }>('GET', `/api/v1/orgs/${org}/connections/${connectionID}`)
        await api('DELETE', `/api/v1/orgs/${org}/connections/${connectionID}`, undefined, { 'If-Match': `"${current.version}"` })
      } catch (error) { errors.push(String(error)) }
    }
    if (inspectorID) await revoke(inspectorID)
    if (authority && !policyChanged) {
      try { await authority.cleanup() } catch (error) { errors.push(String(error)) }
    } else if (authority) await revoke(authority.modelID)
    try { await fixture.cleanup() } catch (error) { errors.push(String(error)) }
    evidence.cleanup_errors = errors
    evidence.finished_at = new Date().toISOString()
    save()
    if (errors.length && !journeyFailed) throw new Error(`positive merge cleanup failed: ${errors.join('; ')}`)
  }
})

async function expectHealthy(page: Page, connectionID: string) {
  const probe = page.waitForResponse(response => response.request().method() === 'POST' && response.url().includes(`/connections/${connectionID}/test`))
  await page.getByRole('button', { name: 'Test capability' }).click()
  const response = await probe
  expect(response.status()).toBe(200)
  expect((await response.json() as { state: string }).state).toBe('healthy')
}

async function findFinding(page: Page, orgID: string, repositoryID: string) {
  let item: { id: string; title: string } | undefined
  await expect.poll(async () => {
    const response = await page.request.get(`/api/v1/orgs/${orgID}/findings?limit=100&repository_id=${encodeURIComponent(repositoryID)}&state=open&category=ci_failure`)
    item = ((await response.json()) as { items?: Array<{ id: string; title: string }> }).items?.[0]
    return Boolean(item)
  }, { timeout: 90_000, intervals: [1000, 2500, 5000] }).toBe(true)
  return item!
}

function summarize(gate: Gate) {
  return { id: gate.id, expires_at: gate.expires_at, configuration_version: gate.configuration_version, connection_version: gate.connection_version, outcome: gate.decision.outcome, blockers: gate.decision.blockers ?? [], required_actions: gate.decision.required_actions ?? [], evidence: (gate.decision.evidence ?? []).map(item => ({ id: item.id, state: item.state, reference: item.reference })), binding: gate.binding, native: gate.snapshot.native, rules: gate.snapshot.rules, approvals: gate.snapshot.approvals ?? [] }
}

function localMinute(value: number) {
  const date = new Date(value)
  const pad = (part: number) => String(part).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}
