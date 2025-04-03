import { Locator, Page } from '@playwright/test';
import { config } from '../config';

export class LoginPage {
    readonly page: Page;
    readonly EmailInput: Locator;
    readonly PasswordInput: Locator;
    readonly LoginButton: Locator;
    readonly RegisterHereButton: Locator;
    readonly ForgotPasswordButton: Locator;

    constructor(page: Page) {
        this.page = page;
        this.EmailInput = this.page.getByRole('textbox', { name: 'Email' });
        this.PasswordInput = this.page.getByRole('textbox', { name: 'Password' });
        this.LoginButton = this.page.getByRole('button', { name: 'Login' });
        this.RegisterHereButton = this.page.getByText('Register here');
        this.ForgotPasswordButton = this.page.getByText('Forgot password?');
    }
    
    async goto() {
        await this.page.goto(config.base_url);
    }

    async login(email: string, password: string) {
        await this.EmailInput.fill(email);
        await this.PasswordInput.fill(password);
        await this.LoginButton.click();
        await this.page.waitForTimeout(1000);
        if (await this.EmailInput.isVisible()) {
            console.log('Login failed. Retrying...');
            await this.EmailInput.fill(email);
            await this.PasswordInput.fill(password);
            await this.LoginButton.click();
        }
    }
}
