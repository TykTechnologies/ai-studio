import { Locator, Page, expect } from '@playwright/test';
import { config } from '../config';

export class PageTemplate {
    readonly page: Page;
    readonly Popup: Locator;
    /** The avatar at the right of the top bar; opens the account menu. */
    readonly accountMenuButton: Locator;
    /** "Log out" inside the account menu (only present while the menu is open). */
    readonly logoutButton: Locator;

    constructor(page: Page) {
        this.page = page;
        this.Popup = this.page.locator('.MuiAlert-message');
        this.accountMenuButton = this.page.getByRole('button', { name: 'Account menu' });
        this.logoutButton = this.page.getByRole('menuitem', { name: 'Log out' });
    }

    /**
     * Saving a form redirects at once and the success snackbar shows on the
     * destination page, so a quick follow-up action (approve right after
     * create) can put two snackbars on screen at the same time. Match the one
     * with the expected text rather than asserting there is exactly one.
     */
    async expectPopupWithText(text: string) {
        await expect(this.Popup.filter({ hasText: text }).first()).toBeVisible();
    }

    /**
     * Logs out through the account menu (avatar -> "Log out"; the header
     * avatar is no longer a one-click logout) and waits for the login form.
     * The check is the login page's URL plus its "Log in" button: a textbox
     * named "Email" also matches the Users page's "Search by name or
     * email..." box, which let a logout that had not happened pass unnoticed.
     */
    async logOut() {
        await this.accountMenuButton.click();
        await expect(this.logoutButton).toBeVisible();
        await this.logoutButton.click();
        await expect(this.page).toHaveURL(/\/login/, { timeout: 15000 });
        await expect(this.page.getByRole('button', { name: /log in/i })).toBeVisible();
    }

    /**
     * Confirms the "Delete <name>?" dialog that row-menu deletes of LLMs,
     * tools, secrets, datasources, filters and model routers now open
     * (ConfirmationDialog; its title is styled text, not a heading, so the
     * dialog is picked by that text). The confirm button is "Delete".
     */
    async confirmDelete(name: string) {
        const dialog = this.page.getByRole('dialog').filter({ hasText: `Delete ${name}?` });
        await expect(dialog).toBeVisible();
        await dialog.getByRole('button', { name: 'Delete', exact: true }).click();
        await expect(dialog).not.toBeVisible();
    }

    /**
     * Loads an administration page by URL. The layout restores the last drawer
     * selection on a full page load, which would send us to whatever page was
     * visited last, so that stored selection is cleared first.
     */
    async gotoAdminPath(path: string) {
        await this.page.evaluate(() => {
            try {
                localStorage.removeItem('drawer_state_admin');
            } catch {
                // Not on the app origin yet; nothing stored to clear.
            }
        }).catch(() => undefined);
        await this.page.goto(`${config.base_url}${path}`);
    }

}