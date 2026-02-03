import { chromium, FullConfig, request } from '@playwright/test';
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
const config = {
  admin_email: 'auto_test@tyk.io',
  password: 'Test#2025',
  admin_name: 'Test Admin',
  dev_user_email: 'dev@tyk.io',
  dev_user_name: 'Dev User',
  base_url: 'http://localhost:8081',
  // API runs on port 8080 inside container, exposed as 8081 on host (see tests/compose.yml)
  api_url: process.env.API_URL || 'http://localhost:8081',
  bootstrap_admin_email: process.env.BOOTSTRAP_ADMIN_EMAIL || 'admin@tyk.io',
  bootstrap_admin_password: process.env.BOOTSTRAP_ADMIN_PASSWORD || 'Admin#2025',
};

/**
 * Global setup for Playwright tests.
 * Ensures the test admin user exists before any tests run.
 *
 * Uses a multi-strategy approach:
 * 1. Try to login as test admin (if already exists)
 * 2. Try API registration (works if first user - becomes admin + verified)
 * 3. Try UI registration (fallback)
 * 4. Use API with bootstrap admin to create test admin (when DB has existing users)
 */
async function globalSetup(playwrightConfig: FullConfig) {
  console.log('Global setup: Ensuring admin user exists...');
  console.log(`Bootstrap admin: ${config.bootstrap_admin_email}`);

  // Strategy 1: Check if test admin already exists
  const canLogin = await tryLogin(config.admin_email, config.password);
  if (canLogin) {
    console.log('Test admin user already exists and can login');
    // Ensure test admin has correct permissions (admin, chat, portal)
    await ensureTestAdminPermissions();
    // Still ensure dev user exists
    await ensureDevUserExists();
    return;
  }

  // Strategy 2: Try API registration (works if first user - becomes admin + verified)
  const apiRegistered = await tryApiRegister();
  if (apiRegistered) {
    console.log('Test admin created via API registration (first user)');
    // Also create dev user
    await ensureDevUserExists();
    return;
  }

  // Strategy 3: Try UI registration (fallback)
  const registered = await tryRegister();
  if (registered) {
    console.log('Test admin created via UI registration (first user)');
    // Also create dev user
    await ensureDevUserExists();
    return;
  }

  // Strategy 4: Use API with bootstrap admin to create test admin
  console.log('Database has existing users. Using API to create test admin...');
  const created = await createViaAPI();
  if (created) {
    console.log('Test admin created via API');
  } else {
    throw new Error(
      'Failed to create test admin user. ' +
        'Ensure bootstrap admin exists (set BOOTSTRAP_ADMIN_EMAIL/BOOTSTRAP_ADMIN_PASSWORD env vars) ' +
        'or start with a clean database.'
    );
  }

  // Now ensure dev user exists (needed for user-app-and-proxy tests)
  await ensureDevUserExists();
}

/**
 * Ensure test admin has correct permissions (is_admin, show_chat, show_portal).
 * This handles cases where the user was created but with wrong permissions.
 */
async function ensureTestAdminPermissions() {
  console.log('Ensuring test admin has correct permissions...');

  const context = await request.newContext();
  try {
    // Get CSRF token
    const csrfResponse = await context.get(`${config.api_url}/csrf-token`);
    const csrfToken = csrfResponse.headers()['x-csrf-token'];

    // Login as test admin
    const loginResponse = await context.post(`${config.api_url}/auth/login`, {
      headers: {
        'X-CSRF-Token': csrfToken || '',
      },
      data: {
        data: {
          attributes: {
            email: config.admin_email,
            password: config.password,
          },
        },
      },
    });

    if (!loginResponse.ok()) {
      console.log('Warning: Could not login to check permissions');
      return;
    }

    // Get user by listing all users and finding by email
    const listResponse = await context.get(`${config.api_url}/api/v1/users`, {
      headers: {
        'X-CSRF-Token': csrfToken || '',
      },
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

    console.log(`Updating test admin permissions: is_admin=${attrs.is_admin}, show_chat=${attrs.show_chat}, show_portal=${attrs.show_portal}`);

    // Update user permissions (include email as it's required for validation)
    const updateResponse = await context.patch(`${config.api_url}/api/v1/users/${userId}`, {
      headers: {
        'X-CSRF-Token': csrfToken || '',
      },
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
  } finally {
    await context.dispose();
  }
}

/**
 * Ensure dev user exists for portal/chat tests.
 * This user is a non-admin with portal and chat access.
 */
async function ensureDevUserExists() {
  console.log('Ensuring dev user exists...');

  // Check if dev user can already login
  const canLogin = await tryLogin(config.dev_user_email, config.password);
  if (canLogin) {
    console.log('Dev user already exists and can login');
    return;
  }

  // Create dev user via API (we should already have admin access from previous step)
  const created = await createDevUserViaAPI();
  if (created) {
    console.log('Dev user created via API');
    return;
  }

  console.warn('Warning: Failed to create dev user. Portal/chat tests may fail.');
}

async function createDevUserViaAPI(): Promise<boolean> {
  const context = await request.newContext();

  try {
    // First, get CSRF token
    const csrfResponse = await context.get(`${config.api_url}/csrf-token`);
    const csrfToken = csrfResponse.headers()['x-csrf-token'];

    // Login as test admin (created in previous step)
    const loginResponse = await context.post(`${config.api_url}/auth/login`, {
      headers: {
        'X-CSRF-Token': csrfToken || '',
      },
      data: {
        data: {
          attributes: {
            email: config.admin_email,
            password: config.password,
          },
        },
      },
    });

    if (!loginResponse.ok()) {
      console.error('Test admin login failed for dev user creation');
      return false;
    }

    // Create dev user via API
    // Note: notifications_enabled can only be set for admin users
    const createResponse = await context.post(`${config.api_url}/api/v1/users`, {
      headers: {
        'X-CSRF-Token': csrfToken || '',
      },
      data: {
        data: {
          type: 'users',
          attributes: {
            email: config.dev_user_email,
            name: config.dev_user_name,
            password: config.password,
            is_admin: false,
            show_chat: true,
            show_portal: true,
            email_verified: true,
          },
        },
      },
    });

    if (createResponse.ok()) {
      return true;
    }

    const createBody = await createResponse.text();

    // If user already exists, try to update them
    if (createBody.includes('Email is already in use')) {
      console.log('Dev user already exists, attempting to update...');

      const listResponse = await context.get(`${config.api_url}/api/v1/users`, {
        headers: {
          'X-CSRF-Token': csrfToken || '',
        },
      });

      if (!listResponse.ok()) {
        console.error(`Failed to list users: ${listResponse.status()}`);
        return false;
      }

      const usersData = await listResponse.json();
      const existingUser = usersData.data?.find(
        (u: { attributes?: { email?: string } }) => u.attributes?.email === config.dev_user_email
      );

      if (!existingUser) {
        console.error('Could not find existing dev user in list');
        return false;
      }

      // Update the user to ensure correct settings
      const updateResponse = await context.patch(`${config.api_url}/api/v1/users/${existingUser.id}`, {
        headers: {
          'X-CSRF-Token': csrfToken || '',
        },
        data: {
          data: {
            type: 'users',
            id: existingUser.id,
            attributes: {
              email_verified: true,
              show_chat: true,
              show_portal: true,
            },
          },
        },
      });

      if (!updateResponse.ok()) {
        const updateBody = await updateResponse.text();
        console.error(`Failed to update dev user: ${updateResponse.status()} - ${updateBody}`);
        return false;
      }

      console.log('Successfully updated dev user settings');
      return true;
    }

    console.error(`Dev user creation failed (${createResponse.status()}): ${createBody}`);
    return false;
  } catch (error) {
    console.error('Dev user creation failed:', error);
    return false;
  } finally {
    await context.dispose();
  }
}

async function tryLogin(email: string, password: string): Promise<boolean> {
  const context = await request.newContext();
  try {
    // Get CSRF token first
    const csrfResponse = await context.get(`${config.api_url}/csrf-token`);
    const csrfToken = csrfResponse.headers()['x-csrf-token'];

    const response = await context.post(`${config.api_url}/auth/login`, {
      headers: {
        'X-CSRF-Token': csrfToken || '',
      },
      data: {
        data: {
          attributes: { email, password },
        },
      },
    });
    return response.ok();
  } catch {
    return false;
  } finally {
    await context.dispose();
  }
}

async function tryApiRegister(): Promise<boolean> {
  const context = await request.newContext();
  try {
    // Get CSRF token first
    const csrfResponse = await context.get(`${config.api_url}/csrf-token`);
    const csrfToken = csrfResponse.headers()['x-csrf-token'];

    console.log('Attempting API registration...');
    const response = await context.post(`${config.api_url}/auth/register`, {
      headers: {
        'X-CSRF-Token': csrfToken || '',
      },
      data: {
        data: {
          attributes: {
            email: config.admin_email,
            name: config.admin_name,
            password: config.password,
          },
        },
      },
    });

    if (response.ok()) {
      // Verify we can now login (first user should be auto-verified)
      const canLogin = await tryLogin(config.admin_email, config.password);
      if (!canLogin) {
        console.log('API registration succeeded but login failed (email not verified?)');
        return false;
      }

      // Login to update user permissions (enable chat and portal)
      console.log('Updating user permissions to enable chat and portal...');
      const loginResponse = await context.post(`${config.api_url}/auth/login`, {
        headers: {
          'X-CSRF-Token': csrfToken || '',
        },
        data: {
          data: {
            attributes: {
              email: config.admin_email,
              password: config.password,
            },
          },
        },
      });

      if (!loginResponse.ok()) {
        console.log('Warning: Could not login to update permissions');
        return true; // Registration still succeeded
      }

      // Get user ID by listing users and finding by email
      const listResponse = await context.get(`${config.api_url}/api/v1/users`, {
        headers: {
          'X-CSRF-Token': csrfToken || '',
        },
      });

      if (!listResponse.ok()) {
        console.log('Warning: Could not list users to update permissions');
        return true; // Registration still succeeded
      }

      const usersData = await listResponse.json();
      const currentUser = usersData.data?.find(
        (u: { attributes?: { email?: string } }) => u.attributes?.email === config.admin_email
      );
      const userId = currentUser?.id;

      if (userId) {
        // Update user to enable chat and portal (include email as it's required for validation)
        const updateResponse = await context.patch(`${config.api_url}/api/v1/users/${userId}`, {
          headers: {
            'X-CSRF-Token': csrfToken || '',
          },
          data: {
            data: {
              type: 'users',
              id: userId,
              attributes: {
                email: config.admin_email,
                show_chat: true,
                show_portal: true,
              },
            },
          },
        });

        if (updateResponse.ok()) {
          console.log('User permissions updated (chat and portal enabled)');
        } else {
          console.log('Warning: Could not update user permissions');
        }
      }

      return true;
    }

    const body = await response.text();
    console.log(`API registration failed (${response.status()}): ${body}`);
    return false;
  } catch (error) {
    console.log('API registration error:', error);
    return false;
  } finally {
    await context.dispose();
  }
}

async function tryRegister(): Promise<boolean> {
  const browser = await chromium.launch();
  const page = await browser.newPage();

  try {
    await page.goto(`${config.base_url}/register`);
    await page.waitForTimeout(1000);

    // Check if we're on register page (not redirected)
    if (!page.url().includes('/register')) {
      console.log('UI registration page not accessible, skipping UI registration strategy');
      return false;
    }

    await page.getByLabel('Name').fill(config.admin_name);
    await page.getByLabel('Email address').fill(config.admin_email);
    await page.getByLabel('Password').fill(config.password);
    await page.getByRole('button', { name: 'Sign up' }).click();

    // Wait for redirect to login
    await page.waitForURL('**/login', { timeout: 10000 });

    // Verify we can now login (first user = verified)
    const canLogin = await tryLogin(config.admin_email, config.password);
    if (!canLogin) {
      console.log('Registration succeeded but login failed (not first user - email unverified)');
      return false;
    }

    return true;
  } catch (error) {
    console.log('Registration failed:', error);
    return false;
  } finally {
    await browser.close();
  }
}

async function createViaAPI(): Promise<boolean> {
  const context = await request.newContext();

  try {
    // First, get CSRF token
    const csrfResponse = await context.get(`${config.api_url}/csrf-token`);
    const csrfToken = csrfResponse.headers()['x-csrf-token'];
    console.log(`Got CSRF token: ${csrfToken ? 'yes' : 'no'}`);

    // Login as bootstrap admin with CSRF token
    const loginResponse = await context.post(`${config.api_url}/auth/login`, {
      headers: {
        'X-CSRF-Token': csrfToken || '',
      },
      data: {
        data: {
          attributes: {
            email: config.bootstrap_admin_email,
            password: config.bootstrap_admin_password,
          },
        },
      },
    });

    if (!loginResponse.ok()) {
      const body = await loginResponse.text();
      console.error(
        `Bootstrap admin login failed (${loginResponse.status()}): ${body}\n` +
          `URL: ${config.api_url}/auth/login\n` +
          `Email: ${config.bootstrap_admin_email}\n` +
          'Check BOOTSTRAP_ADMIN_EMAIL/BOOTSTRAP_ADMIN_PASSWORD env vars or create a bootstrap admin first.'
      );
      return false;
    }

    // Try to create test admin user via API
    const createResponse = await context.post(`${config.api_url}/api/v1/users`, {
      headers: {
        'X-CSRF-Token': csrfToken || '',
      },
      data: {
        data: {
          type: 'users',
          attributes: {
            email: config.admin_email,
            name: config.admin_name,
            password: config.password,
            is_admin: true,
            show_chat: true,
            show_portal: true,
            email_verified: true,
            notifications_enabled: true,
          },
        },
      },
    });

    if (createResponse.ok()) {
      return true;
    }

    const createBody = await createResponse.text();

    // If user already exists, try to update them to set email_verified = true
    if (createBody.includes('Email is already in use')) {
      console.log('User already exists, attempting to update email_verified status...');

      // First, get the user ID by listing users
      const listResponse = await context.get(`${config.api_url}/api/v1/users`, {
        headers: {
          'X-CSRF-Token': csrfToken || '',
        },
      });

      if (!listResponse.ok()) {
        console.error(`Failed to list users: ${listResponse.status()}`);
        return false;
      }

      const usersData = await listResponse.json();
      const existingUser = usersData.data?.find(
        (u: { attributes?: { email?: string } }) => u.attributes?.email === config.admin_email
      );

      if (!existingUser) {
        console.error('Could not find existing user in list');
        return false;
      }

      // Update the user to set email_verified = true
      const updateResponse = await context.patch(`${config.api_url}/api/v1/users/${existingUser.id}`, {
        headers: {
          'X-CSRF-Token': csrfToken || '',
        },
        data: {
          data: {
            type: 'users',
            id: existingUser.id,
            attributes: {
              email_verified: true,
              is_admin: true,
            },
          },
        },
      });

      if (!updateResponse.ok()) {
        const updateBody = await updateResponse.text();
        console.error(`Failed to update user: ${updateResponse.status()} - ${updateBody}`);
        return false;
      }

      console.log('Successfully updated user email_verified status');
      return true;
    }

    console.error(`API user creation failed (${createResponse.status()}): ${createBody}`);
    return false;
  } catch (error) {
    console.error('API user creation failed:', error);
    return false;
  } finally {
    await context.dispose();
  }
}

export default globalSetup;
