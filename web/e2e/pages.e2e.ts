import type { Locator, Page } from '@playwright/test'
import postgres from 'postgres'

import { databaseURL } from '../playwright.config'
import { test as base, expect, importGPX, PASSWORD } from './fixtures'

// Every page and every tab, opened once with data in it. The app has no error boundary: a component that throws
// while rendering blanks the page and reports an uncaught error, which fails only a test that opens that page. The
// journeys test what pages do; this, that they all open.

/** Waits for the page's data to arrive, then checks the page is still there: a render error empties it. */
async function renders(page: Page, where: Locator) {
  await expect(where).toBeVisible()
  await page.waitForLoadState('networkidle')
  await expect(where).toBeVisible()
}

const h1 = (page: Page, name: string) => page.getByRole('heading', { name, level: 1, exact: true })

/** Opens each section of a page that has its own URL, from the page's own navigation. */
async function everySection(page: Page, nav: Locator) {
  await expect(nav.getByRole('link').first()).toBeVisible()
  for (const name of await nav.getByRole('link').allInnerTexts()) {
    await page.goto((await nav.getByRole('link', { name, exact: true }).getAttribute('href'))!)
    await renders(page, h1(page, name).first()) // the header and the section both name it
  }
}

/** Picks every option of a segmented control in turn: each one renders the page another way. */
async function everyOption(page: Page, name: string, where: Locator) {
  const group = page.getByRole('radiogroup', { name })
  const options = group.getByRole('radio')
  await expect(options.first()).toBeVisible()
  for (const name of await options.allInnerTexts()) {
    const option = group.getByRole('radio', { name, exact: true })
    if (await option.isDisabled()) continue
    await option.click()
    await expect(option).toBeChecked()
    await expect(where).toBeVisible()
  }
}

const test = base.extend<{ friend: { page: Page; id: string }; admin: void }>({
  /** A second account, signed up through the API in its own browser and friends with account. Deleted after. */
  friend: async ({ page, account, visitor }, use) => {
    const friend = await visitor()
    const email = `e2e-${crypto.randomUUID()}@test.local`
    for (const step of ['register', 'login']) {
      expect((await friend.request.post(`/api/auth/${step}`, { data: { email, password: PASSWORD } })).ok()).toBeTruthy()
    }
    const { id } = await (await friend.request.get('/api/auth/me')).json()
    expect((await friend.request.put(`/api/friends/${account.id}`)).ok()).toBeTruthy()
    expect((await page.request.put(`/api/friends/${id}`)).ok()).toBeTruthy()

    await use({ page: friend, id })

    await friend.request.delete('/api/me', { data: { password: PASSWORD } })
  },
  /** Makes account an admin, straight in the database: only an admin can make one. */
  admin: async ({ account }, use) => {
    const sql = postgres(databaseURL)
    await sql`UPDATE users SET is_admin = true WHERE id = ${account.id}`
    await use()
    // Demoted again, or the account could be the last admin, which can't be deleted.
    await sql`UPDATE users SET is_admin = false WHERE id = ${account.id}`
    await sql.end()
  },
})

test('every list of your hikes opens, with hikes in it', async ({ page, account }) => {
  const hike = await importGPX(page, 'track.gpx')
  await importGPX(page, 'no_time_no_ele.gpx')

  await page.goto('/')
  await renders(page, page.getByRole('region', { name: 'Map' }))
  await page.goto('/hikes')
  await renders(page, page.getByRole('link', { name: hike.name }).first())
  // July 2026, the month of track.gpx.
  await page.goto('/calendar?month=2026-07')
  await renders(page, page.getByRole('link', { name: hike.name }))
  await page.goto(`/u/${account.id}`)
  await renders(page, page.getByRole('link', { name: hike.name }).first())

  await page.goto('/stats')
  await renders(page, page.getByRole('heading', { name: 'By year' }))
  await everyOption(page, 'Metric', page.getByRole('heading', { name: 'By year' }))
  await page.getByRole('combobox', { name: 'Period' }).click()
  await page.getByRole('option', { name: '2026' }).click()
  await renders(page, page.getByRole('heading', { name: 'By month in 2026' }))
})

test('every view of a hike opens, planned and recording too', async ({ page, account: _ }) => {
  const hike = await importGPX(page, 'track.gpx')
  const undated = await importGPX(page, 'no_time_no_ele.gpx')
  const planned = await importGPX(page, 'route.gpx', true)

  await page.goto(`/hikes/${hike.id}`)
  await renders(page, h1(page, hike.name))
  await everyOption(page, 'X axis', h1(page, hike.name))
  await everyOption(page, 'Speed unit', h1(page, hike.name))
  // No time and no elevation: nothing to chart.
  await page.goto(`/hikes/${undated.id}`)
  await renders(page, page.getByText('This GPX has no elevation, time or sensor data to chart.'))

  await page.goto('/record')
  await renders(page, page.getByRole('button', { name: 'Start recording' }))
  await page.goto(`/hikes/${planned.id}`)
  await renders(page, h1(page, planned.name))
  await page.getByRole('button', { name: 'Mark as done' }).click()
  await expect(page.getByRole('dialog', { name: 'Mark as done' })).toBeVisible()
  await page.keyboard.press('Escape')
  await page.getByRole('button', { name: 'Follow on the trail' }).click()
  await expect(page).toHaveURL(/\/record$/)
  await renders(page, page.getByText(`Following ${planned.name}`))
})

test('every settings section opens', async ({ page, account: _ }) => {
  await page.goto('/settings')
  await everySection(page, page.getByRole('navigation', { name: 'Settings' }))
})

test('every page about a friend opens, with their hikes in it', async ({ page, account: _, friend }) => {
  const hike = await importGPX(friend.page, 'track.gpx')
  const shared = await friend.page.request.patch('/api/me', { data: { name: 'Trail Friend', visibility: 'friends' } })
  expect(shared.ok()).toBeTruthy()

  await page.goto('/feed')
  await renders(page, page.getByRole('link', { name: hike.name }).first())
  await page.goto('/friends')
  await renders(page, page.getByRole('link', { name: 'Trail Friend' }).first())
  await page.goto(`/u/${friend.id}`)
  await renders(page, page.getByRole('link', { name: hike.name }).first())
  await page.goto(`/hikes/${hike.id}`)
  await renders(page, h1(page, hike.name))
})

test('every shared and signed-out page opens', async ({ page, account, visitor }) => {
  const hike = await importGPX(page, 'track.gpx')
  expect((await page.request.patch('/api/me', { data: { visibility: 'public' } })).ok()).toBeTruthy()
  const anon = await visitor()

  await anon.goto(`/u/${account.id}`)
  await renders(anon, anon.getByRole('link', { name: hike.name }).first())
  await anon.goto(`/hikes/${hike.id}`)
  await renders(anon, h1(anon, hike.name))

  for (const [path, text] of [
    ['/login', 'Welcome back'],
    ['/register', 'Create an account'],
    ['/forgot-password', 'Ask an admin'], // no mail server: an admin hands out the link
    ['/reset-password?token=nope', 'Choose a new password'],
    ['/verify?token=nope', 'Could not confirm your email'],
  ]) {
    await anon.goto(path)
    await renders(anon, anon.getByText(text, { exact: true }).first())
  }
})

test('every admin page opens', async ({ page, account, admin: _ }) => {
  await page.goto('/admin')
  await renders(page, h1(page, 'Admin'))
  await page.goto(`/admin/users/${account.id}`)
  await renders(page, h1(page, account.email.split('@')[0]))
})
