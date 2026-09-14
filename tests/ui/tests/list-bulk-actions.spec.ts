import { test } from '@fixtures';
import { expect, Page } from '@playwright/test';
import { config } from '@config';
import { generateRandomString } from '@utils/utils';
import { TableWrapper } from '@wrappers/TableWrapper';

/**
 * Bulk actions on list pages (UX review M3 / F-21).
 *
 * Two filters are created through the API, both are selected in the Filters
 * list, deleted through the bulk toolbar behind the "Delete 2 filters?"
 * confirmation, and the summary snackbar and the emptied list are asserted.
 */

const runId = generateRandomString(4);
const prefix = `Bulk filter ${runId}`;
const filterNames = [`${prefix} A`, `${prefix} B`];

/** CSRF headers for page.request calls, as in global-setup. */
async function apiHeaders(page: Page): Promise<Record<string, string>> {
    const origin = new URL(config.base_url).origin;
    const originHeaders = { Origin: origin, Referer: `${origin}/` };
    const csrf = await page.request.get(`${config.api_url}/csrf-token`, { headers: originHeaders });
    return { ...originHeaders, 'X-CSRF-Token': csrf.headers()['x-csrf-token'] || '' };
}

test('Filters can be selected and deleted in bulk', async ({ page, loginPage, adminMainPage }) => {
    const table = new TableWrapper('table', page);
    const createdIds: string[] = [];

    await test.step('Log in', async () => {
        await loginPage.goto();
        await loginPage.login(config.admin_email, config.password);
        await adminMainPage.dismissQuickStartModal();
    });

    await test.step('Create two filters through the API', async () => {
        const headers = await apiHeaders(page);
        for (const name of filterNames) {
            const response = await page.request.post(`${config.api_url}/api/v1/filters`, {
                headers,
                data: {
                    data: {
                        type: 'filter',
                        attributes: {
                            name,
                            description: 'Created by list-bulk-actions.spec.ts',
                            script: Buffer.from('function processRequest(req) { return req; }').toString('base64'),
                            response_filter: false,
                        },
                    },
                },
            });
            expect(response.ok(), await response.text()).toBeTruthy();
            const body = await response.json();
            createdIds.push(String(body.data?.id ?? body.id));
        }
    });

    await test.step('Select both in the Filters list', async () => {
        await adminMainPage.navigateToFiltersMiddleware();
        await expect(page).toHaveURL(/\/admin\/filters/);
        // The list paginates at 10; narrow it to this run's filters.
        await page.getByPlaceholder('Search filters by name...').fill(prefix);
        await table.expectRowWithTextExists(filterNames[0]);
        await table.expectRowWithTextExists(filterNames[1]);

        await table.selectRow(filterNames[0]);
        await table.expectSelectedCount(1);
        await table.selectRow(filterNames[1]);
        await table.expectSelectedCount(2);
    });

    await test.step('Bulk delete behind the confirmation', async () => {
        await table.triggerBulkAction('Delete');
        const dialog = page.getByTestId('bulk-delete-dialog');
        await expect(dialog).toContainText('Delete 2 filters?');
        // Dependents are looked up for small selections before the sentence lands.
        await expect(dialog).toContainText(/Nothing references these filters|used by|removes them from everything/);
        await table.confirmBulkDelete();
    });

    await test.step('Summary snackbar and the rows are gone', async () => {
        await expect(page.locator('.MuiAlert-message').filter({ hasText: 'Deleted 2 filters' }).first()).toBeVisible();
        await table.expectRowWithTextNotExists(filterNames[0]);
        await table.expectRowWithTextNotExists(filterNames[1]);
        await expect(page.getByTestId('bulk-actions-toolbar')).toHaveCount(0);

        const headers = await apiHeaders(page);
        for (const id of createdIds) {
            const response = await page.request.get(`${config.api_url}/api/v1/filters/${id}`, { headers });
            expect(response.status()).toBe(404);
        }
    });

    await test.step('Clean up anything left behind', async () => {
        const headers = await apiHeaders(page);
        for (const id of createdIds) {
            await page.request.delete(`${config.api_url}/api/v1/filters/${id}`, { headers }).catch(() => undefined);
        }
    });
});
