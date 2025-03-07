import { Locator, Page } from '@playwright/test';
import { config } from '../config';

export class RegisterPage {
    readonly page: Page;
    readonly nameInput: Locator;
    readonly emailInput: Locator;
    readonly passwordInput: Locator;
    readonly signUpForAIDeveloperPortalCheckbox: Locator;
    readonly signUpForAIChatCheckbox: Locator;
    readonly registerButton: Locator;

    constructor(page: Page) {
        this.page = page;
        this.nameInput = this.page.getByRole('textbox', { name: 'Name' });
        this.emailInput = this.page.getByRole('textbox', { name: 'Email' });
        this.passwordInput = this.page.getByRole('textbox', { name: 'Password' });
        this.signUpForAIDeveloperPortalCheckbox = this.page.getByRole('checkbox', { name: 'Sign up for AI Developer' });
        this.signUpForAIChatCheckbox = this.page.getByRole('checkbox', { name: 'Sign up for AI Chat' });
        this.registerButton = this.page.getByRole('button', { name: 'Register' });
    }

    async goto() {
        await this.page.goto(config.register_url);
    }

    async register(name: string, email: string, password: string) {
        await this.nameInput.fill(name);
        await this.emailInput.fill(email);
        await this.passwordInput.fill(password);
        await this.registerButton.click();
        await this.registerButton.click();

    }
}
