import { test, expect, type Locator, type Page } from '@playwright/test'
import { mkdir, writeFile } from 'node:fs/promises'
import { resolve } from 'node:path'

const developmentOrg = '00000000-0000-4000-8000-000000000001'
const acceptanceOrg = '00000000-0000-4000-8000-000000000002'
const artifactDir = resolve(process.env.REFORGE_ACCEPTANCE_ARTIFACTS || '.local/t28a-connected-admin/artifacts')
const acceptanceEnabled = process.env.REFORGE_ADMIN_ACCEPTANCE_URL === 'http://127.0.0.1:8091' && process.env.REFORGE_BASE_URL === process.env.REFORGE_ADMIN_ACCEPTANCE_URL

async function signIn(page: Page) {
  await page.goto('/auth/login')
  await expect(page).toHaveURL(new RegExp(`/org/${developmentOrg}/overview$`))
  const meta = await page.request.get('/api/v1/meta')
  expect(meta.ok()).toBeTruthy()
  expect((await meta.json()).fixture_auth).toBe(true)
  const session = await page.request.get('/api/v1/session')
  expect(session.ok()).toBeTruthy()
  const data = await session.json() as { organisations: Array<{ id: string; name: string }> }
  expect(data.organisations.map(org => org.id)).toEqual(expect.arrayContaining([developmentOrg, acceptanceOrg]))
}

async function switchOrganisation(page: Page, name: string) {
  await page.getByRole('button', { name: 'Switch organisation' }).click()
  await page.getByRole('menuitem', { name }).click()
  const orgID = name === 'Acceptance' ? acceptanceOrg : developmentOrg
  await expect(page).toHaveURL(new RegExp(`/org/${orgID}/[^?#]+$`))
}

test('connected admin GUI persists runner and policy changes across navigation and organisation scope', async ({ page }) => {
  test.skip(!acceptanceEnabled, 'requires the explicitly selected isolated backend at 127.0.0.1:8091')
  await mkdir(artifactDir, { recursive: true })
  const requests: string[] = []
  page.on('request', request => {
    if (request.url().includes('/api/v1/')) requests.push(`${request.method()} ${new URL(request.url()).pathname}`)
  })
  await signIn(page)
  await switchOrganisation(page, 'Acceptance')

  await page.getByRole('link', { name: 'Runners', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Runners', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Create pool' }).click()
  const createDialog = page.getByRole('dialog', { name: 'Create runner pool' })
  await expect(createDialog).toBeVisible()
  const invalidPool = 'x'.repeat(161)
  await createDialog.getByLabel('Name').fill(invalidPool)
  const invalidResponse = page.waitForResponse(response => response.request().method() === 'POST' && response.url().includes(`/api/v1/orgs/${acceptanceOrg}/runner-pools`))
  await createDialog.getByRole('button', { name: 'Save pool' }).click()
  expect((await invalidResponse).status()).toBe(400)
  await expect(createDialog.getByRole('alert')).toBeVisible()
  const poolName = `acceptance-pool-${Date.now()}`
  await createDialog.getByLabel('Name').fill(poolName)
  const createResponse = page.waitForResponse(response => response.request().method() === 'POST' && response.url().includes(`/api/v1/orgs/${acceptanceOrg}/runner-pools`))
  await createDialog.getByRole('button', { name: 'Save pool' }).click()
  expect((await createResponse).ok()).toBeTruthy()
  await expect(createDialog).toBeHidden()
  const poolRow = page.getByRole('row', { name: new RegExp(poolName) })
  await expect(poolRow).toBeVisible()
  await poolRow.getByRole('button', { name: 'Open' }).click()
  await page.getByRole('button', { name: 'Edit', exact: true }).click()
  const editDialog = page.getByRole('dialog', { name: 'Edit runner pool' })
  await editDialog.getByLabel('State').selectOption('draining')
  const updateResponse = page.waitForResponse(response => response.request().method() === 'PUT' && response.url().includes(`/api/v1/orgs/${acceptanceOrg}/runner-pools/`))
  await editDialog.getByRole('button', { name: 'Save pool' }).click()
  expect((await updateResponse).ok()).toBeTruthy()
  await expect(page.getByText('draining', { exact: true }).first()).toBeVisible()

  await page.getByRole('link', { name: 'Policies', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Policies', exact: true })).toBeVisible()
  await expect(page.getByLabel('Policy repository')).toHaveValue('00000000-0000-4000-8000-000000000004')
  const reason = `acceptance policy ${Date.now()}`
  await page.getByLabel('Reason').fill(reason)
  const firstPolicy = page.waitForResponse(response => response.request().method() === 'POST' && response.url().includes(`/api/v1/orgs/${acceptanceOrg}/policies/versions`))
  await page.getByRole('button', { name: 'Save immutable version' }).click()
  expect((await firstPolicy).ok()).toBeTruthy()
  await expect(page.getByRole('row', { name: new RegExp(reason) })).toBeVisible()

  await page.getByText('Advanced policy JSON import/export').click()
  await page.getByLabel('Raw policy JSON').fill('{')
  await page.getByRole('button', { name: 'Save immutable version' }).click()
  await expect(page.getByRole('alert').filter({ hasText: 'Policy JSON must be valid JSON.' })).toBeVisible()
  await page.getByLabel('Raw policy JSON').fill('')
  await page.getByLabel('Reason').fill(`${reason} revision`)
  const secondPolicy = page.waitForResponse(response => response.request().method() === 'POST' && response.url().includes(`/api/v1/orgs/${acceptanceOrg}/policies/versions`))
  await page.getByRole('button', { name: 'Save immutable version' }).click()
  expect((await secondPolicy).ok()).toBeTruthy()
  await expect(page.getByRole('row', { name: new RegExp(`${reason} revision`) })).toBeVisible()

  await page.getByRole('link', { name: 'Runners', exact: true }).click()
  await expect(page.getByRole('row', { name: new RegExp(poolName) })).toBeVisible()
  await page.reload()
  await expect(page.getByRole('row', { name: new RegExp(poolName) })).toBeVisible()
  await page.getByRole('link', { name: 'Policies', exact: true }).click()
  await expect(page.getByRole('row', { name: new RegExp(`${reason} revision`) })).toBeVisible()
  await page.reload()
  await expect(page.getByRole('row', { name: new RegExp(`${reason} revision`) })).toBeVisible()

  await switchOrganisation(page, 'Development')
  await page.getByRole('link', { name: 'Runners', exact: true }).click()
  await expect(page.getByRole('row', { name: new RegExp(poolName) })).toHaveCount(0)
  await expect(page.getByText('No runner pools match these filters.')).toBeVisible()
  await switchOrganisation(page, 'Acceptance')
  await page.getByRole('link', { name: 'Policies', exact: true }).click()
  await expect(page.getByRole('row', { name: new RegExp(`${reason} revision`) })).toBeVisible()

  expect(requests.some(item => item.startsWith(`POST /api/v1/orgs/${acceptanceOrg}/runner-pools`))).toBe(true)
  expect(requests.some(item => item.startsWith(`PUT /api/v1/orgs/${acceptanceOrg}/runner-pools/`))).toBe(true)
  expect(requests.some(item => item.startsWith(`POST /api/v1/orgs/${acceptanceOrg}/policies/versions`))).toBe(true)
  expect(requests.some(item => item.startsWith(`GET /api/v1/orgs/${developmentOrg}/runner-pools`))).toBe(true)
  await writeFile(resolve(artifactDir, 'admin-backend-requests.json'), JSON.stringify(requests, null, 2))
})

test('admin dialogs remain keyboard operable at 390px in light and dark themes', async ({ page }) => {
  test.skip(!acceptanceEnabled, 'requires the explicitly selected isolated backend at 127.0.0.1:8091')
  await mkdir(artifactDir, { recursive: true })
  await page.setViewportSize({ width: 390, height: 844 })
  await signIn(page)
  await switchOrganisation(page, 'Acceptance')
  const menu = page.getByRole('button', { name: 'Menu' })
  await menu.focus()
  await page.keyboard.press('Enter')
  const sidebar = page.locator('#primary-navigation')
  await expect(sidebar.getByRole('link').first()).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(menu).toBeFocused()

  for (const theme of ['light', 'dark'] as const) {
    const currentTheme = await page.locator('html').getAttribute('data-theme')
    if (currentTheme !== theme) await page.getByRole('button', { name: `Switch to ${theme} theme` }).click()
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
    await page.screenshot({ path: resolve(artifactDir, `overview-390-${theme}.png`), fullPage: true })
    await menu.click()
    await page.getByRole('link', { name: 'Runners', exact: true }).click()
    await expectMobileDrawerClosed(page, menu)
    await page.getByRole('button', { name: 'Create pool' }).click()
    const dialog = page.getByRole('dialog', { name: 'Create runner pool' })
    await expect(dialog).toBeVisible()
    await dialog.getByLabel('Name').focus()
    await page.keyboard.press('Tab')
    await expect.poll(async () => dialog.evaluate(element => element.contains(document.activeElement))).toBe(true)
    const bounds = await dialog.boundingBox()
    expect(bounds && bounds.x >= 0 && bounds.x + bounds.width <= 390).toBeTruthy()
    await page.keyboard.press('Escape')
    await expect(dialog).toBeHidden()
    await menu.click()
    await page.getByRole('link', { name: 'Policies', exact: true }).click()
    await expect(page.getByRole('heading', { name: 'Policies', exact: true })).toBeVisible()
    await expectMobileDrawerClosed(page, menu)
    await page.screenshot({ path: resolve(artifactDir, `policies-390-${theme}.png`), fullPage: true })
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.screenshot({ path: resolve(artifactDir, `policies-1440-${theme}.png`), fullPage: true })
    await page.setViewportSize({ width: 390, height: 844 })
  }
})

async function expectMobileDrawerClosed(page: Page, menu: Locator) {
  const sidebar = page.locator('#primary-navigation')
  await expect(menu).toHaveAttribute('aria-expanded', 'false')
  await expect.poll(async () => sidebar.evaluate(element => element.getBoundingClientRect().right)).toBeLessThan(0)
  await expect(page.locator('.main-column')).toHaveJSProperty('inert', false)
}
