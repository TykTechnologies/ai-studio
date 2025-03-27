import { Wrapper } from "./Wrapper";
import { Locator, Page, expect } from '@playwright/test';

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

    async expectRowWithTextExists(text: string): Promise<void> {
        return await expect(this.element.locator(`tbody tr:has-text("${text}")`)).toBeVisible();
    }

    async expectRowWithTextNotExists(text: string): Promise<void> {
        return await expect(this.element.locator(`tbody tr:has-text("${text}")`)).not.toBeVisible();
    }

    async triggerAction(rowNumber: number, action: string) {
        await this.element.locator(`tbody tr:nth-child(${rowNumber}) button`).click();
        await this.page.getByRole('menuitem', { name: action }).click();
    }

    async triggerEditAction(rowNumber: number) {
        await this.triggerAction(rowNumber, 'Edit');
    }

    async triggerDeleteAction(rowNumber: number) {
        await this.triggerAction(rowNumber, 'Delete');
    }

    async deleteRowWithText(text: string) {
        await this.element.locator(`tbody tr:has-text("${text}") button`).click();
        await this.page.getByRole('menuitem', { name: 'Delete' }).click();
    }

    async triggerActivateAction(rowNumber: number) {
        await this.triggerAction(rowNumber, 'Activate');
    }
}

