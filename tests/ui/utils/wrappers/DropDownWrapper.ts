import { Locator, Page } from '@playwright/test';

export class DropDownWrapper {
    readonly element: Locator;
    readonly page: Page;

    constructor(page: Page, selector: string) {
        this.page = page;
        this.element = page.locator(selector);
    }

    async isMultipleChoice(): Promise<boolean> {
        return (await this.element.getAttribute('aria-multiselectable')) === 'true';
    }

    async setValue(text: string) {
        console.log(`Selecting option: ${text}`);
        
        // Get the field name from the selector (e.g., 'llm_ids' from 'input[name="llm_ids"]')
        const fieldName = this.element.toString().match(/name="([^"]+)"/)?.[1] || '';
        
        // Find the visible dropdown element (the div with role="combobox")
        const visibleDropdown = this.page.locator(`#mui-component-select-${fieldName}`);
        
        try {
            // Click on the visible dropdown element to open the options
            await visibleDropdown.click({ timeout: 5000 });
            
            // Wait a moment for the dropdown to fully open
            await this.page.waitForTimeout(300);
            
            // Click the option with the specified text
            await this.page.locator('[role="option"]').filter({ hasText: text }).click({ timeout: 5000 });
            
            // For multiple select, close the dropdown with Escape
            if (await this.isMultipleChoice()) {
                await this.page.keyboard.press('Escape');
                // Wait for the dropdown to close
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
}
