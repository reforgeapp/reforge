import { test, expect, type Page } from '@playwright/test'
import { mkdir, writeFile } from 'node:fs/promises'
import { resolve } from 'node:path'

const developmentOrg = '00000000-0000-4000-8000-000000000001'
const acceptanceOrg = '00000000-0000-4000-8000-000000000002'
const artifactDir = resolve(process.env.REFORGE_ACCEPTANCE_ARTIFACTS || '.local/t28a-connected-gui/artifacts')
const acceptanceEnabled = process.env.REFORGE_CONNECTED_ACCEPTANCE_URL === 'http://127.0.0.1:8090' && process.env.REFORGE_BASE_URL === process.env.REFORGE_CONNECTED_ACCEPTANCE_URL

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
  return data.organisations
}

async function switchOrganisation(page: Page, name: string) {
  await page.getByRole('button', { name: 'Switch organisation' }).click()
  await page.getByRole('menuitem', { name }).click()
  const orgID = name === 'Acceptance' ? acceptanceOrg : developmentOrg
  await expect(page).toHaveURL(new RegExp('/org/' + orgID + '/[^?#]+$'))
}

test('real fixture-auth journey persists org-scoped GUI changes and blocks unqualified Codex', async ({ page }) => {
  test.skip(!acceptanceEnabled, 'requires explicitly selected isolated backend at 127.0.0.1:8090')
  await mkdir(artifactDir, { recursive: true })
  const requests: string[] = []
  page.on('request', request => {
    if (request.url().includes('/api/v1/')) requests.push(`${request.method()} ${new URL(request.url()).pathname}`)
  })
  await signIn(page)
  await switchOrganisation(page, 'Acceptance')
  await page.getByRole('link', { name: 'Organisation', exact: true }).click()
  await expect(page.getByRole('tab', { name: 'Teams', exact: true })).toHaveAttribute('aria-selected', 'true')

  const teamName = `connected-${Date.now()}`
  const created = page.waitForResponse(response => response.request().method() === 'PUT' && response.url().includes(`/api/v1/orgs/${acceptanceOrg}/teams/`))
  await page.getByRole('button', { name: 'Create team' }).click()
  const teamDialog = page.getByRole('dialog', { name: 'Create team' })
  await teamDialog.getByLabel('Team name').fill(teamName)
  await teamDialog.getByRole('button', { name: 'Create team' }).click()
  expect((await created).ok()).toBeTruthy()
  await expect(page.getByRole('navigation', { name: 'Teams' }).getByRole('button', { name: new RegExp(teamName) })).toBeVisible()

  await page.getByRole('link', { name: 'Repositories', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Repositories', exact: true })).toBeVisible()
  await page.getByRole('link', { name: 'Organisation', exact: true }).click()
  await expect(page.getByRole('navigation', { name: 'Teams' }).getByRole('button', { name: new RegExp(teamName) })).toBeVisible()
  await page.reload()
  await expect(page.getByRole('navigation', { name: 'Teams' }).getByRole('button', { name: new RegExp(teamName) })).toBeVisible()
  const persisted = await page.request.get(`/api/v1/orgs/${acceptanceOrg}/teams?limit=100`)
  expect(persisted.ok()).toBeTruthy()
  expect((await persisted.json()).items.map((team: { name: string }) => team.name)).toContain(teamName)

  await page.getByRole('link', { name: 'Connections', exact: true }).click()
  await page.getByRole('button', { name: 'Add connection' }).click()
  const dialog = page.getByRole('dialog', { name: 'Add connection' })
  await dialog.getByLabel('Kind').selectOption('agent')
  const connectionName = `Codex acceptance route ${Date.now()}`
  await dialog.getByLabel('Name').fill(connectionName)
  await dialog.getByLabel('Endpoint').fill('https://runtime.example.invalid')
  const connectionWrite = page.waitForResponse(response => response.request().method() === 'POST' && response.url().includes(`/api/v1/orgs/${acceptanceOrg}/connections/agents`))
  await dialog.getByRole('button', { name: 'Create connection' }).click()
  expect((await connectionWrite).ok()).toBeTruthy()
  await page.getByRole('button', { name: 'Models & agents' }).click()
  const connectionRow = page.getByRole('row', { name: new RegExp(connectionName) })
  await expect(connectionRow).toBeVisible()
  await connectionRow.getByRole('button', { name: 'Open' }).click()
  await page.getByRole('button', { name: 'Qualification' }).click()
  const officialSignIn = page.getByRole('button', { name: 'Sign in with official runtime' })
  await expect(officialSignIn).toBeDisabled()
  await expect(page.getByText(/stays disabled until the operator configures an isolated runtime and records credential-custody evidence/i)).toBeVisible()

  await switchOrganisation(page, 'Development')
  await page.getByRole('link', { name: 'Organisation', exact: true }).click()
  await expect(page.getByRole('navigation', { name: 'Teams' }).getByRole('button', { name: new RegExp(teamName) })).toHaveCount(0)
  expect(requests.some(item => item.startsWith(`PUT /api/v1/orgs/${acceptanceOrg}/teams/`))).toBe(true)
  expect(requests.some(item => item.startsWith(`GET /api/v1/orgs/${developmentOrg}/teams`))).toBe(true)
  await writeFile(resolve(artifactDir, 'backend-requests.json'), JSON.stringify(requests, null, 2))
})

test('current connected GUI stays keyboard operable at 390px in light and dark themes', async ({ page }) => {
  test.skip(!acceptanceEnabled, 'requires explicitly selected isolated backend at 127.0.0.1:8090')
  await mkdir(artifactDir, { recursive: true })
  await page.setViewportSize({ width: 390, height: 844 })
  await signIn(page)
  await switchOrganisation(page, 'Acceptance')
  await expect(page.getByRole('heading', { name: 'Overview', exact: true })).toBeVisible()

  const menu = page.getByRole('button', { name: 'Menu' })
  await menu.focus()
  await page.keyboard.press('Enter')
  const sidebar = page.locator('#primary-navigation')
  await expect(sidebar.getByRole('link').first()).toBeFocused()
  await expect(page.locator('.main-column')).toHaveJSProperty('inert', true)
  await page.keyboard.press('Escape')
  await expect(menu).toBeFocused()

  for (const theme of ['light', 'dark'] as const) {
    const currentTheme = await page.locator('html').getAttribute('data-theme')
    if (currentTheme !== theme) await page.getByRole('button', { name: `Switch to ${theme} theme` }).click()
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
    await expect.poll(async () => sidebar.evaluate(element => element.getBoundingClientRect().right)).toBeLessThan(0)
    await page.screenshot({ path: resolve(artifactDir, `overview-390-${theme}.png`), fullPage: true })
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.screenshot({ path: resolve(artifactDir, `overview-1440-${theme}.png`), fullPage: true })
    await page.setViewportSize({ width: 390, height: 844 })
    await expect.poll(async () => sidebar.evaluate(element => element.getBoundingClientRect().right)).toBeLessThan(0)
  }

  await menu.click()
  await page.getByRole('link', { name: 'Connections', exact: true }).click()
  await expect(menu).toHaveAttribute('aria-expanded', 'false')
  await page.getByRole('button', { name: 'Add connection' }).click()
  const dialog = page.getByRole('dialog', { name: 'Add connection' })
  await expect(dialog).toBeVisible()
  await expect.poll(async () => dialog.evaluate(element => element.contains(document.activeElement))).toBe(true)
  await page.keyboard.press('Tab')
  await expect.poll(async () => dialog.evaluate(element => element.contains(document.activeElement))).toBe(true)
  const bounds = await dialog.boundingBox()
  expect(bounds && bounds.x >= 0 && bounds.x + bounds.width <= 390).toBeTruthy()
  await page.keyboard.press('Escape')
  await expect(dialog).toBeHidden()
})
