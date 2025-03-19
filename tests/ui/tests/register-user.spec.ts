import { test } from '@fixtures';
import { expect } from '@playwright/test';
import { config } from '@config';
import { generateRandomEmail } from '@utils';

test('Registering new user', async ({ page, loginPage, registerPage }) => {
  const randomEmail = generateRandomEmail();
  await loginPage.goto();
  await loginPage.registerHereButton.click();
  await registerPage.register(config.user_name, randomEmail, config.user_password);
  await page.waitForURL(config.base_url + '/login');
  await loginPage.login(randomEmail, config.user_password);
  await expect(page.getByText("Email unverified, please verify email or contact your administrator")).toBeVisible();
});