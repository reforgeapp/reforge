import { test, expect } from '@playwright/test'
import { generateKeyPairSync, randomUUID, sign } from 'node:crypto'
import { execFileSync } from 'node:child_process'

test.use({ trace: 'off' })

test('actual campaign API persists a future scheduled lifecycle', async ({ page }) => {
  test.skip(process.env.REFORGE_LIVE_CAMPAIGNS_BROWSER !== '1', 'requires disposable local development database')
  const database = new URL(process.env.REFORGE_DATABASE_URL ?? '')
  if (!['127.0.0.1', 'localhost'].includes(database.hostname) || database.port !== '55432' || database.pathname !== '/reforge_dev') throw new Error('Requires disposable local reforge_dev on port 55432')
  await page.goto('/auth/login')
  await page.waitForURL(url => !url.pathname.startsWith('/auth/'))
  const identity = await page.request.get('/api/v1/session').then(response => response.json())
  expect(identity.user.id).toMatch(/^[a-f0-9-]{36}$/)
  const meta = await page.request.get('/api/v1/meta').then(response => response.json())
  expect(meta.development && meta.fixture_auth).toBe(true)
  const org = randomUUID(), source = randomUUID(), delivery = randomUUID(), connection = randomUUID()
  const key = generateKeyPairSync('ed25519')
  const publicKey = key.publicKey.export({ type: 'spki', format: 'der' }).subarray(-32).toString('base64')
  const configuration = { environment: 'campaign-live', source_repository_id: source, delivery_repository_id: delivery, version: 1, enabled: true, target_branch: 'main', manifest_path: 'deploy.yaml', pointer: '/image', image_repository: 'registry.example/app', provenance_public_key: publicKey, health_public_key: publicKey, health_checks: ['smoke'], observation_seconds: 1, max_evidence_age_seconds: 300, deadline_seconds: 600, recovery_allowed: false }
  const sqlJSON = JSON.stringify(configuration).replace(/'/g, "''")
  execFileSync('/tmp/reforge-postgres/bin/psql', ['-X', '-q', '-v', 'ON_ERROR_STOP=1'], {
    env: { ...process.env, PGHOST: database.hostname, PGPORT: database.port, PGDATABASE: database.pathname.slice(1), PGUSER: decodeURIComponent(database.username), PGPASSWORD: decodeURIComponent(database.password) },
    input: `BEGIN; SELECT set_config('reforge.org_id','${org}',true); SELECT set_config('reforge.user_id','${identity.user.id}',true); INSERT INTO organisations(id,name) VALUES('${org}','Campaign GUI acceptance fixture'); INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES('${org}','${identity.user.id}','owner',true); INSERT INTO connections(org_id,id,kind,provider,name,endpoint,settings,state,server_version) VALUES('${org}','${connection}','forge','gitea','Scheduled GUI fixture','https://campaign-fixture.invalid','{"auth_kind":"token","billing_route":"forge"}','healthy','1.27.3'); INSERT INTO repositories(org_id,id,connection_id,native_id,name,provider) VALUES('${org}','${source}','${connection}','source','Campaign source','gitea'),('${org}','${delivery}','${connection}','delivery','Campaign delivery','gitea'); INSERT INTO gitops_configurations(org_id,environment,source_repository_id,delivery_repository_id,version,document) VALUES('${org}','campaign-live','${source}','${delivery}',1,'${sqlJSON}'); COMMIT;`,
    stdio: ['pipe', 'ignore', 'pipe'],
  })
  const root = `/api/v1/orgs/${org}`
  const headers = { Origin: new URL(page.url()).origin, 'X-CSRF-Token': identity.csrf_token }
  const post = async (url: string, data: unknown, extra = {}) => {
    const response = await page.request.post(url, { data, headers: { ...headers, ...extra } })
    expect(response.ok(), `${response.status()} ${await response.text()}`).toBe(true)
    return response.json()
  }
  const version = await post(`${root}/policies/versions`, { scope: { kind: 'organisation', id: org }, policy: { schema: 'maintenance/v1' }, reason: 'Scheduled GUI fixture' })
  const simulation = await post(`${root}/policies/versions/${version.id}/simulate`, { repository_id: source, input: { action: 'deploy' } })
  await post(`${root}/policies/versions/${version.id}/activate`, { repository_id: source, simulation_hash: simulation.hash, reason: 'Scheduled GUI fixture' }, { 'If-Match': '"0"' })
  const secondTime = (time: number) => new Date(time).toISOString().slice(0, 19) + 'Z'
  const proof = { org_id: org, repository_id: source, source_sha: 'a'.repeat(40), artifact_digest: `sha256:${'b'.repeat(64)}`, build_id: 'gui-fixture', issued_at: secondTime(Date.now() - 60000), expires_at: secondTime(Date.now() + 3 * 86400000) }
  const member = { repository_id: source, environment: configuration.environment, gitops: { change_id: '1', source_sha: proof.source_sha, artifact_digest: proof.artifact_digest, provenance: { document: proof, signature: sign(null, Buffer.from(JSON.stringify(proof)), key.privateKey).toString('base64') } } }
  let campaignID = ''
  try {
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto(`/org/${org}/campaigns`)
    await page.getByRole('button', { name: 'Plan campaign' }).click()
    const currentStep = page.getByRole('navigation', { name: 'Campaign steps' }).locator('[aria-current="step"]')
    await expect(currentStep).toHaveText('1 Members')
    await page.getByRole('combobox', { name: 'Kind', exact: true }).selectOption('gitops')
    await page.getByLabel('Name', { exact: true }).fill('Future GUI acceptance')
    await page.getByText('Advanced member JSON import').click()
    await page.getByLabel('Advanced member JSON').fill(JSON.stringify([member]))
    await page.getByRole('button', { name: 'Next', exact: true }).click()
    await expect(currentStep).toHaveText('2 Execution')
    await page.getByRole('button', { name: 'Next', exact: true }).click()
    await expect(currentStep).toHaveText('3 Rollout')
    const nextDay = new Date(Date.now() + 86400000).toISOString().slice(0, 16)
    await page.getByLabel('Not before (UTC)').fill(nextDay)
    const previewResponse = page.waitForResponse(response => response.url().endsWith('/campaign-previews') && response.request().method() === 'POST')
    await page.getByRole('button', { name: 'Preview campaign', exact: true }).click()
    const preview = await (await previewResponse).json()
    expect(preview.blockers).toEqual([])
    expect(Date.parse(preview.input.not_before)).toBeGreaterThan(Date.now() + 23 * 3600000)
    expect(preview.members).toHaveLength(1)
    const createdResponse = page.waitForResponse(response => response.url().endsWith('/campaigns') && response.request().method() === 'POST')
    await page.getByRole('button', { name: 'Create planned campaign' }).click()
    const created = await (await createdResponse).json()
    campaignID = created.id
    expect(created.state).toBe('planned')
    const detail = page.getByRole('region', { name: `Campaign ${campaignID}`, exact: true })
    await detail.getByLabel('Campaign action reason').fill('Future fixture has no dispatch authority today')
    for (const [button, state] of [['Start canary', 'canary'], ['Pause', 'paused'], ['Resume current stage', 'canary'], ['Cancel', 'cancelled']]) {
      await detail.getByRole('button', { name: button, exact: true }).click()
      await expect.poll(async () => page.request.get(`${root}/campaigns/${campaignID}`).then(response => response.json()).then(value => value.state)).toBe(state)
      await expect(detail).toContainText(state)
    }
    await page.reload()
    await expect(page).toHaveURL(new RegExp(`[?&]campaign=${campaignID}(&|$)`))
    await expect(detail).toBeVisible()
    await expect(detail).toContainText('cancelled')
    const members = await page.request.get(`${root}/campaigns/${campaignID}/members`).then(response => response.json())
    expect(members.items.every((item: { action_id?: string; state: string }) => !item.action_id && item.state === 'cancelled')).toBe(true)
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    await detail.getByRole('link', { name: 'Open campaign budget' }).focus()
    await page.keyboard.press('Enter')
    await expect(page.getByRole('combobox', { name: 'Scope', exact: true })).toHaveValue('campaign')
    await expect(page.getByRole('combobox', { name: 'Named scope', exact: true })).toHaveValue(campaignID)
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  } finally {
    if (campaignID) {
      const current = await page.request.get(`${root}/campaigns/${campaignID}`).then(response => response.json())
      if (!['cancelled', 'completed', 'failed'].includes(current.state)) await post(`${root}/campaigns/${campaignID}/cancel`, { reason: 'End isolated GUI acceptance' }, { 'If-Match': `"${current.version}"` })
    }
  }
})
