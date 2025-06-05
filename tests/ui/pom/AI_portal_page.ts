import { Locator, Page, expect } from '@playwright/test';
import { DropDownWrapper } from '@wrappers/DropDownWrapper';
import { TableWrapper } from '@wrappers/TableWrapper';
import { PageTemplate } from './Page_template';

interface AppParams {
    name: string;
    description: string;
    llm: string;
    monthlyBudget?: string;
}

export class AIPortalPage extends PageTemplate {
    readonly OverviewMenuButton: Locator;
    readonly AppsMenuButton: Locator;
    readonly CataloguesMenuButton: Locator;
    readonly DatasourcesMenuButton: Locator;
    readonly LLMPrvidersMenuButton: Locator;
    readonly CreateANewAppButton: Locator;
    readonly ViewYourAppsButton: Locator;
    readonly Table: TableWrapper;
    readonly CreateappButton: Locator;
    readonly NameInput: Locator;
    readonly DescriptionInput: Locator;
    readonly LlmDropDown: DropDownWrapper;
    readonly AddLlmButton: Locator;
    readonly MonthlyBudgetInput: Locator;
    readonly CreateAppButton: Locator;
    readonly CancelButton: Locator;
    readonly AppsTab: Locator;
    readonly DatasourcesTab: Locator;
    readonly ToolsTab: Locator;
    readonly AppDetailsTitle: Locator;
    readonly AppStatusBadge: Locator;
    readonly KeyIdValue: Locator;
    readonly SecretValue: Locator;
    readonly KeyIdCopyButton: Locator;
    readonly RestUrlCopyButton: Locator;
    readonly DeleteAppButton: Locator;
    readonly ConfirmDeleteButton: Locator;
    readonly CancelDeleteButton: Locator;
    readonly BackToAppsButton: Locator;


    constructor(page: Page) {
        super(page);
        this.OverviewMenuButton = this.page.getByRole('link', { name: 'Overview' });
        this.AppsMenuButton = this.page.getByRole('link', { name: 'Apps' });
        this.CataloguesMenuButton = this.page.getByRole('button', { name: 'Catalogues' });
        this.DatasourcesMenuButton = this.page.getByRole('button', { name: 'Data sources' });
        this.LLMPrvidersMenuButton = this.page.getByRole('button', { name: 'LLM Providers' });
        this.CreateANewAppButton = this.page.getByRole('button', { name: 'Create a new App' });
        this.ViewYourAppsButton = this.page.getByText('View your Apps and Credentials');
        this.Table = new TableWrapper('table', page);
        this.CreateappButton = this.page.getByRole('button', { name: 'Create app' });
        this.NameInput = this.page.getByRole('textbox', { name: 'Name' });
        this.DescriptionInput = this.page.getByRole('textbox', { name: 'Description' });
        this.LlmDropDown = new DropDownWrapper('#mui-component-select-llm_ids', page);
        this.AddLlmButton = this.page.locator('form div:has-text("LLMs (Optional)Select")').getByRole('button', { name: 'Add' });
        this.MonthlyBudgetInput = this.page.getByRole('spinbutton', { name: 'Monthly Budget' });
        this.CreateAppButton = this.page.getByRole('button', { name: 'Create app' });
        this.CancelButton = this.page.getByRole('button', { name: 'Cancel' });
        this.AppsTab = this.page.getByRole('tab', { name: 'Apps' });
        this.DatasourcesTab = this.page.getByRole('tab', { name: 'Datasources' });
        this.ToolsTab = this.page.getByRole('tab', { name: 'Tools' });
        this.AppDetailsTitle = this.page.getByRole('heading', { name: 'App Details' });
        this.AppStatusBadge = this.page.locator('.MuiChip-root');
        this.KeyIdValue = this.page.locator('div:has-text("Key ID") + div');
        this.SecretValue = this.page.locator('div:has-text("Secret") + div');
        this.KeyIdCopyButton = this.page.getByTestId('ContentCopyIcon').first();
        this.RestUrlCopyButton = this.page.getByTestId('ContentCopyIcon').nth(1);
        this.DeleteAppButton = this.page.getByRole('button', { name: 'Delete app' });
        this.ConfirmDeleteButton = this.page.getByRole('button', { name: 'Delete' });
        this.CancelDeleteButton = this.page.getByRole('button', { name: 'Cancel' }).nth(1);
        this.BackToAppsButton = this.page.getByRole('button', { name: 'Back to apps' });
    }

    async goto() {
        await this.page.goto('/portal/apps');
    }

    async createApp(params: AppParams) {
        await this.CreateappButton.click();
        await this.NameInput.fill(params.name);
        await this.DescriptionInput.fill(params.description);
        await this.LlmDropDown.setValue(params.llm);
        if (params.monthlyBudget) {
            await this.MonthlyBudgetInput.fill(params.monthlyBudget);
        }
        await this.CreateAppButton.click();
    }

    async openAppDetails(appName: string) {
        await this.Table.clickRowByText(appName);
        await expect(this.AppDetailsTitle).toBeVisible();
    }

    async deleteApp(appName: string) {
        await this.Table.clickRowByText(appName);
        await this.DeleteAppButton.click();
        await this.ConfirmDeleteButton.click();
    }

    async getKeyId() {
        await this.KeyIdCopyButton.click();
        const keyId = await this.page.evaluate(() => navigator.clipboard.readText());
        return keyId;
    }

    async getRestUrl() {
        await this.RestUrlCopyButton.click();
        const secret = await this.page.evaluate(() => navigator.clipboard.readText());
        return secret;
    }

    async navigateToAppsTab() {
        await this.AppsTab.click();
    }

    async navigateToDatasourcesTab() {
        await this.DatasourcesTab.click();
    }

    async navigateToToolsTab() {
        await this.ToolsTab.click();
    }

    async backToApps() {
        await this.BackToAppsButton.click();
    }

    async expectAppCreated() {
        await this.expectPopupWithText('App created successfully');
    }

    async expectAppDeleted() {
        await this.expectPopupWithText('App deleted successfully');
    }

    async expectAppStatus(status: string) {
        await expect(this.AppStatusBadge).toHaveText(status);
    }
}
