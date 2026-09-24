import { test } from '@fixtures';
import { expect } from '@playwright/test';
import { config } from '../config';

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
