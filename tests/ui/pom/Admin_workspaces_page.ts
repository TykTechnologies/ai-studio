import { Locator, Page, expect } from '@playwright/test';
import { PageTemplate } from './Page_template';

/**
 * Access → Workspaces (Enterprise): the list, the add/edit form and the
 * detail page with its Access (scoped role assignments) and Objects tabs.
 */
export class AdminWorkspacesPage extends PageTemplate {
    readonly AddWorkspaceButton: Locator;
    readonly NameInput: Locator;
    readonly DescriptionInput: Locator;
    readonly SaveNewButton: Locator;
    readonly Table: Locator;
    readonly AccessTab: Locator;
    readonly ObjectsTab: Locator;
    readonly AccessTable: Locator;
    readonly AddSubjectButton: Locator;

    constructor(page: Page) {
        super(page);
        this.AddWorkspaceButton = this.page.getByRole('button', { name: 'Add Workspace' });
        this.NameInput = this.page.getByTestId('workspace-name');
        this.DescriptionInput = this.page.getByTestId('workspace-description');
        this.SaveNewButton = this.page.getByRole('button', { name: 'Add Workspace' });
        this.Table = this.page.getByTestId('workspaces-table');
        this.AccessTab = this.page.getByRole('tab', { name: 'Access' });
        this.ObjectsTab = this.page.getByRole('tab', { name: 'Objects' });
        this.AccessTable = this.page.getByTestId('workspace-access-table');
        this.AddSubjectButton = this.page.getByRole('button', { name: 'Add user or team' });
    }

    async goto() {
        await this.gotoAdminPath('/admin/workspaces');
        await expect(this.Table).toBeVisible();
    }

    /** Creates a Workspace through the form and returns to the list. */
    async addWorkspace(name: string, description: string) {
        await this.goto();
        await this.AddWorkspaceButton.click();
        await this.NameInput.fill(name);
        await this.DescriptionInput.fill(description);
        await this.SaveNewButton.click();
        await expect(this.page).not.toHaveURL(/\/admin\/workspaces\/new/);
        await this.goto();
        await expect(this.Table.getByRole('row').filter({ hasText: name })).toBeVisible();
    }

    async openWorkspace(name: string) {
        await this.goto();
        await this.Table.getByRole('row').filter({ hasText: name }).first().click();
        await expect(this.AccessTab).toBeVisible();
    }

    /**
     * Access tab: assigns roles to a user (or service account) inside the
     * Workspace. userLabel is the option text the user picker shows.
     */
    async assignUserRoles(userLabel: string | RegExp, roleIds: number[]) {
        await this.AccessTab.click();
        await this.AddSubjectButton.click();
        const dialog = this.page.getByRole('dialog');
        await dialog.getByTestId('subject-select').locator('..').click();
        await this.page.getByRole('option', { name: userLabel }).click();
        await dialog.getByTestId('role-select-input').locator('..').click();
        for (const id of roleIds) {
            await this.page.getByTestId(`role-option-${id}`).click();
        }
        await this.page.keyboard.press('Escape');
        await dialog.getByRole('button', { name: 'Save' }).click();
        await expect(dialog).not.toBeVisible();
    }

    accessRowForUser(userId: number): Locator {
        return this.page.getByTestId(`access-row-user-${userId}`);
    }
}
