import { test } from '@fixtures';
import { expect } from '@playwright/test';
import { config } from '@config';

test('Registering admin user', async ({ page, loginPage, registerPage }) => {
  await loginPage.goto();
  await loginPage.RegisterHereButton.click();
  await registerPage.register(config.admin_name, config.admin_email, config.password);
  await page.waitForURL(config.base_url + '/login');
  await loginPage.login(config.admin_email, config.password);

  await expect(page).toHaveTitle(/Tyk AI Portal/);
});