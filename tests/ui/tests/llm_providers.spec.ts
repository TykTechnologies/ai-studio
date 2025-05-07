import { test } from '@fixtures';
import { expect } from '@playwright/test';
import { config } from '../config';

test('Add LLM provider', async ({ loginPage, adminLLMProvidersPage, adminMainPage }) => {
    const LLMProviderName = 'Anthropic Test';

    await loginPage.goto();
    await loginPage.login(config.admin_email, config.password);

    await adminMainPage.dismissQuickStartModal();
    await adminMainPage.navigateToLLMProviders();
    await adminLLMProvidersPage.AddLLMButton.click();
    await adminLLMProvidersPage.ProviderNameInput.fill(LLMProviderName);
    await adminLLMProvidersPage.ProviderTypeDropDown.setValue('Anthropic');
    await adminLLMProvidersPage.SaveButton.click();
    await adminLLMProvidersPage.Table.expectRowWithTextExists(LLMProviderName);
    const rowNumber = await adminLLMProvidersPage.Table.getRowNumberWithText(LLMProviderName);
    await adminLLMProvidersPage.Table.triggerActivateAction(rowNumber);

    await adminLLMProvidersPage.Table.deleteRowWithText(LLMProviderName);
    await adminLLMProvidersPage.Table.expectRowWithTextNotExists(LLMProviderName);
});

test('Edit LLM provider name shows confirmation dialog', async ({ loginPage, adminLLMProvidersPage, adminMainPage }) => {
    const originalName = 'Original Provider';
    const newName = 'Renamed Provider';

    // Login and navigate to LLM providers page
    await loginPage.goto();
    await loginPage.login(config.admin_email, config.password);
    await adminMainPage.dismissQuickStartModal();
    await adminMainPage.navigateToLLMProviders();

    // Create initial provider
    await adminLLMProvidersPage.AddLLMButton.click();
    await adminLLMProvidersPage.ProviderNameInput.fill(originalName);
    await adminLLMProvidersPage.ProviderTypeDropDown.setValue('OpenAI');
    
    // Use a more direct approach to find and click the save button
    const saveButton = adminLLMProvidersPage.page.locator('button[type="submit"]');
    console.log('Clicking save button to create provider');
    await saveButton.click();
    
    await adminLLMProvidersPage.Table.expectRowWithTextExists(originalName);

    // Edit the provider
    const rowNumber = await adminLLMProvidersPage.Table.getRowNumberWithText(originalName);
    await adminLLMProvidersPage.Table.triggerEditAction(rowNumber);

    // Change the name
    await adminLLMProvidersPage.ProviderNameInput.fill(newName);
    console.log(`Changed provider name from "${originalName}" to "${newName}"`);
    
    // Calculate endpoints for verification
    const oldEndpoint = `/llm/${originalName.toLowerCase().replace(/\s+/g, '-')}`;
    const newEndpoint = `/llm/${newName.toLowerCase().replace(/\s+/g, '-')}`;
    
    // Try to save and verify confirmation dialog appears
    console.log('Clicking save button to trigger confirmation dialog');
    await saveButton.waitFor({ state: 'visible' });
    await saveButton.click();
    
    // Verify confirmation dialog
    const confirmDialog = adminLLMProvidersPage.page.locator('.MuiDialog-root');
    await expect(confirmDialog).toBeVisible({ timeout: 5000 });
    console.log('Confirmation dialog is visible');
    
    // Verify dialog title
    const dialogTitle = confirmDialog.locator('.MuiDialogTitle-root');
    await expect(dialogTitle).toContainText('Confirm Name Change');
    
    // Verify dialog mentions original and new names
    const dialogContent = confirmDialog.locator('.MuiDialogContent-root');
    await expect(dialogContent).toContainText(originalName);
    await expect(dialogContent).toContainText(newName);
    
    // Verify dialog mentions API endpoints
    await expect(dialogContent).toContainText(oldEndpoint);
    await expect(dialogContent).toContainText(newEndpoint);
    
    // Cancel the dialog
    const cancelButton = confirmDialog.locator('button:has-text("Cancel")');
    await cancelButton.waitFor({ state: 'visible' });
    await cancelButton.click();
    
    // Verify we're still on the edit page
    await expect(adminLLMProvidersPage.ProviderNameInput).toBeVisible();
    await expect(adminLLMProvidersPage.ProviderNameInput).toHaveValue(newName);
    
    // Clean up - cancel edit and delete the provider
    const cancelFormButton = adminLLMProvidersPage.page.locator('button:has-text("Cancel")');
    await cancelFormButton.click();
    await adminLLMProvidersPage.Table.deleteRowWithText(originalName);
    await adminLLMProvidersPage.Table.expectRowWithTextNotExists(originalName);
});



