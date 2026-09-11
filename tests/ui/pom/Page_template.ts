import { Locator, Page, expect } from '@playwright/test';
import { config } from '../config';

export class PageTemplate {
    readonly page: Page;
    readonly Popup: Locator;
    readonly logoutButton: Locator;

    constructor(page: Page) {
        this.page = page;
        this.Popup = this.page.locator('.MuiAlert-message');
        this.logoutButton = this.page.getByTestId('LogoutIcon');
    }

    async expectPopupWithText(text: string) {
        await expect(this.Popup).toBeVisible();
        await expect(this.Popup).toHaveText(text);
    }

    async logOut() {
        await this.logoutButton.click();
        await expect(this.page.getByRole('textbox', { name: 'Email' })).toBeVisible();
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