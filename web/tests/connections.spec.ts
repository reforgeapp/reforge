import { test, expect, type Page } from '@playwright/test'
import { spawn } from 'node:child_process'
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'

const organisation = '00000000-0000-4000-8000-000000000001'
test.use({ trace: 'off' })

test.describe('connections and runners administration', () => {
  async function signIn(page: Page) {
    await page.goto('/auth/login')
    await expect(page).toHaveURL(/\/org\/[^/]+\/overview/)
  }

  test('loads the persisted connections route and opens the real create form', async ({ page }) => {
    await signIn(page)
    await page.goto(`/org/${organisation}/connections`)
    await expect(page.getByRole('heading', { name: 'Connections', exact: true })).toBeVisible()
    await page.getByRole('button', { name: 'Add connection' }).click()
    await expect(page.getByRole('dialog', { name: 'Add connection' })).toBeVisible()
    await expect(page.getByLabel('Secret')).toHaveAttribute('type', 'password')
    await expect(page.getByText(/credentials are write-only|capability state comes from the server probe/i)).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'Refresh connections' })).toBeVisible()
  })

  test('persists connection tab and filters in the URL', async ({ page }) => {
    await signIn(page)
    await page.route(`**/api/v1/orgs/${organisation}/connections**`, route => route.fulfill({ json: { items: [
      { id: 'connection-private', org_id: organisation, kind: 'model', provider: 'openai', name: 'private model', endpoint: 'https://example.invalid/private', state: 'unverified', reason: 'Awaiting probe', version: 1, credential_version: 1, settings: {}, capabilities: {}, verified_at: null },
      { id: 'connection-public', org_id: organisation, kind: 'model', provider: 'openai', name: 'public model', endpoint: 'https://example.invalid/public', state: 'healthy', reason: '', version: 1, credential_version: 1, settings: {}, capabilities: {}, verified_at: '2026-09-22T00:00:00Z' },
    ], complete: true } }))
    await page.goto(`/org/${organisation}/connections`)
    await page.getByRole('button', { name: 'Models & agents' }).click()
    await expect(page.getByRole('row', { name: /private model/ })).toBeVisible()
    await expect(page.getByRole('row', { name: /public model/ })).toBeVisible()
    await page.getByLabel('Search', { exact: true }).fill('private')
    await page.getByRole('combobox', { name: 'State', exact: true }).selectOption('unverified')
    await expect(page.getByRole('row', { name: /private model/ })).toBeVisible()
    await expect(page.getByRole('row', { name: /public model/ })).toHaveCount(0)
    await expect(page).toHaveURL(/connection_tab=models/)
    await expect(page).toHaveURL(/q=private/)
    await expect(page).toHaveURL(/state=unverified/)
    await page.reload()
    await expect(page.getByLabel('Search', { exact: true })).toHaveValue('private')
    await expect(page.getByRole('combobox', { name: 'State', exact: true })).toHaveValue('unverified')
    await expect(page.getByRole('row', { name: /private model/ })).toBeVisible()
    await page.getByLabel('Search', { exact: true }).fill('')
    await page.getByRole('combobox', { name: 'State', exact: true }).selectOption('')
    await expect(page.getByRole('row', { name: /private model/ })).toBeVisible()
    await expect(page.getByRole('row', { name: /public model/ })).toBeVisible()
    await expect(page).toHaveURL(/connection_tab=models/)
  })

  test('loads model and agent rows after forge pagination boundaries', async ({ page }) => {
    await signIn(page)
    const forgeRows = Array.from({ length: 101 }, (_, index) => ({
      id: `forge-${String(index).padStart(3, '0')}`,
      org_id: organisation,
      kind: 'forge',
      provider: 'gitea',
      name: `forge-${index}`,
      endpoint: 'https://example.invalid',
      state: 'healthy',
      reason: '',
      version: 1,
      credential_version: 1,
      settings: {},
      capabilities: {},
      verified_at: null,
    }))
    const modelRow = { id: 'model-after-forges', org_id: organisation, kind: 'model', provider: 'openai', name: 'model after forges', endpoint: 'https://example.invalid', state: 'healthy', reason: '', version: 1, credential_version: 1, settings: {}, capabilities: {}, verified_at: null }
    const agentRow = { ...modelRow, id: 'agent-after-forges', kind: 'agent', provider: 'codex', name: 'agent after forges' }
    const requestedKinds = new Set<string>()
    await page.route(`**/api/v1/orgs/${organisation}/connections**`, route => {
      const kind = new URL(route.request().url()).searchParams.get('kind')
      if (kind) requestedKinds.add(kind)
      if (!kind || kind === 'forge') return route.fulfill({ json: { items: forgeRows.slice(0, 100), complete: false, next_cursor: forgeRows[99].id } })
      if (kind === 'model') return route.fulfill({ json: { items: [modelRow], complete: true } })
      if (kind === 'agent') return route.fulfill({ json: { items: [agentRow], complete: true } })
      return route.fulfill({ json: { items: [], complete: true } })
    })
    await page.goto(`/org/${organisation}/connections`)
    await page.getByRole('button', { name: 'Models & agents' }).click()
    await expect(page.getByRole('row', { name: /model after forges/ })).toBeVisible()
    await expect(page.getByRole('row', { name: /agent after forges/ })).toBeVisible()
    expect([...requestedKinds].sort()).toEqual(['agent', 'forge', 'model'])
  })

  test('keeps loaded rows and filters visible after refresh failure', async ({ page }) => {
    await signIn(page)
    const connection = { id: 'refresh-stale-connection', org_id: organisation, kind: 'forge', provider: 'gitea', name: 'cached forge', endpoint: 'https://example.invalid', state: 'healthy', reason: '', version: 1, credential_version: 1, settings: {}, capabilities: {}, verified_at: null }
    let fail = false
    await page.route(`**/api/v1/orgs/${organisation}/connections**`, route => fail
      ? route.fulfill({ status: 503, json: { error: 'temporary outage' } })
      : route.fulfill({ json: { items: [connection], complete: true } }))
    await page.goto(`/org/${organisation}/connections`)
    const row = page.getByRole('row', { name: /cached forge/ })
    await expect(row).toBeVisible()
    fail = true
    await page.getByRole('button', { name: 'Refresh connections' }).click()
    await expect(page.getByRole('alert')).toContainText('Could not load all connection results')
    await expect(page.getByRole('button', { name: 'Retry' })).toBeVisible()
    await expect(row).toBeVisible()
    await expect(page.getByLabel('Search', { exact: true })).toBeVisible()
  })

  test('keeps model rows when agent stream fails', async ({ page }) => {
    await signIn(page)
    const model = { id: 'working-model', org_id: organisation, kind: 'model', provider: 'openai', name: 'available model', endpoint: 'https://example.invalid', state: 'healthy', reason: '', version: 1, credential_version: 1, settings: {}, capabilities: {}, verified_at: null }
    await page.route(`**/api/v1/orgs/${organisation}/connections**`, route => {
      const kind = new URL(route.request().url()).searchParams.get('kind')
      if (kind === 'model') return route.fulfill({ json: { items: [model], complete: true } })
      if (kind === 'agent') return route.fulfill({ status: 503, json: { error: 'agent stream unavailable' } })
      return route.fulfill({ json: { items: [], complete: true } })
    })
    await page.goto(`/org/${organisation}/connections`)
    await page.getByRole('button', { name: 'Models & agents' }).click()
    await expect(page.getByRole('row', { name: /available model/ })).toBeVisible()
    await expect(page.getByRole('alert')).toContainText('Could not load all connection results')
    await expect(page.getByRole('button', { name: 'Retry' })).toBeVisible()
  })

  test('connection revoke requires explicit confirmation and preserves version and CSRF', async ({ page }) => {
    await signIn(page)
    const connection = { id: 'connection-revoke', org_id: organisation, kind: 'forge', provider: 'gitea', name: 'staging forge', endpoint: 'https://example.invalid', state: 'healthy', reason: '', version: 7, credential_version: 1, settings: {}, capabilities: {}, verified_at: null }
    let deleteCount = 0
    let deleteHeaders: Record<string, string> = {}
    await page.route(`**/api/v1/orgs/${organisation}/connections**`, async route => {
      if (route.request().url().endsWith('/connection-revoke') && route.request().method() === 'DELETE') {
        deleteCount++
        deleteHeaders = route.request().headers()
        return route.fulfill({ json: { ...connection, state: 'revoked', version: 8 } })
      }
      if (route.request().url().endsWith('/connection-revoke')) return route.fulfill({ json: connection })
      return route.fulfill({ json: { items: [connection], complete: true } })
    })
    await page.goto(`/org/${organisation}/connections`)
    const row = page.getByRole('row', { name: /staging forge/ })
    await row.getByRole('button', { name: 'Open' }).click()
    await page.getByRole('button', { name: 'Revoke', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: 'Revoke connection' })
    await expect(dialog).toBeVisible()
    await expect(dialog.getByText(/Revoke staging forge\?/)).toBeVisible()
    expect(deleteCount).toBe(0)
    await dialog.getByRole('button', { name: 'Cancel' }).click()
    await expect(dialog).toBeHidden()
    expect(deleteCount).toBe(0)
    await page.getByRole('button', { name: 'Revoke', exact: true }).click()
    await page.getByRole('dialog', { name: 'Revoke connection' }).getByRole('button', { name: 'Revoke connection' }).click()
    await expect.poll(() => deleteCount).toBe(1)
    expect(deleteHeaders['if-match']).toBe('"7"')
    expect(deleteHeaders['x-csrf-token']).toBeTruthy()
  })

  test('empty local filters among loaded pages keep Load more available', async ({ page }) => {
    await signIn(page)
    await page.route(`**/api/v1/orgs/${organisation}/connections**`, route => {
      const url = new URL(route.request().url())
      const kind = url.searchParams.get('kind')
      const cursor = url.searchParams.get('cursor')
      if (kind === 'model' && cursor === 'next') return route.fulfill({ json: { items: [{ id: 'model-after-filter', org_id: organisation, kind: 'model', provider: 'openai', name: 'not loaded model', endpoint: 'https://example.invalid', state: 'healthy', reason: '', version: 1, credential_version: 1, settings: {}, capabilities: {}, verified_at: null }], complete: true } })
      if (kind === 'model') return route.fulfill({ json: { items: [{ id: 'first-model', org_id: organisation, kind: 'model', provider: 'openai', name: 'first model', endpoint: 'https://example.invalid', state: 'healthy', reason: '', version: 1, credential_version: 1, settings: {}, capabilities: {}, verified_at: null }], complete: false, next_cursor: 'next' } })
      if (kind === 'agent') return route.fulfill({ json: { items: [], complete: true } })
      return route.fulfill({ json: { items: [], complete: true } })
    })
    await page.goto(`/org/${organisation}/connections`)
    await page.getByRole('button', { name: 'Models & agents' }).click()
    await page.getByLabel('Search', { exact: true }).fill('not loaded')
    await expect(page.getByText('No matches in loaded results; more may be available.')).toBeVisible()
    await expect(page.getByRole('button', { name: 'Load more' })).toBeVisible()
    await page.getByRole('button', { name: 'Load more' }).click()
    await expect(page.getByRole('row', { name: /not loaded model/ })).toBeVisible()
    await expect(page.getByText('No matches in loaded results; more may be available.')).toHaveCount(0)
  })

  test('closes connection details with the top-right control or Escape', async ({ page }) => {
    await signIn(page)
    const connection = { id: 'connection-close', org_id: organisation, kind: 'forge', provider: 'gitea', name: 'close control fixture with a deliberately long integration name for narrow layouts', endpoint: 'https://example.invalid', state: 'healthy', reason: '', version: 1, credential_version: 1, settings: { billing_route: 'forge' }, capabilities: {}, verified_at: null }
    await page.route(`**/api/v1/orgs/${organisation}/connections**`, route => {
      if (new URL(route.request().url()).pathname.endsWith('/connection-close')) return route.fulfill({ json: connection })
      return route.fulfill({ json: { items: [connection], complete: true } })
    })
    await page.goto(`/org/${organisation}/connections`)
    await page.getByRole('row', { name: /close control fixture/ }).getByRole('button', { name: 'Open' }).click()
    await expect(page.getByRole('button', { name: 'Close connection details' })).toBeVisible()
    await expect(page.getByText('Back to list')).toHaveCount(0)
    await page.setViewportSize({ width: 390, height: 844 })
    const title = page.locator('.detail-panel > header > div:first-child')
    const actions = page.locator('.detail-panel .detail-actions')
    const titleBox = await title.boundingBox()
    const actionsBox = await actions.boundingBox()
    expect(titleBox).not.toBeNull()
    expect(actionsBox).not.toBeNull()
    expect(actionsBox!.y).toBeGreaterThanOrEqual(titleBox!.y + titleBox!.height - 1)
    expect(titleBox!.x + titleBox!.width).toBeLessThanOrEqual(390)
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390)
    const closeBox = await page.getByRole('button', { name: 'Close connection details' }).boundingBox()
    expect(closeBox!.x + closeBox!.width).toBeLessThanOrEqual(390)
    await page.keyboard.press('Escape')
    await expect(page.getByRole('button', { name: 'Close connection details' })).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'Open' })).toBeFocused()
  })

  test('shows runner pool controls without inventing enrolled runners', async ({ page }) => {
    await signIn(page)
    await page.goto(`/org/${organisation}/runners`)
    await expect(page.getByRole('heading', { name: 'Runners', exact: true })).toBeVisible()
    await page.getByRole('button', { name: 'Create pool' }).click()
    await expect(page.getByRole('dialog', { name: 'Create runner pool' })).toBeVisible()
    await expect(page.getByText(/leave the list empty for an onboarding pool/i)).toBeVisible()
  })

  test('keeps connection dialog keyboard accessible on narrow screens', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await signIn(page)
    await page.goto(`/org/${organisation}/connections`)
    await page.getByRole('button', { name: 'Add connection' }).click()
    const dialog = page.getByRole('dialog', { name: 'Add connection' })
    await expect(dialog).toBeVisible()
    await expect.poll(async () => dialog.evaluate(element => element.contains(document.activeElement))).toBe(true)
    await expect.poll(async () => dialog.evaluate(element => { const rect = element.getBoundingClientRect(); return rect.left >= 0 && rect.right <= window.innerWidth && element.scrollWidth <= element.clientWidth })).toBe(true)
    await page.keyboard.press('Escape')
    await expect(dialog).toBeHidden()
  })

  test('creates an empty pool and receives a one-use enrollment token', async ({ page }) => {
    await signIn(page)
    await page.goto(`/org/${organisation}/runners`)
    const poolName = `browser-onboarding-${Date.now()}`
    await page.getByRole('button', { name: 'Create pool' }).click()
    await page.getByLabel('Name').fill(poolName)
    await page.getByRole('button', { name: 'Save pool' }).click()
    await expect(page.getByRole('dialog', { name: 'Create runner pool' })).toBeHidden({ timeout: 15_000 })
    await page.getByLabel('Search', { exact: true }).fill(poolName)
    const row = page.getByRole('row', { name: new RegExp(poolName) })
    await expect(row).toBeVisible({ timeout: 30_000 })
    await row.getByRole('button', { name: 'Enroll runner' }).click()
    const dialog = page.getByRole('dialog', { name: 'Runner enrollment token' })
    await expect(dialog).toBeVisible()
    await expect(dialog.getByLabel('One-use token')).not.toHaveValue('')
    await expect(dialog.getByText(/reforge-runner enroll --endpoint/)).toBeVisible()
    await dialog.getByRole('button', { name: 'Clear token' }).click()
    await expect(dialog).toBeHidden()
  })

  test.describe('live private Gitea lifecycle', () => {
    test('enrolls a runner and verifies, rotates, retests and revokes a private connection', async ({ page }) => {
      test.skip(process.env.REFORGE_LIVE_GITEA_TEST !== '1', 'requires the disposable local Gitea and runner controller')
      test.setTimeout(240_000)
      const root = resolve(process.cwd(), '..')
      const temp = mkdtempSync(join(tmpdir(), 'reforge-gui-runner-'))
      const enrollmentFile = join(temp, 'enrollment-token')
      const credentialFile = join(temp, 'runner-credentials')
      const inventoryRepoName = `browser-inventory-${Date.now()}`
      const adminToken = readFileSync(resolve(root, '.local/gitea/reforge-admin.token'), 'utf8').trim()
      let connector: ReturnType<typeof spawn> | undefined
      let connectorExit: number | null = null
      let connectorStderr = false
      let createdConnectionID = ''
      let createdPoolID = ''
      try {
        await signIn(page)
        await page.goto(`/org/${organisation}/runners`)
        const poolName = `browser-private-${Date.now()}`
        await page.getByRole('button', { name: 'Create pool' }).click()
        await page.getByLabel('Name').fill(poolName)
        await page.getByRole('button', { name: 'Save pool' }).click()
        const poolRow = page.getByRole('row', { name: new RegExp(poolName) })
        await expect(poolRow).toBeVisible()
        const poolPage = await page.request.get(`/api/v1/orgs/${organisation}/runner-pools?limit=100`)
        if (!poolPage.ok()) throw new Error(`pool lookup failed with ${poolPage.status()}`)
        const poolItems = await poolPage.json() as { items?: Array<{ id: string; name: string }> }
        createdPoolID = poolItems.items?.find(item => item.name === poolName)?.id ?? ''
        if (!createdPoolID) throw new Error('created pool ID missing')
        await poolRow.getByRole('button', { name: 'Enroll runner' }).click()
        const enrollmentDialog = page.getByRole('dialog', { name: 'Runner enrollment token' })
        const enrollmentToken = await enrollmentDialog.getByLabel('One-use token').inputValue()
        writeFileSync(enrollmentFile, enrollmentToken, { mode: 0o600 })
        await enrollmentDialog.getByRole('button', { name: 'Clear token' }).click()
        await expect(enrollmentDialog).toBeHidden()

        const runnerName = `browser-runner-${Date.now()}`
        await runCommand(join(root, 'bin/reforge-runner'), ['enroll', '--endpoint', 'http://127.0.0.1:8080', '--credentials', credentialFile, '--token-file', enrollmentFile, '--name', runnerName, '--development'], root)
        connector = spawn(join(root, 'bin/reforge-runner'), ['connector', '--endpoint', 'http://127.0.0.1:8080', '--credentials', credentialFile, '--development'], { cwd: root, stdio: ['ignore', 'ignore', 'pipe'] })
        connector.stderr?.on('data', () => { connectorStderr = true })
        connector.once('exit', code => { connectorExit = code })
        await new Promise(resolveAfterStart => setTimeout(resolveAfterStart, 1_500))
        if (connector.exitCode !== null) throw new Error(`runner connector exited with ${connector.exitCode}`)

        const botToken = readFileSync(resolve(root, '.local/gitea/reforge-bot.token'), 'utf8').trim()
        await page.goto(`/org/${organisation}/connections`)
        await page.getByRole('button', { name: 'Add connection' }).click()
        const form = page.getByRole('dialog', { name: 'Add connection' })
        await form.getByLabel('Provider').selectOption('gitea')
        await form.getByLabel('Name', { exact: true }).fill(`browser-gitea-${Date.now()}`)
        await form.getByLabel('Endpoint').fill('http://127.0.0.1:53000')
        await form.getByLabel('Secret').fill(botToken)
        await form.getByLabel('Private route via enrolled runner').check()
        const runnerSelect = form.getByRole('combobox', { name: 'Runner' })
        await expect.poll(async () => runnerSelect.locator('option').allTextContents(), { timeout: 15_000 }).toContain(`${runnerName} · ${poolName}`)
        await runnerSelect.selectOption({ label: `${runnerName} · ${poolName}` })
        await form.getByLabel('Route host').fill('127.0.0.1')
        await form.getByLabel('Approved CIDRs').fill('127.0.0.1/32')
        const connectionName = await form.getByLabel('Name', { exact: true }).inputValue()
        await form.getByRole('button', { name: 'Create connection' }).click()
        await expect(form).toBeHidden({ timeout: 15_000 })
        const connectionPage = await page.request.get(`/api/v1/orgs/${organisation}/connections?limit=100`)
        if (!connectionPage.ok()) throw new Error(`connection lookup failed with ${connectionPage.status()}`)
        const connectionItems = await connectionPage.json() as { items?: Array<{ id: string; name: string }> }
        createdConnectionID = connectionItems.items?.find(item => item.name === connectionName)?.id ?? ''
        if (!createdConnectionID) throw new Error('created connection ID missing')
        const connectionRow = page.getByRole('row', { name: new RegExp(connectionName) })
        await expect(connectionRow).toBeVisible()

        await connectionRow.getByRole('button', { name: 'Test capability' }).click()
        try { await expect.poll(async () => connectionRow.innerText(), { timeout: 45_000 }).toContain('healthy') } catch (error) { throw new Error(`${error instanceof Error ? error.message : 'private probe failed'}; connector_exit=${connectorExit ?? 'running'}; connector_stderr=${connectorStderr}`) }
        await expect(connectionRow).toContainText('Version 1')
        await connectionRow.getByRole('button', { name: connectionName }).click()
        const webhookDetails = page.getByRole('region', { name: connectionName })
        await webhookDetails.getByRole('button', { name: 'Actions', exact: true }).click()
        await webhookDetails.getByRole('button', { name: 'Issue webhook secret' }).click()
        const firstWebhookSecret = await webhookDetails.getByLabel('One-time webhook secret').inputValue()
        expect(firstWebhookSecret).not.toBe('')
        await webhookDetails.getByRole('button', { name: 'Rotate webhook secret' }).click()
        await expect(webhookDetails.getByLabel('One-time webhook secret')).not.toHaveValue(firstWebhookSecret)
        await webhookDetails.getByRole('button', { name: 'Revoke webhook' }).click()
        await expect(webhookDetails.getByText('Webhook revoked.')).toBeVisible()
        await webhookDetails.getByRole('button', { name: 'Close', exact: true }).click()
        const createRepo = await fetch(`http://127.0.0.1:53000/api/v1/admin/users/reforge-bot/repos`, { method: 'POST', headers: { Authorization: `token ${adminToken}`, 'Content-Type': 'application/json' }, body: JSON.stringify({ name: inventoryRepoName, auto_init: true, default_branch: 'main', private: true }) })
        if (!createRepo.ok) throw new Error(`fixture repo create failed with ${createRepo.status}`)
        await page.goto(`/org/${organisation}/repositories`)
        await page.getByRole('button', { name: 'Sync inventory' }).click()
        const sync = page.getByRole('dialog', { name: 'Sync forge inventory' })
        await sync.getByLabel('Forge connection').selectOption({ label: `${connectionName} · gitea` })
        await sync.getByRole('button', { name: 'Start preview' }).click()
        await expect(sync.getByText('complete', { exact: true })).toBeVisible({ timeout: 45_000 })
        const selectAll = sync.getByRole('checkbox', { name: /Select all loaded/ })
        if (await selectAll.isChecked()) await selectAll.uncheck()
        const repoCandidate = sync.getByRole('checkbox', { name: new RegExp(`reforge-bot/${inventoryRepoName}`) })
        await expect(repoCandidate).toBeVisible()
        await repoCandidate.check()
        await sync.getByRole('button', { name: /Import selected/ }).click()
        await expect(sync.getByText('Import complete. Repository inventory will refresh.')).toBeVisible({ timeout: 150_000 })
        await sync.getByRole('button', { name: 'Close' }).click()
        const importedRow = page.getByRole('row', { name: new RegExp(inventoryRepoName) })
        await expect(importedRow).toBeVisible({ timeout: 15_000 })
        await importedRow.getByRole('button', { name: inventoryRepoName }).click()
        const repositoryDetails = page.getByRole('region', { name: inventoryRepoName })
        await repositoryDetails.getByRole('tab', { name: 'Settings', exact: true }).click()
        await expect(repositoryDetails.getByText('Inventory freshness')).toBeVisible()
        await expect(repositoryDetails.getByRole('heading', { name: 'Maintenance configuration', exact: true })).toBeVisible()
        await repositoryDetails.getByRole('button', { name: 'Save maintenance configuration' }).click()
        await expect(repositoryDetails.getByText('Maintenance configuration saved.')).toBeVisible()
        await repositoryDetails.getByRole('button', { name: 'Close', exact: true }).click()
        await importedRow.getByRole('button', { name: inventoryRepoName }).click()
        const refreshedDetails = page.getByRole('region', { name: inventoryRepoName })
        await refreshedDetails.getByRole('button', { name: 'Start scan' }).click()
        try {
          await expect(refreshedDetails.getByText('complete', { exact: true })).toBeVisible({ timeout: 90_000 })
        } catch (error) {
          const [connectionsResponse, jobsResponse, repositoryResponse] = await Promise.all([
            page.request.get(`/api/v1/orgs/${organisation}/connections?limit=100`),
            page.request.get(`/api/v1/orgs/${organisation}/inventory-syncs?limit=100`),
            page.request.get(`/api/v1/orgs/${organisation}/repositories`),
          ])
          const connections = await connectionsResponse.json() as { items?: Array<{ id: string; name: string }> }
          const jobs = await jobsResponse.json() as { items?: Array<{ id: string; connection_id: string; kind: string; state: string; reason?: string; phase?: string }> }
          const repositories = await repositoryResponse.json() as { items?: Array<{ id: string; name: string; sync_state?: string; changes_observed_at?: string }> }
          const connection = (connections.items ?? []).find(item => item.name === connectionName)
          const ownJobs = (jobs.items ?? []).filter(item => item.connection_id === connection?.id).map(item => ({ id: item.id, kind: item.kind, state: item.state, reason: item.reason ?? '', phase: item.phase ?? '' }))
          const ownRepository = (repositories.items ?? []).find(item => item.name === `reforge-bot/${inventoryRepoName}`)
          process.stderr.write(`discovery diagnostic jobs=${JSON.stringify(ownJobs)} repository=${JSON.stringify({ sync_state: ownRepository?.sync_state ?? '', changes_observed_at: ownRepository?.changes_observed_at ?? '' })}\n`)
          throw error
        }
        await expect(refreshedDetails.getByText(/complete Observed/)).toBeVisible()
        await refreshedDetails.getByRole('button', { name: 'Close', exact: true }).click()
        await importedRow.getByRole('button', { name: inventoryRepoName }).click()
        const persistedDetails = page.getByRole('region', { name: inventoryRepoName })
        await expect(persistedDetails.getByText('Discovery')).toBeVisible()
        await expect(persistedDetails.getByText('complete', { exact: true })).toBeVisible()
        await expect(persistedDetails.getByText(/complete Observed/)).toBeVisible()
        await persistedDetails.getByRole('button', { name: 'Close', exact: true }).click()
        await page.goto(`/org/${organisation}/connections`)
        const refreshedConnectionRow = page.getByRole('row', { name: new RegExp(connectionName) })
        await expect(refreshedConnectionRow).toBeVisible()
        await refreshedConnectionRow.getByRole('button', { name: connectionName }).click()
        const details = page.getByRole('region', { name: connectionName })
        await details.getByRole('button', { name: 'Actions', exact: true }).click()
        await details.getByLabel('Rotate secret').fill(botToken)
        await details.getByRole('button', { name: 'Rotate credential' }).click()
        await expect(details.getByText('Version').first()).toBeVisible({ timeout: 10_000 })
        await details.getByRole('button', { name: 'Close', exact: true }).click()
        await expect(connectionRow).toContainText('Version 2', { timeout: 10_000 })
        await connectionRow.getByRole('button', { name: 'Test capability' }).click()
        await expect.poll(async () => connectionRow.innerText(), { timeout: 45_000 }).toContain('healthy')
        await connectionRow.getByRole('button', { name: 'Revoke' }).click()
        await expect(connectionRow.getByText('revoked')).toBeVisible({ timeout: 10_000 })
      } finally {
        try {
          const session = await page.request.get('/api/v1/session').then(response => response.json() as Promise<{ csrf_token: string }>)
          const headers = { 'X-CSRF-Token': session.csrf_token, Origin: 'http://127.0.0.1:8080' }
          if (createdConnectionID) {
            const connections = await page.request.get(`/api/v1/orgs/${organisation}/connections?limit=100`).then(response => response.json() as Promise<{ items?: Array<{ id: string; version: number }> }>)
            const connection = connections.items?.find(item => item.id === createdConnectionID)
            if (connection) {
              const response = await page.request.delete(`/api/v1/orgs/${organisation}/connections/${connection.id}`, { headers: { ...headers, 'If-Match': `"${connection.version}"` } })
              if (!response.ok() && response.status() !== 404 && response.status() !== 409) throw new Error(`connection cleanup failed with ${response.status()}`)
            }
          }
          if (createdPoolID) {
            const poolResponse = await page.request.get(`/api/v1/orgs/${organisation}/runner-pools?limit=100`)
            const pools = await poolResponse.json() as { items?: Array<{ id: string; name: string; version: number; state: string; repository_ids: string[] }> }
            const pool = pools.items?.find(item => item.id === createdPoolID)
            if (pool) {
              const runners = await page.request.get(`/api/v1/orgs/${organisation}/runner-pools/${pool.id}/runners?limit=100`).then(response => response.json() as Promise<{ items?: Array<{ id: string; version: number }> }>)
              for (const runner of runners.items ?? []) {
                const response = await page.request.delete(`/api/v1/orgs/${organisation}/runners/${runner.id}`, { headers: { ...headers, 'If-Match': `"${runner.version}"` } })
                if (!response.ok() && response.status() !== 404 && response.status() !== 409) throw new Error(`runner cleanup failed with ${response.status()}`)
              }
              if (pool.state !== 'revoked') {
                const response = await page.request.put(`/api/v1/orgs/${organisation}/runner-pools/${pool.id}`, { headers: { ...headers, 'Content-Type': 'application/json', 'If-Match': `"${pool.version}"` }, data: { name: pool.name, state: 'revoked', repository_ids: pool.repository_ids } })
                if (!response.ok() && response.status() !== 404 && response.status() !== 409) throw new Error(`pool cleanup failed with ${response.status()}`)
              }
            }
          }
        } catch (cleanupError) {
          process.stderr.write(`browser fixture cleanup failed: ${cleanupError instanceof Error ? cleanupError.message : 'unknown error'}\n`)
        }
        if (connector && connector.exitCode === null) connector.kill('SIGTERM')
        const deleteRepo = await fetch(`http://127.0.0.1:53000/api/v1/repos/reforge-bot/${encodeURIComponent(inventoryRepoName)}`, { method: 'DELETE', headers: { Authorization: `token ${adminToken}` } })
        if (!deleteRepo.ok && deleteRepo.status !== 404) process.stderr.write(`fixture repo cleanup failed with ${deleteRepo.status}\n`)
        rmSync(temp, { recursive: true, force: true })
      }
    })
  })
})

async function runCommand(command: string, args: string[], cwd: string) {
  await new Promise<void>((resolveCommand, rejectCommand) => {
    const child = spawn(command, args, { cwd, stdio: 'ignore' })
    const timer = setTimeout(() => { child.kill('SIGTERM'); rejectCommand(new Error('runner enrollment timed out')) }, 20_000)
    child.once('error', error => { clearTimeout(timer); rejectCommand(error) })
    child.once('exit', code => { clearTimeout(timer); if (code === 0) resolveCommand(); else rejectCommand(new Error(`runner enrollment exited with ${code ?? 'signal'}`)) })
  })
}
