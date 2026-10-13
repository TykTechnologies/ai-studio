import { test } from '@fixtures';
import { expect } from '@playwright/test';
import { config } from '../config';
import { LoginPage } from '../pom/Login_page';

/**
 * Admin sidebar (UX review M8): the group order puts Catalogs directly
 * after Access, and the highlighted entry follows the URL -- a deep link to
 * an app's detail page lights up "Apps" under Portal administration, not
 * "Overview".
 */
test.describe('admin navigation', () => {
    test.beforeEach(async ({ loginPage, adminMainPage }) => {
        await loginPage.goto();
        await loginPage.login(config.admin_email, config.password);
        await adminMainPage.dismissQuickStartModal();
    });

    test('Catalogs sits directly under Access', async ({ adminMainPage }) => {
        const labels = await adminMainPage.topLevelNavLabels();
        expect(labels.slice(0, 4)).toEqual(['Overview', 'Analytics', 'Access', 'Catalogs']);
        await adminMainPage.expectNavGroupAfter('Catalogs', 'Access');
        // Plugins is the last built-in group; plugin-contributed sections
        // (if any) come after Governance, never after Plugins.
        expect(labels[labels.length - 1]).toBe('Plugins');
    });

    test('a deep link to an app highlights Apps, not Overview', async ({ page, adminMainPage }) => {
        await page.goto(`${config.base_url}/admin/apps/1`);
        await adminMainPage.expectNavSelected('Apps', 'Portal administration');
        await adminMainPage.expectNavNotSelected('Overview');
        await adminMainPage.expectNavNotSelected('Edge Gateways');
    });

    test('the highlight follows in-app navigation', async ({ page, adminMainPage }) => {
        await page.goto(`${config.base_url}/admin`);
        await adminMainPage.expectNavSelected('Overview');
        await adminMainPage.navigateToLLMProviders();
        await expect(page).toHaveURL(/\/admin\/llms$/);
        await adminMainPage.expectNavSelected('LLM providers', 'LLM management');
        await adminMainPage.expectNavNotSelected('Overview');
    });
});

/**
 * Deep links (#255, #514 F1/F5): a reload or a pasted URL opens that page,
 * a signed-out visitor returns to it after signing in, and the old
 * catalogs/data paths forward to catalogs/datasources.
 */
test.describe('admin deep links', () => {
    test('reloading a form route stays on it', async ({ page, loginPage, adminMainPage }) => {
        await loginPage.goto();
        await loginPage.login(config.admin_email, config.password);
        await adminMainPage.dismissQuickStartModal();
        await page.goto(`${config.base_url}/admin/filters/new`);
        await page.reload();
        await expect(page).toHaveURL(/\/admin\/filters\/new$/);
    });

    test('catalogs/data forwards to catalogs/datasources', async ({ page, loginPage, adminMainPage }) => {
        await loginPage.goto();
        await loginPage.login(config.admin_email, config.password);
        await adminMainPage.dismissQuickStartModal();
        await page.goto(`${config.base_url}/admin/catalogs/data`);
        await expect(page).toHaveURL(/\/admin\/catalogs\/datasources$/);
        await adminMainPage.expectNavSelected('Data catalogs', 'Catalogs');
    });

    test('a signed-out visitor returns to the deep link after signing in', async ({ browser }) => {
        const context = await browser.newContext();
        const page = await context.newPage();
        try {
            await page.goto(`${config.base_url}/admin/filters/new`);
            await expect(page).toHaveURL(/\/login\?next=%2Fadmin%2Ffilters%2Fnew$/);
            await new LoginPage(page).login(config.admin_email, config.password);
            await expect(page).toHaveURL(/\/admin\/filters\/new$/);
        } finally {
            await context.close();
        }
    });
});
