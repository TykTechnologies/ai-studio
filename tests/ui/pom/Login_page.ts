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
        this.LoginButton = this.page.getByRole('button').filter({ hasText: /log in/i });
        this.RegisterHereButton = this.page.getByRole('link', { name: 'Sign up' });
        this.ForgotPasswordButton = this.page.getByText('Forgot password?');
    }
    
    /**
     * Opens the app root (which lands on the login form when logged out).
     * Right after a logout the app performs its own full reload to /login; a
     * goto that overlaps it is aborted by the browser (net::ERR_ABORTED or
     * "interrupted by another navigation"), so retry until one completes.
     */
    async goto() {
        for (let attempt = 1; ; attempt++) {
            try {
                await this.page.goto(config.base_url);
                return;
            } catch (error) {
                if (attempt >= 3) {
                    throw error;
                }
                await this.page.waitForLoadState('load').catch(() => undefined);
            }
        }
    }

    async login(email: string, password: string) {
        await this.EmailInput.fill(email);
        await this.PasswordInput.fill(password);
        await this.LoginButton.click();
        await this.page.waitForTimeout(1000);
        
        if (await this.page.getByRole('img', { name: 'Logo' }).isVisible() ||
            await this.page.getByRole('button', { name: 'Explore by myself' }).isVisible()) {
            return;
        }
        
        if (await this.EmailInput.isVisible()) {
            await this.EmailInput.fill(email);
            await this.PasswordInput.fill(password);
            await this.LoginButton.click();
        }
    }
}
