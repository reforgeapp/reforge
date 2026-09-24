import { mkdir, writeFile } from 'node:fs/promises'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { test, expect } from '@playwright/test'

const base = process.env.REFORGE_CONNECTIONS_MOBILE_URL
const org = '00000000-0000-4000-8000-000000000001'
const artifactDir = process.env.REFORGE_CONNECTIONS_MOBILE_ARTIFACTS || fileURLToPath(new URL('../../.local/t29-mobile-connections/', import.meta.url))

test('Connections remain usable on narrow and desktop layouts with persisted backend data', async ({ page }) => {
  test.skip(!base, 'Set REFORGE_CONNECTIONS_MOBILE_URL to isolated fixture app.')
  await mkdir(artifactDir, { recursive: true })
  const name = 'Mobile Gitea'
  const apiFailures: string[] = []
  page.on('response', response => {
    if (response.url().includes('/api/v1/') && response.status() >= 400) apiFailures.push(`${response.status()} ${new URL(response.url()).pathname}`)
  })
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto(`${base}/auth/login`, { waitUntil: 'networkidle' })
  await page.goto(`${base}/org/${org}/connections`, { waitUntil: 'networkidle' })
  await expect(page.getByRole('heading', { name: 'Connections', exact: true })).toBeVisible()
  if (!(await page.getByRole('button', { name, exact: true }).count())) {
    await page.getByRole('button', { name: 'Add connection' }).click()
    const dialog = page.getByRole('dialog')
    await dialog.getByLabel('Provider').selectOption('gitea')
    await dialog.getByLabel('Name', { exact: true }).fill(name)
    await dialog.getByLabel('Server URL').fill('https://gitea.example.invalid')
    await dialog.getByLabel('Personal access token', { exact: true }).fill('disposable-mobile-secret')
    await dialog.getByRole('button', { name: 'Create connection' }).click()
  }
  const row = page.locator('tr').filter({ hasText: name })
  await expect(row).toBeVisible()
  await expect(row.getByText('gitea', { exact: true })).toBeVisible()
  await expect(row.getByText('unverified', { exact: true })).toBeVisible()
  await expect(row.getByRole('button', { name: 'Open' })).toBeVisible()
  const mobileWidths = await page.evaluate(() => {
    const table = document.querySelector<HTMLTableElement>('.connections-table')!
    const wrapper = table.closest('.table-wrap')!
    return { viewport: innerWidth, document: document.documentElement.scrollWidth, table: table.getBoundingClientRect().width, tableContent: table.scrollWidth, wrapper: wrapper.clientWidth }
  })
  expect(mobileWidths.document).toBeLessThanOrEqual(mobileWidths.viewport)
  expect(mobileWidths.tableContent).toBeLessThanOrEqual(mobileWidths.wrapper)
  await page.screenshot({ path: resolve(artifactDir, 'connections-after-390.png') })
  const providerCell = row.locator('.connection-provider')
  for (const provider of ['custom_command', 'claude_code']) {
    const layout = await providerCell.evaluate((cell, value: string) => {
      cell.textContent = value
      const range = document.createRange()
      range.selectNodeContents(cell)
      const fragments = Array.from(range.getClientRects())
      const cellBounds = cell.getBoundingClientRect()
      return { textRight: Math.max(...fragments.map(fragment => fragment.right)), cellRight: cellBounds.right }
    }, provider)
    expect(layout.textRight).toBeLessThanOrEqual(layout.cellRight + 1)
  }
  await providerCell.evaluate(cell => { cell.textContent = 'gitea' })
  await page.evaluate(() => { document.documentElement.dataset.theme = 'dark'; localStorage.setItem('reforge-theme', 'dark') })
  await page.screenshot({ path: resolve(artifactDir, 'connections-after-dark-390.png') })
  await page.evaluate(() => { document.documentElement.dataset.theme = 'light'; localStorage.setItem('reforge-theme', 'light') })
  await page.reload({ waitUntil: 'networkidle' })
  await expect(page.locator('tr').filter({ hasText: name })).toBeVisible()
  const open = page.locator('tr').filter({ hasText: name }).getByRole('button', { name: 'Open' })
  await open.focus()
  await page.keyboard.press('Enter')
  const detail = page.locator('.split-detail')
  await expect(detail).toBeVisible()
  await expect(detail.getByRole('heading', { name, exact: true })).toBeVisible()
  await expect(detail.getByText('https://gitea.example.invalid', { exact: true })).toBeVisible()
  const close = detail.getByRole('button', { name: 'Close connection details', exact: true })
  await close.focus()
  await page.keyboard.press('Enter')
  await expect(detail).toBeHidden()
  await expect(page.locator('tr').filter({ hasText: name })).toBeVisible()
  await expect(page.locator('tr').filter({ hasText: name }).getByRole('button', { name: 'Open' })).toBeFocused()
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.locator('h1').click()
  await page.screenshot({ path: resolve(artifactDir, 'connections-after-1440.png') })
  const desktopWidths = await page.evaluate(() => ({ viewport: innerWidth, document: document.documentElement.scrollWidth, table: document.querySelector('.connections-table')?.getBoundingClientRect().width }))
  expect(desktopWidths.document).toBeLessThanOrEqual(desktopWidths.viewport)
  expect(apiFailures).toEqual([])
  await writeFile(resolve(artifactDir, 'widths.json'), JSON.stringify({ mobile: mobileWidths, desktop: desktopWidths }, null, 2))
})
