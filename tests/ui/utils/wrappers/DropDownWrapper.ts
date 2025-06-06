import { Locator, Page } from '@playwright/test';

export class DropDownWrapper {
    readonly page: Page;
    readonly selector: string;

    constructor(selector: string, page: Page) {
        this.page = page;
        this.selector = selector;
    }

    async isMultipleChoice(): Promise<boolean> {
        try {
            // Get the dropdown element
            const dropdown = await this.getDropdownElement();
            const ariaMultiselectable = await dropdown.getAttribute('aria-multiselectable');
            
            // Also check for MUI's multiple class or data attributes
            const hasMultipleClass = await dropdown.evaluate(el => el.classList.contains('MuiSelect-multiple'));
            
            return ariaMultiselectable === 'true' || hasMultipleClass;
        } catch {
            // If we can't determine, assume single select for safety
            return false;
        }
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
            // Close any existing modal/dropdown that might be open
            await this.closeAnyOpenDropdowns();
            
            // Get the dropdown element
            const dropdown = await this.getDropdownElement();
            
            // Wait for dropdown to be visible and interactable
            await dropdown.waitFor({ state: 'visible', timeout: 10000 });
            
            // Click to open dropdown - use force click to bypass backdrop
            await dropdown.click({ force: true, timeout: 10000 });
            
            // Wait for options to appear
            await this.page.waitForSelector('[role="option"]', { timeout: 10000 });
            
            // Wait for animation
            await this.page.waitForTimeout(300);
            
            // Find and click the option
            const option = this.page.locator('[role="option"]').filter({ hasText: text });
            await option.waitFor({ state: 'visible', timeout: 10000 });
            
            // Scroll option into view if needed and click
            await option.scrollIntoViewIfNeeded();
            await option.click({ force: true, timeout: 10000 });
            
            // For multiple select, close the dropdown with Escape
            if (await this.isMultipleChoice()) {
                await this.page.keyboard.press('Escape');
                // Wait for animation and verify dropdown is closed
                await this.waitForDropdownToClose();
            }
        } catch (error) {
            console.error(`Failed to select option "${text}":`, error);
            throw error;
        }
    }

    private async closeAnyOpenDropdowns() {
        // Press escape a couple times to ensure any existing dropdowns are closed
        await this.page.keyboard.press('Escape');
        await this.page.waitForTimeout(200);
        await this.page.keyboard.press('Escape');
        await this.page.waitForTimeout(200);
    }

    private async waitForDropdownToClose() {
        // Wait for the dropdown menu to close
        try {
            await this.page.waitForSelector('[role="listbox"]', { state: 'hidden', timeout: 3000 });
        } catch {
            // If listbox doesn't exist or timeout, just wait a bit more
            await this.page.waitForTimeout(300);
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

    async removeValue(text: string) {
        console.log(`Removing option: ${text}`);
        
        try {
            // Find the chip with the specified text and click its delete icon
            const chip = this.page.locator('.MuiChip-root').filter({ hasText: text });
            await chip.waitFor({ state: 'visible', timeout: 10000 });
            
            // Look for the delete icon (typically a CancelIcon or similar)
            const deleteIcon = chip.locator('[data-testid="CancelIcon"], .MuiChip-deleteIcon');
            await deleteIcon.waitFor({ state: 'visible', timeout: 10000 });
            await deleteIcon.click({ force: true });
            
            // Wait for animation
            await this.page.waitForTimeout(300);
        } catch (error) {
            console.error(`Failed to remove option "${text}":`, error);
            throw error;
        }
    }
}
