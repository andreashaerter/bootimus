import { expect, loginAsAdmin, openTab, test, uniqueSuffix } from './helpers';

test('create and revoke an API token', async ({ page }) => {
  await loginAsAdmin(page);
  await openTab(page, 'users');
  const name = `ui-token-${uniqueSuffix()}`;
  await page.fill('#api-token-name', name);
  await page.locator('#api-token-form button[type="submit"]').click();
  await expect(page.locator('#api-token-secret')).toBeVisible();
  await expect(page.locator('#api-token-secret-value')).toHaveText(/^btk_[A-Za-z0-9]{40}$/);
  const row = page.locator('#api-tokens-table tr', { hasText: name });
  await expect(row).toContainText('Active');
  await row.getByRole('button', { name: 'Revoke' }).click();
  await expect(page.locator('#api-tokens-table tr', { hasText: name })).toContainText('Revoked');
});

test('two-factor setup shows a QR code', async ({ page }) => {
  await loginAsAdmin(page);
  await openTab(page, 'users');
  await expect(page.locator('#totp-card')).toBeVisible();
  await page.locator('#totp-status button', { hasText: 'Set up' }).click();
  await expect(page.locator('#totp-setup')).toBeVisible();
  await expect(page.locator('#totp-qr')).toHaveAttribute('src', /^data:image\/png;base64,/);
  await expect(page.locator('#totp-secret')).not.toBeEmpty();
});
