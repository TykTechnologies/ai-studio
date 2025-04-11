import { Locator, Page } from '@playwright/test';
import { DropDownWrapper } from '@wrappers/DropDownWrapper';
import { TableWrapper } from '@wrappers/TableWrapper';

interface ProviderParams {
    name: string;
    provider: string;
    shortDescription?: string;
    longDescription?: string;
    defaultModel?: string;
    monthlyBudget?: string;
    budgetStartDate?: string;
    privacyScore?: string;
    modelPattern?: string;
    apiEndpoint?: string;
    apiKey?: string;
    logoUrl?: string;
}

export class AdminLLMProvidersPage {
    readonly page: Page;
    readonly Table: TableWrapper
    readonly AddLLMButton: Locator;
    readonly ProviderNameInput: Locator;
    readonly ShortDescriptionInput: Locator;
    readonly LongDescriptionInput: Locator;
    readonly ProviderTypeDropDown: DropDownWrapper;
    readonly DefaultModelInput: Locator;
    readonly MonthlyBudgetInput: Locator;
    readonly BudgetStartDateInput: Locator;
    readonly PrivacyScoreInput: Locator;
    readonly ModelPatternInput: Locator;
    readonly AccessDetailsButton: Locator;
    readonly ApiEndpointInput: Locator;
    readonly ApiKeyInput: Locator;
    readonly PortalDisplayInformationButton: Locator;
    readonly LogoUrlInput: Locator;
    readonly EnabledInProxyCheckbox: Locator;
    readonly FiltersButton: Locator;
    readonly SaveButton: Locator;
    readonly CancelButton: Locator;
    readonly EditProviderButton: Locator;
    readonly DeactivateProviderButton: Locator;
    readonly BackToLLMsLink: Locator;

    constructor(page: Page) {
        this.page = page;
        this.Table = new TableWrapper('table', this.page);
        this.AddLLMButton = this.page.getByText('Add LLM').first();
        this.ProviderNameInput = this.page.getByRole('textbox', { name: 'Name' });
        this.ShortDescriptionInput = this.page.getByRole('textbox', { name: 'Short Description' });
        this.LongDescriptionInput = this.page.getByRole('textbox', { name: 'Long Description' });
        this.ProviderTypeDropDown = new DropDownWrapper('#mui-component-select-vendor', this.page);
        this.DefaultModelInput = this.page.getByRole('textbox', { name: 'Default Model' });
        this.MonthlyBudgetInput = this.page.getByRole('spinbutton', { name: 'Monthly Budget' });
        this.BudgetStartDateInput = this.page.getByRole('textbox', { name: 'Budget Start Date' });
        this.PrivacyScoreInput = this.page.locator('input[name="privacy_score"]');
        this.ModelPatternInput = this.page.getByRole('textbox', { name: 'Model Pattern' });
        this.AccessDetailsButton = this.page.getByRole('button', { name: 'Access Details' });
        this.ApiEndpointInput = this.page.getByRole('textbox', { name: 'API Endpoint' });
        this.ApiKeyInput = this.page.getByRole('textbox', { name: 'API Key' });
        this.PortalDisplayInformationButton = this.page.getByRole('button', { name: 'Portal Display Information' });
        this.LogoUrlInput = this.page.getByRole('textbox', { name: 'Logo URL' });
        this.EnabledInProxyCheckbox = this.page.getByRole('checkbox', { name: 'Enabled in Proxy' });
        this.FiltersButton = this.page.getByRole('button', { name: 'Filters' });
        this.SaveButton = this.page.getByRole('button', { name: 'Add LLM' });
        this.CancelButton = this.page.getByRole('button', { name: 'Cancel' });
        this.EditProviderButton = this.page.getByRole('menuitem', { name: 'Edit LLM' });
        this.DeactivateProviderButton = this.page.getByRole('menuitem', { name: 'Deactivate LLM' });
        this.BackToLLMsLink = this.page.getByRole('link', { name: 'Back to LLMs' });
    }

    async goto() {
        await this.page.goto('/admin/llm-providers');
    }

    async addProvider(params: ProviderParams) {
        await this.AddLLMButton.click();
        await this.ProviderNameInput.fill(params.name);
        await this.ProviderTypeDropDown.click();
        await this.page.getByText(params.provider, { exact: true }).click();

        if (params.shortDescription) {
            await this.ShortDescriptionInput.fill(params.shortDescription);
        }
        if (params.longDescription) {
            await this.LongDescriptionInput.fill(params.longDescription);
        }
        if (params.defaultModel) {
            await this.DefaultModelInput.fill(params.defaultModel);
        }
        if (params.monthlyBudget) {
            await this.MonthlyBudgetInput.fill(params.monthlyBudget);
        }
        if (params.budgetStartDate) {
            await this.BudgetStartDateInput.fill(params.budgetStartDate);
        }
        if (params.privacyScore) {
            await this.PrivacyScoreInput.fill(params.privacyScore);
        }
        if (params.modelPattern) {
            await this.ModelPatternInput.fill(params.modelPattern);
        }
        if (params.apiEndpoint) {
            await this.AccessDetailsButton.click();
            await this.ApiEndpointInput.fill(params.apiEndpoint);
        }
        if (params.apiKey) {
            await this.ApiKeyInput.fill(params.apiKey);
        }
        if (params.logoUrl) {
            await this.PortalDisplayInformationButton.click();
            await this.LogoUrlInput.fill(params.logoUrl);
        }
        await this.SaveButton.click();
    }
}
