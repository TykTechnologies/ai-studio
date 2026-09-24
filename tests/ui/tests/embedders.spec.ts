import { test } from '@fixtures';
import { expect, Page } from '@playwright/test';
import { config } from '../config';

/** CSRF headers for page.request calls (it sends no Origin or Referer). */
async function csrfHeaders(page: Page): Promise<Record<string, string>> {
    const origin = new URL(config.base_url).origin;
    const originHeaders = { Origin: origin, Referer: `${origin}/` };
    const csrf = await page.request.get(`${config.api_url}/csrf-token`, { headers: originHeaders });
    return { ...originHeaders, 'X-CSRF-Token': csrf.headers()['x-csrf-token'] || '' };
}

/** POSTs a JSON:API document as the logged-in user and returns the new id. */
async function apiCreate(page: Page, path: string, type: string, attributes: Record<string, unknown>): Promise<string> {
    const response = await page.request.post(`${config.api_url}/api/v1/${path}`, {
        headers: await csrfHeaders(page),
        data: { data: { type, attributes } },
    });
    expect(response.status(), await response.text()).toBe(201);
    return (await response.json()).data.id;
}

async function apiDelete(page: Page, path: string) {
    await page.request.delete(`${config.api_url}/api/v1/${path}`, { headers: await csrfHeaders(page) });
}

/**
 * Embedders: create one on the Embedders page, create another inline from a
 * data source form, then the guards: its model is locked and it cannot be
 * deleted while the data source uses it; once the data source is gone it can.
 */
test('Embedders: create, inline create, model lock and guarded delete', async ({ page, loginPage, adminMainPage, adminEmbeddersPage }) => {
    const stamp = Date.now();
    const standalone = `E2E standalone ${stamp}`;
    const inline = `E2E inline ${stamp}`;
    const datasource = `E2E handbook ${stamp}`;

    await loginPage.goto();
    await loginPage.login(config.admin_email, config.password);
    await adminMainPage.dismissQuickStartModal();

    // --- create on the Embedders page -----------------------------------------
    await adminEmbeddersPage.goto();
    await adminEmbeddersPage.AddButton.click();
    await adminEmbeddersPage.NameInput.fill(standalone);
    await adminEmbeddersPage.choose(page, 'API compatibility', 'OpenAI');
    await expect(adminEmbeddersPage.ModelInput).toHaveValue('text-embedding-3-small');
    await adminEmbeddersPage.ApiKeyInput.fill('$SECRET/E2E_EMBED_KEY');
    await adminEmbeddersPage.CreateButton.click();
    // The detail page: title and connection type.
    await expect(page).toHaveURL(/\/admin\/embedders\/\d+$/);
    await expect(page.getByText(standalone, { exact: true })).toBeVisible();
    await expect(page.getByText('Standalone', { exact: true })).toBeVisible();

    // --- create one inline from the data source form ---------------------------
    await page.goto(`${config.base_url}/admin/datasources/new`);
    await page.getByRole('textbox', { name: 'Name', exact: true }).fill(datasource);
    await adminEmbeddersPage.choose(page, 'User', config.admin_name);
    await adminEmbeddersPage.choose(page, 'Vector Database Type', 'pgvector');
    await page.getByRole('button', { name: 'New embedder' }).click();
    const dialog = page.getByTestId('embedder-create-dialog');
    await dialog.getByTestId('embedder-dialog-name').fill(inline);
    await adminEmbeddersPage.choose(dialog, 'API compatibility', 'Ollama');
    await dialog.getByTestId('embedder-dialog-model').fill('nomic-embed-text');
    await dialog.getByRole('button', { name: 'Create embedder' }).click();
    await expect(dialog).toBeHidden();
    await expect(page.getByRole('combobox', { name: /Embedder/ })).toContainText(inline);
    await page.getByRole('button', { name: 'Add data source' }).click();
    await expect(page).toHaveURL(/\/admin\/datasources$/);

    // --- the model is locked while the data source uses it ---------------------
    await adminEmbeddersPage.goto();
    await adminEmbeddersPage.openRowAction(inline, 'Edit embedder');
    await expect(page.getByText(new RegExp(`Used by ${datasource}`))).toBeVisible();
    await expect(adminEmbeddersPage.ModelInput).toBeDisabled();

    // --- and it cannot be deleted ----------------------------------------------
    await adminEmbeddersPage.goto();
    await adminEmbeddersPage.openRowAction(inline, 'Delete embedder');
    const confirm = page.getByRole('dialog');
    await expect(confirm).toContainText(datasource);
    await confirm.getByRole('button', { name: /Delete/ }).click();
    await adminEmbeddersPage.expectPopupWithText(datasource);
    await expect(await adminEmbeddersPage.row(inline)).toBeVisible();

    // --- remove the data source, then the embedder goes -------------------------
    await page.goto(`${config.base_url}/admin/datasources`);
    const dsRow = page.getByRole('row', { name: new RegExp(datasource) });
    await dsRow.getByRole('button', { name: `Actions for ${datasource}` }).click();
    await page.getByRole('menuitem', { name: /Delete/ }).click();
    await page.getByRole('dialog').getByRole('button', { name: /Delete/ }).click();
    await expect(dsRow).toBeHidden();

    await adminEmbeddersPage.goto();
    for (const name of [inline, standalone]) {
        await adminEmbeddersPage.openRowAction(name, 'Delete embedder');
        await page.getByRole('dialog').getByRole('button', { name: /Delete/ }).click();
        await expect(page.getByRole('row', { name: new RegExp(name) })).toBeHidden();
    }
});

/**
 * A Semantic Router's embedding stage uses an embedder picked in the router
 * form; the detail page links to it.
 */
test('Semantic Router picks its embedder', async ({ page, loginPage, adminMainPage, adminEmbeddersPage }) => {
    const stamp = Date.now();
    await loginPage.goto();
    await loginPage.login(config.admin_email, config.password);
    await adminMainPage.dismissQuickStartModal();

    const llmId = await apiCreate(page, 'llms', 'llms', {
        name: `E2E target ${stamp}`, vendor: 'openai', api_endpoint: 'https://api.openai.com/v1', default_model: 'gpt-4o', privacy_score: 80,
    });
    const first = await apiCreate(page, 'embedders', 'embedders', { name: `E2E first ${stamp}`, vendor: 'openai', model: 'text-embedding-3-small', privacy_score: 50 });
    const second = `E2E second ${stamp}`;
    const secondId = await apiCreate(page, 'embedders', 'embedders', { name: second, vendor: 'ollama', model: 'nomic-embed-text', privacy_score: 50 });
    const routerId = await apiCreate(page, 'semantic-routers', 'semantic-routers', {
        name: `E2E router ${stamp}`, slug: `e2e-router-${stamp}`, embedder_id: Number(first),
        settings: { default_route: 'a' },
        routes: [{ name: 'a', utterances: ['hello there'], target: { type: 'llm', llm_id: Number(llmId), model: 'gpt-4o-mini' } }],
    });

    await page.goto(`${config.base_url}/admin/semantic-routers/edit/${routerId}`);
    const picker = page.getByRole('combobox', { name: /Embedder/ });
    await expect(picker).toContainText(`E2E first ${stamp}`);
    await adminEmbeddersPage.choose(page, 'Embedder', second);
    await page.getByRole('button', { name: 'Update' }).click();
    await expect(page).toHaveURL(/\/admin\/semantic-routers/);

    await page.goto(`${config.base_url}/admin/semantic-routers/${routerId}`);
    await expect(page.getByRole('link', { name: second })).toHaveAttribute('href', `/admin/embedders/${secondId}`);

    await apiDelete(page, `semantic-routers/${routerId}`);
    await apiDelete(page, `embedders/${first}`);
    await apiDelete(page, `embedders/${secondId}`);
    await apiDelete(page, `llms/${llmId}`);
});

/**
 * The built-in Viewer role reads embedders but cannot create, edit or delete
 * them: no Add button, and the create form reached by URL shows the denial
 * panel.
 */
test('Viewer sees embedders read-only', async ({ page, loginPage, adminMainPage, adminUsersPage, adminEmbeddersPage }) => {
    const stamp = Date.now();
    const viewerEmail = `emb_viewer_${stamp}@tyk.io`;
    const name = `E2E visible ${stamp}`;

    await loginPage.goto();
    await loginPage.login(config.admin_email, config.password);
    await adminMainPage.dismissQuickStartModal();
    const embedderId = await apiCreate(page, 'embedders', 'embedders', { name, vendor: 'openai', model: 'text-embedding-3-small', privacy_score: 50 });

    await adminUsersPage.goto();
    await adminUsersPage.AddUserButton.click();
    await adminUsersPage.EmailInput.fill(viewerEmail);
    await adminUsersPage.NameInput.fill('Embedder Viewer');
    await adminUsersPage.PasswordInput.fill(config.password);
    await adminUsersPage.EmailVerifiedCheckbox.check();
    await page.locator('#role-select').click();
    await page.getByRole('option', { name: /^Viewer Read-only/ }).click();
    await page.keyboard.press('Escape');
    await adminUsersPage.SaveUserButton.click();
    await adminUsersPage.searchFor(viewerEmail);
    await adminUsersPage.Table.expectRowWithTextExists(viewerEmail);

    await adminUsersPage.logOut();
    await page.waitForLoadState('networkidle');
    await loginPage.goto();
    await loginPage.login(viewerEmail, config.password);
    await expect(page).toHaveURL(/\/admin/);
    await adminMainPage.dismissQuickStartModal();

    await adminEmbeddersPage.goto();
    await expect(await adminEmbeddersPage.row(name)).toBeVisible();
    await expect(page.getByRole('button', { name: 'Add embedder' })).toHaveCount(0);
    await page.goto(`${config.base_url}/admin/embedders/${embedderId}`);
    await expect(page.getByText(name, { exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Edit' })).toHaveCount(0);
    await page.goto(`${config.base_url}/admin/embedders/new`);
    await expect(page.getByTestId('permission-denied-panel')).toBeVisible();

    // Clean up as the administrator.
    await adminUsersPage.logOut();
    await page.waitForLoadState('networkidle');
    await loginPage.goto();
    await loginPage.login(config.admin_email, config.password);
    await adminMainPage.dismissQuickStartModal();
    await apiDelete(page, `embedders/${embedderId}`);
    await adminUsersPage.goto();
    await adminUsersPage.searchFor(viewerEmail);
    await adminUsersPage.Table.deleteRowWithText(viewerEmail);
});
