import { Locator, Page } from '@playwright/test';
import { TableWrapper } from '@wrappers/TableWrapper';
import { PageTemplate } from './Page_template';

interface CatalogueParams {
    name: string;
    llm: string;
}

/**
 * LLM catalogue list and form (/admin/catalogs/llms).
 *
 * The form's membership control is the shared RelationshipPicker: selected
 * LLMs are chips carrying the item name, the add control is the Autocomplete
 * labelled "Add LLM", and removal is the chip's delete icon labelled
 * "Remove {name}". Changes only persist when the form is saved.
 */
export class AdminCataloguesPage extends PageTemplate {
    readonly Table: TableWrapper;
    readonly AddCatalogueButton: Locator;
    readonly CatalogueNameInput: Locator;
    readonly CreateCatalogueButton: Locator;
    readonly UpdateCatalogueButton: Locator;
    readonly BackToCataloguesLink: Locator;
    readonly LlmPicker: Locator;
    readonly AddLlmInput: Locator;
    readonly PickerCaption: Locator;

    constructor(page: Page) {
        super(page);
        this.Table = new TableWrapper('table', page);
        this.AddCatalogueButton = this.page.getByRole('button', { name: 'Add catalog', exact: true });
        this.CatalogueNameInput = this.page.getByRole('textbox', { name: 'Catalog Name' });
        this.CreateCatalogueButton = this.page.getByRole('button', { name: 'Create catalog' });
        this.UpdateCatalogueButton = this.page.getByRole('button', { name: 'Update catalog' });
        this.BackToCataloguesLink = this.page.getByRole('link', { name: 'Back to catalogs' });
        this.LlmPicker = this.page.getByTestId('relationship-picker').filter({ has: this.page.getByRole('combobox', { name: 'Add LLM provider' }) });
        this.AddLlmInput = this.page.getByRole('combobox', { name: 'Add LLM provider' });
        this.PickerCaption = this.page.getByTestId('relationship-picker-caption');
    }

    async goto() {
        await this.page.goto('/admin/catalogs/llms');
    }

    async addCatalogue(params: CatalogueParams) {
        await this.AddCatalogueButton.click();
        await this.CatalogueNameInput.fill(params.name);
        await this.addLlmToCatalogue(params.llm);
        await this.CreateCatalogueButton.click();
    }

    /** Chip for an LLM currently in the catalogue (present only while selected). */
    llmChip(llm: string): Locator {
        return this.LlmPicker.locator('.MuiChip-root').filter({ hasText: llm });
    }

    /** Type into the "Add LLM" Autocomplete and pick the option; it becomes a chip at once. */
    async addLlmToCatalogue(llm: string) {
        await this.AddLlmInput.click();
        await this.AddLlmInput.fill(llm);
        await this.page.getByRole('option', { name: llm, exact: true }).click();
        await this.llmChip(llm).waitFor();
    }

    /** Remove an LLM via its chip's delete icon. */
    async removeLlmFromCatalogue(llm: string) {
        await this.page.getByLabel(`Remove ${llm}`).click();
        await this.llmChip(llm).waitFor({ state: 'detached' });
    }

    async deleteCatalogue(rowNumber: number) {
        await this.Table.triggerDeleteAction(rowNumber);
    }

    async backToCatalogues() {
        await this.BackToCataloguesLink.click();
    }
}
