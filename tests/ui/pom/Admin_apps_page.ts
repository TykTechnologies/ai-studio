import { Locator, Page, expect } from '@playwright/test';
import { DrowDownWrapper } from '@wrappers/DropDownWrapper';
import { TableWrapper } from '@wrappers/TableWrapper';
import { PageTemplate } from './Page_template';

interface AppParams {
    name: string;
    description: string;
    user: string;
    llm: string;
    monthlyBudget: string;
    budgetStartDate: string;
}

export class AdminAppsPage extends PageTemplate {
    readonly Table: TableWrapper;
    readonly AddAppButton: Locator;
    readonly NameInput: Locator;
    readonly DescriptionInput: Locator;
    readonly UserDropDown: DrowDownWrapper;
    readonly LlmDropDown: DrowDownWrapper;
    readonly MonthlyBudgetInput: Locator;
    readonly BudgetStartDateInput: Locator;
    readonly SaveButton: Locator;
    readonly CancelButton: Locator;
    readonly ApproveThisAppButton: Locator;
    readonly KeyIdCopyButton: Locator;
    readonly SecretCopyButton: Locator;

    constructor(page: Page) {
        super(page);
        this.Table = new TableWrapper('table', page);
        this.AddAppButton = this.page.locator('button:has-text("Add app")').first();
        this.NameInput = this.page.getByRole('textbox', { name: 'Name' });
        this.DescriptionInput = this.page.getByRole('textbox', { name: 'Description' });
        this.UserDropDown = new DrowDownWrapper('#mui-component-select-user_id', page);
        this.LlmDropDown = new DrowDownWrapper('#mui-component-select-llm_ids', page);
        this.MonthlyBudgetInput = this.page.getByRole('spinbutton', { name: 'Monthly Budget' });
        this.BudgetStartDateInput = this.page.getByRole('textbox', { name: 'Budget Start Date' });
        this.SaveButton = this.page.getByRole('button', { name: 'Add app' });
        this.CancelButton = this.page.getByRole('button', { name: 'Cancel' });
        this.ApproveThisAppButton = this.page.locator('button:has-text("Approve this app")');
        this.KeyIdCopyButton = this.page.getByTestId('ContentCopyIcon').first();
        this.SecretCopyButton = this.page.getByTestId('ContentCopyIcon').nth(1);
    }

    async goto() {
        await this.page.goto('/admin/apps');
    }

    async addApp(params: AppParams) {
        await this.AddAppButton.click();
        await this.NameInput.fill(params.name);
        await this.DescriptionInput.fill(params.description);
        await this.UserDropDown.setValue(params.user);
        await this.LlmDropDown.setValue(params.llm);
        await this.MonthlyBudgetInput.fill(params.monthlyBudget);
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
}
