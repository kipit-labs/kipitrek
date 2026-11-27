import { expect, importGPX, test } from './fixtures'

// Mobile first: no page scrolls sideways, at phone size above all.
test('every page fits the screen width', async ({ page, account: _ }) => {
  const hike = await importGPX(page, 'track.gpx')
  for (const path of ['/', '/hikes', `/hikes/${hike.id}`, '/calendar', '/stats', '/feed', '/friends', '/settings']) {
    await page.goto(path)
    await page.waitForLoadState('networkidle')
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
    expect(overflow, `${path} scrolls sideways by ${overflow}px`).toBeLessThanOrEqual(0)
  }
})
