import { createClientViaUI, expect, loginAsAdmin, randomMAC, test, uniqueSuffix } from './helpers';

test('create a client, open its detail page and preview its boot menu', async ({ page }) => {
  await loginAsAdmin(page);
  const mac = randomMAC();
  const name = `ui-client-${uniqueSuffix()}`;
  await createClientViaUI(page, mac, name);

  await page.locator('#clients-table tr.row-clickable', { hasText: mac }).first().click();
  await expect(page.locator('#client-detail-tab')).toBeVisible();
  await expect.poll(() => decodeURIComponent(page.url())).toContain(`#clients/${mac}`);
  await expect(page.locator('#client-detail-header')).toContainText(name);
  await expect(page.locator('#client-detail-header')).toContainText(mac);
  await expect(page.locator('#client-detail-overview')).not.toBeEmpty();
  await expect(page.locator('#client-detail-provisioning')).not.toBeEmpty();
  await expect(page.locator('#client-detail-timeline')).not.toBeEmpty();

  await page.locator('#client-detail-actions button', { hasText: 'Preview Menu' }).click();
  await expect(page.locator('#menu-preview-modal')).toBeVisible();
  await expect(page.locator('#menu-preview-script')).toContainText('#!ipxe');
  await expect(page.locator('#menu-preview-summary')).toContainText(mac);
});
