import { test } from '@fixtures';
import { expect } from '@playwright/test';
import { config } from '../config';
import { api, ensurePlugin, pluginAvailable, rpc, storedConfig } from '../utils/prompt_compressor';

/**
 * Prompt Compressor (community plugin): the per-policy tool drop list (eac.40).
 *
 * On the Policies page, a policy gets a drop list: the cache-miss warning
 * shows once the list changes, the never-called suggestions resolve (a fresh
 * stack has no savings records, so it says there are none), an invalid entry
 * is refused, and a valid list is stored. On the Preview page, a body with
 * four tools shows which the list dropped and which it kept, and why. The
 * plugin's original config is restored at the end.
 *
 * Both pages are web components in open shadow roots; Playwright's test-id
 * locators pierce them.
 */

const POLICIES_PATH = '/admin/prompt-compressor/policies';
const PREVIEW_PATH = '/admin/prompt-compressor/preview';

// Four tools; read_file is named by a tool call in the history, so the drop
// list cannot remove it.
const BODY = {
    model: 'claude-sonnet-4-5',
    max_tokens: 512,
    tools: ['Bash', 'read_file', 'web_search_x', 'Grep'].map((name) => ({ name, input_schema: { type: 'object' } })),
    messages: [
        { role: 'user', content: 'hi' },
        { role: 'assistant', content: [{ type: 'tool_use', id: 't1', name: 'read_file', input: {} }] },
        { role: 'user', content: [{ type: 'tool_result', tool_use_id: 't1', content: 'ok' }] },
    ],
};
const DROP = ['read_file', 'web_*', 'Grep'];

test.describe.configure({ mode: 'serial' });

test('Prompt Compressor drop list: editor field, warning, suggestions and preview decisions', async ({ page, loginPage, adminMainPage }) => {
    test.skip(!pluginAvailable(), 'the community submodule does not have the prompt-compressor plugin');
    test.setTimeout(240000);
    const policy = `e2e-drop-${Date.now()}`;

    await loginPage.goto();
    await loginPage.login(config.admin_email, config.password);
    await adminMainPage.dismissQuickStartModal();
    const pluginId = await ensurePlugin(page, adminMainPage, 'getPreviewOptions');
    const original = await storedConfig(page, pluginId);
    page.on('dialog', (dialog) => dialog.accept());

    try {
        // --- the editor field ------------------------------------------------
        await page.goto(`${config.base_url}${POLICIES_PATH}`);
        await adminMainPage.dismissQuickStartModal();
        const ui = page.locator('prompt-compressor-policies');
        await expect(ui.getByRole('heading', { name: 'Compression policies' })).toBeVisible({ timeout: 20000 });
        await ui.getByTestId('pc-new-policy').click();
        await ui.getByTestId('pc-policy-name').fill(policy);

        const field = ui.getByTestId('pc-drop-tools-field');
        await expect(field).toBeVisible();
        await expect(ui.getByTestId('pc-drop-tools-warning')).toHaveCount(0);
        // Suggestions come from the savings records: chips when an App has
        // never-called tools, else a note. Either way they finish loading.
        await expect(field).not.toContainText('Loading suggestions', { timeout: 20000 });
        await expect(field.getByText('No App offered a tool it never called').or(ui.getByTestId('pc-drop-suggestions'))).toBeVisible();

        // An invalid entry is refused on Apply
        await ui.getByTestId('pc-drop-tools').fill('bad name!');
        await ui.getByTestId('pc-drop-tools').blur();
        await expect(ui.getByTestId('pc-drop-tools-warning')).toBeVisible();
        await ui.getByTestId('pc-apply-policy').click();
        await expect(ui.getByTestId('pc-errors')).toContainText('"bad name!" is not a tool name');

        // A valid list applies and saves
        await ui.getByTestId('pc-drop-tools').fill(DROP.join('\n'));
        await ui.getByTestId('pc-drop-tools').blur();
        await expect(ui.getByTestId('pc-drop-tools-warning')).toContainText('misses the prompt cache once');
        await ui.getByTestId('pc-apply-policy').click();
        await expect(ui.locator(`tr[data-policy="${policy}"]`)).toBeVisible();
        await ui.getByTestId('pc-save').click();
        await expect(ui.getByTestId('pc-info')).toContainText('Saved.', { timeout: 20000 });
        await expect.poll(async () => (await storedConfig(page, pluginId)).policies?.[policy]?.drop_tools, { timeout: 20000 }).toEqual(DROP);

        // --- the preview shows each tool's decision -------------------------
        await page.goto(`${config.base_url}${PREVIEW_PATH}`);
        await adminMainPage.dismissQuickStartModal();
        const pv = page.locator('prompt-compressor-preview');
        await expect(pv.getByRole('heading', { name: 'Compression preview' })).toBeVisible({ timeout: 20000 });
        await pv.getByTestId('pc-preview-mode-body').click();
        await pv.getByTestId('pc-preview-vendor').selectOption('anthropic');
        await pv.getByTestId('pc-preview-path').fill('/v1/messages');
        await pv.getByTestId('pc-preview-body').fill(JSON.stringify(BODY));
        await pv.getByTestId('pc-preview-edit-inline').check();
        await pv.getByTestId('pc-preview-policy-json').fill(JSON.stringify({ level: 1, drop_tools: DROP }));
        await pv.getByTestId('pc-preview-run').click();

        const tools = pv.getByTestId('pc-preview-tools');
        await expect(tools).toBeVisible({ timeout: 30000 });
        await expect(tools).toContainText('2 of 4 dropped');
        await expect(pv.getByTestId('pc-preview-tool-dropped')).toHaveCount(2);
        await expect(pv.getByTestId('pc-preview-tool-kept').filter({ hasText: 'read_file' })).toContainText('a tool call in the history names it');
        await expect(pv.getByTestId('pc-preview-tool-kept').filter({ hasText: 'Bash' })).toContainText('Not on the drop list');

        // The RPC reports the same decisions
        const res = await rpc(page, pluginId, 'preview', { mode: 'body', vendor: 'anthropic', path: '/v1/messages', body: BODY, policy: { level: 1, drop_tools: DROP } });
        expect(res.status).toBe(200);
        expect(res.data.report.tool_decisions).toEqual([
            { name: 'Bash', dropped: false, reason: 'not_listed' },
            { name: 'read_file', dropped: false, reason: 'in_history' },
            { name: 'web_search_x', dropped: true, reason: 'dropped' },
            { name: 'Grep', dropped: true, reason: 'dropped' },
        ]);
        expect(res.data.report.strategies?.drop_tools?.leaves).toBe(2);
    } finally {
        const restore = await api(page, 'patch', `/api/v1/plugins/${pluginId}`, { config: original });
        expect(restore.status, JSON.stringify(restore.body)).toBeLessThan(300);
    }
});
