import { test } from '@fixtures';
import { expect } from '@playwright/test';
import { config } from '../config';

/**
 * Agent Call Tracing in the Community Edition (jyx.35): the navigation has
 * no Agent sessions entry, and a deep link shows the Enterprise badge
 * without calling the tracing API. Runs only with TEST_EDITION=ce against a
 * CE build (see the readme); the Enterprise suite skips it.
 */
test.skip(process.env.TEST_EDITION !== 'ce', 'Community Edition only (TEST_EDITION=ce)');

test('Community Edition hides agent tracing', async ({ page, loginPage, adminMainPage }) => {
    const tracingCalls: string[] = [];
    page.on('request', (r) => {
        if (r.url().includes('/api/v1/agent-tracing/')) tracingCalls.push(r.url());
    });

    await loginPage.goto();
    await loginPage.login(config.admin_email, config.password);
    await adminMainPage.dismissQuickStartModal();
    await expect(page.getByRole('link', { name: 'Overview' }).first()).toBeVisible();
    // The navigation manifest the drawer renders has no Agent sessions entry.
    const nav = await page.request.get(`${config.api_url}/common/nav`);
    expect(nav.ok(), await nav.text()).toBeTruthy();
    const manifest = await nav.text();
    expect(manifest).toContain('"llms"');
    expect(manifest).not.toContain('agent-sessions');
    await expect(page.getByRole('link', { name: 'Agent sessions' })).toHaveCount(0);

    for (const path of ['/admin/agent-sessions', '/admin/agent-sessions/00000000-0000-7000-8000-000000000001']) {
        await page.goto(`${config.base_url}${path}`);
        await expect(page.getByRole('heading', { name: 'Agent Call Tracing' })).toBeVisible();
        await expect(page.getByRole('link', { name: 'Learn More About Enterprise' })).toBeVisible();
        await expect(page.getByTestId('transcript')).toHaveCount(0);
    }
    await page.waitForLoadState('networkidle');
    expect(tracingCalls, 'tracing API calls from the CE pages').toEqual([]);
});
