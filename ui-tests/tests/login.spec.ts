import { credentials, expect, loginAsAdmin, test } from './helpers';

test('login page renders and signs the admin in', async ({ page }) => {
  await page.goto('/');
  await expect(page.locator('#login-form')).toBeVisible();
  await expect(page.locator('#login-username')).toBeVisible();
  await expect(page.locator('#login-password')).toBeVisible();
  await loginAsAdmin(page);
  await expect(page.locator('#current-username')).toHaveText(credentials.adminUser);
});

test('wrong password shows an error', async ({ page }) => {
  await page.goto('/');
  await page.fill('#login-username', credentials.adminUser);
  await page.fill('#login-password', 'definitely-not-the-password');
  await page.click('#login-submit');
  await expect(page.locator('#login-error')).toBeVisible();
  await expect(page.locator('#login-error')).toContainText('Invalid username or password');
  await expect(page.locator('#main-app')).toBeHidden();
});

test('logout returns to the login page', async ({ page }) => {
  await loginAsAdmin(page);
  await page.locator('.user-profile-button').click();
  await page.locator('#user-profile-dropdown button', { hasText: 'Logout' }).click();
  await expect(page.locator('#login-screen')).toBeVisible();
  await expect(page.locator('#login-form')).toBeVisible();
});
