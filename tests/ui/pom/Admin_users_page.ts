import { Locator, Page } from '@playwright/test';
import { DropDownWrapper } from '@wrappers/DropDownWrapper';
import { TableWrapper } from '@wrappers/TableWrapper';
import { PageTemplate } from './Page_template';

interface UserParams {
    email: string;
    name: string;
    password: string;
    isAdmin: boolean;
    showChat: boolean;
    showPortal: boolean;
    emailVerified: boolean;
    notificationsEnabled: boolean;
}

export class AdminUsersPage extends PageTemplate {
    readonly Table: TableWrapper;
    readonly AddUserButton: Locator;
    readonly EmailInput: Locator;
    readonly NameInput: Locator;
    readonly PasswordInput: Locator;
    readonly IsAdminCheckbox: Locator;
    readonly ShowChatCheckbox: Locator;
    readonly ShowPortalCheckbox: Locator;
    readonly EmailVerifiedCheckbox: Locator;
    readonly NotificationsEnabledCheckbox: Locator;
    readonly CreateUserButton: Locator;
    readonly SaveUserButton: Locator; // Alias for CreateUserButton for backward compatibility
    readonly UpdateUserButton: Locator;
    readonly CancelButton: Locator;
    readonly RollAPIKeyButton: Locator;
    readonly APIKeyValue: Locator;
    readonly APIKeyCopyButton: Locator;

    constructor(page: Page) {
        super(page);
        this.Table = new TableWrapper('table', page);
        this.AddUserButton = this.page.getByRole('link', { name: 'Add user' });
        this.EmailInput = this.page.getByRole('textbox', { name: 'Email' });
        this.NameInput = this.page.getByRole('textbox', { name: 'Name' });
        this.PasswordInput = this.page.getByRole('textbox', { name: 'Password' });
        this.IsAdminCheckbox = this.page.getByRole('checkbox', { name: 'Is Admin' });
        this.ShowChatCheckbox = this.page.getByRole('checkbox', { name: 'Show Chat' });
        this.ShowPortalCheckbox = this.page.getByRole('checkbox', { name: 'Show Portal' });
        this.EmailVerifiedCheckbox = this.page.getByRole('checkbox', { name: 'Email Verified' });
        this.NotificationsEnabledCheckbox = this.page.getByRole('checkbox', { name: 'Notifications Enabled' });
        this.CreateUserButton = this.page.getByRole('button', { name: 'Create user' });
        this.SaveUserButton = this.page.getByRole('button', { name: 'Add user' });
        this.UpdateUserButton = this.page.getByRole('button', { name: 'Update user' });
        this.CancelButton = this.page.getByRole('button', { name: 'Cancel' });
        this.RollAPIKeyButton = this.page.getByRole('button', { name: 'Roll API Key' });
        this.APIKeyValue = this.page.locator('div:has-text("API Key") + div');
        this.APIKeyCopyButton = this.page.getByTestId('ContentCopyIcon');
    }

    async goto() {
        await this.page.goto('/admin/users');
    }

    async createUser(params: UserParams) {
        await this.AddUserButton.click();
        await this.EmailInput.fill(params.email);
        await this.NameInput.fill(params.name);
        await this.PasswordInput.fill(params.password);
        
        if (params.isAdmin) {
            await this.IsAdminCheckbox.check();
        } else {
            await this.IsAdminCheckbox.uncheck();
        }
        
        if (params.showChat) {
            await this.ShowChatCheckbox.check();
        } else {
            await this.ShowChatCheckbox.uncheck();
        }
        
        if (params.showPortal) {
            await this.ShowPortalCheckbox.check();
        } else {
            await this.ShowPortalCheckbox.uncheck();
        }
        
        if (params.emailVerified) {
            await this.EmailVerifiedCheckbox.check();
        } else {
            await this.EmailVerifiedCheckbox.uncheck();
        }
        
        if (params.notificationsEnabled) {
            await this.NotificationsEnabledCheckbox.check();
        } else {
            await this.NotificationsEnabledCheckbox.uncheck();
        }
        
        await this.CreateUserButton.click();
    }

    async updateUser(email: string, params: Partial<UserParams>) {
        await this.Table.clickRowByText(email);
        
        if (params.email) {
            await this.EmailInput.fill(params.email);
        }
        
        if (params.name) {
            await this.NameInput.fill(params.name);
        }
        
        if (params.password) {
            await this.PasswordInput.fill(params.password);
        }
        
        if (params.isAdmin !== undefined) {
            if (params.isAdmin) {
                await this.IsAdminCheckbox.check();
            } else {
                await this.IsAdminCheckbox.uncheck();
            }
        }
        
        if (params.showChat !== undefined) {
            if (params.showChat) {
                await this.ShowChatCheckbox.check();
            } else {
                await this.ShowChatCheckbox.uncheck();
            }
        }
        
        if (params.showPortal !== undefined) {
            if (params.showPortal) {
                await this.ShowPortalCheckbox.check();
            } else {
                await this.ShowPortalCheckbox.uncheck();
            }
        }
        
        if (params.emailVerified !== undefined) {
            if (params.emailVerified) {
                await this.EmailVerifiedCheckbox.check();
            } else {
                await this.EmailVerifiedCheckbox.uncheck();
            }
        }
        
        if (params.notificationsEnabled !== undefined) {
            if (params.notificationsEnabled) {
                await this.NotificationsEnabledCheckbox.check();
            } else {
                await this.NotificationsEnabledCheckbox.uncheck();
            }
        }
        
        await this.UpdateUserButton.click();
    }

    async deleteUser(email: string) {
        await this.Table.deleteRowWithText(email);
    }

    async rollAPIKey(email: string) {
        await this.Table.clickRowByText(email);
        await this.RollAPIKeyButton.click();
    }

    async getAPIKey() {
        await this.APIKeyCopyButton.click();
        const apiKey = await this.page.evaluate(() => navigator.clipboard.readText());
        return apiKey;
    }

    async expectUserCreated() {
        await this.expectPopupWithText('User created successfully');
    }

    async expectUserUpdated() {
        await this.expectPopupWithText('User updated successfully');
    }

    async expectUserDeleted() {
        await this.expectPopupWithText('User deleted successfully');
    }

    async expectAPIKeyRolled() {
        await this.expectPopupWithText('API key rolled successfully');
    }
}
