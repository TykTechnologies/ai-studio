import { Wrapper } from './Wrapper';
import { Page, Locator } from '@playwright/test';

export class DropDownWrapper extends Wrapper {

    constructor(selector: string | Locator, page: Page) {
        super(selector, page);
    }

    async isMultipleChoice() {
        const classList = await this.element.getAttribute('class');
        return classList && classList.includes('MuiSelect-multiple');
    }

    async setValue(text: string, exact: boolean = true) {
        console.log(`Selecting option: ${text}`);
        await this.element.click();

        // Wait for options to appear
        await this.page.waitForSelector('[role="option"]', { timeout: 5000 });

        if (exact) {
            // For exact matching, find option with exact text content
            // Use getByText with exact:true which matches visible text
            const exactOption = this.page.locator('[role="option"]').getByText(text, { exact: true });
            const count = await exactOption.count();

            if (count === 1) {
                await exactOption.click();
            } else if (count > 1) {
                // Multiple exact matches - use first one
                console.log(`Warning: Found ${count} options matching "${text}" exactly, using first`);
                await exactOption.first().click();
            } else {
                // No exact match found, fall back to partial match with warning
                console.log(`Warning: No exact match for "${text}", trying partial match`);
                const partialOption = this.page.locator('[role="option"]').filter({ hasText: text });
                const partialCount = await partialOption.count();
                if (partialCount === 1) {
                    await partialOption.click();
                } else if (partialCount > 1) {
                    throw new Error(`Found ${partialCount} options containing "${text}". Please use exact option name.`);
                } else {
                    throw new Error(`No option found matching "${text}"`);
                }
            }
        } else {
            await this.page.locator('[role="option"]').filter({ hasText: text }).click();
        }

        if (await this.isMultipleChoice()) {
            await this.page.keyboard.press('Escape');
        }
    }
}
