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

test('Edit LLM provider name shows warning and confirmation dialog', async ({ loginPage, adminLLMProvidersPage, adminMainPage }) => {
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
    await adminLLMProvidersPage.SaveButton.click();
    await adminLLMProvidersPage.Table.expectRowWithTextExists(originalName);

    // Edit the provider
    const rowNumber = await adminLLMProvidersPage.Table.getRowNumberWithText(originalName);
    await adminLLMProvidersPage.Table.triggerEditAction(rowNumber);

    // Change the name and verify warning appears
    await adminLLMProvidersPage.ProviderNameInput.fill(newName);
    
    // Verify warning message appears as helper text below the name field
    const warningMessage = adminLLMProvidersPage.page.locator('.MuiFormHelperText-root');
    await expect(warningMessage).toBeVisible();
    await expect(warningMessage).toContainText('API endpoints will change');
    
    // Verify old and new endpoints are shown in the warning
    await expect(warningMessage).toContainText(`/llm/${originalName.toLowerCase().replace(/\s+/g, '-')}`);
    await expect(warningMessage).toContainText(`/llm/${newName.toLowerCase().replace(/\s+/g, '-')}`);

    // Try to save and verify confirmation dialog appears
    await adminLLMProvidersPage.SaveButton.click();
    
    // Verify confirmation dialog
    const confirmDialog = adminLLMProvidersPage.page.locator('.MuiDialog-root');
    await expect(confirmDialog).toBeVisible();
    await expect(confirmDialog.locator('.MuiDialogTitle-root')).toContainText('Confirm Name Change');
    
    // Verify dialog shows old and new endpoints
    await expect(confirmDialog).toContainText(`/llm/${originalName.toLowerCase().replace(/\s+/g, '-')}`);
    await expect(confirmDialog).toContainText(`/llm/${newName.toLowerCase().replace(/\s+/g, '-')}`);
    
    // Confirm the change
    await adminLLMProvidersPage.page.locator('button:has-text("Confirm")').click();
    
    // Verify the name was changed successfully
    await adminLLMProvidersPage.Table.expectRowWithTextExists(newName);
    await adminLLMProvidersPage.Table.expectRowWithTextNotExists(originalName);
    
    // Clean up - delete the provider
    await adminLLMProvidersPage.Table.deleteRowWithText(newName);
    await adminLLMProvidersPage.Table.expectRowWithTextNotExists(newName);
});



