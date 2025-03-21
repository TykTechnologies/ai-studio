import { Locator, Page, expect } from '@playwright/test';

export class PageTemplate {
    readonly page: Page;
    readonly Popup: Locator;

    constructor(page: Page) {
        this.page = page;
        this.Popup = this.page.locator('.MuiAlert-message');
    }

    async expectPopupWithText(text: string) {
        await expect(this.Popup).toBeVisible();
        await expect(this.Popup).toHaveText(text);
    }

}