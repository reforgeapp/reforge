import { test, expect } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { randomUUID } from 'node:crypto'
import { execFileSync } from 'node:child_process'

test.use({ trace: 'off' })

test('real controls persist policy, budget and exported audit evidence', async ({ page }) => {
  test.skip(process.env.REFORGE_LIVE_CONTROLS_BROWSER !== '1', 'requires disposable local development database')
  const database = new URL(process.env.REFORGE_DATABASE_URL ?? '')
  if (!['127.0.0.1', 'localhost'].includes(database.hostname) || database.pathname !== '/reforge_dev' || database.port !== '55432') throw new Error('Controls test requires disposable local reforge_dev on port 55432')
  await page.goto('/auth/login')
  await page.waitForURL(url => !url.pathname.startsWith('/auth/'))
  const identity = await page.request.get('/api/v1/session').then(response => response.json())
  if (!/^[a-f0-9-]{36}$/.test(identity.user.id)) throw new Error('Invalid fixture user')
  const meta = await page.request.get('/api/v1/meta').then(response => response.json())
  expect(meta.development && meta.fixture_auth).toBe(true)
  const org = randomUUID(), repository = randomUUID()
  execFileSync('/tmp/reforge-postgres/bin/psql', ['-X', '-q', '-v', 'ON_ERROR_STOP=1'], {
    env: { ...process.env, PGHOST: database.hostname, PGPORT: database.port, PGDATABASE: database.pathname.slice(1), PGUSER: decodeURIComponent(database.username), PGPASSWORD: decodeURIComponent(database.password) },
    input: `BEGIN; SELECT set_config('reforge.org_id','${org}',true); SELECT set_config('reforge.user_id','${identity.user.id}',true); INSERT INTO organisations(id,name) VALUES('${org}','Controls acceptance'); INSERT INTO memberships(org_id,user_id,role,all_repositories) VALUES('${org}','${identity.user.id}','owner',true); INSERT INTO repositories(org_id,id,native_id,name) VALUES('${org}','${repository}','controls-fixture','Controls fixture'); COMMIT;`,
    stdio: ['pipe', 'ignore', 'pipe'],
  })
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto(`/org/${org}/policies`)
  await expect(page.getByLabel('Policy repository')).toHaveValue(repository)
  await page.getByLabel('Pause this scope').check()
  await page.getByRole('tab', { name: 'Review' }).click()
  await page.getByLabel('Reason', { exact: true }).fill('Local controls acceptance')
  const created = page.waitForResponse(response => response.request().method() === 'POST' && response.url().endsWith('/policies/versions'))
  await page.getByRole('button', { name: 'Save immutable version' }).click()
  const response = await created
  expect(response.status()).toBe(201)
  const version = await response.json()
  const openSimulation = page.getByRole('button', { name: 'Open simulation', exact: true })
  if (await openSimulation.count()) await openSimulation.click()
  await expect(page.getByRole('button', { name: 'Simulate candidate rollout', exact: true })).toBeEnabled()
  await page.getByRole('combobox', { name: 'Action', exact: true }).selectOption('repair')
  await page.getByRole('button', { name: 'Simulate candidate rollout', exact: true }).click()
  await expect(page.getByRole('region', { name: 'Policy simulation' })).toContainText('paused')
  await page.getByRole('button', { name: 'Activate exact simulation' }).click()
  await expect.poll(async () => page.request.get(`/api/v1/orgs/${org}/policies/effective?repository_id=${repository}`).then(r => r.json()).then(r => r.layers.find((layer: { version_id: string }) => layer.version_id === version.id)?.binding_version)).toBe(1)
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  await page.keyboard.press('Tab')
  expect(await page.evaluate(() => document.activeElement?.tagName)).not.toBe('BODY')
  await page.goto(`/org/${org}/usage`)
  await page.getByRole('tab', { name: 'Budgets', exact: true }).click()
  await page.getByRole('button', { name: 'Create budget' }).click()
  await page.getByLabel('USD cap', { exact: true }).fill('0')
  await page.getByLabel('tokens', { exact: true }).fill('2000')
  const savedBudget = page.waitForResponse(response => response.request().method() === 'PUT' && response.url().includes('/budgets/'))
  await page.getByRole('button', { name: 'Save budget' }).click()
  const budgetResponse = await savedBudget
  expect(budgetResponse.status()).toBe(200)
  expect((await budgetResponse.json()).caps.micro_usd).toBe(0)
  await expect(page.getByText(/Version 1 · held/)).toBeVisible()
  await page.reload()
  await page.getByRole('tab', { name: 'Budgets', exact: true }).click()
  await expect(page.getByLabel('tokens', { exact: true })).toHaveValue('2000')
  await page.getByRole('tab', { name: 'Usage', exact: true }).click()
  await expect(page.getByText('No usage records match these filters.')).toBeVisible()
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  await page.goto(`/org/${org}/audit`)
  await page.getByLabel('Action filter').fill('policy.changed')
  await page.getByLabel('Action filter').press('Enter')
  await expect(page.getByRole('button', { name: 'Open Policy Changed event', exact: true })).toBeVisible()
  const exported = page.waitForResponse(response => response.url().includes('/audit-events/export?'))
  const download = page.waitForEvent('download')
  await page.getByRole('button', { name: 'Export server page' }).click()
  const artifact = await download
  const exportResponse = await exported
  expect(exportResponse.status()).toBe(200)
  expect(exportResponse.headers()['content-type']).toBe('application/x-ndjson')
  const event = JSON.parse(readFileSync((await artifact.path())!, 'utf8').trim())
  expect(event.action).toBe('policy.changed')
  expect(event.object_id).toBe(version.id)
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
})
