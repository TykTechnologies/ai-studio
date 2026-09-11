import { Locator, Page, expect } from '@playwright/test';
import { TableWrapper } from '@wrappers/TableWrapper';
import { PageTemplate } from './Page_template';

/**
 * Access → Roles (Enterprise). Roles are listed in a table; system roles can
 * only be cloned, custom roles can be edited and deleted.
 */
export class AdminRolesPage extends PageTemplate {
    readonly Table: TableWrapper;
    readonly AddRoleButton: Locator;
    readonly RoleNameInput: Locator;
    readonly RoleDescriptionInput: Locator;
    readonly CreateRoleButton: Locator;
    readonly UpdateRoleButton: Locator;
    readonly CloneDialogNameInput: Locator;
    readonly CloneDialogConfirmButton: Locator;
    readonly PermissionMatrix: Locator;

    constructor(page: Page) {
        super(page);
        this.Table = new TableWrapper('table', page);
        this.AddRoleButton = this.page.getByRole('button', { name: 'Add role' });
        this.RoleNameInput = this.page.getByTestId('role-name');
        this.RoleDescriptionInput = this.page.getByTestId('role-description');
        this.CreateRoleButton = this.page.getByRole('button', { name: 'Create role' });
        this.UpdateRoleButton = this.page.getByRole('button', { name: 'Update role' });
        this.CloneDialogNameInput = this.page.getByTestId('clone-role-name');
        this.CloneDialogConfirmButton = this.page.getByRole('button', { name: 'Clone', exact: true });
        this.PermissionMatrix = this.page.getByTestId('permission-matrix');
    }

    async goto() {
        await this.gotoAdminPath('/admin/roles');
        await expect(this.pageTitle('Roles')).toBeVisible();
    }

    /**
     * Page titles are styled Typography rather than heading elements, and the
     * title is the first occurrence of the text in the main region (a role
     * detail page repeats the name in a badge below it).
     */
    pageTitle(text: string): Locator {
        return this.page.getByRole('main').getByText(text, { exact: true }).first();
    }

    /** Opens the row menu of the named role. */
    async openRowMenu(roleName: string) {
        await this.page.getByLabel(`Actions for ${roleName}`, { exact: true }).click();
    }

    /** Clones a role through the row menu and lands on the copy's edit form. */
    async cloneRole(sourceName: string, newName: string) {
        await this.openRowMenu(sourceName);
        await this.page.getByRole('menuitem', { name: 'Clone role' }).click();
        await this.CloneDialogNameInput.fill(newName);
        await this.CloneDialogConfirmButton.click();
        await expect(this.pageTitle('Edit role')).toBeVisible();
    }

    /** Ticks a single permission checkbox in the matrix, e.g. "tags:write". */
    async grant(permission: string) {
        await this.PermissionMatrix.getByLabel(permission, { exact: true }).check();
    }

    /** Deletes a custom role through the row menu, confirming the dialog. */
    async deleteRole(roleName: string) {
        await this.openRowMenu(roleName);
        await this.page.getByRole('menuitem', { name: 'Delete role' }).click();
        await this.page.getByRole('button', { name: 'Delete role' }).click();
        await this.Table.expectRowWithTextNotExists(roleName);
    }
}
