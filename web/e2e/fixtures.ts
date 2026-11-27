import { readFileSync } from 'node:fs'
import path from 'node:path'
import { test as base, expect, request, type Browser, type BrowserContext, type Page } from '@playwright/test'

export { expect }

export const PASSWORD = 'correct horse battery'
export const gpxFile = (name: string) =>
  path.resolve(import.meta.dirname, '../../internal/infrastructure/gpx/testdata', name)

// The map's outside services (styles, tiles, fonts) are never reached: tests stay offline and deterministic. They get
// harmless answers rather than failed requests, which WebKit reports as uncaught errors: an empty style, an empty
// glyph set, a transparent tile.
const emptyStyle = JSON.stringify({ version: 8, sources: {}, layers: [] })
const transparentPNG = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=',
  'base64',
)
const cors = { 'access-control-allow-origin': '*' }

async function offline(context: BrowserContext) {
  await context.route(
    (url) => url.hostname !== 'localhost',
    (route) => {
      const path = new URL(route.request().url()).pathname
      if (path.endsWith('style.json')) return route.fulfill({ headers: cors, contentType: 'application/json', body: emptyStyle })
      if (path.endsWith('.pbf')) return route.fulfill({ headers: cors, contentType: 'application/x-protobuf', body: '' })
      if (/\.(png|jpe?g|webp)$/.test(path) || /\/tile\//.test(path))
        return route.fulfill({ headers: cors, contentType: 'image/png', body: transparentPNG })
      return route.fulfill({ headers: cors, status: 404, body: '' })
    },
  )
}

// Every uncaught error in a page fails the test that caused it. Except one: WebKit reports each request still in
// flight when a test navigates away (page.goto) as "… due to access control checks", same-origin ones included.
// shortcut: matched by message; revisit if a Playwright or WebKit update changes the wording.
const cancelledByNavigation = /due to access control checks\.$/

function watchErrors(page: Page, errors: string[]) {
  page.on('pageerror', (e) => {
    if (!cancelledByNavigation.test(e.message)) errors.push(e.message)
  })
}

type Account = { email: string; id: string }

type Fixtures = {
  /** A fresh account, signed up through the UI and signed in on page. Deleted after the test. */
  account: Account
  /** Opens a signed-out browser, offline like page. */
  visitor: () => Promise<Page>
  /** Whether the project runs at phone size, where navigation hides behind the sidebar toggle. */
  phone: boolean
}

// A client IP per test, so each test has its own rate limits (see playwright.config.ts).
const clientIP = () => `10.${[0, 0, 0].map(() => Math.floor(Math.random() * 254) + 1).join('.')}`

export const test = base.extend<Fixtures & { errors: string[] }>({
  extraHTTPHeaders: async ({}, use) => use({ 'X-Client-IP': clientIP() }),
  errors: async ({}, use) => {
    const errors: string[] = []
    await use(errors)
    expect(errors, 'uncaught errors in the page').toEqual([])
  },
  context: async ({ context }, use) => {
    await offline(context)
    await use(context)
  },
  page: async ({ page, errors }, use) => {
    watchErrors(page, errors)
    await use(page)
  },
  phone: async ({ isMobile }, use) => use(isMobile),
  visitor: async ({ browser, errors }, use) => {
    const contexts: BrowserContext[] = []
    await use(async () => {
      const context = await newContext(browser)
      contexts.push(context)
      const page = await context.newPage()
      watchErrors(page, errors)
      return page
    })
    await Promise.all(contexts.map((c) => c.close()))
  },
  account: async ({ page, baseURL }, use) => {
    const email = `e2e-${crypto.randomUUID()}@test.local`
    await page.goto('/register')
    await page.getByLabel('Email').fill(email)
    await page.getByLabel('Password').fill(PASSWORD)
    await page.getByRole('button', { name: 'Create account' }).click()
    await expect(page.getByRole('region', { name: 'Map' })).toBeVisible()
    const me = await (await page.request.get('/api/auth/me')).json()

    await use({ email, id: me.id })

    // Through its own session: the test may have signed the page out.
    const api = await request.newContext({ baseURL, extraHTTPHeaders: { 'X-Client-IP': clientIP() } })
    await api.post('/api/auth/login', { data: { email, password: PASSWORD } })
    await api.delete('/api/me', { data: { password: PASSWORD } })
    await api.dispose()
  },
})

async function newContext(browser: Browser) {
  const context = await browser.newContext({ ...test.info().project.use, extraHTTPHeaders: { 'X-Client-IP': clientIP() } })
  await offline(context)
  return context
}

/** Opens the app navigation: the sidebar on a phone, already there on a desktop. */
export async function openNav(page: Page, phone: boolean) {
  if (phone) await page.getByRole('button', { name: 'Toggle sidebar' }).click()
}

/** Imports a GPX file through the API, faster than the UI when the import is not what is tested. */
export async function importGPX(page: Page, file: string, planned = false) {
  const res = await page.request.post(`/api/hikes${planned ? '?planned=true' : ''}`, {
    multipart: { files: { name: file, mimeType: 'application/gpx+xml', buffer: readFileSync(gpxFile(file)) } },
  })
  expect(res.ok()).toBeTruthy()
  const { results } = await res.json()
  return results[0].hike as { id: string; name: string }
}
