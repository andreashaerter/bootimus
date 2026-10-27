import { createClientViaUI, expect, loginAsAdmin, openTab, randomMAC, test, uniqueSuffix } from './helpers';

test('housekeeping saves a retention value and runs on demand', async ({ page }) => {
  await loginAsAdmin(page);
  await openTab(page, 'settings');
  await page.fill('#housekeeping-boot-log-days', '45');
  await page.locator('#housekeeping-form button[type="submit"]').click();
  await expect(page.locator('.notification', { hasText: /saved/i })).toBeVisible();
  await page.locator('#housekeeping-run-btn').click();
  await expect(page.locator('#housekeeping-report')).toContainText(/Boot logs|boot_logs/);
  await expect(page.locator('#housekeeping-last-run')).not.toBeEmpty();
  await page.reload();
  await openTab(page, 'settings');
  await expect(page.locator('#housekeeping-boot-log-days')).toHaveValue('45');
});

test('backup export downloads an archive', async ({ page }) => {
  await loginAsAdmin(page);
  await openTab(page, 'settings');
  const downloadPromise = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Download Backup' }).click();
  const download = await downloadPromise;
  expect(download.suggestedFilename()).toMatch(/^bootimus-backup-.*\.tar\.gz$/);
});

test('PXE debug explains that proxyDHCP is off', async ({ page }) => {
  await loginAsAdmin(page);
  await openTab(page, 'pxe-debug');
  await expect(page.locator('#pxe-debug-disabled')).toBeVisible();
  await expect(page.locator('#pxe-debug-disabled')).toContainText(/proxyDHCP/);
});

test('SMB guidance describes server startup configuration in every language', async ({ page }) => {
  await loginAsAdmin(page);
  const languages = await page.locator('#lang-select option').evaluateAll(options =>
    options.map(option => (option as HTMLOptionElement).value));
  expect(languages.length).toBeGreaterThan(1);
  for (const language of languages) {
    await page.selectOption('#lang-select', language);
    const warning = page.locator('[data-i18n="props.warn.smb_disabled"]');
    await expect(warning).toContainText('--windows-smb');
    await expect(warning).toContainText('BOOTIMUS_WINDOWS_SMB_ENABLED=true');
  }
});

test('audit log lists admin actions', async ({ page }) => {
  await loginAsAdmin(page);
  const mac = randomMAC();
  await createClientViaUI(page, mac, `ui-audit-${uniqueSuffix()}`);
  await openTab(page, 'audit');
  await expect(page.locator('#audit-table')).toContainText('/api/clients');
  await expect(page.locator('#audit-table')).toContainText('POST');
  await expect(page.locator('#audit-status')).toContainText(/Showing \d+ of \d+/);
});

test('running status reports the Windows SMB state', async ({ page }) => {
  await loginAsAdmin(page);
  await openTab(page, 'server');
  const row = page.locator('.rs-metric', { hasText: 'Windows SMB' });
  await expect(row).toBeVisible();
  await expect(row.locator('.badge')).toHaveText(/Enabled|Disabled|Requested/);
});
