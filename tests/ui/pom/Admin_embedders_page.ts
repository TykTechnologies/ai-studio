import { Locator, Page, expect } from '@playwright/test';
import { config } from '../config';
import { PageTemplate } from './Page_template';

/** Embedders: the admin list, form and the inline creator on other forms. */
export class AdminEmbeddersPage extends PageTemplate {
    readonly AddButton: Locator;
    readonly NameInput: Locator;
    readonly ModelInput: Locator;
    readonly EndpointInput: Locator;
    readonly ApiKeyInput: Locator;
    readonly CreateButton: Locator;
    readonly SaveButton: Locator;

    constructor(page: Page) {
        super(page);
        // The title bar's button; an empty list shows a second one in its empty state.
        this.AddButton = page.getByRole('button', { name: 'Add embedder' }).first();
        this.NameInput = page.getByTestId('embedder-name');
        this.ModelInput = page.getByTestId('embedder-model');
        this.EndpointInput = page.getByLabel('Endpoint URL');
        this.ApiKeyInput = page.getByLabel('API key');
        this.CreateButton = page.getByRole('button', { name: 'Create embedder' });
        this.SaveButton = page.getByRole('button', { name: 'Save embedder' });
    }

    async goto() {
        await this.page.goto(`${config.base_url}/admin/embedders`);
    }

    /** Picks an option of an MUI select by its label. */
    async choose(scope: Page | Locator, label: string, option: string) {
        await scope.getByRole('combobox', { name: label }).click();
        await this.page.getByRole('option', { name: option }).first().click();
    }

    async row(name: string): Promise<Locator> {
        const row = this.page.getByRole('row', { name: new RegExp(name) });
        await expect(row).toBeVisible();
        return row;
    }

    async openRowAction(name: string, action: string) {
        const row = await this.row(name);
        await row.getByRole('button', { name: `Actions for ${name}` }).click();
        await this.page.getByRole('menuitem', { name: action }).click();
    }
}
