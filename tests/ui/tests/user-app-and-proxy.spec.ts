import { test } from '@fixtures';
import { expect } from '@playwright/test';
import { config } from '../config';
import { generateRandomString } from '@utils/utils';

test('Apps on AI Portal page', async ({ page, loginPage, aiPortalPage, adminAppsPage, adminMainPage }) => {
  const app_name = `My user app ${generateRandomString(3)}`;
  const app_description = 'Long Description';
  let keyID: string;
  let restUrl: string;

  await test.step('Crating new app', async () => {
    await loginPage.goto();
    await loginPage.login(config.dev_user_email, config.password);
    await aiPortalPage.CreateANewAppButton.click();
    await aiPortalPage.NameInput.fill(app_name);
    await aiPortalPage.DescriptionInput.fill(app_description);
    await aiPortalPage.LlmDropDown.setValue('Anthropic');
    await aiPortalPage.AddLlmButton.click();
    await aiPortalPage.CreateappButton.click();
    await aiPortalPage.ViewYourAppsButton.click();
    await aiPortalPage.Table.expectRowWithTextExists(app_name);
  });

  await test.step('Approve app', async () => {
    await aiPortalPage.logOut();
    await loginPage.login(config.admin_email, config.password);
    await adminMainPage.navigateToApps();
    await adminAppsPage.Table.clickRowByText(app_name);
    await adminAppsPage.ApproveThisAppButton.click();
    await adminAppsPage.expectPopupAppApproved();
    await expect(page.getByText('Yes')).toBeVisible(); // checking if status changed to active
  });

  await test.step('Get key and secret', async () => {
    await adminAppsPage.logOut();
    await loginPage.login(config.dev_user_email, config.password);
    await aiPortalPage.AppsMenuButton.click();
    await aiPortalPage.Table.clickRowByText(app_name);
    keyID = await aiPortalPage.getKeyId();
    restUrl = await aiPortalPage.getRestUrl();
    console.log('keyID:', keyID);
    console.log('Proxy URL:', restUrl);
    expect(keyID).not.toBeNull();
    expect(restUrl).not.toBeNull();
  });

  await test.step('Delete app', async () => {
    await aiPortalPage.DeleteAppButton.click();
    await aiPortalPage.ConfirmDeleteButton.click();
    await aiPortalPage.Table.expectRowWithTextNotExists(app_name);
  });
})