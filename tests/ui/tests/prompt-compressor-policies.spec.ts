import { test } from '@fixtures';
import { expect, Page } from '@playwright/test';
import { config } from '../config';
import { Json, PLUGIN_KEY, api, ensurePlugin, idOf, pluginAvailable, rpc, storedConfig } from '../utils/prompt_compressor';

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
 */

const PAGE_PATH = '/admin/prompt-compressor/policies';
const TAG = 'prompt-compressor-policies';
const APP_ID = '999999';

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
    test.skip(!pluginAvailable(), 'the community submodule does not have the prompt-compressor plugin');
    test.setTimeout(300000);
    const ts = Date.now();
    const policy = `e2e-${ts}`;

    await loginPage.goto();
    await loginPage.login(config.admin_email, config.password);
    await adminMainPage.dismissQuickStartModal();

    const pluginId = await ensurePlugin(page, adminMainPage);
    // The config as the editor shows it (engine tokens masked), which is also
    // what saveConfig accepts back for settings outside the editor
    const editorConfig = async (): Promise<Json> => (await rpc(page, pluginId, 'getEditor')).data.config ?? {};
    // The stored config as is, restored at the end through the host's API
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
        await expect(ui.getByTestId('pc-info')).toContainText('Saved. Compression on edge gateways does not change yet', { timeout: 20000 });
        // Edges apply the change only after a push; the message links there
        await expect(ui.getByTestId('pc-info').getByRole('link', { name: 'Edge Gateways' })).toHaveAttribute('href', '/admin/edge-gateways');

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
        const current = await editorConfig();
        const refused = await rpc(page, pluginId, 'saveConfig', { config: { ...current, bindings: { ...(current.bindings ?? {}), 'llm_id:1': 'no-such-policy' } } });
        expect(refused.status).toBe(200);
        expect(refused.data.saved).toBe(false);
        expect(JSON.stringify(refused.data.errors)).toContain('no-such-policy');

        // --- engine settings belong to the plugin config form (eac.45) --------
        // An administrator stores an engine through the host's plugin API; the
        // editor shows its token masked and refuses to repoint or drop it.
        {
            const patched = await api(page, 'patch', `/api/v1/plugins/${pluginId}`, {
                config: { ...(await storedConfig(page, pluginId)), engine_e2e_url: 'http://127.0.0.1:9', engine_e2e_token: 'e2e-literal' },
            });
            expect(patched.status, JSON.stringify(patched.body)).toBeLessThan(300);
            await expect.poll(async () => (await editorConfig()).engine_e2e_url, { timeout: 30000 }).toBe('http://127.0.0.1:9');
            const shown = await editorConfig();
            expect(shown.engine_e2e_token).toBe('********');

            const { engine_e2e_url: _dropped, ...withoutURL } = shown;
            const drop = await rpc(page, pluginId, 'saveConfig', { config: { ...withoutURL, mode: 'off' } });
            expect(drop.data.saved).toBe(false);
            expect(JSON.stringify(drop.data.errors)).toContain('engine_e2e_url: the policy editor cannot remove');

            const repoint = await rpc(page, pluginId, 'saveConfig', { config: { ...shown, engine_e2e_url: 'https://attacker.example' } });
            expect(repoint.data.saved).toBe(false);
            expect(JSON.stringify(repoint.data.errors)).toContain('engine_e2e_url: the policy editor cannot change');

            const stored = await storedConfig(page, pluginId);
            expect([stored.engine_e2e_url, stored.mode]).toEqual(['http://127.0.0.1:9', shown.mode]);

            // Unchanged engine keys with a policy change save, the token kept
            const keep = await rpc(page, pluginId, 'saveConfig', { config: { ...shown, mode: 'audit' } });
            expect(keep.data.saved, JSON.stringify(keep.data)).toBe(true);
            const kept = await storedConfig(page, pluginId);
            expect([kept.mode, kept.engine_e2e_url, kept.engine_e2e_token]).toEqual(['audit', 'http://127.0.0.1:9', 'e2e-literal']);
        }

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
        await expect(ui2.getByTestId('pc-info')).toContainText('Saved. Compression on edge gateways does not change yet', { timeout: 20000 });
        const after = await storedConfig(page, pluginId);
        expect(after.policies?.[policy]).toBeUndefined();
        expect(after.bindings?.[`app:${APP_ID}`]).toBeUndefined();
    } finally {
        // The host's plugin API restores every key, engine settings included
        const restore = await api(page, 'patch', `/api/v1/plugins/${pluginId}`, { config: original });
        expect(restore.status, JSON.stringify(restore.body)).toBeLessThan(300);
    }
});
