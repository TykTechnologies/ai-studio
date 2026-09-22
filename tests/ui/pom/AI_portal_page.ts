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
    // The builder's "Add access" picker: a tab per asset type and a list with
    // an "Add {name}" row per item. What has been added is listed under
    // "Access requested" in the details column, grouped by type.
    readonly AccessPicker: Locator;
    readonly AccessOptions: Locator;
    readonly RequestedAccess: Locator;
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
        // Exact: the overview's "All apps (n)" link would otherwise match too.
        this.AppsMenuButton = this.page.getByRole('link', { name: 'Apps', exact: true });
        // The portal sidebar's "Browse" section (the unified catalog, one entry per asset type).
        this.CataloguesMenuButton = this.page.getByRole('button', { name: 'Browse' });
        this.DatasourcesMenuButton = this.page.getByRole('link', { name: 'Data sources' });
        this.LLMPrvidersMenuButton = this.page.getByRole('link', { name: 'LLM providers' });
        // The overview's title-bar action (its empty state repeats the same button).
        this.CreateANewAppButton = this.page.getByRole('button', { name: 'Create app' }).first();
        this.ViewYourAppsButton = this.page.getByText('View your Apps and Credentials');
        this.Table = new TableWrapper('table', page);
        this.CreateappButton = this.page.getByRole('button', { name: 'Create app' });
        this.NameInput = this.page.getByRole('textbox', { name: 'Name' });
        this.DescriptionInput = this.page.getByRole('textbox', { name: 'Description' });
        this.AccessPicker = this.page.getByTestId('app-access-picker');
        this.AccessOptions = this.page.getByTestId('app-access-options');
        this.RequestedAccess = this.page.getByTestId('requested-access');
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

    /** The sidebar highlights one entry, derived from the URL (aria-current). */
    async expectNavSelected(linkName: string) {
        await expect(this.page.getByRole('link', { name: linkName, exact: true })).toHaveAttribute('aria-current', 'page');
        await expect(this.page.locator('a[aria-current="page"][data-nav-id]')).toHaveCount(1);
    }

    /** A row under "Access requested" (the details column or the summary). */
    requestedItem(name: string): Locator {
        return this.RequestedAccess.getByTestId('requested-access-item').filter({ hasText: name });
    }

    /** The picker's tab for an asset type ("LLM providers", "Tools", ...). */
    accessTab(typeLabel: string): Locator {
        return this.AccessPicker.getByRole('tab').filter({
            has: this.page.getByTestId('app-access-tab-label').getByText(typeLabel, { exact: true }),
        });
    }

    /** Open the type's tab and add the item; it is listed as requested at once. */
    async addAccess(typeLabel: string, name: string) {
        await this.accessTab(typeLabel).click();
        await this.AccessOptions.getByRole('button', { name: `Add ${name}`, exact: true }).click();
        await this.requestedItem(name).waitFor();
    }

    async addLlm(name: string) {
        await this.addAccess('LLM providers', name);
    }

    async addDataSource(name: string) {
        await this.addAccess('Data sources', name);
    }

    async addTool(name: string) {
        await this.addAccess('Tools', name);
    }

    /** Remove a requested item from the details column. */
    async removeRequested(name: string) {
        await this.RequestedAccess.getByRole('button', { name: `Remove ${name}`, exact: true }).click();
        await this.requestedItem(name).waitFor({ state: 'detached' });
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
