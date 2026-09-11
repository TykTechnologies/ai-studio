import { test } from '@fixtures';
import { expect, Page } from '@playwright/test';
import { config } from '../config';

/**
 * Headers the backend's CSRF middleware needs on state-changing calls made
 * through page.request (which sends neither Origin nor Referer itself).
 */
async function csrfHeaders(page: Page): Promise<Record<string, string>> {
    const origin = new URL(config.base_url).origin;
    const originHeaders = { Origin: origin, Referer: `${origin}/` };
    const csrf = await page.request.get(`${config.api_url}/csrf-token`, { headers: originHeaders });
    return { ...originHeaders, 'X-CSRF-Token': csrf.headers()['x-csrf-token'] || '' };
}

/**
 * Role-based access control (Enterprise): an administrator clones the Viewer
 * role, creates a user holding only that role, and the new user sees the
 * administration surface read-only: no "Add" buttons, and a denial panel on
 * a create form reached by URL. Finally the user and role are deleted.
 */
test('Viewer-derived role is read-only in the administration UI', async ({
    page,
    loginPage,
    adminMainPage,
    adminRolesPage,
    adminUsersPage,
}) => {
    const roleName = `Viewer copy ${Date.now()}`;
    const viewerEmail = `viewer_${Date.now()}@tyk.io`;

    // --- as the administrator -------------------------------------------------
    await loginPage.goto();
    await loginPage.login(config.admin_email, config.password);
    await adminMainPage.dismissQuickStartModal();

    await adminRolesPage.goto();
    await adminRolesPage.Table.expectRowWithTextExists('Viewer');
    await adminRolesPage.Table.expectRowWithTextExists('Owner');
    await adminRolesPage.cloneRole('Viewer', roleName);
    await adminRolesPage.grant('tags:write');
    await adminRolesPage.UpdateRoleButton.click();
    await expect(adminRolesPage.pageTitle(roleName)).toBeVisible();

    // Create a user holding only the new role.
    await adminUsersPage.goto();
    await adminUsersPage.AddUserButton.click();
    await adminUsersPage.EmailInput.fill(viewerEmail);
    await adminUsersPage.NameInput.fill('Viewer Copy User');
    await adminUsersPage.PasswordInput.fill(config.password);
    await adminUsersPage.EmailVerifiedCheckbox.check();
    await page.locator('#role-select').click();
    await page.getByRole('option', { name: roleName }).click();
    await page.keyboard.press('Escape');
    await adminUsersPage.SaveUserButton.click();
    await adminUsersPage.searchFor(viewerEmail);
    await adminUsersPage.Table.expectRowWithTextExists(viewerEmail);

    await adminUsersPage.logOut();
    // Logging out reloads the app; wait for it to settle and open a fresh
    // login form, otherwise the credentials typed next are wiped by the reload.
    await page.waitForLoadState('networkidle');
    await loginPage.goto();

    // --- as the read-only user ------------------------------------------------
    await loginPage.login(viewerEmail, config.password);

    // Lands on the administration overview.
    await expect(page).toHaveURL(/\/admin/);
    await adminMainPage.navigateToLLMProviders();
    await expect(adminRolesPage.pageTitle('LLM providers')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Add LLM' })).toHaveCount(0);

    // A create form reached by URL shows the denial panel, not a blank page.
    await adminRolesPage.gotoAdminPath('/admin/llms/new');
    await expect(page.getByTestId('permission-denied-panel')).toBeVisible();
    await expect(page.getByTestId('permission-denied-panel')).toContainText(/LLM providers: write|Llms: write/);

    // Navigation shows only what the role can read: Viewer reads roles but
    // never identity providers.
    await adminMainPage.AccessButton.click();
    await expect(page.getByRole('link', { name: 'Roles' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Identity providers' })).toHaveCount(0);

    // The one write the role carries (tags) is honoured by the API; others are
    // refused with a typed permission error.
    const headers = await csrfHeaders(page);
    const tagResponse = await page.request.post(`${config.api_url}/api/v1/tags`, {
        headers,
        data: { data: { attributes: { name: `viewer-tag-${Date.now()}` } } },
    });
    expect(tagResponse.status()).toBe(201);
    const denied = await page.request.post(`${config.api_url}/api/v1/llms`, {
        headers,
        data: { data: { attributes: { name: 'nope' } } },
    });
    expect(denied.status()).toBe(403);
    expect((await denied.json()).errors[0].code).toBe('permission_denied');

    // --- clean up as the administrator ----------------------------------------
    await adminUsersPage.logOut();
    await page.waitForLoadState('networkidle');
    await loginPage.goto();
    await loginPage.login(config.admin_email, config.password);
    await adminMainPage.dismissQuickStartModal();
    await adminUsersPage.goto();
    await adminUsersPage.searchFor(viewerEmail);
    await adminUsersPage.Table.deleteRowWithText(viewerEmail);
    await adminUsersPage.Table.expectRowWithTextNotExists(viewerEmail);
    await adminRolesPage.goto();
    await adminRolesPage.deleteRole(roleName);
});
