import { test } from '@fixtures';
import { expect } from '@playwright/test';
import { config } from '../config';

/**
 * The avatar in the top bar opens an account menu (F-20): who is signed in,
 * "My API key", "Notification preferences" and "Log out". The one-click
 * logout is gone; logOut() in the page objects goes through this menu.
 *
 * Runs as auto_test@tyk.io. The API-key dialog is only opened and read here;
 * the key is neither rolled nor revoked, so other suites using this account
 * keep working.
 */
test('account menu shows identity, API key and preferences, and logs out', async ({
    page,
    loginPage,
    adminMainPage,
    adminUsersPage,
}) => {
    await loginPage.goto();
    await loginPage.login(config.admin_email, config.password);
    await adminMainPage.dismissQuickStartModal();

    // --- open the menu -------------------------------------------------------
    const avatar = page.getByRole('button', { name: 'Account menu' });
    await expect(avatar).toBeVisible();
    await expect(avatar).toHaveAttribute('aria-haspopup', 'menu');
    await avatar.click();

    const menu = page.getByRole('menu');
    await expect(menu).toBeVisible();
    const header = page.getByTestId('account-menu-header');
    await expect(header).toContainText(config.admin_name);
    await expect(header).toContainText(config.admin_email);
    await expect(header).toContainText(/Administrator/);
    await expect(menu.getByRole('menuitem', { name: 'My API key' })).toBeVisible();
    await expect(menu.getByRole('menuitem', { name: 'Notification preferences' })).toBeVisible();
    await expect(menu.getByRole('menuitem', { name: 'Log out' })).toBeVisible();

    // Escape closes the menu without logging out.
    await page.keyboard.press('Escape');
    await expect(menu).not.toBeVisible();
    await expect(avatar).toBeVisible();

    // --- My API key ----------------------------------------------------------
    await avatar.click();
    await page.getByRole('menuitem', { name: 'My API key' }).click();
    const apiKeyDialog = page.getByRole('dialog', { name: 'My API key' });
    await expect(apiKeyDialog).toBeVisible();
    // Either the status line (key or no key) or the SSO policy notice is shown;
    // the roll and revoke buttons are deliberately not pressed.
    await expect(apiKeyDialog.getByTestId('api-key-status').or(apiKeyDialog.getByTestId('api-key-policy'))).toBeVisible();
    await expect(apiKeyDialog.getByTestId('api-key-value')).toHaveCount(0);
    await apiKeyDialog.getByRole('button', { name: 'Close' }).click();
    await expect(apiKeyDialog).not.toBeVisible();

    // --- Notification preferences: email off, then on ------------------------
    await avatar.click();
    await page.getByRole('menuitem', { name: 'Notification preferences' }).click();
    const prefsDialog = page.getByRole('dialog', { name: 'Notification preferences' });
    await expect(prefsDialog).toBeVisible();
    const emailSwitch = prefsDialog.getByRole('checkbox', { name: 'Email notifications' });
    await expect(emailSwitch).toBeVisible();
    // Let the preferences fetch settle before reading the initial state.
    await page.waitForTimeout(500);
    const initiallyOn = await emailSwitch.isChecked();

    const patched = page.waitForResponse((r) => r.url().includes('/common/me/preferences') && r.request().method() === 'PATCH');
    await emailSwitch.click();
    const first = await patched;
    expect(first.ok()).toBeTruthy();
    await expect(emailSwitch).toBeChecked({ checked: !initiallyOn });

    const patchedBack = page.waitForResponse((r) => r.url().includes('/common/me/preferences') && r.request().method() === 'PATCH');
    await emailSwitch.click();
    const second = await patchedBack;
    expect(second.ok()).toBeTruthy();
    await expect(emailSwitch).toBeChecked({ checked: initiallyOn });

    // Reopening shows the persisted value.
    await prefsDialog.getByRole('button', { name: 'Done' }).click();
    await expect(prefsDialog).not.toBeVisible();
    await avatar.click();
    await page.getByRole('menuitem', { name: 'Notification preferences' }).click();
    await expect(prefsDialog).toBeVisible();
    await page.waitForTimeout(500);
    await expect(prefsDialog.getByRole('checkbox', { name: 'Email notifications' })).toBeChecked({ checked: initiallyOn });
    await prefsDialog.getByRole('button', { name: 'Done' }).click();
    await expect(prefsDialog).not.toBeVisible();

    // --- log out through the menu -------------------------------------------
    await adminUsersPage.logOut();
});
