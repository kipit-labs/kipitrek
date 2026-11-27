import { expect, openNav, PASSWORD, test } from './fixtures'

test('sign out, a wrong password, and sign in again', async ({ page, account, phone }) => {
  await openNav(page, phone)
  await page.getByRole('button', { name: account.email }).click()
  await page.getByRole('menuitem', { name: 'Log out' }).click()
  await expect(page).toHaveURL(/\/login$/)

  await page.getByLabel('Email').fill(account.email)
  await page.getByLabel('Password').fill('not my password')
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page.getByText('Invalid email or password.')).toBeVisible()

  await page.getByLabel('Password').fill(PASSWORD)
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page.getByRole('region', { name: 'Map' })).toBeVisible()
})

test('signed-out visitors are sent to sign in', async ({ visitor }) => {
  const page = await visitor()
  await page.goto('/hikes')
  await expect(page).toHaveURL(/\/login/)
})

test('the language switch translates the page', async ({ visitor }) => {
  const page = await visitor()
  await page.goto('/register')
  await page.getByRole('radio', { name: 'Français' }).click()
  await expect(page.getByText('Créer un compte').first()).toBeVisible()
  await page.getByRole('radio', { name: 'English' }).click()
  await expect(page.getByText('Create an account').first()).toBeVisible()
})
