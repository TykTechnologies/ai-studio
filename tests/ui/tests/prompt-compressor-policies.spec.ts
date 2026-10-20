import { test } from '@fixtures';
import { expect, Page } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';
import { config } from '../config';

/**
 * Prompt Compressor (community plugin): the Policies page.
 *
 * Registers the plugin from the dev plugin volume if it is not installed yet,
 * then creates a policy, binds it to an App, saves, checks the stored config,
 * shows that an invalid config is refused with a message, and deletes the
 * policy again. On Enterprise it also checks that a role holding only
 * policies:read gets a read-only page and a 403 from saveConfig. The
 * plugin's original config is restored at the end; the registration is kept.
 *
 * The page is a web component in an open shadow root; Playwright's CSS and
 * test-id locators pierce it.
 *
 * tests/compose.yml (and the dev plugin watcher) build the plugin when the
 * community submodule has it; with an older submodule pin the spec skips.
 */

const PLUGIN_SOURCE = path.resolve(__dirname, '../../../community/plugins/prompt-compressor/main.go');

const PLUGIN_KEY = 'plugin:com.tyk.prompt-compressor';
const PLUGIN_COMMAND = 'file:///app/bin/plugins/prompt-compressor';
const SECTION = 'Prompt Compressor';
const PAGE_PATH = '/admin/prompt-compressor/policies';
const TAG = 'prompt-compressor-policies';
const APP_ID = '999999';

type Json = Record<string, any>;

async function csrfHeaders(page: Page): Promise<Record<string, string>> {
    const origin = new URL(config.base_url).origin;
    const originHeaders = { Origin: origin, Referer: `${origin}/` };
    const csrf = await page.request.get(`${config.api_url}/csrf-token`, { headers: originHeaders });
    return { ...originHeaders, 'X-CSRF-Token': csrf.headers()['x-csrf-token'] || '' };
}

async function api(page: Page, method: 'get' | 'post' | 'patch' | 'delete', path: string, data?: Json): Promise<{ status: number; body: Json }> {
    const headers = await csrfHeaders(page);
    const res = await page.request[method](`${config.api_url}${path}`, { headers, data });
    let body: Json = {};
    try { body = await res.json(); } catch { body = {}; }
    return { status: res.status(), body };
}

/** Admin RPC through Studio: the HTTP status and the plugin's own answer. */
async function rpc(page: Page, pluginId: string, method: string, payload: Json = {}): Promise<{ status: number; data: Json }> {
    const { status, body } = await api(page, 'post', `/api/v1/plugins/${pluginId}/rpc/${method}`, payload);
    return { status, data: body && body.data !== undefined ? body.data : body };
}

function idOf(body: Json): string {
    return String(body?.data?.id ?? body?.id ?? '');
}

async function storedConfig(page: Page, pluginId: string): Promise<Json> {
    const { body } = await api(page, 'get', `/api/v1/plugins/${pluginId}`);
    const data = body.data ?? body;
    return data.attributes?.config ?? data.config ?? {};
}

async function openPolicies(page: Page, adminMainPage: any) {
    await page.goto(`${config.base_url}${PAGE_PATH}`);
    await adminMainPage.dismissQuickStartModal();
    await expect(page.locator(TAG)).toBeVisible({ timeout: 20000 });
    await expect(page.locator(TAG).getByRole('heading', { name: 'Compression policies' })).toBeVisible({ timeout: 20000 });
    await expect(page.locator(TAG).getByRole('heading', { name: 'Bindings' })).toBeVisible({ timeout: 20000 });
}

test.describe.configure({ mode: 'serial' });

test('Prompt Compressor policies can be created, bound, refused when invalid and deleted', async ({
    page,
    loginPage,
    adminMainPage,
    adminUsersPage,
}) => {
    test.skip(!fs.existsSync(PLUGIN_SOURCE), 'the community submodule does not have the prompt-compressor plugin');
    test.setTimeout(300000);
    const ts = Date.now();
    const policy = `e2e-${ts}`;

    await loginPage.goto();
    await loginPage.login(config.admin_email, config.password);
    await adminMainPage.dismissQuickStartModal();

    // --- plugin registration --------------------------------------------------
    let pluginId = '';
    {
        const { body } = await api(page, 'get', '/api/v1/plugins?limit=100');
        const list: Json[] = Array.isArray(body) ? body : (body.data ?? body.plugins ?? []);
        const existing = list.find((p) => (p.attributes?.command ?? p.command) === PLUGIN_COMMAND);
        if (existing) {
            pluginId = String(existing.id);
        } else {
            const res = await api(page, 'post', '/api/v1/plugins', {
                name: 'Prompt Compressor', description: 'E2E policies editor', command: PLUGIN_COMMAND,
                is_active: true, load_immediately: true, config: {},
            });
            expect(res.status, JSON.stringify(res.body)).toBeLessThan(300);
            pluginId = idOf(res.body);
        }
        expect(pluginId).not.toBe('');
        const load = await api(page, 'post', `/api/v1/plugins/${pluginId}/validate-and-load`, { command: PLUGIN_COMMAND });
        expect(load.status, JSON.stringify(load.body)).toBeLessThan(300);
        const approve = await api(page, 'post', `/api/v1/plugins/${pluginId}/approve-scopes`, { approved: true });
        expect(approve.status, JSON.stringify(approve.body)).toBeLessThan(300);
        await api(page, 'post', `/api/v1/plugins/${pluginId}/reload`, {});
        // The first session can run before the scopes are approved; retry
        await expect.poll(async () => (await rpc(page, pluginId, 'getEditor')).status, { timeout: 90000, intervals: [2000] }).toBe(200);
        await expect.poll(async () => {
            await page.reload();
            await adminMainPage.dismissQuickStartModal();
            return page.getByRole('button', { name: SECTION, exact: true }).isVisible();
        }, { timeout: 90000, intervals: [2000] }).toBe(true);
    }
    const original = await storedConfig(page, pluginId);
    page.on('dialog', (dialog) => dialog.accept());

    try {
        // --- create a policy and bind it to an App ----------------------------
        await openPolicies(page, adminMainPage);
        const ui = page.locator(TAG);
        await ui.getByTestId('pc-new-policy').click();
        await ui.getByTestId('pc-policy-name').fill(policy);
        await ui.getByTestId('pc-policy-level').selectOption('2');
        await ui.getByTestId('pc-apply-policy').click();
        await expect(ui.locator(`tr[data-policy="${policy}"]`)).toBeVisible();

        await ui.getByTestId('pc-bind-app').fill(APP_ID);
        const appRow = ui.locator('.add-binding').filter({ hasText: 'Bind App' });
        await appRow.locator('select').selectOption(policy);
        await appRow.getByRole('button', { name: 'Add' }).click();
        await expect(ui.locator(`tr[data-binding="app:${APP_ID}"]`)).toBeVisible();

        await ui.getByTestId('pc-save').click();
        await expect(ui.getByText('Saved. Gateways apply the change')).toBeVisible({ timeout: 20000 });

        await expect.poll(async () => {
            const cfg = await storedConfig(page, pluginId);
            return [cfg.policies?.[policy]?.level, cfg.bindings?.[`app:${APP_ID}`]];
        }, { timeout: 20000 }).toEqual([2, policy]);

        // --- an invalid config is refused with a message ----------------------
        await ui.locator(`tr[data-policy="${policy}"]`).getByRole('button', { name: 'Edit' }).click();
        await ui.getByTestId('pc-protect-patterns').fill('[unclosed');
        await ui.getByTestId('pc-apply-policy').click();
        await ui.getByTestId('pc-save').click();
        await expect(ui.getByTestId('pc-errors')).toContainText('protect_patterns');
        const stillStored = await storedConfig(page, pluginId);
        expect(stillStored.policies?.[policy]?.protect_patterns).toBeUndefined();
        await ui.getByRole('button', { name: 'Discard changes' }).click();

        // --- the RPC refuses it too ---------------------------------------------
        const refused = await rpc(page, pluginId, 'saveConfig', { config: { bindings: { 'llm_id:1': 'no-such-policy' } } });
        expect(refused.status).toBe(200);
        expect(refused.data.saved).toBe(false);
        expect(JSON.stringify(refused.data.errors)).toContain('no-such-policy');

        // --- read-only role (Enterprise) ----------------------------------------
        const role = await api(page, 'post', '/api/v1/rbac/roles', {
            name: `PC reader ${ts}`, description: 'prompt compressor e2e',
            permissions: [`${PLUGIN_KEY}:read`, `${PLUGIN_KEY}:policies:read`],
        });
        if (role.status < 300) {
            const roleId = idOf(role.body);
            const email = `pc_reader_${ts}@tyk.io`;
            const user = await api(page, 'post', '/api/v1/users', {
                data: { type: 'users', attributes: {
                    email, name: 'PC reader', password: config.password, is_admin: false,
                    show_chat: false, show_portal: true, email_verified: true, notifications_enabled: false,
                    role_ids: [Number(roleId)],
                } },
            });
            expect(user.status, JSON.stringify(user.body)).toBeLessThan(300);
            const userId = idOf(user.body);
            try {
                await adminUsersPage.logOut();
                await loginPage.goto();
                await loginPage.login(email, config.password);
                await adminMainPage.dismissQuickStartModal();

                expect((await rpc(page, pluginId, 'getEditor')).status).toBe(200);
                expect((await rpc(page, pluginId, 'saveConfig', { config: {} })).status).toBe(403);
                await openPolicies(page, adminMainPage);
                await expect(page.locator(TAG).getByText('You can view these settings')).toBeVisible();
                await expect(page.locator(TAG).getByTestId('pc-save')).toHaveCount(0);
            } finally {
                await adminUsersPage.logOut();
                await loginPage.goto();
                await loginPage.login(config.admin_email, config.password);
                await adminMainPage.dismissQuickStartModal();
                await api(page, 'delete', `/api/v1/users/${userId}`);
                await api(page, 'delete', `/api/v1/rbac/roles/${roleId}`);
            }
        } else {
            test.info().annotations.push({ type: 'skipped', description: `custom roles unavailable (${role.status}): RBAC part needs Enterprise` });
        }

        // --- delete the binding and the policy ----------------------------------
        await openPolicies(page, adminMainPage);
        const ui2 = page.locator(TAG);
        await ui2.locator(`tr[data-binding="app:${APP_ID}"]`).getByRole('button', { name: 'Remove' }).click();
        await ui2.locator(`tr[data-policy="${policy}"]`).getByRole('button', { name: 'Delete' }).click();
        await expect(ui2.locator(`tr[data-policy="${policy}"]`)).toHaveCount(0);
        await ui2.getByTestId('pc-save').click();
        await expect(ui2.getByText('Saved. Gateways apply the change')).toBeVisible({ timeout: 20000 });
        const after = await storedConfig(page, pluginId);
        expect(after.policies?.[policy]).toBeUndefined();
        expect(after.bindings?.[`app:${APP_ID}`]).toBeUndefined();
    } finally {
        const restore = await rpc(page, pluginId, 'saveConfig', { config: original });
        expect(restore.data.saved, JSON.stringify(restore.data)).toBe(true);
    }
});
