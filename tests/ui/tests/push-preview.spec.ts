import { test } from '@fixtures';
import { expect, Page } from '@playwright/test';
import { config } from '../config';

/**
 * Push configuration preview (UX review M7). An LLM provider is edited via
 * the API; the sync banner then offers Push Configuration, whose dialog
 * lists that provider under "LLM providers" as updated. After pushing, the
 * banner clears as soon as the edge acknowledges (well under 30 s).
 *
 * Needs a connected edge gateway (the dev stack's edge-dev-docker).
 */

type Json = Record<string, any>;

async function csrfHeaders(page: Page): Promise<Record<string, string>> {
    const origin = new URL(config.base_url).origin;
    const originHeaders = { Origin: origin, Referer: `${origin}/` };
    const csrf = await page.request.get(`${config.api_url}/csrf-token`, { headers: originHeaders });
    return { ...originHeaders, 'X-CSRF-Token': csrf.headers()['x-csrf-token'] || '' };
}

async function api(page: Page, method: 'get' | 'post' | 'patch', path: string, data?: Json): Promise<{ status: number; body: Json }> {
    const headers = await csrfHeaders(page);
    const res = await page.request[method](`${config.api_url}${path}`, { headers, data });
    let body: Json = {};
    try { body = await res.json(); } catch { body = {}; }
    return { status: res.status(), body };
}

test('the push dialog lists what changed and the banner clears after the push', async ({ page, loginPage, adminMainPage }) => {
    test.setTimeout(120000);

    await loginPage.goto();
    await loginPage.login(config.admin_email, config.password);
    await adminMainPage.dismissQuickStartModal();

    // A connected edge is a precondition, not something this test can arrange.
    const edges = await api(page, 'get', '/api/v1/edges');
    expect(edges.status).toBe(200);
    const edge = (edges.body.data || [])[0];
    test.skip(!edge, 'no edge gateway registered with this Studio');
    const heartbeatAge = Date.now() - new Date(edge.attributes.last_heartbeat).getTime();
    test.skip(heartbeatAge > 5 * 60 * 1000, `edge ${edge.attributes.edge_id} has not sent a heartbeat for ${Math.round(heartbeatAge / 60000)} min`);

    // Edit an LLM provider through the API (PATCH is partial: only the
    // description changes).
    const llms = await api(page, 'get', '/api/v1/llms?page_size=1');
    expect(llms.status).toBe(200);
    const llm = llms.body.data[0];
    expect(llm, 'at least one LLM provider exists').toBeTruthy();
    const marker = `UX push preview ${Date.now()}`;
    const patched = await api(page, 'patch', `/api/v1/llms/${llm.id}`, {
        data: { type: 'LLM', attributes: { short_description: marker } },
    });
    expect(patched.status).toBe(200);

    // A fresh load picks the pending state up straight away.
    await page.goto(`${config.base_url}/admin`);
    const banner = page.getByText('Configuration Sync Pending');
    await expect(banner).toBeVisible({ timeout: 20000 });
    await expect(page.getByText(/not yet pushed to|configuration updates pending/)).toBeVisible();

    await page.getByRole('button', { name: 'Push Configuration' }).first().click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText('What will be pushed')).toBeVisible();

    // The edited provider is listed under its type, as updated, with a link.
    const llmGroup = dialog.getByTestId('pending-changes-group-llm');
    await expect(llmGroup).toBeVisible({ timeout: 20000 });
    await expect(llmGroup.getByText('LLM providers')).toBeVisible();
    const row = llmGroup.locator('li').filter({ hasText: llm.attributes.name }).first();
    await expect(row).toBeVisible();
    await expect(row.getByText('updated')).toBeVisible();
    await expect(row.getByRole('link', { name: llm.attributes.name })).toHaveAttribute('href', `/admin/llms/${llm.id}`);
    await expect(dialog.getByTestId('pending-changes-summary').first()).toContainText(/change/);

    // Push, then the banner clears once the edge acknowledges.
    await dialog.getByRole('button', { name: /^Push (Configuration|anyway)$/ }).click();
    await expect(dialog.getByText(/successfully pushed|push initiated/i)).toBeVisible({ timeout: 20000 });
    await dialog.getByRole('button', { name: 'Close' }).click();
    await expect(banner).toBeHidden({ timeout: 30000 });

    // And a fresh dialog now says there is nothing to push.
    await adminMainPage.navigateToEdgeGateways();
    await expect(page.getByTestId('last-pushed').first()).toContainText(/Last pushed \d{2}:\d{2}/, { timeout: 20000 });
    await page.getByRole('button', { name: 'Push Configuration' }).click();
    await expect(page.getByRole('dialog').getByText(/Nothing has changed since the last push/)).toBeVisible({ timeout: 20000 });
    await expect(page.getByRole('dialog').getByRole('button', { name: 'Push anyway' })).toBeVisible();
    await page.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click();
});
