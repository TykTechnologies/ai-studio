import { Wrapper } from "./Wrapper";
import { Locator, Page, expect } from '@playwright/test';

/**
 * Wraps a list table. Every admin list renders through the shared DataTable:
 * an optional search box above the table, sortable headers, an optional
 * selection checkbox column ("Select {name}" / "Select all on this page"),
 * a bulk-actions toolbar while something is selected, and an overflow menu
 * button per row labelled "Actions for {name}" in the last column.
 */
export class TableWrapper extends Wrapper {
    async getNumberOfRows(): Promise<number> {
        return await this.element.locator('tbody tr').count();
    }

    async clickRowByNumber(rowNumber: number): Promise<void> {
        await this.element.locator(`tbody tr:nth-child(${rowNumber})`).click();
    }

    async clickRowByText(text: string): Promise<void> {
        await this.element.locator(`tbody tr:has-text("${text}")`).click();
    }

    async rowWithTextExists(text: string): Promise<boolean> {
        return await this.element.locator(`tbody tr:has-text("${text}")`).isVisible();
    }

    async getRowNumberWithText(text: string): Promise<number> {
        const allRows = await this.element.locator('tbody tr').all();
        for (let i = 0; i < allRows.length; i++) {
            if (await allRows[i].locator(`:text("${text}")`).isVisible()) {
                return i + 1;
            }
        }
        throw new Error(`Row with text "${text}" not found`);
    }

    async expectRowWithTextExists(text: string): Promise<void> {
        return await expect(this.element.locator(`tbody tr:has-text("${text}")`).first()).toBeVisible();
    }

    async expectRowWithTextNotExists(text: string): Promise<void> {
        return await expect(this.element.locator(`tbody tr:has-text("${text}")`)).not.toBeVisible();
    }

    /**
     * The row's overflow-menu button. It is the last button in the row: the
     * selection checkbox is not a button, and Actions is the last column.
     */
    rowMenuButton(row: Locator): Locator {
        return row.locator('button').last();
    }

    async triggerAction(rowNumber: number, action: string) {
        await this.rowMenuButton(this.element.locator(`tbody tr:nth-child(${rowNumber})`)).click();
        await this.page.getByRole('menuitem', { name: action }).click();
    }

    async triggerEditAction(rowNumber: number) {
        await this.triggerAction(rowNumber, 'Edit');
    }

    async triggerDeleteAction(rowNumber: number) {
        await this.triggerAction(rowNumber, 'Delete');
    }

    async deleteRowWithText(text: string) {
        await this.rowMenuButton(this.element.locator(`tbody tr:has-text("${text}")`)).click();
        await this.page.getByRole('menuitem', { name: 'Delete' }).click();
    }

    async triggerActivateAction(rowNumber: number) {
        await this.triggerAction(rowNumber, 'Activate');
    }

    // --- DataTable v2: search, selection and bulk actions ---------------------

    /** The row's selection checkbox, labelled "Select {name}". */
    rowCheckbox(name: string): Locator {
        return this.page.getByRole('checkbox', { name: `Select ${name}`, exact: true });
    }

    async selectRow(name: string) {
        await this.rowCheckbox(name).check();
    }

    async deselectRow(name: string) {
        await this.rowCheckbox(name).uncheck();
    }

    async selectAllOnPage() {
        await this.page.getByRole('checkbox', { name: 'Select all on this page' }).check();
    }

    /** The "N selected" toolbar shown above the rows while something is selected. */
    bulkToolbar(): Locator {
        return this.page.getByTestId('bulk-actions-toolbar');
    }

    async expectSelectedCount(count: number) {
        await expect(this.page.getByTestId('bulk-selected-count')).toHaveText(`${count} selected`);
    }

    /** Presses a bulk action button (Activate, Deactivate, Delete) in the toolbar. */
    async triggerBulkAction(label: string) {
        await this.bulkToolbar().getByRole('button', { name: label, exact: true }).click();
    }

    async clearSelection() {
        await this.bulkToolbar().getByRole('button', { name: 'Clear' }).click();
    }

    /**
     * Confirms the "Delete N things?" dialog a bulk delete opens (a single
     * item titles "Delete {name}?"). The confirm button is "Delete".
     */
    async confirmBulkDelete() {
        const dialog = this.page.getByTestId('bulk-delete-dialog');
        await expect(dialog).toBeVisible();
        await dialog.getByRole('button', { name: 'Delete', exact: true }).click();
        await expect(dialog).not.toBeVisible();
    }
}
