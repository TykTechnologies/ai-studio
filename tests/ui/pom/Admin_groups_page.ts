import { Locator, Page } from '@playwright/test';
import { TableWrapper } from '@wrappers/TableWrapper';
import { PageTemplate } from './Page_template';

interface GroupParams {
    name: string;
}

/**
 * Teams list and form (/admin/groups).
 *
 * Every membership control on the team form and in the "Manage team members"
 * / "Manage Catalogs" modals is the shared RelationshipPicker:
 * - members use the dual variant: the search box is labelled "Add user", each
 *   available row has an "Add {name}" button and each current row a
 *   "Remove {name}" button;
 * - catalogs use the compact variant: chips carry the catalog name, the add
 *   control is the Autocomplete labelled "Add LLM catalog" / "Add data catalog"
 *   / "Add tool catalog", and removal is the chip's "Remove {name}" icon.
 * Changes only persist when the form or modal is saved.
 */
export class AdminGroupsPage extends PageTemplate {
    readonly Table: GroupsTable;
    readonly AddGroupButton: Locator;
    readonly GroupNameInput: Locator;
    readonly CreateGroupButton: Locator;
    readonly UpdateGroupButton: Locator;
    readonly DeleteGroupButton: Locator;
    readonly ManageTeamMembersSection: Locator;
    readonly AddCatalogsSection: Locator;
    readonly AddUserSearchInput: Locator;
    readonly AddLlmCatalogInput: Locator;
    readonly AddDataCatalogInput: Locator;
    readonly AddToolCatalogInput: Locator;
    readonly ModalSaveButton: Locator;
    readonly ModalCancelButton: Locator;

    constructor(page: Page) {
        super(page);
        this.Table = new GroupsTable('table', page);
        this.AddGroupButton = this.page.getByRole('link', { name: 'Add team' });
        this.GroupNameInput = this.page.getByRole('textbox', { name: 'Team name' });
        this.CreateGroupButton = this.page.getByRole('button', { name: 'Create team' });
        this.UpdateGroupButton = this.page.getByRole('button', { name: 'Update team' });
        this.DeleteGroupButton = this.page.getByRole('button', { name: 'Delete team' });
        this.ManageTeamMembersSection = this.page.getByText('Manage team members', { exact: true });
        this.AddCatalogsSection = this.page.getByText('Add catalogs', { exact: true });
        this.AddUserSearchInput = this.page.getByRole('textbox', { name: 'Add user' });
        this.AddLlmCatalogInput = this.page.getByRole('combobox', { name: 'Add LLM catalog' });
        this.AddDataCatalogInput = this.page.getByRole('combobox', { name: 'Add data catalog' });
        this.AddToolCatalogInput = this.page.getByRole('combobox', { name: 'Add tool catalog' });
        this.ModalSaveButton = this.page.getByRole('button', { name: 'Save', exact: true });
        this.ModalCancelButton = this.page.getByRole('button', { name: 'Cancel', exact: true });
    }

    async goto() {
        await this.page.goto('/admin/groups');
    }

    async addGroup(params: GroupParams) {
        await this.AddGroupButton.click();
        await this.GroupNameInput.fill(params.name);
        await this.CreateGroupButton.click();
    }

    /** Expand a collapsed form section by its title. */
    async expandSection(section: Locator) {
        await section.click();
    }

    // --- members (dual picker) ---------------------------------------------

    /** Search the available users and press the row's "Add {name}" button. */
    async addUserToTeam(name: string) {
        await this.AddUserSearchInput.fill(name);
        await this.page.getByRole('button', { name: `Add ${name}`, exact: true }).click();
        await this.page.getByRole('button', { name: `Remove ${name}`, exact: true }).waitFor();
    }

    /** Press the current member's "Remove {name}" button. */
    async removeUserFromTeam(name: string) {
        await this.page.getByRole('button', { name: `Remove ${name}`, exact: true }).click();
    }

    // --- catalogs (compact pickers) ----------------------------------------

    private async pickCatalog(input: Locator, catalog: string) {
        await input.click();
        await input.fill(catalog);
        await this.page.getByRole('option', { name: catalog, exact: true }).click();
        await this.catalogChip(catalog).waitFor();
    }

    /** Chip for a catalog currently assigned to the team (present only while selected). */
    catalogChip(catalog: string): Locator {
        return this.page.getByTestId('relationship-picker').locator('.MuiChip-root').filter({ hasText: catalog });
    }

    async addLlmCatalogToTeam(catalog: string) {
        await this.pickCatalog(this.AddLlmCatalogInput, catalog);
    }

    async addDataCatalogToTeam(catalog: string) {
        await this.pickCatalog(this.AddDataCatalogInput, catalog);
    }

    async addToolCatalogToTeam(catalog: string) {
        await this.pickCatalog(this.AddToolCatalogInput, catalog);
    }

    async removeCatalogFromTeam(catalog: string) {
        await this.page.getByLabel(`Remove ${catalog}`).click();
        await this.catalogChip(catalog).waitFor({ state: 'detached' });
    }
}

class GroupsTable extends TableWrapper {
    constructor(locator: string, page: Page) {
        super(locator, page);
    }

    async triggerManageMembersAction(rowNumber: number) {
        await this.triggerAction(rowNumber, 'Manage team members');
    }

    async triggerManageCatalogsAction(rowNumber: number) {
        await this.triggerAction(rowNumber, 'Manage catalogs');
    }

    async triggerEditAction(rowNumber: number) {
        await this.triggerAction(rowNumber, 'Edit team');
    }
}
