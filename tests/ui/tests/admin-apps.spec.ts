import { test } from '@fixtures';
import { expect } from '@playwright/test';
import { config } from '@config';
import { generateRandomString } from '@utils/utils';

const app_name = 'My app ' + generateRandomString(3);
const app_description = 'Long Description';

test('Apps on admin page', async ({ page, loginPage, adminMainPage, adminAppsPage }) => {
  await test.step('Crating new app', async () => {
    await loginPage.goto();
    await loginPage.login(config.admin_email, config.password);
    await adminMainPage.navigateToApps();
    await adminAppsPage.AddAppButton.click();
    await adminAppsPage.NameInput.fill(app_name);
    await adminAppsPage.DescriptionInput.fill(app_description);
    await adminAppsPage.UserDropDown.setValue('Test Admin');
    await adminAppsPage.MonthlyBudgetInput.fill('10');
    await adminAppsPage.LlmDropDown.setValue('Anthropic LLM');
    await adminAppsPage.AddAppButton.click();
    await adminAppsPage.Table.expectRowWithTextExists(app_name);
  });

  await test.step('Approve app', async () => {
    await adminAppsPage.Table.clickRowByText(app_name);
    await adminAppsPage.ApproveThisAppButton.click();
    await adminAppsPage.expectPopupAppApproved();
    await expect(page.getByText('Yes')).toBeVisible(); // checking if status changed to active
  });

  await test.step('Get key and secret', async () => {
    const keyID = await adminAppsPage.getKeyId();
    const secret = await adminAppsPage.getSecret();
    console.log('keyID:', keyID);
    console.log('secret:', secret);
    expect(keyID).not.toBeNull();
    expect(secret).not.toBeNull();
  });

  await test.step('Delete app', async () => {
    await adminMainPage.navigateToApps();
    await adminAppsPage.Table.deleteRowWithText(app_name);
    await adminAppsPage.expectPopupAppDeleted();
    await adminAppsPage.Table.expectRowWithTextNotExists(app_name);
  });
})
