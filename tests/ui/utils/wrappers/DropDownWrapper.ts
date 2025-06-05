import { Locator, Page } from '@playwright/test';

export class DropDownWrapper {
    readonly page: Page;
    readonly selector: string;

    constructor(selector: string, page: Page) {
        this.page = page;
        this.selector = selector;
    }

    async isMultipleChoice(): Promise<boolean> {
        // Get the dropdown element
        const dropdown = await this.getDropdownElement();
        const ariaMultiselectable = await dropdown.getAttribute('aria-multiselectable');
        return ariaMultiselectable === 'true';
    }

    private async getDropdownElement(): Promise<Locator> {
        // Handle different selector types
        if (this.selector.startsWith('#mui-component-select-')) {
            // Direct MUI dropdown selector
            return this.page.locator(this.selector);
        } else if (this.selector.includes('name=')) {
            // Input name selector - extract field name and convert to MUI dropdown selector
            const fieldName = this.selector.match(/name="([^"]+)"/)?.[1];
            if (fieldName) {
                return this.page.locator(`#mui-component-select-${fieldName}`);
            }
        }
        
        // If it's a direct locator or other selector type, use as is
        return this.page.locator(this.selector);
    }

    async setValue(text: string) {
        console.log(`Selecting option: ${text}`);
        
        try {
            // Get the dropdown element
            const dropdown = await this.getDropdownElement();
            
            // Click to open dropdown
            await dropdown.click({ timeout: 5000 });
            
            // Wait for animation
            await this.page.waitForTimeout(300);
            
            // Select the option
            await this.page.locator('[role="option"]').filter({ hasText: text }).click({ timeout: 5000 });
            
            // For multiple select, close the dropdown with Escape
            if (await this.isMultipleChoice()) {
                await this.page.keyboard.press('Escape');
                // Wait for animation
                await this.page.waitForTimeout(300);
            }
        } catch (error) {
            console.error(`Failed to select option "${text}":`, error);
            throw error;
        }
    }

    async setMultipleValues(texts: string[]) {
        for (const text of texts) {
            await this.setValue(text);
        }
    }

    async getValue(): Promise<string> {
        const dropdown = await this.getDropdownElement();
        return await dropdown.innerText();
    }

    async getValues(): Promise<string[]> {
        const chips = this.page.locator('.MuiChip-label');
        const count = await chips.count();
        const values: string[] = [];
        
        for (let i = 0; i < count; i++) {
            values.push(await chips.nth(i).innerText());
        }
        
        return values;
    }
}
