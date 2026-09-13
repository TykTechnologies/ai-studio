import { test } from '@fixtures';
import { expect, Page } from '@playwright/test';
import { config } from '../config';

/**
 * Asset Catalog plugin (Enterprise) under role-based access control.
 *
 * Registers the plugin from the dev plugin volume if it is not installed yet,
 * then creates one custom role per README recipe, a user per role, one asset
 * awaiting review, one gated asset with a pending access request, and walks
 * the plugin's admin pages as every user: controls the role cannot use are
 * absent, the permitted actions succeed through the UI, and the API refuses
 * what the role does not hold. Users, roles and test assets are removed at
 * the end; the plugin registration is kept.
 */

const PLUGIN_KEY = 'plugin:com.tyk.enterprise.asset-catalog';
const PLUGIN_COMMAND = 'file:///app/bin/plugins/asset-catalog';
const SECTION = 'Asset Catalog';

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

/** Admin RPC through Studio: 403 when the platform gate refuses, else the plugin envelope. */
async function rpc(page: Page, pluginId: string, method: string, payload: Json = {}): Promise<{ status: number; env: Json }> {
    const { status, body } = await api(page, 'post', `/api/v1/plugins/${pluginId}/rpc/${method}`, payload);
    const env = body && typeof body.ok === 'boolean' ? body : (body.data && typeof body.data.ok === 'boolean' ? body.data : body);
    return { status, env };
}

async function portalRpc(page: Page, pluginId: string, method: string, payload: Json = {}): Promise<{ status: number; env: Json }> {
    const { status, body } = await api(page, 'post', `/common/plugins/${pluginId}/portal-rpc/${method}`, payload);
    const env = body && typeof body.ok === 'boolean' ? body : (body.data && typeof body.data.ok === 'boolean' ? body.data : body);
    return { status, env };
}

function idOf(body: Json): string {
    return String(body?.data?.id ?? body?.id ?? '');
}

const PAGES: Record<string, string> = {
    Overview: '/admin/enterprise/asset-catalog/overview',
    'Asset Types': '/admin/enterprise/asset-catalog/types',
    Assets: '/admin/enterprise/asset-catalog/assets',
    'Access Requests': '/admin/enterprise/asset-catalog/requests',
};

/** Open a page of the plugin's sidebar section by in-app navigation. */
async function openPluginPage(page: Page, link: string, tag: string) {
    // The sidebar entry comes first in the DOM; the Overview page repeats the
    // same links as quick-link cards inside the plugin component.
    const item = page.locator(`a[href="${PAGES[link]}"]`).first();
    if (!(await item.isVisible().catch(() => false))) {
        await page.getByRole('button', { name: SECTION }).click();
    }
    await item.click();
    await expect(page.locator(tag)).toBeVisible({ timeout: 20000 });
    // The component paints a spinner first; wait for its heading.
    await expect(page.locator(tag).getByRole('heading').first()).toBeVisible({ timeout: 20000 });
}

async function relogin(page: Page, loginPage: any, adminMainPage: any, usersPage: any, email: string) {
    await usersPage.logOut();
    await page.waitForLoadState('networkidle');
    await loginPage.goto();
    await loginPage.login(email, config.password);
    await expect(page).toHaveURL(/\/admin/, { timeout: 20000 });
    await adminMainPage.dismissQuickStartModal();
}

test.describe.configure({ mode: 'serial' });

test('Asset Catalog rows in the role editor are honoured end to end', async ({
    page,
    loginPage,
    adminMainPage,
    adminUsersPage,
}) => {
    test.setTimeout(600000);
    const ts = Date.now();
    const created = { roles: [] as string[], users: [] as string[], assets: [] as string[] };

    // --- administrator: plugin, roles, users, fixtures -------------------------
    await loginPage.goto();
    await loginPage.login(config.admin_email, config.password);
    await adminMainPage.dismissQuickStartModal();

    let pluginId = '';
    {
        const { body } = await api(page, 'get', '/api/v1/plugins?limit=100');
        const list: Json[] = Array.isArray(body) ? body : (body.data ?? body.plugins ?? []);
        const existing = list.find((p) => (p.attributes?.command ?? p.command) === PLUGIN_COMMAND);
        if (existing) {
            pluginId = String(existing.id);
        } else {
            const res = await api(page, 'post', '/api/v1/plugins', {
                name: 'Asset Catalog', description: 'E2E RBAC walkthrough', command: PLUGIN_COMMAND,
                is_active: true, load_immediately: true, config: { seed_examples: 'true' },
            });
            expect(res.status, JSON.stringify(res.body)).toBeLessThan(300);
            pluginId = idOf(res.body);
        }
        expect(pluginId).not.toBe('');
        // Same steps as the Plugins page: load the manifest (this records the
        // service scopes it asks for), approve them, restart the session.
        const load = await api(page, 'post', `/api/v1/plugins/${pluginId}/validate-and-load`, { command: PLUGIN_COMMAND });
        expect(load.status, JSON.stringify(load.body)).toBeLessThan(300);
        const approve = await api(page, 'post', `/api/v1/plugins/${pluginId}/approve-scopes`, { approved: true });
        expect(approve.status, JSON.stringify(approve.body)).toBeLessThan(300);
        // Restart the plugin session so the per-type permission rows are
        // registered with the now-approved rbac.register scope.
        await api(page, 'post', `/api/v1/plugins/${pluginId}/reload`, {});
        await expect.poll(async () => (await rpc(page, pluginId, 'admin_stats')).env.ok === true, { timeout: 90000, intervals: [2000] }).toBe(true);
    }

    // Per-type rows are registered at runtime for the seeded Agent and Prompt types.
    await expect.poll(async () => {
        const { body } = await api(page, 'get', '/api/v1/rbac/permissions');
        const text = JSON.stringify(body);
        return [`${PLUGIN_KEY}:assets-agent`, `${PLUGIN_KEY}:assets-prompt`, `${PLUGIN_KEY}:access-requests`].every((k) => text.includes(k));
    }, { timeout: 60000, intervals: [2000] }).toBe(true);

    const roleDefs: Record<string, string[]> = {
        reviewer: [`${PLUGIN_KEY}:read`, `${PLUGIN_KEY}:assets:publish`],
        accessReviewer: [`${PLUGIN_KEY}:read`, `${PLUGIN_KEY}:access-requests:write`],
        agentEditor: [`${PLUGIN_KEY}:read`, `${PLUGIN_KEY}:assets-agent:write`],
        typeDesigner: [`${PLUGIN_KEY}:read`, `${PLUGIN_KEY}:asset-types:write`],
        viewer: [`${PLUGIN_KEY}:read`],
    };
    const roleIds: Record<string, number> = {};
    for (const [key, permissions] of Object.entries(roleDefs)) {
        const res = await api(page, 'post', '/api/v1/rbac/roles', { name: `AC ${key} ${ts}`, description: 'asset catalog e2e', permissions });
        expect(res.status, JSON.stringify(res.body)).toBeLessThan(300);
        roleIds[key] = Number(idOf(res.body));
        created.roles.push(idOf(res.body));
    }
    const users: Record<string, string> = {};
    for (const key of [...Object.keys(roleDefs), 'requester']) {
        const email = `ac_${key.toLowerCase()}_${ts}@tyk.io`;
        const roleId = key === 'requester' ? roleIds.viewer : roleIds[key];
        const res = await api(page, 'post', '/api/v1/users', {
            data: { type: 'users', attributes: {
                email, name: `AC ${key}`, password: config.password, is_admin: false,
                show_chat: false, show_portal: true, email_verified: true, notifications_enabled: false,
                role_ids: [roleId],
            } },
        });
        expect(res.status, JSON.stringify(res.body)).toBeLessThan(300);
        users[key] = email;
        created.users.push(idOf(res.body));
    }

    const promptName = `E2E review prompt ${ts}`;
    const agentName = `E2E gated agent ${ts}`;
    {
        const p = await rpc(page, pluginId, 'admin_create_asset', { type_slug: 'prompt', name: promptName, lifecycle: 'in_review', metadata: { body: 'hello', language: 'en' }, change_notes: 'e2e' });
        expect(p.env.ok, JSON.stringify(p.env)).toBe(true);
        created.assets.push(p.env.data.id);
        const a = await rpc(page, pluginId, 'admin_create_asset', { type_slug: 'agent', name: agentName, lifecycle: 'production', requires_approval: true, metadata: { purpose: 'triage', endpoint_url: 'https://secret.example.com', risk_level: 'low' }, change_notes: 'e2e' });
        expect(a.env.ok, JSON.stringify(a.env)).toBe(true);
        created.assets.push(a.env.data.id);
    }
    const [promptId, agentId] = created.assets;

    // Full administrator sees every control (regression).
    await openPluginPage(page, 'Assets', 'asset-catalog-admin-assets');
    await expect(page.getByRole('button', { name: 'New asset' })).toBeVisible();
    const adminRow = page.getByRole('row', { name: new RegExp(promptName) });
    await expect(adminRow).toBeVisible();
    await expect(adminRow.getByRole('button', { name: 'Delete' })).toBeVisible();
    await openPluginPage(page, 'Overview', 'asset-catalog-admin-overview');
    await expect(page.getByRole('button', { name: 'Re-seed examples' })).toBeVisible();

    // --- requester (base read only) asks for access to the gated agent -------
    await relogin(page, loginPage, adminMainPage, adminUsersPage, users.requester);
    {
        const req = await portalRpc(page, pluginId, 'request_access', { asset_id: agentId, form: { justification: 'e2e', intended_use: 'internal_app' } });
        expect(req.env.ok, JSON.stringify(req.env)).toBe(true);
        // Reading the catalog does not unmask gated values.
        const view = await portalRpc(page, pluginId, 'get_asset', { id: agentId });
        expect(view.env.ok).toBe(true);
        expect(view.env.data.has_access).toBe(false);
        expect(view.env.data.metadata.endpoint_url).toBeUndefined();
    }

    // --- reviewer: base read + assets:publish ---------------------------------
    await relogin(page, loginPage, adminMainPage, adminUsersPage, users.reviewer);
    await openPluginPage(page, 'Assets', 'asset-catalog-admin-assets');
    await expect(page.getByRole('button', { name: 'New asset' })).toHaveCount(0);
    {
        const row = page.getByRole('row', { name: new RegExp(promptName) });
        await expect(row).toBeVisible();
        await expect(row.getByRole('button', { name: 'Delete' })).toHaveCount(0);
        await expect(row.getByRole('button', { name: 'Grants' })).toHaveCount(0);
        await row.getByRole('button', { name: 'Lifecycle' }).click();
        const select = page.locator(`select[data-stage-for="${promptId}"]`);
        await expect(select).toBeVisible();
        await expect(select.locator('option[value="approved"]')).toHaveCount(1);
        await select.selectOption('approved');
        await page.getByRole('button', { name: 'Apply' }).click();
        await expect(page.getByText(/Moved to/)).toBeVisible({ timeout: 15000 });
        const after = await rpc(page, pluginId, 'admin_list_assets', { q: promptName });
        expect(after.env.data.items[0].lifecycle).toBe('approved');
        const denied = await rpc(page, pluginId, 'admin_create_asset', { type_slug: 'prompt', name: `nope ${ts}` });
        expect(denied.env.code).toBe('forbidden');
        const del = await rpc(page, pluginId, 'admin_delete_asset', { id: promptId });
        expect(del.env.code).toBe('forbidden');
    }

    // --- access reviewer: base read + access-requests:write -------------------
    await relogin(page, loginPage, adminMainPage, adminUsersPage, users.accessReviewer);
    await openPluginPage(page, 'Access Requests', 'asset-catalog-admin-requests');
    {
        const card = page.locator('[data-request-row]', { hasText: agentName });
        await expect(card).toBeVisible();
        await card.getByRole('button', { name: 'Details' }).click();
        await card.getByRole('button', { name: 'Approve' }).click();
        await expect(page.getByText('Request approved.')).toBeVisible({ timeout: 15000 });
    }
    await openPluginPage(page, 'Assets', 'asset-catalog-admin-assets');
    await expect(page.getByRole('button', { name: 'New asset' })).toHaveCount(0);
    {
        const row = page.getByRole('row', { name: new RegExp(agentName) });
        await expect(row).toBeVisible();
        await expect(row.getByRole('button', { name: 'Lifecycle' })).toHaveCount(0);
        await expect(row.getByRole('button', { name: 'Delete' })).toHaveCount(0);
        const revoke = await rpc(page, pluginId, 'admin_revoke_grant', { asset_id: agentId, user_id: Number(created.users[created.users.length - 1]) });
        expect(revoke.env.code).toBe('forbidden');
    }

    // --- agent editor: base read + assets-agent:write -------------------------
    await relogin(page, loginPage, adminMainPage, adminUsersPage, users.agentEditor);
    await openPluginPage(page, 'Assets', 'asset-catalog-admin-assets');
    {
        await page.getByRole('button', { name: 'New asset' }).click();
        const options = page.locator('#new-type option');
        await expect(options).toHaveCount(1);
        await expect(options.first()).toHaveText('Agent');
        await page.getByRole('button', { name: 'Cancel' }).click();
        // The prompt is approved (publicly visible) but of another type: no controls.
        const promptRow = page.getByRole('row', { name: new RegExp(promptName) });
        await expect(promptRow).toBeVisible();
        for (const name of ['Lifecycle', 'Grants', 'Delete']) {
            await expect(promptRow.getByRole('button', { name })).toHaveCount(0);
        }
        const agentRow = page.getByRole('row', { name: new RegExp(agentName) });
        await expect(agentRow).toBeVisible();
        await expect(agentRow.getByRole('button', { name: 'Delete' })).toHaveCount(0);
        await agentRow.getByRole('button', { name: 'Grants' }).click();
        await expect(page.getByRole('button', { name: 'Revoke' })).toBeVisible();
        const ok = await rpc(page, pluginId, 'admin_create_asset', { type_slug: 'agent', name: `E2E editor agent ${ts}`, metadata: { purpose: 'x' }, change_notes: 'e2e' });
        expect(ok.env.ok, JSON.stringify(ok.env)).toBe(true);
        created.assets.push(ok.env.data.id);
        const denied = await rpc(page, pluginId, 'admin_create_asset', { type_slug: 'prompt', name: `nope ${ts}`, metadata: { body: 'x' } });
        expect(denied.env.code).toBe('forbidden');
        const view = await rpc(page, pluginId, 'admin_list_assets', { q: agentName });
        expect(view.env.data.items[0].can_manage).toBe(true);
        expect(view.env.data.items[0].can_delete).toBe(false);
        expect(view.env.data.items[0].metadata.endpoint_url).toBe('https://secret.example.com');
    }

    // --- type designer: base read + asset-types:write -------------------------
    await relogin(page, loginPage, adminMainPage, adminUsersPage, users.typeDesigner);
    await openPluginPage(page, 'Asset Types', 'asset-catalog-admin-types');
    await expect(page.getByRole('button', { name: 'New type' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Edit' }).first()).toBeVisible();
    await openPluginPage(page, 'Assets', 'asset-catalog-admin-assets');
    await expect(page.getByRole('button', { name: 'New asset' })).toHaveCount(0);
    {
        // Without an Assets read row the page lists only the publicly visible
        // stages (the approved prompt, the production agent), with no controls.
        const row = page.getByRole('row', { name: new RegExp(promptName) });
        await expect(row).toBeVisible();
        for (const name of ['Lifecycle', 'Grants', 'Delete']) {
            await expect(row.getByRole('button', { name })).toHaveCount(0);
        }
        const draft = await rpc(page, pluginId, 'admin_create_asset', { type_slug: 'prompt', name: `nope ${ts}` });
        expect(draft.env.code).toBe('forbidden');
    }

    // --- viewer: base read only -----------------------------------------------
    await relogin(page, loginPage, adminMainPage, adminUsersPage, users.viewer);
    await openPluginPage(page, 'Overview', 'asset-catalog-admin-overview');
    await expect(page.getByRole('button', { name: 'Re-seed examples' })).toHaveCount(0);
    await openPluginPage(page, 'Asset Types', 'asset-catalog-admin-types');
    await expect(page.getByRole('button', { name: 'New type' })).toHaveCount(0);
    await expect(page.getByText('read only').first()).toBeVisible();
    await openPluginPage(page, 'Access Requests', 'asset-catalog-admin-requests');
    await expect(page.getByText(/Access requests: read/)).toBeVisible();
    {
        const reseed = await rpc(page, pluginId, 'admin_reseed_examples');
        expect(reseed.status).toBe(403); // base write: refused by Studio itself
    }

    // --- clean up as the administrator ----------------------------------------
    await relogin(page, loginPage, adminMainPage, adminUsersPage, config.admin_email);
    await sweep(page, pluginId, created);
});

/**
 * Removes what this spec created, plus leftovers of earlier aborted runs
 * (same naming scheme), so the shared dev database does not accumulate them.
 */
async function sweep(page: Page, pluginId: string, created: { roles: string[]; users: string[]; assets: string[] }) {
    const assets = await rpc(page, pluginId, 'admin_list_assets', { q: 'E2E', page_size: 200 });
    const assetIds = new Set<string>(created.assets);
    for (const a of assets.env.data?.items ?? []) {
        if (/^E2E /.test(a.name)) assetIds.add(a.id);
    }
    for (const id of assetIds) {
        await rpc(page, pluginId, 'admin_delete_asset', { id, hard: true, force: true });
    }
    const users = await api(page, 'get', '/api/v1/users?all=true&search=ac_');
    const userIds = new Set<string>(created.users);
    for (const u of users.body.data ?? []) {
        if (/^ac_[a-z]+_\d+@tyk\.io$/.test(u.attributes?.email ?? '')) userIds.add(String(u.id));
    }
    for (const id of userIds) {
        const res = await api(page, 'delete', `/api/v1/users/${id}`);
        expect(res.status, `delete user ${id}: ${JSON.stringify(res.body)}`).toBeLessThan(300);
    }
    const roles = await api(page, 'get', '/api/v1/rbac/roles');
    const roleIds = new Set<string>(created.roles);
    const roleList: Json[] = Array.isArray(roles.body) ? roles.body : (roles.body.roles ?? roles.body.data ?? []);
    for (const r of roleList) {
        if (/^AC [a-zA-Z]+ \d+$/.test(r.name ?? r.attributes?.name ?? '')) roleIds.add(String(r.id));
    }
    for (const id of roleIds) {
        await api(page, 'delete', `/api/v1/rbac/roles/${id}`);
    }
}
