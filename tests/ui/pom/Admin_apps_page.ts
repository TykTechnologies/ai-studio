import { Locator, Page, expect } from '@playwright/test';
import { DropDownWrapper } from '@wrappers/DropDownWrapper';
import { TableWrapper } from '@wrappers/TableWrapper';
import { PageTemplate } from './Page_template';

interface AppParams {
    name: string;
    description: string;
    user: string;
    llms?: string[]; // Optional, allow multiple LLMs
    tools?: string[]; // Added for tools, optional, allow multiple
    monthlyBudget?: string; // Optional
    budgetStartDate?: string; // Optional
}

export class AdminAppsPage extends PageTemplate {
    readonly Table: TableWrapper;
    readonly AddAppButton: Locator;
    readonly NameInput: Locator;
    readonly DescriptionInput: Locator;
    readonly UserDropDown: DropDownWrapper;
    readonly LlmDropDown: DropDownWrapper;
    readonly ToolDropDown: DropDownWrapper; // Added for tools
    readonly MonthlyBudgetInput: Locator;
    readonly BudgetStartDateInput: Locator;
    readonly SaveButton: Locator; // More generic name for the save/submit button on the form
    readonly CancelButton: Locator;
    readonly ApproveThisAppButton: Locator;
    readonly KeyIdCopyButton: Locator;
    readonly SecretCopyButton: Locator;
    readonly AppDetailsToolsSection: Locator; // For verifying tools on details page


    constructor(page: Page) {
        super(page);
        this.Table = new TableWrapper('table', page);
        this.AddAppButton = this.page.locator('button:has-text("Add app")').first(); // Button to initiate adding an app
        
        // Form fields
        this.NameInput = this.page.getByRole('textbox', { name: 'Name' });
        this.DescriptionInput = this.page.getByRole('textbox', { name: 'Description' });
        this.UserDropDown = new DropDownWrapper('#mui-component-select-user_id', page); // Assuming specific ID, adjust if needed
        this.LlmDropDown = new DropDownWrapper('input[name="llm_ids"]', page); // More robust selector for CustomSelectMany
        this.ToolDropDown = new DropDownWrapper('input[name="tool_ids"]', page); // More robust selector for CustomSelectMany
        this.MonthlyBudgetInput = this.page.getByRole('textbox', { name: 'Monthly Budget' }); // Changed from spinbutton for flexibility
        this.BudgetStartDateInput = this.page.getByRole('textbox', { name: 'Budget Start Date' });
        this.SaveButton = this.page.getByRole('button', { name: /Add app|Update app/i }); // Regex for Add or Update app button
        this.CancelButton = this.page.getByRole('button', { name: 'Cancel' });

        // App Details specific elements
        this.ApproveThisAppButton = this.page.locator('button:has-text("Approve this app")');
        // Assuming Key ID and Secret are identifiable, e.g., by aria-label or a more specific structure
        this.KeyIdCopyButton = this.page.locator('[aria-label="Copy Key ID"] button, [data-testid="copy-key-id-button"]').first(); // Example more specific selectors
        this.SecretCopyButton = this.page.locator('[aria-label="Copy Secret"] button, [data-testid="copy-secret-button"]').first(); // Example
        this.AppDetailsToolsSection = this.page.locator('//div[h6[text()="App Information"]]//div[label[text()="Tools:"]]/following-sibling::div'); // XPath to find tools section on details page

    }

    async goto() {
        await this.page.goto('/admin/apps');
    }

    async createApp(params: AppParams) {
        await this.AddAppButton.click();
        await this.NameInput.fill(params.name);
        await this.DescriptionInput.fill(params.description);
        await this.UserDropDown.setValue(params.user);
        await this.page.keyboard.press('Escape');

        if (params.llms && params.llms.length > 0) {
            for (const llmName of params.llms) {
                await this.LlmDropDown.setValue(llmName); // CustomSelectMany might need selectValue
            }
        }
        if (params.tools && params.tools.length > 0) {
            for (const toolName of params.tools) {
                await this.ToolDropDown.setValue(toolName); // CustomSelectMany might need selectValue
            }
        }
        if (params.monthlyBudget) {
            await this.MonthlyBudgetInput.fill(params.monthlyBudget);
        }
        if (params.budgetStartDate) {
            await this.BudgetStartDateInput.fill(params.budgetStartDate);
        }
        await this.SaveButton.click();
    }
    
    async updateApp(params: Partial<AppParams>) {
        if (params.name) await this.NameInput.fill(params.name);
        if (params.description) await this.DescriptionInput.fill(params.description);
        if (params.user) {
            await this.UserDropDown.setValue(params.user);
            await this.page.keyboard.press('Escape');
        }
        
        // For multi-select, clearing existing and adding new ones might be needed
        // This depends on how CustomSelectMany handles updates.
        // Assuming CustomSelectMany handles re-selection correctly if values are just set.
        if (params.llms) {
            // May need to clear existing selections first if CustomSelectMany doesn't overwrite
            // await this.LlmDropDown.clear(); // Example, if a clear method exists
            for (const llmName of params.llms) {
                await this.LlmDropDown.setValue(llmName);
            }
        }
        if (params.tools) {
            // await this.ToolDropDown.clear(); // Example
            for (const toolName of params.tools) {
                await this.ToolDropDown.setValue(toolName);
            }
        }

        if (params.monthlyBudget !== undefined) {
            await this.MonthlyBudgetInput.fill(params.monthlyBudget || '');
        }
        if (params.budgetStartDate !== undefined) {
            await this.BudgetStartDateInput.fill(params.budgetStartDate || '');
        }
        await this.SaveButton.click();
    }


    async getKeyId(){
        await this.KeyIdCopyButton.click();
        const keyId = await this.page.evaluate(() => navigator.clipboard.readText());
        return keyId;
    }

    async getSecret(){
        await this.SecretCopyButton.click();
        const secret = await this.page.evaluate(() => navigator.clipboard.readText());
        return secret;
    }

    async expectPopupAppUpdated() {
       await this.expectPopupWithText('App updated successfully');
    }

    async expectPopupAppCreated() {
        await this.expectPopupWithText('App created successfully');
    }

    async expectPopupAppApproved() {
        await this.expectPopupWithText('App approved successfully');
    }

    async expectPopupAppDeleted() {
        await this.expectPopupWithText('App deleted successfully');
    }

    async getDisplayedTools(): Promise<string[]> {
        await this.AppDetailsToolsSection.waitFor({ state: 'visible', timeout: 5000 });
        const toolChips = this.AppDetailsToolsSection.locator('.MuiChip-label'); // Adjust selector for chips
        const toolNames: string[] = [];
        for (let i = 0; i < await toolChips.count(); i++) {
            toolNames.push(await toolChips.nth(i).innerText());
        }
        return toolNames;
    }
}
