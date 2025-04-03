import { test } from '@fixtures';
import { expect } from '@playwright/test';
import { config } from '../config';
import { generateRandomEmail } from '@utils/utils';

test('Register and manage users', async ({ page, loginPage, registerPage, adminMainPage, adminUsersPage }) => {
  const userRandomEmail = generateRandomEmail();
  const adminRandomEmail = generateRandomEmail();

  await test.step('Register a new user', async () => {
    await loginPage.goto();
    await loginPage.RegisterHereButton.click();
    await registerPage.register("Registered User", userRandomEmail, config.password);
    await page.waitForURL(config.base_url + '/login');
    await loginPage.login(userRandomEmail, config.password);
    await expect(page.getByText("Email unverified, please verify email or contact your administrator")).toBeVisible();
  });

  await test.step('Add a new admin user', async () => {
    await loginPage.goto();
    await loginPage.login(config.admin_email, config.password);
    await adminMainPage.navigateToUsers();
    await adminUsersPage.AddUserButton.click();
    await adminUsersPage.NameInput.fill('Admin User');
    await adminUsersPage.EmailInput.fill(adminRandomEmail);
    await adminUsersPage.PasswordInput.fill(config.password);
    await adminUsersPage.EmailVerifiedCheckbox.check();
    await adminUsersPage.SaveUserButton.click();
    await adminUsersPage.Table.expectRowWithTextExists(adminRandomEmail);
  });

  await test.step('Delete the created users', async () => {
    await adminMainPage.navigateToUsers();
    await adminUsersPage.Table.deleteRowWithText(adminRandomEmail);
    await adminUsersPage.Table.expectRowWithTextNotExists(adminRandomEmail);
    await adminUsersPage.Table.deleteRowWithText(userRandomEmail);
    await adminUsersPage.Table.expectRowWithTextNotExists(userRandomEmail);
  });
});