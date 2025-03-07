import { test as base, expect } from '@playwright/test';
import { LoginPage } from '../pom/Login_page';
import { RegisterPage } from '../pom/Register_page';

export const test = base.extend<{
    loginPage: LoginPage;
    registerPage: RegisterPage;
    }>({
    loginPage: async ({ page }, use) => {
        const loginPage = new LoginPage(page);
        await use(loginPage);
    },
    registerPage: async ({ page }, use) => {
        const registerPage = new RegisterPage(page);
        await use(registerPage);
    },
});