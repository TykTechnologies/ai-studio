import { Wrapper } from './Wrapper';
import { Page } from '@playwright/test';

export class DrowDownWrapper extends Wrapper {

    constructor(selector: string, page: Page) {
        super(selector, page);
    }

    async isMultipleChoice() {
        const classList = await this.element.getAttribute('class');
        return classList && classList.includes('MuiSelect-multiple');
    }

    async setValue(text: string) {
        console.log(`Selecting option: ${text}`);
        await this.element.click();
        await this.page.locator('[role="option"]').filter({ hasText: text }).click();
        if (await this.isMultipleChoice()) {
            await this.page.keyboard.press('Escape');
        }
    }
}