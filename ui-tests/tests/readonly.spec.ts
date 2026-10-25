import { credentials, expect, loginAsViewer, openTab, test } from './helpers';

test('a read-only user sees clients but no admin controls', async ({ page }) => {
  await loginAsViewer(page);
  await expect(page.locator('#current-username')).toHaveText(credentials.viewerUser);
  await expect(page.locator('body')).toHaveClass(/read-only/);
  await openTab(page, 'clients');
  await expect(page.locator('#clients-table')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Add Client' })).toBeHidden();
  await expect(page.locator('.sidebar-nav .nav-item[data-tab="users"]')).toBeHidden();
  await expect(page.locator('.sidebar-nav .nav-item[data-tab="audit"]')).toBeHidden();
  await expect(page.locator('.sidebar-nav .nav-item[data-tab="settings"]')).toBeHidden();
});
