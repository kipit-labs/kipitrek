import { expect, gpxFile, importGPX, openNav, test } from './fixtures'

test('import a GPX, open it, rename it and delete it', async ({ page, account: _, phone }) => {
  await openNav(page, phone)
  await page.getByRole('button', { name: 'Import GPX' }).click()
  const dialog = page.getByRole('dialog', { name: 'Import GPX files' })
  await dialog.locator('input[type=file]').setInputFiles(gpxFile('track.gpx'))
  await dialog.getByRole('button', { name: 'Import 1' }).click()
  await expect(page.getByText('Imported 1 hike')).toBeVisible()

  await page.goto('/hikes')
  await page.getByRole('link', { name: 'Col de la Croix' }).first().click()
  await expect(page).toHaveURL(/\/hikes\/[0-9a-f-]{36}$/)
  await expect(page.getByRole('heading', { name: 'Col de la Croix' })).toBeVisible()

  await page.getByRole('button', { name: 'Rename hike' }).click()
  await page.getByLabel('Hike name').fill('Croix du Sud')
  await page.getByRole('button', { name: 'Save name' }).click()
  await expect(page.getByRole('heading', { name: 'Croix du Sud' })).toBeVisible()

  await page.getByRole('button', { name: 'Delete hike' }).click()
  const confirm = page.getByRole('alertdialog')
  await expect(confirm).toContainText('Delete this hike?')
  await confirm.getByRole('button', { name: 'Delete' }).click()
  await expect(page.getByText('Hike deleted')).toBeVisible()

  await page.goto('/hikes')
  await expect(page.getByText('No hikes yet.')).toBeVisible()
})

test('a planned route is marked as planned', async ({ page, account: _, phone }) => {
  await openNav(page, phone)
  await page.getByRole('button', { name: 'Import GPX' }).click()
  const dialog = page.getByRole('dialog', { name: 'Import GPX files' })
  await dialog.locator('input[type=file]').setInputFiles(gpxFile('route.gpx'))
  await dialog.getByText('Routes I plan to do').click()
  await dialog.getByRole('button', { name: 'Import 1' }).click()
  await expect(page.getByText('Imported 1 planned hike')).toBeVisible()
})

test('an invalid file is reported, not imported', async ({ page, account: _, phone }) => {
  await openNav(page, phone)
  await page.getByRole('button', { name: 'Import GPX' }).click()
  const dialog = page.getByRole('dialog', { name: 'Import GPX files' })
  await dialog.locator('input[type=file]').setInputFiles(gpxFile('invalid.gpx'))
  await dialog.getByRole('button', { name: 'Import 1' }).click()
  await expect(page.getByText(/not a valid GPX/i)).toBeVisible()
  await page.goto('/hikes')
  await expect(page.getByText('No hikes yet.')).toBeVisible()
})

test('the hike page shows its stats', async ({ page, account: _ }) => {
  const hike = await importGPX(page, 'track.gpx')
  await page.goto(`/hikes/${hike.id}`)
  await expect(page.getByRole('heading', { name: 'Col de la Croix' })).toBeVisible()
  await expect(page.getByText('Distance').first()).toBeVisible()
  await expect(page.getByText('2.0 km').first()).toBeVisible()
})
