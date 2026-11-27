import type { Page } from '@playwright/test'

import { expect, importGPX, test } from './fixtures'

async function setVisibility(page: Page, name: RegExp) {
  const saved = page.waitForResponse((r) => r.url().endsWith('/api/me') && r.request().method() === 'PATCH' && r.ok())
  await page.getByRole('radio', { name }).click()
  await saved
  await expect(page.getByRole('radio', { name })).toBeChecked()
}

test('profile visibility decides who sees a hike', async ({ page, account: _, visitor }) => {
  const hike = await importGPX(page, 'track.gpx')
  const anon = await visitor()

  await anon.goto(`/hikes/${hike.id}`)
  await expect(anon.getByText('Hike not found')).toBeVisible()

  await page.goto('/settings')
  await setVisibility(page, /Public/)
  await expect(page.getByText('Visibility updated')).toBeVisible()
  await anon.reload()
  await expect(anon.getByRole('heading', { name: 'Col de la Croix' })).toBeVisible()

  await setVisibility(page, /Private/)
  await anon.reload()
  await expect(anon.getByText('Hike not found')).toBeVisible()
})
