import { Locator, Page } from '@playwright/test';

export class Wrapper {
    element: Locator;
    page: Page;

    constructor(selector: string | Locator, page: Page) {
        if (typeof selector === 'string') {
            this.element = page.locator(selector);
        }
        else {
            this.element = selector;
        }
        this.page = page;
    }

    async click() {
        await this.element.click();
    }

    async getText() {
        return await this.element.textContent();
    }

    async getValue() {
        return await this.element.getAttribute('value');
    }

    async isVisible() {
        return await this.element.isVisible();
    }

    async isDisabled() {
        return await this.element.isDisabled();
    }
}