import { test } from '@fixtures';
import { expect, request } from '@playwright/test';
import { config } from '../config';
import { apiDeleteQuietly, apiExpect, roleId } from '@utils/adminApi';

/**
 * Service accounts. An administrator creates one through the users form
 * with the Viewer role; the API key is shown once, in the dialog after
 * saving, and never again on the user's page. The account cannot sign in
 * through the login page, and its key works against /api/v1 with the
 * role's permissions. The account is deleted at the end, also on failure.
 */
test('A service account gets its key once, cannot log in, and its key works on the API', async ({
    page,
    loginPage,
    adminMainPage,
    adminUsersPage,
}) => {
    const name = `PW service account ${Date.now()}`;
    let accountId = 0;

    await loginPage.goto();
    await loginPage.login(config.admin_email, config.password);
    await adminMainPage.dismissQuickStartModal();

    try {
        // --- create it through the users form ----------------------------------------
        const viewer = await roleId(page, 'Viewer');
        await adminUsersPage.goto();
        await adminUsersPage.AddUserButton.click();
        await page.getByRole('radio', { name: 'Service account' }).check();
        await adminUsersPage.NameInput.fill(name);
        // No password, email, Portal, Chat or teams for a service account.
        await expect(page.getByRole('textbox', { name: 'Password' })).toHaveCount(0);
        await expect(page.getByTestId('user-teams-field')).toHaveCount(0);
        await page.locator('#role-select').click();
        await page.getByTestId(`role-option-${viewer}`).click();
        await page.keyboard.press('Escape');
        await page.getByRole('button', { name: 'Add service account' }).click();

        // The key is shown once, in the dialog.
        const dialog = page.getByRole('dialog').filter({ hasText: 'Service account created' });
        await expect(dialog).toBeVisible();
        const key = (await dialog.getByTestId('issued-api-key').innerText()).trim();
        expect(key.length).toBeGreaterThan(20);
        await dialog.getByRole('button', { name: 'Done' }).click();
        await expect(dialog).not.toBeVisible();

        const list = await apiExpect(page, 'GET', `/api/v1/users?search=${encodeURIComponent(name)}`, 200);
        const account = (list.data || []).find((u: any) => u.attributes?.name === name);
        expect(account, 'the new service account is listed').toBeTruthy();
        accountId = Number(account.id);
        expect(account.attributes.account_type).toBe('service');
        expect(account.attributes.api_key || '').toBe('');
        const email: string = account.attributes.email;
        expect(email).toMatch(/@service-account\.invalid$/);

        // Never again: the user's page does not show the key.
        await adminUsersPage.gotoAdminPath(`/admin/users/${accountId}`);
        await expect(page.getByText(name).first()).toBeVisible();
        await expect(page.getByText(key)).toHaveCount(0);

        // --- the key works on the API, with the Viewer role --------------------------
        const keyed = await request.newContext({
            baseURL: config.api_url,
            extraHTTPHeaders: { Authorization: `Bearer ${key}` },
        });
        try {
            expect((await keyed.get('/api/v1/llms')).status()).toBe(200);
            expect((await keyed.get('/common/me')).status()).toBe(200);
            // Viewer cannot write, and a service account has no Portal.
            expect((await keyed.post('/api/v1/llms', { data: { data: { attributes: { name: 'nope' } } } })).status()).toBe(403);
            expect((await keyed.get('/common/apps')).status()).toBe(403);
        } finally {
            await keyed.dispose();
        }

        // --- it cannot sign in through the login page --------------------------------
        await adminUsersPage.logOut();
        await page.waitForLoadState('networkidle');
        await loginPage.goto();
        await loginPage.EmailInput.fill(email);
        await loginPage.PasswordInput.fill('Any#Password123');
        await loginPage.LoginButton.click();
        await expect(page.getByText(/invalid email or password/i)).toBeVisible();
        await expect(page).toHaveURL(/\/login/);
    } finally {
        // --- clean up as the administrator -------------------------------------------
        if (accountId) {
            await page.context().clearCookies();
            await loginPage.goto();
            await loginPage.login(config.admin_email, config.password);
            await apiDeleteQuietly(page, `/api/v1/users/${accountId}`);
        }
    }
});
