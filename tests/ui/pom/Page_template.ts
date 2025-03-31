import { Locator, Page, expect } from '@playwright/test';

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

}