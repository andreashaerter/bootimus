import { expect, loginAsAdmin, openTab, test } from './helpers';

const tabs: Array<[string, string]> = [
  ['server', 'Server Information'],
  ['images', 'Image Management'],
  ['disk-images', 'Disk Images'],
  ['public-files', 'Public Files'],
  ['autoinstall', 'Auto-Install Files'],
  ['profiles', 'Distro Profiles'],
  ['boot-menu', 'Boot Menu'],
  ['bootloaders', 'Bootloader Management'],
  ['tools', 'Boot Tools'],
  ['clients', 'Client Management'],
  ['clusters', 'Clusters'],
  ['users', 'User Management'],
  ['audit', 'Audit Log'],
  ['settings', 'Webhook'],
  ['api-reference', 'API Reference'],
  ['pxe-debug', 'PXE Debug'],
  ['logs', 'Server Logs'],
];

test('every sidebar tab renders without browser errors', async ({ page, errors }) => {
  await loginAsAdmin(page);
  for (const [tab, heading] of tabs) {
    await openTab(page, tab);
    await expect(page.locator(`#${tab}-tab h2`, { hasText: heading }).first()).toBeVisible();
    await page.waitForLoadState('networkidle');
  }
  expect(errors.consoleErrors, 'console errors while opening tabs').toEqual([]);
});
