import { Locator, Page } from '@playwright/test';
import { DropDownWrapper } from '@wrappers/DropDownWrapper';
import { TableWrapper } from '@wrappers/TableWrapper';
import { PageTemplate } from './Page_template';

interface CatalogueParams {
    name: string;
    llm: string;
}

interface GroupParams {
    name: string;
}

export class AdminGroupsPage extends PageTemplate {
    readonly Table: GroupsTable;
    readonly AddGroupButton: Locator;
    readonly GroupNameInput: Locator;
    readonly CreateGroupButton: Locator;
    readonly UpdateGroupButton: Locator;
    readonly DeleteGroupButton: Locator;
    readonly AddLlmCatalogueToGroupButton: Locator;
    readonly AddDataCatalogueToGroupButton: Locator;
    readonly AddUserToGroupButton: Locator;
    readonly AddToolCatalogueToGroupButton: Locator;
    readonly SelectDropDown: DropDownWrapper;

    constructor(page: Page) {
        super(page);
        this.Table = new GroupsTable('table', page);
        this.AddGroupButton = this.page.getByRole('link', { name: 'Add group' });
        this.GroupNameInput = this.page.getByRole('textbox', { name: 'Group Name' });
        this.CreateGroupButton = this.page.getByRole('button', { name: 'Create group' });
        this.UpdateGroupButton = this.page.getByRole('button', { name: 'Update group' });
        this.DeleteGroupButton = this.page.getByRole('button', { name: 'Delete' });
        this.AddLlmCatalogueToGroupButton = this.page.getByRole('menuitem', { name: 'Add LLM catalogue to group' });
        this.AddDataCatalogueToGroupButton = this.page.getByRole('menuitem', { name: 'Add data catalogue to group' });
        this.AddUserToGroupButton = this.page.getByRole('menuitem', { name: 'Add user to group' });
        this.AddToolCatalogueToGroupButton = this.page.getByRole('menuitem', { name: 'Add tool catalogue to group' });
        this.SelectDropDown = new DropDownWrapper('combobox', page);
    }

    async goto() {
        await this.page.goto('/admin/groups');
    }

    async addGroup(params: GroupParams) {
        await this.AddGroupButton.click();
        await this.GroupNameInput.fill(params.name);
        await this.CreateGroupButton.click();
    }
}

class GroupsTable extends TableWrapper {
    constructor(locator: string, page: Page) {
        super(locator, page);
    }

    async triggerAddUserAction(rowNumber: number) {
        await this.triggerAction(rowNumber, 'Add user to group');
    }

    async triggerAddLlmCatalogueAction(rowNumber: number) {
        await this.triggerAction(rowNumber, 'Add LLM catalogue to group');
    }

    async triggerAddDataCatalogueAction(rowNumber: number) {
        await this.triggerAction(rowNumber, 'Add data catalogue to group');
    }

    async triggerAddToolCatalogueAction(rowNumber: number) {
        await this.triggerAction(rowNumber, 'Add tool catalogue to group');
    }

    async triggerEditAction(rowNumber: number) {
        await this.triggerAction(rowNumber, 'Edit group');
    }
}
