import { createClientViaUI, expect, loginAsAdmin, openClientsSubtab, randomMAC, test, uniqueSuffix } from './helpers';

test('create a deployment profile', async ({ page }) => {
  await loginAsAdmin(page);
  await openClientsSubtab(page, 'deployment-profiles');
  const name = `ui-profile-${uniqueSuffix()}`;
  await page.getByRole('button', { name: 'Add Deployment Profile' }).click();
  await expect(page.locator('#deployment-profile-modal')).toBeVisible();
  await page.fill('#deployment-profile-form input[name="name"]', name);
  await page.fill('#deployment-profile-form input[name="description"]', 'Created by the UI smoke test');
  await page.locator('#deployment-profile-form button[type="submit"]').click();
  await expect(page.locator('#deployment-profile-modal')).toBeHidden();
  await expect(page.locator('#deployment-profiles-table')).toContainText(name);
});

test('create an enrolment rule and see the live match count', async ({ page }) => {
  await loginAsAdmin(page);
  const mac = randomMAC();
  await createClientViaUI(page, mac, `ui-rule-target-${uniqueSuffix()}`);
  const prefix = mac.slice(0, 8);

  await openClientsSubtab(page, 'enrolment-rules');
  await page.getByRole('button', { name: 'Add Rule' }).click();
  await expect(page.locator('#enrolment-rule-modal')).toBeVisible();
  const name = `ui-rule-${uniqueSuffix()}`;
  await page.fill('#enrolment-rule-form input[name="name"]', name);
  await page.fill('#enrolment-rule-form input[name="mac_prefix"]', prefix);
  await page.locator('#enrolment-rule-form input[name="mac_prefix"]').dispatchEvent('input');
  await expect(page.locator('#enrolment-rule-preview')).toContainText(/Matches \d+ existing client/);
  await page.check('#er-make-static');
  await page.fill('#enrolment-rule-form input[name="name_pattern"]', 'lab-{{HOSTNAME_SAFE}}');
  await page.locator('#enrolment-rule-form button[type="submit"]').click();
  await expect(page.locator('#enrolment-rule-modal')).toBeHidden();
  await expect(page.locator('#enrolment-rules-table')).toContainText(name);
});

test('create a SAN target', async ({ page }) => {
  await loginAsAdmin(page);
  await openClientsSubtab(page, 'san-targets');
  const name = `ui-san-${uniqueSuffix()}`;
  await page.getByRole('button', { name: 'Add SAN Target' }).click();
  await expect(page.locator('#san-target-modal')).toBeVisible();
  await page.fill('#san-target-form input[name="name"]', name);
  await page.fill('#san-target-form input[name="uri"]', 'iscsi:10.0.0.5::::iqn.2024-01.lan:disk0');
  await page.locator('#san-target-form button[type="submit"]').click();
  await expect(page.locator('#san-target-modal')).toBeHidden();
  await expect(page.locator('#san-targets-table')).toContainText(name);
});
