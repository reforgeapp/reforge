import { test, expect, type Page } from '@playwright/test'

const org = '00000000-0000-4000-8000-000000000001'
const invitationID = '00000000-0000-4000-8000-000000000101'
const token = 'a'.repeat(43)
const nextToken = 'b'.repeat(43)
const authorizationURL = 'https://idp.example.test/authorize?state=fixture'
const settings = { configured: true, secret_present: true, issuer: 'https://idp.example.test', client_id: 'reforge', status: 'active', version: 4, verified: true, activation_available: false }
const invitation = { id: invitationID, email: 'new@example.test', role: 'viewer', expires_at: new Date(Date.now() + 86400000).toISOString(), created_at: new Date().toISOString(), redeemed: false }
const meta = { name: 'Reforge', version: 'test', edition: 'self-hosted', development: false, fixture_auth: false }

async function owner(page: Page) {
  await page.route('**/api/v1/session', route => route.fulfill({ json: { user: { id: 'owner-1', name: 'Owner', email: 'owner@example.test' }, organisations: [{ id: org, name: 'Fixture', version: 1 }], memberships: [{ org_id: org, role: 'owner', team_ids: [], repository_ids: [], all_repositories: true }], csrf_token: 'csrf-1' } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: meta }))
  await page.route(`**/api/v1/orgs/${org}/identity/oidc`, route => route.fulfill({ json: settings }))
}

async function invitee(page: Page) {
  await page.route('**/api/v1/session', route => route.fulfill({ status: 401, json: { code: 'unauthenticated', message: 'Sign in required', request_id: 'invite-test', retryable: false } }))
  await page.route('**/api/v1/meta', route => route.fulfill({ json: meta }))
  await page.route('https://idp.example.test/**', route => route.fulfill({ status: 200, contentType: 'text/html', body: '<title>IdP</title>Identity provider sign-in' }))
}

test('owner creates one-time invitation, revokes pending invite and keeps link out of storage', async ({ page }) => {
  let createdBody: Record<string, unknown> | undefined
  let revoked = false
  await owner(page)
  await page.route(`**/api/v1/orgs/${org}/identity/oidc/invitations**`, async route => {
    const method = route.request().method()
    if (method === 'POST') {
      createdBody = route.request().postDataJSON() as Record<string, unknown>
      return route.fulfill({ status: 201, json: { ...invitation, email: createdBody.email, role: createdBody.role, redemption_url: `http://127.0.0.1:8080/invite#token=${token}` } })
    }
    if (method === 'DELETE') { revoked = true; return route.fulfill({ status: 204 }) }
    return route.fulfill({ json: { items: revoked ? [] : [invitation], complete: true } })
  })
  await page.goto(`/org/${org}/organisation#token=${token}`)
  await page.getByRole('tab', { name: 'Identity' }).click()
  await expect(page.getByRole('heading', { name: 'Invitations' })).toBeVisible()
  expect(new URL(page.url()).hash).toBe(`#token=${token}`)
  await expect(page.getByText('new@example.test')).toBeVisible()
  await page.getByLabel('Email', { exact: true }).fill('next@example.test')
  await page.getByLabel('Role').last().selectOption('reviewer')
  await page.getByLabel('Expires').selectOption('30')
  await page.getByRole('button', { name: 'Create invitation' }).click()
  await expect.poll(() => createdBody?.email).toBe('next@example.test')
  expect(createdBody?.role).toBe('reviewer')
  expect(Date.parse(String(createdBody?.expires_at))).toBeGreaterThan(Date.now() + 29 * 86400000)
  const dialog = page.getByRole('dialog', { name: 'Invitation created' })
  await expect(dialog.getByLabel('Invitation link')).toHaveValue(new RegExp(`#token=${token}$`))
  expect(await page.evaluate(() => JSON.stringify({ ...localStorage, ...sessionStorage }))).not.toContain(token)
  await dialog.getByRole('button', { name: 'Done' }).click()
  await expect(dialog).toHaveCount(0)
  await expect(page.locator('main')).not.toContainText(token)
  await page.getByRole('button', { name: 'Revoke' }).click()
  const revokeDialog = page.getByRole('dialog', { name: 'Revoke invitation' })
  await expect(revokeDialog).toBeVisible()
  await revokeDialog.getByRole('button', { name: 'Revoke' }).click()
  await expect.poll(() => revoked).toBe(true)
  await expect(page.getByText('No invitations')).toBeVisible()
})

test('clipboard and revoke failures stay inside their dialogs', async ({ page }) => {
  await owner(page)
  await page.addInitScript(() => { Object.defineProperty(navigator, 'clipboard', { value: { writeText: () => Promise.reject(new Error('denied')) } }) })
  await page.route(`**/api/v1/orgs/${org}/identity/oidc/invitations**`, route => {
    const method = route.request().method()
    if (method === 'POST') return route.fulfill({ status: 201, json: { ...invitation, redemption_url: `http://127.0.0.1:8080/invite#token=${token}` } })
    if (method === 'DELETE') return route.fulfill({ status: 409, json: { code: 'conflict', message: 'Invitation already redeemed', request_id: 'revoke-conflict', retryable: false } })
    return route.fulfill({ json: { items: [invitation], complete: true } })
  })
  await page.goto(`/org/${org}/organisation`)
  await page.getByRole('tab', { name: 'Identity' }).click()
  await page.getByLabel('Email', { exact: true }).fill('new@example.test')
  await page.getByRole('button', { name: 'Create invitation' }).click()
  const created = page.getByRole('dialog', { name: 'Invitation created' })
  await created.getByRole('button', { name: 'Copy link' }).click()
  await expect(created.getByRole('alert')).toHaveText('Clipboard unavailable. Select and copy the link.')
  await created.getByRole('button', { name: 'Done' }).click()
  await page.getByRole('button', { name: 'Revoke' }).click()
  const revokeDialog = page.getByRole('dialog', { name: 'Revoke invitation' })
  await revokeDialog.getByRole('button', { name: 'Revoke' }).click()
  await expect(revokeDialog.getByRole('alert')).toHaveText('Invitation already redeemed')
})

test('cached invitations stay visible when refresh fails and retry recovers', async ({ page }) => {
  let failing = false
  await owner(page)
  await page.route(`**/api/v1/orgs/${org}/identity/oidc/invitations**`, route => {
    if (route.request().method() === 'POST') { failing = true; return route.fulfill({ status: 201, json: { ...invitation, redemption_url: `http://127.0.0.1:8080/invite#token=${token}` } }) }
    if (failing) return route.fulfill({ status: 503, json: { code: 'unavailable', message: 'Database unavailable', request_id: 'list-fail', retryable: true } })
    return route.fulfill({ json: { items: [invitation], complete: true } })
  })
  await page.goto(`/org/${org}/organisation`)
  await page.getByRole('tab', { name: 'Identity' }).click()
  await expect(page.getByText('new@example.test')).toBeVisible()
  await page.getByLabel('Email', { exact: true }).fill('next@example.test')
  await page.getByRole('button', { name: 'Create invitation' }).click()
  await page.getByRole('dialog', { name: 'Invitation created' }).getByRole('button', { name: 'Done' }).click()
  const alert = page.locator('.identity-invitations').getByRole('alert')
  await expect(alert).toContainText('Invitations unavailable: Database unavailable')
  await expect(page.getByText('new@example.test')).toBeVisible()
  failing = false
  await alert.getByRole('button', { name: 'Retry' }).click()
  await expect(alert).toHaveCount(0)
  await expect(page.getByText('new@example.test')).toBeVisible()
})

test('public invite strips fragment, posts form body and navigates to returned authorization URL', async ({ page }) => {
  let submitted: { body: string; contentType: string; origin: string; accept: string } | undefined
  await invitee(page)
  await page.route('**/auth/invitations/redeem', route => {
    const headers = route.request().headers()
    submitted = { body: route.request().postData() ?? '', contentType: headers['content-type'] ?? '', origin: headers.origin ?? '', accept: headers.accept ?? '' }
    return route.fulfill({ status: 200, json: { authorization_url: authorizationURL } })
  })
  await page.goto(`/invite#token=${token}`)
  await expect(page).toHaveURL(/\/invite$/)
  await expect(page.getByRole('heading', { name: 'Join your organisation' })).toBeVisible()
  await page.setViewportSize({ width: 390, height: 844 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390)
  await page.getByRole('button', { name: 'Accept invitation' }).focus()
  await page.keyboard.press('Enter')
  await expect(page).toHaveURL(authorizationURL)
  expect(submitted?.body).toBe(`token=${token}`)
  expect(submitted?.contentType).toContain('application/x-www-form-urlencoded')
  expect(submitted?.origin).toContain('127.0.0.1')
  expect(submitted?.accept).toBe('application/json')
})

test('rejected invitation drops token and offers sign-in without retry', async ({ page }) => {
  await invitee(page)
  await page.route('**/auth/invitations/redeem', route => route.fulfill({ status: 401, json: { code: 'unauthenticated', message: 'Authentication required', request_id: 'redeem-denied', retryable: false } }))
  await page.goto(`/invite#token=${token}`)
  await page.getByRole('button', { name: 'Accept invitation' }).click()
  await expect(page.getByRole('alert')).toHaveText('Invitation is invalid, expired or already used.')
  await expect(page.getByRole('link', { name: 'Sign in' })).toHaveAttribute('href', '/sign-in')
  await expect(page.getByRole('button', { name: /Retry|Accept invitation/ })).toHaveCount(0)
  expect(await page.content()).not.toContain(token)
})

test('transient redemption failure offers retry with the in-memory token', async ({ page }) => {
  const bodies: string[] = []
  await invitee(page)
  await page.route('**/auth/invitations/redeem', route => {
    bodies.push(route.request().postData() ?? '')
    return bodies.length === 1
      ? route.fulfill({ status: 429, json: { code: 'probe_capacity', message: 'Issuer probe capacity is busy', request_id: 'redeem-busy', retryable: true } })
      : route.fulfill({ status: 200, json: { authorization_url: authorizationURL } })
  })
  await page.goto(`/invite#token=${token}`)
  await page.getByRole('button', { name: 'Accept invitation' }).click()
  await expect(page.getByRole('alert')).toHaveText('Sign-in could not start.')
  expect(await page.content()).not.toContain(token)
  await page.getByRole('button', { name: 'Retry' }).click()
  await expect(page).toHaveURL(authorizationURL)
  expect(bodies).toEqual([`token=${token}`, `token=${token}`])
})

test('public invite with missing fragment recovers when a link fragment arrives', async ({ page }) => {
  let body = ''
  await invitee(page)
  await page.route('**/auth/invitations/redeem', route => { body = route.request().postData() ?? ''; return route.fulfill({ status: 200, json: { authorization_url: authorizationURL } }) })
  await page.goto('/invite')
  await expect(page.getByRole('alert')).toHaveText('Invitation link is missing or invalid.')
  await expect(page.getByRole('link', { name: 'Sign in' })).toHaveAttribute('href', '/sign-in')
  await page.evaluate(value => { window.location.hash = `token=${value}` }, nextToken)
  await page.getByRole('button', { name: 'Accept invitation' }).click()
  await expect(page).toHaveURL(authorizationURL)
  expect(body).toBe(`token=${nextToken}`)
})

test('invitation creation errors remain visible and preserve the address', async ({ page }) => {
  await owner(page)
  await page.route(`**/api/v1/orgs/${org}/identity/oidc/invitations**`, route => route.request().method() === 'POST'
    ? route.fulfill({ status: 409, json: { code: 'conflict', message: 'Invitation already exists', request_id: 'invite-conflict', retryable: false } })
    : route.fulfill({ json: { items: [], complete: true } }))
  await page.goto(`/org/${org}/organisation`)
  await page.getByRole('tab', { name: 'Identity' }).click()
  await page.getByLabel('Email', { exact: true }).fill('already@example.test')
  await page.getByRole('button', { name: 'Create invitation' }).click()
  await expect(page.getByRole('alert')).toHaveText('Invitation already exists')
  await expect(page.getByLabel('Email', { exact: true })).toHaveValue('already@example.test')
  await expect(page.getByRole('dialog', { name: 'Invitation created' })).toHaveCount(0)
})
