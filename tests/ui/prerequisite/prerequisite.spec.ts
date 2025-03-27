import { test } from '@fixtures';
import { expect } from '@playwright/test';
import { config } from '../config';

test('Registering admin user', async ({ page, loginPage, registerPage, adminLLMProvidersPage, adminMainPage }) => {
  await loginPage.goto();
  await loginPage.RegisterHereButton.click();
  await registerPage.register(config.admin_name, config.admin_email, config.admin_password);
  await page.waitForURL(config.base_url + '/login');
  await loginPage.login(config.admin_email, config.admin_password);

  await expect(page).toHaveTitle(/Tyk AI Portal/);
});

test('Add LLM provider', async ({ page, loginPage, adminLLMProvidersPage, adminMainPage }) => {
  await loginPage.goto();
  await loginPage.login(config.admin_email, config.admin_password);
  await adminMainPage.navigateToLLMProviders();
  await adminLLMProvidersPage.AddLLMButton.click();
  await adminLLMProvidersPage.ProviderNameInput.fill('Anthropic LLM');
  await adminLLMProvidersPage.ProviderTypeDropDown.setValue('Anthropic');
  await adminLLMProvidersPage.SaveButton.click();
  await adminLLMProvidersPage.Table.expectRowWithTextExists('Anthropic LLM');
  await adminLLMProvidersPage.Table.triggerActivateAction(1);
});

test('Add user', async ({ page, loginPage, adminMainPage, adminUsersPage }) => {
  await loginPage.goto();
  await loginPage.login(config.admin_email, config.admin_password);
  await adminMainPage.navigateToUsers();
  await adminUsersPage.AddUserButton.click();
  await adminUsersPage.NameInput.fill('Test User');
  await adminUsersPage.EmailInput.fill('dev@tyk.io');
  await adminUsersPage.PasswordInput.fill(config.admin_password);
  await adminUsersPage.EmailVerifiedCheckbox.check();
  await adminUsersPage.SaveUserButton.click();
  await adminUsersPage.Table.expectRowWithTextExists('Test User');
});


