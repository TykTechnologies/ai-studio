import { Locator, Page } from '@playwright/test';
import { config } from '../config';

export class RegisterPage {
    readonly page: Page;
    readonly NameInput: Locator;
    readonly EmailInput: Locator;
    readonly PasswordInput: Locator;
    readonly SignUpForAIDeveloperPortalCheckbox: Locator;
    readonly SignUpForAIChatCheckbox: Locator;
    readonly RegisterButton: Locator;

    constructor(page: Page) {
        this.page = page;
        this.NameInput = this.page.getByRole('textbox', { name: 'Name' });
        this.EmailInput = this.page.getByRole('textbox', { name: 'Email' });
        this.PasswordInput = this.page.getByRole('textbox', { name: 'Password' });
        this.SignUpForAIDeveloperPortalCheckbox = this.page.getByRole('checkbox', { name: 'Sign up for AI Developer' });
        this.SignUpForAIChatCheckbox = this.page.getByRole('checkbox', { name: 'Sign up for AI Chat' });
        this.RegisterButton = this.page.getByRole('button', { name: 'Register' });
    }

    async goto() {
        await this.page.goto(config.register_url);
    }

    async register(name: string, email: string, password: string) {
        await this.NameInput.fill(name);
        await this.EmailInput.fill(email);
        await this.PasswordInput.fill(password);
        await this.RegisterButton.click();
        await this.RegisterButton.click();
    }
}
