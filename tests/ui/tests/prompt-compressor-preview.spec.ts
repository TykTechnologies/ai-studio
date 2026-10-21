import { test } from '@fixtures';
import { expect } from '@playwright/test';
import { config } from '../config';
import { ensurePlugin, pluginAvailable, rpc } from '../utils/prompt_compressor';

/**
 * Prompt Compressor (community plugin): the preview tool.
 *
 * Pastes a failing pytest run as a tool output, previews it with a
 * level-2 policy edited on the page, and checks the failures survive in
 * the output, a marker shows where lines were cut, and the decisions name
 * the pytest reducer. Also checks the 2 MiB refusal.
 */

const PAGE_PATH = '/admin/prompt-compressor/preview';
const TAG = 'prompt-compressor-preview';

// A verbose pytest run: 120 passing tests and 2 failures. Level 2 keeps
// the failures and drops the passing lines; a level-2 cut must remove at
// least 30% of the text (eac.70), and this one removes about 80%, so a
// sample too small to clear that bar can't make the spec flaky.
const FILES = ['tests/test_accounts.py', 'tests/test_ledger.py', 'tests/test_payments.py', 'tests/test_reports.py', 'tests/test_users.py'];
const FAILED: Record<number, string> = {
    37: 'tests/test_ledger.py::test_ledger_balances_after_refund',
    101: 'tests/test_reports.py::test_monthly_report_totals',
};
const RESULTS = Array.from({ length: 122 }, (_, k) => {
    const i = k + 1;
    const pct = String(Math.floor((i * 100) / 122)).padStart(3);
    const id = FAILED[i] ?? `${FILES[k % 5]}::test_case_${String(i).padStart(3, '0')}`;
    return `${id} ${FAILED[i] ? 'FAILED' : 'PASSED'} [${pct}%]`;
}).join('\n');

const PYTEST = `============================= test session starts ==============================
platform linux -- Python 3.12.3, pytest-8.3.2, pluggy-1.5.0
rootdir: /srv/ledger
collected 122 items

${RESULTS}

=================================== FAILURES ===================================
_____________________ test_ledger_balances_after_refund ______________________

    def test_ledger_balances_after_refund():
>       assert ledger.balance("ACC-1042") == Decimal("120.00")
E       AssertionError: assert Decimal('100.00') == Decimal('120.00')

tests/test_ledger.py:88: AssertionError
_________________________ test_monthly_report_totals __________________________

    def test_monthly_report_totals():
>       assert report.total("2026-09") == 4410
E       assert 4401 == 4410

tests/test_reports.py:57: AssertionError
=========================== short test summary info ============================
FAILED tests/test_ledger.py::test_ledger_balances_after_refund - AssertionError
FAILED tests/test_reports.py::test_monthly_report_totals - assert 4401 == 4410
======================== 2 failed, 120 passed in 9.81s =========================`;

test.describe.configure({ mode: 'serial' });

test('Prompt Compressor preview shows pytest failures kept at level 2', async ({ page, loginPage, adminMainPage }) => {
    test.skip(!pluginAvailable(), 'the community submodule does not have the prompt-compressor plugin');
    test.setTimeout(240000);

    await loginPage.goto();
    await loginPage.login(config.admin_email, config.password);
    await adminMainPage.dismissQuickStartModal();
    const pluginId = await ensurePlugin(page, adminMainPage, 'getPreviewOptions');

    await page.goto(`${config.base_url}${PAGE_PATH}`);
    await adminMainPage.dismissQuickStartModal();
    const ui = page.locator(TAG);
    await expect(ui.getByRole('heading', { name: 'Compression preview' })).toBeVisible({ timeout: 20000 });

    await ui.getByTestId('pc-preview-mode-tool_output').click();
    await ui.getByTestId('pc-preview-text').fill(PYTEST);
    await ui.getByTestId('pc-preview-edit-inline').check();
    await ui.getByTestId('pc-preview-policy-json').fill(JSON.stringify({ level: 2, min_unit_tokens: 50 }));
    await ui.getByTestId('pc-preview-run').click();

    const result = ui.getByTestId('pc-preview-result');
    await expect(result).toBeVisible({ timeout: 30000 });
    await expect(result).toContainText('compressed');
    await expect(result).toContainText('passed'); // post-check
    const units = ui.getByTestId('pc-preview-units');
    // The decision names the level and says the text changed; if a threshold
    // change leaves this sample alone, this line fails with the decision shown
    await expect(units, 'the pytest output should be cut at level 2').toContainText('Level 2, changed');
    await expect(units).toContainText('pytest');
    await expect(units).toContainText('reduce_pytest');

    // The failures survive; a highlighted marker shows the cut
    await ui.locator('summary', { hasText: 'Show the output body' }).click();
    const output = ui.getByTestId('pc-preview-output');
    await expect(output).toContainText('test_ledger_balances_after_refund');
    await expect(output).toContainText('test_monthly_report_totals');
    await expect(output).toContainText('2 failed, 120 passed');
    await expect(output.locator('mark').first()).toContainText('tyk-compressed');

    // Inputs over 2 MiB are refused (by the plugin, or earlier by the host)
    const big = await rpc(page, pluginId, 'preview', { mode: 'tool_output', format: 'openai', text: 'x'.repeat(2 * 1024 * 1024 + 1) });
    expect(big.status).toBeGreaterThanOrEqual(400);
});
