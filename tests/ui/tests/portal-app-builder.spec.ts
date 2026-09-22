import { test } from '@fixtures';
import { expect, Locator, Page } from '@playwright/test';
import { config } from '@config';
import { generateRandomString } from '@utils/utils';
import { LoginPage } from '@pom/Login_page';

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

/** Distance from the bottom of the fixed top bar to the top of a page title. */
async function titleGapBelowTopBar(page: Page, title: Locator): Promise<number> {
    await title.waitFor();
    const bar = await page.locator('header.MuiAppBar-root').boundingBox();
    const heading = await title.boundingBox();
    expect(bar).not.toBeNull();
    expect(heading).not.toBeNull();
    return heading!.y - (bar!.y + bar!.height);
}

/**
 * The builder's titles must sit where other portal pages put theirs (the
 * submitted page's title used to touch the top bar). My Apps is the
 * reference, so a theme-wide spacing change moves all of them together.
 */
async function expectTitleSpacedLikeMyApps(page: Page, title: Locator, referenceGap: number) {
    const gap = await titleGapBelowTopBar(page, title);
    expect(gap).toBeGreaterThan(0);
    expect(Math.abs(gap - referenceGap)).toBeLessThanOrEqual(1);
}

/**
 * A fresh CI database has the default LLM providers but no tool an app can be
 * granted, so the spec brings its own: a REST tool in a tool catalogue, given
 * to the dev user through a group. Created and removed as the admin.
 */
const TOOL_OAS = Buffer.from(JSON.stringify({
    openapi: '3.0.0',
    info: { title: 'Builder E2E', version: '1.0.0' },
    servers: [{ url: 'https://example.com' }],
    paths: {
        '/forecast': {
            get: { operationId: 'getForecast', summary: 'Forecast', responses: { '200': { description: 'OK' } } },
        },
    },
})).toString('base64');

interface ToolFixture {
    toolName: string;
    toolId?: string;
    catalogueId?: string;
    groupId?: string;
}

async function asAdmin<T>(browser: import('@playwright/test').Browser, fn: (page: Page) => Promise<T>): Promise<T> {
    const context = await browser.newContext();
    try {
        const page = await context.newPage();
        const loginPage = new LoginPage(page);
        await loginPage.goto();
        await loginPage.login(config.admin_email, config.password);
        return await fn(page);
    } finally {
        await context.close();
    }
}

async function createToolFixture(page: Page, fixture: ToolFixture) {
    const headers = await apiHeaders(page);
    const post = async (path: string, data: unknown) => {
        const response = await page.request.post(`${config.api_url}/api/v1${path}`, { headers, data });
        expect(response.ok(), `POST ${path}: ${response.status()} ${await response.text()}`).toBeTruthy();
        return response.json();
    };

    const users = await page.request.get(
        `${config.api_url}/api/v1/users?search=${encodeURIComponent(config.dev_user_email)}`,
        { headers },
    );
    const devUser = (await users.json()).data?.find(
        (u: { attributes?: { email?: string } }) => u.attributes?.email === config.dev_user_email,
    );
    expect(devUser, 'dev user exists').toBeTruthy();

    const tool = await post('/tools', {
        data: {
            type: 'tools',
            attributes: {
                name: fixture.toolName,
                description: 'Forecasts for the app builder spec',
                tool_type: 'REST',
                oas_spec: TOOL_OAS,
                privacy_score: 0,
                operations: ['getForecast'],
                rest_access_enabled: true,
            },
        },
    });
    fixture.toolId = tool.data.id;

    const catalogue = await post('/tool-catalogues', {
        data: { type: 'tool-catalogues', attributes: { name: `${fixture.toolName} catalogue` } },
    });
    fixture.catalogueId = catalogue.data.id;
    await post(`/tool-catalogues/${fixture.catalogueId}/tools`, { data: { type: 'tools', id: String(fixture.toolId) } });

    const group = await post('/groups', {
        data: {
            type: 'groups',
            attributes: {
                name: `${fixture.toolName} team`,
                members: [Number(devUser.id)],
                catalogues: [],
                data_catalogues: [],
                tool_catalogues: [Number(fixture.catalogueId)],
            },
        },
    });
    fixture.groupId = group.data.id;
}

async function removeToolFixture(page: Page, fixture: ToolFixture) {
    const headers = await apiHeaders(page);
    const del = (path: string) => page.request.delete(`${config.api_url}/api/v1${path}`, { headers });
    if (fixture.groupId) await del(`/groups/${fixture.groupId}`);
    if (fixture.catalogueId) await del(`/tool-catalogues/${fixture.catalogueId}`);
    if (fixture.toolId) await del(`/tools/${fixture.toolId}`);
}

async function deleteApp(page: Page, appId: string | undefined) {
    if (!appId) return;
    await page.request.delete(`${config.api_url}/common/apps/${appId}`, { headers: await apiHeaders(page) });
}

test.describe('Portal app builder', () => {
    const fixture: ToolFixture = { toolName: `Forecast ${generateRandomString(4)}` };

    test.beforeAll(async ({ browser }) => {
        await asAdmin(browser, (page) => createToolFixture(page, fixture));
    });

    test.afterAll(async ({ browser }) => {
        await asAdmin(browser, (page) => removeToolFixture(page, fixture));
    });

    test.beforeEach(async ({ loginPage }) => {
        await loginPage.goto();
        await loginPage.login(config.dev_user_email, config.password);
    });

    test('builds a multi-asset app from scratch and summarises the request', async ({ page, aiPortalPage }) => {
        const appName = `Builder app ${generateRandomString(4)}`;
        let appId: string | undefined;
        let referenceGap = 0;

        try {
            await test.step('Measure the title spacing on My Apps', async () => {
                await page.goto(`${config.base_url}/portal/apps`);
                referenceGap = await titleGapBelowTopBar(page, page.getByRole('heading', { name: 'My Apps', level: 1 }));
            });

            await test.step('Open an empty form', async () => {
                await page.goto(`${config.base_url}/portal/app/new`);
                const title = page.getByRole('heading', { name: 'Create New App', level: 1 });
                await expect(title).toBeVisible();
                await expectTitleSpacedLikeMyApps(page, title, referenceGap);
                await expect(page.getByTestId('requested-access-empty')).toBeVisible();
                await expect(aiPortalPage.accessTab('LLM providers')).toHaveAttribute('aria-selected', 'true');
                await expect(aiPortalPage.CreateAppButton).toBeDisabled();
            });

            await test.step('Add items of two types and remove one', async () => {
                await aiPortalPage.NameInput.fill(appName);
                await aiPortalPage.DescriptionInput.fill('Answers forecast questions');
                await aiPortalPage.addLlm('Anthropic');
                await aiPortalPage.addLlm('OpenAI');
                await aiPortalPage.addTool(fixture.toolName);
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
                await expectTitleSpacedLikeMyApps(page, title, referenceGap);

                const summary = page.getByTestId('app-submitted-summary');
                await expect(summary.getByRole('heading', { name: appName })).toBeVisible();
                await expect(summary).toContainText('Answers forecast questions');
                await expect(summary.getByTestId('requested-access-llm')).toContainText('Anthropic');
                await expect(summary.getByTestId('requested-access-llm')).not.toContainText('OpenAI');
                await expect(summary.getByTestId('requested-access-tool')).toContainText(fixture.toolName);
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
