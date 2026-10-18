import { test, expect, type Page } from '@playwright/test'

test.use({ trace: 'off' })

const org = '00000000-0000-4000-8000-000000000001'
const base = `/api/v1/orgs/${org}`
const setup = '00000000-0000-4000-8000-000000000021'
const connection = { id: '00000000-0000-4000-8000-000000000022', org_id: org, kind: 'model', provider: 'openai', name: 'ChatGPT', endpoint: 'https://api.openai.com/v1', state: 'healthy', reason: '', version: 1, credential_version: 1, capabilities: {}, settings: { model: 'eligible-model', profile: 'chatgpt_plan', auth_kind: 'chatgpt_oauth', billing_route: 'subscription', namespace: 'owner@example.test' } }

async function fixture(page: Page) {
  await page.route(`**${base}/**`, route => route.fulfill({ json: { items: [], complete: true } }))
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'owner', name: 'Owner' }, organisations: [{ id: org, name: 'Local' }], memberships: [{ org_id: org, role: 'owner', all_repositories: true }], csrf_token: 'csrf' } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: { development: true, fixture_auth: true } }))
}

test('ChatGPT plan explains unavailable hosted approval without asking for an API key', async ({ page }) => {
  await fixture(page)
  await page.route(`**${base}/chatgpt`, route => route.fulfill({ json: { available: false, reason: 'ChatGPT plan connection awaits OpenAI hosted integration approval' } }))
  await page.goto(`/org/${org}/connections?connection_tab=models`)
  await page.getByRole('button', { name: 'Add connection', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Add connection' })
  await dialog.getByLabel('Connection type').selectOption('chatgpt')
  await expect(dialog.getByText(/awaits OpenAI hosted integration approval/)).toBeVisible()
  await expect(dialog.getByRole('button', { name: 'Continue with ChatGPT' })).toBeDisabled()
  await expect(dialog.getByLabel('API key', { exact: true })).toHaveCount(0)
})

test('owner completes consent with account-specific model and plan billing', async ({ page }) => {
  await fixture(page)
  let connected = false
  await page.route(`**${base}/connections?**`, route => route.fulfill({ json: { items: connected ? [connection] : [], complete: true } }))
  await page.route(`**${base}/connections/${connection.id}`, route => route.fulfill({ json: connection }))
  await page.route(`**${base}/chatgpt/setups/${setup}`, async route => {
    if (route.request().method() === 'POST') {
      expect(route.request().postDataJSON()).toEqual({ model: 'eligible-model' })
      expect(route.request().headers()['x-csrf-token']).toBe('csrf')
      connected = true
      await route.fulfill({ status: 201, json: connection })
    } else {
      await route.fulfill({ json: { id: setup, name: 'ChatGPT', account: 'owner@example.test', models: [{ id: 'eligible-model', name: 'Eligible model' }] } })
    }
  })
  await page.goto(`/org/${org}/connections?connection_tab=models&chatgpt_result=authorized&chatgpt_setup=${setup}`)
  const dialog = page.getByRole('dialog', { name: 'Connect ChatGPT plan' })
  await expect(dialog.getByText('owner@example.test')).toBeVisible()
  await expect(dialog.getByLabel('Model', { exact: true })).toHaveValue('eligible-model')
  await dialog.getByRole('button', { name: 'Connect plan', exact: true }).click()
  await expect(dialog).not.toBeVisible()
  await expect(page.getByText('ChatGPT plan connected. Eligible repairs use your plan.')).toBeVisible()
  await expect(page).not.toHaveURL(/chatgpt_setup/)
  await expect(page.getByRole('form', { name: 'Model pricing' })).toHaveCount(0)
  expect(connected).toBe(true)
})

test('private self-hosted sign-in downloads local callback helper before opening ChatGPT', async ({ page }) => {
  await fixture(page)
  await page.route(`**${base}/chatgpt`, route => route.fulfill({ json: { available: true } }))
  await page.route(`**${base}/chatgpt/setups`, route => route.fulfill({ status: 201, json: { url: 'https://auth.openai.com/api/accounts/authorize?client_id=dynamic_agent_client', helper: 'print("fixture helper")' } }))
  await page.goto(`/org/${org}/connections?connection_tab=models`)
  await page.getByRole('button', { name: 'Add connection', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Add connection' })
  await dialog.getByLabel('Connection type').selectOption('chatgpt')
  const download = page.waitForEvent('download')
  await dialog.getByRole('button', { name: 'Continue with ChatGPT' }).click()
  expect((await download).suggestedFilename()).toBe('reforge-chatgpt.py')
  await expect(dialog.getByRole('status')).toContainText('Python 3 on this computer')
  await expect(page).toHaveURL(new RegExp(`/org/${org}/connections`))
  await expect(dialog.getByRole('button', { name: 'Cancel', exact: true })).toBeEnabled()
  await expect(dialog.getByLabel('API key', { exact: true })).toHaveCount(0)
})
