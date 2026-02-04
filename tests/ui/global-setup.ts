import { chromium, FullConfig, Page } from '@playwright/test';
import * as dotenv from 'dotenv';
import * as path from 'path';
import * as fs from 'fs';

// Load environment variables from dev/.env.secrets if it exists
const secretsPath = path.resolve(__dirname, '../../dev/.env.secrets');
if (fs.existsSync(secretsPath)) {
  console.log(`Loading environment from ${secretsPath}`);
  dotenv.config({ path: secretsPath });
}

// Build config with environment variables (evaluated after dotenv loads)
// Dev: frontend on 3000 (proxies API to 8080), CI sets TEST_BASE_URL to 8081 (Go embedded)
const config = {
  admin_email: 'auto_test@tyk.io',
  password: 'Test#2025',
  admin_name: 'Test Admin',
  dev_user_email: 'dev@tyk.io',
  dev_user_name: 'Dev User',
  base_url: process.env.TEST_BASE_URL || 'http://localhost:3000',
  api_url: process.env.TEST_BASE_URL || 'http://localhost:3000',
  bootstrap_admin_email: process.env.BOOTSTRAP_ADMIN_EMAIL || 'admin@tyk.io',
  bootstrap_admin_password: process.env.BOOTSTRAP_ADMIN_PASSWORD || 'Admin#2025',
};

/**
 * Global setup for Playwright tests.
 * Ensures the test admin user exists before any tests run.
 *
 * Uses a browser-based approach to avoid request context issues:
 * 1. Try to login as test admin via UI (if already exists)
 * 2. Register via UI (first user = admin + verified)
 * 3. While logged in, create dev user via API using browser's session
 */
async function globalSetup(playwrightConfig: FullConfig) {
  console.log('Global setup: Ensuring admin user exists...');
  console.log(`Base URL: ${config.base_url}`);

  const browser = await chromium.launch();
  const context = await browser.newContext();
  const page = await context.newPage();

  try {
    // Strategy 1: Check if test admin already exists (try login via UI)
    const canLogin = await tryLoginViaBrowser(page, config.admin_email, config.password);
    if (canLogin) {
      console.log('Test admin user already exists and can login');
      // Ensure permissions are correct and dev user exists
      await ensureTestAdminPermissions(page);
      await ensureDevUserExists(page);
      return;
    }

    // Strategy 2: Register via UI (first user = admin + verified)
    console.log('Attempting UI registration...');
    const registered = await registerViaBrowser(page);
    if (registered) {
      console.log('Test admin created via UI registration (first user)');
      // Login to set up session, then create dev user
      const loggedIn = await tryLoginViaBrowser(page, config.admin_email, config.password);
      if (loggedIn) {
        await ensureTestAdminPermissions(page);
        await ensureDevUserExists(page);
      } else {
        console.warn('Warning: Could not login after registration');
      }
      return;
    }

    // Strategy 3: Use bootstrap admin to create test admin
    console.log('UI registration failed. Trying bootstrap admin approach...');
    const bootstrapLogin = await tryLoginViaBrowser(
      page,
      config.bootstrap_admin_email,
      config.bootstrap_admin_password
    );
    if (bootstrapLogin) {
      const created = await createUserViaAPI(page, {
        email: config.admin_email,
        name: config.admin_name,
        password: config.password,
        is_admin: true,
        show_chat: true,
        show_portal: true,
        email_verified: true,
        notifications_enabled: true,
      });
      if (created) {
        console.log('Test admin created via bootstrap admin');
        // Re-login as test admin to create dev user
        await tryLoginViaBrowser(page, config.admin_email, config.password);
        await ensureDevUserExists(page);
        return;
      }
    }

    throw new Error(
      'Failed to create test admin user. ' +
        'Ensure bootstrap admin exists (set BOOTSTRAP_ADMIN_EMAIL/BOOTSTRAP_ADMIN_PASSWORD env vars) ' +
        'or start with a clean database.'
    );
  } finally {
    await browser.close();
  }
}

/**
 * Try to login via the UI. Returns true if successful.
 */
async function tryLoginViaBrowser(page: Page, email: string, password: string): Promise<boolean> {
  try {
    // Navigate to root first to initialize the frontend app
    await page.goto(config.base_url);
    await page.waitForLoadState('networkidle');

    // Then navigate to login
    await page.goto(`${config.base_url}/login`);
    await page.waitForLoadState('networkidle');

    // Check if already logged in (redirected to dashboard)
    if (!page.url().includes('/login')) {
      console.log('Already logged in, logging out first...');
      // Could be logged in as different user, logout first
      try {
        await page.goto(config.base_url);
        await page.waitForLoadState('networkidle');
        await page.goto(`${config.base_url}/logout`);
        await page.waitForLoadState('networkidle');
        await page.goto(config.base_url);
        await page.waitForLoadState('networkidle');
        await page.goto(`${config.base_url}/login`);
        await page.waitForLoadState('networkidle');
      } catch {
        // Ignore logout errors
      }
    }

    // Fill login form
    await page.getByLabel('Email address').fill(email);
    await page.getByLabel('Password').fill(password);
    await page.getByRole('button', { name: 'Log in' }).click();

    // Wait for navigation or error
    await page.waitForTimeout(2000);

    // Check if login succeeded (not on login page anymore)
    const currentUrl = page.url();
    if (!currentUrl.includes('/login')) {
      console.log(`Login successful for ${email}`);
      return true;
    }

    // Check for error message
    const errorVisible = await page.locator('.text-red-600, .error-message, [role="alert"]').isVisible();
    if (errorVisible) {
      console.log(`Login failed for ${email}: error message displayed`);
    }

    return false;
  } catch (error) {
    console.log(`Login attempt failed for ${email}:`, error);
    return false;
  }
}

/**
 * Register a new user via the UI. Returns true if successful.
 */
async function registerViaBrowser(page: Page): Promise<boolean> {
  try {
    // Navigate to root first to initialize the frontend app
    await page.goto(config.base_url);
    await page.waitForLoadState('networkidle');

    // Go to login page first (direct /register navigation doesn't work)
    await page.goto(`${config.base_url}/login`);
    await page.waitForLoadState('networkidle');

    // Click the "Sign up" link to get to registration
    const signUpLink = page.getByRole('link', { name: 'Sign up' });
    if (!(await signUpLink.isVisible())) {
      console.log('Sign up link not visible on login page');
      return false;
    }
    await signUpLink.click();
    await page.waitForLoadState('networkidle');

    // Check if registration is available
    if (!page.url().includes('/register')) {
      console.log('Registration page not accessible');
      return false;
    }

    // Fill registration form
    await page.getByLabel('Name').fill(config.admin_name);
    await page.getByLabel('Email address').fill(config.admin_email);
    await page.getByLabel('Password').fill(config.password);
    await page.getByRole('button', { name: 'Sign up' }).click();

    // Wait for redirect to login
    try {
      await page.waitForURL('**/login', { timeout: 10000 });
      console.log('Registration completed, redirected to login');
      return true;
    } catch {
      // Check if we're on a success page or dashboard
      const currentUrl = page.url();
      if (!currentUrl.includes('/register')) {
        console.log('Registration completed');
        return true;
      }

      // Check for error
      const errorVisible = await page.locator('.text-red-600, .error-message, [role="alert"]').isVisible();
      if (errorVisible) {
        const errorText = await page.locator('.text-red-600, .error-message, [role="alert"]').textContent();
        console.log(`Registration failed: ${errorText}`);
      }
      return false;
    }
  } catch (error) {
    console.log('Registration failed:', error);
    return false;
  }
}

/**
 * Ensure test admin has correct permissions using the browser's session.
 */
async function ensureTestAdminPermissions(page: Page): Promise<void> {
  console.log('Ensuring test admin has correct permissions...');

  try {
    // Get CSRF token
    const csrfResponse = await page.request.get(`${config.api_url}/csrf-token`);
    const csrfToken = csrfResponse.headers()['x-csrf-token'] || '';

    // List users to find test admin
    const listResponse = await page.request.get(`${config.api_url}/api/v1/users`, {
      headers: { 'X-CSRF-Token': csrfToken },
    });

    if (!listResponse.ok()) {
      console.log('Warning: Could not list users');
      return;
    }

    const usersData = await listResponse.json();
    const currentUser = usersData.data?.find(
      (u: { attributes?: { email?: string } }) => u.attributes?.email === config.admin_email
    );

    if (!currentUser) {
      console.log('Warning: Could not find test admin user');
      return;
    }

    const userId = currentUser.id;
    const attrs = currentUser.attributes || {};

    // Check if permissions need updating
    if (attrs.is_admin && attrs.show_chat && attrs.show_portal) {
      console.log('Test admin already has correct permissions');
      return;
    }

    console.log(
      `Updating test admin permissions: is_admin=${attrs.is_admin}, show_chat=${attrs.show_chat}, show_portal=${attrs.show_portal}`
    );

    // Update user permissions
    const updateResponse = await page.request.patch(`${config.api_url}/api/v1/users/${userId}`, {
      headers: { 'X-CSRF-Token': csrfToken },
      data: {
        data: {
          type: 'users',
          id: userId,
          attributes: {
            email: config.admin_email,
            is_admin: true,
            show_chat: true,
            show_portal: true,
          },
        },
      },
    });

    if (updateResponse.ok()) {
      console.log('Test admin permissions updated successfully');
    } else {
      const body = await updateResponse.text();
      console.log(`Warning: Could not update test admin permissions: ${body}`);
    }
  } catch (error) {
    console.log('Warning: Error updating test admin permissions:', error);
  }
}

/**
 * Ensure dev user exists using the browser's session.
 */
async function ensureDevUserExists(page: Page): Promise<void> {
  console.log('Ensuring dev user exists...');

  // Check if dev user already exists by trying to find them
  try {
    const csrfResponse = await page.request.get(`${config.api_url}/csrf-token`);
    const csrfToken = csrfResponse.headers()['x-csrf-token'] || '';

    const listResponse = await page.request.get(`${config.api_url}/api/v1/users`, {
      headers: { 'X-CSRF-Token': csrfToken },
    });

    if (listResponse.ok()) {
      const usersData = await listResponse.json();
      const devUser = usersData.data?.find(
        (u: { attributes?: { email?: string } }) => u.attributes?.email === config.dev_user_email
      );

      if (devUser) {
        console.log('Dev user already exists');
        // Update to ensure correct settings
        const updateResponse = await page.request.patch(`${config.api_url}/api/v1/users/${devUser.id}`, {
          headers: { 'X-CSRF-Token': csrfToken },
          data: {
            data: {
              type: 'users',
              id: devUser.id,
              attributes: {
                email_verified: true,
                show_chat: true,
                show_portal: true,
              },
            },
          },
        });
        if (updateResponse.ok()) {
          console.log('Dev user settings verified');
        }
        return;
      }
    }

    // Create dev user
    const created = await createUserViaAPI(page, {
      email: config.dev_user_email,
      name: config.dev_user_name,
      password: config.password,
      is_admin: false,
      show_chat: true,
      show_portal: true,
      email_verified: true,
    });

    if (created) {
      console.log('Dev user created successfully');
    } else {
      console.warn('Warning: Failed to create dev user. Portal/chat tests may fail.');
    }
  } catch (error) {
    console.warn('Warning: Error ensuring dev user exists:', error);
  }
}

/**
 * Create a user via API using the browser's session.
 */
async function createUserViaAPI(
  page: Page,
  user: {
    email: string;
    name: string;
    password: string;
    is_admin: boolean;
    show_chat: boolean;
    show_portal: boolean;
    email_verified: boolean;
    notifications_enabled?: boolean;
  }
): Promise<boolean> {
  try {
    const csrfResponse = await page.request.get(`${config.api_url}/csrf-token`);
    const csrfToken = csrfResponse.headers()['x-csrf-token'] || '';

    const createResponse = await page.request.post(`${config.api_url}/api/v1/users`, {
      headers: { 'X-CSRF-Token': csrfToken },
      data: {
        data: {
          type: 'users',
          attributes: user,
        },
      },
    });

    if (createResponse.ok()) {
      return true;
    }

    const body = await createResponse.text();
    console.log(`User creation failed (${createResponse.status()}): ${body}`);
    return false;
  } catch (error) {
    console.log('User creation failed:', error);
    return false;
  }
}

export default globalSetup;
