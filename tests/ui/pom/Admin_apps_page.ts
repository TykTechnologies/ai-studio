import { Locator, Page, expect } from '@playwright/test';
import { DropDownWrapper } from '@wrappers/DropDownWrapper';
import { TableWrapper } from '@wrappers/TableWrapper';
import { PageTemplate } from './Page_template';

interface AppParams {
    name: string;
    description: string;
    user: string;
    llm: string;
    tools?: string[]; // Added for tools, optional
    monthlyBudget: string;
    budgetStartDate: string;
}

export class AdminAppsPage extends PageTemplate {
    readonly Table: TableWrapper;
    readonly AddAppButton: Locator;
    readonly NameInput: Locator;
    readonly DescriptionInput: Locator;
    readonly UserDropDown: DropDownWrapper;
    // LLMs / data sources / tools are RelationshipPickers (compact): selected
    // items are chips, the add control is the Autocomplete labelled
    // "Add LLM provider" / "Add data source" / "Add tool", and removal is
    // the chip's "Remove {name}" icon. Changes persist when the form is saved.
    readonly AddLlmInput: Locator;
    readonly AddDataSourceInput: Locator;
    readonly AddToolInput: Locator;
    readonly MonthlyBudgetMode: Locator;
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
        this.AddLlmInput = this.page.getByRole('combobox', { name: 'Add LLM provider' });
        this.AddDataSourceInput = this.page.getByRole('combobox', { name: 'Add data source' });
        this.AddToolInput = this.page.getByRole('combobox', { name: 'Add tool' });
        // The budget field is a mode choice ("No limit"/"Default" vs "Fixed
        // amount"); the amount box only appears for a fixed amount.
        this.MonthlyBudgetMode = this.page.getByRole('combobox', { name: /^Monthly budget/ });
        this.MonthlyBudgetInput = this.page.getByTestId('app-budget-amount');
        this.BudgetStartDateInput = this.page.getByRole('textbox', { name: 'Budget Start Date' });
        this.SaveButton = this.page.getByRole('button', { name: 'Add app' });
        this.CancelButton = this.page.getByRole('button', { name: 'Cancel' });

        // App Details specific elements
        this.ApproveThisAppButton = this.page.locator('button:has-text("Approve this app")');
        this.KeyIdCopyButton = this.page.getByTestId('ContentCopyIcon').first();
        this.SecretCopyButton = this.page.getByTestId('ContentCopyIcon').nth(1);
        this.AppDetailsToolsSection = this.page.locator('//div[h6[text()="App Information"]]//div[label[text()="Tools:"]]/following-sibling::div'); // XPath to find tools section on details page

    }

    async goto() {
        await this.page.goto('/admin/apps');
    }

    /**
     * Asserts the Status cell of the named app's row. Values: "Active",
     * "Awaiting approval" (credential exists, not yet approved), "No credential"
     * and "Disabled" (the app's live switch is off) -- the same words the portal uses.
     */
    async expectAppStatus(appName: string, status: string | RegExp) {
        await expect(this.Table.element.locator(`tbody tr:has-text("${appName}")`).first()).toContainText(status);
    }

    /** Asserts the Status line on the app detail page (same words as the list). */
    async expectDetailStatus(status: string) {
        await expect(this.page.getByTestId('app-status')).toHaveText(status);
    }

    /** Approves the app's credentials from the list's row menu. */
    async approveCredentialsFromList(appName: string) {
        await this.Table.element.locator(`tbody tr:has-text("${appName}") button`).click();
        await this.page.getByRole('menuitem', { name: 'Approve credentials' }).click();
    }

    /** Chip for a member currently selected in any of the form's pickers. */
    relationshipChip(name: string): Locator {
        return this.page.getByTestId('relationship-picker').locator('.MuiChip-root').filter({ hasText: name });
    }

    /** Type into a picker's Autocomplete and pick the option; it becomes a chip at once. */
    private async pickRelationship(input: Locator, name: string) {
        await input.click();
        await input.fill(name);
        await this.page.getByRole('option', { name, exact: true }).click();
        await this.relationshipChip(name).waitFor();
    }

    async addLlm(name: string) {
        await this.pickRelationship(this.AddLlmInput, name);
    }

    async addDataSource(name: string) {
        await this.pickRelationship(this.AddDataSourceInput, name);
    }

    async addTool(name: string) {
        await this.pickRelationship(this.AddToolInput, name);
    }

    /** Remove a member via its chip's delete icon (any picker on the form). */
    async removeRelationship(name: string) {
        await this.page.getByLabel(`Remove ${name}`).click();
        await this.relationshipChip(name).waitFor({ state: 'detached' });
    }

    async addApp(params: AppParams) {
        await this.AddAppButton.click();
        await this.NameInput.fill(params.name);
        await this.DescriptionInput.fill(params.description);
        await this.UserDropDown.setValue(params.user);
        await this.addLlm(params.llm);
        if (params.tools && params.tools.length > 0) {
            for (const toolName of params.tools) {
                await this.addTool(toolName);
            }
        }
        await this.setMonthlyBudget(params.monthlyBudget);
        await this.BudgetStartDateInput.fill(params.budgetStartDate);
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


    // setMonthlyBudget picks "Fixed amount" and fills the amount.
    async setMonthlyBudget(amount: string) {
        await this.MonthlyBudgetMode.click();
        await this.page.getByRole('option', { name: 'Fixed amount' }).click();
        await this.MonthlyBudgetInput.fill(amount);
    }
}
