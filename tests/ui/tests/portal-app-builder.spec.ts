import { test } from '@fixtures';
import { expect, Locator, Page } from '@playwright/test';
import { config } from '@config';
import { generateRandomString } from '@utils/utils';

/**
 * Portal "Create New App" form: pick an asset type, add items with "+",
 * see them listed under "Access requested" beside Name and Description, and
 * get a summary of the request once it is submitted.
 */

/** CSRF headers for page.request calls, as in global-setup. */
async function apiHeaders(page: Page): Promise<Record<string, string>> {
    const origin = new URL(config.base_url).origin;
    const originHeaders = { Origin: origin, Referer: `${origin}/` };
    const csrf = await page.request.get(`${config.api_url}/csrf-token`, { headers: originHeaders });
    return { ...originHeaders, 'X-CSRF-Token': csrf.headers()['x-csrf-token'] || '' };
}

/** The page title sits clear of the fixed top bar, as on the other portal pages. */
async function expectTitleClearOfTopBar(page: Page, title: Locator) {
    const bar = await page.locator('header.MuiAppBar-root').boundingBox();
    const heading = await title.boundingBox();
    expect(bar).not.toBeNull();
    expect(heading).not.toBeNull();
    expect(heading!.y - (bar!.y + bar!.height)).toBeGreaterThanOrEqual(16);
}

async function deleteApp(page: Page, appId: string | undefined) {
    if (!appId) return;
    await page.request.delete(`${config.api_url}/common/apps/${appId}`, { headers: await apiHeaders(page) });
}

test.describe('Portal app builder', () => {
    test.beforeEach(async ({ loginPage }) => {
        await loginPage.goto();
        await loginPage.login(config.dev_user_email, config.password);
    });

    test('builds a multi-asset app from scratch and summarises the request', async ({ page, aiPortalPage }) => {
        const appName = `Builder app ${generateRandomString(4)}`;
        let appId: string | undefined;

        try {
            await test.step('Open an empty form', async () => {
                await page.goto(`${config.base_url}/portal/app/new`);
                const title = page.getByRole('heading', { name: 'Create New App', level: 1 });
                await expect(title).toBeVisible();
                await expectTitleClearOfTopBar(page, title);
                await expect(page.getByTestId('requested-access-empty')).toBeVisible();
                await expect(aiPortalPage.accessTab('LLM providers')).toHaveAttribute('aria-selected', 'true');
                await expect(aiPortalPage.CreateAppButton).toBeDisabled();
            });

            await test.step('Add items of two types and remove one', async () => {
                await aiPortalPage.NameInput.fill(appName);
                await aiPortalPage.DescriptionInput.fill('Answers weather questions');
                await aiPortalPage.addLlm('Anthropic');
                await aiPortalPage.addLlm('OpenAI');
                await aiPortalPage.addTool('Weather');
                await expect(page.getByText('Access requested (3)')).toBeVisible();
                // The tab shows how many of its items were added.
                await expect(aiPortalPage.accessTab('LLM providers')).toContainText('2');

                await aiPortalPage.removeRequested('OpenAI');
                await expect(page.getByText('Access requested (2)')).toBeVisible();
                await aiPortalPage.accessTab('LLM providers').click();
                await expect(aiPortalPage.AccessOptions.getByRole('button', { name: 'Add OpenAI', exact: true })).toBeVisible();
                await expect(aiPortalPage.AccessOptions.getByRole('button', { name: 'Remove Anthropic', exact: true })).toBeVisible();
            });

            await test.step('Submit and see the summary', async () => {
                const created = page.waitForResponse(
                    (r) => r.url().endsWith('/common/apps') && r.request().method() === 'POST',
                );
                await aiPortalPage.CreateAppButton.click();
                const response = await created;
                expect(response.status()).toBeLessThan(300);
                appId = (await response.json())?.id;

                const title = page.getByRole('heading', { name: 'App Submitted', level: 1 });
                await expect(title).toBeVisible();
                await expectTitleClearOfTopBar(page, title);

                const summary = page.getByTestId('app-submitted-summary');
                await expect(summary.getByRole('heading', { name: appName })).toBeVisible();
                await expect(summary).toContainText('Answers weather questions');
                await expect(summary.getByTestId('requested-access-llm')).toContainText('Anthropic');
                await expect(summary.getByTestId('requested-access-llm')).not.toContainText('OpenAI');
                await expect(summary.getByTestId('requested-access-tool')).toContainText('Weather');
                await expect(summary.getByRole('button', { name: /Remove/ })).toHaveCount(0);
                await expect(page.getByText(/You must select at least one resource/)).toHaveCount(0);
            });

            await test.step('Open the app from the summary', async () => {
                await page.getByRole('button', { name: 'Open app' }).click();
                await expect(page).toHaveURL(new RegExp(`/portal/apps/${appId}$`));
                await expect(page.getByRole('heading', { name: appName, level: 1 })).toBeVisible();
            });
        } finally {
            await deleteApp(page, appId);
        }
    });

    test('pre-seeds the form from a catalog "Build app" link', async ({ page, aiPortalPage }) => {
        await test.step('Follow Build app from the Anthropic catalog page', async () => {
            await page.goto(`${config.base_url}/portal/catalog/llms/2`);
            await page.getByRole('button', { name: 'Build app' }).or(page.getByRole('link', { name: 'Build app' })).first().click();
            await expect(page).toHaveURL(/\/portal\/app\/new\?llm=2/);
        });

        await test.step('The provider is requested and its tab is open', async () => {
            await expect(aiPortalPage.requestedItem('Anthropic')).toBeVisible();
            await expect(page.getByText('Access requested (1)')).toBeVisible();
            await expect(aiPortalPage.accessTab('LLM providers')).toHaveAttribute('aria-selected', 'true');
            await expect(aiPortalPage.AccessOptions.getByRole('button', { name: 'Remove Anthropic', exact: true })).toBeVisible();
        });

        await test.step('Leaving an untouched pre-seeded form does not prompt', async () => {
            await aiPortalPage.CancelButton.click();
            await expect(page).toHaveURL(/\/portal\/apps$/);
            await expect(page.getByTestId('unsaved-changes-dialog')).toHaveCount(0);
        });
    });
});
