import { Locator, Page } from '@playwright/test';
import { DropDownWrapper } from '@wrappers/DropDownWrapper';
import { TableWrapper } from '@wrappers/TableWrapper';
import { PageTemplate } from './Page_template';

interface UserParams {
    name: string;
    email: string;
    password: string;
    isAdmin: boolean;
    showPortal: boolean;
    showChat: boolean;
    emailVerified: boolean;
    enableNotifications: boolean;
}

export class AdminUsersPage extends PageTemplate {
    readonly Table: TableWrapper;
    readonly AddUserButton: Locator;
    readonly SaveUserButton: Locator;
    readonly NameInput: Locator;
    readonly EmailInput: Locator;
    readonly PasswordInput: Locator;
    readonly AdminUserCheckbox: Locator;
    readonly ShowPortalCheckbox: Locator;
    readonly ShowChatCheckbox: Locator;
    readonly EmailVerifiedCheckbox: Locator;
    readonly EnableNotificationsCheckbox: Locator;
    readonly AddToGroupButton: Locator;
    readonly GroupDropDown: DropDownWrapper;
    readonly UpdateUserButton: Locator;
    readonly BackToUsersLink: Locator;

    constructor(page: Page) {
        super(page);
        this.Table = new TableWrapper('table', page);
        this.AddUserButton = this.page.getByRole('link', { name: 'Add user' });
        this.SaveUserButton = this.page.getByRole('button', { name: 'Add user' });
        this.NameInput = this.page.getByRole('textbox', { name: 'Name' });
        this.EmailInput = this.page.getByRole('textbox', { name: 'Email' });
        this.PasswordInput = this.page.getByRole('textbox', { name: 'Password' });
        this.AdminUserCheckbox = this.page.getByRole('checkbox', { name: 'Admin User' });
        this.ShowPortalCheckbox = this.page.getByRole('checkbox', { name: 'Show Portal' });
        this.ShowChatCheckbox = this.page.getByRole('checkbox', { name: 'Show Chat' });
        this.EmailVerifiedCheckbox = this.page.getByRole('checkbox', { name: 'Email Verified' });
        this.EnableNotificationsCheckbox = this.page.getByRole('checkbox', { name: 'Enable Notifications' });
        this.AddToGroupButton = this.page.getByRole('button', { name: 'Add to Group' });
        this.GroupDropDown = new DropDownWrapper('combobox', page);
        this.UpdateUserButton = this.page.getByRole('button', { name: 'Update user' });
        this.BackToUsersLink = this.page.getByRole('link', { name: 'Back to users' });
    }

    async goto() {
        await this.page.goto('/admin/users');
    }

    async addUser(params: UserParams) {
        await this.AddUserButton.click();
        await this.NameInput.fill(params.name);
        await this.EmailInput.fill(params.email);
        await this.PasswordInput.fill(params.password);
        if (params.isAdmin) await this.AdminUserCheckbox.check();
        if (!params.showPortal) await this.ShowPortalCheckbox.uncheck();
        if (!params.showChat) await this.ShowChatCheckbox.uncheck();
        if (params.emailVerified) await this.EmailVerifiedCheckbox.check();
        if (params.enableNotifications) await this.EnableNotificationsCheckbox.check();
        await this.AddUserButton.click();
    }

    async addToGroup(groupName: string) {
        await this.GroupDropDown.setValue(groupName);
        await this.AddToGroupButton.click();
    }

    async updateUser() {
        await this.UpdateUserButton.click();
    }

    async backToUsers() {
        await this.BackToUsersLink.click();
    }

    async deleteUser(rowNumber: number) {
        await this.Table.triggerDeleteAction(rowNumber);
    }

    async editUser(rowNumber: number) {
        await this.Table.triggerEditAction(rowNumber);
    }
}
