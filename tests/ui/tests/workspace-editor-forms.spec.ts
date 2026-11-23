import { test } from '@fixtures';
import { expect, Page } from '@playwright/test';
import { config } from '../config';
import { LoginPage } from '@pom/Login_page';
import { AdminWorkspacesPage } from '@pom/Admin_workspaces_page';
import {
    apiDeleteQuietly,
    apiExpect,
    createUser,
    deleteUserBindings,
    roleId,
    workspaceIdByName,
} from '@utils/adminApi';

/**
 * Workspaces (Enterprise), midsommar-clean-6bg5.10. A user whose only role is
 * Editor within one Workspace could not use the admin forms: the lookups
 * behind them (users, teams, namespaces, vendors, metadata) answered 403, the
 * required User picker stayed empty and the forms could not be submitted.
 *
 * As such a user this test opens the model provider, data source and App
 * forms, checks that no request was refused (403), that the User picker
 * offers the members of the user's Workspace and nobody else, and submits
 * the data source and App forms.
 * Everything the test creates is removed afterwards.
 */
const stamp = Date.now();
const w1Name = `PW forms W1 ${stamp}`;
const w2Name = `PW forms W2 ${stamp}`;
const probeEmail = `pw_forms_${stamp}@tyk.io`;
const probeName = `PW forms probe ${stamp}`;
const outsiderEmail = `pw_forms_out_${stamp}@tyk.io`;
const outsiderName = `PW forms outsider ${stamp}`;
const embedderName = `pw-forms-embedder-${stamp}`;
const appName = `pw-forms-app-${stamp}`;
const datasourceName = `pw-forms-ds-${stamp}`;

const created = { userIds: [] as number[], embedderId: 0 };

async function deleteByName(p: Page, collection: string, name: string) {
    const found = await p.request.get(`${config.api_url}/api/v1/${collection}?search=${encodeURIComponent(name)}`);
    if (!found.ok()) return;
    for (const row of (await found.json()).data || []) {
        if (row.attributes?.name === name) await apiDeleteQuietly(p, `/api/v1/${collection}/${row.id}`);
    }
}

async function cleanUp(p: Page) {
    await deleteByName(p, 'apps', appName);
    await deleteByName(p, 'datasources', datasourceName);
    if (created.embedderId) await apiDeleteQuietly(p, `/api/v1/embedders/${created.embedderId}`);
    for (const id of created.userIds) {
        await deleteUserBindings(p, id);
        await apiDeleteQuietly(p, `/api/v1/users/${id}`);
    }
    for (const name of [w1Name, w2Name]) {
        const id = await workspaceIdByName(p, name);
        if (id) await apiDeleteQuietly(p, `/api/v1/workspaces/${id}`);
    }
}

test.afterAll(async ({ browser }) => {
    const context = await browser.newContext();
    const page = await context.newPage();
    const loginPage = new LoginPage(page);
    await loginPage.goto();
    await loginPage.login(config.admin_email, config.password);
    await cleanUp(page);
    await context.close();
});

test('A Workspace-scoped Editor can fill and submit the data source and App forms', async ({
    page,
    loginPage,
    adminMainPage,
}) => {
    test.setTimeout(240_000);
    const workspacesPage = new AdminWorkspacesPage(page);

    // --- as the administrator: fixtures through the API ------------------------
    await loginPage.goto();
    await loginPage.login(config.admin_email, config.password);
    await adminMainPage.dismissQuickStartModal();
    for (const name of [w1Name, w2Name]) {
        await apiExpect(page, 'POST', '/api/v1/workspaces', 201, { name, description: 'Playwright: Workspace forms' });
    }
    const w1 = await workspaceIdByName(page, w1Name);
    const w2 = await workspaceIdByName(page, w2Name);
    expect(w1).toBeTruthy();
    expect(w2).toBeTruthy();

    const probeId = await createUser(page, probeEmail, probeName, config.password);
    const outsiderId = await createUser(page, outsiderEmail, outsiderName, config.password);
    created.userIds.push(probeId, outsiderId);
    const editor = await roleId(page, 'Editor');
    for (const [user, ws] of [[probeId, w1], [outsiderId, w2]] as const) {
        await apiExpect(page, 'POST', '/api/v1/rbac/bindings', 201, {
            subject_type: 'user', subject_id: user, role_id: editor, scope_type: 'workspace', scope_id: String(ws),
        });
    }
    const embedder = await apiExpect(page, 'POST', '/api/v1/embedders', 201, {
        data: {
            type: 'embedders',
            attributes: {
                name: embedderName, vendor: 'openai', endpoint: 'https://api.openai.com/v1',
                api_key: 'sk-playwright', model: 'text-embedding-3-small', privacy_score: 50, workspace_id: w1,
            },
        },
    });
    created.embedderId = Number(embedder.data.id);

    await workspacesPage.logOut();
    await loginPage.goto();
    await expect(loginPage.EmailInput).toBeVisible();

    // --- as the Workspace-scoped probe ----------------------------------------
    const refused: string[] = [];
    page.on('response', (response) => {
        if (response.status() === 403) {
            refused.push(`${response.request().method()} ${new URL(response.url()).pathname}`);
        }
    });
    await loginPage.login(probeEmail, config.password);
    await expect(page).toHaveURL(/\/admin/);
    await adminMainPage.dismissQuickStartModal();

    // The lookups answer the probe with its own Workspace's members only.
    const users = await apiExpect(page, 'GET', '/api/v1/users?all=true', 200);
    const userNames = (users.data || []).map((u: any) => u.attributes.name);
    expect(userNames).toContain(probeName);
    expect(userNames).not.toContain(outsiderName);
    expect(Object.keys(users.data[0].attributes).sort()).toEqual(['email', 'name']);
    await apiExpect(page, 'GET', `/api/v1/users/${probeId}`, 200);
    await apiExpect(page, 'GET', `/api/v1/users/${outsiderId}`, 404);

    // Model provider form: opens without a refused request.
    await workspacesPage.gotoAdminPath('/admin/llms/new');
    await expect(page.getByRole('button', { name: 'Add model provider' })).toBeVisible();
    await page.waitForLoadState('networkidle');

    // Data source form: the User picker is filled, the embedder can be picked,
    // and the form submits.
    await workspacesPage.gotoAdminPath('/admin/datasources/new');
    await page.locator('#datasource-field-name').fill(datasourceName);
    await page.locator('#mui-component-select-user_id').click();
    await expect(page.getByRole('option', { name: outsiderName })).toHaveCount(0);
    await page.getByRole('option', { name: probeName, exact: true }).click();
    await page.locator('#mui-component-select-db_source_type').click();
    await page.getByRole('option').first().click();
    await expect(page.getByText(/Your role does not include access to embedders/)).toHaveCount(0);
    await page.getByRole('combobox', { name: /Embedder/ }).click();
    await page.getByRole('option', { name: new RegExp(embedderName) }).click();
    await page.waitForLoadState('networkidle');
    await page.getByRole('button', { name: 'Add data source' }).click();
    await expect(page).toHaveURL(/\/admin\/datasources\/?$/);
    const datasources = await apiExpect(page, 'GET', `/api/v1/datasources?search=${encodeURIComponent(datasourceName)}`, 200);
    const ds = (datasources.data || []).find((d: any) => d.attributes.name === datasourceName);
    expect(ds, 'the data source was created').toBeTruthy();
    expect(ds.attributes.workspace_id).toBe(w1);
    expect(Number(ds.attributes.user_id)).toBe(probeId);

    // App form: the owner picker is filled and the form submits.
    await workspacesPage.gotoAdminPath('/admin/apps/new');
    await page.getByRole('textbox', { name: 'Name' }).fill(appName);
    await page.locator('#mui-component-select-user_id').click();
    await expect(page.getByRole('option', { name: outsiderName })).toHaveCount(0);
    await page.getByRole('option', { name: probeName, exact: true }).click();
    await page.waitForLoadState('networkidle');
    await page.getByRole('button', { name: 'Add app' }).click();
    await expect(page).toHaveURL(/\/admin\/apps\/?$/);
    await expect(page.getByRole('row').filter({ hasText: appName })).toBeVisible();

    // The probe's own App details page loads, owner shown, nothing refused.
    await page.getByRole('row').filter({ hasText: appName }).click();
    await expect(page.getByText('App Information')).toBeVisible();
    await expect(page.getByText(probeName).first()).toBeVisible();
    await page.waitForLoadState('networkidle');

    expect(refused, 'requests refused (403) while a Workspace Editor used the forms').toEqual([]);
});
