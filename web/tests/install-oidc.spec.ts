import { test, expect } from '@playwright/test'

const base = process.env.REFORGE_INSTALL_URL

test.use({
  ignoreHTTPSErrors: true,
  launchOptions: {
    executablePath: process.env.PLAYWRIGHT_CHROMIUM_PATH,
    args: ['--ignore-certificate-errors', ...(base ? [`--unsafely-treat-insecure-origin-as-secure=${base}`] : [])],
  },
})

test.describe('non-development OIDC install', () => {
  test.skip(!base, 'set REFORGE_INSTALL_URL to a non-development origin backed by a disposable OIDC issuer')

  test('completes an OIDC login without fixture authentication', async ({ page }) => {
    const origin = base as string
    await page.goto(`${origin}/auth/login`)
    await expect(page.getByRole('heading', { name: /No organisation access|Organisation access denied/ })).toBeVisible({ timeout: 30_000 })
    const session = await page.request.get(`${origin}/api/v1/session`, { headers: { Origin: origin } })
    expect(session.ok()).toBeTruthy()
    const body = await session.json() as { user: { email: string }; organisations: unknown[] }
    expect(body.user.email).toBe('owner@example.test')
    expect(Array.isArray(body.organisations)).toBe(true)
  })
})
