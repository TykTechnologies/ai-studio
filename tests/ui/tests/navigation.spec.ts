import { test } from '@fixtures';
import { expect } from '@playwright/test';
import { config } from '../config';

/**
 * Admin sidebar (UX review M8): the group order puts Catalogs directly
 * after Access, and the highlighted entry follows the URL -- a deep link to
 * an app's detail page lights up "Apps" under AI Portal, not "Overview".
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
        await adminMainPage.expectNavSelected('Apps', 'AI Portal');
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
