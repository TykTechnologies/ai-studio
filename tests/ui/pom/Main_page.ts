import { Locator, Page } from '@playwright/test';

export class MainPage {
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
    readonly ExploreByMyselfButton: Locator;

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
        this.UsersLink = this.page.getByRole('link', { name: 'Users' });
        this.UserGroupsLink = this.page.getByRole('link', { name: 'User groups' });
        this.FiltersMiddlewareLink = this.page.getByRole('link', { name: 'Filters & Middleware' });
        this.SecretsLink = this.page.getByRole('link', { name: 'Secrets' });
        this.AiPortalButton = this.page.getByRole('button', { name: 'AI Portal' });
        this.AppsLink = this.page.getByRole('link', { name: 'Apps' });
        this.ChatButton = this.page.getByRole('button', { name: 'Chat' });
        this.CatalogsButton = this.page.getByRole('button', { name: 'Catalogs' });
        this.ModelCallSettingsLink = this.page.getByRole('link', { name: 'Model call settings' });
        this.BannerButton = this.page.getByRole('banner').getByRole('button').filter({ hasText: /^$/ });
        this.ExploreByMyselfButton = this.page.getByRole('button', { name: 'Explore by myself' });
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
        await this.GovernanceButton.click();
        await this.UsersLink.click();
    }

    async navigateToUserGroups() {
        await this.GovernanceButton.click();
        await this.UserGroupsLink.click();
    }

    async navigateToFiltersMiddleware() {
        await this.GovernanceButton.click();
        await this.FiltersMiddlewareLink.click();
    }

    async navigateToSecrets() {
        await this.GovernanceButton.click();
        await this.SecretsLink.click();
    }

    async navigateToApps() {
        await this.AiPortalButton.click();
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
        
        if (await this.ExploreByMyselfButton.isVisible()) {
            await this.ExploreByMyselfButton.scrollIntoViewIfNeeded();
            await this.ExploreByMyselfButton.click();
            await this.page.waitForTimeout(2000);
        }
    }
}
