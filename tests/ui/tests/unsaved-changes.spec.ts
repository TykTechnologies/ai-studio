import { test } from '@fixtures';
import { expect, Page } from '@playwright/test';
import { config } from '@config';
import { generateRandomString } from '@utils/utils';

/**
 * Unsaved-changes guard (UX review M2 / F-03).
 *
 * A dirty form must prompt before an in-app navigation discards the edits;
 * a form that was opened and not touched must not prompt at all.
 */

const filterName = 'Guard filter ' + generateRandomString(4);

/** CSRF headers for page.request calls, as in global-setup. */
async function apiHeaders(page: Page): Promise<Record<string, string>> {
    const origin = new URL(config.base_url).origin;
    const originHeaders = { Origin: origin, Referer: `${origin}/` };
    const csrf = await page.request.get(`${config.api_url}/csrf-token`, { headers: originHeaders });
    return { ...originHeaders, 'X-CSRF-Token': csrf.headers()['x-csrf-token'] || '' };
}

test('Unsaved changes are never discarded silently', async ({ page, loginPage, adminMainPage }) => {
    let filterId: string | undefined;
    const dialog = page.getByTestId('unsaved-changes-dialog');

    await test.step('Log in', async () => {
        await loginPage.goto();
        await loginPage.login(config.admin_email, config.password);
        await adminMainPage.dismissQuickStartModal();
    });

    await test.step('A dirty secret form prompts on a sidebar link', async () => {
        await adminMainPage.navigateToSecrets();
        await page.getByRole('button', { name: 'Add secret' }).first().click();
        await expect(page).toHaveURL(/\/admin\/secrets\/new/);

        await page.getByLabel('Variable Name').fill('GUARD_TEST_SECRET');

        await adminMainPage.navigateToUsers();
        await expect(dialog).toBeVisible();
        await expect(dialog).toContainText('Discard unsaved changes?');
    });

    await test.step('Stay keeps the form and the typed value', async () => {
        await dialog.getByRole('button', { name: 'Stay' }).click();
        await expect(dialog).not.toBeVisible();
        await expect(page).toHaveURL(/\/admin\/secrets\/new/);
        await expect(page.getByLabel('Variable Name')).toHaveValue('GUARD_TEST_SECRET');
    });

    await test.step('The browser back button prompts too and Stay restores the form entry', async () => {
        await page.goBack();
        await expect(dialog).toBeVisible();
        await dialog.getByRole('button', { name: 'Stay' }).click();
        await expect(dialog).not.toBeVisible();
        await expect(page).toHaveURL(/\/admin\/secrets\/new/);
        await expect(page.getByLabel('Variable Name')).toHaveValue('GUARD_TEST_SECRET');
    });

    await test.step('A top tab switch prompts and Stay keeps the admin layout', async () => {
        await adminMainPage.PortalTab.click();
        await expect(dialog).toBeVisible();
        await dialog.getByRole('button', { name: 'Stay' }).click();
        await expect(dialog).not.toBeVisible();
        await expect(page).toHaveURL(/\/admin\/secrets\/new/);
        await expect(page.getByLabel('Variable Name')).toHaveValue('GUARD_TEST_SECRET');
    });

    await test.step('Leave without saving goes to Users', async () => {
        // The Access section is still expanded from the first attempt.
        await adminMainPage.UsersLink.click();
        await expect(dialog).toBeVisible();
        await dialog.getByRole('button', { name: 'Leave without saving' }).click();
        await expect(page).toHaveURL(/\/admin\/users/);
        await expect(dialog).not.toBeVisible();
    });

    await test.step('Create a filter to edit', async () => {
        const headers = await apiHeaders(page);
        // Remove guard filters an earlier, failed run may have left behind.
        const existing = await page.request.get(`${config.api_url}/api/v1/filters`, { headers });
        if (existing.ok()) {
            const list = await existing.json();
            for (const item of list.data || []) {
                if (String(item.attributes?.name || '').startsWith('Guard filter ')) {
                    await page.request.delete(`${config.api_url}/api/v1/filters/${item.id}`, { headers });
                }
            }
        }
        const response = await page.request.post(`${config.api_url}/api/v1/filters`, {
            headers,
            data: {
                data: {
                    type: 'filter',
                    attributes: {
                        name: filterName,
                        description: 'Created by unsaved-changes.spec.ts',
                        script: Buffer.from('function processRequest(req) { return req; }').toString('base64'),
                        response_filter: false,
                    },
                },
            },
        });
        expect(response.ok(), await response.text()).toBeTruthy();
        const body = await response.json();
        filterId = String(body.data?.id ?? body.id);
        expect(filterId).not.toBe('undefined');
    });

    // getByLabel('Name') also matches the page's other "...Name" controls, so
    // the fields are picked by role and exact accessible name.
    const nameField = page.getByRole('textbox', { name: 'Name', exact: true });
    const descriptionField = page.getByRole('textbox', { name: 'Description', exact: true });

    await test.step('An untouched filter edit form does not prompt on back', async () => {
        await page.goto(`${config.base_url}/admin/filters/edit/${filterId}`);
        await expect(nameField).toHaveValue(filterName);

        await page.getByRole('link', { name: 'Back to filters' }).click();
        await expect(page).toHaveURL(/\/admin\/filters$/);
        await expect(dialog).not.toBeVisible();
    });

    await test.step('Cancel on a dirty filter edit prompts, and a clean Cancel does not', async () => {
        await page.goto(`${config.base_url}/admin/filters/edit/${filterId}`);
        await expect(nameField).toHaveValue(filterName);
        await descriptionField.fill('changed');

        await page.getByRole('button', { name: 'Cancel' }).click();
        await expect(dialog).toBeVisible();
        await dialog.getByRole('button', { name: 'Stay' }).click();
        await expect(page).toHaveURL(new RegExp(`/admin/filters/edit/${filterId}`));

        // Undo the edit: back to the baseline means back to clean.
        await descriptionField.fill('Created by unsaved-changes.spec.ts');
        await page.getByRole('button', { name: 'Cancel' }).click();
        await expect(page).toHaveURL(/\/admin\/filters$/);
        await expect(dialog).not.toBeVisible();
    });

    await test.step('Clean up', async () => {
        if (filterId) {
            const headers = await apiHeaders(page);
            await page.request.delete(`${config.api_url}/api/v1/filters/${filterId}`, { headers });
        }
    });
});
