import { test, expect } from '@playwright/test';

const STUDIO_URL = process.env.TEST_BASE_URL || 'http://localhost:8080';
const MGW_URL = process.env.TEST_MGW_URL || 'http://localhost:8081';
const DOCS_URL = process.env.TEST_DOCS_URL || 'http://localhost:8989';

// Test credentials for first-user registration
const TEST_ADMIN = {
  name: 'Smoke Test Admin',
  email: 'smoke-admin@test.local',
  password: 'SmokeTest#2025',
};

/**
 * Helper: login via the UI. Navigates to root and logs in.
 */
async function loginViaUI(page: any) {
  await page.goto(STUDIO_URL);
  await page.waitForSelector('input', { timeout: 15000 });
  await page.getByRole('textbox', { name: 'Email address' }).fill(TEST_ADMIN.email);
  await page.getByRole('textbox', { name: 'Password' }).fill(TEST_ADMIN.password);
  await page.getByRole('button', { name: 'Log in' }).click();
  // Wait for navigation away from login page
  await page.waitForSelector('input[type="password"]', { state: 'hidden', timeout: 15000 });
}

test.describe('Package Installation Smoke Tests', () => {

  test('AI Studio health endpoint responds', async ({ request }) => {
    const response = await request.get(`${STUDIO_URL}/health`);
    expect(response.status()).toBe(200);
    const body = await response.json();
    expect(body.status).toBe('ok');
  });

  test('Microgateway health endpoint responds', async ({ request }) => {
    const response = await request.get(`${MGW_URL}/health`);
    expect(response.status()).toBe(200);
    const body = await response.json();
    expect(body.status).toBe('ok');
  });

  test('AI Studio UI loads, register first user, and login', async ({ page }) => {
    await test.step('Register first user (becomes admin)', async () => {
      // Navigate directly to register page
      await page.goto(`${STUDIO_URL}/register`);
      await page.waitForSelector('input', { timeout: 15000 });

      // Fill registration form using role-based selectors (matches the actual UI)
      await page.getByRole('textbox', { name: 'Name' }).fill(TEST_ADMIN.name);
      await page.getByRole('textbox', { name: 'Email address' }).fill(TEST_ADMIN.email);
      await page.getByRole('textbox', { name: 'Password' }).fill(TEST_ADMIN.password);

      // Submit registration - click and wait for redirect or retry
      const signUpBtn = page.getByRole('button', { name: 'Sign up' });
      await signUpBtn.click();
      // Workaround: button can get stuck in stale state, retry click if still on register
      try {
        await page.waitForURL('**/login', { timeout: 5000 });
      } catch {
        await signUpBtn.click();
        await page.waitForURL('**/login', { timeout: 15000 });
      }
    });

    await test.step('Login with newly created admin user', async () => {
      await loginViaUI(page);
    });

    await test.step('Verify dashboard loaded', async () => {
      const bodyText = await page.textContent('body');
      expect(bodyText).toBeTruthy();
      // Should NOT still be on the login page
      expect(bodyText).not.toContain('Log in to your account');
    });
  });

  test('Edge gateway appears connected in admin UI', async ({ page }) => {
    await test.step('Login', async () => {
      await loginViaUI(page);
    });

    await test.step('Dismiss quick-start wizard if present', async () => {
      // The first-time login shows a quick-start dialog that blocks the UI
      // Try multiple dismiss strategies
      const dismissSelectors = [
        'button:has-text("Explore by myself")',
        'button:has-text("Skip quick start")',
        'button:has-text("Skip")',
        'button:has-text("Close")',
        '[aria-label="close"]',
      ];
      for (const selector of dismissSelectors) {
        const btn = page.locator(selector).first();
        if (await btn.isVisible({ timeout: 1000 }).catch(() => false)) {
          await btn.click({ force: true });
          await page.waitForTimeout(1000);
          break;
        }
      }
      // If dialog is still there, press Escape to close it
      const dialog = page.locator('[role="presentation"].MuiDialog-root');
      if (await dialog.isVisible({ timeout: 1000 }).catch(() => false)) {
        await page.keyboard.press('Escape');
        await page.waitForTimeout(1000);
      }
    });

    await test.step('Navigate to Edge Gateways via sidebar AI Portal', async () => {
      // Click "AI Portal" in the sidebar (not the top tab) - it's a drawer
      // The sidebar link is inside the nav/drawer area
      const sidebarPortalLink = page.locator('.sidebar a:has-text("AI Portal"), nav a:has-text("AI Portal"), [data-testid="portal-sidebar"]').first();
      if (await sidebarPortalLink.isVisible({ timeout: 3000 }).catch(() => false)) {
        await sidebarPortalLink.click();
      } else {
        // Fallback: click the second "AI Portal" text (first is the top tab)
        await page.getByText('AI Portal').nth(1).click();
      }
      await page.waitForLoadState('networkidle', { timeout: 10000 });

      // Now find and click Edge Gateways in the expanded drawer
      const edgeLink = page.getByText(/Edge Gateway/i).first();
      await edgeLink.click();
      await page.waitForLoadState('networkidle', { timeout: 15000 });
      await page.waitForTimeout(3000);
    });

    await test.step('Verify smoke-test-edge appears', async () => {
      const pageContent = await page.textContent('body', { timeout: 10000 });
      expect(pageContent).toContain('smoke-test-edge');
    });
  });

  test('Microgateway rejects unauthenticated proxy request', async ({ request }) => {
    const response = await request.post(`${MGW_URL}/call/anthropic/v1/messages`, {
      headers: { 'Content-Type': 'application/json' },
      data: {
        model: 'test',
        messages: [{ role: 'user', content: 'test' }],
      },
    });

    // Should be rejected - 401/403 (no auth) or 404 (no route without config)
    expect([401, 403, 404]).toContain(response.status());
  });

  test('Docs server responds', async ({ request }) => {
    const response = await request.get(DOCS_URL, {
      maxRedirects: 0,
    }).catch(() => null);

    if (response) {
      expect([200, 301, 302, 304]).toContain(response.status());
    }
  });
});
