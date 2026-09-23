import { Locator, Page } from '@playwright/test';
import { DropDownWrapper } from '@wrappers/DropDownWrapper';
import { TableWrapper } from '@wrappers/TableWrapper';
import { PageTemplate } from './Page_template';

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

export class AdminLLMProvidersPage extends PageTemplate {
    readonly Table: TableWrapper
    readonly AddLLMButton: Locator;
    readonly ProviderNameInput: Locator;
    readonly ShortDescriptionInput: Locator;
    readonly LongDescriptionInput: Locator;
    readonly ProviderTypeDropDown: DropDownWrapper;
    readonly DefaultModelInput: Locator;
    readonly MonthlyBudgetMode: Locator;
    readonly MonthlyBudgetInput: Locator;
    readonly BudgetStartDateInput: Locator;
    /** The 0–100 number half of the privacy control (name="privacy_score"). */
    readonly PrivacyScoreInput: Locator;
    /** The named-level half of the privacy control (Public / Internal / Confidential / Restricted). */
    readonly PrivacyLevelSelect: Locator;
    readonly ModelPatternInput: Locator;
    readonly AccessDetailsButton: Locator;
    readonly ApiEndpointInput: Locator;
    readonly ApiKeyInput: Locator;
    readonly PortalDisplayInformationButton: Locator;
    readonly LogoUrlInput: Locator;
    /** The form's "Active" switch. */
    readonly ActiveCheckbox: Locator;
    /** The list's "Active" column header. */
    readonly ActiveColumnHeader: Locator;
    readonly FiltersButton: Locator;
    readonly SaveButton: Locator;
    readonly CancelButton: Locator;
    readonly EditProviderButton: Locator;
    readonly DeactivateProviderButton: Locator;
    readonly BackToLLMsLink: Locator;

    constructor(page: Page) {
        super(page);
        this.Table = new TableWrapper('table', this.page);
        this.AddLLMButton = this.page.getByText('Add LLM provider').first();
        this.ProviderNameInput = this.page.getByRole('textbox', { name: 'Name' });
        this.ShortDescriptionInput = this.page.getByRole('textbox', { name: 'Short Description' });
        this.LongDescriptionInput = this.page.getByRole('textbox', { name: 'Long Description' });
        this.ProviderTypeDropDown = new DropDownWrapper('#mui-component-select-vendor', this.page);
        this.DefaultModelInput = this.page.getByRole('textbox', { name: 'Default Model' });
        // The budget field is a mode choice ("No limit"/"Default" vs "Fixed
        // amount"); the amount box only appears for a fixed amount.
        this.MonthlyBudgetMode = this.page.getByRole('combobox', { name: /^Monthly budget/ });
        this.MonthlyBudgetInput = this.page.getByTestId('llm-budget-amount');
        this.BudgetStartDateInput = this.page.getByRole('textbox', { name: 'Budget Start Date' });
        this.PrivacyScoreInput = this.page.locator('input[name="privacy_score"]');
        this.PrivacyLevelSelect = this.page.getByRole('combobox', { name: 'Privacy level' });
        this.ModelPatternInput = this.page.getByRole('textbox', { name: 'Model Pattern' });
        this.AccessDetailsButton = this.page.getByRole('button', { name: 'Access Details' });
        this.ApiEndpointInput = this.page.getByRole('textbox', { name: 'API Endpoint' });
        this.ApiKeyInput = this.page.getByRole('textbox', { name: 'API Key' });
        this.PortalDisplayInformationButton = this.page.getByRole('button', { name: 'Portal Display Information' });
        this.LogoUrlInput = this.page.getByRole('textbox', { name: 'Logo URL' });
        this.ActiveCheckbox = this.page.getByRole('checkbox', { name: 'Active' });
        this.ActiveColumnHeader = this.page.getByRole('columnheader', { name: 'Active' });
        this.FiltersButton = this.page.getByRole('button', { name: 'Filters' });
        this.SaveButton = this.page.getByRole('button', { name: 'Add LLM provider' });
        this.CancelButton = this.page.getByRole('button', { name: 'Cancel' });
        this.EditProviderButton = this.page.getByRole('menuitem', { name: 'Edit LLM provider' });
        this.DeactivateProviderButton = this.page.getByRole('menuitem', { name: 'Deactivate LLM provider' });
        this.BackToLLMsLink = this.page.getByRole('link', { name: 'Back to LLM providers' });
    }

    async goto() {
        await this.page.goto('/admin/llm-providers');
    }

    /** Picks a named privacy level from the select; the score follows to the band's default. */
    async selectPrivacyLevel(level: 'Public' | 'Internal' | 'Confidential' | 'Restricted') {
        await this.PrivacyLevelSelect.click();
        await this.page.getByRole('option', { name: new RegExp(`^${level} \\(`) }).click();
    }

    /** The named level currently shown by the privacy control, e.g. "Confidential". */
    async privacyLevelText(): Promise<string> {
        const text = (await this.PrivacyLevelSelect.textContent()) || '';
        return text.replace(/\s*\(.*$/, '').trim();
    }

    /** Deletes an LLM from its row menu and confirms the "Delete <name>?" dialog. */
    async deleteProvider(name: string) {
        await this.Table.deleteRowWithText(name);
        await this.confirmDelete(name);
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
            await this.setMonthlyBudget(params.monthlyBudget);
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

    // setMonthlyBudget picks "Fixed amount" and fills the amount.
    async setMonthlyBudget(amount: string) {
        await this.MonthlyBudgetMode.click();
        await this.page.getByRole('option', { name: 'Fixed amount' }).click();
        await this.MonthlyBudgetInput.fill(amount);
    }
}
