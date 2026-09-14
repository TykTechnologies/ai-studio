import { Locator, Page, expect } from '@playwright/test';
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
    // The builder's resources are RelationshipPickers (compact): the add
    // control is the Autocomplete labelled "Add LLM provider" / "Add data source" /
    // "Add tool", selected items are chips with a "Remove {name}" icon.
    readonly AddLlmInput: Locator;
    readonly AddDataSourceInput: Locator;
    readonly AddToolInput: Locator;
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
        this.CataloguesMenuButton = this.page.getByRole('button', { name: 'Catalogs' });
        this.DatasourcesMenuButton = this.page.getByRole('button', { name: 'Data sources' });
        this.LLMPrvidersMenuButton = this.page.getByRole('button', { name: 'LLM Providers' });
        this.CreateANewAppButton = this.page.getByRole('button', { name: 'Create a new App' });
        this.ViewYourAppsButton = this.page.getByText('View your Apps and Credentials');
        this.Table = new TableWrapper('table', page);
        this.CreateappButton = this.page.getByRole('button', { name: 'Create app' });
        this.NameInput = this.page.getByRole('textbox', { name: 'Name' });
        this.DescriptionInput = this.page.getByRole('textbox', { name: 'Description' });
        this.AddLlmInput = this.page.getByRole('combobox', { name: 'Add LLM provider' });
        this.AddDataSourceInput = this.page.getByRole('combobox', { name: 'Add data source' });
        this.AddToolInput = this.page.getByRole('combobox', { name: 'Add tool' });
        this.MonthlyBudgetInput = this.page.getByRole('spinbutton', { name: 'Monthly Budget' });
        this.CreateAppButton = this.page.getByRole('button', { name: 'Create app' });
        this.CancelButton = this.page.getByRole('button', { name: 'Cancel' });
        this.AppsTab = this.page.getByRole('tab', { name: 'Apps' });
        this.DatasourcesTab = this.page.getByRole('tab', { name: 'Datasources' });
        this.ToolsTab = this.page.getByRole('tab', { name: 'Tools' });
        this.AppDetailsTitle = this.page.getByRole('heading', { name: 'App Details' });
        // The single status chip on the app detail page ("Active", "Awaiting
        // approval" or "Disabled"); the resource chips beside it are plain
        // MuiChips too, so it is picked by test id.
        this.AppStatusBadge = this.page.getByTestId('app-status');
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

    /** Chip for a resource currently selected in any of the builder's pickers. */
    relationshipChip(name: string): Locator {
        return this.page.getByTestId('relationship-picker').locator('.MuiChip-root').filter({ hasText: name });
    }

    /** Type into a picker's Autocomplete and pick the option; it becomes a chip at once. */
    private async pickRelationship(input: Locator, name: string) {
        await input.click();
        await input.fill(name);
        await this.page.getByRole('option', { name, exact: true }).click();
        await this.relationshipChip(name).waitFor();
    }

    async addLlm(name: string) {
        await this.pickRelationship(this.AddLlmInput, name);
    }

    async addDataSource(name: string) {
        await this.pickRelationship(this.AddDataSourceInput, name);
    }

    async addTool(name: string) {
        await this.pickRelationship(this.AddToolInput, name);
    }

    /** Remove a resource via its chip's delete icon. */
    async removeRelationship(name: string) {
        await this.page.getByLabel(`Remove ${name}`).click();
        await this.relationshipChip(name).waitFor({ state: 'detached' });
    }

    async createApp(params: AppParams) {
        await this.CreateappButton.click();
        await this.NameInput.fill(params.name);
        await this.DescriptionInput.fill(params.description);
        await this.addLlm(params.llm);
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

    /** Asserts the Status column of the named app's row in the My Apps table. */
    async expectAppStatusInList(appName: string, status: string) {
        const row = this.Table.element.locator(`tbody tr:has-text("${appName}")`).first();
        await expect(row.getByTestId('app-status')).toHaveText(status);
    }
}
