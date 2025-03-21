import { test as base, expect } from '@playwright/test';
import { LoginPage } from '../pom/Login_page';
import { RegisterPage } from '../pom/Register_page';
import { AdminLLMProvidersPage } from '@pom/Admin_LLM_providers_page';
import { AdminMainPage } from '@pom/Admin_main_page';
import { AdminAppsPage } from '@pom/Admin_apps_page';

export const test = base.extend<{
    loginPage: LoginPage;
    registerPage: RegisterPage;
    adminLLMProvidersPage: AdminLLMProvidersPage;
    adminMainPage: AdminMainPage;
    adminAppsPage: AdminAppsPage;
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
});