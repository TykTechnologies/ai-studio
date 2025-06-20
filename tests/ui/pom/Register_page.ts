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
        this.nameInput = this.page.getByLabel('Name');
        this.emailInput = this.page.getByLabel('Email address');
        this.passwordInput = this.page.getByLabel('Password');
        this.signUpForAIDeveloperPortalCheckbox = this.page.getByLabel('Sign up for AI Portal');
        this.signUpForAIChatCheckbox = this.page.getByLabel('Sign up for AI Chats');
        this.registerButton = this.page.getByRole('button', { name: 'Sign up' });
    }

    async goto() {
        await this.page.goto(config.register_url);
    }

    async register(name: string, email: string, password: string) {
        await this.nameInput.fill(name);
        await this.emailInput.fill(email);
        await this.passwordInput.fill(password);
        await this.registerButton.click();
    }
}
