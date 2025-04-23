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




