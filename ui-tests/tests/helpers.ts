import { expect, test as base, type Page } from '@playwright/test';

export const credentials = {
  adminUser: process.env.BOOTIMUS_UI_ADMIN_USER || 'admin',
  adminPassword: process.env.BOOTIMUS_UI_ADMIN_PASSWORD || '',
  viewerUser: process.env.BOOTIMUS_UI_VIEWER_USER || 'viewer',
  viewerPassword: process.env.BOOTIMUS_UI_VIEWER_PASSWORD || '',
};

export type BrowserErrors = { pageErrors: string[]; consoleErrors: string[] };

export function collectErrors(page: Page): BrowserErrors {
  const errors: BrowserErrors = { pageErrors: [], consoleErrors: [] };
  page.on('pageerror', (err) => errors.pageErrors.push(err.message));
  page.on('console', (msg) => {
    if (msg.type() === 'error') errors.consoleErrors.push(msg.text());
  });
  return errors;
}

export const test = base.extend<{ errors: BrowserErrors }>({
  errors: [
    async ({ page }, use) => {
      const errors = collectErrors(page);
      page.on('dialog', (dialog) => dialog.accept());
      await use(errors);
      expect(errors.pageErrors, 'uncaught exceptions in the page').toEqual([]);
    },
    { auto: true },
  ],
});

export { expect };

export async function login(page: Page, username: string, password: string) {
  await page.goto('/');
  await expect(page.locator('#login-form')).toBeVisible();
  await page.fill('#login-username', username);
  await page.fill('#login-password', password);
  await page.click('#login-submit');
  await expect(page.locator('#main-app')).toBeVisible();
  await expect(page.locator('#login-screen')).toBeHidden();
}

export async function loginAsAdmin(page: Page) {
  await login(page, credentials.adminUser, credentials.adminPassword);
}

export async function loginAsViewer(page: Page) {
  await login(page, credentials.viewerUser, credentials.viewerPassword);
}

export async function openTab(page: Page, tab: string) {
  await page.locator(`.sidebar-nav .nav-item[data-tab="${tab}"]`).click();
  await expect(page.locator(`#${tab}-tab`)).toBeVisible();
}

export async function openClientsSubtab(page: Page, subtab: string) {
  await openTab(page, 'clients');
  await page.locator(`#clients-tab .subtab[data-subtab="${subtab}"]`).click();
  await expect(page.locator(`#${subtab}-subtab`)).toBeVisible();
}

export function uniqueSuffix() {
  return Date.now().toString(36) + Math.floor(Math.random() * 1000).toString(36);
}

export function randomMAC() {
  const octet = () => Math.floor(Math.random() * 256).toString(16).padStart(2, '0');
  return `02:${octet()}:${octet()}:${octet()}:${octet()}:${octet()}`;
}

export async function createClientViaUI(page: Page, mac: string, name: string) {
  await openTab(page, 'clients');
  await page.getByRole('button', { name: 'Add Client' }).click();
  await expect(page.locator('#add-client-modal')).toBeVisible();
  await page.fill('#add-client-form input[name="mac_address"]', mac);
  await page.fill('#add-client-form input[name="name"]', name);
  await page.locator('#add-client-form button[type="submit"]').click();
  await expect(page.locator('#add-client-modal')).toBeHidden();
  await expect(page.locator('#clients-table')).toContainText(mac);
}
