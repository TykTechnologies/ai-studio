import { Locator, Page } from '@playwright/test';
import { DropDownWrapper } from '@wrappers/DropDownWrapper';
import { TableWrapper } from '@wrappers/TableWrapper';
import { PageTemplate } from './Page_template';

interface CatalogueParams {
    name: string;
    llm: string;
}

export class AdminCataloguesPage extends PageTemplate {
    readonly Table: TableWrapper;
    readonly AddCatalogueButton: Locator;
    readonly CatalogueNameInput: Locator;
    readonly LlmDropDown: DropDownWrapper;
    readonly CreateCatalogueButton: Locator;
    readonly UpdateCatalogueButton: Locator;
    readonly BackToCataloguesLink: Locator;
    readonly AddLlmToCatalogueButton: Locator;
    readonly RemoveLlmFromCatalogueButton: Locator;
    readonly SelectLlmDropDown: DropDownWrapper;

    constructor(page: Page) {
        super(page);
        this.Table = new TableWrapper('table', page);
        this.AddCatalogueButton = this.page.getByRole('button', { name: 'Add catalog', exact: true });
        this.CatalogueNameInput = this.page.getByRole('textbox', { name: 'Catalog Name' });
        this.LlmDropDown = new DropDownWrapper('combobox', page);
        this.CreateCatalogueButton = this.page.getByRole('button', { name: 'Create catalog' });
        this.UpdateCatalogueButton = this.page.getByRole('button', { name: 'Update catalog' });
        this.BackToCataloguesLink = this.page.getByRole('link', { name: 'Back to catalogs' });
        this.AddLlmToCatalogueButton = this.page.getByRole('menuitem', { name: 'Add LLM to catalog' });
        this.RemoveLlmFromCatalogueButton = this.page.getByRole('menuitem', { name: 'Remove LLM from catalog' });
        this.SelectLlmDropDown = new DropDownWrapper('combobox', page);
    }

    async goto() {
        await this.page.goto('/admin/catalogues');
    }

    async addCatalogue(params: CatalogueParams) {
        await this.AddCatalogueButton.click();
        await this.CatalogueNameInput.fill(params.name);
        await this.LlmDropDown.setValue(params.llm);
        await this.CreateCatalogueButton.click();
    }

    async addLlmToCatalogue(llm: string) {
        await this.AddLlmToCatalogueButton.click();
        await this.SelectLlmDropDown.setValue(llm);
        await this.page.getByRole('button', { name: 'Add' }).click();
    }

    async removeLlmFromCatalogue(llm: string) {
        await this.RemoveLlmFromCatalogueButton.click();
        await this.SelectLlmDropDown.setValue(llm);
        await this.page.getByRole('button', { name: 'Remove' }).click();
    }

    async deleteCatalogue(rowNumber: number) {
        await this.Table.triggerDeleteAction(rowNumber);
    }

    async backToCatalogues() {
        await this.BackToCataloguesLink.click();
    }
}
