import { test as base, expect } from '@playwright/test';
import { LoginPage } from '../pom/Login_page';
import { RegisterPage } from '../pom/Register_page';
import { AdminLLMProvidersPage } from '@pom/Admin_LLM_providers_page';
import { AdminMainPage } from '@pom/Admin_main_page';
import { AdminAppsPage } from '@pom/Admin_apps_page';
import { AdminUsersPage } from '@pom/Admin_users_page';
import { AdminCataloguesPage } from '@pom/Admin_catalogues_page';
import { AdminGroupsPage } from '@pom/Admin_groups_page';
import { AIPortalPage } from '@pom/AI_portal_page';

export const test = base.extend<{
    loginPage: LoginPage;
    registerPage: RegisterPage;
    adminLLMProvidersPage: AdminLLMProvidersPage;
    adminMainPage: AdminMainPage;
    adminAppsPage: AdminAppsPage;
    adminUsersPage: AdminUsersPage;
    adminCataloguesPage: AdminCataloguesPage;
    adminGroupsPage: AdminGroupsPage;
    aiPortalPage: AIPortalPage;
    }>({
    loginPage: async ({ page }, use) => {
        const loginPage = new LoginPage(page);
        await use(loginPage);
    },
    registerPage: async ({ page }, use) => {
        const registerPage = new RegisterPage(page);
        await use(registerPage);
    },
    adminLLMProvidersPage: async ({ page }, use) => {
        const adminLLMProvidersPage = new AdminLLMProvidersPage(page);
        await use(adminLLMProvidersPage);
    },
    adminMainPage: async ({ page }, use) => {
        const adminMainPage = new AdminMainPage(page);
        await use(adminMainPage);
    },
    adminAppsPage: async ({ page }, use) => {
        const adminAppsPage = new AdminAppsPage(page);
        await use(adminAppsPage);
    },
    adminUsersPage: async ({ page }, use) => {
        const adminUsersPage = new AdminUsersPage(page);
        await use(adminUsersPage);
    },
    adminCataloguesPage: async ({ page }, use) => {
        const adminCataloguesPage = new AdminCataloguesPage(page);
        await use(adminCataloguesPage);
    },
    adminGroupsPage: async ({ page }, use) => {
        const adminGroupsPage = new AdminGroupsPage(page);
        await use(adminGroupsPage);
    },
    aiPortalPage: async ({ page }, use) => {
        const aiPortalPage = new AIPortalPage(page);
        await use(aiPortalPage);
    }
});