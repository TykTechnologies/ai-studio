import { Locator, Page } from '@playwright/test';
import { config } from '../config';

export class LoginPage {
    readonly page: Page;
    readonly emailInput: Locator;
    readonly passwordInput: Locator;
    readonly loginButton: Locator;
    readonly registerHereButton: Locator;
    readonly forgotPasswordButton: Locator;

    constructor(page: Page) {
        this.page = page;
        this.emailInput = this.page.getByLabel('Email address');
        this.passwordInput = this.page.getByLabel('Password');
        this.loginButton = this.page.getByRole('button', { name: 'Log in' });
        this.registerHereButton = this.page.getByText('Sign up');
        this.forgotPasswordButton = this.page.getByText('Forgot password?');
    }
    
    async goto() {
        await this.page.goto(config.base_url);
    }

    async login(email: string, password: string) {
        await this.emailInput.fill(email);
        await this.passwordInput.fill(password);
        await this.loginButton.click();
    }
}
