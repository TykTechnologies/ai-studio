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
 * Workspaces (Enterprise). An administrator creates two Workspaces and binds
 * a fresh probe user as Editor in the first one only, through the
 * Workspaces pages. Logged in as that user:
 *   - the Apps list shows only the first Workspace's App;
 *   - an LLM of the second Workspace is listed (LLMs are readable
 *     everywhere) but offers no edit or delete controls;
 *   - an LLM the user creates lands in the first Workspace;
 *   - the second Workspace's App, opened by URL, reads as not found.
 * Everything the test creates is removed afterwards (afterAll, in its own
 * browser context, so a timeout or failure still cleans up).
 */
const stamp = Date.now();
const w1Name = `PW W1 ${stamp}`;
const w2Name = `PW W2 ${stamp}`;
const probeEmail = `pw_ws_${stamp}@tyk.io`;
const probeName = `PW Workspace probe ${stamp}`;
const w1App = `pw-w1-app-${stamp}`;
const w2App = `pw-w2-app-${stamp}`;
const w2Llm = `pw-w2-llm-${stamp}`;
const ownLlm = `pw-own-llm-${stamp}`;

const created = {
    userId: 0,
    llmIds: [] as number[],
    appIds: [] as number[],
};

async function cleanUp(p: Page) {
    for (const id of created.appIds) await apiDeleteQuietly(p, `/api/v1/apps/${id}`);
    for (const name of [w2Llm, ownLlm]) {
        const found = await p.request.get(`${config.api_url}/api/v1/llms?search=${encodeURIComponent(name)}`);
        if (found.ok()) {
            for (const l of (await found.json()).data || []) {
                if (l.attributes?.name === name) await apiDeleteQuietly(p, `/api/v1/llms/${l.id}`);
            }
        }
    }
    if (created.userId) {
        await deleteUserBindings(p, created.userId);
        await apiDeleteQuietly(p, `/api/v1/users/${created.userId}`);
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

test('A Workspace-scoped Editor sees and changes only its own Workspace', async ({
    page,
    loginPage,
    adminMainPage,
    adminLLMProvidersPage,
}) => {
    test.setTimeout(180_000);
    const workspacesPage = new AdminWorkspacesPage(page);

    // --- as the administrator: two Workspaces through the UI -------------------
    await loginPage.goto();
    await loginPage.login(config.admin_email, config.password);
    await adminMainPage.dismissQuickStartModal();
    await workspacesPage.addWorkspace(w1Name, 'Playwright: the probe works here');
    await workspacesPage.addWorkspace(w2Name, 'Playwright: the probe must not change this');
    const w1 = await workspaceIdByName(page, w1Name);
    const w2 = await workspaceIdByName(page, w2Name);
    expect(w1).toBeTruthy();
    expect(w2).toBeTruthy();

    // Fixtures through the API: one App per Workspace, one LLM in W2, and a
    // probe user with no global role.
    for (const [name, ws] of [[w1App, w1], [w2App, w2]] as const) {
        const app = await apiExpect(page, 'POST', '/api/v1/apps', 201, {
            data: { type: 'apps', attributes: { name, workspace_id: ws } },
        });
        created.appIds.push(Number(app.data.id));
    }
    await apiExpect(page, 'POST', '/api/v1/llms', 201, {
        data: { type: 'llms', attributes: { name: w2Llm, vendor: 'openai', workspace_id: w2 } },
    });
    created.userId = await createUser(page, probeEmail, probeName, config.password);

    // Bind the probe as Editor in W1 only, through the Access tab.
    const editor = await roleId(page, 'Editor');
    await workspacesPage.openWorkspace(w1Name);
    await workspacesPage.assignUserRoles(probeName, [editor]);
    await expect(workspacesPage.accessRowForUser(created.userId)).toContainText('Editor');

    await workspacesPage.logOut();
    await page.waitForLoadState('networkidle');
    await loginPage.goto();

    // --- as the Workspace-scoped probe ----------------------------------------
    await loginPage.login(probeEmail, config.password);
    await expect(page).toHaveURL(/\/admin/);
    await adminMainPage.dismissQuickStartModal();

    // Apps: only W1's App is listed.
    await workspacesPage.gotoAdminPath('/admin/apps');
    await expect(page.getByRole('row').filter({ hasText: w1App })).toBeVisible();
    await expect(page.getByRole('row').filter({ hasText: w2App })).toHaveCount(0);

    // The W2 LLM is listed but offers no edit or delete.
    await adminMainPage.navigateToLLMProviders();
    const search = page.getByPlaceholder('Search model providers by name...');
    await search.fill(w2Llm);
    await adminLLMProvidersPage.Table.expectRowWithTextExists(w2Llm);
    const w2Actions = page.getByLabel(`Actions for ${w2Llm}`, { exact: true });
    if (await w2Actions.count()) {
        await w2Actions.click();
        await expect(page.getByRole('menuitem', { name: 'Edit model provider' })).toHaveCount(0);
        await expect(page.getByRole('menuitem', { name: 'Delete model provider' })).toHaveCount(0);
        await page.keyboard.press('Escape');
    }

    // An LLM the probe creates lands in W1 (its only Workspace).
    await adminLLMProvidersPage.AddLLMButton.click();
    await adminLLMProvidersPage.ProviderNameInput.fill(ownLlm);
    await adminLLMProvidersPage.ProviderTypeDropDown.setValue('Anthropic');
    await adminLLMProvidersPage.SaveButton.click();
    await page.getByPlaceholder('Search model providers by name...').fill(ownLlm);
    await adminLLMProvidersPage.Table.expectRowWithTextExists(ownLlm);
    await expect(page.getByRole('row').filter({ hasText: ownLlm })).toContainText(w1Name);
    const mine = await apiExpect(page, 'GET', `/api/v1/llms?search=${encodeURIComponent(ownLlm)}`, 200);
    expect(mine.data.map((l: any) => l.attributes.workspace_id)).toEqual([w1]);

    // W2's App by URL reads as not found.
    await workspacesPage.gotoAdminPath(`/admin/apps/${created.appIds[1]}`);
    await expect(page.getByText(/not found/i).first()).toBeVisible();
    await expect(page.getByText(w2App)).toHaveCount(0);
});
