// c1g.15 step 9b, the UI half: with AI_STUDIO_PLUGIN_ALLOWED_SOURCES=oci the
// plugin wizard offers only oci:// and refuses a file path on the client.
// Run with NODE_PATH pointing at a node_modules that has @playwright/test
// (e.g. tests/packaging/node_modules):
//   NODE_PATH=tests/packaging/node_modules node tests/e2e/cosign-distroless/ui_lockdown.js <shot.png>
const { chromium } = require('@playwright/test');

const STUDIO = process.env.STUDIO_URL || 'http://127.0.0.1:18080';
const EMAIL = 'e2e-admin@test.local';
const PASSWORD = 'E2eTest#2026-c1g';

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage();
  let failed = false;
  const check = (ok, what) => {
    console.log(`${ok ? 'PASS' : 'FAIL'}: ${what}`);
    if (!ok) failed = true;
  };
  try {
    await page.goto(`${STUDIO}/login`);
    await page.getByRole('textbox', { name: 'Email address' }).fill(EMAIL);
    await page.getByRole('textbox', { name: 'Password' }).fill(PASSWORD);
    await page.getByRole('button', { name: 'Log in' }).click();
    await page.waitForSelector('input[type="password"]', { state: 'hidden', timeout: 15000 });

    await page.goto(`${STUDIO}/admin/plugins/create`);
    const command = page.getByRole('textbox', { name: 'Command' });
    await command.waitFor({ timeout: 15000 });
    const body = await page.textContent('body');
    const help = body.match(/Plugin command - use [^\n]*?(?=\s{2}|$|Back|Cancel|Next)/);
    console.log(`helper text: ${help ? help[0] : '(not found)'}`);
    check(body.includes('oci:// for OCI artifacts (they must be signed)'), 'helper text offers oci:// and says artifacts must be signed');
    check(!body.includes('grpc:// for external services') && !body.includes('local path for binaries'), 'helper text offers no file or grpc source');
    check((await command.getAttribute('placeholder')) === 'e.g., oci://registry.com/my-plugin:latest', 'placeholder is an oci:// example');

    await page.getByRole('textbox', { name: 'Plugin Name' }).fill('ui-refused').catch(() => {});
    await command.fill('/opt/plugins/request_enricher');
    await page.getByRole('button', { name: 'Load Plugin' }).click();
    await page.waitForTimeout(1000);
    const after = await page.textContent('body');
    check(after.includes('plugin source "file" is not allowed by this installation (allowed: oci). Set AI_STUDIO_PLUGIN_ALLOWED_SOURCES to change this.'),
      'a file path is refused on the client with the backend wording');
    if (process.argv[2]) await page.screenshot({ path: process.argv[2], fullPage: true });
  } catch (err) {
    console.log(`FAIL: ${err.message}`);
    failed = true;
  } finally {
    await browser.close();
  }
  process.exit(failed ? 1 : 0);
})();
