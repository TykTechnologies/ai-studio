import { test } from '@fixtures';
import { expect } from '@playwright/test';
import { config } from '../config';
import { ensurePlugin, pluginAvailable, rpc } from '../utils/prompt_compressor';

/**
 * Prompt Compressor (community plugin): the savings dashboard.
 *
 * The UI test stack runs no Microgateway, so no savings records reach
 * Studio: the spec checks the getSavings contract and the dashboard's empty
 * state, range control and charts' absence of data. Totals against real
 * traffic are checked on the dev stack (eac.19 / eac.21).
 */

const PAGE_PATH = '/admin/prompt-compressor/dashboard';
const TAG = 'prompt-compressor-dashboard';

test.describe.configure({ mode: 'serial' });

test('Prompt Compressor dashboard loads and shows its empty state', async ({ page, loginPage, adminMainPage }) => {
    test.skip(!pluginAvailable(), 'the community submodule does not have the prompt-compressor plugin');
    test.setTimeout(240000);

    await loginPage.goto();
    await loginPage.login(config.admin_email, config.password);
    await adminMainPage.dismissQuickStartModal();
    const pluginId = await ensurePlugin(page, adminMainPage, 'getSavings');

    // The RPC answers with aggregates for the range asked
    const res = await rpc(page, pluginId, 'getSavings', { range_hours: 168 });
    expect(res.status).toBe(200);
    expect(res.data.to - res.data.from).toBe(167);
    expect(res.data.totals).toBeTruthy();
    expect(Array.isArray(res.data.series)).toBe(true);
    expect(Array.isArray(res.data.markers)).toBe(true);
    expect(typeof res.data.alert_threshold).toBe('number');
    const empty = res.data.empty === true;

    await page.goto(`${config.base_url}${PAGE_PATH}`);
    await adminMainPage.dismissQuickStartModal();
    const ui = page.locator(TAG);
    await expect(ui.getByRole('heading', { name: 'Compression savings' })).toBeVisible({ timeout: 20000 });

    if (empty) {
        await expect(ui.getByTestId('pc-empty')).toBeVisible({ timeout: 20000 });
        await expect(ui.getByTestId('pc-empty')).toContainText('No savings recorded in this range yet');
    } else {
        // A stack that has seen traffic shows the tiles and both charts
        await expect(ui.getByTestId('pc-tokens-saved')).toBeVisible({ timeout: 20000 });
        await expect(ui.getByTestId('pc-cost-upper')).toContainText('at input price');
        await expect(ui.getByTestId('pc-cache-share')).toBeVisible();
        await expect(ui.getByTestId('pc-savings-chart')).toBeVisible();
        await expect(ui.getByTestId('pc-share-chart')).toBeVisible();
    }

    // The range control asks again for the range picked
    const ranged = page.waitForResponse((r) => r.url().includes(`/plugins/${pluginId}/rpc/getSavings`) && r.request().method() === 'POST');
    await ui.getByTestId('pc-range').selectOption('720');
    const answer = await ranged;
    expect(answer.status()).toBe(200);
    expect(JSON.parse(answer.request().postData() || '{}').range_hours).toBe(720);
});
