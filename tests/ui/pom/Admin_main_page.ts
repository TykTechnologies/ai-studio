import { Locator, Page, expect } from '@playwright/test';

export class AdminMainPage {
    readonly page: Page;
    readonly ChatTab: Locator;
    readonly PortalTab: Locator;
    readonly AdminTab: Locator;
    readonly AnalyticsLink: Locator;
    readonly LlmManagementButton: Locator;
    readonly LlmProvidersLink: Locator;
    readonly ModelPricesLink: Locator;
    readonly ContextManagementButton: Locator;
    readonly DataSourcesLink: Locator;
    readonly ToolsLink: Locator;
    readonly GovernanceButton: Locator;
    readonly AccessButton: Locator;
    readonly SettingsButton: Locator;
    readonly UsersLink: Locator;
    readonly UserGroupsLink: Locator;
    readonly FiltersMiddlewareLink: Locator;
    readonly SecretsLink: Locator;
    readonly AiPortalButton: Locator;
    readonly AppsLink: Locator;
    readonly ChatButton: Locator;
    readonly CatalogsButton: Locator;
    readonly ModelCallSettingsLink: Locator;
    readonly BannerButton: Locator;
    readonly OverviewLink: Locator;
    readonly EdgeGatewaysLink: Locator;
    readonly PluginsButton: Locator;
    /** Top-level sidebar entries (groups and links) in the order they render. */
    readonly TopLevelNavItems: Locator;

    constructor(page: Page) {
        this.page = page;
        this.ChatTab = this.page.getByTestId('chat-tab');
        this.PortalTab = this.page.getByTestId('portal-tab');
        this.AdminTab = this.page.getByTestId('admin-tab');
        this.AnalyticsLink = this.page.getByRole('link', { name: 'Analytics' });
        this.LlmManagementButton = this.page.getByRole('button', { name: 'LLM management' });
        this.LlmProvidersLink = this.page.getByRole('link', { name: 'LLM providers' });
        this.ModelPricesLink = this.page.getByRole('link', { name: 'Model prices' });
        this.ContextManagementButton = this.page.getByRole('button', { name: 'Context management' });
        this.DataSourcesLink = this.page.getByRole('link', { name: 'Data sources' });
        this.ToolsLink = this.page.getByRole('link', { name: 'Tools' });
        this.GovernanceButton = this.page.getByRole('button', { name: 'Governance' });
        this.AccessButton = this.page.getByRole('button', { name: 'Access' });
        this.SettingsButton = this.page.getByRole('button', { name: 'Settings' });
        this.UsersLink = this.page.getByRole('link', { name: 'Users' });
        this.UserGroupsLink = this.page.getByRole('link', { name: 'Teams' });
        this.FiltersMiddlewareLink = this.page.getByRole('link', { name: 'Filters' });
        this.SecretsLink = this.page.getByRole('link', { name: 'Secrets' });
        this.AiPortalButton = this.page.getByRole('button', { name: 'AI Portal' });
        this.AppsLink = this.page.getByRole('link', { name: 'Apps' });
        this.ChatButton = this.page.getByRole('button', { name: 'Chat' });
        this.CatalogsButton = this.page.getByRole('button', { name: 'Catalogs' });
        this.ModelCallSettingsLink = this.page.getByRole('link', { name: 'Model call settings' });
        this.BannerButton = this.page.getByRole('banner').getByRole('button').filter({ hasText: /^$/ });
        this.OverviewLink = this.page.getByRole('link', { name: 'Overview' });
        this.EdgeGatewaysLink = this.page.getByRole('link', { name: 'Edge Gateways' });
        this.PluginsButton = this.page.getByRole('button', { name: 'Plugins', exact: true });
        this.TopLevelNavItems = this.page.locator('[data-nav-depth="0"]');
    }

    /** Labels of the top-level sidebar entries, top to bottom. */
    async topLevelNavLabels(): Promise<string[]> {
        await this.TopLevelNavItems.first().waitFor();
        return this.TopLevelNavItems.allTextContents();
    }

    /**
     * Expects `group` to render directly after `before` in the sidebar
     * (Catalogs after Access, plugin sections after Governance, ...).
     */
    async expectNavGroupAfter(group: string, before: string) {
        const labels = await this.topLevelNavLabels();
        const index = labels.indexOf(before);
        expect(index, `"${before}" is in the sidebar: ${labels.join(', ')}`).toBeGreaterThanOrEqual(0);
        expect(labels[index + 1], `"${group}" follows "${before}" in ${labels.join(', ')}`).toBe(group);
    }

    /** The sidebar link that is highlighted for the current page (aria-current). */
    selectedNavLink(): Locator {
        return this.page.locator('a[aria-current="page"][data-nav-id]');
    }

    /** Asserts that exactly this link is highlighted and its group is open. */
    async expectNavSelected(linkName: string, group?: string) {
        const link = this.page.getByRole('link', { name: linkName, exact: true });
        await expect(link).toHaveAttribute('aria-current', 'page');
        await expect(this.selectedNavLink()).toHaveCount(1);
        if (group) {
            await expect(this.page.locator(`[data-nav-depth="0"][data-nav-selected="true"]`)).toHaveText(group);
        }
    }

    async expectNavNotSelected(linkName: string) {
        await expect(this.page.getByRole('link', { name: linkName, exact: true })).not.toHaveAttribute('aria-current', 'page');
    }

    async navigateToEdgeGateways() {
        await this.AiPortalButton.click();
        if (!await this.EdgeGatewaysLink.isVisible()) {
            await this.AiPortalButton.click();
        }
        await this.EdgeGatewaysLink.click();
    }

    async navigateToAnalytics() {
        await this.AnalyticsLink.click();
    }

    async navigateToLLMProviders() {
        await this.LlmManagementButton.click();
        await this.LlmProvidersLink.click();
    }

    async navigateToModelPrices() {
        await this.LlmManagementButton.click();
        await this.ModelPricesLink.click();
    }

    async navigateToDataSources() {
        await this.ContextManagementButton.click();
        await this.DataSourcesLink.click();
    }

    async navigateToTools() {
        await this.ContextManagementButton.click();
        await this.ToolsLink.click();
    }

    async navigateToUsers() {
        await this.AccessButton.click();
        await this.UsersLink.click();
    }

    async navigateToUserGroups() {
        await this.AccessButton.click();
        await this.UserGroupsLink.click();
    }

    /** Filters sit under Context management (next to Data sources and Tools). */
    async navigateToFiltersMiddleware() {
        await this.ContextManagementButton.click();
        if (!await this.FiltersMiddlewareLink.isVisible()) {
            await this.ContextManagementButton.click();
        }
        await this.FiltersMiddlewareLink.click();
    }

    async navigateToSecrets() {
        await this.SettingsButton.click();
        await this.SecretsLink.click();
    }

    async navigateToApps() {
        await this.AiPortalButton.click();
        if (!await this.AppsLink.isVisible()) {  
            await this.AiPortalButton.click();
        }
        await this.AppsLink.click();
    }

    async navigateToChats() {
        await this.ChatButton.click();
        await this.page.getByRole('link', { name: 'Chats' }).click();
    }

    async navigateToModelCallSettings() {
        await this.ChatButton.click();
        await this.ModelCallSettingsLink.click();
    }

    async navigateToCatalogs() {
        await this.CatalogsButton.click();
    }

    async closeBanner() {
        await this.BannerButton.click();
    }

    async dismissQuickStartModal() {
        await this.page.waitForTimeout(2000);
        
        const exploreByMyselfButton = this.page.getByRole('button', { name: 'Explore by myself' });
        if (await exploreByMyselfButton.isVisible()) {
            await exploreByMyselfButton.scrollIntoViewIfNeeded();
            await exploreByMyselfButton.click();
            await this.page.waitForTimeout(2000);
        }
    }
}
