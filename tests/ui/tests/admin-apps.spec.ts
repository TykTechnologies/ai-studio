import { test } from '@fixtures';
import { expect } from '@playwright/test';
import { config } from '@config';
import { generateRandomString } from '@utils/utils';

const app_name_prefix = 'My e2e app ';
const app_description = 'Long E2E Description';
const default_monthly_budget = '10';

// Define some tool names that are expected to be available in the test environment
const tool_alpha = 'Test Tool Alpha'; // Assume this tool exists
const tool_beta = 'Test Tool Beta';   // Assume this tool exists
const tool_gamma = 'Test Tool Gamma'; // Assume this tool exists


test('Apps on admin page - Full CRUD with Tools', async ({ page, loginPage, adminMainPage, adminAppsPage }) => {
  const unique_app_name = app_name_prefix + generateRandomString(3);

  await test.step('Login and Navigate to Apps', async () => {
    await loginPage.goto();
    await loginPage.login(config.admin_email, config.password);
    await adminMainPage.dismissQuickStartModal();
    await adminMainPage.navigateToApps();
  });

  await test.step('Creating new app with tools', async () => {
    await adminAppsPage.AddAppButton.click();
    await adminAppsPage.NameInput.fill(unique_app_name);
    await adminAppsPage.DescriptionInput.fill(app_description);
    await adminAppsPage.UserDropDown.setValue('Test Admin'); // Assuming 'Test Admin' user exists
    
    // Select LLMs (optional, but good to include if part of the form)
    await adminAppsPage.LlmDropDown.selectValue('Anthropic LLM'); // Assuming this LLM exists

    // Select Tools
    await adminAppsPage.ToolDropDown.selectValue(tool_alpha);
    await adminAppsPage.ToolDropDown.selectValue(tool_beta);

    // Now interact with MonthlyBudgetInput
    await adminAppsPage.page.waitForTimeout(1000); // Add a brief static pause
    await adminAppsPage.MonthlyBudgetInput.waitFor({ state: 'editable', timeout: 15000 });
    await adminAppsPage.MonthlyBudgetInput.fill(default_monthly_budget);
    
    await adminAppsPage.SaveButton.click(); // Changed from AddAppButton to SaveButton for consistency
    await adminAppsPage.expectPopupAppCreated();
    await adminAppsPage.Table.expectRowWithTextExists(unique_app_name);
  });

  await test.step('Verify tools on App Details page after creation', async () => {
    await adminAppsPage.Table.clickRowByText(unique_app_name);
    const displayedTools = await adminAppsPage.getDisplayedTools();
    expect(displayedTools).toContain(tool_alpha);
    expect(displayedTools).toContain(tool_beta);
    expect(displayedTools).not.toContain(tool_gamma);
  });

  await test.step('Approve app', async () => {
    // Already on app details page from previous step
    await adminAppsPage.ApproveThisAppButton.click();
    await adminAppsPage.expectPopupAppApproved();
    // Check for visual indication of approval (e.g., "Active: Yes")
    // This depends on how AppDetails shows status, assuming a text "Yes" for active credential
    await expect(page.getByText('Active:')).toBeVisible(); // Generic check, make more specific if possible
    await expect(page.locator('//div[label[text()="Active:"]]/following-sibling::div[text()="Yes"]')).toBeVisible();


  });

  await test.step('Get key and secret', async () => {
    // Still on app details page
    const keyID = await adminAppsPage.getKeyId();
    const secret = await adminAppsPage.getSecret();
    console.log(`App [${unique_app_name}] KeyID: ${keyID}`);
    console.log(`App [${unique_app_name}] Secret: ${secret}`);
    expect(keyID).not.toBeNull();
    expect(keyID?.length).toBeGreaterThan(5); // Basic sanity check
    expect(secret).toEqual('********************************'); // Secret is masked in UI
  });

  await test.step('Edit app and manage tools', async () => {
    // Still on app details page, navigate to edit
    await adminAppsPage.page.getByRole('button', { name: 'Edit app' }).click();
    
    // Update description
    const updated_description = app_description + " - Updated";
    await adminAppsPage.DescriptionInput.fill(updated_description);

    // Manage tools: remove tool_beta, add tool_gamma
    // How CustomSelectMany handles removal needs to be known.
    // If it's by clicking the chip's delete icon:
    await adminAppsPage.ToolDropDown.removeValue(tool_beta); // Assuming removeValue deselects/clicks remove icon
    await adminAppsPage.ToolDropDown.selectValue(tool_gamma);
    
    await adminAppsPage.SaveButton.click(); // Name of button might be "Update app" now
    await adminAppsPage.expectPopupAppUpdated();

    // Navigate back to app details to verify
    await adminMainPage.navigateToApps(); // Go to list
    await adminAppsPage.Table.clickRowByText(unique_app_name); // Click the app to go to its details

    const displayedToolsAfterUpdate = await adminAppsPage.getDisplayedTools();
    expect(displayedToolsAfterUpdate).toContain(tool_alpha);
    expect(displayedToolsAfterUpdate).not.toContain(tool_beta);
    expect(displayedToolsAfterUpdate).toContain(tool_gamma);
    
    // Verify description updated
    await expect(page.getByText(updated_description)).toBeVisible();
  });


  await test.step('Delete app', async () => {
    await adminMainPage.navigateToApps();
    await adminAppsPage.Table.deleteRowWithText(unique_app_name);
    await adminAppsPage.expectPopupAppDeleted();
    await adminAppsPage.Table.expectRowWithTextNotExists(unique_app_name);
  });
});

test('App form - Tool selection validation (optional)', async ({ page, loginPage, adminMainPage, adminAppsPage }) => {
    // This test is optional and depends on whether "Tools" is a required field
    // or if there are any specific validation rules for it.
    // For now, assuming it's not mandatory.

    await loginPage.goto();
    await loginPage.login(config.admin_email, config.password);
    await adminMainPage.dismissQuickStartModal();
    await adminMainPage.navigateToApps();

    await adminAppsPage.AddAppButton.click();
    await adminAppsPage.NameInput.fill("App For Tool Validation Test");
    await adminAppsPage.UserDropDown.setValue('Test Admin');
    // Not selecting any tools
    await adminAppsPage.SaveButton.click();
    
    // Check if there's a specific error for tools, or if it saves successfully
    // For this example, we'll assume it saves successfully without tools.
    await adminAppsPage.expectPopupAppCreated();
    await adminAppsPage.Table.expectRowWithTextExists("App For Tool Validation Test");

    // Clean up
    await adminAppsPage.Table.deleteRowWithText("App For Tool Validation Test");
    await adminAppsPage.expectPopupAppDeleted();
});
